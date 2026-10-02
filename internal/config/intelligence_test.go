package config_test

import (
	"math"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/router"
)

func TestIntelligenceConfigurationStrictDecodeAndDefaults(t *testing.T) {
	yaml := `server:
  listen: 127.0.0.1:8080
state:
  directory: ./state
providers:
  - id: local
    type: ollama
    base_url: http://127.0.0.1:11434
intelligence:
  enabled: true
  refresh_interval: 24h
  source_timeout: 10m
  sources:
    - id: aa
      type: artificial_analysis
      enabled: true
      api_key:
        env: AA_API_KEY
    - id: hf
      type: huggingface
      enabled: true
      datasets:
        - dataset: org/results
          config: default
          split: train
          metrics:
            accuracy:
              category: knowledge
              max: 100
  identities:
    - canonical_id: model-a
      provider_ids: [local/model-a]
      aliases: [Model A]
      source_ids:
        aa: [external-id]
  aggregation:
    source_weights: {aa: 2}
    overall_weights: {preference: 0}
  routing:
    task_weights:
      coding: {coding: 3, agentic: 1}
    latency: 0.1
`
	cfg, err := config.Decode(strings.NewReader(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Intelligence.Sources[0].APIKey.Env != "AA_API_KEY" || !cfg.Intelligence.Routing.Enabled() {
		t.Fatal("benchmark configuration lost")
	}
	if cfg.Intelligence.Options().HalfLife <= 0 || cfg.Intelligence.Options().StaleAfter <= 0 {
		t.Fatal("freshness defaults missing")
	}
	if _, err := config.Decode(strings.NewReader(strings.Replace(yaml, "refresh_interval:", "unknown_option:", 1))); err == nil {
		t.Fatal("unknown benchmark field accepted")
	}
}

func TestIntelligenceConfigurationRejectsUnsafeOrAmbiguousSettings(t *testing.T) {
	ptr := func(value float64) *float64 { return &value }
	cases := []config.IntelligenceConfig{
		{RefreshInterval: "-1s"}, {Retries: func() *int { v := 9; return &v }()},
		{Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "same", Type: "benchlm"}}, {SourceSettings: intelligence.SourceSettings{ID: "same", Type: "benchlm"}}}},
		{Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "aa", Type: "artificial_analysis", Enabled: true}}}},
		{Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "hf", Type: "huggingface"}}}},
		{Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "bad", Type: "benchlm", URL: "https://user:secret@example.org"}}}},
		{Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "bad", Type: "benchlm", URL: "https://example.org?api_key=secret"}}}},
		{Aggregation: intelligence.Aggregation{SourceWeights: map[string]float64{"aa": math.NaN()}}},
		{Routing: router.IntelligenceWeights{Price: -1}}, {Routing: router.IntelligenceWeights{MissingScore: ptr(2)}},
		{Routing: router.IntelligenceWeights{Capabilities: map[string]float64{"not_a_capability": 1}}},
		{Identities: []intelligence.Identity{{CanonicalID: "a", ProviderIDs: []string{"local/model"}}, {CanonicalID: "b", ProviderIDs: []string{"local/model"}}}},
		{Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "bad", Type: "benchlm", Metrics: map[string]intelligence.Mapping{"score": {Category: intelligence.Coding, Max: 0}}}}}},
	}
	for index, cfg := range cases {
		if cfg.Validate() == nil {
			t.Fatalf("unsafe configuration %d accepted", index)
		}
	}
	if err := (config.IntelligenceConfig{}).Validate(); err != nil {
		t.Fatalf("backwards-compatible empty configuration rejected: %v", err)
	}
}
