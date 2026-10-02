package intelligence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryAfterSecondsDatesAndLongPauses(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, item := range []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"120", 2 * time.Minute, true}, {now.Add(2 * time.Minute).Format(http.TimeFormat), 2 * time.Minute, true}, {now.Add(-time.Second).Format(http.TimeFormat), 0, true}, {"0", 0, true}, {"bad", 0, false}, {"-1", 0, false}, {"9999999999999999999", time.Duration((1<<63-1)/int64(time.Second)) * time.Second, true},
	} {
		got, ok := retryAfter(item.raw, now)
		if ok != item.ok || got != item.want {
			t.Fatalf("Retry-After %q=%v,%v", item.raw, got, ok)
		}
	}
	if delay := retryBackoff(0, true); delay < 30*time.Second {
		t.Fatal("429 without headers uses aggressive retry")
	}
}

func TestSharedLaneSpacesRequestsAndSerializesAcrossAdapters(t *testing.T) {
	var mu sync.Mutex
	var starts []time.Time
	var concurrent, maxConcurrent atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := concurrent.Add(1)
		defer concurrent.Add(-1)
		for old := maxConcurrent.Load(); current > old && !maxConcurrent.CompareAndSwap(old, current); old = maxConcurrent.Load() {
		}
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	first, _ := newRemote(SourceSettings{ID: "hf", URL: server.URL}, "", "", FetchOptions{RequestInterval: 25 * time.Millisecond})
	second, _ := newRemote(SourceSettings{ID: "arena", URL: server.URL}, "", "", FetchOptions{RequestInterval: 25 * time.Millisecond})
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := first
			if i%2 == 1 {
				client = second
			}
			if _, err := client.get(context.Background(), server.URL, "Authorization"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if maxConcurrent.Load() != 1 || len(starts) != 5 {
		t.Fatal("adapters did not share their request lane")
	}
	for i := 1; i < len(starts); i++ {
		if starts[i].Sub(starts[i-1]) < 20*time.Millisecond {
			t.Fatal("burst requests bypassed pacing")
		}
	}
}

func Test429CooldownAppliesToOtherSourcesAndHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		io.WriteString(w, "secret upstream response")
	}))
	defer server.Close()
	first, _ := newRemote(SourceSettings{ID: "first", URL: server.URL}, "", "", FetchOptions{RequestInterval: time.Millisecond})
	if _, err := first.get(context.Background(), server.URL, "Authorization"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe throttle error")
	}
	lane := laneFor(server.URL)
	lane.mu.Lock()
	remaining := time.Until(lane.cooldown)
	lane.mu.Unlock()
	if remaining < 119*time.Second {
		t.Fatal("Retry-After was capped or not shared")
	}
	second, _ := newRemote(SourceSettings{ID: "second", URL: server.URL}, "", "", FetchOptions{RequestInterval: time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := second.get(ctx, server.URL, "Authorization"); err == nil || time.Since(start) > time.Second {
		t.Fatal("cooldown ignored cancellation")
	}
	// A cancelled waiter must not leak its lane permit.
	lane.mu.Lock()
	lane.cooldown = time.Time{}
	lane.next = time.Time{}
	lane.mu.Unlock()
	release, err := lane.acquire(context.Background(), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestThrottledRequestRetriesAfterServerPause(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	remote, _ := newRemote(SourceSettings{ID: "retry", URL: server.URL}, "", "", FetchOptions{RequestInterval: 20 * time.Millisecond, Retries: 1})
	var activity string
	ctx := context.WithValue(context.Background(), refreshActivityKey{}, func(message string) { activity = message })
	data, err := remote.get(ctx, server.URL, "Authorization")
	if err != nil || string(data) != `{"ok":true}` || calls.Load() != 2 || !strings.Contains(activity, "rate limit") {
		t.Fatalf("retry failed: %s %v", data, err)
	}
}

func TestLaneCancellationWhileAnotherRequestOwnsPermit(t *testing.T) {
	lane := &requestLane{permit: make(chan struct{}, 1)}
	release, err := lane.acquire(context.Background(), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lane.acquire(ctx, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}

func TestQuotaWindowParsingAndProactivePacing(t *testing.T) {
	for _, item := range []struct {
		header string
		left   uint64
		reset  time.Duration
		valid  bool
	}{
		{`"api";r=0;t=120`, 0, 120 * time.Second, true},
		{`"api";r=10;t=60`, 10, time.Minute, true},
		{`"api";r=10;t=60, "pages";r=0;t=120`, 0, 120 * time.Second, true},
		{`"api";r=0;t=0`, 0, 0, true},
		{`"api";r=10`, 0, 0, false},
		{`"api";r=-1;t=10`, 0, 0, false},
		{`"api";r=bad;t=secret`, 0, 0, false},
	} {
		h := http.Header{"Ratelimit": []string{item.header}}
		remaining, reset, valid := quotaWindow(h)
		if remaining != item.left || reset != item.reset || valid != item.valid {
			t.Fatalf("header %q => %d, %v, %v", item.header, remaining, reset, valid)
		}
	}
	lane := &requestLane{permit: make(chan struct{}, 1)}
	lane.observeQuota(http.Header{"Ratelimit": []string{`"api";r=10;t=60`}})
	if time.Until(lane.next) < 5*time.Second {
		t.Fatal("successful responses did not pace the remaining budget")
	}
	lane.observeQuota(http.Header{"Ratelimit": []string{`"api";r=0;t=120`}})
	if time.Until(lane.cooldown) < 119*time.Second {
		t.Fatal("quota reset did not pause the shared lane")
	}
}

func Test429UsesRateLimitResetWhenRetryAfterIsAbsent(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("RateLimit", `"api";r=0;t=120`)
		w.WriteHeader(429)
	}))
	defer server.Close()
	remote, _ := newRemote(SourceSettings{ID: "quota", URL: server.URL}, "", "", FetchOptions{RequestInterval: time.Millisecond, Retries: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var activity string
	ctx = context.WithValue(ctx, refreshActivityKey{}, func(message string) { activity = message })
	if _, err := remote.get(ctx, server.URL, "Authorization"); err == nil {
		t.Fatal("quota wait ignored the source deadline")
	}
	if calls.Load() != 1 || !strings.Contains(activity, "120.0 seconds") {
		t.Fatalf("retried before reset: %d %q", calls.Load(), activity)
	}
	if time.Until(laneFor(server.URL).cooldown) < 119*time.Second {
		t.Fatal("reset header was not shared between sources")
	}
}

func TestRepeated429BackoffIsSharedBetweenPagesAndSources(t *testing.T) {
	lane := &requestLane{permit: make(chan struct{}, 1)}
	first := lane.throttleBackoff(0)
	lane.pause(first, time.Second, true)
	second := lane.throttleBackoff(0)
	lane.pause(second, time.Second, true)
	third := lane.throttleBackoff(0)
	if first < 30*time.Second || second < time.Minute || third < 2*time.Minute {
		t.Fatal("fresh pages reset the origin's throttle backoff")
	}
	for range 10 {
		lane.pause(time.Second, time.Second, true)
	}
	if lane.penalty != time.Minute || lane.throttleBackoff(0) < 5*time.Minute {
		t.Fatal("sustained throttling did not slow the shared origin")
	}
}
