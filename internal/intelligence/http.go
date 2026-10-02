package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type FetchOptions struct {
	ResolveKey      func() (string, error)
	Client          *http.Client
	Timeout         time.Duration
	Retries         int
	RequestInterval time.Duration
}

type SourceError struct {
	Stage  string
	Status int
}

func (e *SourceError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s returned HTTP %d", e.Stage, e.Status)
	}
	return e.Stage + " failed"
}

type remote struct {
	resolveKey      func() (string, error)
	cfg             SourceSettings
	key             string
	client          *http.Client
	retries         int
	requestInterval time.Duration
}

func (r *remote) ID() string { return r.cfg.ID }

type Factory func(SourceSettings, string, FetchOptions) (Source, error)
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }
func (r *Registry) Register(name string, factory Factory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" || factory == nil || r.factories[name] != nil {
		return errors.New("invalid or duplicate benchmark source registration")
	}
	r.factories[name] = factory
	return nil
}
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.factories[name] != nil
}
func (r *Registry) New(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
	r.mu.RLock()
	factory := r.factories[cfg.Type]
	r.mu.RUnlock()
	if factory == nil {
		return nil, errors.New("unsupported benchmark source type")
	}
	return factory(cfg, key, options)
}

// Builtins is only a registration list, not part of matching/aggregation/routing.
func Builtins() *Registry {
	r := NewRegistry()
	_ = r.Register("artificial_analysis", newArtificialAnalysis)
	_ = r.Register("benchlm", newBenchLM)
	_ = r.Register("huggingface", newHuggingFace)
	_ = r.Register("lmarena", newArena)
	_ = r.Register("livebench", newLiveBench)
	return r
}

func newRemote(cfg SourceSettings, key, defaultURL string, options FetchOptions) (*remote, error) {
	if cfg.URL == "" {
		cfg.URL = defaultURL
	}
	if err := ValidateURL(cfg.URL); err != nil {
		return nil, err
	}
	if strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("benchmark API key contains a newline")
	}
	if cfg.MaxRows == 0 {
		cfg.MaxRows = 100000
	}
	if options.Timeout <= 0 {
		options.Timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: options.Timeout}
	if options.Client != nil {
		*client = *options.Client
		client.Timeout = options.Timeout
	}
	// Never forward credentials to redirects, including cross-origin downloads.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if options.RequestInterval <= 0 {
		options.RequestInterval = time.Second
	}
	return &remote{cfg: cfg, key: key, resolveKey: options.ResolveKey, client: client, retries: options.Retries, requestInterval: options.RequestInterval}, nil
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return errors.New("benchmark URL must be an HTTP(S) URL without embedded credentials or fragment")
	}
	for name := range u.Query() {
		n := strings.ToLower(name)
		if strings.Contains(n, "key") || strings.Contains(n, "token") || strings.Contains(n, "secret") {
			return errors.New("benchmark credentials must be sent in headers, not URL query parameters")
		}
	}
	return nil
}

