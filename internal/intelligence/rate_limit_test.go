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
