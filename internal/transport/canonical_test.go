package transport_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Apartment38/torii-router/internal/transport"
)

func TestCanonicalRequestRoundTripPreservesTranslationFeatures(t *testing.T) {
	limit := 2048
	want := transport.CanonicalRequest{
		Endpoint: transport.CanonicalResponses, Model: "openai-codex/codex", Instructions: "Be exact.", Stream: true, MaxOutputTokens: &limit,
		Messages: []transport.CanonicalMessage{
			{Role: "developer", Content: []transport.CanonicalContent{{Type: "text", Text: "Use Go."}}},
			{Role: "user", Content: []transport.CanonicalContent{{Type: "text", Text: "Inspect this"}, {Type: "image_url", ImageURL: "data:image/png;base64,AA==", Detail: "high"}}},
			{Role: "assistant", ToolCalls: []transport.CanonicalToolCall{{ID: "call-1", Type: "function", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)}}},
			{Role: "tool", ToolResult: &transport.CanonicalToolResult{CallID: "call-1", Content: []transport.CanonicalContent{{Type: "text", Text: "package main"}}}},
		},
		Tools:      []transport.CanonicalTool{{Type: "function", Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: json.RawMessage(`{"type":"function","name":"read"}`), OutputFormat: json.RawMessage(`{"type":"json_schema","name":"answer"}`),
		Reasoning: &transport.CanonicalReasoning{Effort: "high", Summary: "auto"}, RawExtension: map[string]json.RawMessage{"parallel_tool_calls": json.RawMessage(`true`)},
	}
	if err := want.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got transport.CanonicalRequest
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestCanonicalResponseAndDeltaRoundTrip(t *testing.T) {
	values := []any{
		transport.CanonicalResponse{ID: "resp-1", Model: "codex", Output: []transport.CanonicalMessage{{Role: "assistant", Content: []transport.CanonicalContent{{Type: "text", Text: "done"}}}}, FinishReason: "stop", Usage: transport.CanonicalUsage{InputTokens: 10, OutputTokens: 4, ReasoningTokens: 2, CachedTokens: 3}, RawExtension: map[string]json.RawMessage{"service_tier": json.RawMessage(`"default"`)}},
		transport.CanonicalDelta{ResponseID: "resp-1", Index: 0, Content: &transport.CanonicalContent{Type: "text", Text: "do"}, Usage: &transport.CanonicalUsage{OutputTokens: 1}},
	}
	for _, want := range values {
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got := reflect.New(reflect.TypeOf(want))
		if err := json.Unmarshal(encoded, got.Interface()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Elem().Interface(), want) {
			t.Fatalf("round trip mismatch: %#v", got.Elem().Interface())
		}
	}
}

func TestCanonicalRequestValidation(t *testing.T) {
	valid := transport.CanonicalRequest{Endpoint: transport.CanonicalChat, Model: "physical/model", Messages: []transport.CanonicalMessage{{Role: "user", Content: []transport.CanonicalContent{{Type: "text", Text: "hello"}}}}}
	tests := []struct {
		name   string
		mutate func(*transport.CanonicalRequest)
		want   string
	}{
		{"endpoint", func(r *transport.CanonicalRequest) { r.Endpoint = "files" }, "endpoint"},
		{"model", func(r *transport.CanonicalRequest) { r.Model = "" }, "model"},
		{"role", func(r *transport.CanonicalRequest) { r.Messages[0].Role = "unknown" }, "role"},
		{"content", func(r *transport.CanonicalRequest) { r.Messages[0].Content[0].Text = "" }, "empty"},
		{"tool schema", func(r *transport.CanonicalRequest) {
			r.Tools = []transport.CanonicalTool{{Type: "function", Name: "x", Parameters: json.RawMessage(`[]`)}}
		}, "tool"},
		{"extension", func(r *transport.CanonicalRequest) {
			r.RawExtension = map[string]json.RawMessage{"x": json.RawMessage(`{`)}
		}, "extension"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid
			r.Messages = append([]transport.CanonicalMessage(nil), valid.Messages...)
			r.Messages[0].Content = append([]transport.CanonicalContent(nil), valid.Messages[0].Content...)
			tt.mutate(&r)
			if err := r.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}
