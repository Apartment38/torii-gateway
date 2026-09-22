package provider_test

import (
	"strings"
	"testing"

	"github.com/Apartment38/torii-router/internal/provider"
)

func TestParseModelID(t *testing.T) {
	tests := []struct {
		name         string
		publicID     string
		wantProvider string
		wantModel    string
	}{
		{
			name:         "simple model ID",
			publicID:     "ollama-local/qwen3",
			wantProvider: "ollama-local",
			wantModel:    "qwen3",
		},
		{
			name:         "OpenRouter nested model ID",
			publicID:     "openrouter/google/gemini-2.5-pro",
			wantProvider: "openrouter",
			wantModel:    "google/gemini-2.5-pro",
		},
		{
			name:         "multiple upstream path components",
			publicID:     "openrouter/vendor/family/model",
			wantProvider: "openrouter",
			wantModel:    "vendor/family/model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerID, modelID, err := provider.ParseModelID(tt.publicID)
			if err != nil {
				t.Fatalf("ParseModelID(%q) error = %v", tt.publicID, err)
			}
			if providerID != tt.wantProvider || modelID != tt.wantModel {
				t.Fatalf("ParseModelID(%q) = (%q, %q), want (%q, %q)",
					tt.publicID, providerID, modelID, tt.wantProvider, tt.wantModel)
			}
		})
	}
}

func TestParseModelIDRejectsInvalidIDs(t *testing.T) {
	tests := []struct {
		name     string
		publicID string
		want     string
	}{
		{name: "missing separator", publicID: "qwen3", want: "<provider>/<model>"},
		{name: "empty provider", publicID: "/qwen3", want: "provider ID"},
		{name: "invalid provider", publicID: "OpenRouter/model", want: "provider ID"},
		{name: "empty model", publicID: "openrouter/", want: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerID, modelID, err := provider.ParseModelID(tt.publicID)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseModelID(%q) error = %v, want error containing %q", tt.publicID, err, tt.want)
			}
			if providerID != "" || modelID != "" {
				t.Fatalf("ParseModelID(%q) returned partial result (%q, %q) on error", tt.publicID, providerID, modelID)
			}
		})
	}
}

func TestValidateID(t *testing.T) {
	for _, id := range []string{"openrouter", "ollama-local", "provider_1", "provider.example"} {
		if err := provider.ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) error = %v", id, err)
		}
	}

	for _, id := range []string{"", "OpenRouter", "-provider", "provider/model", "provider id"} {
		if err := provider.ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) succeeded, want error", id)
		}
	}
}
