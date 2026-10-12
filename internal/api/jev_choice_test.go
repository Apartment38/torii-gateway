package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Apartment38/torii-gateway/internal/activity"
	"github.com/Apartment38/torii-gateway/internal/state"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
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
	if request.State != "hi" || len(criteria) != 2 || criteria["critic/a"] == "" || criteria["critic/b"] == "" || !strings.Contains(criteria["critic/b"], `"coding":`) {
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
	a.Capabilities.Tools = catalog.Supported
	b.Capabilities.Tools = catalog.Supported
	h := criticHandler(t, p, a, b, outside)
	_, benchmarks := consoleBenchmarkManager(t, nil)
	h.SetIntelligence(benchmarks)
	score := .95
	if err := benchmarks.Import(context.Background(), []intelligence.Observation{{ExternalModelID: "critic/b", Benchmark: "my-code-eval", Category: intelligence.Coding, Score: 95, Normalized: &score}}); err != nil {
		t.Fatal(err)
	}
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
	w := criticPost(h, "/v1/chat/completions", `{"model":"smart","messages":[{"role":"user","content":"hi"},{"role":"tool","tool_call_id":"opaque","content":"tool result"}]}`)
	if w.Code != 200 || p.calls[2] != "b" {
		t.Fatalf("tool continuation lost originating model: %d %s", w.Code, w.Body.String())
	}
	if fixture.calls != 2 {
		t.Fatalf("Jev did not evaluate each new prompt: %d", fixture.calls)
	}
}

func TestActivityJevCaptureIndependentOfPromptLogging(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			p := &criticProvider{models: []string{"only"}}
			h := criticHandler(t, p, criticModel("only"))
			db, err := state.OpenDatabase(filepath.Join(t.TempDir(), "activity.db"), bytes.Repeat([]byte{2}, 32))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			events, err := activity.Open(db, activity.Options{JevDecisionLogging: enabled})
			if err != nil {
				t.Fatal(err)
			}
			defer events.Close()
			h.SetActivity(events)
			ch, cancel := events.Subscribe()
			defer cancel()
			// A malformed provider response must still be inspectable for debugging.
			rawResponse := `{"provider_error":"invalid decision fixture"}`
			jev, err := classifier.NewJevTransport(rawDecisionFixture(rawResponse), "jev-latest")
			if err != nil {
				t.Fatal(err)
			}
			h.SetClassifier(classifier.Runner{Classifier: jev, OnFailure: classifier.FailureFail})
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			criticPost(h, "/v1/chat/completions", criticGreeting)
			for range 2 {
				select {
				case e := <-ch:
					if e.Prompt != "" {
						t.Fatal("Jev setting enabled API prompt logging")
					}
					if e.Kind == "request_complete" {
						if enabled {
							var request map[string]any
							if json.Unmarshal([]byte(e.JevRequest), &request) != nil || request["state"] != "hi" || e.JevResponse != rawResponse {
								t.Fatal("missing exact Jev request/response", e)
							}
						} else if e.JevRequest != "" || e.JevResponse != "" {
							t.Fatal("Jev capture defaulted on")
						}
					}
				case <-time.After(time.Second):
					t.Fatal("event missing")
				}
			}
			events.Close()
			if strings.Contains(logs.String(), "smart_routing_decision") != enabled || strings.Contains(logs.String(), "invalid decision fixture") {
				t.Fatal("decision logging opt-in or redaction failed", logs.String())
			}
		})
	}
}

type rawDecisionFixture string

func (f rawDecisionFixture) ForwardDecision(context.Context, []byte) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(f)))}, nil
}
