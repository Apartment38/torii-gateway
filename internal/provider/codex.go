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
	"strings"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/requestid"
)

// Codex is the endpoint-bound ChatGPT subscription provider. Its credential
// store is intentionally read only by this provider; normal OpenAI API
// providers never use it.
type Codex struct {
	id        string
	base      *url.URL
	models    []string
	client    *http.Client
	load      func(string) (CodexCredential, error)
	save      func(string, CodexCredential) error
	issuer    string
	refreshMu sync.Mutex
}

type CodexCredential struct {
	AccessToken, RefreshToken, Endpoint string
	ExpiresAt                           time.Time
	AccountID                           string
	ClientID, IDToken, Issuer           string
	Subject, Email, HostID              string
	Scopes                              []string
}

func NewCodex(id, baseURL string, models []string, load func(string) (CodexCredential, error), client *http.Client) (*Codex, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Codex base URL must be HTTPS without credentials, query, or fragment")
	}
	if load == nil {
		return nil, errors.New("Codex credential store is required")
	}
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Codex{id: id, base: u, models: append([]string(nil), models...), client: client, load: load}, nil
}

func (c *Codex) ConfigureRefresh(issuer string, save func(string, CodexCredential) error) {
	c.issuer = strings.TrimRight(issuer, "/")
	c.save = save
}
func (c *Codex) ID() string { return c.id }
func (c *Codex) Models(context.Context) ([]Model, error) {
	result := make([]Model, 0, len(c.models))
	for _, id := range c.models {
		result = append(result, Model{ID: c.id + "/" + id, Object: "model", OwnedBy: c.id})
	}
	return result, nil
}

func (c *Codex) Forward(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	if endpoint != "/v1/chat/completions" && endpoint != "/v1/responses" {
		return nil, fmt.Errorf("Codex does not support %s", endpoint)
	}
	credential, err := c.load(c.id)
	if err != nil {
		return nil, fmt.Errorf("load Codex credential: %w", err)
	}
	if credential.Endpoint != c.base.String() {
		return nil, errors.New("Codex credential endpoint binding does not match provider")
	}
	if !credential.ExpiresAt.IsZero() && !credential.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
		if credential.RefreshToken == "" || c.save == nil || c.issuer == "" {
			return nil, errors.New("Codex credential is expired or cannot be refreshed")
		}
		c.refreshMu.Lock()
		defer c.refreshMu.Unlock()
		// Another request may have refreshed and persisted a rotated token while
		// this request was waiting for the lock. Re-read before making a second
		// token exchange.
		latest, loadErr := c.load(c.id)
		if loadErr == nil && latest.Endpoint == c.base.String() {
			credential = latest
		}
		if !credential.ExpiresAt.IsZero() && !credential.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
			credential, err = c.refresh(ctx, credential)
			if err != nil {
				return nil, err
			}
		}
	}
	translated, stream, err := codexRequest(endpoint, body, model)
	if err != nil {
		return nil, err
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	if credential.AccountID != "" {
		req.Header.Set("chatgpt-account-id", credential.AccountID)
	}
	if id, ok := requestid.FromContext(ctx); ok {
		req.Header.Set("X-Request-ID", id)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Codex request: %w", err)
	}
	if stream || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body = &codexStreamReader{reader: bufio.NewReader(resp.Body), closer: resp.Body}
			resp.Header.Set("Content-Type", "text/event-stream")
		}
		return resp, nil
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	converted, err := codexResponse(endpoint, data, model)
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(converted))
	resp.ContentLength = int64(len(converted))
	resp.Header.Set("Content-Type", "application/json")
	return resp, nil
}

type codexStreamReader struct {
	reader  *bufio.Reader
	closer  io.Closer
	pending []byte
	done    bool
	id      string
}

