// Package transport contains transparent and translated upstream transports.
package transport

import (
	"encoding/json"
	"errors"
	"fmt"
)

// CanonicalEndpoint identifies the public request/response shape being
// translated. Transparent providers do not use CanonicalRequest.
type CanonicalEndpoint string

const (
	CanonicalChat      CanonicalEndpoint = "chat.completions"
	CanonicalResponses CanonicalEndpoint = "responses"
)

// CanonicalRequest is the translation-only representation shared by protocol
// adapters. RawExtension retains fields an adapter does not interpret so a
// translator must either preserve them or explicitly reject them.
type CanonicalRequest struct {
	Endpoint        CanonicalEndpoint          `json:"endpoint"`
	Model           string                     `json:"model"`
	Instructions    string                     `json:"instructions,omitempty"`
	Messages        []CanonicalMessage         `json:"messages,omitempty"`
	Tools           []CanonicalTool            `json:"tools,omitempty"`
	ToolChoice      json.RawMessage            `json:"tool_choice,omitempty"`
	OutputFormat    json.RawMessage            `json:"output_format,omitempty"`
	Reasoning       *CanonicalReasoning        `json:"reasoning,omitempty"`
	MaxOutputTokens *int                       `json:"max_output_tokens,omitempty"`
	Stream          bool                       `json:"stream,omitempty"`
	RawExtension    map[string]json.RawMessage `json:"extensions,omitempty"`
}

type CanonicalMessage struct {
	Role       string               `json:"role"`
	Content    []CanonicalContent   `json:"content,omitempty"`
	ToolCalls  []CanonicalToolCall  `json:"tool_calls,omitempty"`
	ToolResult *CanonicalToolResult `json:"tool_result,omitempty"`
	Name       string               `json:"name,omitempty"`
}

type CanonicalContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL string          `json:"image_url,omitempty"`
	Detail   string          `json:"detail,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

type CanonicalTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type CanonicalToolCall struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type CanonicalToolResult struct {
	CallID  string             `json:"call_id"`
	Content []CanonicalContent `json:"content"`
}

type CanonicalReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type CanonicalResponse struct {
	ID           string                     `json:"id"`
	Model        string                     `json:"model"`
	Output       []CanonicalMessage         `json:"output,omitempty"`
	FinishReason string                     `json:"finish_reason,omitempty"`
	Usage        CanonicalUsage             `json:"usage"`
	RawExtension map[string]json.RawMessage `json:"extensions,omitempty"`
}

type CanonicalUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
}

type CanonicalDelta struct {
	ResponseID   string             `json:"response_id,omitempty"`
	Index        int                `json:"index"`
	Role         string             `json:"role,omitempty"`
	Content      *CanonicalContent  `json:"content,omitempty"`
	ToolCall     *CanonicalToolCall `json:"tool_call,omitempty"`
	FinishReason string             `json:"finish_reason,omitempty"`
	Usage        *CanonicalUsage    `json:"usage,omitempty"`
}

// Validate rejects ambiguous or lossy canonical requests before an adapter
// emits provider-specific traffic.
func (r CanonicalRequest) Validate() error {
	if r.Endpoint != CanonicalChat && r.Endpoint != CanonicalResponses {
		return errors.New("canonical request has unsupported endpoint")
	}
	if r.Model == "" {
		return errors.New("canonical request model is required")
	}
	if r.MaxOutputTokens != nil && *r.MaxOutputTokens <= 0 {
		return errors.New("max output tokens must be positive")
	}
	for i, message := range r.Messages {
		switch message.Role {
		case "system", "developer", "user", "assistant", "tool":
		default:
			return fmt.Errorf("message %d has unsupported role %q", i, message.Role)
		}
		if message.Role == "tool" && message.ToolResult == nil {
			return fmt.Errorf("tool message %d has no tool result", i)
		}
		for j, content := range message.Content {
			if err := content.validate(); err != nil {
				return fmt.Errorf("message %d content %d: %w", i, j, err)
			}
		}
	}
	for i, tool := range r.Tools {
		if tool.Type != "function" || tool.Name == "" || !validRawObject(tool.Parameters) {
			return fmt.Errorf("tool %d is invalid", i)
		}
	}
	if len(r.OutputFormat) > 0 && !json.Valid(r.OutputFormat) {
		return errors.New("output format is invalid JSON")
	}
	if len(r.ToolChoice) > 0 && !json.Valid(r.ToolChoice) {
		return errors.New("tool choice is invalid JSON")
	}
	for key, value := range r.RawExtension {
		if key == "" || !json.Valid(value) {
			return fmt.Errorf("extension %q is invalid", key)
		}
	}
	return nil
}

func (c CanonicalContent) validate() error {
	switch c.Type {
	case "text":
		if c.Text == "" {
			return errors.New("text content is empty")
		}
	case "image_url":
		if c.ImageURL == "" {
			return errors.New("image URL is empty")
		}
	case "raw":
		if !json.Valid(c.Raw) {
			return errors.New("raw content is invalid JSON")
		}
	default:
		return fmt.Errorf("unsupported content type %q", c.Type)
	}
	return nil
}

func validRawObject(raw json.RawMessage) bool {
	if !json.Valid(raw) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}
