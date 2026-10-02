// Package config loads and validates the gateway's YAML configuration.
package config

import (
	"errors"
	"fmt"
	"github.com/Apartment38/torii-gateway/internal/state"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/provider"
	"gopkg.in/yaml.v3"
)

// Config is the configuration implemented by the current gateway milestone.
// Additional sections must not be added until their behavior is implemented;
// the decoder rejects keys that are not represented here.
type Config struct {
	Credentials  *state.Database    `yaml:"-"`
	Server       ServerConfig       `yaml:"server"`
	State        StateConfig        `yaml:"state"`
	Providers    []ProviderConfig   `yaml:"providers"`
	Router       RouterConfig       `yaml:"router,omitempty"`
	Intelligence IntelligenceConfig `yaml:"intelligence,omitempty"`
}

type RouterConfig struct {
	ModelName       string           `yaml:"model_name,omitempty"`
	CatalogFile     string           `yaml:"catalog_file,omitempty"`
	AllowPaidModels *bool            `yaml:"allow_paid_models,omitempty"`
	Classifier      ClassifierConfig `yaml:"classifier,omitempty"`
	Sessions        SessionConfig    `yaml:"sessions,omitempty"`
	Quota           QuotaConfig      `yaml:"quota,omitempty"`
	Selection       SelectionConfig  `yaml:"selection,omitempty"`
}

type ClassifierConfig struct {
	Type      string     `yaml:"type,omitempty"`
	Model     string     `yaml:"model,omitempty"`
	Provider  string     `yaml:"provider,omitempty"`
	OnFailure string     `yaml:"on_failure,omitempty"`
	BaseURL   string     `yaml:"base_url,omitempty"`
	APIKey    *SecretRef `yaml:"api_key,omitempty"`
	Transport string     `yaml:"transport,omitempty"`
}
type SessionConfig struct {
	InactivityTimeout string `yaml:"inactivity_timeout,omitempty"`
	OnModelFailure    string `yaml:"on_model_failure,omitempty"`
}
type QuotaConfig struct {
	CacheTTL   string             `yaml:"cache_ttl,omitempty"`
	Allowances map[string]float64 `yaml:"allowances,omitempty"`
}
type SelectionConfig struct {
	ContextUtilizationLimit     float64            `yaml:"context_utilization_limit,omitempty"`
	DefaultExpectedOutputTokens int64              `yaml:"default_expected_output_tokens,omitempty"`
	LocationPreference          string             `yaml:"location_preference,omitempty"`
	MinimumQuality              map[string]float64 `yaml:"minimum_quality,omitempty"`
}

// ServerConfig controls the gateway listener.
type ServerConfig struct {
	Listen         string                      `yaml:"listen"`
	Authentication *ServerAuthenticationConfig `yaml:"authentication,omitempty"`
}

// ServerAuthenticationConfig identifies the secret used to authenticate
// clients. Authentication enforcement is implemented separately.
type ServerAuthenticationConfig struct {
	APIKey SecretRef `yaml:"api_key"`
}

// StateConfig identifies the database and optional legacy migration directory.
type StateConfig struct {
	Directory    string `yaml:"directory,omitempty"`
	DatabasePath string `yaml:"database_path,omitempty"`
}

// ProviderConfig contains common and standard compatible provider settings.
type ProviderConfig struct {
	ID               string               `yaml:"id"`
	Type             string               `yaml:"type"`
	BaseURL          string               `yaml:"base_url"`
	APIPath          string               `yaml:"api_path,omitempty"`
	PortalURL        string               `yaml:"portal_url,omitempty"`
	ClientID         string               `yaml:"client_id,omitempty"`
	BearerToken      *SecretRef           `yaml:"bearer_token,omitempty"`
	ManagementToken  *SecretRef           `yaml:"management_token,omitempty"`
	Headers          map[string]SecretRef `yaml:"headers,omitempty"`
	TLS              *TLSConfig           `yaml:"tls,omitempty"`
	Models           []string             `yaml:"models,omitempty"`
	DiscoverModels   *bool                `yaml:"discover_models,omitempty"`
	Transport        string               `yaml:"transport,omitempty"`
	Organization     string               `yaml:"organization,omitempty"`
	Project          string               `yaml:"project,omitempty"`
	ModelRewrites    map[string]string    `yaml:"model_rewrites,omitempty"`
	ConnectTimeout   string               `yaml:"connect_timeout,omitempty"`
	RequestTimeout   string               `yaml:"request_timeout,omitempty"`
	DiscoveryTimeout string               `yaml:"discovery_timeout,omitempty"`
}

