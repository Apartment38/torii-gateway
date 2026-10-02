package router

import (
	"math"
	"sort"
)

// IntelligenceWeights belongs to routing, not benchmark ingestion. All-zero
// weights preserve the existing selector's decision behavior.
type IntelligenceWeights struct {
	Capabilities        map[string]float64            `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	TaskWeights         map[string]map[string]float64 `yaml:"task_weights,omitempty" json:"task_weights,omitempty"`
	Price               float64                       `yaml:"price,omitempty" json:"price,omitempty"`
	Latency             float64                       `yaml:"latency,omitempty" json:"latency,omitempty"`
	TTFT                float64                       `yaml:"ttft,omitempty" json:"ttft,omitempty"`
	Throughput          float64                       `yaml:"throughput,omitempty" json:"throughput,omitempty"`
	Context             float64                       `yaml:"context,omitempty" json:"context,omitempty"`
	CostReference       float64                       `yaml:"cost_reference_usd,omitempty" json:"cost_reference_usd,omitempty"`
	LatencyReference    float64                       `yaml:"latency_reference_seconds,omitempty" json:"latency_reference_seconds,omitempty"`
	ThroughputReference float64                       `yaml:"throughput_reference,omitempty" json:"throughput_reference,omitempty"`
	MissingScore        *float64                      `yaml:"missing_score,omitempty" json:"missing_score,omitempty"`
}

func (w IntelligenceWeights) Enabled() bool {
	if w.Price+w.Latency+w.TTFT+w.Throughput+w.Context > 0 {
		return true
	}
	for _, value := range w.Capabilities {
		if value > 0 {
			return true
		}
	}
	for _, task := range w.TaskWeights {
		for _, value := range task {
			if value > 0 {
				return true
			}
		}
	}
	return false
}

func intelligenceRank(candidates []ranked, req RequestRequirements, weights IntelligenceWeights) bool {
	capabilities := weights.Capabilities
	if task, ok := weights.TaskWeights[req.TaskType]; ok {
		capabilities = task
	}
	missing := 0.5
	if weights.MissingScore != nil {
		missing = *weights.MissingScore
	}
	costRef := weights.CostReference
	if costRef <= 0 {
		costRef = 0.01
	}
	latencyRef := weights.LatencyReference
	if latencyRef <= 0 {
		latencyRef = 2
	}
	throughputRef := weights.ThroughputReference
	if throughputRef <= 0 {
		throughputRef = 100
	}
	anyEvidence := false
	for i := range candidates {
		candidate := &candidates[i]
		model := candidate.candidate.Intelligence
		var sum, total float64
		add := func(value float64, weight float64, known bool) {
			if weight <= 0 {
				return
			}
			if !known {
				value = missing
			} else if model != nil {
				anyEvidence = true
			}
			sum += value * weight
			total += weight
		}
		for _, category := range sortedCapabilityKeys(capabilities) {
			value, known := 0.0, false
			if model != nil {
				value, known = model.Scores[category]
			}
			add(value, capabilities[category], known)
		}
		operation := func(name string, reference float64, lower bool, weight float64) {
			value, known := 0.0, false
			if model != nil {
				value, known = model.Operations[name]
			}
			if name == "latency" && candidate.candidate.LatencyEWMA > 0 {
				value, known = candidate.candidate.LatencyEWMA.Seconds(), true
			}
			utility := value / (value + reference)
			if lower {
				utility = reference / (reference + value)
			}
			add(utility, weight, known)
		}
		operation("latency", latencyRef, true, weights.Latency)
		operation("ttft", latencyRef, true, weights.TTFT)
		operation("throughput", throughputRef, false, weights.Throughput)
		cost, known := 0.0, candidate.cost != nil
		if known {
			cost = *candidate.cost
		}
		add(costRef/(costRef+cost), weights.Price, known)
		context, contextKnown := 0.0, candidate.candidate.Model.ContextWindow != nil
		if contextKnown {
			needed := max(1, req.EstimatedInputTokens+req.RequestedOutputTokens)
			context = 1 - float64(needed)/float64(*candidate.candidate.Model.ContextWindow)
			context = math.Max(0, math.Min(1, context))
		}
		add(context, weights.Context, contextKnown)
		if total > 0 {
			value := sum / total
			candidate.intelligenceScore = &value
		}
	}
	if !anyEvidence {
		for i := range candidates {
			candidates[i].intelligenceScore = nil
		}
	}
	return anyEvidence
}

func sortedCapabilityKeys(weights map[string]float64) []string {
	keys := make([]string, 0, len(weights))
	for key := range weights {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func applyOperational(candidate Candidate, weights IntelligenceWeights) Candidate {
	if !weights.Enabled() || candidate.Intelligence == nil {
		return candidate
	}
	operations := candidate.Intelligence.Operations
	// Metadata remains authoritative for known price/subscription/local status.
	if candidate.Model.Pricing.InputPerMillion == nil {
		if value, ok := operations["input_price"]; ok {
			candidate.Model.Pricing.InputPerMillion = &value
		}
	}
	if candidate.Model.Pricing.OutputPerMillion == nil {
		if value, ok := operations["output_price"]; ok {
			candidate.Model.Pricing.OutputPerMillion = &value
		}
	}
	if value, ok := operations["context_length"]; ok && value >= 1 && value < float64(math.MaxInt64) {
		limit := int64(value)
		if candidate.Model.ContextWindow == nil || limit < *candidate.Model.ContextWindow {
			candidate.Model.ContextWindow = &limit
		}
	}
	return candidate
}
