// Package router contains deterministic request inspection and model selection.
package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Endpoint string

const (
	EndpointChat        Endpoint = "chat.completions"
	EndpointCompletions Endpoint = "completions"
	EndpointResponses   Endpoint = "responses"
	EndpointEmbeddings  Endpoint = "embeddings"
)

type RequestRequirements struct {
	TaskType              string
	Endpoint              Endpoint
	Tools                 bool
	Vision                bool
	StructuredOutput      bool
	Reasoning             bool
	Streaming             bool
	LatencySensitive      bool
	EstimatedInputTokens  int64
	RequestedOutputTokens int64
}

// RequestEnvelope retains raw bytes for transparent forwarding while carrying
// the objective facts needed by classification and selection.
type RequestEnvelope struct {
	Endpoint        Endpoint
	RawBody         []byte
	ModelPublicID   string
	ModelUpstreamID string
	Requirements    RequestRequirements
}

func ExtractRequirements(endpoint Endpoint, body []byte) (RequestRequirements, error) {
	if !validEndpoint(endpoint) {
		return RequestRequirements{}, fmt.Errorf("unsupported endpoint %q", endpoint)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return RequestRequirements{}, fmt.Errorf("decode requirements: %w", err)
	}
	if root == nil {
		return RequestRequirements{}, errors.New("request must be an object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return RequestRequirements{}, errors.New("request must contain one JSON value")
	}
	r := RequestRequirements{Endpoint: endpoint, EstimatedInputTokens: estimateTokens(root)}
	if endpoint == EndpointEmbeddings {
		return r, nil
	}
	if stream, ok := root["stream"].(bool); ok {
		r.Streaming = stream
	}
	if tools, ok := root["tools"].([]any); ok && len(tools) > 0 {
		r.Tools = true
	}
	if functions, ok := root["functions"].([]any); ok && len(functions) > 0 {
		r.Tools = true
	}
	r.Tools = r.Tools || containsTool(root["messages"]) || containsTool(root["input"])
	r.StructuredOutput = structuredFormat(root["response_format"])
	if text, ok := root["text"].(map[string]any); ok {
		r.StructuredOutput = r.StructuredOutput || structuredFormat(text["format"])
	}
	r.Reasoning = reasoningRequested(root)
	r.Vision = containsImage(root["messages"]) || containsImage(root["input"])
	for _, key := range []string{"max_output_tokens", "max_completion_tokens", "max_tokens"} {
		if n, ok := positiveInteger(root[key]); ok {
			r.RequestedOutputTokens = n
			break
		}
	}
	return r, nil
}

func structuredFormat(v any) bool {
	format, _ := v.(map[string]any)
	kind, _ := format["type"].(string)
	return kind == "json_schema" || kind == "json_object"
}

func containsTool(v any) bool {
	switch value := v.(type) {
	case []any:
		for _, item := range value {
			if containsTool(item) {
				return true
			}
		}
	case map[string]any:
		role, _ := value["role"].(string)
		kind, _ := value["type"].(string)
		if role == "tool" || role == "function" || kind == "function_call" || kind == "function_call_output" {
			return true
		}
		if calls, ok := value["tool_calls"].([]any); ok && len(calls) > 0 {
			return true
		}
		if call, ok := value["function_call"].(map[string]any); ok && len(call) > 0 {
			return true
		}
		for _, nested := range value {
			if containsTool(nested) {
				return true
			}
		}
	}
	return false
}

func validEndpoint(e Endpoint) bool {
	return e == EndpointChat || e == EndpointCompletions || e == EndpointResponses || e == EndpointEmbeddings
}

// Disabling reasoning does not require a reasoning-capable model. Keep unknown
// non-null controls conservative and combine both API forms independently.
func reasoningRequested(root map[string]any) bool {
	if effort, ok := root["reasoning_effort"]; ok && effort != nil && effort != "none" {
		return true
	}
	value := root["reasoning"]
	if value == nil {
		return false
	}
	if reasoning, ok := value.(map[string]any); ok {
		return reasoning["effort"] != "none" || len(reasoning) != 1
	}
	return true
}
func positiveInteger(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil && i > 0
}
func containsImage(v any) bool {
	switch value := v.(type) {
	case []any:
		for _, item := range value {
			if containsImage(item) {
				return true
			}
		}
	case map[string]any:
		if kind, _ := value["type"].(string); kind == "image_url" || kind == "input_image" || kind == "image" {
			return true
		}
		for _, nested := range value {
			if containsImage(nested) {
				return true
			}
		}
	}
	return false
}
func estimateTokens(root map[string]any) int64 {
	data, _ := json.Marshal(root)
	if len(data) == 0 {
		return 0
	}
	return int64((len(data) + 3) / 4)
}