// TLSConfig controls certificate verification for one provider origin.
type TLSConfig struct {
	CAFile     string `yaml:"ca_file,omitempty"`
	ServerName string `yaml:"server_name,omitempty"`
}

// Load reads, strictly decodes, and validates a YAML configuration file.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	defer f.Close()

	cfg, err := Decode(f)
	if err != nil {
		return nil, fmt.Errorf("load configuration %q: %w", path, err)
	}
	return cfg, nil
}

// Decode strictly decodes and validates one YAML document from r.
func Decode(r io.Reader) (*Config, error) {
	if r == nil {
		return nil, errors.New("decode configuration: nil reader")
	}

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("decode configuration: empty document")
		}
		return nil, errors.New("decode configuration: invalid YAML or unknown configuration field (input omitted for credential safety)")
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, errors.New("decode configuration: invalid trailing YAML (input omitted for credential safety)")
		}
		return nil, errors.New("decode configuration: multiple YAML documents are not allowed")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	return &cfg, nil
}

// Validate checks configuration invariants without performing network or
// filesystem access.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" {
		return errors.New("server.listen is required")
	}
	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		return fmt.Errorf("server.listen must be a host:port address: %w", err)
	}
	if c.Server.Authentication != nil {
		if err := c.Server.Authentication.APIKey.Validate(); err != nil {
			return fmt.Errorf("server.authentication.api_key: %w", err)
		}
	}
	if len(c.Providers) == 0 {
		return errors.New("at least one provider is required")
	}

	ids := make(map[string]struct{}, len(c.Providers))
	for i, providerConfig := range c.Providers {
		field := fmt.Sprintf("providers[%d]", i)
		if err := provider.ValidateID(providerConfig.ID); err != nil {
			return fmt.Errorf("%s.id: %w", field, err)
		}
		if _, exists := ids[providerConfig.ID]; exists {
			return fmt.Errorf("%s.id %q is duplicated", field, providerConfig.ID)
		}
		ids[providerConfig.ID] = struct{}{}

		if providerConfig.Type != "ollama" && providerConfig.Type != "openai_compatible" && providerConfig.Type != "openai" && providerConfig.Type != "openai-codex" && providerConfig.Type != "nous" && providerConfig.Type != "openrouter" && providerConfig.Type != "gemini" && providerConfig.Type != "xai" && providerConfig.Type != "anthropic" && providerConfig.Type != "jev" && providerConfig.Type != "jev_compatible" {
			return fmt.Errorf("%s.type %q is unsupported", field, providerConfig.Type)
		}
		if err := validateBaseURL(providerConfig.BaseURL); err != nil {
			return fmt.Errorf("%s.base_url: %w", field, err)
		}
		if providerConfig.Type == "ollama" && (providerConfig.BearerToken != nil || len(providerConfig.Headers) > 0 || providerConfig.TLS != nil || len(providerConfig.Models) > 0 || providerConfig.DiscoverModels != nil || providerConfig.Transport != "" || providerConfig.Organization != "" || providerConfig.Project != "" || len(providerConfig.ModelRewrites) > 0 || providerConfig.ConnectTimeout != "" || providerConfig.RequestTimeout != "" || providerConfig.DiscoveryTimeout != "") {
			return fmt.Errorf("%s: generic provider options require type openai_compatible", field)
		}
		if providerConfig.Type == "openai_compatible" || providerConfig.Type == "jev" || providerConfig.Type == "jev_compatible" {
			if err := validateCompatibleProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
			if providerConfig.Type == "jev" {
				parsed, _ := url.Parse(providerConfig.BaseURL)
				if parsed.Scheme != "https" || parsed.Host != "api.typesafe.ai" {
					return fmt.Errorf("%s: type jev requires https://api.typesafe.ai; use jev_compatible for other endpoints", field)
				}
				if providerConfig.BearerToken == nil {
					return fmt.Errorf("%s: type jev requires bearer_token", field)
				}
			}
		}
		if providerConfig.Type == "openrouter" || providerConfig.Type == "gemini" || providerConfig.Type == "xai" {
			if err := validateCompatibleProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
			if providerConfig.BearerToken == nil {
				return fmt.Errorf("%s: bearer_token is required", field)
			}
		}
		if providerConfig.Type != "openrouter" && providerConfig.ManagementToken != nil {
			return fmt.Errorf("%s: management_token requires type openrouter", field)
		}
		if providerConfig.Type == "openrouter" && providerConfig.ManagementToken != nil {
			if err := providerConfig.ManagementToken.Validate(); err != nil {
				return fmt.Errorf("%s: management_token: %w", field, err)
			}
		}
		if providerConfig.Type == "anthropic" {
			if err := validateAnthropicProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		}
		if providerConfig.Type == "openai" {
			if err := validateOpenAIProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		}
		if providerConfig.Type == "openai-codex" {
			if err := validateCodexProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		}
		if providerConfig.Type == "nous" {
			if err := validateNousProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		} else if providerConfig.PortalURL != "" || providerConfig.ClientID != "" {
			return fmt.Errorf("%s: portal_url and client_id require type nous", field)
		}
	}
	if err := c.Router.Validate(); err != nil {
		return fmt.Errorf("router: %w", err)
	}
	if err := c.Intelligence.Validate(); err != nil {
		return fmt.Errorf("intelligence: %w", err)
	}
	return nil
}

