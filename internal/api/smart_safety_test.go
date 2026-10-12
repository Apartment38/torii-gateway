package api_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/router"
	"time"
)

type safetyProvider struct {
	calls        []string
	models       []string
	statuses     map[string]int
	fail         bool
	closed       int
	headers      http.Header
	responseBody string
}

func (p *safetyProvider) ID() string { return "safety" }
func (p *safetyProvider) Models(context.Context) ([]provider.Model, error) {
	var models []provider.Model
	for _, name := range p.models {
		models = append(models, provider.Model{ID: "safety/" + name})
	}
	return models, nil
}
func (p *safetyProvider) Forward(_ context.Context, _ string, _ []byte, model string) (*http.Response, error) {
	p.calls = append(p.calls, model)
	if p.fail {
		return nil, fmt.Errorf("fixture failed")
	}
	status := p.statuses[model]
	if status == 0 {
		status = 200
	}
	headers := p.headers
	if headers == nil {
		headers = http.Header{"Content-Type": []string{"application/json"}}
	}
	responseBody := p.responseBody
	if responseBody == "" {
		responseBody = `{"fixture":"preserved"}`
	}
	return &http.Response{StatusCode: status, Header: headers, Body: &safetyBody{Reader: strings.NewReader(responseBody), closed: &p.closed}}, nil
}

type safetyBody struct {
	io.Reader
	closed *int
}

func (b *safetyBody) Close() error { *b.closed++; return nil }
func safetyPtr[T any](v T) *T      { return &v }
func safetyModel(name string, local bool) catalog.Descriptor {
	return catalog.Descriptor{PublicID: "safety/" + name, Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Capabilities: catalog.ModelCapabilities{Streaming: catalog.Supported, Tools: catalog.Supported, Reasoning: catalog.Supported}, Local: safetyPtr(local), Pricing: catalog.Pricing{InputPerMillion: safetyPtr(0.0), OutputPerMillion: safetyPtr(0.0)}, ContextWindow: safetyPtr(int64(100000)), Quality: catalog.Quality{General: safetyPtr(.95)}}
}
func safetyHandler(t *testing.T, p *safetyProvider, models ...catalog.Descriptor) *api.OpenAIHandler {
	t.Helper()
	h, err := api.NewOpenAIHandlerWithOverrides([]provider.OpenAIProvider{p}, models)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func safetyPost(h *api.OpenAIHandler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("X-Session-ID", "safety-session")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const safetyGreeting = `{"model":"smart","messages":[{"role":"user","content":"hi"}]}`
const safetyToolOutput = `{"model":"smart","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"call_a","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_a","content":"result"}]}`

type safetyClassifier struct{ calls int }

func (*safetyClassifier) ID() string { return "safety/classifier" }
func (c *safetyClassifier) Classify(context.Context, classifier.ClassificationRequest) (classifier.Classification, error) {
	c.calls++
	return classifier.Classification{TaskType: classifier.TaskReasoning, Complexity: classifier.ComplexityHigh, Confidence: 1, Requirements: classifier.FeatureRequirements{Reasoning: true}}, nil
}

func TestSmartSafetyRetryIsBoundedAndPreservesFailedResponse(t *testing.T) {
	for _, count := range []int{1, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			p := &safetyProvider{statuses: map[string]int{"a": 503, "b": 503, "c": 503}}
			var models []catalog.Descriptor
			for _, name := range []string{"a", "b", "c"}[:count] {
				p.models = append(p.models, name)
				models = append(models, safetyModel(name, true))
			}
			h := safetyHandler(t, p, models...)
			w := safetyPost(h, safetyGreeting)
			if w.Code != 503 || w.Body.String() != `{"fixture":"preserved"}` {
				t.Fatalf("response=%d %s", w.Code, w.Body.String())
			}
			want := min(count, 2)
			if len(p.calls) != want || p.closed != want {
				t.Fatalf("calls=%v closed=%d", p.calls, p.closed)
			}
		})
	}
}
func TestSmartSafetyFailedFinalStatusIsNotPinned(t *testing.T) {
	for _, status := range []int{400, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			p := &safetyProvider{models: []string{"a", "b"}, statuses: map[string]int{"a": status, "b": status}}
			h := safetyHandler(t, p, safetyModel("a", true), safetyModel("b", false))
			policy := router.DefaultSelectionPolicy()
			policy.LocationPreference = router.LocationLocal
			h.SetSelectionPolicy(policy)
			if w := safetyPost(h, safetyGreeting); w.Code != status {
				t.Fatal(w.Code, w.Body.String())
			}
			p.calls = nil
			p.statuses = nil
			if status == 400 {
				policy.LocationPreference = router.LocationCloud
				h.SetSelectionPolicy(policy)
			}
			w := safetyPost(h, safetyGreeting)
			want := "a"
			if status == 400 {
				want = "b"
			}
			if w.Code != 200 || len(p.calls) != 1 || p.calls[0] != want {
				t.Fatalf("failed response pinned model: calls=%v response=%d", p.calls, w.Code)
			}
		})
	}
}
func TestSmartSafetyToolContinuationKeepsClassificationAndEligibleAffinity(t *testing.T) {
	p := &safetyProvider{models: []string{"a", "b"}}
	h := safetyHandler(t, p, safetyModel("a", true), safetyModel("b", false))
	policy := router.DefaultSelectionPolicy()
	policy.LocationPreference = router.LocationLocal
	h.SetSelectionPolicy(policy)
	c := &safetyClassifier{}
	h.SetClassifier(classifier.Runner{Classifier: c})
	if w := safetyPost(h, safetyGreeting); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	policy.LocationPreference = router.LocationCloud
	h.SetSelectionPolicy(policy)
	w := safetyPost(h, safetyToolOutput)
	if w.Code != 200 || c.calls != 1 || strings.Join(p.calls, ",") != "a,a" {
		t.Fatalf("continuity lost: status=%d calls=%v classifier=%d", w.Code, p.calls, c.calls)
	}
}
func TestSmartSafetyToolContinuationNeverCrossesModelsAfterFailure(t *testing.T) {
	for _, kind := range []string{"quota", "upstream", "upstream_status"} {
		t.Run(kind, func(t *testing.T) {
			p := &safetyProvider{models: []string{"a", "b"}}
			h := safetyHandler(t, p, safetyModel("a", true), safetyModel("b", false))
			policy := router.DefaultSelectionPolicy()
			policy.LocationPreference = router.LocationLocal
			h.SetSelectionPolicy(policy)
			if w := safetyPost(h, safetyGreeting); w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if kind == "quota" {
				q := quota.NewCache(time.Hour, nil)
				_ = q.SetLocal("safety/a", 1, 1)
				h.SetQuotaCache(q)
			} else if kind == "upstream_status" {
				p.statuses = map[string]int{"a": http.StatusServiceUnavailable}
			} else {
				p.fail = true
			}
			w := safetyPost(h, safetyToolOutput)
			if w.Code < 400 {
				t.Fatal("tool continuation unexpectedly succeeded")
			}
			want := "a"
			if kind == "upstream" || kind == "upstream_status" {
				want = "a,a"
			}
			if strings.Join(p.calls, ",") != want {
				t.Fatalf("tool continuation crossed models: %v", p.calls)
			}
		})
	}
}

