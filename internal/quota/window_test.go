package quota

import (
	"context"
	"errors"
	"testing"
	"time"
)

type windowFetcher struct {
	calls int
	err   error
	reset time.Time
}

func (f *windowFetcher) Quota(context.Context, string) (Status, error) {
	f.calls++
	return Status{Known: true, Limit: 100, Used: 100, Windows: []Window{{Name: "primary", Unit: "percent", Limit: 100, Used: 100, ResetAt: &f.reset}}}, f.err
}
func TestWindowResetBoundsCacheExpiry(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	f := &windowFetcher{reset: now.Add(time.Second)}
	c := NewCache(time.Hour, f)
	c.now = func() time.Time { return now }
	s, err := c.Get(context.Background(), "model")
	if err != nil || !s.ExpiresAt.Equal(f.reset) {
		t.Fatalf("snapshot expiry %+v %v", s, err)
	}
	now = now.Add(time.Second)
	_, _ = c.Get(context.Background(), "model")
	if f.calls != 2 {
		t.Fatal("stale exhausted quota survived reset")
	}
}
func TestQuotaFailuresAreBoundedAndInvalidate(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	f := &windowFetcher{err: errors.New("unavailable")}
	c := NewCache(time.Minute, f)
	c.now = func() time.Time { return now }
	for range 3 {
		if _, err := c.Get(context.Background(), "provider"); err == nil {
			t.Fatal("failure became known quota")
		}
	}
	if f.calls != 1 {
		t.Fatal("failed quota polling repeated for every model")
	}
	c.Invalidate("provider")
	_, _ = c.Get(context.Background(), "provider")
	if f.calls != 2 {
		t.Fatal("invalidation did not clear failure")
	}
	now = now.Add(16 * time.Second)
	_, _ = c.Get(context.Background(), "provider")
	if f.calls != 3 {
		t.Fatal("failed quota did not retry")
	}
}
