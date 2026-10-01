package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	if endpoint == "/v1/responses" {
		chatBody, stream, err := responsesToChat(body, model)
		if err != nil {
			return nil, err
		}
		response, err := p.Forward(ctx, "/v1/chat/completions", chatBody, model)
		if err != nil {
			return response, err
		}
		if stream {
			response.Body = newAnthropicResponsesStreamBody(response.Body, model)
			return response, nil
		}
		defer response.Body.Close()
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		if readErr != nil {
			return nil, readErr
		}
		translated, translateErr := chatToResponses(data, model)
		if translateErr != nil {
			return nil, translateErr
		}
		return &http.Response{StatusCode: response.StatusCode, Status: response.Status, Header: response.Header, Body: io.NopCloser(bytes.NewReader(translated)), Request: response.Request}, nil
	}
	if endpoint != "/v1/chat/completions" {
		return nil, fmt.Errorf("Anthropic adapter does not support %q", endpoint)
	}
	var request openAIChatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("decode OpenAI chat request: %w", err)
	}
	request.Model = model
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
	if request.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	p.setHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send Anthropic request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}
	if request.Stream {
		resp.Body = newAnthropicStreamBody(resp.Body, model)
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

func responsesToChat(body []byte, model string) ([]byte, bool, error) {
	var request struct {
		Input           any    `json:"input"`
		Instructions    string `json:"instructions"`
		MaxOutputTokens int    `json:"max_output_tokens"`
		Stream          bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, false, fmt.Errorf("decode OpenAI Responses request: %w", err)
	}
	content, err := json.Marshal(request.Input)
	if err != nil || len(content) == 0 || string(content) == "null" {
		return nil, false, errors.New("Responses input is required")
	}
	messages := []openAIMessage{{Role: "user", Content: content}}
	if request.Instructions != "" {
		messages = append([]openAIMessage{{Role: "system", Content: json.RawMessage(strconv.Quote(request.Instructions))}}, messages...)
	}
	if array, ok := request.Input.([]any); ok {
		messages = make([]openAIMessage, 0, len(array))
		for _, item := range array {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, false, errors.New("Responses input items must be objects")
			}
			role, _ := object["role"].(string)
			if role == "" {
				role = "user"
			}
			value, marshalErr := json.Marshal(object["content"])
			if marshalErr != nil {
				return nil, false, marshalErr
			}
			messages = append(messages, openAIMessage{Role: role, Content: value})
		}
	}
	maxTokens := request.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	encoded, err := json.Marshal(openAIChatRequest{Model: model, Messages: messages, MaxTokens: maxTokens, Stream: request.Stream})
	return encoded, request.Stream, err
}

func chatToResponses(body []byte, model string) ([]byte, error) {
	var chat struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage any `json:"usage"`
	}
	if err := json.Unmarshal(body, &chat); err != nil {
		return nil, fmt.Errorf("decode translated chat response: %w", err)
	}
	if len(chat.Choices) == 0 {
		return nil, errors.New("translated chat response has no choices")
	}
	if chat.Model == "" {
		chat.Model = model
	}
	response := map[string]any{"id": chat.ID, "object": "response", "model": chat.Model, "status": "completed", "output": []any{map[string]any{"type": "message", "role": chat.Choices[0].Message.Role, "content": []any{map[string]any{"type": "output_text", "text": chat.Choices[0].Message.Content}}}}, "usage": chat.Usage}
	return json.Marshal(response)
}

func (p *Anthropic) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")
}

