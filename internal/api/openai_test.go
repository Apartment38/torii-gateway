package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-router/internal/api"
	"github.com/Apartment38/torii-router/internal/provider"
)

func TestOpenAIHandlerOllamaEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"qwen3","object":"model"}]}`)
		case "/v1/chat/completions":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), `local/qwen3`) || !strings.Contains(string(body), `"extension":{"keep":true}`) {
				t.Errorf("unexpected upstream body: %s", body)
			}
			w.Header().Set("X-Request-Id", "upstream-1")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ollama, err := provider.NewOllama("local", upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewOpenAIHandler([]provider.ChatCompletionsProvider{ollama})
	if err != nil {
		t.Fatal(err)
	}

	models := httptest.NewRecorder()
	handler.ServeHTTP(models, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"id":"local/qwen3"`) {
		t.Fatalf("models response = %d %s", models.Code, models.Body.String())
	}
	single := httptest.NewRecorder()
	handler.ServeHTTP(single, httptest.NewRequest(http.MethodGet, "/v1/models/local/qwen3", nil))
	if single.Code != http.StatusOK || !strings.Contains(single.Body.String(), `"id":"local/qwen3"`) {
		t.Fatalf("model response = %d %s", single.Code, single.Body.String())
	}
	chat := httptest.NewRecorder()
	body := `{"model":"local/qwen3","messages":[{"role":"user","content":"hi"}],"extension":{"keep":true}}`
	handler.ServeHTTP(chat, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if chat.Code != http.StatusOK || !strings.Contains(chat.Body.String(), `"content":"hello"`) {
		t.Fatalf("chat response = %d %s", chat.Code, chat.Body.String())
	}
	if chat.Header().Get("X-Request-Id") != "upstream-1" {
		t.Fatalf("X-Request-Id = %q", chat.Header().Get("X-Request-Id"))
	}
}

type failingProvider struct{}

func (failingProvider) ID() string { return "broken" }
func (failingProvider) Models(context.Context) ([]provider.Model, error) {
	return nil, context.Canceled
}
func (failingProvider) ChatCompletions(context.Context, []byte, string) (*http.Response, error) {
	return nil, context.Canceled
}

func TestOpenAIHandlerRejectsStreamingWithoutCallingUpstream(t *testing.T) {
	handler, _ := api.NewOpenAIHandler([]provider.ChatCompletionsProvider{failingProvider{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"broken/model","stream":true}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unsupported_streaming") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestOpenAIHandlerMatchesRoutingFieldsCaseSensitively(t *testing.T) {
	localCalled := false
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localCalled = true
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		const expected = `{"model":"a","Model":"cloud/b","Stream":true}`
		if string(body) != expected {
			t.Errorf("upstream body = %s, want %s", body, expected)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer localServer.Close()
	cloudServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("case-variant Model field routed request to cloud provider")
	}))
	defer cloudServer.Close()

	local, err := provider.NewOllama("local", localServer.URL, localServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	cloud, err := provider.NewOllama("cloud", cloudServer.URL, cloudServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewOpenAIHandler([]provider.ChatCompletionsProvider{local, cloud})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	requestBody := `{"model":"local/a","Model":"cloud/b","Stream":true}`
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody)))
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if !localCalled {
		t.Fatal("local provider was not called")
	}
}
