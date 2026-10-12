package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

type modelDecisionFixture struct {
	t     *testing.T
	calls int
}

func (f *modelDecisionFixture) ForwardDecision(_ context.Context, body []byte) (*http.Response, error) {
	f.calls++
	var request struct {
		State     string `json:"state"`
		Questions map[string]struct {
			Criteria map[string]string `json:"criteria"`
		} `json:"questions"`
	}
	_ = json.Unmarshal(body, &request)
	criteria := request.Questions["model"].Criteria
	if request.State != "hi" || len(criteria) != 2 || criteria["critic/a"] == "" || criteria["critic/b"] == "" {
		f.t.Fatalf("wrong routing criteria: %s", body)
	}
	var response map[string]any
	_ = json.Unmarshal([]byte(criticTypedDecision(f.t, false)), &response)
	answers := response["answers"].(map[string]any)
	answers["tools"] = map[string]any{"type": "noul", "noul": 0}
	answers["model"] = map[string]any{"type": "choice", "choice": "critic/b", "confidence": 1, "probabilities": map[string]float64{"critic/a": 0, "critic/b": 1}}
	encoded, _ := json.Marshal(response)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
}

var _ provider.DecisionProvider = (*modelDecisionFixture)(nil)

func TestJevSelectedDestinationHonorsConfiguredPoolAndReevaluatesPrompts(t *testing.T) {
	p := &criticProvider{models: []string{"a", "b", "outside"}}
	a, b, outside := criticModel("a"), criticModel("b"), criticModel("outside")
	a.Pricing.InputPerMillion = criticPtr(0.0)
	a.Pricing.OutputPerMillion = criticPtr(0.0)
	b.Pricing.InputPerMillion = criticPtr(1.0)
	b.Pricing.OutputPerMillion = criticPtr(1.0)
	h := criticHandler(t, p, a, b, outside)
	h.SetRouterScope("smart", nil, []string{"critic/a", "critic/b"})
	fixture := &modelDecisionFixture{t: t}
	jev, err := classifier.NewJevTransport(fixture, "jev-latest")
	if err != nil {
		t.Fatal(err)
	}
	h.SetClassifier(classifier.Runner{Classifier: jev, OnFailure: classifier.FailureFail})
	for i := 0; i < 2; i++ {
		w := criticPost(h, "/v1/chat/completions", criticGreeting)
		if w.Code != 200 || p.calls[i] != "b" {
			t.Fatalf("Jev choice replaced: %d %s calls=%v", w.Code, w.Body.String(), p.calls)
		}
	}
	if fixture.calls != 2 {
		t.Fatalf("Jev did not evaluate each new prompt: %d", fixture.calls)
	}
}
