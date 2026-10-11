package quota

import (
	"context"
	"testing"
	"time"
)

type codexCacheFetcher struct {
	calls  int
	status Status
}

func (f *codexCacheFetcher) Quota(context.Context, string) (Status, error) {
	f.calls++
	return clone(f.status), nil
}
func (f *codexCacheFetcher) QuotaKey(string) string { return "account" }
func TestCodexCacheStaleBackoffRecoveryAndIsolation(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	reset := now.Add(10 * time.Second)
	f := &codexCacheFetcher{status: Status{Provider: "openai-codex", AccountID: "a", State: "available", Source: "codex_usage_endpoint", CollectedAt: now, Windows: []Window{{Name: "primary", Limit: 100, Used: 35, Unit: "percent", ResetAt: &reset}}, Limits: []LimitGroup{{LimitID: "codex", Scope: "account", Windows: []Window{{Name: "primary", Limit: 100, Used: 35, Unit: "percent"}}}}}}
	c := NewCache(time.Minute, f)
	c.now = func() time.Time { return now }
	first, _ := c.Get(context.Background(), "p/m")
	if first.Remaining != 65 {
		t.Fatal("normalization")
	}
	first.Limits[0].Windows[0].Used = 99
	copy, _ := c.Get(context.Background(), "p/n")
	if copy.Limits[0].Windows[0].Used != 35 {
		t.Fatal("shared snapshot mutated")
	}
	now = now.Add(11 * time.Second)
	retry := now.Add(2 * time.Minute)
	f.status = Status{Provider: "openai-codex", AccountID: "a", State: "rate_limited", UnavailableReason: "rate_limited", RetryAt: &retry}
	stale, _ := c.Get(context.Background(), "p/m")
	if stale.State != "stale" || stale.Remaining != 65 || !stale.CollectedAt.Equal(first.CollectedAt) || stale.Exhausted || !stale.ExpiresAt.Equal(retry) {
		t.Fatalf("lost good evidence: %+v", stale)
	}
	if c.RequestRefresh("p/m") {
		t.Fatal("manual refresh bypassed retry-after")
	}
	now = now.Add(121 * time.Second)
	f.status = Status{Provider: "openai-codex", AccountID: "a", State: "available", Source: "codex_usage_endpoint", CollectedAt: now, Windows: []Window{{Name: "primary", Limit: 100, Used: 40, Unit: "percent"}}}
	recovered, _ := c.Get(context.Background(), "p/m")
	if recovered.State != "available" || recovered.Remaining != 60 || f.calls != 3 {
		t.Fatal("recovery")
	}
	if c.RequestRefresh("p/m") {
		t.Fatal("manual refresh not throttled")
	}
	now = now.Add(11 * time.Second)
	if !c.RequestRefresh("p/m") {
		t.Fatal("manual refresh rejected after cooldown")
	}
}
func TestCodexCacheDoesNotMixResponseHeaders(t *testing.T) {
	c := NewCache(time.Minute, nil)
	s := Status{Provider: "openai-codex", Source: "codex_usage_endpoint", Known: true, Limit: 100, Used: 35, Limits: []LimitGroup{{LimitID: "codex"}}}
	c.Set("p/m", s)
	c.Observe("p/m", Status{Source: "response_headers", Windows: []Window{{Name: "primary", Unit: "percent", Limit: 100, Used: 80}}})
	got, _ := c.Get(context.Background(), "p/m")
	if got.Source != "codex_usage_endpoint" || got.Used != 35 {
		t.Fatal("mixed sources")
	}
}
