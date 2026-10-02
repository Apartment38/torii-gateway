package intelligence

import (
	"context"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// All benchmark adapters share a single lane per upstream origin, including
// across handler reloads. The inference HTTP transport never uses these lanes.
var benchmarkRequestLanes sync.Map

type requestLane struct {
	permit         chan struct{}
	mu             sync.Mutex
	next, cooldown time.Time
	penalty        time.Duration
	successes      int
}

func laneFor(rawURL string) *requestLane {
	u, _ := url.Parse(rawURL)
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	origin := strings.ToLower(u.Hostname()) + ":" + port
	lane, _ := benchmarkRequestLanes.LoadOrStore(origin, &requestLane{permit: make(chan struct{}, 1)})
	return lane.(*requestLane)
}

func waitDuration(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *requestLane) acquire(ctx context.Context, interval time.Duration) (func(), error) {
	select {
	case l.permit <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-l.permit }
	for {
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		l.mu.Lock()
		now := time.Now()
		ready := l.next
		if l.cooldown.After(ready) {
			ready = l.cooldown
		}
		delay := ready.Sub(now)
		if delay <= 0 {
			l.next = now.Add(max(interval, l.penalty))
			l.mu.Unlock()
			return release, nil
		}
		l.mu.Unlock()
		if err := waitDuration(ctx, delay); err != nil {
			release()
			return nil, err
		}
	}
}

func (l *requestLane) pause(delay, interval time.Duration, throttled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	deadline := time.Now().Add(delay)
	if deadline.After(l.cooldown) {
		l.cooldown = deadline
	}
	if throttled {
		l.penalty = min(15*time.Second, max(interval, l.penalty)) * 2
		l.successes = 0
	}
}

func (l *requestLane) succeeded() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successes++
	if l.successes >= 10 {
		l.penalty /= 2
		l.successes = 0
	}
}

// Honor both standard Retry-After forms without the previous five-second cap.
// The source context bounds even an extremely long server-requested pause.
func retryAfter(raw string, now time.Time) (time.Duration, bool) {
	if seconds, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64); err == nil {
		const maxSeconds = uint64((1<<63 - 1) / int64(time.Second))
		return time.Duration(min(seconds, maxSeconds)) * time.Second, true
	}
	if date, err := http.ParseTime(raw); err == nil {
		return max(0, date.Sub(now)), true
	}
	return 0, false
}

func retryBackoff(attempt int, throttled bool) time.Duration {
	base, ceiling := 250*time.Millisecond, 5*time.Second
	if throttled {
		base, ceiling = 30*time.Second, 2*time.Minute
	}
	return min(ceiling, base*time.Duration(1<<min(attempt, 5))) + time.Duration(rand.Int64N(int64(250*time.Millisecond)))
}

type refreshActivityKey struct{}

func reportRateLimit(ctx context.Context, message string) {
	if report, ok := ctx.Value(refreshActivityKey{}).(func(string)); ok {
		report(message)
	}
}
