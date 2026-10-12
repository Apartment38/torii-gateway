// Package quota normalizes and caches provider allowance state.
package quota

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

type Status struct {
	Provider          string          `json:"provider,omitempty"`
	AccountID         string          `json:"account_id,omitempty"`
	AccountIdentity   string          `json:"account_identity,omitempty"`
	State             string          `json:"status,omitempty"`
	CollectionState   string          `json:"collection_status,omitempty"`
	CollectedAt       time.Time       `json:"collected_at,omitempty"`
	RetryAt           *time.Time      `json:"retry_at,omitempty"`
	Limits            []LimitGroup    `json:"limits,omitempty"`
	Known             bool            `json:"known"`
	Limit             float64         `json:"limit"`
	Used              float64         `json:"used"`
	Remaining         float64         `json:"remaining"`
	FractionRemaining float64         `json:"fraction_remaining"`
	Exhausted         bool            `json:"exhausted"`
	Blocked           bool            `json:"blocked,omitempty"`
	CheckedAt         time.Time       `json:"checked_at"`
	ExpiresAt         time.Time       `json:"expires_at"`
	Unit              string          `json:"unit,omitempty"`
	Source            string          `json:"source,omitempty"`
	ResetAt           *time.Time      `json:"reset_at,omitempty"`
	Windows           []Window        `json:"windows,omitempty"`
	Usage             *Spend          `json:"usage_usd,omitempty"`
	Balances          []Balance       `json:"balances,omitempty"`
	ReportedUsage     []ReportedUsage `json:"reported_usage,omitempty"`
	Plan              string          `json:"plan,omitempty"`
	ResetCredits      *float64        `json:"reset_credits,omitempty"`
	UnavailableReason string          `json:"unavailable_reason,omitempty"`
	Partial           bool            `json:"partial,omitempty"`
}

// LimitGroup keeps provider quota pools independent of their time windows.
// Scope stays unknown unless the source establishes an account/model association.
type LimitGroup struct {
	LimitID   string         `json:"limit_id,omitempty"`
	MapKey    string         `json:"map_key,omitempty"`
	LimitName string         `json:"limit_name,omitempty"`
	Scope     string         `json:"scope"`
	Model     string         `json:"model,omitempty"`
	State     string         `json:"status"`
	Windows   []Window       `json:"windows"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func WindowType(seconds int64) string {
	switch seconds {
	case 18000:
		return "5h"
	case 604800:
		return "weekly"
	case 0:
		return "unknown"
	}
	return fmt.Sprintf("custom:%d", seconds)
}

// Balance preserves the provider's native denomination. A remaining balance
// without a reported original allowance must never become a percentage quota.
type Balance struct {
	Name      string     `json:"name"`
	Unit      string     `json:"unit"`
	Remaining *float64   `json:"remaining,omitempty"`
	Granted   *float64   `json:"granted,omitempty"`
	Purchased *float64   `json:"purchased,omitempty"`
	Unlimited bool       `json:"unlimited,omitempty"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

// ReportedUsage is authoritative provider usage, separate from gateway estimates.
type ReportedUsage struct {
	Period string  `json:"period"`
	Unit   string  `json:"unit"`
	Used   float64 `json:"used"`
	Scope  string  `json:"scope,omitempty"`
}

// Spend is upstream-reported API-key spend, separate from gateway estimates.
type Spend struct {
	Total   *float64 `json:"total,omitempty"`
	Daily   *float64 `json:"daily,omitempty"`
	Weekly  *float64 `json:"weekly,omitempty"`
	Monthly *float64 `json:"monthly,omitempty"`
}

// Window preserves independent constraints (for example five-hour and weekly
// subscription usage). Percentages are not token or dollar balances.
type Window struct {
	WindowType        string     `json:"window_type,omitempty"`
	Slot              string     `json:"slot,omitempty"`
	State             string     `json:"status,omitempty"`
	Source            string     `json:"source,omitempty"`
	LastUpdated       time.Time  `json:"last_updated,omitempty"`
	Name              string     `json:"name"`
	Unit              string     `json:"unit"`
	Limit             float64    `json:"limit"`
	Used              float64    `json:"used"`
	Remaining         float64    `json:"remaining"`
	FractionRemaining float64    `json:"fraction_remaining"`
	Exhausted         bool       `json:"exhausted"`
	ResetAt           *time.Time `json:"reset_at,omitempty"`
	WindowSeconds     int64      `json:"window_seconds,omitempty"`
	Model             string     `json:"model,omitempty"`
	Informational     bool       `json:"informational,omitempty"`
}

// Keyer lets account-scoped fetchers share a snapshot across their models.
type Keyer interface{ QuotaKey(string) string }
type Fetcher interface {
	Quota(context.Context, string) (Status, error)
}
type Cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	now      func() time.Time
	fetcher  Fetcher
	entries  map[string]Status
	locals   map[string]Status
	errors   map[string]error
	calls    map[string]*call
	failures map[string]int
	attempts map[string]time.Time
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
	return &Cache{ttl: ttl, now: time.Now, fetcher: fetcher, entries: map[string]Status{}, locals: map[string]Status{}, errors: map[string]error{}, calls: map[string]*call{}, failures: map[string]int{}, attempts: map[string]time.Time{}}
}

