package router

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
)

type LocationPreference string

const (
	LocationNeutral LocationPreference = "neutral"
	LocationLocal   LocationPreference = "local"
	LocationCloud   LocationPreference = "cloud"
)

type SelectionPolicy struct {
	AllowPaidModels             bool
	MaxContextFraction          float64
	MinimumQuality              map[string]float64
	MinimumToolQuality          float64
	MinimumReasoningQuality     float64
	DefaultExpectedOutputTokens int64
	LocationPreference          LocationPreference
	Intelligence                IntelligenceWeights
}

func DefaultSelectionPolicy() SelectionPolicy {
	return SelectionPolicy{AllowPaidModels: true, MaxContextFraction: .85, MinimumQuality: map[string]float64{"low": .55, "medium": .70, "high": .82}, MinimumToolQuality: .55, MinimumReasoningQuality: .55, DefaultExpectedOutputTokens: 1024, LocationPreference: LocationNeutral}
}

type Candidate struct {
	Model          catalog.Descriptor
	QuotaKnown     bool
	QuotaRemaining float64
	QuotaExhausted bool
	LatencyEWMA    time.Duration
	Intelligence   *intelligence.Model
}
type Decision struct {
	Model         catalog.Descriptor
	EstimatedCost *float64
	Reasons       []ModelReason
}
type ModelReason struct {
	Model    string
	Eligible bool
	Reason   string
}

func Select(candidates []Candidate, requirements RequestRequirements, complexity string, policy SelectionPolicy) (Decision, error) {
	if len(candidates) == 0 {
		return Decision{}, errors.New("no candidate models")
	}
	if err := normalizePolicy(&policy, complexity); err != nil {
		return Decision{}, err
	}
	reasons := make([]ModelReason, 0, len(candidates))
	eligible := make([]ranked, 0, len(candidates))
	unknownQuality := make([]ranked, 0, len(candidates))
	for _, candidate := range candidates {
		rank, reason, knownQuality := evaluate(candidate, requirements, complexity, policy)
		if reason != "" {
			reasons = append(reasons, ModelReason{Model: candidate.Model.PublicID, Reason: reason})
			continue
		}
		if knownQuality {
			eligible = append(eligible, rank)
		} else {
			unknownQuality = append(unknownQuality, rank)
		}
	}
	if len(eligible) == 0 {
		eligible = unknownQuality
	} else {
		for _, rank := range unknownQuality {
			reasons = append(reasons, ModelReason{Model: rank.candidate.Model.PublicID, Reason: "known-quality eligible models are available"})
		}
	}
	if len(eligible) == 0 {
		return Decision{Reasons: reasons}, errors.New("no model satisfies request requirements")
	}
	benchmarkRanked := intelligenceRank(eligible, requirements, policy.Intelligence)
	sort.SliceStable(eligible, func(i, j int) bool {
		return less(eligible[i], eligible[j], policy.LocationPreference, requirements.LatencySensitive)
	})
	winner := eligible[0]
	sort.Slice(reasons, func(i, j int) bool { return reasons[i].Model < reasons[j].Model })
	reason := "selected by deterministic cost, quota, location, latency, context, and ID ordering"
	if benchmarkRanked {
		reason = "selected by configured benchmark capability and operational weights; deterministic tie-breaks applied"
	}
	reasons = append(reasons, ModelReason{Model: winner.candidate.Model.PublicID, Eligible: true, Reason: reason})
	return Decision{Model: winner.candidate.Model, EstimatedCost: winner.cost, Reasons: reasons}, nil
}

type ranked struct {
	candidate         Candidate
	cost              *float64
	headroom          int64
	intelligenceScore *float64
}

