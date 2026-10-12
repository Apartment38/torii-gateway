package api_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
)

func TestSmartContextResolvesFollowupWithoutForwardingPayloads(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		t.Run(path, func(t *testing.T) {
			p := &criticProvider{models: []string{"a"}}
			m := criticModel("a")
			m.Endpoints.Responses = catalog.Supported
			m.Capabilities.Tools, m.Capabilities.Vision = catalog.Supported, catalog.Supported
			h := criticHandler(t, p, m)
			c := &criticClassifier{result: classifier.Classification{TaskType: classifier.TaskMath, Complexity: classifier.ComplexityHigh, Confidence: 1}}
			h.SetClassifier(classifier.Runner{Classifier: c})
			messages := []any{
				map[string]any{"role": "developer", "content": "Explain every lemma rigorously."},
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Prove the spectral theorem."}, map[string]any{"type": "input_image", "image_url": "data:image/png;base64,IMAGE_PAYLOAD_SENTINEL"}}},
				map[string]any{"type": "reasoning", "summary": "PRIVATE_REASONING_SENTINEL"},
				map[string]any{"role": "tool", "content": "TOOL_PAYLOAD_SENTINEL"},
				map[string]any{"type": "function_call_output", "call_id": "OPAQUE_PROVIDER_SENTINEL", "output": "FUNCTION_PAYLOAD_SENTINEL"},
				map[string]any{"role": "assistant", "content": "Shall I develop the complete proof?", "reasoning_content": "ASSISTANT_REASONING_SENTINEL", "tool_calls": []any{map[string]any{"function": map[string]any{"arguments": "CALL_ARGUMENT_SENTINEL"}}}},
				map[string]any{"role": "user", "content": "Yes, do that."},
			}
			body := map[string]any{"model": "smart"}
			if path == "/v1/responses" {
				body["input"], body["instructions"] = messages, "Use a mathematical proof style."
			} else {
				body["messages"] = messages
			}
			encoded, _ := json.Marshal(body)
			w := criticPost(h, path, string(encoded))
			if w.Code != 200 || len(c.texts) != 1 {
				t.Fatalf("status=%d classifier calls=%d", w.Code, len(c.texts))
			}
			state := c.texts[0]
			for _, text := range []string{"Yes, do that.", "spectral theorem", "complete proof", "every lemma"} {
				if !strings.Contains(state, text) {
					t.Fatalf("required task context missing: %q", text)
				}
			}
			if path == "/v1/responses" && !strings.Contains(state, "mathematical proof style") {
				t.Fatal("Responses instructions missing")
			}
			if strings.Contains(state, "SENTINEL") || strings.Contains(state, "data:image") {
				t.Fatal("non-conversation payload reached classifier")
			}
		})
	}
}

func TestSmartContextBudgetsEscapedUnicodeAndPreservesTaskEnds(t *testing.T) {
	p := &criticProvider{models: []string{"a"}}
	m := criticModel("a")
	m.ContextWindow = criticPtr(int64(1000000))
	h := criticHandler(t, p, m)
	c := &criticClassifier{result: classifier.Classification{TaskType: classifier.TaskGeneral, Complexity: classifier.ComplexityMedium, Confidence: 1}}
	h.SetClassifier(classifier.Runner{Classifier: c})
	long := "Analyze this task. " + strings.Repeat("世界<>&\n", 10000) + " Preserve the final constraint."
	body, _ := json.Marshal(map[string]any{"model": "smart", "messages": []any{
		map[string]any{"role": "system", "content": long},
		map[string]any{"role": "user", "content": long},
		map[string]any{"role": "assistant", "content": long},
		map[string]any{"role": "user", "content": long},
	}})
	w := criticPost(h, "/v1/chat/completions", string(body))
	if w.Code != 200 || len(c.texts) != 1 {
		t.Fatalf("status=%d classifier calls=%d", w.Code, len(c.texts))
	}
	state := c.texts[0]
	encoded, _ := json.Marshal(state)
	if len(encoded) > 32768 || !utf8.ValidString(state) {
		t.Fatalf("decision context invalid or over budget: %d serialized bytes", len(encoded))
	}
	if !strings.Contains(state, "Analyze this task.") || !strings.Contains(state, "Preserve the final constraint.") {
		t.Fatal("long current task lost its goal or final constraints")
	}
}

func TestSmartContextPreservesSimpleRequestState(t *testing.T) {
	for _, body := range []string{
		`{"model":"smart","messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"smart","input":"hello"}`,
		`{"model":"smart","prompt":"hello"}`,
	} {
		p := &criticProvider{models: []string{"a"}}
		m := criticModel("a")
		m.Endpoints.Responses, m.Endpoints.Completions = catalog.Supported, catalog.Supported
		h := criticHandler(t, p, m)
		c := &criticClassifier{result: classifier.Classification{TaskType: classifier.TaskGeneral, Complexity: classifier.ComplexityMedium, Confidence: 1}}
		h.SetClassifier(classifier.Runner{Classifier: c})
		path := "/v1/chat/completions"
		if strings.Contains(body, `"input"`) {
			path = "/v1/responses"
		} else if strings.Contains(body, `"prompt"`) {
			path = "/v1/completions"
		}
		w := criticPost(h, path, body)
		if w.Code != 200 || len(c.texts) != 1 || c.texts[0] != "hello" {
			t.Fatalf("simple request state changed: status=%d state=%q", w.Code, c.texts)
		}
	}
}

// Hermes titles use a schema and explicitly turn reasoning off. A local model
// with structured output must remain eligible even without reasoning support.
func TestSmartTitleWithDisabledReasoning(t *testing.T) {
	for _, effort := range []string{"none", "high"} {
		t.Run(effort, func(t *testing.T) {
			p := &criticProvider{models: []string{"title"}}
			m := criticModel("title")
			m.Capabilities.StructuredOutput = catalog.Supported
			h := criticHandler(t, p, m)
			body := `{"model":"smart","messages":[{"role":"user","content":"Give this session a short title."}],"reasoning_effort":"` + effort + `","response_format":{"type":"json_schema","json_schema":{"name":"session_title","strict":true,"schema":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}}}}`
			w := criticPost(h, "/v1/chat/completions", body)
			if effort == "none" {
				if w.Code != 200 || len(p.calls) != 1 {
					t.Fatalf("title rejected: status=%d calls=%v body=%s", w.Code, p.calls, w.Body.String())
				}
			} else if w.Code != 503 || len(p.calls) != 0 {
				t.Fatalf("required reasoning bypassed: status=%d calls=%v", w.Code, p.calls)
			}
		})
	}
}