type openAIChatRequest struct {
	Model           string            `json:"model"`
	Messages        []openAIMessage   `json:"messages"`
	MaxTokens       int               `json:"max_tokens,omitempty"`
	Temperature     *float64          `json:"temperature,omitempty"`
	Stream          bool              `json:"stream,omitempty"`
	System          json.RawMessage   `json:"system,omitempty"`
	Tools           []json.RawMessage `json:"tools,omitempty"`
	ToolChoice      json.RawMessage   `json:"tool_choice,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	ResponseFormat  json.RawMessage   `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content"`
	ToolCalls  []json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type anthropicRequest struct {
	Model       string                  `json:"model"`
	Messages    []anthropicInputMessage `json:"messages"`
	MaxTokens   int                     `json:"max_tokens"`
	System      json.RawMessage         `json:"system,omitempty"`
	Temperature *float64                `json:"temperature,omitempty"`
	Tools       []anthropicTool         `json:"tools,omitempty"`
	ToolChoice  json.RawMessage         `json:"tool_choice,omitempty"`
	Stream      bool                    `json:"stream,omitempty"`
	Thinking    json.RawMessage         `json:"thinking,omitempty"`
}

type anthropicInputMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
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
	if len(in.ResponseFormat) > 0 {
		return anthropicRequest{}, errors.New("Anthropic structured output translation is not supported")
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
	messages, err := translateMessages(in.Messages)
	if err != nil {
		return anthropicRequest{}, err
	}
	var thinking json.RawMessage
	switch in.ReasoningEffort {
	case "low":
		thinking, _ = json.Marshal(map[string]any{"type": "enabled", "budget_tokens": 1024})
	case "medium":
		thinking, _ = json.Marshal(map[string]any{"type": "enabled", "budget_tokens": 4096})
	case "high":
		thinking, _ = json.Marshal(map[string]any{"type": "enabled", "budget_tokens": 8192})
	case "":
	default:
		return anthropicRequest{}, fmt.Errorf("unsupported reasoning_effort %q for Anthropic", in.ReasoningEffort)
	}
	return anthropicRequest{Model: in.Model, Messages: messages, MaxTokens: in.MaxTokens, System: in.System, Temperature: in.Temperature, Tools: tools, ToolChoice: toolChoice, Stream: in.Stream, Thinking: thinking}, nil
}

func translateMessages(messages []openAIMessage) ([]anthropicInputMessage, error) {
	result := make([]anthropicInputMessage, 0, len(messages))
	for _, message := range messages {
		content := bytes.TrimSpace(message.Content)
		if message.Role == "tool" {
			if message.ToolCallID == "" || len(content) == 0 {
				return nil, errors.New("tool messages require tool_call_id and content")
			}
			var text string
			if content[0] == '"' {
				if err := json.Unmarshal(content, &text); err != nil {
					return nil, fmt.Errorf("decode tool result content: %w", err)
				}
			} else {
				text = string(content)
			}
			result = append(result, anthropicInputMessage{Role: "user", Content: []map[string]any{{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": text}}})
			continue
		}
		if len(content) == 0 || string(content) == "null" {
			if message.Role == "assistant" && len(message.ToolCalls) > 0 {
				blocks, err := translateToolCalls(message.ToolCalls)
				if err != nil {
					return nil, err
				}
				result = append(result, anthropicInputMessage{Role: "assistant", Content: blocks})
				continue
			}
			return nil, errors.New("message content is required")
		}
		if content[0] == '"' {
			var text string
			if err := json.Unmarshal(content, &text); err != nil {
				return nil, fmt.Errorf("decode message content: %w", err)
			}
			result = append(result, anthropicInputMessage{Role: message.Role, Content: text})
			continue
		}
		var blocks []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url,omitempty"`
		}
		if err := json.Unmarshal(content, &blocks); err != nil {
			return nil, fmt.Errorf("decode message content blocks: %w", err)
		}
		converted := make([]map[string]any, 0, len(blocks))
		for _, block := range blocks {
			switch block.Type {
			case "text":
				converted = append(converted, map[string]any{"type": "text", "text": block.Text})
			case "image_url":
				if block.ImageURL == nil || block.ImageURL.URL == "" {
					return nil, errors.New("image_url content is missing url")
				}
				source, err := anthropicImageSource(block.ImageURL.URL)
				if err != nil {
					return nil, err
				}
				converted = append(converted, map[string]any{"type": "image", "source": source})
			default:
				return nil, fmt.Errorf("unsupported Anthropic content block %q", block.Type)
			}
		}
		result = append(result, anthropicInputMessage{Role: message.Role, Content: converted})
	}
	return result, nil
}

func translateToolCalls(rawCalls []json.RawMessage) ([]map[string]any, error) {
	blocks := make([]map[string]any, 0, len(rawCalls))
	for _, raw := range rawCalls {
		var call struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &call); err != nil || call.ID == "" || call.Function.Name == "" {
			return nil, errors.New("invalid assistant tool call for Anthropic")
		}
		var input any
		if call.Function.Arguments == "" {
			input = map[string]any{}
		} else if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
			return nil, fmt.Errorf("decode assistant tool arguments: %w", err)
		}
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": input})
	}
	return blocks, nil
}

func anthropicImageSource(raw string) (map[string]string, error) {
	if strings.HasPrefix(raw, "data:") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
			return nil, errors.New("Anthropic images require a base64 data URL or HTTPS URL")
		}
		mediaType := strings.TrimPrefix(strings.SplitN(parts[0], ";", 2)[0], "data:")
		return map[string]string{"type": "base64", "media_type": mediaType, "data": parts[1]}, nil
	}
	if strings.HasPrefix(raw, "https://") {
		return map[string]string{"type": "url", "url": raw}, nil
	}
	return nil, errors.New("Anthropic images require a base64 data URL or HTTPS URL")
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

type anthropicStreamBody struct {
	reader   *io.PipeReader
	upstream io.Closer
	once     sync.Once
}

func newAnthropicStreamBody(upstream io.ReadCloser, model string) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &anthropicStreamBody{reader: reader, upstream: upstream}
	go func() {
		defer writer.Close()
		defer upstream.Close()
		scanner := bufio.NewScanner(upstream)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		event := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				event = strings.TrimPrefix(line, "event: ")
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			chunk := anthropicStreamChunk(event, strings.TrimPrefix(line, "data: "), model)
			if chunk == "" {
				continue
			}
			if _, err := io.WriteString(writer, "data: "+chunk+"\n\n"); err != nil {
				return
			}
			event = ""
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}()
	return body
}

