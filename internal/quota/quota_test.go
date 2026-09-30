package quota_test

import (
	"context"
	"github.com/Apartment38/torii-router/internal/quota"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fetcher struct {
	calls   atomic.Int64
	release chan struct{}
}

func (f *fetcher) Quota(ctx context.Context, _ string) (quota.Status, error) {
	f.calls.Add(1)
	if f.release != nil {
		select {
		case <-ctx.Done():
			return quota.Status{}, ctx.Err()
		case <-f.release:
		}
	}
	return quota.Status{Known: true, Limit: 100, Used: 25}, nil
}
func TestCacheNormalizesAndExpires(t *testing.T) {
	f := &fetcher{}
	c := quota.NewCache(time.Hour, f)
	first, err := c.Get(context.Background(), "model")
	if err != nil {
		t.Fatal(err)
	}
	if first.Remaining != 75 || first.FractionRemaining != .75 || first.Exhausted {
		t.Fatalf("status=%#v", first)
	}
	_, _ = c.Get(context.Background(), "model")
	if f.calls.Load() != 1 {
		t.Fatalf("calls=%d", f.calls.Load())
	}
	c.Invalidate("model")
	_, _ = c.Get(context.Background(), "model")
	if f.calls.Load() != 2 {
		t.Fatalf("calls=%d", f.calls.Load())
	}
}
func TestCacheCoalescesConcurrentRefresh(t *testing.T) {
	f := &fetcher{release: make(chan struct{})}
	c := quota.NewCache(time.Minute, f)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), "same"); err != nil {
				t.Error(err)
			}
		}()
	}
	for f.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(f.release)
	wg.Wait()
	if f.calls.Load() != 1 {
		t.Fatalf("calls=%d", f.calls.Load())
	}
}
func TestLocalAllowanceAndUnknownSemantics(t *testing.T) {
	c := quota.NewCache(time.Minute, nil)
	unknown, err := c.Get(context.Background(), "unknown")
	if err != nil || unknown.Known || unknown.Exhausted {
		t.Fatalf("unknown=%#v,%v", unknown, err)
	}
	if err := c.SetLocal("local", 10, 10); err != nil {
		t.Fatal(err)
	}
	local, err := c.Get(context.Background(), "local")
	if err != nil || !local.Exhausted || local.Remaining != 0 {
		t.Fatalf("local=%#v,%v", local, err)
	}
}
