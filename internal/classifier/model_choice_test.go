package classifier

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/router"
)

func TestJevModelChoiceCriteriaAndValidation(t *testing.T) {
	price := .25
	request := ClassificationRequest{Text: "write a parser", Requirements: router.RequestRequirements{EstimatedInputTokens: 1000}, ExpectedOutputTokens: 2000, Candidates: []router.Candidate{
		{Model: catalog.Descriptor{PublicID: "local/cheap", Pricing: catalog.Pricing{InputPerMillion: &price, OutputPerMillion: &price}, Capabilities: catalog.ModelCapabilities{Tools: catalog.Supported}}, Intelligence: &intelligence.Model{Scores: map[string]float64{intelligence.Coding: .9}}},
		{Model: catalog.Descriptor{PublicID: "local/unknown"}},
	}}
	body, err := jevRequestBody("jev-latest", request, DecisionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		State     string                 `json:"state"`
		Questions map[string]jevQuestion `json:"questions"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.State != request.Text || len(envelope.Questions["model"].Criteria) != 2 {
		t.Fatalf("wrong choices: %s", body)
	}
	criterion := envelope.Questions["model"].Criteria["local/cheap"]
	for _, want := range []string{`"coding":0.9`, `"tools":"supported"`, `"estimated_request_cost_usd":0.00075`, `"expected_output_tokens":2000`} {
		if !strings.Contains(criterion, want) {
			t.Fatalf("missing evidence %s in %s", want, criterion)
		}
	}
	if !strings.Contains(envelope.Questions["model"].Criteria["local/unknown"], `"estimated_request_cost_usd":null`) {
		t.Fatal("missing price became zero")
	}
	var decision map[string]any
	_ = json.Unmarshal([]byte(layaDecision), &decision)
	answers := decision["answers"].(map[string]any)
	answers["model"] = map[string]any{"type": "choice", "choice": "local/cheap", "confidence": .1, "probabilities": map[string]float64{"local/cheap": 1, "local/unknown": 0}}
	encoded, _ := json.Marshal(decision)
	got, err := decodeJevResponse(response(string(encoded)), request)
	if err != nil || got.SelectedModel != "local/cheap" || got.Confidence != .1 || got.Decision.Choices["model"].Choice != "local/cheap" {
		t.Fatalf("decision: %+v %v", got, err)
	}
	answers["model"].(map[string]any)["choice"] = "other/rogue"
	encoded, _ = json.Marshal(decision)
	if _, err := decodeJevResponse(response(string(encoded)), request); err == nil {
		t.Fatal("out-of-pool model accepted")
	}
	request.Candidates = make([]router.Candidate, 251)
	if _, err := jevRequestBody("jev", request, DecisionOptions{}); err == nil {
		t.Fatal("oversized choice pool accepted")
	}
}
