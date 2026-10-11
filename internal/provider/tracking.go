package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

// TrackingConfig describes a read-only billing surface. Its client and headers
// are separate from inference so admin/OAuth credentials cannot reach inference.
type TrackingConfig struct {
	Provider, BaseURL, UsageURL, TeamID, Mode string
	Headers                                   http.Header
	Client                                    *http.Client
}

type trackedProvider struct {
	OpenAIProvider
	tracking TrackingConfig
}

func WithTracking(p OpenAIProvider, cfg TrackingConfig) OpenAIProvider {
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}
	client := *cfg.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	cfg.Client = &client
	cfg.Headers = cfg.Headers.Clone()
	return &trackedProvider{OpenAIProvider: p, tracking: cfg}
}

func (p *trackedProvider) QuotaKey(string) string { return p.ID() + "/@account" }
func (p *trackedProvider) Models(ctx context.Context) ([]Model, error) {
	models, err := p.OpenAIProvider.Models(ctx)
	kind := p.tracking.Provider
	included := kind == "opencode-go" || kind == "standard-compute" || kind == "kimi-coding" || p.tracking.Mode == "subscription" && kind != "anthropic"
	for i := range models {
		priceModel := models[i]
		if compatible, ok := p.OpenAIProvider.(*Compatible); ok {
			alias := strings.TrimPrefix(priceModel.ID, p.ID()+"/")
			if upstream := compatible.rewrites[alias]; upstream != "" {
				priceModel.ID = p.ID() + "/" + upstream
			}
		}
		applyTrackingPrice(kind, &priceModel)
		models[i].InputPerMillion, models[i].OutputPerMillion, models[i].PricingSource = priceModel.InputPerMillion, priceModel.OutputPerMillion, priceModel.PricingSource
	}
	if included {
		for i := range models {
			models[i].Subscription = true
			zero := 0.0
			models[i].InputPerMillion, models[i].OutputPerMillion = &zero, &zero
			models[i].PricingSource = "subscription_included"
		}
	}
	return models, err
}

func (p *trackedProvider) Quota(ctx context.Context, _ string) (quota.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cfg := p.tracking
	status := quota.Status{Source: cfg.Provider + "_usage"}
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint := cfg.UsageURL
	if endpoint == "" {
		switch cfg.Provider {
		case "deepseek":
			endpoint = base + "/user/balance"
		case "opencode-go", "opencode-zen":
			endpoint = base + "/zen/go/v1/usage"
		case "zai":
			endpoint = base + "/api/monitor/usage/quota/limit"
		case "kimi-coding":
			endpoint = base + "/coding/v1/usages"
		case "minimax":
			endpoint = base + "/v1/token_plan/remains"
		case "xai":
			if cfg.TeamID == "" || cfg.Headers.Get("Authorization") == "" {
				status.UnavailableReason = "management_credentials_required"
				return status, nil
			}
			endpoint = base + "/v1/billing/teams/" + url.PathEscape(cfg.TeamID) + "/prepaid/balance"
		case "anthropic":
			if cfg.Mode != "subscription" {
				return p.anthropicReports(ctx, status)
			}
			if cfg.Headers.Get("Authorization") == "" {
				status.UnavailableReason = "oauth_credentials_required"
				return status, nil
			}
			endpoint = base + "/api/oauth/usage"
		case "standard-compute":
			status.UnavailableReason = "response_headers_only"
			return status, nil
		default:
			status.UnavailableReason = "unsupported_tracking_provider"
			return status, nil
		}
	}
	payload, err := trackingJSON(ctx, cfg, http.MethodGet, endpoint, nil)
	if err != nil {
		status.UnavailableReason = trackingReason(err)
		if cfg.Provider == "opencode-zen" && cfg.UsageURL == "" {
			status.UnavailableReason = "wallet_balance_not_exposed"
		}
		return status, nil
	}
	status = parseTrackingPayload(cfg.Provider, payload, time.Now().UTC())
	if cfg.Provider == "anthropic" && cfg.Mode == "subscription" {
		for i := range status.Windows {
			status.Windows[i].Informational = true
			status.Windows[i].Model = ""
		}
	}
	if cfg.Provider == "xai" {
		p.xaiSpend(ctx, &status)
	}
	return status, nil
}

type trackingHTTPError int

func (e trackingHTTPError) Error() string { return fmt.Sprintf("tracking HTTP %d", e) }
func trackingReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var code trackingHTTPError
	if errors.As(err, &code) {
		switch code {
		case 401, 403:
			return "credential_rejected"
		case 404, 405:
			return "endpoint_unavailable"
		case 429:
			return "rate_limited"
		}
	}
	return "tracking_unavailable"
}

