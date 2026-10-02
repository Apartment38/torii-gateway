// Package intelligence ingests benchmark evidence independently of inference.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"
)

const (
	General     = "general_intelligence"
	Coding      = "coding"
	Agentic     = "agentic"
	Reasoning   = "reasoning"
	Math        = "math"
	Knowledge   = "knowledge"
	Instruction = "instruction_following"
	Multimodal  = "multimodal"
	Preference  = "preference"
	Operational = "operational"
)

var Categories = []string{General, Coding, Agentic, Reasoning, Math, Knowledge, Instruction, Multimodal, Preference}

// Observation retains the original measurement, even when no safe model match
// or score normalization exists. Metadata may include the upstream row.
type Observation struct {
	Source          string         `json:"source"`
	Benchmark       string         `json:"benchmark"`
	Category        string         `json:"category"`
	ExternalModelID string         `json:"external_model_id"`
	ModelID         string         `json:"model_id,omitempty"`
	Score           float64        `json:"original_score"`
	Normalized      *float64       `json:"normalized_score,omitempty"`
	Unit            string         `json:"unit,omitempty"`
	Version         string         `json:"version,omitempty"`
	EvaluatedAt     time.Time      `json:"evaluated_at,omitempty"`
	RetrievedAt     time.Time      `json:"retrieved_at"`
	DateBasis       string         `json:"date_basis"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type Result struct {
	Observations []Observation
	Metadata     map[string]any
}

// Source implementations own upstream retrieval, parsing, and category mapping.
// Matching uses the shared alias resolver so every adapter has identical safety.
type Source interface {
	ID() string
	Fetch(context.Context) (Result, error)
}

type Identity struct {
	CanonicalID string              `yaml:"canonical_id" json:"canonical_id"`
	ProviderIDs []string            `yaml:"provider_ids,omitempty" json:"provider_ids,omitempty"`
	SourceIDs   map[string][]string `yaml:"source_ids,omitempty" json:"source_ids,omitempty"`
	Aliases     []string            `yaml:"aliases,omitempty" json:"aliases,omitempty"`
}

type Mapping struct {
	Metric        string  `yaml:"metric,omitempty" json:"metric,omitempty"`
	Category      string  `yaml:"category" json:"category"`
	Min           float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max           float64 `yaml:"max" json:"max"`
	LowerIsBetter bool    `yaml:"lower_is_better,omitempty" json:"lower_is_better,omitempty"`
	Unit          string  `yaml:"unit,omitempty" json:"unit,omitempty"`
}

type Dataset struct {
	ID         string             `yaml:"dataset" json:"dataset"`
	Config     string             `yaml:"config" json:"config"`
	Split      string             `yaml:"split" json:"split"`
	ModelField string             `yaml:"model_field,omitempty" json:"model_field,omitempty"`
	DateField  string             `yaml:"date_field,omitempty" json:"date_field,omitempty"`
	Version    string             `yaml:"version,omitempty" json:"version,omitempty"`
	Metrics    map[string]Mapping `yaml:"metrics,omitempty" json:"metrics,omitempty"`
}

// SourceSettings has no credentials; callers resolve secrets through Torii's
// existing SecretRef abstraction and pass them separately to the factory.
type SourceSettings struct {
	ID               string             `yaml:"id" json:"id"`
	Type             string             `yaml:"type" json:"type"`
	Enabled          bool               `yaml:"enabled" json:"enabled"`
	URL              string             `yaml:"url,omitempty" json:"url,omitempty"`
	Datasets         []Dataset          `yaml:"datasets,omitempty" json:"datasets,omitempty"`
	MaxRows          int                `yaml:"max_rows,omitempty" json:"max_rows,omitempty"`
	IncludeEstimated bool               `yaml:"include_estimated,omitempty" json:"include_estimated,omitempty"`
	Metrics          map[string]Mapping `yaml:"metrics,omitempty" json:"metrics,omitempty"`
}

type Aggregation struct {
	SourceWeights    map[string]float64 `yaml:"source_weights,omitempty" json:"source_weights,omitempty"`
	BenchmarkWeights map[string]float64 `yaml:"benchmark_weights,omitempty" json:"benchmark_weights,omitempty"`
	OverallWeights   map[string]float64 `yaml:"overall_weights,omitempty" json:"overall_weights,omitempty"`
}

type Model struct {
	ID               string             `json:"id"`
	ProviderIDs      []string           `json:"provider_ids"`
	Scores           map[string]float64 `json:"scores"`
	Operations       map[string]float64 `json:"operations"`
	Observations     []Observation      `json:"observations"`
	ObservationCount int                `json:"observation_count"`
	Sources          []string           `json:"sources"`
	LatestEvaluation time.Time          `json:"latest_evaluation"`
	LatestRetrieval  time.Time          `json:"latest_retrieval"`
	Stale            bool               `json:"stale"`
	AgeSeconds       float64            `json:"age_seconds"`
	ValidUntil       time.Time          `json:"valid_until,omitempty"`
}

type Unmatched struct {
	Source     string   `json:"source"`
	ExternalID string   `json:"external_id"`
	Reason     string   `json:"reason"`
	Candidates []string `json:"candidates,omitempty"`
}

type SourceState struct {
	ID               string         `json:"id"`
	Type             string         `json:"type"`
	Enabled          bool           `json:"enabled"`
	LastAttempt      time.Time      `json:"last_attempt,omitempty"`
	LastSuccess      time.Time      `json:"last_success,omitempty"`
	Error            string         `json:"error,omitempty"`
	Stale            bool           `json:"stale"`
	AgeSeconds       float64        `json:"age_seconds"`
	Observations     []Observation  `json:"observations"`
	ObservationCount int            `json:"observation_count"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// RefreshEvent describes real source-level work, not an estimated byte count.
// Messages are generated locally and never contain upstream URLs or credentials.
type RefreshEvent struct {
	Phase        string `json:"phase"`
	Source       string `json:"source,omitempty"`
	Completed    int    `json:"completed"`
	Total        int    `json:"total"`
	Failed       int    `json:"failed"`
	Observations int    `json:"observations,omitempty"`
	Message      string `json:"message"`
}

type Snapshot struct {
	Version   int                    `json:"version"`
	UpdatedAt time.Time              `json:"updated_at"`
	Sources   map[string]SourceState `json:"sources"`
	Models    map[string]Model       `json:"models"`
	Unmatched []Unmatched            `json:"unmatched"`
}

func Normalize(score, min, max float64, lower bool) (*float64, error) {
	if math.IsNaN(score) || math.IsInf(score, 0) || math.IsNaN(min) || math.IsInf(min, 0) || math.IsNaN(max) || math.IsInf(max, 0) || max <= min || score < min || score > max {
		return nil, errors.New("score is outside its declared finite scale")
	}
	n := (score - min) / (max - min)
	if lower {
		n = 1 - n
	}
	return &n, nil
}

func validCategory(category string) bool {
	for _, c := range Categories {
		if c == category {
			return true
		}
	}
	return category == Operational || category == "unknown"
}

// cloneJSON protects immutable snapshots from callers mutating maps or pointers.
func cloneJSON[T any](v T) T {
	b, _ := json.Marshal(v)
	var copy T
	_ = json.Unmarshal(b, &copy)
	return copy
}
