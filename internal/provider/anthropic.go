package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Anthropic is the small translation adapter for Anthropic's Messages API.
// It intentionally returns OpenAI-shaped responses because the gateway's
// public contract is OpenAI-compatible.
type Anthropic struct {
	id       string
	baseURL  string
	apiKey   string
	client   *http.Client
	models   []string
	discover bool
}

func NewAnthropic(id, baseURL, apiKey string, models []string, discover bool, client *http.Client) (*Anthropic, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid Anthropic base URL")
	}
	if strings.TrimSpace(apiKey) == "" || strings.ContainsAny(apiKey, "\r\n") {
		return nil, fmt.Errorf("Anthropic API key is empty or contains a newline")
	}
	if client == nil {
		client = &http.Client{}
	}
	return &Anthropic{id: id, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: client, models: append([]string(nil), models...), discover: discover}, nil
}

func (p *Anthropic) ID() string { return p.id }

func (p *Anthropic) Models(ctx context.Context) ([]Model, error) {
	result := make([]Model, 0, len(p.models))
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		result = append(result, Model{ID: p.id + "/" + id, Object: "model", OwnedBy: "anthropic"})
	}
	for _, id := range p.models {
		add(id)
	}
	if !p.discover {
		return result, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover Anthropic models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discover Anthropic models: upstream returned %s", resp.Status)
	}
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode Anthropic models: %w", err)
	}
	for _, model := range envelope.Data {
		add(model.ID)
	}
	return result, nil
}

func (p *Anthropic) Forward(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	if endpoint != "/v1/chat/completions" {
		return nil, fmt.Errorf("Anthropic adapter does not support %q", endpoint)
	}
	var request openAIChatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("decode OpenAI chat request: %w", err)
	}
	request.Model = model
	if request.Stream {
		return nil, fmt.Errorf("Anthropic streaming translation is not supported")
	}
	anthropicRequest, err := translateAnthropicRequest(request)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(anthropicRequest)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.setHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send Anthropic request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	var message anthropicMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&message); err != nil {
		return nil, fmt.Errorf("decode Anthropic response: %w", err)
	}
	translated, err := translateOpenAIResponse(message)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(translated)
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded)), Request: req}, nil
}

func (p *Anthropic) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")
}

type openAIChatRequest struct {
	Model       string            `json:"model"`
	Messages    []openAIMessage   `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	System      json.RawMessage   `json:"system,omitempty"`
	Tools       []json.RawMessage `json:"tools,omitempty"`
	ToolChoice  json.RawMessage   `json:"tool_choice,omitempty"`
}

type openAIMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	System      json.RawMessage `json:"system,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	Tools       []anthropicTool `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func translateAnthropicRequest(in openAIChatRequest) (anthropicRequest, error) {
	if in.MaxTokens <= 0 {
		return anthropicRequest{}, fmt.Errorf("Anthropic requires a positive max_tokens value")
	}
	for i := range in.Messages {
		if in.Messages[i].Role == "system" {
			if len(in.System) == 0 {
				in.System = in.Messages[i].Content
			}
			in.Messages = append(in.Messages[:i], in.Messages[i+1:]...)
			break
		}
	}
	tools := make([]anthropicTool, 0, len(in.Tools))
	for _, raw := range in.Tools {
		var tool struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil || tool.Type != "function" || tool.Function.Name == "" || len(tool.Function.Parameters) == 0 {
			return anthropicRequest{}, fmt.Errorf("invalid OpenAI tool for Anthropic")
		}
		tools = append(tools, anthropicTool{Name: tool.Function.Name, Description: tool.Function.Description, InputSchema: tool.Function.Parameters})
	}
	toolChoice := in.ToolChoice
	if len(in.ToolChoice) > 0 {
		var choice struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if json.Unmarshal(in.ToolChoice, &choice) == nil && choice.Type == "function" && choice.Function.Name != "" {
			toolChoice, _ = json.Marshal(map[string]string{"type": "tool", "name": choice.Function.Name})
		}
	}
	return anthropicRequest{Model: in.Model, Messages: in.Messages, MaxTokens: in.MaxTokens, System: in.System, Temperature: in.Temperature, Tools: tools, ToolChoice: toolChoice}, nil
}

type anthropicMessage struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text,omitempty"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func translateOpenAIResponse(message anthropicMessage) (map[string]any, error) {
	var text strings.Builder
	toolCalls := make([]any, 0)
	for _, block := range message.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
		if block.Type == "tool_use" {
			toolCalls = append(toolCalls, map[string]any{"id": block.ID, "type": "function", "function": map[string]any{"name": block.Name, "arguments": string(block.Input)}})
		}
	}
	finish := message.StopReason
	if finish == "end_turn" || finish == "stop_sequence" {
		finish = "stop"
	}
	if finish == "max_tokens" {
		finish = "length"
	}
	if finish == "" {
		finish = "stop"
	}
	messageBody := map[string]any{"role": "assistant", "content": text.String()}
	if len(toolCalls) > 0 {
		messageBody["tool_calls"] = toolCalls
		messageBody["content"] = nil
	}
	return map[string]any{
		"id": message.ID, "object": "chat.completion", "created": time.Now().Unix(), "model": message.Model,
		"choices": []any{map[string]any{"index": 0, "message": messageBody, "finish_reason": finish}},
		"usage":   map[string]any{"prompt_tokens": message.Usage.InputTokens, "completion_tokens": message.Usage.OutputTokens, "total_tokens": message.Usage.InputTokens + message.Usage.OutputTokens},
	}, nil
}
