package config_test

import (
	"github.com/Apartment38/torii-gateway/internal/config"
	"strings"
	"testing"
)

func TestNamedRouterConfiguration(t *testing.T) {
	for _, name := range []string{"coding-local", "smart", "bad/name"} {
		input := validConfig + "smart_routers:\n  - model_name: " + name + "\n    providers: [ollama-local]\n    models: [ollama-local/example]\n    allow_paid_models: false\n    allow_free_models: true\n"
		cfg, err := config.Decode(strings.NewReader(input))
		if name != "coding-local" {
			if err == nil {
				t.Fatal("invalid router name accepted")
			}
			continue
		}
		if err != nil || len(cfg.SmartRouters) != 1 || *cfg.SmartRouters[0].AllowPaidModels || !*cfg.SmartRouters[0].AllowFreeModels {
			t.Fatalf("router configuration: %+v %v", cfg, err)
		}
	}
}
func TestNousRequiresAPIKey(t *testing.T) {
	input := "server:\n  listen: 127.0.0.1:8080\nproviders:\n  - id: nous\n    type: nous\n    base_url: https://inference-api.nousresearch.com/v1\n"
	if _, err := config.Decode(strings.NewReader(input)); err == nil {
		t.Fatal("missing API key accepted")
	}
	if _, err := config.Decode(strings.NewReader(input + "    bearer_token: {value: fixture-key}\n")); err != nil {
		t.Fatal(err)
	}
}
