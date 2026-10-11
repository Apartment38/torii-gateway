package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func buildProviderWithState(cfg config.ProviderConfig, databases ...*state.Database) (provider.OpenAIProvider, error) {
	p, err := buildInferenceProviderWithState(cfg, databases...)
	if err != nil {
		return nil, err
	}
	kind := cfg.Type
	u, _ := url.Parse(cfg.BaseURL)
	if kind == "openai_compatible" {
		kind = map[string]string{"opencode.ai": "opencode-zen", "api.stdcmpt.com": "standard-compute", "api.z.ai": "zai", "open.bigmodel.cn": "zai", "api.kimi.com": "kimi-coding", "api.minimax.io": "minimax", "api.minimaxi.com": "minimax", "api.deepseek.com": "deepseek", "api.x.ai": "xai"}[u.Hostname()]
		if u.Hostname() == "opencode.ai" && strings.Contains(cfg.APIPath+u.Path, "/go/") {
			kind = "opencode-go"
		}
	}
	t := cfg.Tracking
	if t != nil && t.Provider != "" {
		kind = t.Provider
	}
	switch kind {
	case "opencode-go", "opencode-zen", "standard-compute", "zai", "kimi-coding", "minimax", "deepseek", "xai", "anthropic":
	default:
		if t != nil {
			return nil, errors.New("tracking provider is not supported or could not be inferred")
		}
		return p, nil
	}
	tc := provider.TrackingConfig{Provider: kind, BaseURL: u.Scheme + "://" + u.Host, Headers: make(http.Header)}
	if kind == "xai" {
		tc.BaseURL = "https://management-api.x.ai"
	}
	var ref *config.SecretRef
	if kind == "xai" || kind == "anthropic" {
		ref = cfg.ManagementToken
	} else {
		ref = cfg.BearerToken
	}
	if t != nil {
		if t.BaseURL != "" {
			tc.BaseURL = t.BaseURL
		}
		tc.UsageURL, tc.TeamID, tc.Mode = t.UsageURL, t.TeamID, t.Mode
		if t.Token != nil {
			ref = t.Token
		}
	}
	if kind == "anthropic" && tc.Mode == "subscription" && ref == nil {
		ref = cfg.BearerToken
	}
	if kind == "zai" && (cfg.APIPath == "" || strings.Contains(cfg.APIPath, "/coding/")) {
		tc.Mode = "subscription"
	}
	if ref == cfg.BearerToken && ref != nil {
		for _, endpoint := range []string{tc.BaseURL, tc.UsageURL} {
			if endpoint == "" {
				continue
			}
			target, err := url.Parse(endpoint)
			if err != nil || target.Scheme != u.Scheme || target.Host != u.Host {
				return nil, errors.New("different tracking origin requires a separate tracking token")
			}
		}
	}
	header := "Authorization"
	bearer := true
	if kind == "anthropic" {
		tc.Headers.Set("anthropic-version", "2023-06-01")
		if tc.Mode == "subscription" {
			tc.Headers.Set("anthropic-beta", "oauth-2025-04-20")
		} else {
			header = "X-Api-Key"
			bearer = false
		}
	}
	client := &http.Client{Transport: http.DefaultTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if ref != nil {
		copyRef := *ref
		if len(databases) > 0 && databases[0] != nil {
			copyRef.BindDefault(databases[0])
			tc.Headers.Set(header, "database-managed")
			client.Transport = &secretTransport{base: http.DefaultTransport, headers: map[string]config.SecretRef{header: copyRef}, bearer: bearer}
		} else {
			value, err := copyRef.Resolve()
			if err != nil || value == "" || strings.ContainsAny(value, "\r\n") {
				return nil, errors.New("tracking credential is unavailable or invalid")
			}
			if bearer {
				value = "Bearer " + value
			}
			tc.Headers.Set(header, value)
		}
	}
	tc.Client = client
	return provider.WithTracking(p, tc), nil
}
