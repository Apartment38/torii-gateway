package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

const maxClassificationResponse = 1 << 20

// ConnectedModel invokes a configured physical OpenAI-compatible model. It is
// deliberately independent of the smart virtual model, so recursive routing
// cannot occur accidentally.
type ConnectedModel struct {
	model         string
	providers     map[string]provider.OpenAIProvider
	clientTimeout time.Duration
}

func NewConnectedModel(model string, providers []provider.OpenAIProvider) (*ConnectedModel, error) {
	if model == "" || model == "smart" {
		return nil, errors.New("a physical classifier model is required")
	}
	byID := make(map[string]provider.OpenAIProvider, len(providers))
	for _, p := range providers {
		if p == nil {
			return nil, errors.New("nil classifier provider")
		}
		byID[p.ID()] = p
	}
	return &ConnectedModel{model: model, providers: byID, clientTimeout: 10 * time.Second}, nil
}

func (c *ConnectedModel) ID() string { return c.model }

func (c *ConnectedModel) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	result, err := c.classifyOnce(ctx, request)
	if errors.Is(err, ErrMalformedClassification) {
		return c.classifyOnce(ctx, request)
	}
	return result, err
}

var ErrMalformedClassification = errors.New("malformed classifier output")

func (c *ConnectedModel) classifyOnce(ctx context.Context, request ClassificationRequest) (Classification, error) {
	if request.Model == "smart" {
		return Classification{}, errors.New("classifier recursion is forbidden")
	}
	providerID, upstream, err := provider.ParseModelID(c.model)
	if err != nil {
		return Classification{}, err
	}
	p := c.providers[providerID]
	if p == nil {
		return Classification{}, fmt.Errorf("classifier provider %q is unavailable", providerID)
	}
	body, err := json.Marshal(map[string]any{
		"model": upstream,
		"messages": []map[string]any{
			{"role": "system", "content": "Classify the request as JSON only. Do not select a model."},
			{"role": "user", "content": request.Text},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "classification", "strict": true, "schema": classificationSchema}},
		"max_tokens":      256,
	})
	if err != nil {
		return Classification{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.clientTimeout)
	defer cancel()
	resp, err := p.Forward(callCtx, "/v1/chat/completions", body, upstream)
	if err != nil {
		return Classification{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Classification{}, fmt.Errorf("classifier upstream returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxClassificationResponse+1))
	if err != nil || len(data) > maxClassificationResponse {
		return Classification{}, errors.New("classifier response is too large")
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		return Classification{}, ErrMalformedClassification
	}
	var result Classification
	decoder := json.NewDecoder(bytes.NewReader([]byte(envelope.Choices[0].Message.Content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Classification{}, fmt.Errorf("%w: decode classification: %v", ErrMalformedClassification, err)
	}
	if err := result.Validate(); err != nil {
		return Classification{}, fmt.Errorf("%w: %v", ErrMalformedClassification, err)
	}
	return result, nil
}

var classificationSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"task_type", "complexity", "confidence", "requirements", "preferences"},
	"properties": map[string]any{
		"task_type": map[string]any{"type": "string"}, "complexity": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"},
		"requirements": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"tools", "vision", "reasoning", "structured_output"}, "properties": map[string]any{"tools": map[string]any{"type": "boolean"}, "vision": map[string]any{"type": "boolean"}, "reasoning": map[string]any{"type": "boolean"}, "structured_output": map[string]any{"type": "boolean"}}},
		"preferences":  map[string]any{"type": "object", "additionalProperties": false, "required": []string{"latency_sensitive"}, "properties": map[string]any{"latency_sensitive": map[string]any{"type": "boolean"}}},
	},
}

var _ Classifier = (*ConnectedModel)(nil)
