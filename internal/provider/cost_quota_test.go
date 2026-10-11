package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

func TestLivePricingUpdatesStaticAliasesAndKeepsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"id":"vendor/paid","context_length":32768,"pricing":{"prompt":"0.000002","completion":0.000004}},{"id":"unknown","pricing":{"prompt":"NaN","completion":"-1"}},{"id":"included","billing_mode":"subscription","pricing":{"prompt":"0.1"}}]}`)
	}))
	defer server.Close()
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "or", BaseURL: server.URL, DiscoverModels: true, StaticModels: []string{"vendor/paid"}, ModelRewrites: map[string]string{"alias": "vendor/paid"}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 || models[0].ID != "or/alias" || *models[0].InputPerMillion != 2 || *models[0].OutputPerMillion != 4 || *models[0].ContextWindow != 32768 {
		t.Fatalf("metadata=%+v", models)
	}
	if models[1].InputPerMillion != nil || models[1].OutputPerMillion != nil {
		t.Fatal("invalid price treated as known")
	}
	if !models[2].Subscription || *models[2].InputPerMillion != 0 || *models[2].OutputPerMillion != 0 {
		t.Fatal("subscription cost")
	}
}

func TestNousLivePricingUpdatesConfiguredModels(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"id":"paid","pricing":{"prompt":0.000001,"completion":"0.000003"}},{"id":"subscription","billing_mode":"subscription"}]}`)
	}))
	defer server.Close()
	p, err := NewNous("nous", server.URL, server.URL, "test", []string{"paid"}, true, func(string) (NousCredential, error) {
		return NousCredential{Endpoint: server.URL, AccessToken: "synthetic"}, nil
	}, func(string, NousCredential) error { return nil }, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || *models[0].InputPerMillion != 1 || *models[0].OutputPerMillion != 3 || !models[1].Subscription {
		t.Fatalf("models=%+v", models)
	}
}

func TestOpenRouterKeyAndCreditCredentialsAndWindows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/key":
			if r.Header.Get("Authorization") != "Bearer inference" {
				t.Error("key credential")
			}
			io.WriteString(w, `{"data":{"limit":10,"limit_remaining":3,"limit_reset":"monthly","usage":45,"usage_daily":2,"usage_monthly":7,"effective_budget":{"limit_usd":20,"remaining_usd":2,"resets_at":"2030-01-02T00:00:00Z"}}}`)
		case "/credits":
			if r.Header.Get("Authorization") != "Bearer management" {
				t.Error("credit credential")
			}
			io.WriteString(w, `{"data":{"total_credits":100,"total_usage":90}}`)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
		}
	}))
	defer server.Close()
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "or", BaseURL: server.URL, StaticModels: []string{"m"}, KeyQuotaURL: server.URL + "/key", Headers: http.Header{"Authorization": {"Bearer inference"}}, QuotaURL: server.URL + "/credits", QuotaHeaders: http.Header{"Authorization": {"Bearer management"}}})
	if err != nil {
		t.Fatal(err)
	}
	cache := quota.NewCache(time.Minute, p)
	s, err := cache.Get(context.Background(), "or/m")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Known || s.Remaining != 2 || len(s.Windows) != 3 || s.Usage == nil || *s.Usage.Total != 45 || s.Windows[0].ResetAt == nil || s.Unit != "USD" {
		t.Fatalf("snapshot=%+v", s)
	}
	if !s.ResetAt.Equal(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("wrong constraining reset")
	}
}

func TestOpenRouterUnlimitedKeyHasUsageWithoutInventedQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"limit":null,"limit_remaining":null,"usage":9}}`)
	}))
	defer server.Close()
	p, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "or", BaseURL: server.URL, StaticModels: []string{"m"}, KeyQuotaURL: server.URL + "/key"})
	s, err := quota.NewCache(time.Minute, p).Get(context.Background(), "or/m")
	if err != nil || s.Known || s.Exhausted || s.Usage == nil || *s.Usage.Total != 9 {
		t.Fatalf("snapshot=%+v err=%v", s, err)
	}
}

func TestCodexQuotaAccountWindowsAndReset(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" || r.Header.Get("ChatGPT-Account-ID") != "account" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Errorf("path/auth: %s", r.URL.Path)
		}
		io.WriteString(w, `{"rate_limit":{"allowed":false,"limit_reached":true,"primary_window":{"used_percent":50,"limit_window_seconds":18000,"reset_at":1893456000},"secondary_window":{"used_percent":100,"limit_window_seconds":604800,"reset_at":1893542400}}}`)
	}))
	defer server.Close()
	endpoint := server.URL + "/backend-api/codex"
	p, _ := NewCodex("codex", endpoint, []string{"m"}, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "synthetic", AccountID: "account", Endpoint: endpoint}, nil
	}, server.Client())
	s, err := quota.NewCache(time.Minute, p).Get(context.Background(), "codex/m")
	if err != nil || !s.Known || !s.Exhausted || s.Remaining != 0 || len(s.Windows) != 2 || s.Windows[0].Remaining != 50 || s.Windows[1].WindowSeconds != 604800 || s.ResetAt == nil {
		t.Fatalf("snapshot=%+v err=%v", s, err)
	}
}

func TestOpenRouterResetIntervals(t *testing.T) {
	now := time.Date(2026, 10, 10, 23, 45, 0, 0, time.UTC)
	for interval, want := range map[string]string{"daily": "2026-10-11", "weekly": "2026-10-12", "monthly": "2026-11-01"} {
		got := openRouterReset(interval, now)
		if got == nil || !strings.HasPrefix(got.Format(time.RFC3339), want) {
			t.Fatalf("%s=%v", interval, got)
		}
	}
	if openRouterReset("", now) != nil {
		t.Fatal("invented reset")
	}
}

func TestFreeRequestQuotaDoesNotExhaustPaidModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"limit":null,"limit_remaining":null,"usage":0,"free_model_daily_requests":{"limit":50,"remaining":0}}}`)
	}))
	defer server.Close()
	p, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "or", BaseURL: server.URL, StaticModels: []string{"m"}, KeyQuotaURL: server.URL + "/key"})
	c := quota.NewCache(time.Minute, p)
	free, err := c.Get(context.Background(), "or/vendor/m:free")
	if err != nil || !free.Exhausted || len(free.Windows) != 1 || free.Unit != "requests" || free.ResetAt == nil {
		t.Fatal(free, err)
	}
	paid, err := c.Get(context.Background(), "or/vendor/m")
	if err != nil || paid.Exhausted || paid.Known {
		t.Fatal("free cap applied to paid model", paid, err)
	}
}
