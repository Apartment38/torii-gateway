package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/router"
)

// These public-handler fixtures freeze the pool and requests independently of
// the selection implementation. No listener, credentials, or network is used.
type criticProvider struct {
	models []string
	calls  []string
	fail   map[string]bool
}

func (p *criticProvider) ID() string { return "critic" }
func (p *criticProvider) Models(context.Context) ([]provider.Model, error) {
	var models []provider.Model
	for _, name := range p.models {
		models = append(models, provider.Model{ID: "critic/" + name})
	}
	return models, nil
}
func (p *criticProvider) Forward(_ context.Context, _ string, _ []byte, model string) (*http.Response, error) {
	p.calls = append(p.calls, model)
	if p.fail[model] {
		return nil, fmt.Errorf("frozen upstream failure")
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
}

type criticClassifier struct {
	texts  []string
	result classifier.Classification
}

func (c *criticClassifier) ID() string { return "critic/classifier" }
func (c *criticClassifier) Classify(_ context.Context, r classifier.ClassificationRequest) (classifier.Classification, error) {
	c.texts = append(c.texts, r.Text)
	return c.result, nil
}
func criticPtr[T any](v T) *T { return &v }
func criticModel(name string) catalog.Descriptor {
	return catalog.Descriptor{PublicID: "critic/" + name, Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Capabilities: catalog.ModelCapabilities{Tools: catalog.Unsupported, Vision: catalog.Unsupported, Reasoning: catalog.Unsupported, StructuredOutput: catalog.Unsupported, Streaming: catalog.Supported}, Local: criticPtr(true), Quality: catalog.Quality{General: criticPtr(.95)}, ContextWindow: criticPtr(int64(100000))}
}
func criticHandler(t *testing.T, p *criticProvider, models ...catalog.Descriptor) *api.OpenAIHandler {
	t.Helper()
	h, err := api.NewOpenAIHandlerWithOverrides([]provider.OpenAIProvider{p}, models)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func criticPost(h *api.OpenAIHandler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("X-Session-ID", "critic-session")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const criticGreeting = `{"model":"smart","messages":[{"role":"user","content":"hi"}]}`

func TestGauntletCriticInitialRejections(t *testing.T) {
	for _, kind := range []string{"unsupported_endpoint", "paid_forbidden", "quota_exhausted", "context_overflow", "quality_floor"} {
		t.Run(kind, func(t *testing.T) {
			p := &criticProvider{models: []string{"only"}}
			m := criticModel("only")
			body := criticGreeting
			switch kind {
			case "unsupported_endpoint":
				m.Endpoints.ChatCompletions = catalog.Unsupported
			case "paid_forbidden":
				m.Local = criticPtr(false)
				m.Pricing = catalog.Pricing{InputPerMillion: criticPtr(1.0), OutputPerMillion: criticPtr(1.0)}
			case "context_overflow":
				m.ContextWindow = criticPtr(int64(64))
				body = `{"model":"smart","max_tokens":1000,"messages":[{"role":"user","content":"hi"}]}`
			case "quality_floor":
				m.Quality.General = criticPtr(.1)
			}
			h := criticHandler(t, p, m)
			policy := router.DefaultSelectionPolicy()
			if kind == "paid_forbidden" {
				policy.AllowPaidModels = false
			}
			h.SetSelectionPolicy(policy)
			if kind == "quota_exhausted" {
				q := quota.NewCache(time.Hour, nil)
				if err := q.SetLocal("critic/only", 1, 1); err != nil {
					t.Fatal(err)
				}
				h.SetQuotaCache(q)
			}
			w := criticPost(h, "/v1/chat/completions", body)
			if len(p.calls) > 0 {
				t.Fatalf("ineligible physical model was invoked: calls=%v status=%d body=%s", p.calls, w.Code, w.Body.String())
			}
			if w.Code < 400 {
				t.Fatalf("expected routing rejection: status=%d", w.Code)
			}
		})
	}
}

func TestGauntletCriticAffinityRevalidation(t *testing.T) {
	for _, kind := range []string{"tools", "quota", "free_policy", "high_semantic_complexity"} {
		t.Run(kind, func(t *testing.T) {
			p := &criticProvider{models: []string{"a", "b"}}
			a, b := criticModel("a"), criticModel("b")
			b.Capabilities.Tools = catalog.Supported
			if kind == "free_policy" {
				a.Local = criticPtr(false)
				a.Pricing = catalog.Pricing{InputPerMillion: criticPtr(0.0), OutputPerMillion: criticPtr(0.0)}
				b.Local = criticPtr(false)
				b.Pricing = catalog.Pricing{InputPerMillion: criticPtr(1.0), OutputPerMillion: criticPtr(1.0)}
			}
			if kind == "high_semantic_complexity" {
				a.Quality.General = criticPtr(.75)
				b.Quality.General = criticPtr(.95)
			}
			h := criticHandler(t, p, a, b)
			c := &criticClassifier{result: classifier.Classification{TaskType: classifier.TaskGeneral, Complexity: classifier.ComplexityMedium, Confidence: 1}}
			h.SetClassifier(classifier.Runner{Classifier: c})
			if w := criticPost(h, "/v1/chat/completions", criticGreeting); w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if len(p.calls) != 1 || p.calls[0] != "a" {
				t.Fatal("fixture did not start on a", p.calls)
			}
			body := `{"model":"smart","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"Use a tool to solve this"}]}`
			switch kind {
			case "tools":
				body = `{"model":"smart","tools":[{"type":"function","function":{"name":"lookup"}}],"messages":[{"role":"user","content":"hi"},{"role":"user","content":"Use lookup"}]}`
			case "quota":
				q := quota.NewCache(time.Hour, nil)
				_ = q.SetLocal("critic/a", 1, 1)
				h.SetQuotaCache(q)
			case "free_policy":
				policy := router.DefaultSelectionPolicy()
				policy.AllowPaidModels = false
				policy.MinimumQuality = map[string]float64{"medium": .99}
				h.SetSelectionPolicy(policy)
			case "high_semantic_complexity":
				c.result.Complexity = classifier.ComplexityHigh
			}
			w := criticPost(h, "/v1/chat/completions", body)
			if kind == "free_policy" {
				if len(p.calls) != 1 || w.Code < 400 {
					t.Fatalf("affinity bypassed new policy: calls=%v status=%d", p.calls, w.Code)
				}
				return
			}
			if len(p.calls) != 2 || p.calls[1] != "b" {
				t.Fatalf("affinity bypassed current %s: calls=%v status=%d classifier_inputs=%v", kind, p.calls, w.Code, c.texts)
			}
		})
	}
}

func TestGauntletCriticRerouteRetainsSemantics(t *testing.T) {
	p := &criticProvider{models: []string{"a", "b", "c"}, fail: map[string]bool{"a": true}}
	a, b, c := criticModel("a"), criticModel("b"), criticModel("c")
	a.Capabilities.Reasoning, c.Capabilities.Reasoning = catalog.Supported, catalog.Supported
	h := criticHandler(t, p, a, b, c)
	sem := &criticClassifier{result: classifier.Classification{TaskType: classifier.TaskReasoning, Complexity: classifier.ComplexityHigh, Confidence: 1, Requirements: classifier.FeatureRequirements{Reasoning: true}}}
	h.SetClassifier(classifier.Runner{Classifier: sem})
	w := criticPost(h, "/v1/chat/completions", `{"model":"smart","messages":[{"role":"user","content":"Prove the theorem with deep logical reasoning"}]}`)
	if w.Code != 200 || len(p.calls) != 2 || p.calls[0] != "a" || p.calls[1] != "c" {
		t.Fatalf("reroute discarded semantic reasoning requirement: calls=%v status=%d", p.calls, w.Code)
	}
}

func TestGauntletCriticClassifierCurrentInput(t *testing.T) {
	for _, f := range []struct{ name, path, body, want string }{
		{"chat_last_user", "/v1/chat/completions", `{"model":"smart","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"Hello"},{"role":"user","content":"Design a concurrent compiler"}]}`, "Design a concurrent compiler"},
		{"completion_prompt", "/v1/completions", `{"model":"smart","prompt":"Prove the theorem"}`, "Prove the theorem"},
		{"responses_array", "/v1/responses", `{"model":"smart","input":[{"role":"user","content":[{"type":"input_text","text":"Prove the theorem"}]}]}`, "Prove the theorem"},
		{"chat_content_array", "/v1/chat/completions", `{"model":"smart","messages":[{"role":"user","content":[{"type":"text","text":"Prove the theorem"}]}]}`, "Prove the theorem"},
	} {
		t.Run(f.name, func(t *testing.T) {
			p := &criticProvider{models: []string{"a"}}
			m := criticModel("a")
			m.Endpoints.Completions, m.Endpoints.Responses = catalog.Supported, catalog.Supported
			h := criticHandler(t, p, m)
			c := &criticClassifier{result: classifier.Classification{TaskType: classifier.TaskGeneral, Complexity: classifier.ComplexityMedium, Confidence: 1}}
			h.SetClassifier(classifier.Runner{Classifier: c})
			w := criticPost(h, f.path, f.body)
			if len(c.texts) != 1 || !strings.Contains(c.texts[0], f.want) {
				t.Fatalf("current request missing from classifier input: inputs=%q want=%q status=%d", c.texts, f.want, w.Code)
			}
		})
	}
}

type criticDecisionTransport struct{ body string }

func (p criticDecisionTransport) ForwardDecision(context.Context, []byte) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(p.body))}, nil
}

