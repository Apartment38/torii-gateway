// Package config loads and validates the gateway's YAML configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

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

// ProviderConfig contains the common fields needed to configure an Ollama
// provider. Provider-specific settings will be added with their providers.
type ProviderConfig struct {
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	BaseURL string `yaml:"base_url"`
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
	for i, provider := range c.Providers {
		field := fmt.Sprintf("providers[%d]", i)
		if !providerIDPattern.MatchString(provider.ID) {
			return fmt.Errorf("%s.id must match %s", field, providerIDPattern.String())
		}
		if _, exists := ids[provider.ID]; exists {
			return fmt.Errorf("%s.id %q is duplicated", field, provider.ID)
		}
		ids[provider.ID] = struct{}{}

		if provider.Type != "ollama" {
			return fmt.Errorf("%s.type %q is unsupported; only %q is currently supported", field, provider.Type, "ollama")
		}
		if err := validateBaseURL(provider.BaseURL); err != nil {
			return fmt.Errorf("%s.base_url: %w", field, err)
		}
	}

	return nil
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
