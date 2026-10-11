package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/state"
)

type codexLifecycleTransport func(*http.Request) (*http.Response, error)

func (f codexLifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCodexFactorySharesEncryptedRotationAcrossHandlerInstances(t *testing.T) {
	db := runtimeStateDB(t)
	v := &state.Credential{AccessToken: "expired-synthetic", RefreshToken: "refresh-synthetic", ClientID: provider.CodexOAuthClientID, AccountID: "account", Endpoint: provider.DefaultCodexBaseURL, ExpiresAt: time.Now().Add(-time.Minute)}
	if err := db.Save("codex", v); err != nil {
		t.Fatal(err)
	}
	var refreshes atomic.Int32
	previous := http.DefaultTransport
	http.DefaultTransport = codexLifecycleTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"rate_limit":{"primary_window":{"used_percent":35,"limit_window_seconds":18000},"secondary_window":{"used_percent":62,"limit_window_seconds":604800}},"additional_rate_limits":[{"metered_feature":"extra","limit_name":"Extra allowance","rate_limit":{"primary_window":{"used_percent":42,"limit_window_seconds":18000}}}]}`
		if r.URL.Host == "auth.openai.com" {
			refreshes.Add(1)
			body = `{"access_token":"rotated-synthetic","refresh_token":"next-refresh-synthetic","expires_in":3600}`
		} else {
			if r.Header.Get("Authorization") != "Bearer rotated-synthetic" || r.Header.Get("ChatGPT-Account-ID") != "account" {
				t.Error("inference and polling credential paths disagree")
			}
			if strings.HasSuffix(r.URL.Path, "/responses") {
				body = `{"id":"r","output":[]}`
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	cfg := config.ProviderConfig{ID: "codex", Type: "openai-codex", BaseURL: provider.DefaultCodexBaseURL}
	first, err := buildProviderWithState(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildProviderWithState(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				s, err := first.(quota.Fetcher).Quota(context.Background(), "codex/m")
				if err != nil || len(s.Limits) != 2 {
					t.Error("missing quota buckets")
				}
			} else {
				resp, err := second.Forward(context.Background(), "/v1/responses", []byte(`{"input":"test"}`), "m")
				if err != nil {
					t.Error(err)
				} else {
					resp.Body.Close()
				}
			}
		}()
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("competing handler refreshes=%d", refreshes.Load())
	}
	stored, err := db.Load("codex")
	if err != nil || stored.AccessToken != "rotated-synthetic" || stored.RefreshToken != "next-refresh-synthetic" {
		t.Fatal("encrypted rotation not persisted")
	}
}
