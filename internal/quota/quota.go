// Package quota normalizes and caches provider allowance state.
package quota

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

type Status struct {
	Known             bool
	Limit             float64
	Used              float64
	Remaining         float64
	FractionRemaining float64
	Exhausted         bool
	CheckedAt         time.Time
	ExpiresAt         time.Time
}
type Fetcher interface {
	Quota(context.Context, string) (Status, error)
}
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	fetcher Fetcher
	entries map[string]Status
	calls   map[string]*call
}
type call struct {
	done   chan struct{}
	status Status
	err    error
}

func NewCache(ttl time.Duration, fetcher Fetcher) *Cache {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &Cache{ttl: ttl, now: time.Now, fetcher: fetcher, entries: map[string]Status{}, calls: map[string]*call{}}
}
func (c *Cache) Get(ctx context.Context, model string) (Status, error) {
	now := c.now().UTC()
	c.mu.Lock()
	if value, ok := c.entries[model]; ok && now.Before(value.ExpiresAt) {
		c.mu.Unlock()
		return value, nil
	}
	if pending, ok := c.calls[model]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-pending.done:
			return pending.status, pending.err
		}
	}
	pending := &call{done: make(chan struct{})}
	c.calls[model] = pending
	c.mu.Unlock()
	var value Status
	var err error
	if c.fetcher == nil {
		value = Status{Known: false}
	} else {
		value, err = c.fetcher.Quota(ctx, model)
	}
	if err == nil {
		err = normalize(&value)
		value.CheckedAt = now
		value.ExpiresAt = now.Add(c.ttl)
	}
	c.mu.Lock()
	if err == nil {
		c.entries[model] = value
	}
	pending.status, pending.err = value, err
	delete(c.calls, model)
	close(pending.done)
	c.mu.Unlock()
	return value, err
}
func (c *Cache) SetLocal(model string, limit, used float64) error {
	value := Status{Known: true, Limit: limit, Used: used}
	if err := normalize(&value); err != nil {
		return err
	}
	now := c.now().UTC()
	value.CheckedAt = now
	value.ExpiresAt = now.Add(c.ttl)
	c.mu.Lock()
	c.entries[model] = value
	c.mu.Unlock()
	return nil
}
func (c *Cache) Invalidate(model string) { c.mu.Lock(); delete(c.entries, model); c.mu.Unlock() }
func normalize(s *Status) error {
	if !s.Known {
		s.Limit = 0
		s.Used = 0
		s.Remaining = 0
		s.FractionRemaining = 0
		s.Exhausted = false
		return nil
	}
	if math.IsNaN(s.Limit) || math.IsInf(s.Limit, 0) || s.Limit < 0 || math.IsNaN(s.Used) || math.IsInf(s.Used, 0) || s.Used < 0 {
		return errors.New("quota values must be finite and non-negative")
	}
	s.Remaining = math.Max(0, s.Limit-s.Used)
	if s.Limit == 0 {
		s.FractionRemaining = 0
		s.Exhausted = true
	} else {
		s.FractionRemaining = s.Remaining / s.Limit
		s.Exhausted = s.Remaining == 0
	}
	return nil
}
