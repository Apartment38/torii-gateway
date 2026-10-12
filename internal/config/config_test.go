package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
)

const validConfig = `
server:
  listen: 127.0.0.1:8080
state:
  database_path: /data/torii.db
providers:
  - id: ollama-local
    type: ollama
    base_url: http://localhost:11434
`

func TestDecodeValidConfiguration(t *testing.T) {
	cfg, err := config.Decode(strings.NewReader(validConfig))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got := cfg.Providers[0].ID; got != "ollama-local" {
		t.Fatalf("provider ID = %q, want %q", got, "ollama-local")
	}
}

func TestOllamaRequestTimeout(t *testing.T) {
	for _, raw := range []string{"5m", "10m", "45s", "0s", "-1m", "forever"} {
		t.Run(raw, func(t *testing.T) {
			cfg, err := config.Decode(strings.NewReader(validConfig + "    request_timeout: " + raw + "\n"))
			if raw == "0s" || raw == "-1m" || raw == "forever" {
				if err == nil || !strings.Contains(err.Error(), "request_timeout must be a positive duration") {
					t.Fatalf("invalid timeout accepted: %v", err)
				}
				return
			}
			if err != nil || cfg.Providers[0].RequestTimeout != raw {
				t.Fatalf("Ollama request timeout = %q, error = %v", raw, err)
			}
		})
	}
}

func TestDecodeAllowsNoProviders(t *testing.T) {
	for _, input := range []string{
		"server:\n  listen: 127.0.0.1:8080\nproviders: []\n",
		"server:\n  listen: 127.0.0.1:8080\n",
	} {
		cfg, err := config.Decode(strings.NewReader(input))
		if err != nil || len(cfg.Providers) != 0 {
			t.Fatalf("empty provider configuration rejected: cfg=%+v err=%v", cfg, err)
		}
	}
}

func TestLoadConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestDecodeRejectsRemovedAuthenticationAndDirectory(t *testing.T) {
	for _, input := range []string{
		strings.Replace(validConfig, "state:\n", "  authentication:\n    api_key: {value: obsolete}\nstate:\n", 1),
		strings.Replace(validConfig, "database_path: /data/torii.db", "directory: /state", 1),
	} {
		if _, err := config.Decode(strings.NewReader(input)); err == nil {
			t.Fatal("removed configuration accepted")
		}
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	input := strings.Replace(validConfig, "  listen:", "  unknown: true\n  listen:", 1)
	_, err := config.Decode(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "unknown configuration field") {
		t.Fatalf("Decode() error = %v, want unknown-field error", err)
	}
}

func TestDecodeRejectsMultipleDocuments(t *testing.T) {
	_, err := config.Decode(strings.NewReader(validConfig + "---\n{}\n"))
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("Decode() error = %v, want multiple-document error", err)
	}
}

func TestDecodeValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		with    string
		want    string
	}{
		{name: "invalid listener", replace: "127.0.0.1:8080", with: "localhost", want: "server.listen"},
		{name: "invalid provider ID", replace: "ollama-local", with: "Ollama/Local", want: "providers[0].id"},
		{name: "unsupported type", replace: "type: ollama", with: "type: openai", want: "unsupported"},
		{name: "invalid base URL", replace: "http://localhost:11434", with: "localhost:11434", want: "scheme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Replace(validConfig, tt.replace, tt.with, 1)
			_, err := config.Decode(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Decode() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsDuplicateProviderIDs(t *testing.T) {
	input := validConfig + `  - id: ollama-local
    type: ollama
    base_url: https://ollama.example.test
`
	_, err := config.Decode(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("Decode() error = %v, want duplicate-ID error", err)
	}
}

func TestDecodeOpenAICompatibleProvider(t *testing.T) {
	input := strings.Replace(validConfig, `  - id: ollama-local
    type: ollama
    base_url: http://localhost:11434`, `  - id: private
    type: openai_compatible
    base_url: https://inference.example.test/api
    bearer_token:
      env: PROVIDER_TOKEN
    headers:
      X-Tenant:
        file: /run/secrets/tenant
    discover_models: false
    models: [vendor/model]
    model_rewrites:
      friendly: vendor/model
    transport: openai_compatible
    connect_timeout: 2s
    request_timeout: 5m
    discovery_timeout: 5s
    tls:
      ca_file: /run/secrets/ca.pem
      server_name: inference.example.test`, 1)
	cfg, err := config.Decode(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].BearerToken.Env != "PROVIDER_TOKEN" || cfg.Providers[0].Models[0] != "vendor/model" {
		t.Fatalf("provider = %#v", cfg.Providers[0])
	}
}

func TestDecodeRejectsUnsafeCompatibleProviderConfiguration(t *testing.T) {
	base := strings.Replace(validConfig, "type: ollama", "type: openai_compatible", 1)
	tests := []struct{ name, extra, want string }{
		{"inline unknown field", "    api_key: secret\n", "unknown configuration field"},
		{"unsafe authorization header", "    headers:\n      Authorization:\n        env: TOKEN\n", "unsafe"},
		{"no model source", "    discover_models: false\n", "models are required"},
		{"invalid timeout", "    connect_timeout: forever\n", "positive duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := base + tt.extra
			_, err := config.Decode(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodeJevProviderTypes(t *testing.T) {
	for _, profile := range []string{
		"type: jev\n    base_url: https://api.typesafe.ai/v1\n    bearer_token:\n      env: JEV_API_KEY",
		"type: jev_compatible\n    base_url: http://localhost:8000/v1",
	} {
		input := strings.Replace(validConfig, "type: ollama\n    base_url: http://localhost:11434", profile, 1)
		if _, err := config.Decode(strings.NewReader(input)); err != nil {
			t.Fatalf("valid decision provider rejected: %v", err)
		}
	}
	for _, profile := range []string{
		"type: jev\n    base_url: https://api.typesafe.ai/v1",
		"type: jev\n    base_url: http://api.typesafe.ai\n    bearer_token:\n      env: JEV_API_KEY",
		"type: jev\n    base_url: https://other.example\n    bearer_token:\n      env: JEV_API_KEY",
	} {
		input := strings.Replace(validConfig, "type: ollama\n    base_url: http://localhost:11434", profile, 1)
		if _, err := config.Decode(strings.NewReader(input)); err == nil {
			t.Fatal("unsafe hosted Jev configuration accepted")
		}
	}
}

func TestExplicitlyDisabledClassifierConfiguration(t *testing.T) {
	if _, err := config.Decode(strings.NewReader(validConfig + "router:\n  classifier:\n    type: disabled\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Decode(strings.NewReader(validConfig + "router:\n  classifier:\n    type: disabled\n    model: local/test\n")); err == nil {
		t.Fatal("disabled classifier accepted a model")
	}
}

func TestDecodeOpenAIProfile(t *testing.T) {
	input := strings.Replace(validConfig, "type: ollama\n    base_url: http://localhost:11434", "type: openai\n    base_url: https://api.openai.com\n    bearer_token:\n      env: OPENAI_API_KEY\n    organization: org_example\n    project: proj_example", 1)
	cfg, err := config.Decode(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].Type != "openai" || cfg.Providers[0].BearerToken.Env != "OPENAI_API_KEY" {
		t.Fatalf("provider = %#v", cfg.Providers[0])
	}
}

func TestDecodeOpenAIProfileRejectsCrossOriginAndGenericOptions(t *testing.T) {
	base := strings.Replace(validConfig, "type: ollama\n    base_url: http://localhost:11434", "type: openai\n    base_url: https://api.openai.com\n    bearer_token:\n      env: OPENAI_API_KEY", 1)
	tests := []struct {
		name, input, want string
	}{
		{name: "wrong origin", input: strings.Replace(base, "https://api.openai.com", "http://proxy.example", 1), want: "origin"},
		{name: "missing key", input: strings.Replace(base, "    bearer_token:\n      env: OPENAI_API_KEY", "", 1), want: "bearer_token is required"},
		{name: "model rewrites", input: base + "\n    model_rewrites: {alias: upstream}\n", want: "model rewrites"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Decode(strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodeRouterAndCodexConfiguration(t *testing.T) {
	input := validConfig + `
  - id: openai-codex
    type: openai-codex
    base_url: https://chatgpt.com/backend-api/codex
    discover_models: true
router:
  model_name: smart
  allow_paid_models: false
  classifier:
    type: model
    model: local/laya
    on_failure: capability_fallback
  sessions:
    inactivity_timeout: 30m
  quota:
    cache_ttl: 60s
  selection:
    context_utilization_limit: 0.85
    default_expected_output_tokens: 512
    location_preference: local
`
	cfg, err := config.Decode(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Router.Classifier.Model != "local/laya" || cfg.Providers[1].Type != "openai-codex" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestDisplayTimezoneValidation(t *testing.T) {
	for _, zone := range []string{"", "UTC", "America/New_York", "Asia/Tokyo", "Europe/London"} {
		cfg, err := config.Decode(strings.NewReader(strings.Replace(validConfig, "listen: 127.0.0.1:8080", "listen: 127.0.0.1:8080\n  timezone: '"+zone+"'", 1)))
		if err != nil || cfg.Server.Timezone != zone {
			t.Fatalf("zone %q: %v", zone, err)
		}
	}
	for _, zone := range []string{"Local", "Mars/Olympus", "/etc/passwd"} {
		if _, err := config.Decode(strings.NewReader(strings.Replace(validConfig, "listen: 127.0.0.1:8080", "listen: 127.0.0.1:8080\n  timezone: '"+zone+"'", 1))); err == nil {
			t.Fatal("accepted timezone", zone)
		}
	}
}