func validateNousProvider(p ProviderConfig) error {
	if p.BearerToken != nil || len(p.Headers) > 0 || p.TLS != nil || p.Transport != "" || p.Organization != "" || p.Project != "" || len(p.ModelRewrites) > 0 {
		return errors.New("Nous credentials and transport are managed by the provider")
	}
	if p.BaseURL == "" {
		return errors.New("base_url is required")
	}
	parsed, _ := url.Parse(p.BaseURL)
	if parsed.Scheme != "https" {
		return errors.New("base_url must use HTTPS")
	}
	if p.PortalURL != "" {
		portal, err := url.Parse(p.PortalURL)
		if err != nil || portal.Scheme != "https" || portal.Host == "" || portal.User != nil || portal.RawQuery != "" || portal.Fragment != "" {
			return errors.New("portal_url must be an HTTPS URL without credentials, query, or fragment")
		}
	}
	if p.DiscoverModels != nil && !*p.DiscoverModels && len(p.Models) == 0 {
		return errors.New("models are required when discover_models is false")
	}
	return nil
}

func (r RouterConfig) Validate() error {
	if r.ModelName != "" && r.ModelName != "smart" {
		return errors.New("model_name must be smart")
	}
	if strings.TrimSpace(r.CatalogFile) != "" && strings.ContainsAny(r.CatalogFile, "\r\n") {
		return errors.New("catalog_file must not contain newlines")
	}
	if r.Classifier.Type != "" && r.Classifier.Type != "disabled" && r.Classifier.Type != "model" && r.Classifier.Type != "laya" && r.Classifier.Type != "embedding" && r.Classifier.Type != "jev" {
		return fmt.Errorf("classifier.type %q is unsupported", r.Classifier.Type)
	}
	if r.Classifier.Type == "disabled" && (r.Classifier.Model != "" || r.Classifier.Provider != "" || r.Classifier.BaseURL != "" || r.Classifier.APIKey != nil || r.Classifier.Transport != "") {
		return errors.New("disabled classifier must not specify a model or connection")
	}
	if r.Classifier.Type == "model" || r.Classifier.Type == "laya" {
		if r.Classifier.Model == "" || r.Classifier.Model == "smart" {
			return errors.New("classifier.model must be a physical provider/model")
		}
	}
	if r.Classifier.Type == "embedding" && (r.Classifier.Provider == "" || r.Classifier.Model == "") {
		return errors.New("embedding classifier requires provider and model")
	}
	if r.Classifier.Type == "jev" {
		if r.Classifier.Transport == "" || r.Classifier.Transport == "direct" {
			if r.Classifier.APIKey == nil || r.Classifier.BaseURL == "" {
				return errors.New("direct Jev classifier requires base_url and api_key")
			}
			if err := validateBaseURL(r.Classifier.BaseURL); err != nil {
				return fmt.Errorf("classifier.base_url: %w", err)
			}
			parsed, _ := url.Parse(r.Classifier.BaseURL)
			if parsed.Scheme != "https" {
				return errors.New("classifier.base_url must use HTTPS")
			}
		} else if r.Classifier.Transport == "provider" {
			if r.Classifier.Provider == "" {
				return errors.New("Jev provider transport requires classifier.provider")
			}
			if r.Classifier.APIKey != nil || r.Classifier.BaseURL != "" {
				return errors.New("Jev provider transport uses the configured provider credentials")
			}
		} else {
			return fmt.Errorf("classifier.transport %q is unsupported", r.Classifier.Transport)
		}
	}
	if r.Classifier.OnFailure != "" && r.Classifier.OnFailure != "fail" && r.Classifier.OnFailure != "capability_fallback" {
		return fmt.Errorf("classifier.on_failure %q is unsupported", r.Classifier.OnFailure)
	}
	if r.Sessions.InactivityTimeout != "" {
		if d, err := time.ParseDuration(r.Sessions.InactivityTimeout); err != nil || d <= 0 {
			return errors.New("sessions.inactivity_timeout must be positive")
		}
	}
	if r.Quota.CacheTTL != "" {
		if d, err := time.ParseDuration(r.Quota.CacheTTL); err != nil || d <= 0 {
			return errors.New("quota.cache_ttl must be positive")
		}
	}
	for model, allowance := range r.Quota.Allowances {
		if strings.TrimSpace(model) == "" || allowance < 0 || math.IsNaN(allowance) || math.IsInf(allowance, 0) {
			return errors.New("quota.allowances must contain finite non-negative model limits")
		}
	}
	if r.Selection.LocationPreference != "" && r.Selection.LocationPreference != "neutral" && r.Selection.LocationPreference != "local" && r.Selection.LocationPreference != "cloud" {
		return errors.New("selection.location_preference is invalid")
	}
	if r.Sessions.OnModelFailure != "" && r.Sessions.OnModelFailure != "fail" && r.Sessions.OnModelFailure != "reroute" {
		return errors.New("sessions.on_model_failure is invalid")
	}
	for name, value := range r.Selection.MinimumQuality {
		if value < 0 || value > 1 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("selection.minimum_quality.%s must be finite and between zero and one", name)
		}
	}
	if r.Selection.ContextUtilizationLimit < 0 || r.Selection.ContextUtilizationLimit > 1 {
		return errors.New("selection.context_utilization_limit must be between zero and one")
	}
	return nil
}