func (c *Cache) QuotaKey(model string) string {
	if keyer, ok := c.fetcher.(Keyer); ok {
		return keyer.QuotaKey(model)
	}
	return model
}
func (c *Cache) Get(ctx context.Context, model string) (Status, error) {
	status, err := c.get(ctx, model)
	upstream := model
	if _, suffix, ok := strings.Cut(model, "/"); ok {
		upstream = suffix
	}
	for i := range status.Windows {
		w := &status.Windows[i]
		if w.Model != "" {
			w.Informational = !strings.Contains(strings.ToLower(upstream), strings.ToLower(w.Model))
		}
	}
	if err == nil {
		err = normalize(&status)
	}
	c.mu.Lock()
	local, exists := c.locals[model]
	c.mu.Unlock()
	if exists && c.now().Before(local.ExpiresAt) {
		if status.Known && len(status.Windows) == 0 {
			status.Windows = []Window{{Name: status.Source, Unit: status.Unit, Limit: status.Limit, Used: status.Used, ResetAt: status.ResetAt}}
		}
		status.Windows = append(status.Windows, Window{Name: "local_allowance", Unit: "USD", Limit: local.Limit, Used: local.Used})
		if local.ExpiresAt.Before(status.ExpiresAt) {
			status.ExpiresAt = local.ExpiresAt
		}
		if err != nil {
			status.Source = "local_allowance;provider_unavailable"
			err = nil
		}
		if normalizeErr := normalize(&status); normalizeErr != nil {
			return Status{}, normalizeErr
		}
	}
	return status, err
}

func (c *Cache) get(ctx context.Context, model string) (Status, error) {
	requestedModel := model
	now := c.now().UTC()
	c.mu.Lock()
	if value, ok := c.entries[model]; ok && now.Before(value.ExpiresAt) {
		err := c.errors[model]
		c.mu.Unlock()
		return clone(value), err
	}
	if keyer, ok := c.fetcher.(Keyer); ok {
		model = keyer.QuotaKey(model)
	}
	if value, ok := c.entries[model]; ok && now.Before(value.ExpiresAt) {
		err := c.errors[model]
		c.mu.Unlock()
		return clone(value), err
	}
	if pending, ok := c.calls[model]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-pending.done:
			return clone(pending.status), pending.err
		}
	}
	previous := clone(c.entries[model])
	c.attempts[model] = now
	pending := &call{done: make(chan struct{})}
	c.calls[model] = pending
	c.mu.Unlock()
	var value Status
	var err error
	if c.fetcher == nil {
		value = Status{Known: false}
	} else {
		value, err = c.fetcher.Quota(ctx, requestedModel)
	}
	if err == nil {
		err = normalize(&value)
		if value.State != "stale" {
			value.CheckedAt = now
		}
		value.ExpiresAt = now.Add(c.ttl)
		// Recheck at a reset rather than serving exhausted state for a whole TTL.
		for _, window := range value.Windows {
			if window.ResetAt != nil && window.ResetAt.After(now) && window.ResetAt.Before(value.ExpiresAt) {
				value.ExpiresAt = *window.ResetAt
			}
		}
		if value.ResetAt != nil && value.ResetAt.After(now) && value.ResetAt.Before(value.ExpiresAt) {
			value.ExpiresAt = *value.ResetAt
		}
		for _, b := range value.Balances {
			if b.ResetAt != nil && b.ResetAt.After(now) && b.ResetAt.Before(value.ExpiresAt) {
				value.ExpiresAt = *b.ResetAt
			}
		}
	}
	failed := err != nil || value.Provider == "openai-codex" && value.UnavailableReason != "" && value.State != "available"
	c.mu.Lock()
	if failed {
		c.failures[model]++
		baseDelay := min(c.ttl, 15*time.Second)
		if value.Provider == "openai-codex" {
			baseDelay = c.ttl
		}
		delay := baseDelay
		if value.Provider == "openai-codex" {
			delay = min(baseDelay*time.Duration(1<<min(c.failures[model]-1, 5)), 30*time.Minute)
		}
		retry := now.Add(delay)
		if value.State == "stale" && value.RetryAt != nil {
			retry = *value.RetryAt
		}
		if value.RetryAt != nil && value.RetryAt.After(retry) {
			retry = *value.RetryAt
		}
		if previous.Provider == "openai-codex" && previous.AccountID == value.AccountID && !previous.CollectedAt.IsZero() {
			previous.State = "stale"
			previous.CollectionState = value.CollectionState
			if previous.CollectionState == "" {
				previous.CollectionState = value.State
			}
			previous.UnavailableReason = value.UnavailableReason
			previous.RetryAt = &retry
			value = previous
		} else if err != nil {
			value = Status{Source: "unavailable", State: "unavailable", CheckedAt: now}
		}
		value.ExpiresAt = retry
	} else {
		delete(c.failures, model)
	}
	c.entries[model] = clone(value)
	c.errors[model] = err
	pending.status, pending.err = value, err
	delete(c.calls, model)
	close(pending.done)
	c.mu.Unlock()
	return clone(value), err
}
func (c *Cache) SetLocal(model string, limit, used float64) error {
	value := Status{Known: true, Limit: limit, Used: used, Unit: "USD", Source: "local_allowance"}
	if err := normalize(&value); err != nil {
		return err
	}
	value.CheckedAt = c.now().UTC()
	value.ExpiresAt = value.CheckedAt.Add(c.ttl)
	c.mu.Lock()
	c.locals[model] = value
	c.mu.Unlock()
	return nil
}

