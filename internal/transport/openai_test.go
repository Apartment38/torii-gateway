package transport_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apartment38/torii-router/internal/transport"
)

func TestOpenAICompatibleRewritesModelAndPreservesUnknownFields(t *testing.T) {
	const requestBody = "{\n  \"model\" : \"ollama/qwen3\",\n  \"large_number\": 9007199254740993,\n  \"future_option\": {\"nested\":[1,true]}\n}\n"
	const expectedBody = "{\n  \"model\" : \"qwen3\",\n  \"large_number\": 9007199254740993,\n  \"future_option\": {\"nested\":[1,true]}\n}\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization was forwarded: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != expectedBody {
			t.Errorf("body = %q, want %q", body, expectedBody)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client, err := transport.NewOpenAICompatible(server.URL+"/prefix", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), "/v1/chat/completions", []byte(requestBody), "qwen3")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestOpenAICompatibleRejectsStreaming(t *testing.T) {
	client, err := transport.NewOpenAICompatible("http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), "/v1/chat/completions", []byte(`{"model":"x","stream":true}`), "x"); err == nil {
		t.Fatal("Do() error = nil, want streaming error")
	}
}

func TestOpenAICompatibleRejectsMalformedRequest(t *testing.T) {
	client, err := transport.NewOpenAICompatible("http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`[]`, `{`, `{} {`, `{}`, `{"model":null}`, `{"model":"one","model":"two"}`, `{"model":"one",}`, `{"model":"one","messages":[true false]}`} {
		if _, err := client.Do(context.Background(), "/v1/chat/completions", []byte(body), "x"); err == nil {
			t.Errorf("Do(%q) error = nil", body)
		}
	}
}

func TestOpenAICompatiblePreservesEscapedTrailingBasePathSlash(t *testing.T) {
	var escapedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client, err := transport.NewOpenAICompatible(server.URL+`/tenant%2F`, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), "/v1/chat/completions", []byte(`{"model":"local/a"}`), "a")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if escapedPath != "/tenant%2F/v1/chat/completions" {
		t.Fatalf("escaped path = %q", escapedPath)
	}
}

func TestInspectRequestDoesNotCopyUnrelatedFields(t *testing.T) {
	body := append([]byte(`{"model":"local/a","messages":"`), bytes.Repeat([]byte("x"), 1<<20)...)
	body = append(body, []byte(`"}`)...)
	allocations := testing.AllocsPerRun(10, func() {
		fields, err := transport.InspectRequest(body)
		if err != nil || fields.Model != "local/a" {
			t.Fatalf("fields = %#v, error = %v", fields, err)
		}
	})
	if allocations > 20 {
		t.Fatalf("allocations = %v, want constant low-allocation inspection", allocations)
	}
}

func TestOpenAICompatiblePreservesEscapedBasePath(t *testing.T) {
	var escapedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client, err := transport.NewOpenAICompatible(server.URL+`/tenants/acme%2Fprod`, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), "/v1/chat/completions", []byte(`{"model":"local/a"}`), "a")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if escapedPath != "/tenants/acme%2Fprod/v1/chat/completions" {
		t.Fatalf("escaped path = %q", escapedPath)
	}
}
