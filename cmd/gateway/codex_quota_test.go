package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestCodexQuotaManagementRefreshBoundaryAndPublicPrivacy(t *testing.T) {
	r, db := credentialRuntime(t)
	if err := db.Save("codex", &state.Credential{AccessToken: "access-synthetic", AccountID: "account", Subject: "user", Email: "synthetic@example.test", Endpoint: provider.DefaultCodexBaseURL}); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = codexLifecycleTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":35,"limit_window_seconds":18000},"secondary_window":{"used_percent":62,"limit_window_seconds":604800}},"additional_rate_limits":[{"metered_feature":"extra","limit_name":"Extra","rate_limit":{"primary_window":{"used_percent":42,"limit_window_seconds":18000}}}]}`
		if strings.HasSuffix(req.URL.Path, "/models") {
			body = `{"models":[{"slug":"m","visibility":"list"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	cfg := &config.Config{Credentials: db, Server: config.ServerConfig{Listen: "127.0.0.1:8080"}, Providers: []config.ProviderConfig{{ID: "codex", Type: "openai-codex", BaseURL: provider.DefaultCodexBaseURL}}}
	handler, err := buildHandlerWithRuntime(cfg, "database", r)
	if err != nil {
		t.Fatal(err)
	}
	r.swap(handler)
	request := func(method, path string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if admin {
			req.SetBasicAuth("chosen-admin", "administrator-password")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := request("POST", "/gateway/v1/quotas/refresh", false); w.Code != 401 {
		t.Fatalf("unauthorized refresh=%d", w.Code)
	}
	w := request("POST", "/gateway/v1/quotas/refresh", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"limit_id":"extra"`) || !strings.Contains(w.Body.String(), "synthetic@example.test") {
		t.Fatalf("management snapshot=%d %s", w.Code, w.Body.String())
	}
	if w := request("POST", "/gateway/v1/quotas/refresh", true); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("manual refresh not rate limited")
	}
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer admin-sentinel")
	public := httptest.NewRecorder()
	r.ServeHTTP(public, req)
	if public.Code != 200 || strings.Contains(public.Body.String(), "synthetic@example.test") || strings.Contains(public.Body.String(), "access-synthetic") {
		t.Fatalf("public metadata privacy: HTTP %d", public.Code)
	}
}
