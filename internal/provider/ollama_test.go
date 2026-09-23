package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAppendURLPathPreservesEscapedTrailingSlash(t *testing.T) {
	base, err := url.Parse("http://host/tenant%2F")
	if err != nil {
		t.Fatal(err)
	}
	got := appendURLPath(base, "/v1/models").String()
	if want := "http://host/tenant%2F/v1/models"; got != want {
		t.Fatalf("appendURLPath() = %q, want %q", got, want)
	}
}

func TestOllamaModelsBoundsDiscoveryRequest(t *testing.T) {
	base, _ := url.Parse("http://ollama.example")
	ollama := &Ollama{
		baseURL: base,
		client: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		})},
		discoveryTimeout: 10 * time.Millisecond,
	}
	_, err := ollama.Models(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Models() error = %v, want deadline exceeded", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestOllamaModelsRejectsMissingOrNullData(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":null}`, `{"data":{}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			ollama, err := NewOllama(base, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = ollama.Models(context.Background())
			if err == nil || !strings.Contains(err.Error(), "data must be an array") && body != `{"data":{}}` {
				t.Fatalf("Models() error = %v, want invalid data error", err)
			}
		})
	}
}

func TestOllamaModelsAcceptsDataArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen"}]}`))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	ollama, _ := NewOllama(base, server.Client())
	models, err := ollama.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("Models() returned %d models, want 1", len(models))
	}
}
