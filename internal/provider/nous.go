package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"

	"github.com/Apartment38/torii-gateway/internal/transport"
)

const (
	DefaultNousPortalURL    = "https://portal.nousresearch.com"
	DefaultNousInferenceURL = "https://inference-api.nousresearch.com/v1"
	DefaultNousClientID     = "hermes-agent"
	nousRefreshSkew         = 60 * time.Second
	maxNousCatalogSize      = 16 << 20
)

// NousCredential is the endpoint-bound OAuth state needed by the Portal
// inference API. Refresh tokens are rotated and must be persisted on refresh.
type NousCredential struct {
	AccessToken, RefreshToken, Endpoint string
	ExpiresAt                           time.Time
}

// NousDeviceCodeLogin performs the Portal's RFC 8628 device authorization
// flow. The callback receives the verification URL and user code for display.
func NousDeviceCodeLogin(ctx context.Context, portalBase, clientID, scope string, client *http.Client, onVerification func(string, string)) (NousCredential, error) {
	if client == nil {
		client = &http.Client{}
	}
	portalBase = strings.TrimRight(portalBase, "/")
	form := url.Values{"client_id": {clientID}}
	if scope != "" {
		form.Set("scope", scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, portalBase+"/api/oauth/device/code", strings.NewReader(form.Encode()))
	if err != nil {
		return NousCredential{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return NousCredential{}, fmt.Errorf("request Nous device code: %w", err)
	}
	var device struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_uri_complete"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := decodeJSONResponse(resp, &device); err != nil {
		return NousCredential{}, fmt.Errorf("Nous device code: %w", err)
	}
	if device.DeviceCode == "" || device.VerificationURL == "" || device.ExpiresIn <= 0 {
		return NousCredential{}, errors.New("Nous device response is missing required fields")
	}
	if onVerification != nil {
		onVerification(device.VerificationURL, device.UserCode)
	}
	interval := time.Duration(device.Interval) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return NousCredential{}, ctx.Err()
		case <-time.After(interval):
		}
		pollForm := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {clientID}, "device_code": {device.DeviceCode}}
		pollReq, err := http.NewRequestWithContext(ctx, http.MethodPost, portalBase+"/api/oauth/token", strings.NewReader(pollForm.Encode()))
		if err != nil {
			return NousCredential{}, err
		}
		pollReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		pollResp, err := client.Do(pollReq)
		if err != nil {
			return NousCredential{}, fmt.Errorf("poll Nous device code: %w", err)
		}
		var payload struct {
			AccessToken      string `json:"access_token"`
			RefreshToken     string `json:"refresh_token"`
			ExpiresIn        int    `json:"expires_in"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		pollBody, readErr := io.ReadAll(io.LimitReader(pollResp.Body, 1<<20))
		_ = pollResp.Body.Close()
		if readErr != nil {
			return NousCredential{}, fmt.Errorf("read Nous device token: %w", readErr)
		}
		if decodeErr := json.Unmarshal(pollBody, &payload); decodeErr != nil {
			return NousCredential{}, fmt.Errorf("decode Nous device token: %w", decodeErr)
		}
		if payload.AccessToken != "" {
			expires := time.Time{}
			if payload.ExpiresIn > 0 {
				expires = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
			}
			return NousCredential{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, ExpiresAt: expires}, nil
		}
		if payload.Error == "authorization_pending" {
			continue
		}
		if payload.Error == "slow_down" {
			interval += time.Second
			continue
		}
		return NousCredential{}, fmt.Errorf("Nous device authorization failed: %s", strings.TrimSpace(payload.ErrorDescription+" ("+payload.Error+")"))
	}
	return NousCredential{}, errors.New("Nous device authorization timed out")
}

func decodeJSONResponse(resp *http.Response, target any) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upstream returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target)
}

// Nous provides API-key access to the OpenAI-compatible Nous inference API.
// The legacy credential callback can still refresh when a saver is supplied.
type Nous struct {
	id, inferenceBase, portalBase, clientID string
	models                                  []string
	discover                                bool
	client                                  *http.Client
	load                                    func(string) (NousCredential, error)
	save                                    func(string, NousCredential) error
	refreshMu                               sync.Mutex
}

func NewNous(id, inferenceBase, portalBase, clientID string, models []string, discover bool, load func(string) (NousCredential, error), save func(string, NousCredential) error, client *http.Client) (*Nous, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	clonedClient := *client
	clonedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &clonedClient
	if load == nil {
		return nil, errors.New("Nous credential store is required")
	}
	if clientID == "" {
		clientID = DefaultNousClientID
	}
	if portalBase == "" {
		portalBase = DefaultNousPortalURL
	}
	if err := validateHTTPSOrigin(inferenceBase); err != nil {
		return nil, fmt.Errorf("Nous inference URL: %w", err)
	}
	if err := validateHTTPSOrigin(portalBase); err != nil {
		return nil, fmt.Errorf("Nous Portal URL: %w", err)
	}
	if !discover && len(models) == 0 {
		return nil, errors.New("static models are required when Nous discovery is disabled")
	}
	return &Nous{id: id, inferenceBase: strings.TrimRight(inferenceBase, "/"), portalBase: strings.TrimRight(portalBase, "/"), clientID: clientID, models: append([]string(nil), models...), discover: discover, client: client, load: load, save: save}, nil
}

func (p *Nous) ID() string { return p.id }

func (p *Nous) QuotaKey(string) string { return p.id + "/@account" }

func (p *Nous) Quota(ctx context.Context, _ string) (quota.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	credential, err := p.credential(ctx)
	if err != nil {
		return quota.Status{}, errors.New("Nous quota requires login")
	}
	cfg := TrackingConfig{Client: p.client, Headers: http.Header{"Authorization": {"Bearer " + credential.AccessToken}}}
	payload, err := trackingJSON(ctx, cfg, http.MethodGet, p.portalBase+"/api/oauth/account", nil)
	if err != nil {
		return quota.Status{Source: "nous_account", UnavailableReason: trackingReason(err)}, nil
	}
	return parseNousAccount(payload, time.Now().UTC()), nil
}

func (p *Nous) Models(ctx context.Context) ([]Model, error) {
	models := make([]Model, 0, len(p.models))
	seen := make(map[string]struct{})
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		models = append(models, Model{ID: p.id + "/" + id, Object: "model", OwnedBy: p.id})
	}
	for _, id := range p.models {
		add(id)
	}
	if !p.discover {
		return models, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.inferenceBase+"/models", nil)
	if err != nil {
		return nil, err
	}
	credential, err := p.credential(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ToriiGateway")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover Nous models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("discover Nous models: upstream returned %s", resp.Status)
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxNousCatalogSize + 1}
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(limited).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode Nous models: %w", err)
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("decode Nous models: response exceeds %d bytes", maxNousCatalogSize)
	}
	for _, raw := range envelope.Data {
		var model Model
		if json.Unmarshal(raw, &model) != nil || model.ID == "" {
			return nil, errors.New("decode Nous models: invalid model")
		}
		applyCatalogMetadata(raw, &model)
		upstreamID := model.ID
		model.ID, model.Object = p.id+"/"+model.ID, "model"
		if model.OwnedBy == "" {
			model.OwnedBy = p.id
		}
		if _, ok := seen[upstreamID]; ok {
			for i := range models {
				if models[i].ID == model.ID {
					models[i] = model
					break
				}
			}
			continue
		}
		seen[upstreamID] = struct{}{}
		models = append(models, model)
	}
	return models, nil
}

func (p *Nous) Forward(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	switch endpoint {
	case "/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/responses":
	default:
		return nil, fmt.Errorf("Nous does not support %s", endpoint)
	}
	credential, err := p.credential(ctx)
	if err != nil {
		return nil, err
	}
	transportClient, err := transport.NewOpenAICompatibleWithHeaders(p.inferenceBase, p.client, http.Header{"Authorization": []string{"Bearer " + credential.AccessToken}, "User-Agent": []string{"ToriiGateway"}})
	if err != nil {
		return nil, err
	}
	resp, err := transportClient.Do(ctx, strings.TrimPrefix(endpoint, "/v1"), body, model)
	if err != nil {
		return nil, fmt.Errorf("Nous request: %w", err)
	}
	if resp.StatusCode != http.StatusUnauthorized || credential.RefreshToken == "" {
		return resp, nil
	}
	_ = resp.Body.Close()
	if err := p.refresh(ctx, credential); err != nil {
		return nil, err
	}
	refreshed, err := p.load(p.id)
	if err != nil {
		return nil, err
	}
	transportClient, err = transport.NewOpenAICompatibleWithHeaders(p.inferenceBase, p.client, http.Header{"Authorization": []string{"Bearer " + refreshed.AccessToken}, "User-Agent": []string{"ToriiGateway"}})
	if err != nil {
		return nil, err
	}
	return transportClient.Do(ctx, strings.TrimPrefix(endpoint, "/v1"), body, model)
}

func (p *Nous) credential(ctx context.Context) (NousCredential, error) {
	credential, err := p.load(p.id)
	if err != nil {
		return NousCredential{}, fmt.Errorf("load Nous credential: %w", err)
	}
	if credential.Endpoint != p.inferenceBase {
		return NousCredential{}, errors.New("Nous credential endpoint binding does not match provider")
	}
	if credential.AccessToken == "" {
		return NousCredential{}, errors.New("Nous access token is empty")
	}
	if !credential.ExpiresAt.IsZero() && !credential.ExpiresAt.After(time.Now().Add(nousRefreshSkew)) && credential.RefreshToken != "" {
		if err := p.refresh(ctx, credential); err != nil {
			return NousCredential{}, err
		}
		credential, err = p.load(p.id)
		if err != nil {
			return NousCredential{}, err
		}
	}
	return credential, nil
}

func (p *Nous) refresh(ctx context.Context, credential NousCredential) error {
	if p.save == nil {
		return errors.New("Nous API key cannot be refreshed; replace it in the provider editor")
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	latest, err := p.load(p.id)
	if err == nil && latest.Endpoint == p.inferenceBase && latest.ExpiresAt.After(time.Now().Add(nousRefreshSkew)) {
		return nil
	}
	if credential.RefreshToken == "" {
		return errors.New("Nous credential cannot be refreshed without a refresh token")
	}
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {p.clientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.portalBase+"/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-nous-refresh-token", credential.RefreshToken)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh Nous credential: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh Nous credential: upstream returned %s; run auth login", resp.Status)
	}
	var payload struct {
		AccessToken  string      `json:"access_token"`
		RefreshToken string      `json:"refresh_token"`
		ExpiresIn    json.Number `json:"expires_in"`
		InferenceURL string      `json:"inference_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return fmt.Errorf("decode Nous refresh response: %w", err)
	}
	if payload.AccessToken == "" {
		return errors.New("Nous refresh response did not include access_token")
	}
	if payload.InferenceURL != "" && payload.InferenceURL != p.inferenceBase {
		return errors.New("Nous refresh response changed the bound inference endpoint")
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = credential.RefreshToken
	}
	expiresAt := time.Time{}
	if seconds, parseErr := strconv.ParseInt(payload.ExpiresIn.String(), 10, 64); parseErr == nil && seconds > 0 {
		expiresAt = time.Now().Add(time.Duration(seconds) * time.Second)
	}
	return p.save(p.id, NousCredential{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, Endpoint: p.inferenceBase, ExpiresAt: expiresAt})
}

func validateHTTPSOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must be an HTTPS URL without credentials, query, or fragment")
	}
	return nil
}
