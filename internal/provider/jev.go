package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/transport"
)

const DefaultJevAPIURL = "https://api.typesafe.ai/v1"

// JevProvider discovers and forwards typed System One decision models.
type JevProvider struct {
	id, root, apiPath        string
	client                   *http.Client
	headers                  http.Header
	transport                *transport.OpenAICompatible
	models                   []string
	discover, healthFallback bool
	discoveryTimeout         time.Duration
	rewrites                 map[string]string
}

func NewJevProvider(cfg OpenAICompatibleConfig, healthFallback bool) (*JevProvider, error) {
	if err := ValidateID(cfg.ID); err != nil {
		return nil, err
	}
	root := strings.TrimRight(cfg.BaseURL, "/")
	apiPath := strings.TrimRight(cfg.APIPath, "/")
	if apiPath == "" {
		apiPath = "/v1"
	}
	if !strings.HasPrefix(apiPath, "/") || strings.ContainsAny(apiPath, "?#") {
		return nil, errors.New("classifier API path must be an absolute path without query or fragment")
	}
	// Accept either the service root or its /v1 API root without doubling /v1.
	parsed, err := url.Parse(root)
	if err != nil {
		return nil, errors.New("invalid classifier API URL")
	}
	if strings.HasSuffix(parsed.Path, apiPath) {
		parsed.Path = strings.TrimSuffix(parsed.Path, apiPath)
		parsed.RawPath = ""
		root = parsed.String()
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{}
	}
	cloned := *client
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if cfg.RequestTimeout > 0 {
		cloned.Timeout = cfg.RequestTimeout
	}
	client = &cloned
	t, err := transport.NewOpenAICompatibleWithHeaders(root, client, cfg.Headers)
	if err != nil {
		return nil, err
	}
	if !cfg.DiscoverModels && len(cfg.StaticModels) == 0 {
		return nil, errors.New("classifier models are required when discovery is disabled")
	}
	timeout := cfg.DiscoveryTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	rewrites := make(map[string]string, len(cfg.ModelRewrites))
	for alias, model := range cfg.ModelRewrites {
		rewrites[alias] = model
	}
	return &JevProvider{id: cfg.ID, root: root, apiPath: apiPath, client: client, headers: cfg.Headers.Clone(), transport: t, models: append([]string(nil), cfg.StaticModels...), discover: cfg.DiscoverModels, healthFallback: healthFallback, discoveryTimeout: timeout, rewrites: rewrites}, nil
}

func (p *JevProvider) ID() string           { return p.id }
func (p *JevProvider) ClassifierOnly() bool { return true }
func (p *JevProvider) Models(ctx context.Context) ([]Model, error) {
	if !p.discover {
		return p.publicModels(p.models)
	}
	ctx, cancel := context.WithTimeout(ctx, p.discoveryTimeout)
	defer cancel()
	data, status, err := p.get(ctx, p.apiPath+"/models")
	if err != nil {
		return nil, err
	}
	if (status == http.StatusNotFound || status == http.StatusMethodNotAllowed) && p.healthFallback {
		data, status, err = p.get(ctx, "/health")
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("classifier health endpoint returned HTTP %d", status)
		}
		var health struct {
			Status string   `json:"status"`
			Loaded []string `json:"loaded"`
		}
		if json.Unmarshal(data, &health) != nil || health.Status != "ok" || health.Loaded == nil {
			return nil, errors.New("classifier health response does not advertise loaded models")
		}
		return p.publicModels(health.Loaded)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("classifier model discovery returned HTTP %d", status)
	}
	var envelope struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return nil, errors.New("invalid classifier model catalog")
	}
	entries := envelope.Data
	if entries == nil {
		entries = envelope.Models
	}
	if entries == nil {
		return nil, errors.New("classifier catalog must contain data or models")
	}
	ids := make([]string, 0, len(entries))
	for _, raw := range entries {
		var id string
		if json.Unmarshal(raw, &id) != nil {
			var model struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Slug string `json:"slug"`
			}
			if json.Unmarshal(raw, &model) != nil {
				return nil, errors.New("invalid classifier model entry")
			}
			id = model.ID
			if id == "" {
				id = model.Name
			}
			if id == "" {
				id = model.Slug
			}
		}
		ids = append(ids, id)
	}
	return p.publicModels(ids)
}

func (p *JevProvider) publicModels(ids []string) ([]Model, error) {
	models := make([]Model, 0, len(ids))
	seen := make(map[string]bool)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == "unavailable" || strings.ContainsAny(id, "\r\n\t ") || seen[id] {
			continue
		}
		seen[id] = true
		for alias, upstream := range p.rewrites {
			if upstream == id {
				id = alias
				break
			}
		}
		model := Model{ID: p.id + "/" + id, Object: "model", OwnedBy: p.id}
		markDecisionModel(&model)
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, errors.New("no classifier models are available; load a checkpoint or configure provider models")
	}
	return models, nil
}

func (p *JevProvider) get(ctx context.Context, path string) ([]byte, int, error) {
	u, _ := url.Parse(p.root)
	u.Path = strings.TrimRight(u.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header = p.headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, errors.New("classifier discovery request failed (network or timeout)")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, 0, errors.New("classifier catalog unreadable or exceeds 1 MiB")
	}
	return data, resp.StatusCode, nil
}

func (p *JevProvider) ForwardDecision(ctx context.Context, body []byte) (*http.Response, error) {
	model := modelFromBody(body)
	if upstream, ok := p.rewrites[model]; ok {
		model = upstream
	}
	return p.transport.Do(ctx, p.apiPath+"/systemone", body, model)
}
func (p *JevProvider) Forward(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	if strings.TrimRight(endpoint, "/") != "/v1/systemone" {
		return nil, errors.New("decision providers only serve System One")
	}
	if upstream, ok := p.rewrites[model]; ok {
		model = upstream
	}
	return p.transport.Do(ctx, p.apiPath+"/systemone", body, model)
}

var _ OpenAIProvider = (*JevProvider)(nil)
var _ DecisionProvider = (*JevProvider)(nil)
