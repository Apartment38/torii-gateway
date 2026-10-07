package classifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
)

// The extra action and routing fields, optional noul confidence, and rounded
// choice probabilities follow Laya's actual Agent._decode_answers wire format.
const layaDecision = `{"model":"laya-rl-agent","answers":{
 "task_type":{"type":"choice","choice":"coding","probabilities":{"general":0.02,"conversation":0.01,"coding":0.8,"sysadmin":0.05,"reasoning":0.03,"math":0.02,"research":0.02,"summarization":0.02,"extraction":0.01,"creative":0.01,"translation":0.01},"confidence":0.7,"action":{"act_probability":0.8}},
 "complexity":{"type":"choice","choice":"medium","probabilities":{"low":0.3333,"medium":0.3334,"high":0.3333},"confidence":0.2},
 "tools":{"type":"noul","noul":0.5,"confidence":0.5,"action":{"act_probability":0.7}},
 "vision":{"type":"noul","noul":0},
 "reasoning":{"type":"noul","noul":0.4999},
 "structured_output":{"type":"noul","noul":1},
 "latency_sensitive":{"type":"noul","noul":0.9}
},"usage":{"input_tokens":129,"output_tokens":0},"routing":{"model":"english","reason":"explicit model"}}`

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestJevCompatibleLayaDecision(t *testing.T) {
	got, err := decodeJevResponse(response(layaDecision), ClassificationRequest{Requirements: router.RequestRequirements{Vision: true, Reasoning: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskType != TaskCoding || got.Complexity != ComplexityMedium || got.Confidence != .2 || !got.Requirements.Tools || !got.Requirements.Vision || !got.Requirements.Reasoning || !got.Requirements.StructuredOutput || !got.Preferences.LatencySensitive {
		t.Fatalf("classification = %+v", got)
	}
	if got.Decision == nil || got.Decision.Model != "laya-rl-agent" || got.Decision.Usage == nil || got.Decision.Usage.InputTokens != 129 || got.Decision.Usage.OutputTokens != 0 || got.Decision.Choices["task_type"].Probabilities["coding"] != .8 {
		t.Fatalf("decision metadata = %+v", got.Decision)
	}
	// Real four-decimal rounding can make a normalized distribution sum .9999.
	rounded := strings.Replace(layaDecision, `"medium":0.3334`, `"medium":0.3333`, 1)
	if _, err := decodeJevResponse(response(rounded), ClassificationRequest{}); err != nil {
		t.Fatalf("rounded/tied Laya distribution rejected: %v", err)
	}
	zero := strings.Replace(layaDecision, `"confidence":0.7`, `"confidence":0`, 1)
	got, err = decodeJevResponse(response(zero), ClassificationRequest{})
	if err != nil || got.Confidence != 0 {
		t.Fatalf("zero confidence = %+v, %v", got, err)
	}
}

func TestJevUsageRequiresMeasuredIntegerCounts(t *testing.T) {
	for _, bad := range []string{`null`, `{}`, `[]`, `"unknown"`, `{"input_tokens":129}`, `{"input_tokens":129,"output_tokens":null}`, `{"input_tokens":null,"output_tokens":0}`, `{"input_tokens":-1,"output_tokens":0}`, `{"input_tokens":129,"output_tokens":0.5}`, `{"input_tokens":129,"output_tokens":"0"}`, `{"input_tokens":129,"output_tokens":9223372036854775808}`} {
		body := strings.Replace(layaDecision, `{"input_tokens":129,"output_tokens":0}`, bad, 1)
		got, err := decodeJevResponse(response(body), ClassificationRequest{})
		if err != nil || got.TaskType != TaskCoding || got.Decision == nil || got.Decision.Usage != nil {
			t.Fatalf("optional usage %s must stay unknown without rejecting decision: %+v, %v", bad, got, err)
		}
	}
	unknown := strings.Replace(layaDecision, `,"usage":{"input_tokens":129,"output_tokens":0}`, "", 1)
	got, err := decodeJevResponse(response(unknown), ClassificationRequest{})
	if err != nil || got.Decision == nil || got.Decision.Usage != nil {
		t.Fatalf("absent usage must stay unknown: %+v, %v", got.Decision, err)
	}
}

type decisionFixture string

func (d decisionFixture) ForwardDecision(context.Context, []byte) (*http.Response, error) {
	return response(string(d)), nil
}

func TestMalformedJevDecisionsReachFailurePolicy(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"missing answer":        func(a map[string]any) { delete(a, "tools") },
		"null answer":           func(a map[string]any) { a["tools"] = nil },
		"wrong choice type":     func(a map[string]any) { a["task_type"].(map[string]any)["type"] = "score" },
		"wrong noul type":       func(a map[string]any) { a["vision"].(map[string]any)["type"] = "choice" },
		"missing noul":          func(a map[string]any) { delete(a["vision"].(map[string]any), "noul") },
		"null noul":             func(a map[string]any) { a["vision"].(map[string]any)["noul"] = nil },
		"negative noul":         func(a map[string]any) { a["tools"].(map[string]any)["noul"] = -0.1 },
		"large noul":            func(a map[string]any) { a["latency_sensitive"].(map[string]any)["noul"] = 1.1 },
		"string noul":           func(a map[string]any) { a["reasoning"].(map[string]any)["noul"] = "0.9" },
		"unknown choice":        func(a map[string]any) { a["task_type"].(map[string]any)["choice"] = "private-secret" },
		"missing confidence":    func(a map[string]any) { delete(a["task_type"].(map[string]any), "confidence") },
		"null confidence":       func(a map[string]any) { a["complexity"].(map[string]any)["confidence"] = nil },
		"large confidence":      func(a map[string]any) { a["task_type"].(map[string]any)["confidence"] = 1.1 },
		"negative confidence":   func(a map[string]any) { a["complexity"].(map[string]any)["confidence"] = -0.1 },
		"missing probabilities": func(a map[string]any) { delete(a["task_type"].(map[string]any), "probabilities") },
		"incomplete probabilities": func(a map[string]any) {
			delete(a["task_type"].(map[string]any)["probabilities"].(map[string]any), "translation")
		},
		"null probability": func(a map[string]any) {
			a["complexity"].(map[string]any)["probabilities"].(map[string]any)["low"] = nil
		},
		"large probability": func(a map[string]any) {
			a["complexity"].(map[string]any)["probabilities"].(map[string]any)["low"] = 1.5
		},
		"negative probability": func(a map[string]any) {
			a["complexity"].(map[string]any)["probabilities"].(map[string]any)["low"] = -.1
		},
		"wrong sum":    func(a map[string]any) { a["complexity"].(map[string]any)["probabilities"].(map[string]any)["low"] = .2 },
		"wrong winner": func(a map[string]any) { a["task_type"].(map[string]any)["choice"] = "general" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var envelope map[string]any
			if err := json.Unmarshal([]byte(layaDecision), &envelope); err != nil {
				t.Fatal(err)
			}
			mutate(envelope["answers"].(map[string]any))
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			assertDecisionPolicies(t, string(body))
		})
	}
	for name, body := range map[string]string{"empty": `{}`, "null answers": `{"answers":null}`, "invalid JSON": `{"answers":`, "trailing JSON": layaDecision + `{}`, "oversized": strings.Repeat("x", maxJevResponseSize+1), "unsafe decoder error": `{"answers":{"private-secret":[]}}`} {
		t.Run(name, func(t *testing.T) { assertDecisionPolicies(t, body) })
	}
}

func assertDecisionPolicies(t *testing.T, body string) {
	t.Helper()
	c, err := NewJevTransport(decisionFixture(body), "english")
	if err != nil {
		t.Fatal(err)
	}
	req := ClassificationRequest{Model: "physical/model", Requirements: router.RequestRequirements{Tools: true, Vision: true, Reasoning: true, StructuredOutput: true}}
	runner := Runner{Classifier: c, OnFailure: FailureFail}
	if _, err := runner.Classify(context.Background(), req); err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("fail policy = %v", err)
	}
	runner.OnFailure = FailureCapabilityFallback
	got, err := runner.Classify(context.Background(), req)
	if err != nil || got.TaskType != TaskGeneral || got.Complexity != ComplexityMedium || !got.Requirements.Tools || !got.Requirements.Vision || !got.Requirements.Reasoning || !got.Requirements.StructuredOutput {
		t.Fatalf("fallback = %+v, %v", got, err)
	}
}

