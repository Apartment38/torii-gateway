package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

// Fixtures follow official Codex GetAccountRateLimitsResponse and backend usage
// schemas, inspected 2026-10-11; values and identities are synthetic.
func TestCodexProtocolBucketsAndWindowClassification(t *testing.T) {
	tests := []struct {
		name, body string
		groups     int
		types      []string
	}{
		{"legacy weekly primary", `{"rateLimits":{"primary":{"usedPercent":62,"windowDurationMins":10080,"resetsAt":1900000000},"planType":"plus"}}`, 1, []string{"weekly"}},
		{"map and mirrored legacy", `{"rateLimits":{"limitId":"codex","primary":{"usedPercent":99,"windowDurationMins":300}},"rateLimitsByLimitId":{"codex":{"limitId":"codex","primary":{"usedPercent":35,"windowDurationMins":300,"resetsAt":1900000000},"secondary":{"usedPercent":62,"windowDurationMins":10080,"resetsAt":1900100000}},"other":{"limitId":"other","limitName":"Model allowance","primary":{"usedPercent":100,"windowDurationMins":30,"resetsAt":1900200000}}}}`, 2, []string{"5h", "weekly", "custom:1800"}},
		{"duplicate provider ids preserve map keys", `{"rateLimitsByLimitId":{"a":{"limitId":"shared","primary":{"usedPercent":20}},"b":{"limitId":"shared","primary":{"usedPercent":40}}}}`, 2, []string{"unknown", "unknown"}},
		{"unnamed direct groups", `{"rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":604800}},"additional_rate_limits":[{"rate_limit":{"primary_window":{"used_percent":100,"limit_window_seconds":7200}}},{"limit_name":"Future pool","metered_feature":"future","rate_limit":{"secondary_window":{"used_percent":42}}}]}`, 3, []string{"weekly", "custom:7200", "unknown"}},
		{"incomplete window", `{"rateLimits":{"limitId":"codex","primary":{"windowDurationMins":300,"resetsAt":1900000000}}}`, 1, []string{"5h"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now().UTC()
			s := parseCodexUsage(fixturePayload(t, tt.body), now)
			if len(s.Limits) != tt.groups {
				t.Fatalf("groups=%d", len(s.Limits))
			}
			var types []string
			for _, g := range s.Limits {
				for _, w := range g.Windows {
					types = append(types, w.WindowType)
					if w.State == "available" && w.Remaining != 100-w.Used {
						t.Fatal("remaining percentage")
					}
				}
			}
			if strings.Join(types, ",") != strings.Join(tt.types, ",") {
				t.Fatalf("types=%v", types)
			}
			if tt.name == "incomplete window" && (s.Limits[0].Windows[0].State != "unavailable" || s.Limits[0].Windows[0].Exhausted) {
				t.Fatal("missing usage became zero/exhausted")
			}
			if tt.name == "map and mirrored legacy" && (s.Limits[0].Windows[0].ResetAt.Unix() != 1900000000 || s.Limits[0].Windows[1].ResetAt.Unix() != 1900100000 || s.Limits[1].Windows[0].ResetAt.Unix() != 1900200000) {
				t.Fatal("reset/group association lost")
			}
			if tt.name == "legacy weekly primary" && s.Limits[0].Scope != "unknown" {
				t.Fatal("missing limitId fabricated account scope")
			}
		})
	}
}