func validateCodexProvider(p ProviderConfig) error {
	if p.BearerToken != nil || len(p.Headers) > 0 || p.TLS != nil {
		return errors.New("openai-codex credentials are managed by state, not provider headers")
	}
	if p.BaseURL == "" {
		return errors.New("base_url is required")
	}
	if err := validateBaseURL(p.BaseURL); err != nil {
		return fmt.Errorf("base_url: %w", err)
	}
	parsed, _ := url.Parse(p.BaseURL)
	if parsed.Scheme != "https" {
		return errors.New("base_url must use HTTPS")
	}
	// Subscription catalogs are always discovered after login. Models are an
	// optional fallback, not an allowlist; accept legacy discover_models: false.
	return nil
}

func validateOpenAIProvider(p ProviderConfig) error {
	parsed, err := url.Parse(p.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "api.openai.com" || parsed.Path != "" {
		return errors.New("type openai has an unsupported origin; base_url must be https://api.openai.com")
	}
	if p.BearerToken == nil {
		return errors.New("bearer_token is required")
	}
	if err := p.BearerToken.Validate(); err != nil {
		return fmt.Errorf("bearer_token: %w", err)
	}
	if len(p.Headers) != 0 {
		return errors.New("headers are not configurable for type openai; use organization or project")
	}
	if p.TLS != nil || len(p.ModelRewrites) != 0 || p.Transport != "" {
		return errors.New("TLS, model rewrites, and transport are not configurable for type openai")
	}
	if p.DiscoverModels != nil && !*p.DiscoverModels {
		return errors.New("discover_models must remain enabled for type openai")
	}
	if len(p.Models) != 0 {
		return errors.New("models are not configurable for type openai")
	}
	for name, value := range map[string]string{"organization": p.Organization, "project": p.Project} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s contains a newline", name)
		}
	}
	return validatePositiveDurations(p.ConnectTimeout, p.RequestTimeout, p.DiscoveryTimeout)
}

