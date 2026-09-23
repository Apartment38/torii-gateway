package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apartment38/torii-router/internal/provider"
)

func TestOllamaDiscoversNamespacedModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen3","object":"model","created":123,"owned_by":"library"},{"id":"gemma3:4b"}]}`))
	}))
	defer server.Close()

	ollama, err := provider.NewOllama("ollama-local", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := ollama.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "ollama-local/qwen3" || models[1].ID != "ollama-local/gemma3:4b" {
		t.Fatalf("models = %#v", models)
	}
	if models[0].Created != 123 || models[0].OwnedBy != "library" || models[1].Object != "model" {
		t.Fatalf("metadata = %#v", models)
	}
}

func TestOllamaChatCompletionsUsesCompatibleTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "qwen3" {
			t.Errorf("model = %q", request.Model)
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1"}`))
	}))
	defer server.Close()
	ollama, err := provider.NewOllama("local", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := ollama.ChatCompletions(context.Background(), []byte(`{"model":"local/qwen3","messages":[]}`), "qwen3")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}