func TestCodexQuota401RefreshIsSharedWithInference(t *testing.T) {
	var mu sync.Mutex
	credential := CodexCredential{AccessToken: "old-secret", RefreshToken: "refresh-secret", AccountID: "account", ClientID: CodexOAuthClientID, ExpiresAt: time.Now().Add(time.Hour)}
	var refreshes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			refreshes.Add(1)
			r.ParseForm()
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-secret" {
				t.Error("wrong refresh grant")
			}
			io.WriteString(w, `{"access_token":"new-secret","refresh_token":"rotated-secret","expires_in":3600}`)
			return
		}
		if r.Header.Get("ChatGPT-Account-ID") != "account" {
			t.Error("wrong account")
		}
		if r.Header.Get("Authorization") == "Bearer old-secret" {
			w.WriteHeader(401)
			io.WriteString(w, "secret-upstream-body")
			return
		}
		if r.Header.Get("Authorization") != "Bearer new-secret" {
			t.Error("latest token not shared")
		}
		if strings.HasSuffix(r.URL.Path, "/responses") {
			io.WriteString(w, `{"id":"r","output":[]}`)
			return
		}
		io.WriteString(w, `{"account_id":"account","rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":18000}}}`)
	}))
	defer server.Close()
	credential.Endpoint = server.URL + "/backend-api/codex"
	c, _ := NewCodex("codex", credential.Endpoint, nil, func(string) (CodexCredential, error) { mu.Lock(); defer mu.Unlock(); return credential, nil }, server.Client())
	c.ConfigureRefresh(server.URL, func(_ string, v CodexCredential) error { mu.Lock(); defer mu.Unlock(); credential = v; return nil })
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := c.Quota(context.Background(), "codex/m")
			if err != nil || s.State != "available" {
				t.Errorf("quota state=%s err=%v", s.State, err)
			}
		}()
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("refreshes=%d", refreshes.Load())
	}
	mu.Lock()
	rotated := credential.RefreshToken
	mu.Unlock()
	if rotated != "rotated-secret" {
		t.Fatal("refresh rotation not persisted")
	}
	response, err := c.Forward(context.Background(), "/v1/responses", []byte(`{"input":"hello"}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestCodexQuotaFailuresDoNotStopInference(t *testing.T) {
	for _, code := range []int{401, 403, 429, 500, 404} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			v := CodexCredential{AccessToken: "old-secret", RefreshToken: "refresh-secret", AccountID: "account", ClientID: CodexOAuthClientID}
			var refreshes atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/token" {
					refreshes.Add(1)
					io.WriteString(w, `{"access_token":"new-secret","refresh_token":"rotated-secret","expires_in":3600}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/responses") {
					io.WriteString(w, `{"id":"r","output":[]}`)
					return
				}
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(code)
				io.WriteString(w, "secret-upstream-body")
			}))
			defer server.Close()
			v.Endpoint = server.URL + "/backend-api/codex"
			c, _ := NewCodex("codex", v.Endpoint, nil, func(string) (CodexCredential, error) { return v, nil }, server.Client())
			c.ConfigureRefresh(server.URL, func(_ string, next CodexCredential) error { v = next; return nil })
			s, err := c.Quota(context.Background(), "codex/m")
			if err != nil || s.State == "available" {
				t.Fatalf("failed collection: %s %v", s.State, err)
			}
			b, _ := json.Marshal(s)
			if strings.Contains(string(b), "secret") {
				t.Fatal("secret leaked")
			}
			if code == 401 && (s.State != "auth_expired" || refreshes.Load() != 1) {
				t.Fatal("401 retry not bounded")
			}
			if code == 403 && s.State != "permission_denied" || code == 429 && (s.State != "rate_limited" || s.RetryAt == nil) {
				t.Fatal("wrong operational state")
			}
			resp, err := c.Forward(context.Background(), "/v1/responses", []byte(`{"input":"hello"}`), "m")
			if err != nil || resp.StatusCode != 200 {
				t.Fatal("quota failure stopped inference")
			}
			resp.Body.Close()
		})
	}
}

