package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
)

func TestBuildOllamaProviderUsesRequestTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeout string
		wantErr bool
	}{
		{"default permits loading", "", false},
		{"short override cancels loading", "50ms", true},
		{"longer override permits loading", "1s", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-time.After(150 * time.Millisecond):
					_, _ = io.WriteString(w, `{"choices":[]}`)
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			p, err := buildProvider(config.ProviderConfig{ID: "local", Type: "ollama", BaseURL: server.URL, RequestTimeout: tc.timeout})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			response, err := p.Forward(ctx, "/v1/chat/completions", []byte(`{"model":"cold","messages":[]}`), "cold")
			if response != nil {
				response.Body.Close()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("Forward() error = %v, want error = %v", err, tc.wantErr)
			}
		})
	}
}
