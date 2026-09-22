package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Apartment38/torii-router/internal/config"
)

const validConfig = `
server:
  listen: 127.0.0.1:8080
state:
  directory: /state
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

func TestLoadConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	input := strings.Replace(validConfig, "  listen:", "  unknown: true\n  listen:", 1)
	_, err := config.Decode(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
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