// This separate response is derived from the documented TypeSafe Choice/Noul
// shapes, rather than from the implementation's internal decoder structs.
func criticTypedDecision(t *testing.T, malformed bool) string {
	t.Helper()
	tasks := []string{"general", "conversation", "coding", "sysadmin", "reasoning", "math", "research", "summarization", "extraction", "creative", "translation"}
	distribution := map[string]float64{}
	for _, task := range tasks {
		distribution[task] = 0
	}
	distribution["coding"] = 1
	answers := map[string]any{
		"task_type":  map[string]any{"type": "choice", "choice": "coding", "confidence": 1, "probabilities": distribution},
		"complexity": map[string]any{"type": "choice", "choice": "medium", "confidence": 1, "probabilities": map[string]float64{"low": 0, "medium": 1, "high": 0}},
	}
	for _, name := range []string{"tools", "vision", "reasoning", "structured_output", "latency_sensitive"} {
		n := 0
		if name == "tools" {
			n = 1
		}
		answers[name] = map[string]any{"type": "noul", "noul": n}
	}
	if malformed {
		answers["tools"] = map[string]any{"type": "noul", "noul": nil}
	}
	body, err := json.Marshal(map[string]any{"model": "jev-1.13.0", "answers": answers, "usage": map[string]int{"input_tokens": 123, "output_tokens": 20}})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestGauntletCriticTypedDecisionAtHandler(t *testing.T) {
	for _, mode := range []string{"valid_semantic_tools", "malformed_fail", "malformed_fallback_objective_tools"} {
		t.Run(mode, func(t *testing.T) {
			p := &criticProvider{models: []string{"a", "b"}}
			a, b := criticModel("a"), criticModel("b")
			b.Capabilities.Tools = catalog.Supported
			h := criticHandler(t, p, a, b)
			decision, err := classifier.NewJevTransport(criticDecisionTransport{body: criticTypedDecision(t, mode != "valid_semantic_tools")}, "jev-latest")
			if err != nil {
				t.Fatal(err)
			}
			policy := classifier.FailureFail
			body := criticGreeting
			if mode == "malformed_fallback_objective_tools" {
				policy = classifier.FailureCapabilityFallback
				body = `{"model":"smart","tools":[{"type":"function","function":{"name":"lookup"}}],"messages":[{"role":"user","content":"hi"}]}`
			}
			h.SetClassifier(classifier.Runner{Classifier: decision, OnFailure: policy})
			w := criticPost(h, "/v1/chat/completions", body)
			if mode == "malformed_fail" {
				if len(p.calls) != 0 || w.Code < 400 {
					t.Fatalf("malformed typed answer reached physical model: calls=%v status=%d", p.calls, w.Code)
				}
				return
			}
			if len(p.calls) != 1 || p.calls[0] != "b" || w.Code != 200 {
				t.Fatalf("typed decision lost tools constraint: calls=%v status=%d body=%s", p.calls, w.Code, w.Body.String())
			}
		})
	}
}

func TestGauntletCriticPlainTextFormat(t *testing.T) {
	for _, f := range []struct{ name, path, body string }{
		{"chat_plain_text", "/v1/chat/completions", `{"model":"smart","response_format":{"type":"text"},"messages":[{"role":"user","content":"hi"}]}`},
		{"responses_plain_text", "/v1/responses", `{"model":"smart","text":{"format":{"type":"text"}},"input":"hi"}`},
		{"responses_verbosity_only", "/v1/responses", `{"model":"smart","text":{"verbosity":"medium"},"input":"hi"}`},
	} {
		t.Run(f.name, func(t *testing.T) {
			p := &criticProvider{models: []string{"a"}}
			m := criticModel("a")
			m.Endpoints.Responses = catalog.Supported
			h := criticHandler(t, p, m)
			w := criticPost(h, f.path, f.body)
			if w.Code != 200 || len(p.calls) != 1 {
				t.Fatalf("plain text request incorrectly requires structured-output support: calls=%v status=%d body=%s", p.calls, w.Code, w.Body.String())
			}
		})
	}
}

type criticRoundTrip func(*http.Request) (*http.Response, error)

func (f criticRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGauntletCriticOllamaMinimalSetup(t *testing.T) {
	var inferenceCalls int
	client := &http.Client{Transport: criticRoundTrip(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/v1/models":
			body = `{"data":[{"id":"qwen3","object":"model"}]}`
		case "/api/show":
			body = `{"capabilities":["completion","tools","thinking"],"model_info":{"qwen3.context_length":32768},"details":{"family":"qwen3"}}`
		case "/v1/chat/completions":
			inferenceCalls++
			body = `{"choices":[{"message":{"content":"hello"}}]}`
		default:
			return nil, fmt.Errorf("unexpected discovery path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	physical, err := provider.NewOllama("local", "http://ollama.invalid", client)
	if err != nil {
		t.Fatal(err)
	}
	h, err := api.NewOpenAIHandler([]provider.OpenAIProvider{physical})
	if err != nil {
		t.Fatal(err)
	}
	direct := criticPost(h, "/v1/chat/completions", strings.Replace(criticGreeting, `"smart"`, `"local/qwen3"`, 1))
	if direct.Code != 200 || inferenceCalls != 1 {
		t.Fatalf("minimal setup fixture cannot directly infer: status=%d calls=%d", direct.Code, inferenceCalls)
	}
	w := criticPost(h, "/v1/chat/completions", criticGreeting)
	if w.Code != 200 || inferenceCalls != 2 {
		t.Fatalf("adding smart requires external capability catalog: direct=%d smart=%d calls=%d body=%s", direct.Code, w.Code, inferenceCalls, w.Body.String())
	}
}

func TestGauntletCriticRejectedDecisionUsage(t *testing.T) {
	p := &criticProvider{models: []string{"a"}}
	h := criticHandler(t, p, criticModel("a"))
	// A complete, valid typed answer can still be rejected by policy. Its
	// already-consumed classifier tokens remain measured routing overhead.
	body := strings.ReplaceAll(criticTypedDecision(t, false), `"confidence":1`, `"confidence":0.1`)
	decision, err := classifier.NewJevTransport(criticDecisionTransport{body: body}, "jev-latest")
	if err != nil {
		t.Fatal(err)
	}
	h.SetClassifier(classifier.Runner{Classifier: decision, OnFailure: classifier.FailureFail, MinimumConfidence: .5})
	w := criticPost(h, "/v1/chat/completions", criticGreeting)
	if w.Code != 502 || len(p.calls) != 0 {
		t.Fatalf("low-confidence rejection failed: status=%d calls=%v", w.Code, p.calls)
	}
	if w.Header().Get("X-Torii-Classifier-Input-Tokens") != "123" || w.Header().Get("X-Torii-Classifier-Output-Tokens") != "20" || w.Header().Get("X-Torii-Classifier-Model") != "jev-1.13.0" {
		t.Fatalf("valid rejected decision lost measured overhead: headers=%v", w.Header())
	}
}

func TestGauntletCriticIncompleteUsageIsUnknown(t *testing.T) {
	for _, field := range []string{"missing", "null", "explicit_zero"} {
		t.Run(field, func(t *testing.T) {
			p := &criticProvider{models: []string{"a"}}
			m := criticModel("a")
			m.Capabilities.Tools = catalog.Supported
			h := criticHandler(t, p, m)
			var envelope map[string]any
			if err := json.Unmarshal([]byte(criticTypedDecision(t, false)), &envelope); err != nil {
				t.Fatal(err)
			}
			usage := envelope["usage"].(map[string]any)
			switch field {
			case "missing":
				delete(usage, "output_tokens")
			case "null":
				usage["output_tokens"] = nil
			case "explicit_zero":
				usage["output_tokens"] = 0
			}
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			decision, err := classifier.NewJevTransport(criticDecisionTransport{body: string(body)}, "jev-latest")
			if err != nil {
				t.Fatal(err)
			}
			h.SetClassifier(classifier.Runner{Classifier: decision, OnFailure: classifier.FailureFail})
			w := criticPost(h, "/v1/chat/completions", criticGreeting)
			input, output := w.Header().Get("X-Torii-Classifier-Input-Tokens"), w.Header().Get("X-Torii-Classifier-Output-Tokens")
			if field == "explicit_zero" {
				if w.Code != 200 || input != "123" || output != "0" {
					t.Fatalf("explicit measured zero lost: status=%d headers=%v", w.Code, w.Header())
				}
				return
			}
			if input != "" || output != "" {
				t.Fatalf("missing/null usage falsely labeled measured: status=%d input=%q output=%q", w.Code, input, output)
			}
		})
	}
}
