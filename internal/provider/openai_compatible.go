package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Apartment38/torii-router/internal/transport"
)

// OpenAICompatibleConfig configures an arbitrary standard OpenAI-compatible
// endpoint. Header values must already be resolved by the configuration layer.
type OpenAICompatibleConfig struct {
	ID               string
	BaseURL          string
	Client           *http.Client
	Headers          http.Header
	DiscoverModels   bool
	StaticModels     []string
	ModelRewrites    map[string]string
	DiscoveryTimeout time.Duration
	RequestTimeout   time.Duration
	// APIPath identifies the upstream OpenAI-compatible API root relative to
	// BaseURL. The default is /v1; providers such as Gemini use a different
	// compatibility root.
	APIPath string
}

// Compatible is a generic transparent OpenAI-compatible provider.
type Compatible struct {
	id, baseURL string
	client      *http.Client
	transport   *transport.OpenAICompatible
	headers     http.Header
	discover    bool
	static      []string
	rewrites    map[string]string
	discoveryTO time.Duration
	apiPath     string
}

func NewOpenAICompatibleProvider(cfg OpenAICompatibleConfig) (*Compatible, error) {
	if err := ValidateID(cfg.ID); err != nil {
		return nil, err
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}
	if cfg.RequestTimeout > 0 {
		cloned := *cfg.Client
		cloned.Timeout = cfg.RequestTimeout
		cfg.Client = &cloned
	}
	t, err := transport.NewOpenAICompatibleWithHeaders(cfg.BaseURL, cfg.Client, cfg.Headers)
	if err != nil {
		return nil, err
	}
	if !cfg.DiscoverModels && len(cfg.StaticModels) == 0 {
		return nil, errors.New("static models are required when discovery is disabled")
	}
	if cfg.DiscoveryTimeout <= 0 {
		cfg.DiscoveryTimeout = modelDiscoveryTimeout
	}
	if cfg.APIPath == "" {
		cfg.APIPath = "/v1"
	}
	cfg.APIPath = "/" + strings.Trim(strings.TrimSpace(cfg.APIPath), "/")
	static := append([]string(nil), cfg.StaticModels...)
	rewrites := make(map[string]string, len(cfg.ModelRewrites))
	for k, v := range cfg.ModelRewrites {
		rewrites[k] = v
	}
	return &Compatible{id: cfg.ID, baseURL: cfg.BaseURL, client: cfg.Client, transport: t, headers: cfg.Headers.Clone(), discover: cfg.DiscoverModels, static: static, rewrites: rewrites, discoveryTO: cfg.DiscoveryTimeout, apiPath: cfg.APIPath}, nil
}

func (p *Compatible) ID() string { return p.id }

func (p *Compatible) Models(ctx context.Context) ([]Model, error) {
	models := make([]Model, 0, len(p.static))
	seen := make(map[string]struct{})
	add := func(id string, created int64, owned string) {
		public := id
		for alias, upstream := range p.rewrites {
			if upstream == id {
				public = alias
				break
			}
		}
		if _, ok := seen[public]; ok {
			return
		}
		seen[public] = struct{}{}
		if owned == "" {
			owned = p.id
		}
		models = append(models, Model{ID: p.id + "/" + public, Object: "model", Created: created, OwnedBy: owned})
	}
	for _, id := range p.static {
		add(id, 0, p.id)
	}
	if !p.discover {
		return models, nil
	}

	u, _ := url.Parse(p.baseURL)
	escaped := strings.TrimRight(u.EscapedPath(), "/") + p.apiPath + "/models"
	path, err := url.PathUnescape(escaped)
	if err != nil {
		return nil, err
	}
	u.Path, u.RawPath = path, escaped
	dctx, cancel := context.WithTimeout(ctx, p.discoveryTO)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for name, values := range p.headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("discover models: upstream returned %s", resp.Status)
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxModelCatalogSize + 1}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	dec := json.NewDecoder(limited)
	if err := dec.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("decode models: response must contain one JSON value")
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("decode models: response exceeds %d bytes", maxModelCatalogSize)
	}
	var entries []json.RawMessage
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, &entries) != nil || entries == nil {
		return nil, errors.New("decode models: data must be an array")
	}
	for _, raw := range entries {
		if trimmed := bytes.TrimSpace(raw); len(trimmed) < 2 || trimmed[0] != '{' {
			return nil, errors.New("decode models: each model must be an object")
		}
		var model Model
		if err := json.Unmarshal(raw, &model); err != nil || model.ID == "" {
			return nil, errors.New("decode models: model has empty or invalid ID")
		}
		add(model.ID, model.Created, model.OwnedBy)
	}
	return models, nil
}

func (p *Compatible) Forward(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	switch endpoint {
	case "/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/responses":
	default:
		return nil, fmt.Errorf("unsupported OpenAI-compatible endpoint %q", endpoint)
	}
	if rewritten, ok := p.rewrites[model]; ok {
		model = rewritten
	}
	upstreamEndpoint := p.apiPath + strings.TrimPrefix(endpoint, "/v1")
	return p.transport.Do(ctx, upstreamEndpoint, body, model)
}

// ForwardDecision forwards a Jev/System One request through the provider's
// configured origin and credentials. The provider must explicitly opt into
// this transport by implementing DecisionProvider.
func (p *Compatible) ForwardDecision(ctx context.Context, body []byte) (*http.Response, error) {
	return p.transport.Do(ctx, "/v1/systemone/", body, modelFromBody(body))
}

func modelFromBody(body []byte) string {
	var request struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &request) != nil {
		return "jev-latest"
	}
	if request.Model == "" {
		return "jev-latest"
	}
	return request.Model
}