// Set records an observed model-scoped snapshot; configured local allowances
// remain independent of shared provider-account snapshots.
func (c *Cache) SetSnapshot(model string, value Status) error {
	return c.Set(model, value)
}

func (c *Cache) Set(model string, value Status) error {
	return c.set(model, value, time.Time{})
}

func (c *Cache) set(model string, value Status, expiresBefore time.Time) error {
	if err := normalize(&value); err != nil {
		return err
	}
	now := c.now().UTC()
	value.CheckedAt = now
	value.ExpiresAt = now.Add(c.ttl)
	if value.ResetAt != nil && value.ResetAt.After(now) && value.ResetAt.Before(value.ExpiresAt) {
		value.ExpiresAt = *value.ResetAt
	}
	for _, window := range value.Windows {
		if window.ResetAt != nil && window.ResetAt.After(now) && window.ResetAt.Before(value.ExpiresAt) {
			value.ExpiresAt = *window.ResetAt
		}
	}
	for _, b := range value.Balances {
		if b.ResetAt != nil && b.ResetAt.After(now) && b.ResetAt.Before(value.ExpiresAt) {
			value.ExpiresAt = *b.ResetAt
		}
	}
	if !expiresBefore.IsZero() && expiresBefore.Before(value.ExpiresAt) {
		value.ExpiresAt = expiresBefore
	}
	c.mu.Lock()
	c.entries[model] = clone(value)
	delete(c.errors, model)
	c.mu.Unlock()
	return nil
}