func TestSmartSafetyToolContinuationRequiresVerifiedAffinityBeforeClassifier(t *testing.T) {
	for _, kind := range []string{"missing", "expired", "changed_context"} {
		t.Run(kind, func(t *testing.T) {
			p := &safetyProvider{models: []string{"a", "b"}}
			h := safetyHandler(t, p, safetyModel("a", true), safetyModel("b", false))
			c := &safetyClassifier{}
			h.SetClassifier(classifier.Runner{Classifier: c})
			policy := router.DefaultSelectionPolicy()
			policy.LocationPreference = router.LocationLocal
			h.SetSelectionPolicy(policy)
			if kind == "expired" {
				h.SetSessionTimeout(time.Nanosecond)
			}
			if kind != "missing" {
				if w := safetyPost(h, safetyGreeting); w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if kind == "expired" {
				time.Sleep(time.Millisecond)
			}
			policy.LocationPreference = router.LocationCloud
			h.SetSelectionPolicy(policy)
			callsBefore, classificationsBefore := len(p.calls), c.calls
			body := safetyToolOutput
			if kind == "changed_context" {
				body = strings.Replace(body, `"content":"hi"`, `"content":"another conversation"`, 1)
			}
			w := safetyPost(h, body)
			if w.Code < 400 || len(p.calls) != callsBefore || c.calls != classificationsBefore {
				t.Fatalf("unverified tool affinity invoked classifier or physical model: status=%d calls=%v classifier=%d", w.Code, p.calls, c.calls)
			}
		})
	}
}

func TestSmartSafetyExplicitToolContinuationDoesNotRequireSmartAffinity(t *testing.T) {
	p := &safetyProvider{models: []string{"a"}}
	h := safetyHandler(t, p, safetyModel("a", true))
	c := &safetyClassifier{}
	h.SetClassifier(classifier.Runner{Classifier: c})
	body := strings.Replace(safetyToolOutput, `"model":"smart"`, `"model":"safety/a"`, 1)
	w := safetyPost(h, body)
	if w.Code != 200 || strings.Join(p.calls, ",") != "a" || c.calls != 0 {
		t.Fatalf("explicit model continuation altered: status=%d calls=%v classifier=%d", w.Code, p.calls, c.calls)
	}
}

func TestSmartSafetyResponsesToolOutputRequiresAffinity(t *testing.T) {
	p := &safetyProvider{models: []string{"a"}}
	m := safetyModel("a", true)
	m.Endpoints.Responses = catalog.Supported
	h := safetyHandler(t, p, m)
	c := &safetyClassifier{}
	h.SetClassifier(classifier.Runner{Classifier: c})
	body := `{"model":"smart","input":[{"type":"function_call_output","call_id":"opaque_original_call","output":"result"}]}`
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code < 400 || len(p.calls) != 0 || c.calls != 0 {
		t.Fatalf("unverified Responses tool output invoked classifier or model: status=%d calls=%v classifier=%d", w.Code, p.calls, c.calls)
	}
}

type evidenceClassifier struct{ version string }

func (*evidenceClassifier) ID() string { return "evidence/classifier" }
func (c *evidenceClassifier) Classify(context.Context, classifier.ClassificationRequest) (classifier.Classification, error) {
	return classifier.Classification{TaskType: classifier.TaskGeneral, Complexity: classifier.ComplexityMedium, Confidence: 1,
		Decision: &classifier.DecisionMetadata{Model: c.version, Usage: &classifier.DecisionUsage{InputTokens: 120, OutputTokens: 20}}}, nil
}
func TestSmartSafetyEvidenceIsTransparentAndCannotBeSpoofed(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			p := &safetyProvider{models: []string{"a"}, headers: http.Header{"X-Torii-Model": []string{"spoof"}, "X-Torii-Classifier-Output-Tokens": []string{"9999"}, "X-Torii-Injected": []string{"spoof"}}}
			if stream {
				p.responseBody = "data: {\"opaque\":true}\n\ndata: [DONE]\n\n"
			}
			m := safetyModel("a", false)
			m.Pricing = catalog.Pricing{}
			h := safetyHandler(t, p, m)
			h.SetClassifier(classifier.Runner{Classifier: &evidenceClassifier{version: "jev-fixture\r\nInjected: bad"}})
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			body := `{"model":"smart","messages":[{"role":"user","content":"private-prompt-sentinel"}]}`
			if stream {
				body = strings.TrimSuffix(body, "}") + `,"stream":true}`
			}
			w := safetyPost(h, body)
			want := p.responseBody
			if want == "" {
				want = `{"fixture":"preserved"}`
			}
			if w.Code != 200 || w.Body.String() != want {
				t.Fatalf("body changed: %d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("X-Torii-Model") != "safety/a" || w.Header().Get("X-Torii-Injected") != "" || w.Header().Get("X-Torii-Classifier-Model") != "" {
				t.Fatal("spoofed/injected evidence", w.Header())
			}
			if w.Header().Get("X-Torii-Estimated-Inference-Cost-Usd") != "unknown" || w.Header().Get("X-Torii-Classifier-Cost-Usd") != "unknown" || w.Header().Get("X-Torii-Classifier-Input-Tokens") != "120" || w.Header().Get("X-Torii-Classifier-Output-Tokens") != "20" {
				t.Fatal("cost/usage evidence", w.Header())
			}
			if elapsed, err := strconv.ParseFloat(w.Header().Get("X-Torii-Routing-Ms"), 64); err != nil || elapsed < 0 {
				t.Fatal("missing routing time")
			}
			if strings.Contains(logs.String(), "smart_routing_decision") || strings.Contains(logs.String(), "classifier_actual_usage") || strings.Contains(logs.String(), "private-prompt-sentinel") || strings.Contains(logs.String(), "safety-session") {
				t.Fatal("unsafe/missing logs", logs.String())
			}
		})
	}
}