func (r *codexStreamReader) Read(out []byte) (int, error) {
	for len(r.pending) == 0 && !r.done {
		data, err := r.nextEvent()
		if err != nil {
			if errors.Is(err, io.EOF) {
				r.done = true
				break
			}
			return 0, err
		}
		r.pending = append(r.pending, data...)
	}
	if len(r.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(out, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
func (r *codexStreamReader) Close() error { return r.closer.Close() }
func (r *codexStreamReader) nextEvent() ([]byte, error) {
	var data string
	for {
		line, err := r.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if data == "" {
				if err != nil {
					return nil, err
				}
				continue
			}
			var event struct {
				Type     string `json:"type"`
				Delta    string `json:"delta"`
				Response struct {
					ID string `json:"id"`
				} `json:"response"`
			}
			if json.Unmarshal([]byte(data), &event) != nil {
				data = ""
				if err != nil {
					return nil, err
				}
				continue
			}
			if event.Response.ID != "" {
				r.id = event.Response.ID
			}
			var output []byte
			switch event.Type {
			case "response.output_text.delta":
				output, _ = json.Marshal(map[string]any{"id": r.id, "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": event.Delta}, "finish_reason": nil}}})
			case "response.completed", "response.done":
				output, _ = json.Marshal(map[string]any{"id": r.id, "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
				output = append([]byte("data: "), output...)
				output = append(output, '\n', '\n')
				output = append(output, []byte("data: [DONE]\n\n")...)
				r.done = true
			}
			if len(output) > 0 && !r.done {
				output = append([]byte("data: "), output...)
				output = append(output, '\n', '\n')
			}
			return output, nil
		}
		if strings.HasPrefix(line, "data:") {
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if err != nil {
			return nil, err
		}
	}
}

func (c *Codex) refresh(ctx context.Context, credential CodexCredential) (CodexCredential, error) {
	clientID := credential.ClientID
	if clientID == "" {
		clientID = CodexOAuthClientID
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {credential.RefreshToken}, "client_id": {clientID}, "resource": {"https://api.openai.com/v1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+"/api/accounts/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return credential, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return credential, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return credential, fmt.Errorf("Codex token refresh returned %s", response.Status)
	}
	var refreshed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		IDToken      string `json:"id_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&refreshed); err != nil || refreshed.AccessToken == "" {
		return credential, errors.New("Codex token refresh response is invalid")
	}
	credential.AccessToken = refreshed.AccessToken
	if refreshed.RefreshToken != "" {
		credential.RefreshToken = refreshed.RefreshToken
	}
	if refreshed.IDToken != "" {
		credential.IDToken = refreshed.IDToken
	}
	if refreshed.Scope != "" {
		credential.Scopes = strings.Fields(refreshed.Scope)
	}
	if refreshed.ExpiresIn > 0 {
		credential.ExpiresAt = time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	}
	if err := c.save(c.id, credential); err != nil {
		return credential, fmt.Errorf("persist refreshed Codex credential: %w", err)
	}
	return credential, nil
}

func codexRequest(endpoint string, body []byte, model string) ([]byte, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, false, err
	}
	result := map[string]any{"model": model, "store": false}
	var stream bool
	if raw := root["stream"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &stream); err != nil {
			return nil, false, errors.New("stream must be boolean")
		}
	}
	result["stream"] = stream
	if endpoint == "/v1/responses" {
		var input any
		if raw := root["input"]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &input)
		}
		if input == nil {
			input = root["messages"]
		}
		result["input"] = input
	} else {
		var messages []map[string]any
		if err := json.Unmarshal(root["messages"], &messages); err != nil {
			return nil, false, errors.New("messages are required")
		}
		input, err := translateChatMessages(messages)
		if err != nil {
			return nil, false, err
		}
		result["input"] = input
	}
	if raw := root["tools"]; len(raw) > 0 {
		tools, err := translateTools(raw)
		if err != nil {
			return nil, false, err
		}
		result["tools"] = tools
	}
	for _, key := range []string{"tool_choice", "reasoning", "max_output_tokens", "response_format"} {
		if raw := root[key]; len(raw) > 0 {
			var value any
			if json.Unmarshal(raw, &value) == nil {
				result[key] = value
			}
		}
	}
	data, err := json.Marshal(result)
	return data, stream, err
}

func translateChatMessages(messages []map[string]any) ([]any, error) {
	result := make([]any, 0, len(messages))
	for _, message := range messages {
		role, _ := message["role"].(string)
		if role == "" {
			return nil, errors.New("Codex message role is required")
		}
		if role != "user" && role != "system" && role != "developer" && role != "assistant" && role != "tool" {
			return nil, fmt.Errorf("Codex message role %q is unsupported", role)
		}
		if role == "tool" {
			callID, _ := message["tool_call_id"].(string)
			if callID == "" {
				return nil, errors.New("Codex tool result is missing tool_call_id")
			}
			result = append(result, map[string]any{"type": "function_call_output", "call_id": callID, "output": message["content"]})
			continue
		}
		if role == "assistant" {
			if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
				for _, raw := range calls {
					call, ok := raw.(map[string]any)
					if !ok {
						return nil, errors.New("Codex assistant tool call is invalid")
					}
					function, ok := call["function"].(map[string]any)
					if !ok {
						return nil, errors.New("Codex assistant function call is invalid")
					}
					result = append(result, map[string]any{"type": "function_call", "id": call["id"], "call_id": call["id"], "name": function["name"], "arguments": function["arguments"]})
				}
				continue
			}
		}
		content := make([]any, 0)
		switch value := message["content"].(type) {
		case string:
			content = append(content, map[string]any{"type": map[string]string{"user": "input_text", "system": "input_text", "developer": "input_text", "assistant": "output_text"}[role], "text": value})
		case []any:
			for _, item := range value {
				part, ok := item.(map[string]any)
				if !ok {
					return nil, errors.New("Codex message content item is invalid")
				}
				kind, _ := part["type"].(string)
				switch kind {
				case "text":
					content = append(content, map[string]any{"type": "input_text", "text": part["text"]})
				case "image_url":
					image, _ := part["image_url"].(map[string]any)
					if image == nil || image["url"] == nil {
						return nil, errors.New("Codex image_url content is invalid")
					}
					content = append(content, map[string]any{"type": "input_image", "image_url": image["url"]})
				default:
					return nil, fmt.Errorf("Codex does not support message content type %q", kind)
				}
			}
		default:
			return nil, errors.New("Codex message content is required")
		}
		item := map[string]any{"role": role, "content": content}
		if role == "system" {
			item["role"] = "developer"
		}
		result = append(result, item)
	}
	return result, nil
}

func translateTools(raw json.RawMessage) ([]any, error) {
	var tools []map[string]any
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, errors.New("Codex tools are invalid")
	}
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		if tool["type"] != "function" {
			return nil, errors.New("Codex supports function tools only")
		}
		function, ok := tool["function"].(map[string]any)
		if !ok {
			return nil, errors.New("Codex function tool is invalid")
		}
		result = append(result, map[string]any{"type": "function", "name": function["name"], "description": function["description"], "parameters": function["parameters"], "strict": function["strict"]})
	}
	return result, nil
}

func codexResponse(endpoint string, data []byte, model string) ([]byte, error) {
	if endpoint == "/v1/responses" {
		return data, nil
	}
	var response struct {
		ID     string `json:"id"`
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			CallID    string `json:"call_id"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	var text string
	toolCalls := make([]any, 0)
	for _, item := range response.Output {
		if item.Type == "function_call" {
			toolCalls = append(toolCalls, map[string]any{"id": item.CallID, "type": "function", "function": map[string]any{"name": item.Name, "arguments": item.Arguments}})
			continue
		}
		for _, content := range item.Content {
			text += content.Text
		}
	}
	message := map[string]any{"role": "assistant", "content": text}
	finish := "stop"
	if len(toolCalls) > 0 {
		message["content"] = nil
		message["tool_calls"] = toolCalls
		finish = "tool_calls"
	}
	return json.Marshal(map[string]any{"id": response.ID, "object": "chat.completion", "model": model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": response.Usage.InputTokens, "completion_tokens": response.Usage.OutputTokens, "total_tokens": response.Usage.InputTokens + response.Usage.OutputTokens}})
}
