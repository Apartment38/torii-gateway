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
	Endpoint              Endpoint
	Tools                 bool
	Vision                bool
	StructuredOutput      bool
	Reasoning             bool
	Streaming             bool
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
	_, r.StructuredOutput = firstPresent(root, "response_format", "text")
	_, r.Reasoning = firstPresent(root, "reasoning", "reasoning_effort")
	r.Vision = containsImage(root["messages"]) || containsImage(root["input"])
	for _, key := range []string{"max_output_tokens", "max_completion_tokens", "max_tokens"} {
		if n, ok := positiveInteger(root[key]); ok {
			r.RequestedOutputTokens = n
			break
		}
	}
	return r, nil
}

func validEndpoint(e Endpoint) bool {
	return e == EndpointChat || e == EndpointCompletions || e == EndpointResponses || e == EndpointEmbeddings
}
func firstPresent(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
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