func TestJevProviderRoundTripUsesLayaTypedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/v1/systemone" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer local-key" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		var body struct {
			Model     string
			State     string
			Questions map[string]jevQuestion
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "english" || body.State != "Debug this function" || len(body.Questions) != 7 || len(body.Questions["task_type"].Criteria) != 11 || body.Questions["complexity"].Type != "choice" || body.Questions["tools"].Type != "noul" {
			t.Errorf("body = %+v", body)
		}
		_, _ = io.WriteString(w, layaDecision)
	}))
	defer server.Close()
	p, err := provider.NewJevProvider(provider.OpenAICompatibleConfig{ID: "laya", BaseURL: server.URL + "/prefix/v1", StaticModels: []string{"english"}, Headers: http.Header{"Authorization": {"Bearer local-key"}}, ModelRewrites: map[string]string{"friendly": "english"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewJevTransport(p, "friendly")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Classify(context.Background(), ClassificationRequest{Text: "Debug this function"})
	if err != nil || got.TaskType != TaskCoding {
		t.Fatalf("classification = %+v, %v", got, err)
	}
}

func TestDirectJevValidatesURLAndUsesCanonicalEndpoint(t *testing.T) {
	for _, base := range []string{"", "://bad", "relative", "ftp://host", "http://user:password@host", "http://host?secret=value", "http://host#fragment"} {
		if _, err := NewJev(base, "key", "", nil); err == nil {
			t.Fatalf("accepted %q", base)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/v1/systemone" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("request = %s", r.URL)
		}
		_, _ = io.WriteString(w, layaDecision)
	}))
	defer server.Close()
	for _, suffix := range []string{"/prefix", "/prefix/v1/"} {
		c, err := NewJev(server.URL+suffix, "key", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Classify(context.Background(), ClassificationRequest{Text: "hello"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDirectJevNeverFollowsRedirects(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/capture-secret", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := &http.Client{}
	c, err := NewJev(server.URL, "key", "", client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Classify(context.Background(), ClassificationRequest{Text: "private request"}); err == nil {
		t.Fatal("redirect accepted")
	}
	if calls != 1 || client.CheckRedirect != nil {
		t.Fatalf("calls = %d; supplied client mutated = %t", calls, client.CheckRedirect != nil)
	}
}