// Observe merges fresh header windows with a cached provider balance, so a
// rate-limit update cannot erase an independently exhausted spending cap.
func (c *Cache) Observe(model string, observed Status) error {
	if keyer, ok := c.fetcher.(Keyer); ok {
		model = keyer.QuotaKey(model)
	}
	c.mu.Lock()
	previous, ok := c.entries[model]
	c.mu.Unlock()
	var expiresBefore time.Time
	if ok && previous.Provider == "openai-codex" && len(previous.Limits) > 0 {
		return nil
	}
	if ok && previous.Source != "local_allowance" {
		// Header freshness must not extend independently collected billing data.
		// Keep expired data only in an expired entry, so the next Get refetches it.
		if len(previous.Balances) > 0 || previous.Usage != nil || len(previous.ReportedUsage) > 0 || previous.Plan != "" || previous.ResetCredits != nil || previous.UnavailableReason != "" || (previous.Source != "" && previous.Source != "response_headers") {
			expiresBefore = previous.ExpiresAt
		}
		if previous.Known && len(previous.Windows) == 0 {
			previous.Windows = []Window{{Name: previous.Source, Unit: previous.Unit, Limit: previous.Limit, Used: previous.Used, ResetAt: previous.ResetAt}}
		}
		for _, old := range previous.Windows {
			replaced := false
			for _, newWindow := range observed.Windows {
				if newWindow.Name == old.Name && newWindow.Unit == old.Unit && newWindow.Model == old.Model {
					replaced = true
					break
				}
			}
			if !replaced {
				observed.Windows = append(observed.Windows, old)
				expiresBefore = previous.ExpiresAt
			}
		}
		observed.Usage = previous.Usage
		if len(observed.Balances) == 0 {
			observed.Balances = previous.Balances
		}
		observed.ReportedUsage = previous.ReportedUsage
		observed.Plan = previous.Plan
		observed.ResetCredits = previous.ResetCredits
		observed.Partial = previous.Partial
		observed.UnavailableReason = previous.UnavailableReason
	}
	return c.set(model, observed, expiresBefore)
}
func (c *Cache) Invalidate(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, model)
	delete(c.locals, model)
	delete(c.errors, model)
	delete(c.failures, model)
	delete(c.attempts, model)
	if keyer, ok := c.fetcher.(Keyer); ok {
		key := keyer.QuotaKey(model)
		delete(c.entries, key)
		delete(c.errors, key)
		delete(c.failures, key)
		delete(c.attempts, key)
	}
}

