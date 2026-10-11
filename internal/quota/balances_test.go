package quota

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestObservedHeadersPreserveIndependentBalancesAndCopy(t *testing.T) {
	left, resets := 12.5, 2.0
	c := NewCache(time.Minute, nil)
	s := Status{Balances: []Balance{{Name: "account", Unit: "USD", Remaining: &left}}, ResetCredits: &resets, ReportedUsage: []ReportedUsage{{Period: "monthly", Unit: "tokens", Used: 50}}}
	if err := c.Set("p/m", s); err != nil {
		t.Fatal(err)
	}
	got, _ := c.Get(context.Background(), "p/m")
	*got.Balances[0].Remaining = 0
	*got.ResetCredits = 0
	got.ReportedUsage[0].Used = 100
	observed, _ := FromHeaders(http.Header{"X-Ratelimit-Limit-Requests": {"100"}, "X-Ratelimit-Remaining-Requests": {"42"}}, time.Now())
	if err := c.Observe("p/m", observed); err != nil {
		t.Fatal(err)
	}
	got, _ = c.Get(context.Background(), "p/m")
	if *got.Balances[0].Remaining != 12.5 || *got.ResetCredits != 2 || got.ReportedUsage[0].Used != 50 || got.Remaining != 42 {
		t.Fatalf("merged snapshot %+v", got)
	}
}

func TestProviderNativeHeaders(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	s, ok := FromHeaders(http.Header{"Anthropic-Ratelimit-Tokens-Limit": {"1000"}, "Anthropic-Ratelimit-Tokens-Remaining": {"800"}, "Anthropic-Ratelimit-Tokens-Reset": {"2026-10-11T13:00:00Z"}}, now)
	if !ok || len(s.Windows) != 1 || s.Windows[0].Used != 200 || s.Windows[0].ResetAt == nil {
		t.Fatalf("Anthropic %+v", s)
	}
	s, ok = FromHeaders(http.Header{"X-Sc-Budget-Renews-At": {"2026-11-01T00:00:00Z"}}, now)
	if !ok || s.Known || s.ResetAt == nil {
		t.Fatalf("reset-only header %+v", s)
	}
}

func TestScopedQuotaDoesNotMatchProviderName(t *testing.T) {
	c := NewCache(time.Minute, nil)
	if err := c.Set("sonnet-provider/claude-opus", Status{Windows: []Window{{Name: "weekly_sonnet", Model: "sonnet", Unit: "percent", Limit: 100, Used: 100, Informational: true}}}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(context.Background(), "sonnet-provider/claude-opus")
	if err != nil || got.Known || got.Exhausted || !got.Windows[0].Informational {
		t.Fatalf("unrelated model inherited a provider-name match: %+v %v", got, err)
	}
}
