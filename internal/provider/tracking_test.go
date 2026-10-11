package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

func fixturePayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTrackingProviderFixtures(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		kind, raw         string
		windows, balances int
		first             float64
	}{
		{"deepseek", `{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"12.50","granted_balance":"2","topped_up_balance":"10.50"},{"currency":"CNY","total_balance":"42"}]}`, 0, 2, 12.5},
		{"opencode-go", `{"usage":{"rolling":{"percent":1,"resetsAt":"2026-10-11T13:00:00Z"},"weekly":{"percent":25,"resetInSec":60},"monthly":{"percent":0}}}`, 3, 0, 99},
		{"opencode-zen", `{"data":{"remaining_credits":5,"currency":"USD"}}`, 0, 1, 5},
		{"standard-compute", `{"data":{"budget_usd":40,"used_usd":10,"remaining_usd":30,"renews_at":"2026-11-01T00:00:00Z"}}`, 1, 1, 30},
		{"zai", `{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"usage":1000,"currentValue":250,"nextResetTime":1791723600000},{"type":"TOKENS_LIMIT","unit":6,"percentage":30},{"type":"TIME_LIMIT","usage":50,"currentValue":10}]}}`, 3, 0, 750},
		{"kimi-coding", `{"usage":{"used":"20","limit":"100","resetTime":"2026-10-12T00:00:00Z"},"usages":{"limit_5h":{"used_ratio":0.25,"reset_time":"2026-10-11T13:00:00Z"}},"limits":[{"scope":"requests","detail":{"limit":20,"remaining":15,"resetTime":"2026-10-11T13:00:00Z"}}]}`, 3, 0, 75},
		{"minimax", `{"base_resp":{"status_code":0},"model_remains":[{"model_name":"general","current_interval_total_count":100,"current_interval_usage_count":40,"end_time":1791723600000,"current_weekly_remaining_percent":75,"weekly_end_time":"2026-10-12T00:00:00Z"}]}`, 2, 0, 60},
		{"xai", `{"total":{"val":"-1250"}}`, 0, 1, 12.5},
		{"anthropic", `{"five_hour":{"utilization":0.25,"resets_at":"2026-10-11T13:00:00Z"},"seven_day":{"utilization":10}}`, 2, 0, 99.75},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			p := parseTrackingPayload(tc.kind, fixturePayload(t, tc.raw), now)
			cache := quota.NewCache(time.Minute, fixedTrackingQuota{p})
			s, err := cache.Get(context.Background(), "p/m")
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Windows) != tc.windows || len(s.Balances) != tc.balances {
				t.Fatalf("shape %+v", s)
			}
			var first float64
			if tc.windows > 0 {
				first = s.Windows[0].Remaining
			} else {
				first = *s.Balances[0].Remaining
			}
			if first != tc.first {
				t.Fatalf("remaining %v want %v", first, tc.first)
			}
			if tc.windows == 0 && s.Known {
				t.Fatal("balance fabricated a quota denominator")
			}
		})
	}
}

type fixedTrackingQuota struct{ status quota.Status }

func (f fixedTrackingQuota) Quota(context.Context, string) (quota.Status, error) {
	return f.status, nil
}

func TestTrackingEndpointContracts(t *testing.T) {
	for _, tc := range []struct{ kind, path string }{{"deepseek", "/user/balance"}, {"opencode-go", "/zen/go/v1/usage"}, {"opencode-zen", "/zen/go/v1/usage"}, {"zai", "/api/monitor/usage/quota/limit"}, {"kimi-coding", "/coding/v1/usages"}, {"minimax", "/v1/token_plan/remains"}, {"anthropic", "/api/oauth/usage"}} {
		t.Run(tc.kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path || r.Header.Get("Authorization") != "Bearer billing-synthetic" {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				io.WriteString(w, `{}`)
			}))
			defer server.Close()
			base, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "p", BaseURL: server.URL, StaticModels: []string{"m"}})
			p := WithTracking(base, TrackingConfig{Provider: tc.kind, BaseURL: server.URL, Mode: "subscription", Client: server.Client(), Headers: http.Header{"Authorization": {"Bearer billing-synthetic"}}})
			if _, err := p.(quota.Fetcher).Quota(context.Background(), "p/m"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNousBalancesPreserveTopupsRolloverAndRenewal(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/account" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		io.WriteString(w, `{"subscription":{"plan":"pro","monthly_credits":20,"credits_remaining":0,"current_period_end":"2026-11-01T00:00:00Z"},"paid_service_access":{"subscription_credits_remaining":0,"purchased_credits_remaining":5,"total_usable_credits":5}}`)
	}))
	defer server.Close()
	p, err := NewNous("nous", server.URL+"/v1", server.URL, "hermes-agent", []string{"m"}, false, func(string) (NousCredential, error) {
		return NousCredential{AccessToken: "synthetic", Endpoint: server.URL + "/v1"}, nil
	}, func(string, NousCredential) error { return nil }, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	s, err := quota.NewCache(time.Minute, p).Get(context.Background(), "nous/m")
	if err != nil || s.Exhausted || s.Plan != "pro" || len(s.Balances) != 3 || *s.Balances[0].Remaining != 5 || s.Windows[0].ResetAt == nil {
		t.Fatalf("status %+v err %v", s, err)
	}
	rollover := parseNousAccount(fixturePayload(t, `{"subscription":{"monthly_credits":20,"credits_remaining":25},"purchased_credits_remaining":0}`), time.Now())
	if len(rollover.Windows) != 0 || *rollover.Balances[0].Remaining != 25 {
		t.Fatalf("rollover %+v", rollover)
	}
}

