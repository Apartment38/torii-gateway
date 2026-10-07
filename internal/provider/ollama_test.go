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

	"github.com/Apartment38/torii-gateway/internal/provider"
)

func TestOllamaDiscoversNamespacedModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			http.NotFound(w, r)
			return
		}
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

func TestOllamaInferenceDeadlineAndClientIsolation(t *testing.T) {
	for _, timeout := range []time.Duration{0, 10 * time.Minute} {
		t.Run(timeout.String(), func(t *testing.T) {
			want := timeout
			if want == 0 {
				want = 5 * time.Minute
			}
			client := &http.Client{Timeout: timeout, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining > want || remaining < want-time.Second {
					t.Fatalf("inference deadline remaining = %v, present = %v, want %v", remaining, ok, want)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}
			ollama, err := provider.NewOllama("local", "http://ollama.test", client)
			if err != nil {
				t.Fatal(err)
			}
			response, err := ollama.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"cold","messages":[]}`), "cold")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if client.Timeout != timeout {
				t.Fatal("constructor changed the caller's shared client timeout")
			}
		})
	}
}

func TestOllamaTimeoutIncludesStreamAndPropagatesCancellation(t *testing.T) {
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 100 * time.Millisecond
	ollama, err := provider.NewOllama("local", server.URL, client)
	if err != nil {
		t.Fatal(err)
	}
	response, err := ollama.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"cold","stream":true}`), "cold")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if _, err := io.ReadAll(response.Body); err == nil {
		t.Fatal("stream continued beyond the configured inference deadline")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("inference timeout did not cancel the upstream stream")
	}
}

func TestOllamaInferenceTimeoutDoesNotShortenDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(150 * time.Millisecond):
			_, _ = io.WriteString(w, `{"data":[]}`)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 50 * time.Millisecond
	ollama, err := provider.NewOllama("local", server.URL, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ollama.Models(context.Background()); err != nil {
		t.Fatalf("inference timeout shortened model discovery: %v", err)
	}
}

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

func TestOllamaForwardsCompatibleEndpoints(t *testing.T) {
	for _, endpoint := range []string{"/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/responses"} {
		t.Run(endpoint, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != endpoint {
					t.Fatalf("path = %q, want %q", r.URL.Path, endpoint)
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
			response, err := ollama.Forward(context.Background(), endpoint, []byte(`{"model":"local/qwen3","messages":[]}`), "qwen3")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
		})
	}
}

func TestOllamaRejectsUnsupportedEndpoint(t *testing.T) {
	ollama, err := provider.NewOllama("local", "http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ollama.Forward(context.Background(), "/v1/files", []byte(`{"model":"local/qwen3"}`), "qwen3"); err == nil {
		t.Fatal("Forward() error = nil")
	}
}

func TestOllamaNativeDiscoveryPreservesUnknownAndCloudPolicy(t *testing.T) {
	for _, f := range []struct {
		name, show   string
		status       int
		known, local bool
	}{
		{"local", `{"capabilities":["completion","tools","vision","thinking"],"model_info":{"general.architecture":"qwen3","qwen3.context_length":32768}}`, 200, true, true},
		{"cloud", `{"capabilities":["completion","tools"],"remote_host":"https://ollama.com","remote_model":"qwen3-cloud","model_info":{}}`, 200, true, false},
		{"names_only", `{"model_info":{}}`, 200, false, false},
		{"failed_show", `{}`, 404, false, false},
		{"malformed_show", `{"capabilities":["completion"]} {}`, 200, false, false},
	} {
		t.Run(f.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body, status := `{"data":[{"id":"custom-name"}]}`, 200
				if r.URL.Path == "/api/show" {
					var payload map[string]string
					if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&payload) != nil || payload["model"] != "custom-name" {
						t.Error("invalid native show request")
					}
					body, status = f.show, f.status
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			o, err := provider.NewOllama("local", "http://ollama.test", client)
			if err != nil {
				t.Fatal(err)
			}
			models, err := o.Models(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			metadata := models[0].Discovery
			if (metadata != nil) != f.known {
				t.Fatalf("unknown capability promoted: %#v", metadata)
			}
			if f.known {
				if metadata.Local == nil || *metadata.Local != f.local || !metadata.Endpoints["chat_completions"] || !metadata.Capabilities["tools"] {
					t.Fatalf("wrong native facts: %#v", metadata)
				}
				if _, known := metadata.Endpoints["responses"]; known {
					t.Fatal("unverified Responses support advertised")
				}
				if _, known := metadata.Capabilities["structured_output"]; known {
					t.Fatal("unverified structured support advertised")
				}
				if f.local && (models[0].ContextWindow == nil || *models[0].ContextWindow != 32768) {
					t.Fatal("native context absent")
				}
			}
		})
	}
}