func validateAnthropicProvider(p ProviderConfig) error {
	parsed, err := url.Parse(p.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "api.anthropic.com" || parsed.Path != "" {
		return errors.New("type anthropic has an unsupported origin; base_url must be https://api.anthropic.com")
	}
	if p.BearerToken == nil {
		return errors.New("bearer_token is required")
	}
	if err := p.BearerToken.Validate(); err != nil {
		return fmt.Errorf("bearer_token: %w", err)
	}
	if len(p.Headers) != 0 || p.TLS != nil || p.APIPath != "" || p.Transport != "" || p.Organization != "" || p.Project != "" || len(p.ModelRewrites) != 0 {
		return errors.New("headers, TLS, api_path, transport, organization, project, and model rewrites are not configurable for type anthropic")
	}
	if p.DiscoverModels != nil && !*p.DiscoverModels && len(p.Models) == 0 {
		return errors.New("models are required when discover_models is false")
	}
	return validatePositiveDurations(p.ConnectTimeout, p.RequestTimeout, p.DiscoveryTimeout)
}

func validatePositiveDurations(values ...string) error {
	for _, raw := range values {
		if raw == "" {
			continue
		}
		if d, err := time.ParseDuration(raw); err != nil || d <= 0 {
			return errors.New("timeouts must be positive durations")
		}
	}
	return nil
}

func validateCompatibleProvider(p ProviderConfig) error {
	if p.APIPath != "" {
		parsed, err := url.Parse(p.APIPath)
		if err != nil || !strings.HasPrefix(p.APIPath, "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("api_path must be an absolute path without query or fragment")
		}
	}
	if p.Transport != "" && p.Transport != "openai_compatible" {
		return fmt.Errorf("transport %q is unsupported", p.Transport)
	}
	if p.BearerToken != nil {
		if err := p.BearerToken.Validate(); err != nil {
			return fmt.Errorf("bearer_token: %w", err)
		}
	}
	for name, ref := range p.Headers {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "" || !validCustomHeader(canonical) {
			return fmt.Errorf("header %q is unsafe or not canonical", name)
		}
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("header %q: %w", name, err)
		}
	}
	discover := true
	if p.DiscoverModels != nil {
		discover = *p.DiscoverModels
	}
	if !discover && len(p.Models) == 0 {
		return errors.New("models are required when discover_models is false")
	}
	seen := map[string]struct{}{}
	for _, model := range p.Models {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("model %q must be non-empty", model)
		}
		if _, ok := seen[model]; ok {
			return fmt.Errorf("model %q is duplicated", model)
		}
		seen[model] = struct{}{}
	}
	rewriteTargets := map[string]string{}
	for alias, upstream := range p.ModelRewrites {
		if strings.TrimSpace(alias) == "" || strings.TrimSpace(upstream) == "" {
			return fmt.Errorf("invalid model rewrite %q", alias)
		}
		if prior, ok := rewriteTargets[upstream]; ok {
			return fmt.Errorf("model rewrites %q and %q have the same upstream model", prior, alias)
		}
		rewriteTargets[upstream] = alias
	}
	for name, raw := range map[string]string{"connect_timeout": p.ConnectTimeout, "request_timeout": p.RequestTimeout, "discovery_timeout": p.DiscoveryTimeout} {
		if raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil || d <= 0 {
				return fmt.Errorf("%s must be a positive duration", name)
			}
		}
	}
	if p.TLS != nil && strings.TrimSpace(p.TLS.CAFile) == "" && strings.TrimSpace(p.TLS.ServerName) == "" {
		return errors.New("tls must configure ca_file or server_name")
	}
	return nil
}

func validCustomHeader(name string) bool {
	switch name {
	case "Authorization", "Host", "Content-Type", "Accept", "Connection", "Proxy-Connection", "Transfer-Encoding", "Upgrade", "X-Request-Id":
		return false
	}
	return !strings.HasPrefix(name, "Sec-") && !strings.ContainsAny(name, " \t\r\n:")
}

func validateBaseURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("must be a valid URL (input omitted for credential safety)")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("host is required")
	}
	if parsed.User != nil {
		return errors.New("must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must not contain a query or fragment")
	}
	return nil
}