func (r *remote) get(ctx context.Context, rawURL, authHeader string) ([]byte, error) {
	var last error
	lane := laneFor(rawURL)
	for attempt := 0; attempt <= r.retries; attempt++ {
		release, err := lane.acquire(ctx, r.requestInterval)
		if err != nil {
			return nil, &SourceError{Stage: "upstream timeout or cancellation"}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			release()
			return nil, &SourceError{Stage: "request construction"}
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Torii-Gateway/benchmark-intelligence")
		key := r.key
		if r.resolveKey != nil {
			key, err = r.resolveKey()
			if err != nil || strings.ContainsAny(key, "\r\n") {
				release()
				return nil, &SourceError{Stage: "source credentials"}
			}
		}
		if key != "" {
			value := key
			if authHeader == "Authorization" {
				value = "Bearer " + value
			}
			req.Header.Set(authHeader, value)
		}
		resp, err := r.client.Do(req)
		delay := retryBackoff(attempt, false)
		if err != nil {
			last = &SourceError{Stage: "upstream request"}
		} else {
			lane.observeQuota(resp.Header)
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lane.succeeded()
				release()
				if readErr != nil || len(data) > 16<<20 {
					return nil, &SourceError{Stage: "response size or read"}
				}
				return data, nil
			}
			last = &SourceError{Stage: "upstream request", Status: resp.StatusCode}
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				release()
				return nil, last
			}
			throttled := resp.StatusCode == http.StatusTooManyRequests
			if throttled {
				delay = lane.throttleBackoff(attempt)
			}
			if remaining, reset, ok := quotaWindow(resp.Header); ok && remaining == 0 {
				delay = max(reset, r.requestInterval)
			}
			if instructed, ok := retryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
				if remaining, reset, valid := quotaWindow(resp.Header); valid && remaining == 0 {
					instructed = max(instructed, reset)
				}
				delay = max(instructed, r.requestInterval)
			}
			if throttled || resp.Header.Get("Retry-After") != "" {
				lane.pause(delay, r.requestInterval, throttled)
			}
			if throttled {
				slog.Warn("benchmark_upstream_rate_limited", "source", r.ID(), "retry", attempt, "wait_seconds", delay.Seconds())
				reportRateLimit(ctx, fmt.Sprintf("Upstream rate limit; waiting %.1f seconds before more requests", delay.Seconds()))
			}
		}
		release()
		if attempt == r.retries {
			break
		}
		if err := waitDuration(ctx, delay); err != nil {
			return nil, &SourceError{Stage: "upstream timeout or cancellation"}
		}
	}
	return nil, last
}

func decodeObject(data []byte) (map[string]any, error) {
	var document map[string]any
	if json.Unmarshal(data, &document) != nil || document == nil {
		return nil, &SourceError{Stage: "JSON parsing"}
	}
	return document, nil
}
func object(value any) map[string]any  { m, _ := value.(map[string]any); return m }
func text(value any) string            { s, _ := value.(string); return s }
func number(value any) (float64, bool) { n, ok := value.(float64); return n, ok }
func pathValue(row map[string]any, path string) any {
	if value, ok := row[path]; ok {
		return value
	}
	parts := strings.Split(path, ".")
	var value any = row
	for _, part := range parts {
		value = object(value)[part]
	}
	return value
}
func parseDate(value any) time.Time {
	if seconds, ok := number(value); ok {
		return time.Unix(int64(seconds), 0).UTC()
	}
	for _, format := range []string{time.RFC3339Nano, "2006-01-02", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(format, text(value)); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

// Kept as a helper so source parsers always produce deterministic observations.
func sortStrings(keys []string) {
	sort.Strings(keys)
}

func measurement(model, benchmark, category string, score float64, mapping *Mapping, date time.Time, version string, metadata map[string]any) (Observation, error) {
	copyMetadata := map[string]any{}
	for key, value := range metadata {
		copyMetadata[key] = value
	}
	o := Observation{ExternalModelID: model, Benchmark: benchmark, Category: category, Score: score, EvaluatedAt: date, Version: version, Metadata: copyMetadata}
	if mapping != nil {
		o.Category, o.Unit = mapping.Category, mapping.Unit
		if mapping.Category == Operational && mapping.Metric != "" {
			o.Benchmark = mapping.Metric
		}
		if mapping.Category != Operational {
			normalized, err := Normalize(score, mapping.Min, mapping.Max, mapping.LowerIsBetter)
			if err != nil {
				return o, &SourceError{Stage: "score normalization"}
			}
			o.Normalized = normalized
			o.Metadata["normalization"] = map[string]any{"min": mapping.Min, "max": mapping.Max, "lower_is_better": mapping.LowerIsBetter}
		}
	}
	return o, nil
}

func operationUnit(metric string) string {
	switch metric {
	case "input_price", "output_price":
		return "USD_per_million_tokens"
	case "latency", "ttft":
		return "seconds"
	case "throughput":
		return "tokens_per_second"
	case "context_length":
		return "tokens"
	}
	return ""
}