func trackingJSON(ctx context.Context, cfg TrackingConfig, method, endpoint string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, errors.New("invalid tracking URL")
	}
	req.Header = cfg.Headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cfg.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("tracking request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, trackingHTTPError(resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("invalid tracking response")
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || payload == nil {
		return nil, errors.New("invalid tracking JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid tracking JSON")
	}
	return payload, nil
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func textValue(v any) string      { s, _ := v.(string); return s }
func numeric(v any) *float64 {
	var raw string
	switch n := v.(type) {
	case json.Number:
		raw = string(n)
	case string:
		raw = n
	case float64:
		raw = strconv.FormatFloat(n, 'g', -1, 64)
	default:
		return nil
	}
	n, ok := finiteNumber(raw)
	if !ok {
		return nil
	}
	return &n
}
func finiteNumber(raw string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}
func firstNumber(m map[string]any, names ...string) *float64 {
	for _, n := range names {
		if v := numeric(m[n]); v != nil {
			return v
		}
	}
	return nil
}
func firstText(m map[string]any, names ...string) string {
	for _, n := range names {
		if v := textValue(m[n]); v != "" {
			return v
		}
	}
	return ""
}
func resetTime(v any, now time.Time) *time.Time {
	if n := numeric(v); n != nil {
		if *n <= 0 {
			return nil
		}
		return quota.ParseReset(strconv.FormatFloat(*n, 'f', -1, 64), now)
	}
	return quota.ParseReset(textValue(v), now)
}
func firstReset(m map[string]any, now time.Time, names ...string) *time.Time {
	for _, n := range names {
		if t := resetTime(m[n], now); t != nil {
			return t
		}
	}
	return nil
}
func appendWindow(s *quota.Status, name, unit string, limit, used *float64, reset *time.Time, seconds int64) {
	if limit == nil || used == nil || *limit < 0 || *used < 0 {
		return
	}
	s.Windows = append(s.Windows, quota.Window{Name: name, Unit: unit, Limit: *limit, Used: *used, ResetAt: reset, WindowSeconds: seconds})
}
func appendPercent(s *quota.Status, name string, percent *float64, reset *time.Time, seconds int64) {
	if percent == nil || *percent < 0 || *percent > 100 {
		return
	}
	cap := 100.0
	appendWindow(s, name, "percent", &cap, percent, reset, seconds)
}
func balance(s *quota.Status, name, unit string, remaining *float64, reset *time.Time) {
	if remaining != nil {
		s.Balances = append(s.Balances, quota.Balance{Name: name, Unit: unit, Remaining: remaining, ResetAt: reset})
	}
}

func parseTrackingPayload(kind string, payload map[string]any, now time.Time) quota.Status {
	s := quota.Status{Source: kind + "_usage"}
	switch kind {
	case "deepseek":
		available, ok := payload["is_available"].(bool)
		if !ok {
			s.UnavailableReason = "invalid_response"
			break
		}
		s.Blocked = !available
		for _, v := range array(payload["balance_infos"]) {
			m := object(v)
			unit := textValue(m["currency"])
			n := numeric(m["total_balance"])
			if unit == "" || n == nil {
				continue
			}
			s.Balances = append(s.Balances, quota.Balance{Name: "account", Unit: unit, Remaining: n, Granted: numeric(m["granted_balance"]), Purchased: numeric(m["topped_up_balance"])})
		}
	case "opencode-go", "opencode-zen":
		parseOpenCode(&s, payload, now, 0)
		if kind == "opencode-zen" && len(s.Balances) == 0 {
			s.Partial = true
			s.UnavailableReason = "wallet_balance_not_exposed"
		}
	case "zai":
		parseZAI(&s, payload, now)
	case "kimi-coding":
		parseKimi(&s, payload, now)
	case "minimax":
		parseMiniMax(&s, payload, now)
	case "xai":
		if n := numeric(object(payload["total"])["val"]); n != nil {
			v := -*n / 100
			balance(&s, "prepaid", "USD", &v, nil)
		}
	case "anthropic":
		parseAnthropic(&s, payload, now)
	case "standard-compute":
		m := payload
		if d := object(payload["data"]); d != nil {
			m = d
		}
		reset := firstReset(m, now, "renews_at", "reset_at", "budget_renews_at")
		balance(&s, "compute_budget", "USD", firstNumber(m, "remaining_usd", "balance_usd"), reset)
		appendWindow(&s, "monthly_compute", "USD", numeric(m["budget_usd"]), numeric(m["used_usd"]), reset, 0)
	}
	if len(s.Windows) == 0 && len(s.Balances) == 0 && s.UnavailableReason == "" {
		s.UnavailableReason = "no_reported_data"
	}
	return s
}