func TestCodexQuotaCredentialContextFailures(t *testing.T) {
	token := "head." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"actual"}}`)) + ".sig"
	for _, tt := range []struct{ token, account, reason string }{{"sk-api-key", "account", "oauth_required_api_key_supplied"}, {token, "wrong", "account_context_mismatch"}} {
		c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
			return CodexCredential{AccessToken: tt.token, AccountID: tt.account, Endpoint: DefaultCodexBaseURL}, nil
		}, &http.Client{Transport: codexTestTransport(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid context sent to endpoint")
			return nil, nil
		})})
		s, _ := c.Quota(context.Background(), "m")
		if s.UnavailableReason != tt.reason {
			t.Fatalf("reason=%s", s.UnavailableReason)
		}
	}
}

func TestCodexRefreshFailureDoesNotOverwriteOrLeak(t *testing.T) {
	original := CodexCredential{AccessToken: "old-secret", RefreshToken: "refresh-secret", ExpiresAt: time.Now().Add(-time.Minute)}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, "refresh-secret old-secret")
	}))
	defer server.Close()
	original.Endpoint = server.URL
	c, _ := NewCodex("codex", server.URL, nil, func(string) (CodexCredential, error) { return original, nil }, server.Client())
	c.ConfigureRefresh(server.URL, func(string, CodexCredential) error { t.Fatal("failed refresh persisted"); return nil })
	_, err := c.credential(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error=%v", err)
	}
}

func TestCodexQuotaAccountSwitchDoesNotReuseCache(t *testing.T) {
	v := CodexCredential{AccessToken: "synthetic", AccountID: "first", Endpoint: DefaultCodexBaseURL}
	var calls int
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return v, nil }, &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))}, nil
	})})
	cache := quota.NewCache(time.Minute, c)
	first, _ := cache.Get(context.Background(), "codex/m")
	v.AccountID = "second"
	next, _ := cache.Get(context.Background(), "codex/m")
	if calls != 2 || first.AccountID == next.AccountID {
		t.Fatal("mixed account snapshots")
	}
}

func TestCodexWorkspaceSnapshotsDoNotMixUsers(t *testing.T) {
	v := CodexCredential{AccessToken: "first-login", AccountID: "workspace", Endpoint: DefaultCodexBaseURL}
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return v, nil }, nil)
	first := c.QuotaKey("m")
	v.AccessToken = "second-login"
	if first == c.QuotaKey("m") {
		t.Fatal("unknown workspace users shared cache")
	}
	v.Subject = "same-user"
	first = c.QuotaKey("m")
	v.AccessToken = "rotated"
	if first != c.QuotaKey("m") {
		t.Fatal("known user rotation lost shared account identity")
	}
}
func TestCodexReportedAccountMismatch(t *testing.T) {
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "synthetic", AccountID: "selected", Endpoint: DefaultCodexBaseURL}, nil
	}, &http.Client{Transport: codexTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"account_id":"other","rate_limit":{"primary_window":{"used_percent":20}}}`))}, nil
	})})
	s, _ := c.Quota(context.Background(), "m")
	if s.UnavailableReason != "account_context_mismatch" || len(s.Limits) > 0 {
		t.Fatal("attached another account's quota")
	}
}