func TestCodexPublicPathCreditsAndScopedLimits(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/codex/usage" || r.Header.Get("ChatGPT-Account-ID") != "synthetic-account" {
			t.Errorf("request %s", r.URL.Path)
		}
		io.WriteString(w, `{"plan_type":"pro","credits":{"has_credits":true,"balance":"8.75"},"rate_limit_reset_credits":{"available_count":2},"rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":18000,"reset_at":1893456000}},"additional_rate_limits":[{"limit_name":"spark","rate_limit":{"primary_window":{"used_percent":100}}}]}`)
	}))
	defer server.Close()
	p, _ := NewCodex("codex", server.URL, []string{"main", "spark"}, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "synthetic", AccountID: "synthetic-account", Endpoint: server.URL}, nil
	}, server.Client())
	cache := quota.NewCache(time.Minute, p)
	main, err := cache.Get(context.Background(), "codex/main")
	if err != nil || main.Exhausted || main.Plan != "pro" || main.ResetCredits == nil || *main.ResetCredits != 2 || *main.Balances[0].Remaining != 8.75 {
		t.Fatalf("main %+v %v", main, err)
	}
	spark, err := cache.Get(context.Background(), "codex/spark")
	if err != nil || !spark.Exhausted {
		t.Fatalf("spark %+v %v", spark, err)
	}
}

func TestTrackingRejectsRedirectsAndMalformedData(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	base, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "p", BaseURL: server.URL, StaticModels: []string{"m"}})
	p := WithTracking(base, TrackingConfig{Provider: "deepseek", BaseURL: server.URL, Client: server.Client(), Headers: http.Header{"Authorization": {"Bearer secret-synthetic"}}})
	s, err := p.(quota.Fetcher).Quota(context.Background(), "p/m")
	if err != nil || leaked || s.Known || s.UnavailableReason == "" {
		t.Fatalf("redirect handling %+v %v", s, err)
	}
	for _, raw := range []string{`{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"NaN"}]}`, `{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":true}]}`} {
		s := parseTrackingPayload("deepseek", fixturePayload(t, raw), time.Now())
		if len(s.Balances) != 0 {
			t.Fatal("invalid balance accepted")
		}
	}
	s = parseTrackingPayload("opencode-go", fixturePayload(t, `{"usage":{"weekly":{"percent":35},"monthly":{"percent":0}}}`), time.Now())
	if len(s.Windows) != 2 {
		t.Fatal("missing rolling window hid healthy weekly/monthly data")
	}
}

