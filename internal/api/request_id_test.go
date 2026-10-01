package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/requestid"
)

func TestRequestIDMiddlewareAcceptsAndGeneratesIDs(t *testing.T) {
	for _, tt := range []struct {
		name, supplied, want string
	}{
		{name: "accept", supplied: "client-request_123", want: "client-request_123"},
		{name: "generate missing"},
		{name: "replace unsafe", supplied: "contains spaces"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var contextID string
			handler := api.RequestIDMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextID, _ = requestid.FromContext(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if tt.supplied != "" {
				request.Header.Set("X-Request-ID", tt.supplied)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			got := response.Header().Get("X-Request-ID")
			if tt.want != "" && got != tt.want {
				t.Fatalf("request ID = %q, want %q", got, tt.want)
			}
			if tt.want == "" && (!strings.HasPrefix(got, "req_") || len(got) < 20) {
				t.Fatalf("generated request ID = %q", got)
			}
			if contextID != got {
				t.Fatalf("context request ID = %q, response ID = %q", contextID, got)
			}
		})
	}
}

func TestRequestIDPropagatesUpstreamAndCannotBeOverridden(t *testing.T) {
	var upstreamID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamID = r.Header.Get("X-Request-ID")
		w.Header().Set("X-Request-ID", "upstream-id")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	ollama, err := provider.NewOllama("local", upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	openAI, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{ollama})
	handler := api.RequestIDMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), openAI)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"local/model"}`))
	request.Header.Set("X-Request-ID", "client-id")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if upstreamID != "client-id" {
		t.Fatalf("upstream request ID = %q", upstreamID)
	}
	if got := response.Header().Values("X-Request-ID"); len(got) != 1 || got[0] != "client-id" {
		t.Fatalf("response request IDs = %q", got)
	}
}

func TestRequestLogExcludesHeadersAndBodies(t *testing.T) {
	var output bytes.Buffer
	handler := api.RequestIDMiddleware(slog.New(slog.NewJSONHandler(&output, nil)), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"prompt":"body-secret"}`))
	request.Header.Set("Authorization", "Bearer token-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if strings.Contains(output.String(), "body-secret") || strings.Contains(output.String(), "token-secret") {
		t.Fatalf("log leaked request data: %s", output.String())
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["request_id"] == "" || record["status"] != float64(http.StatusBadRequest) {
		t.Fatalf("log record = %#v", record)
	}
}
