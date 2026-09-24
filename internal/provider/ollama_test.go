package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestOllamaPreservesEscapedTrailingBasePathSlash(t *testing.T) {
	var escapedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	ollama, err := provider.NewOllama("local", server.URL+`/tenant%2F`, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ollama.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if escapedPath != "/tenant%2F/v1/models" {
		t.Fatalf("escaped path = %q", escapedPath)
	}
}

func TestOllamaRemovesRepeatedTrailingBasePathSlashes(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	ollama, err := provider.NewOllama("local", server.URL+`/root///`, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ollama.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/root/v1/models" {
		t.Fatalf("path = %q", path)
	}
}

func TestOllamaRejectsMissingOrNullModelData(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":null}`, `{"data":{}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			ollama, err := provider.NewOllama("local", server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ollama.Models(context.Background()); err == nil || !strings.Contains(err.Error(), "data must be an array") {
				t.Fatalf("Models() error = %v", err)
			}
		})
	}
}

func TestOllamaRejectsMalformedCatalogDocument(t *testing.T) {
	for _, body := range []string{`{"data":[]}garbage`, `{"data":[]} {"data":[]}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			ollama, err := provider.NewOllama("local", server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ollama.Models(context.Background()); err == nil {
				t.Fatal("Models() error = nil")
			}
		})
	}
}

func TestOllamaRejectsNonObjectCatalogEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[null,1,"qwen"]}`))
	}))
	defer server.Close()
	ollama, err := provider.NewOllama("local", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ollama.Models(context.Background()); err == nil || !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("Models() error = %v", err)
	}
}

func TestOllamaRejectsOversizedCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"padding":"`))
		chunk := bytes.Repeat([]byte("x"), 1<<20)
		for range 17 {
			_, _ = io.Copy(w, bytes.NewReader(chunk))
		}
	}))
	defer server.Close()
	ollama, err := provider.NewOllama("local", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ollama.Models(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Models() error = %v, want oversized response error", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestOllamaModelDiscoveryHasDeadline(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) > 11*time.Second || time.Until(deadline) < 8*time.Second {
			t.Fatalf("discovery deadline = %v, present = %v", deadline, ok)
		}
		return nil, context.Canceled
	})}
	ollama, err := provider.NewOllama("local", "http://example.test", client)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = ollama.Models(context.Background())
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
