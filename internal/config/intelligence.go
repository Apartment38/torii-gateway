package config

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
)

type IntelligenceConfig struct {
	Enabled           bool                       `yaml:"enabled,omitempty"`
	RefreshInterval   string                     `yaml:"refresh_interval,omitempty"`
	HTTPTimeout       string                     `yaml:"http_timeout,omitempty"`
	SourceTimeout     string                     `yaml:"source_timeout,omitempty"`
	Retries           *int                       `yaml:"retries,omitempty"`
	StaleAfter        string                     `yaml:"stale_after,omitempty"`
	MaxAge            string                     `yaml:"max_age,omitempty"`
	FreshnessHalfLife string                     `yaml:"freshness_half_life,omitempty"`
	Sources           []BenchmarkSourceConfig    `yaml:"sources,omitempty"`
	Identities        []intelligence.Identity    `yaml:"identities,omitempty"`
	Aggregation       intelligence.Aggregation   `yaml:"aggregation,omitempty"`
	Routing           router.IntelligenceWeights `yaml:"routing,omitempty"`
}
type BenchmarkSourceConfig struct {
	intelligence.SourceSettings `yaml:",inline"`
	APIKey                      *SecretRef `yaml:"api_key,omitempty"`
}

func (c IntelligenceConfig) Validate() error {
	for name, raw := range map[string]string{"refresh_interval": c.RefreshInterval, "http_timeout": c.HTTPTimeout, "source_timeout": c.SourceTimeout, "stale_after": c.StaleAfter, "max_age": c.MaxAge, "freshness_half_life": c.FreshnessHalfLife} {
		if raw != "" {
			if value, err := time.ParseDuration(raw); err != nil || value <= 0 {
				return fmt.Errorf("%s must be a positive Go duration", name)
			}
		}
	}
	if c.Retries != nil && (*c.Retries < 0 || *c.Retries > 5) {
		return errors.New("retries must be between 0 and 5")
	}
	if len(c.Sources) > 32 {
		return errors.New("at most 32 benchmark sources may be configured")
	}
	seen := map[string]bool{}
	for _, source := range c.Sources {
		if provider.ValidateID(source.ID) != nil || seen[source.ID] {
			return errors.New("source IDs must be valid and unique")
		}
		seen[source.ID] = true
		if !intelligence.Builtins().Has(source.Type) {
			return fmt.Errorf("source %q has unsupported type", source.ID)
		}
		if source.URL != "" {
			if err := intelligence.ValidateURL(source.URL); err != nil {
				return fmt.Errorf("source %q: %w", source.ID, err)
			}
		}
		if source.APIKey != nil {
			if err := source.APIKey.Validate(); err != nil {
				return fmt.Errorf("source %q API key: %w", source.ID, err)
			}
		}
		if source.Enabled && source.Type == "artificial_analysis" && source.APIKey == nil {
			return fmt.Errorf("source %q requires api_key", source.ID)
		}
		if source.MaxRows < 0 || source.MaxRows > 100000 {
			return errors.New("max_rows must be between 1 and 100000, or omitted")
		}
		if source.Type == "huggingface" && len(source.Datasets) == 0 {
			return errors.New("Hugging Face source requires datasets")
		}
		if len(source.Datasets) > 8 {
			return errors.New("at most eight datasets may be configured per source")
		}
		for _, dataset := range source.Datasets {
			if strings.TrimSpace(dataset.ID) == "" || dataset.Config == "" || dataset.Split == "" {
				return errors.New("datasets require dataset, config and split")
			}
			if source.Type == "huggingface" && len(dataset.Metrics) == 0 {
				return errors.New("Hugging Face datasets require explicit metric mappings and scales")
			}
			for _, mapping := range dataset.Metrics {
				if err := validateMetric(mapping); err != nil {
					return err
				}
			}
		}
		for _, mapping := range source.Metrics {
			if err := validateMetric(mapping); err != nil {
				return err
			}
		}
	}
	canonical := map[string]bool{}
	physical := map[string]bool{}
	for _, identity := range c.Identities {
		if strings.TrimSpace(identity.CanonicalID) == "" || canonical[identity.CanonicalID] {
			return errors.New("canonical model IDs must be nonempty and unique")
		}
		canonical[identity.CanonicalID] = true
		for _, id := range identity.ProviderIDs {
			if _, _, err := provider.ParseModelID(id); err != nil || physical[id] {
				return errors.New("identity provider IDs must be valid and cannot belong to multiple canonical models")
			}
			physical[id] = true
		}
	}
	for _, weights := range []map[string]float64{c.Aggregation.SourceWeights, c.Aggregation.BenchmarkWeights, c.Aggregation.OverallWeights, c.Routing.Capabilities} {
		if err := validateWeights(weights); err != nil {
			return err
		}
	}
	for _, weights := range c.Routing.TaskWeights {
		if err := validateWeights(weights); err != nil {
			return err
		}
		for category := range weights {
			if !validScoreCategory(category) {
				return fmt.Errorf("unknown routing category %q", category)
			}
		}
	}
	for category := range c.Routing.Capabilities {
		if !validScoreCategory(category) {
			return fmt.Errorf("unknown routing category %q", category)
		}
	}
	for category := range c.Aggregation.OverallWeights {
		if !validScoreCategory(category) || category == "overall" {
			return fmt.Errorf("unknown overall category %q", category)
		}
	}
	if err := validateWeights(map[string]float64{"price": c.Routing.Price, "latency": c.Routing.Latency, "ttft": c.Routing.TTFT, "throughput": c.Routing.Throughput, "context": c.Routing.Context, "cost_reference": c.Routing.CostReference, "latency_reference": c.Routing.LatencyReference, "throughput_reference": c.Routing.ThroughputReference}); err != nil {
		return err
	}
	if c.Routing.MissingScore != nil && (math.IsNaN(*c.Routing.MissingScore) || *c.Routing.MissingScore < 0 || *c.Routing.MissingScore > 1) {
		return errors.New("missing_score must be between 0 and 1")
	}
	return nil
}
func validateMetric(m intelligence.Mapping) error {
	if m.Category == intelligence.Operational {
		units := map[string]string{"input_price": "USD_per_million_tokens", "output_price": "USD_per_million_tokens", "latency": "seconds", "ttft": "seconds", "throughput": "tokens_per_second", "context_length": "tokens"}
		if units[m.Metric] == "" || units[m.Metric] != m.Unit {
			return errors.New("operational mappings require a canonical metric and its unit")
		}
		return nil
	}
	if !validScoreCategory(m.Category) || m.Category == "overall" {
		return errors.New("metric mapping has unsupported category")
	}
	if math.IsNaN(m.Min) || math.IsNaN(m.Max) || math.IsInf(m.Min, 0) || math.IsInf(m.Max, 0) || m.Max <= m.Min {
		return errors.New("metric mapping requires a finite max greater than min")
	}
	return nil
}
func validScoreCategory(category string) bool {
	if category == "overall" {
		return true
	}
	for _, c := range intelligence.Categories {
		if c == category {
			return true
		}
	}
	return false
}
func validateWeights(weights map[string]float64) error {
	for name, value := range weights {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1000000 {
			return fmt.Errorf("weight %q must be finite and between 0 and 1000000", name)
		}
	}
	return nil
}

func (c IntelligenceConfig) Options() intelligence.Options {
	duration := func(raw string) time.Duration { value, _ := time.ParseDuration(raw); return value }
	options := intelligence.Options{RefreshInterval: duration(c.RefreshInterval), SourceTimeout: duration(c.SourceTimeout), StaleAfter: duration(c.StaleAfter), MaxAge: duration(c.MaxAge), HalfLife: duration(c.FreshnessHalfLife), Aggregation: c.Aggregation, Identities: c.Identities}
	for _, source := range c.Sources {
		options.Sources = append(options.Sources, source.SourceSettings)
	}
	return options.Defaults()
}
