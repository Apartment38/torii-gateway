package quota

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type accountFixture struct {
	calls  int
	fail   bool
	status Status
}

func (f *accountFixture) QuotaKey(string) string { return "provider/@account" }
func (f *accountFixture) Quota(context.Context, string) (Status, error) {
	f.calls++
	if f.fail {
		return Status{}, errors.New("secret must not escape")
	}
	return f.status, nil
}

func TestAccountCacheFailureAndSnapshotIsolation(t *testing.T) {
	f := &accountFixture{fail: true}
	c := NewCache(time.Minute, f)
	for _, id := range []string{"provider/a", "provider/b"} {
		s, err := c.Get(context.Background(), id)
		if err == nil || s.Known || s.Source != "unavailable" {
			t.Fatal(s, err)
		}
	}
	if f.calls != 1 {
		t.Fatal("failure refetched per model", f.calls)
	}
	f.fail = false
	reset := time.Now().Add(time.Hour)
	f.status = Status{Windows: []Window{{Name: "primary", Unit: "percent", Limit: 100, Used: 25, ResetAt: &reset}}}
	c.Invalidate("provider/a")
	s, err := c.Get(context.Background(), "provider/a")
	if err != nil {
		t.Fatal(err)
	}
	s.Windows[0].Used = 99
	*s.Windows[0].ResetAt = time.Time{}
	next, _ := c.Get(context.Background(), "provider/b")
	if next.Windows[0].Used != 25 || next.Windows[0].ResetAt.IsZero() {
		t.Fatal("caller mutated cache")
	}
}

func TestResetsExpireEarlierThanTTLAndLocalDoesNotMaskAccount(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	reset := now.Add(time.Second)
	f := &accountFixture{status: Status{Windows: []Window{{Name: "weekly", Unit: "percent", Limit: 100, Used: 100, ResetAt: &reset}}}}
	c := NewCache(time.Hour, f)
	c.now = func() time.Time { return now }
	c.SetLocal("provider/a", 10, 0)
	first, err := c.Get(context.Background(), "provider/a")
	if err != nil || !first.Exhausted || len(first.Windows) != 2 || !first.ExpiresAt.Equal(reset) {
		t.Fatal(first, err)
	}
	now = now.Add(2 * time.Second)
	f.status.Windows[0].Used = 5
	f.status.Windows[0].ResetAt = nil
	next, err := c.Get(context.Background(), "provider/b")
	if err != nil || next.Exhausted || f.calls != 2 {
		t.Fatal(next, err, f.calls)
	}
}

func TestHeadersPreserveUnitsAndSpendingCaps(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("X-RateLimit-Limit-Requests", "100")
	headers.Set("X-RateLimit-Remaining-Requests", "80")
	headers.Set("X-RateLimit-Reset-Requests", "30s")
	headers.Set("X-RateLimit-Limit-Tokens", "1000")
	headers.Set("X-RateLimit-Remaining-Tokens", "0")
	headers.Set("X-RateLimit-Reset-Tokens", "1893456060000")
	observed, ok := FromHeaders(headers, now)
	if !ok || len(observed.Windows) != 2 {
		t.Fatal(observed)
	}
	f := &accountFixture{status: Status{Known: true, Limit: 5, Used: 5, Unit: "USD", Source: "credits"}}
	c := NewCache(time.Minute, f)
	c.now = func() time.Time { return now }
	c.Get(context.Background(), "provider/a")
	if err := c.Observe("provider/b", observed); err != nil {
		t.Fatal(err)
	}
	s, _ := c.Get(context.Background(), "provider/a")
	if !s.Exhausted || len(s.Windows) != 3 || s.Windows[0].Remaining != 80 || s.Windows[1].Unit != "tokens" || s.Windows[2].Unit != "USD" {
		t.Fatal(s)
	}
	if !s.Windows[0].ResetAt.Equal(now.Add(30*time.Second)) || !s.Windows[1].ResetAt.Equal(now.Add(time.Minute)) {
		t.Fatal("reset parsing", s.Windows)
	}
	incomplete := http.Header{"X-Ratelimit-Limit-Requests": {"100"}, "X-Ratelimit-Remaining-Tokens": {"10"}}
	if _, ok := FromHeaders(incomplete, now); ok {
		t.Fatal("mixed request and token headers")
	}
}

func TestCodexHeaderWindowsAndUnknownReset(t *testing.T) {
	now := time.Now().UTC()
	h := http.Header{}
	h.Set("X-Codex-Primary-Used-Percent", "95")
	h.Set("X-Codex-Primary-Reset-After-Seconds", "10")
	h.Set("X-Codex-Primary-Window-Minutes", "300")
	s, ok := FromHeaders(h, now)
	if !ok || len(s.Windows) != 1 || s.Windows[0].Unit != "percent" || s.Windows[0].WindowSeconds != 18000 || !s.Windows[0].ResetAt.Equal(now.Add(10*time.Second)) {
		t.Fatal(s)
	}
	if ParseReset("not a reset", now) != nil {
		t.Fatal("invented reset")
	}
}

func TestOutOfRangeResetCannotBreakModelJSON(t *testing.T) {
	if ParseReset("999999999999", time.Now()) != nil {
		t.Fatal("accepted reset beyond RFC3339 year range")
	}
	h := http.Header{}
	h.Set("X-Codex-Primary-Used-Percent", "10")
	h.Set("X-Codex-Primary-Reset-At", "1893456000")
	s, ok := FromHeaders(h, time.Now())
	if !ok || s.Windows[0].ResetAt == nil || s.Windows[0].ResetAt.Unix() != 1893456000 {
		t.Fatal("official reset-at header", s)
	}
}