func (b *anthropicStreamBody) Read(p []byte) (int, error) { return b.reader.Read(p) }
func (b *anthropicStreamBody) Close() error {
	b.once.Do(func() { _ = b.reader.Close(); _ = b.upstream.Close() })
	return nil
}

type anthropicResponsesStreamBody struct {
	reader   *io.PipeReader
	upstream io.ReadCloser
	once     sync.Once
}

func newAnthropicResponsesStreamBody(upstream io.ReadCloser, model string) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &anthropicResponsesStreamBody{reader: reader, upstream: upstream}
	go func() {
		defer writer.Close()
		defer upstream.Close()
		encoder := bufio.NewScanner(upstream)
		encoder.Buffer(make([]byte, 4096), 1<<20)
		_, _ = io.WriteString(writer, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"object\":\"response\",\"model\":\""+model+"\"}}\n\n")
		for encoder.Scan() {
			line := encoder.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			if choice.Delta.Content != "" {
				event := map[string]any{"type": "response.output_text.delta", "delta": choice.Delta.Content}
				encoded, _ := json.Marshal(event)
				if _, err := io.WriteString(writer, "event: response.output_text.delta\ndata: "+string(encoded)+"\n\n"); err != nil {
					return
				}
			}
			if choice.FinishReason != nil {
				encoded, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"object": "response", "model": model, "status": "completed"}})
				_, _ = io.WriteString(writer, "event: response.completed\ndata: "+string(encoded)+"\n\n")
			}
		}
	}()
	return body
}

func (b *anthropicResponsesStreamBody) Read(p []byte) (int, error) { return b.reader.Read(p) }
func (b *anthropicResponsesStreamBody) Close() error {
	b.once.Do(func() { _ = b.reader.Close(); _ = b.upstream.Close() })
	return nil
}

func anthropicStreamChunk(event, data, model string) string {
	var envelope struct {
		Message struct{ ID, Model string } `json:"message"`
		Delta   struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			StopReason  string `json:"stop_reason"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
		Index int `json:"index"`
	}
	if json.Unmarshal([]byte(data), &envelope) != nil {
		return ""
	}
	if model == "" {
		model = envelope.Message.Model
	}
	chunk := map[string]any{"id": envelope.Message.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model, "choices": []any{map[string]any{"index": envelope.Index, "delta": map[string]any{}, "finish_reason": nil}}}
	choice := chunk["choices"].([]any)[0].(map[string]any)
	delta := choice["delta"].(map[string]any)
	switch event {
	case "content_block_delta":
		if envelope.Delta.Type == "text_delta" {
			delta["content"] = envelope.Delta.Text
		} else if envelope.Delta.Type == "input_json_delta" {
			delta["tool_calls"] = []any{map[string]any{"index": envelope.Index, "function": map[string]string{"arguments": envelope.Delta.PartialJSON}}}
		} else {
			return ""
		}
	case "message_delta":
		if envelope.Delta.StopReason != "" {
			finish := envelope.Delta.StopReason
			if finish == "end_turn" || finish == "stop_sequence" {
				finish = "stop"
			} else if finish == "max_tokens" {
				finish = "length"
			}
			choice["finish_reason"] = finish
		}
	default:
		return ""
	}
	encoded, _ := json.Marshal(chunk)
	return string(encoded)
}
