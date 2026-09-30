// Package config loads and validates the gateway's YAML configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Apartment38/torii-router/internal/provider"
	"gopkg.in/yaml.v3"
)

// Config is the configuration implemented by the current gateway milestone.
// Additional sections must not be added until their behavior is implemented;
// the decoder rejects keys that are not represented here.
type Config struct {
	Server    ServerConfig     `yaml:"server"`
	State     StateConfig      `yaml:"state"`
	Providers []ProviderConfig `yaml:"providers"`
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

// StateConfig identifies the directory used for persistent gateway state.
type StateConfig struct {
	Directory string `yaml:"directory"`
}

// ProviderConfig contains common and standard compatible provider settings.
type ProviderConfig struct {
	ID               string               `yaml:"id"`
	Type             string               `yaml:"type"`
	BaseURL          string               `yaml:"base_url"`
	BearerToken      *SecretRef           `yaml:"bearer_token,omitempty"`
	Headers          map[string]SecretRef `yaml:"headers,omitempty"`
	TLS              *TLSConfig           `yaml:"tls,omitempty"`
	Models           []string             `yaml:"models,omitempty"`
	DiscoverModels   *bool                `yaml:"discover_models,omitempty"`
	Transport        string               `yaml:"transport,omitempty"`
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
		return nil, fmt.Errorf("decode configuration: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("decode configuration: %w", err)
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
	if strings.TrimSpace(c.State.Directory) == "" {
		return errors.New("state.directory is required")
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

		if providerConfig.Type != "ollama" && providerConfig.Type != "openai_compatible" {
			return fmt.Errorf("%s.type %q is unsupported", field, providerConfig.Type)
		}
		if err := validateBaseURL(providerConfig.BaseURL); err != nil {
			return fmt.Errorf("%s.base_url: %w", field, err)
		}
		if providerConfig.Type == "ollama" && (providerConfig.BearerToken != nil || len(providerConfig.Headers) > 0 || providerConfig.TLS != nil || len(providerConfig.Models) > 0 || providerConfig.DiscoverModels != nil || providerConfig.Transport != "" || len(providerConfig.ModelRewrites) > 0 || providerConfig.ConnectTimeout != "" || providerConfig.RequestTimeout != "" || providerConfig.DiscoveryTimeout != "") {
			return fmt.Errorf("%s: generic provider options require type openai_compatible", field)
		}
		if providerConfig.Type == "openai_compatible" {
			if err := validateCompatibleProvider(providerConfig); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		}
	}

	return nil
}

func validateCompatibleProvider(p ProviderConfig) error {
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
		return fmt.Errorf("must be a valid URL: %w", err)
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