func TestAnthropicAdminReportsPaginationAndPartialFailure(t *testing.T) {
	for _, failUsage := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "partial"}[failUsage], func(t *testing.T) {
			stamp := time.Now().UTC().Format("2006-01-02") + "T00:00:00Z"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Key") != "admin-synthetic" || r.Header.Get("Authorization") != "" {
					t.Error("incorrect admin credential")
				}
				if r.URL.Query().Get("bucket_width") != "1d" {
					t.Error("incorrect bucket")
				}
				if strings.HasSuffix(r.URL.Path, "cost_report") {
					if r.URL.Query().Get("page") == "next" {
						io.WriteString(w, `{"data":[{"starting_at":"`+stamp+`","results":[{"amount":"25","currency":"USD"}]}],"has_more":false}`)
					} else {
						io.WriteString(w, `{"data":[{"starting_at":"`+stamp+`","results":[{"amount":"125","currency":"USD"}]}],"has_more":true,"next_page":"next"}`)
					}
				} else if failUsage {
					w.WriteHeader(403)
				} else {
					io.WriteString(w, `{"data":[{"starting_at":"`+stamp+`","results":[{"uncached_input_tokens":7,"cache_read_input_tokens":2,"cache_creation":{"ephemeral_5m_input_tokens":3},"output_tokens":4}]}],"has_more":false}`)
				}
			}))
			defer server.Close()
			base, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "p", BaseURL: server.URL, StaticModels: []string{"m"}})
			p := WithTracking(base, TrackingConfig{Provider: "anthropic", BaseURL: server.URL, Client: server.Client(), Headers: http.Header{"X-Api-Key": {"admin-synthetic"}}})
			s, err := p.(quota.Fetcher).Quota(context.Background(), "p/m")
			if err != nil || s.Usage == nil || *s.Usage.Monthly != 1.5 || s.Partial != failUsage {
				t.Fatalf("reports %+v %v", s, err)
			}
			if !failUsage {
				found := false
				for _, u := range s.ReportedUsage {
					if u.Unit == "tokens" && u.Period == "monthly" {
						found = true
						if u.Used != 16 || u.Scope != "organization" {
							t.Fatalf("token report %+v", u)
						}
					}
				}
				if !found {
					t.Fatal("missing token report")
				}
			}
		})
	}
}

func TestTrackingPricesUseProviderRatesAndCustomAliases(t *testing.T) {
	base, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "custom", BaseURL: "https://example.com", StaticModels: []string{"claude-sonnet-4-6"}, ModelRewrites: map[string]string{"friendly": "claude-sonnet-4-6"}})
	p := WithTracking(base, TrackingConfig{Provider: "anthropic"})
	models, err := p.Models(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "custom/friendly" || models[0].InputPerMillion == nil || *models[0].InputPerMillion != 3 || models[0].PricingSource == "" {
		t.Fatalf("prices %+v %v", models, err)
	}
	m := models[0]
	m.ID = "custom/claude-sonnet-4-6"
	native := 5.0
	m.InputPerMillion, m.OutputPerMillion = &native, &native
	applyTrackingPrice("anthropic", &m)
	if *m.InputPerMillion != 5 {
		t.Fatal("fallback replaced native price")
	}
	sub := WithTracking(base, TrackingConfig{Provider: "anthropic", Mode: "subscription"})
	models, _ = sub.Models(context.Background())
	if models[0].Subscription || *models[0].InputPerMillion != 3 {
		t.Fatal("OAuth tracking changed API inference pricing")
	}
}

func TestAnthropicReportAmountsAvoidDuplicateAndInventedUsage(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		cost    bool
		want    float64
		invalid bool
	}{
		{`{"input_tokens":12,"uncached_input_tokens":7,"cache_read_input_tokens":2,"cache_creation":{"ephemeral_5m_input_tokens":3},"output_tokens":4}`, false, 16, false},
		{`{"input_tokens":12,"output_tokens":4}`, false, 16, false},
		{`{"amount":"NaN","currency":"USD"}`, true, 0, true},
		{`{"unrelated_field":10}`, false, 0, true},
		{`{"input_tokens":false}`, false, 0, true},
		{`{"input_tokens":0,"output_tokens":0}`, false, 0, false},
	} {
		got, err := anthropicReportAmount(fixturePayload(t, tc.raw), tc.cost)
		if (err != nil) != tc.invalid || got != tc.want {
			t.Fatalf("%s: got %v, %v", tc.raw, got, err)
		}
	}
}

func TestZAIWebToolAllowanceDoesNotBlockChat(t *testing.T) {
	s := parseTrackingPayload("zai", fixturePayload(t, `{"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"usage":100,"currentValue":25},{"type":"TIME_LIMIT","usage":100,"currentValue":100}]}}`), time.Now())
	c := quota.NewCache(time.Minute, nil)
	if err := c.Set("zai/glm", s); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(context.Background(), "zai/glm")
	if err != nil || !got.Known || got.Remaining != 75 || got.Exhausted || len(got.Windows) != 2 || !got.Windows[1].Informational {
		t.Fatalf("web tools gated chat: %+v %v", got, err)
	}
}