// AuthenticationChanged allows a new OAuth login to retry an authentication
// failure immediately, without clearing balances, local allowances or 429 backoff.
func (c *Cache) AuthenticationChanged(model string) {
	if k, ok := c.fetcher.(Keyer); ok {
		model = k.QuotaKey(model)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[model]
	if !ok {
		return
	}
	state := v.CollectionState
	if state == "" {
		state = v.State
	}
	if state != "auth_expired" && state != "permission_denied" {
		return
	}
	v.ExpiresAt = c.now().UTC()
	v.RetryAt = nil
	v.UnavailableReason = ""
	v.CollectionState = "unavailable"
	c.entries[model] = v
	delete(c.errors, model)
	delete(c.failures, model)
	delete(c.attempts, model)
}

// RequestRefresh permits a user refresh after ten seconds, while honoring provider
// backoff. It preserves the last successful data and concurrent request deduplication.
func (c *Cache) RequestRefresh(model string) bool {
	if k, ok := c.fetcher.(Keyer); ok {
		model = k.QuotaKey(model)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now().UTC()
	v, ok := c.entries[model]
	if !ok {
		return true
	}
	if now.Sub(c.attempts[model]) < 10*time.Second || v.RetryAt != nil && now.Before(*v.RetryAt) {
		return false
	}
	v.ExpiresAt = now
	c.entries[model] = v
	return true
}

// Peek returns cached evidence immediately; inference must not wait for usage HTTP.
func (c *Cache) Peek(model string) (Status, bool) {
	requested := model
	if k, ok := c.fetcher.(Keyer); ok {
		model = k.QuotaKey(model)
	}
	c.mu.Lock()
	v, ok := c.entries[model]
	local, hasLocal := c.locals[requested]
	now := c.now()
	c.mu.Unlock()
	v = clone(v)
	if ok && now.After(v.ExpiresAt) {
		v.State = "stale"
	}
	if hasLocal && now.Before(local.ExpiresAt) {
		if v.Known && len(v.Windows) == 0 {
			v.Windows = []Window{{Name: v.Source, Unit: v.Unit, Limit: v.Limit, Used: v.Used, ResetAt: v.ResetAt}}
		}
		v.Windows = append(v.Windows, Window{Name: "local_allowance", Unit: "USD", Limit: local.Limit, Used: local.Used})
		_ = normalize(&v)
		ok = true
	}
	return v, ok
}

func clone(s Status) Status {
	s.Limits = append([]LimitGroup(nil), s.Limits...)
	for i := range s.Limits {
		g := &s.Limits[i]
		g.Windows = clone(Status{Windows: g.Windows}).Windows
		if g.Metadata != nil {
			m := make(map[string]any, len(g.Metadata))
			for k, v := range g.Metadata {
				m[k] = cloneMetadata(v)
			}
			g.Metadata = m
		}
	}
	if s.RetryAt != nil {
		t := *s.RetryAt
		s.RetryAt = &t
	}
	s.Balances = append([]Balance(nil), s.Balances...)
	for i := range s.Balances {
		b := &s.Balances[i]
		for _, p := range []**float64{&b.Remaining, &b.Granted, &b.Purchased} {
			if *p != nil {
				v := **p
				*p = &v
			}
		}
		if b.ResetAt != nil {
			t := *b.ResetAt
			b.ResetAt = &t
		}
	}
	s.ReportedUsage = append([]ReportedUsage(nil), s.ReportedUsage...)
	if s.ResetCredits != nil {
		v := *s.ResetCredits
		s.ResetCredits = &v
	}
	if s.Usage != nil {
		v := *s.Usage
		s.Usage = &v
		for _, p := range []**float64{&v.Total, &v.Daily, &v.Weekly, &v.Monthly} {
			if *p != nil {
				x := **p
				*p = &x
			}
		}
	}
	if s.ResetAt != nil {
		t := *s.ResetAt
		s.ResetAt = &t
	}
	s.Windows = append([]Window(nil), s.Windows...)
	for i := range s.Windows {
		if s.Windows[i].ResetAt != nil {
			t := *s.Windows[i].ResetAt
			s.Windows[i].ResetAt = &t
		}
	}
	return s
}
func cloneMetadata(v any) any {
	switch value := v.(type) {
	case map[string]any:
		copy := make(map[string]any, len(value))
		for k, item := range value {
			copy[k] = cloneMetadata(item)
		}
		return copy
	case []any:
		copy := make([]any, len(value))
		for i, item := range value {
			copy[i] = cloneMetadata(item)
		}
		return copy
	default:
		return v
	}
}
func normalize(s *Status) error {
	if s.Provider == "openai-codex" && s.CollectionState == "" {
		s.CollectionState = s.State
	}
	for i := range s.Limits {
		group := &s.Limits[i]
		for j := range group.Windows {
			w := &group.Windows[j]
			if w.State == "unavailable" {
				continue
			}
			v := Status{Known: true, Limit: w.Limit, Used: w.Used}
			if err := normalize(&v); err != nil {
				return err
			}
			w.Remaining, w.FractionRemaining, w.Exhausted = v.Remaining, v.FractionRemaining, v.Exhausted
		}
	}
	if s.ResetCredits != nil && (math.IsNaN(*s.ResetCredits) || math.IsInf(*s.ResetCredits, 0) || *s.ResetCredits < 0) {
		return errors.New("invalid reset credits")
	}
	for _, b := range s.Balances {
		for _, n := range []*float64{b.Remaining, b.Granted, b.Purchased} {
			if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0)) {
				return errors.New("invalid balance")
			}
		}
	}
	for _, u := range s.ReportedUsage {
		if math.IsNaN(u.Used) || math.IsInf(u.Used, 0) || u.Used < 0 {
			return errors.New("invalid reported usage")
		}
	}
	if len(s.Windows) > 0 {
		s.Known = false
		s.Limit, s.Used, s.Remaining, s.FractionRemaining = 0, 0, 0, 0
		s.Exhausted = s.Blocked
		s.Unit = ""
		for i := range s.Windows {
			w := &s.Windows[i]
			v := Status{Known: true, Limit: w.Limit, Used: w.Used}
			if err := normalize(&v); err != nil {
				return err
			}
			w.Remaining, w.FractionRemaining, w.Exhausted = v.Remaining, v.FractionRemaining, v.Exhausted
			if !w.Informational && (!s.Known || w.FractionRemaining < s.FractionRemaining) {
				s.Known, s.Limit, s.Used, s.Remaining, s.FractionRemaining, s.Exhausted = true, v.Limit, v.Used, v.Remaining, v.FractionRemaining, v.Exhausted
				s.Unit, s.ResetAt = w.Unit, w.ResetAt
			}
		}
		s.Exhausted = s.Exhausted || s.Blocked
		return nil
	}
	if !s.Known {
		s.Limit = 0
		s.Used = 0
		s.Remaining = 0
		s.FractionRemaining = 0
		s.Exhausted = s.Blocked
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
		s.Exhausted = s.Remaining == 0 || s.Blocked
	}
	return nil
}

// HTTPError carries only a status code, never an upstream response or token.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	if e.StatusCode == 401 || e.StatusCode == 403 {
		return fmt.Sprintf("Quota endpoint HTTP %d · reconnect a credential with account-usage access", e.StatusCode)
	}
	return fmt.Sprintf("Quota endpoint HTTP %d", e.StatusCode)
}
