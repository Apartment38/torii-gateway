package router_test

import (
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/router"
)

func TestBenchmarkTaskWeightsAndFallback(t *testing.T) {
	a, b := model("a/cheap"), model("b/strong-coder")
	b.Pricing.InputPerMillion = ptr(10.0)
	b.Pricing.OutputPerMillion = ptr(20.0)
	candidates := []router.Candidate{{Model: a, Intelligence: &intelligence.Model{Scores: map[string]float64{intelligence.Coding: .2, intelligence.Reasoning: .9}}}, {Model: b, Intelligence: &intelligence.Model{Scores: map[string]float64{intelligence.Coding: .95, intelligence.Reasoning: .1}}}}
	req := router.RequestRequirements{Endpoint: router.EndpointChat, TaskType: "coding", EstimatedInputTokens: 1000, RequestedOutputTokens: 100}
	policy := router.DefaultSelectionPolicy()
	policy.Intelligence = router.IntelligenceWeights{TaskWeights: map[string]map[string]float64{"coding": {intelligence.Coding: 4}, "reasoning": {intelligence.Reasoning: 4}}, Price: .1}
	decision, err := router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != b.PublicID {
		t.Fatalf("coding weights ignored: %+v %v", decision, err)
	}
	req.TaskType = "reasoning"
	decision, err = router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != a.PublicID {
		t.Fatal("reasoning weights ignored")
	}
	for i := range candidates {
		candidates[i].Intelligence = nil
	}
	req.TaskType = "coding"
	decision, err = router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != a.PublicID {
		t.Fatal("missing benchmarks changed fallback")
	}
	policy.Intelligence = router.IntelligenceWeights{}
	candidates[1].Intelligence = &intelligence.Model{Scores: map[string]float64{intelligence.Coding: 1}}
	decision, err = router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != a.PublicID {
		t.Fatal("zero weights changed routing")
	}
}

func TestBenchmarkOperationalWeightsAndHardRequirements(t *testing.T) {
	a, b := model("a/model"), model("b/model")
	candidates := []router.Candidate{{Model: a, Intelligence: &intelligence.Model{Scores: map[string]float64{intelligence.Agentic: 1}, Operations: map[string]float64{"latency": 10, "ttft": 8, "throughput": 10}}}, {Model: b, Intelligence: &intelligence.Model{Scores: map[string]float64{intelligence.Agentic: .8}, Operations: map[string]float64{"latency": 1, "ttft": .1, "throughput": 200}}}}
	req := router.RequestRequirements{Endpoint: router.EndpointChat, Tools: true}
	policy := router.DefaultSelectionPolicy()
	policy.Intelligence = router.IntelligenceWeights{Latency: 1, TTFT: 1, Throughput: 1}
	decision, err := router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != b.PublicID {
		t.Fatalf("performance weights ignored: %+v %v", decision, err)
	}
	candidates[0].LatencyEWMA = time.Millisecond
	policy.Intelligence = router.IntelligenceWeights{Latency: 1}
	decision, err = router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != a.PublicID {
		t.Fatal("measured gateway latency should supersede external latency")
	}
	candidates[0].QuotaExhausted = true
	decision, err = router.Select(candidates, req, "medium", policy)
	if err != nil || decision.Model.PublicID != b.PublicID {
		t.Fatal("benchmark weights bypassed exhausted quota")
	}
	candidates[1].Model.Capabilities.Tools = "unknown"
	if _, err = router.Select(candidates, req, "medium", policy); err == nil {
		t.Fatal("agentic benchmark score granted unverified tool support")
	}
}

func TestBenchmarkContextAndPricingNeverRelaxKnownMetadata(t *testing.T) {
	a := model("a/model")
	candidate := router.Candidate{Model: a, Intelligence: &intelligence.Model{Operations: map[string]float64{"context_length": 1000000, "input_price": 0, "output_price": 0}}}
	policy := router.DefaultSelectionPolicy()
	policy.Intelligence = router.IntelligenceWeights{Context: 1, Price: 1}
	req := router.RequestRequirements{Endpoint: router.EndpointChat, EstimatedInputTokens: 20000}
	if _, err := router.Select([]router.Candidate{candidate}, req, "medium", policy); err == nil {
		t.Fatal("external context relaxed verified hard limit")
	}
	req.EstimatedInputTokens = 1000
	policy.AllowPaidModels = false
	if _, err := router.Select([]router.Candidate{candidate}, req, "medium", policy); err == nil {
		t.Fatal("external pricing replaced known paid pricing")
	}
	candidate.Model.Pricing.InputPerMillion = nil
	candidate.Model.Pricing.OutputPerMillion = nil
	candidate.Model.ContextWindow = nil
	if decision, err := router.Select([]router.Candidate{candidate}, req, "medium", policy); err != nil || decision.EstimatedCost == nil || *decision.EstimatedCost != 0 {
		t.Fatal("operational evidence did not fill unknown metadata")
	}
}
