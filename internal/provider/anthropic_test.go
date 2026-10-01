package provider_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

type ipv4TestServer struct {
	URL      string
	client   *http.Client
	server   *http.Server
	listener net.Listener
}

func (s *ipv4TestServer) Client() *http.Client { return s.client }
func (s *ipv4TestServer) Close() {
	_ = s.server.Close()
	_ = s.listener.Close()
}

func newIPv4Server(t *testing.T, handler http.Handler) *ipv4TestServer {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener unavailable: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	testServer := &ipv4TestServer{URL: "http://" + listener.Addr().String(), client: &http.Client{}, server: server, listener: listener}
	t.Cleanup(testServer.Close)
	return testServer
}

func TestAnthropicTranslatesVisionAndTools(t *testing.T) {
	server := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "secret" {
			t.Fatalf("request=%s key=%q", r.URL.Path, r.Header.Get("x-api-key"))
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		messages := request["messages"].([]any)
		blocks := messages[0].(map[string]any)["content"].([]any)
		if blocks[1].(map[string]any)["type"] != "image" {
			t.Fatalf("content=%#v", blocks)
		}
		_, _ = io.WriteString(w, `{"id":"msg_1","model":"claude-sonnet-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":3,"output_tokens":2}}`)
	}))
	p, err := provider.NewAnthropic("anthropic", server.URL, "secret", nil, false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"claude-sonnet-5-5","max_tokens":20,"messages":[{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`), "claude-sonnet-5-5")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(data), `"chat.completion"`) || !strings.Contains(string(data), `"prompt_tokens":3`) {
		t.Fatalf("response=%s", data)
	}
}

func TestAnthropicTranslatesStreamingText(t *testing.T) {
	server := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
	}))
	p, err := provider.NewAnthropic("anthropic", server.URL, "secret", nil, false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"m","max_tokens":20,"stream":true,"messages":[{"role":"user","content":"hi"}]}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	var output strings.Builder
	for scanner.Scan() {
		output.WriteString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "hello") || !strings.Contains(output.String(), "[DONE]") {
		t.Fatalf("stream=%s", output.String())
	}
}