func evaluate(candidate Candidate, req RequestRequirements, complexity string, policy SelectionPolicy) (ranked, string, bool) {
	candidate = applyOperational(candidate, policy.Intelligence)
	m := candidate.Model
	if !m.Availability.Available {
		return ranked{}, "model is unavailable", false
	}
	if candidate.QuotaExhausted {
		return ranked{}, "quota is exhausted", false
	}
	if endpointState(m, req.Endpoint) != catalog.Supported {
		return ranked{}, "endpoint support is not verified", false
	}
	checks := []struct {
		name  string
		need  bool
		state catalog.TriState
	}{
		{"tools", req.Tools, m.Capabilities.Tools},
		{"vision", req.Vision, m.Capabilities.Vision},
		{"structured output", req.StructuredOutput, m.Capabilities.StructuredOutput},
		{"reasoning", req.Reasoning, m.Capabilities.Reasoning},
		{"streaming", req.Streaming, m.Capabilities.Streaming},
	}
	for _, check := range checks {
		if check.need && check.state != catalog.Supported {
			return ranked{}, check.name + " support is not verified", false
		}
	}
	headroom := int64(math.MaxInt64)
	if m.ContextWindow != nil {
		limit := int64(float64(*m.ContextWindow) * policy.MaxContextFraction)
		output := req.RequestedOutputTokens
		if output == 0 && req.Endpoint != EndpointEmbeddings {
			output = policy.DefaultExpectedOutputTokens
		}
		if req.EstimatedInputTokens < 0 || output < 0 || req.EstimatedInputTokens > limit || output > limit-req.EstimatedInputTokens {
			return ranked{}, "context headroom is insufficient", false
		}
		needed := req.EstimatedInputTokens + output
		headroom = *m.ContextWindow - needed
	}
	if !policy.AllowPaidModels && !knownFree(m) {
		return ranked{}, "model is not positively known to be free", false
	}
	quality, known := m.Quality.General, true
	if quality == nil {
		known = false
	} else if *quality < policy.MinimumQuality[complexity] {
		return ranked{}, "general quality is below the configured threshold", true
	}
	if req.Tools && m.Quality.Tools != nil && *m.Quality.Tools < policy.MinimumToolQuality {
		return ranked{}, "tool quality is below the configured threshold", true
	}
	if req.Reasoning && m.Quality.Reasoning != nil && *m.Quality.Reasoning < policy.MinimumReasoningQuality {
		return ranked{}, "reasoning quality is below the configured threshold", true
	}
	return ranked{candidate: candidate, cost: estimateCost(m, req, policy), headroom: headroom}, "", known
}
func endpointState(m catalog.Descriptor, e Endpoint) catalog.TriState {
	switch e {
	case EndpointChat:
		return m.Endpoints.ChatCompletions
	case EndpointCompletions:
		return m.Endpoints.Completions
	case EndpointResponses:
		return m.Endpoints.Responses
	case EndpointEmbeddings:
		return m.Endpoints.Embeddings
	}
	return catalog.Unsupported
}
func knownFree(m catalog.Descriptor) bool {
	return m.Pricing.InputPerMillion != nil && m.Pricing.OutputPerMillion != nil && *m.Pricing.InputPerMillion == 0 && *m.Pricing.OutputPerMillion == 0
}
func estimateCost(m catalog.Descriptor, r RequestRequirements, p SelectionPolicy) *float64 {
	if m.Pricing.InputPerMillion == nil || m.Pricing.OutputPerMillion == nil {
		return nil
	}
	output := r.RequestedOutputTokens
	if output == 0 && r.Endpoint != EndpointEmbeddings {
		output = p.DefaultExpectedOutputTokens
	}
	v := (float64(r.EstimatedInputTokens)*(*m.Pricing.InputPerMillion) + float64(output)*(*m.Pricing.OutputPerMillion)) / 1_000_000
	return &v
}
func less(a, b ranked, preference LocationPreference, latencySensitive bool) bool {
	if latencySensitive && a.candidate.LatencyEWMA != b.candidate.LatencyEWMA {
		if a.candidate.LatencyEWMA == 0 {
			return false
		}
		if b.candidate.LatencyEWMA == 0 {
			return true
		}
		return a.candidate.LatencyEWMA < b.candidate.LatencyEWMA
	}
	if a.intelligenceScore != nil && b.intelligenceScore != nil && *a.intelligenceScore != *b.intelligenceScore {
		return *a.intelligenceScore > *b.intelligenceScore
	}
	if a.cost != nil || b.cost != nil {
		if a.cost == nil {
			return false
		}
		if b.cost == nil {
			return true
		}
		if *a.cost != *b.cost {
			return *a.cost < *b.cost
		}
	}
	if a.candidate.QuotaKnown || b.candidate.QuotaKnown {
		if !a.candidate.QuotaKnown {
			return false
		}
		if !b.candidate.QuotaKnown {
			return true
		}
		if a.candidate.QuotaRemaining != b.candidate.QuotaRemaining {
			return a.candidate.QuotaRemaining > b.candidate.QuotaRemaining
		}
	}
	if preference != LocationNeutral {
		al, bl := isLocal(a.candidate.Model), isLocal(b.candidate.Model)
		if al != bl {
			if preference == LocationLocal {
				return al
			}
			return !al
		}
	}
	if a.candidate.LatencyEWMA != b.candidate.LatencyEWMA {
		if a.candidate.LatencyEWMA == 0 {
			return false
		}
		if b.candidate.LatencyEWMA == 0 {
			return true
		}
		return a.candidate.LatencyEWMA < b.candidate.LatencyEWMA
	}
	if a.headroom != b.headroom {
		return a.headroom > b.headroom
	}
	return a.candidate.Model.PublicID < b.candidate.Model.PublicID
}
func isLocal(m catalog.Descriptor) bool { return m.Local != nil && *m.Local }
func normalizePolicy(p *SelectionPolicy, complexity string) error {
	defaults := DefaultSelectionPolicy()
	if p.MaxContextFraction <= 0 || p.MaxContextFraction > 1 {
		p.MaxContextFraction = defaults.MaxContextFraction
	}
	if p.MinimumQuality == nil {
		p.MinimumQuality = defaults.MinimumQuality
	}
	if _, ok := p.MinimumQuality[complexity]; !ok {
		return fmt.Errorf("no quality threshold for complexity %q", complexity)
	}
	if p.DefaultExpectedOutputTokens <= 0 {
		p.DefaultExpectedOutputTokens = defaults.DefaultExpectedOutputTokens
	}
	if p.LocationPreference != "" && p.LocationPreference != LocationNeutral && p.LocationPreference != LocationLocal && p.LocationPreference != LocationCloud {
		return fmt.Errorf("invalid location preference %q", p.LocationPreference)
	}
	if p.LocationPreference == "" {
		p.LocationPreference = LocationNeutral
	}
	return nil
}