func TestCodexJWTAccountHintDoesNotPrevent401Refresh(t *testing.T) {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user","https://api.openai.com/auth":{"chatgpt_account_id":"account"}}`))
	v := CodexCredential{AccessToken: "old." + claims + ".sig", RefreshToken: "synthetic-refresh", Endpoint: DefaultCodexBaseURL, ClientID: CodexOAuthClientID}
	var refreshed bool
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return v, nil }, &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"rate_limit":{"primary_window":{"used_percent":20}}}`
		status := 200
		if r.URL.Path == "/oauth/token" {
			refreshed = true
			body = `{"access_token":"new.` + claims + `.sig","expires_in":3600}`
		} else {
			if r.Header.Get("ChatGPT-Account-ID") != "account" {
				t.Error("missing account hint")
			}
			if !refreshed {
				status = 401
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	c.ConfigureRefresh("https://auth.openai.com", func(_ string, next CodexCredential) error { v = next; return nil })
	s, _ := c.Quota(context.Background(), "m")
	if !refreshed || s.State != "available" || s.AccountID != "account" {
		t.Fatalf("hint/refreshed context=%s %s", s.State, s.AccountID)
	}
}

func TestCodexRemainingPercentageClampsOverage(t *testing.T) {
	s := parseCodexUsage(fixturePayload(t, `{"rateLimits":{"limitId":"codex","primary":{"usedPercent":120,"windowDurationMins":300}}}`), time.Now())
	w := s.Limits[0].Windows[0]
	if w.Used != 120 || w.Remaining != 0 || !w.Exhausted || s.Limits[0].State != "exhausted" {
		t.Fatal("overage was discarded or remaining escaped bounds")
	}
}

func TestCodexAuthenticationContextChangesInvalidateQuotaKey(t *testing.T) {
	v := CodexCredential{AccessToken: "synthetic", AccountID: "account", Subject: "user", ClientID: "first", Scopes: []string{"a", "b"}, Endpoint: DefaultCodexBaseURL}
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return v, nil }, nil)
	first := c.QuotaKey("m")
	v.Scopes = []string{"b", "a"}
	if c.QuotaKey("m") != first {
		t.Fatal("scope order changed identity")
	}
	v.ClientID = "second"
	if c.QuotaKey("m") == first {
		t.Fatal("incompatible OAuth contexts shared cache")
	}
}

func TestCodexMapDoesNotInventScopeFromContradictoryIdentifiers(t *testing.T) {
	s := parseCodexUsage(fixturePayload(t, `{"rateLimits":{"limitId":"codex","primary":{"usedPercent":20}},"rateLimitsByLimitId":{"codex":{"limitId":"other","primary":{"usedPercent":100}}}}`), time.Now())
	// The map key conflicts with its reported ID; preserve it independently from legacy codex.
	if len(s.Limits) != 2 || s.Limits[0].Scope != "unknown" || s.Limits[0].LimitID != "other" || s.Limits[1].Scope != "account" || s.Limits[1].LimitID != "codex" {
		t.Fatal("contradictory identifiers became account scope")
	}
}

func TestCodexReportedExhaustionWithUnavailableWindows(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		raw := `{"rate_limit":{"allowed":` + strconv.FormatBool(allowed) + `,"limit_reached":` + strconv.FormatBool(!allowed) + `}}`
		c := quota.NewCache(time.Minute, staticCodexStatus{parseCodexUsage(fixturePayload(t, raw), time.Now())})
		s, _ := c.Get(context.Background(), "m")
		if s.State != "available" || s.Exhausted == allowed || s.Known || len(s.Limits) != 1 || len(s.Limits[0].Windows) != 0 {
			t.Fatal("explicit quota state fabricated percentages or became a collection failure")
		}
	}
}

type staticCodexStatus struct{ status quota.Status }

func (f staticCodexStatus) Quota(context.Context, string) (quota.Status, error) { return f.status, nil }

func TestCodexRefreshAcceptsJWTExpiryWithoutExpiresIn(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":` + strconv.FormatInt(exp, 10) + `}`))
	v := CodexCredential{AccessToken: "old", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Minute), Endpoint: DefaultCodexBaseURL}
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return v, nil }, &http.Client{Transport: codexTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"new.` + claims + `.sig","refresh_token":"rotated"}`))}, nil
	})})
	c.ConfigureRefresh("https://auth.openai.com", func(_ string, next CodexCredential) error { v = next; return nil })
	got, err := c.credential(context.Background())
	if err != nil || got.ExpiresAt.Unix() != exp || v.RefreshToken != "rotated" {
		t.Fatalf("optional expires_in handling: %v", err)
	}
}
func TestCodexAccountClaimSwitchChangesQuotaIdentity(t *testing.T) {
	token := func(account string) string {
		return "h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user","https://api.openai.com/auth":{"chatgpt_account_id":"`+account+`"}}`)) + ".s"
	}
	v := CodexCredential{AccessToken: token("first"), Subject: "user", Endpoint: DefaultCodexBaseURL}
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return v, nil }, nil)
	first := c.QuotaKey("m")
	v.AccessToken = token("second")
	if first == c.QuotaKey("m") {
		t.Fatal("missing saved account ID mixed workspaces")
	}
}
func TestCodexPreservesLongCustomDuration(t *testing.T) {
	s := parseCodexUsage(fixturePayload(t, `{"rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":63072000}}}`), time.Now())
	if s.Limits[0].Windows[0].WindowType != "custom:63072000" {
		t.Fatal("provider duration discarded")
	}
}
