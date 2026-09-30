package provider_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-router/internal/provider"
)

func TestCompatibleProviderDiscoveryAuthenticationAndRewrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-secret" || r.Header.Get("X-Tenant") != "tenant-a" {
			t.Errorf("provider headers = %#v", r.Header)
		}
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"vendor/model","created":42}]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"upstream-model"`) || !strings.Contains(string(body), `"future":{"kept":true}`) {
			t.Errorf("body = %s", body)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	p, err := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "cloud", BaseURL: server.URL, Client: server.Client(), Headers: http.Header{"Authorization": {"Bearer provider-secret"}, "X-Tenant": {"tenant-a"}}, DiscoverModels: true, ModelRewrites: map[string]string{"friendly": "upstream-model", "catalog-name": "vendor/model"}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "cloud/catalog-name" || models[0].Created != 42 {
		t.Fatalf("models = %#v", models)
	}
	resp, err := p.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"cloud/friendly","future":{"kept":true}}`), "friendly")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestCompatibleProviderStaticModelsWithoutDiscovery(t *testing.T) {
	p, err := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "local", BaseURL: "http://example.test", DiscoverModels: false, StaticModels: []string{"nested/model"}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "local/nested/model" {
		t.Fatalf("models = %#v", models)
	}
}

func TestCompatibleProviderStreamsWithoutBuffering(t *testing.T) {
	flushed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		close(flushed)
		_, _ = io.WriteString(w, "data: second\n\n")
	}))
	defer server.Close()
	p, err := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "cloud", BaseURL: server.URL, Client: server.Client(), StaticModels: []string{"m"}})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Forward(context.Background(), "/v1/responses", []byte(`{"model":"cloud/m","stream":true}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-flushed
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Fatalf("first line = %q, %v", line, err)
	}
}

func TestCompatibleProviderCredentialsAreIsolated(t *testing.T) {
	seen := make(chan string, 2)
	newServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen <- r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `{"ok":true}`)
		}))
	}
	a, b := newServer(), newServer()
	defer a.Close()
	defer b.Close()
	pa, _ := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "a", BaseURL: a.URL, Client: a.Client(), Headers: http.Header{"Authorization": {"Bearer a"}}, StaticModels: []string{"m"}})
	pb, _ := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "b", BaseURL: b.URL, Client: b.Client(), Headers: http.Header{"Authorization": {"Bearer b"}}, StaticModels: []string{"m"}})
	for _, p := range []*provider.Compatible{pa, pb} {
		resp, err := p.Forward(context.Background(), "/v1/embeddings", []byte(`{"model":"x"}`), "m")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if got1, got2 := <-seen, <-seen; got1 != "Bearer a" || got2 != "Bearer b" {
		t.Fatalf("credentials = %q, %q", got1, got2)
	}
}
