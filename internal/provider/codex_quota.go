package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

// Account changes invalidate shared snapshots. No token material is exposed.
func (c *Codex) QuotaKey(string) string {
	v, err := c.load(c.id)
	if err != nil {
		return c.id + "/@signed-out"
	}
	return c.quotaIdentityKey(v)
}
func (c *Codex) quotaIdentityKey(v CodexCredential) string {
	subject := v.Subject
	if subject == "" {
		claims := codexClaims(v.AccessToken)
		subject = textValue(object(claims["https://api.openai.com/auth"])["chatgpt_user_id"])
		if subject == "" {
			subject = textValue(claims["sub"])
		}
	}
	scopes := append([]string(nil), v.Scopes...)
	sort.Strings(scopes)
	account := v.AccountID
	if account == "" {
		account = codexTokenAccount(v.AccessToken)
	}
	identity := account + "/" + subject + "/" + v.ClientID + "/" + v.Issuer + "/" + strings.Join(scopes, " ")
	if subject == "" || account == "" {
		identity += "/" + v.AccessToken
	} // Unknown user: isolate logins, including shared workspaces.
	hash := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%s/@account/%x", c.id, hash[:12])
}

// QuotaResponseKey carries internal account association without adding HTTP headers.
// It contains an opaque cache key, never tokens or login metadata.
type codexQuotaContextKey struct{}

func CodexQuotaResponseKey(response *http.Response) string {
	if response == nil || response.Request == nil {
		return ""
	}
	key, _ := response.Request.Context().Value(codexQuotaContextKey{}).(string)
	return key
}

type codexAuthError struct{ error }

func (c *Codex) Quota(ctx context.Context, _ string) (quota.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	v, err := c.credential(ctx)
	if err != nil {
		saved, _ := c.load(c.id)
		return quota.Status{Provider: "openai-codex", AccountID: saved.AccountID, AccountIdentity: saved.Email, State: "auth_expired", UnavailableReason: "oauth_login_or_refresh_required"}, nil
	}
	fail := quota.Status{Provider: "openai-codex", AccountID: v.AccountID, AccountIdentity: v.Email, Source: "codex_usage_endpoint", State: "unavailable"}
	if strings.HasPrefix(v.AccessToken, "sk-") {
		fail.State = "permission_denied"
		fail.UnavailableReason = "oauth_required_api_key_supplied"
		return fail, nil
	}
	// The account header and a persisted OAuth account must agree with token hints.
	if account := codexTokenAccount(v.AccessToken); account != "" && v.AccountID != "" && account != v.AccountID {
		fail.State = "permission_denied"
		fail.UnavailableReason = "account_context_mismatch"
		return fail, nil
	}
	if v.AccountID == "" {
		v.AccountID = codexTokenAccount(v.AccessToken)
		fail.AccountID = v.AccountID
	}
	if c.appServer.Path != "" {
		appCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		payload, appErr := c.appServerQuota(appCtx, v)
		stop()
		if appErr == nil {
			s := parseCodexUsage(payload, time.Now().UTC())
			s.Source = "codex_app_server"
			stampCodex(&s, v)
			if s.State == "available" {
				return s, nil
			}
		}
	}
	base := c.inferenceBase(v)
	path := strings.TrimSuffix(strings.TrimRight(base.Path, "/"), "/codex")
	if strings.Contains(path, "/backend-api") {
		base.Path = path + "/wham/usage"
	} else {
		if base.Host == "api.openai.com" {
			path = ""
		}
		base.Path = path + "/api/codex/usage"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	req.Header.Set("Accept", "application/json")
	resp, err := c.doAuthenticated(ctx, req, v)
	if err != nil {
		var authErr *codexAuthError
		if errors.As(err, &authErr) {
			fail.State = "auth_expired"
			fail.UnavailableReason = "oauth_refresh_failed"
		} else {
			fail.UnavailableReason = "usage_request_failed"
		}
		return fail, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		switch resp.StatusCode {
		case 401:
			fail.State = "auth_expired"
			fail.UnavailableReason = "usage_auth_rejected_after_refresh_check_account_and_oauth_context"
			if v.RefreshToken == "" || c.save == nil || c.issuer == "" {
				fail.UnavailableReason = "usage_auth_rejected_refresh_unavailable"
			}
		case 403:
			fail.State = "permission_denied"
			fail.UnavailableReason = "usage_permission_denied"
		case 429:
			fail.State = "rate_limited"
			fail.UnavailableReason = "rate_limited"
			fail.RetryAt = quota.ParseReset(resp.Header.Get("Retry-After"), time.Now().UTC())
		case 404, 405:
			fail.UnavailableReason = "endpoint_unavailable"
		default:
			fail.UnavailableReason = "usage_endpoint_failed"
		}
		return fail, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		fail.UnavailableReason = "invalid_usage_response"
		return fail, nil
	}
	var payload map[string]any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if dec.Decode(&payload) != nil || payload == nil || dec.Decode(new(any)) != io.EOF {
		fail.UnavailableReason = "invalid_usage_response"
		return fail, nil
	}
	// Re-read after a 401 refresh and never attach a result to a replacement login.
	latest, loadErr := c.load(c.id)
	if loadErr != nil || latest.AccountID != v.AccountID && latest.AccountID != "" || latest.Subject != v.Subject {
		fail.UnavailableReason = "account_changed_during_collection"
		return fail, nil
	}
	if reported := textValue(payload["account_id"]); reported != "" && v.AccountID != "" && reported != v.AccountID {
		fail.State = "permission_denied"
		fail.UnavailableReason = "account_context_mismatch"
		return fail, nil
	}
	if v.AccountID == "" {
		v.AccountID = textValue(payload["account_id"])
	}
	s := parseCodexUsage(payload, time.Now().UTC())
	s.Source = "codex_usage_endpoint"
	stampCodex(&s, v)
	return s, nil
}

func codexClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	return payload
}
func codexTokenAccount(token string) string {
	return textValue(object(codexClaims(token)["https://api.openai.com/auth"])["chatgpt_account_id"])
}
func stampCodex(s *quota.Status, v CodexCredential) {
	s.Provider = "openai-codex"
	s.AccountID = v.AccountID
	s.AccountIdentity = v.Email
	for i := range s.Limits {
		for j := range s.Limits[i].Windows {
			s.Limits[i].Windows[j].Source = s.Source
		}
	}
	for i := range s.Windows {
		s.Windows[i].Source = s.Source
	}
}

// Both schemas are verified against official Codex protocol/backend fixtures.
func parseCodexUsage(payload map[string]any, now time.Time) quota.Status {
	s := quota.Status{Provider: "openai-codex", Source: "codex_subscription", Plan: textValue(payload["plan_type"]), State: "available", CollectedAt: now}
	add := func(m map[string]any, id, key, name, scope string, camel bool) {
		if m == nil {
			return
		}
		g := quota.LimitGroup{LimitID: id, MapKey: key, LimitName: name, Scope: scope, State: "unavailable", Windows: []quota.Window{}}
		if camel {
			if v := textValue(m["limitId"]); v != "" {
				g.LimitID = v
			}
			g.LimitName = textValue(m["limitName"])
			g.Model = textValue(m["normalModelSlug"])
			if s.Plan == "" {
				s.Plan = textValue(m["planType"])
			}
		}
		g.Metadata = map[string]any{}
		if _, ok := m["allowed"].(bool); ok {
			g.State = "available"
		}
		if _, ok := m["limit_reached"].(bool); ok {
			g.State = "available"
		}
		if m["allowed"] == false || m["limit_reached"] == true || m["spendControlReached"] == true {
			g.State = "exhausted"
		}
		if scope == "account" && m["spendControlReached"] == true {
			s.Blocked = true
		}
		for _, k := range []string{"normalModelSlug", "normal_model_slug", "metered_feature", "spendControlReached", "individualLimit", "rateLimitReachedType", "planType", "credits", "allowed", "limit_reached"} {
			if v, ok := m[k]; ok {
				g.Metadata[k] = v
			}
		}
		for _, slot := range []string{"primary", "secondary"} {
			w := object(m[slot+"_window"])
			usedKey, durationKey, resetKey := "used_percent", "limit_window_seconds", "reset_at"
			multiplier := int64(1)
			if camel {
				w = object(m[slot])
				usedKey, durationKey, resetKey = "usedPercent", "windowDurationMins", "resetsAt"
				multiplier = 60
			}
			if w == nil {
				continue
			}
			seconds := int64(0)
			if n := numeric(w[durationKey]); n != nil && *n > 0 && *n < float64(math.MaxInt64/multiplier) {
				seconds = int64(*n) * multiplier
			}
			reset := codexReset(w[resetKey])
			if reset == nil && !camel {
				if n := numeric(w["reset_after_seconds"]); n != nil && *n >= 0 && *n < float64(math.MaxInt64)/float64(time.Second) {
					t := now.Add(time.Duration(*n) * time.Second).UTC()
					reset = &t
				}
			}
			window := quota.Window{Name: slot, Slot: slot, WindowType: quota.WindowType(seconds), WindowSeconds: seconds, Unit: "percent", ResetAt: reset, Source: s.Source, LastUpdated: now, State: "unavailable", Informational: scope != "account"}
			if n := numeric(w[usedKey]); n != nil && *n >= 0 {
				window.Limit = 100
				window.Used = *n
				window.Remaining = math.Max(0, math.Min(100, 100-*n))
				window.FractionRemaining = window.Remaining / 100
				window.Exhausted = *n >= 100
				window.State = "available"
				if g.State != "exhausted" {
					g.State = "available"
				}
				if window.Exhausted {
					g.State = "exhausted"
				}
				// Backward compatible ordinary quota fields. Unknown pools never gate routing.
				flat := window
				if g.LimitName != "" {
					flat.Name = g.LimitName + "_" + slot
				}
				s.Windows = append(s.Windows, flat)
			}
			g.Windows = append(g.Windows, window)
		}
		s.Limits = append(s.Limits, g)
	}
	if groups := object(payload["rateLimitsByLimitId"]); groups != nil && len(groups) > 0 {
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			scope := "unknown"
			id := textValue(object(groups[key])["limitId"])
			if key == "codex" && (id == "" || id == "codex") {
				scope = "account"
			}
			add(object(groups[key]), key, key, "", scope, true)
		}
		legacy := object(payload["rateLimits"])
		if legacy != nil {
			id := textValue(legacy["limitId"])
			duplicate := false
			for _, g := range s.Limits {
				if id != "" && (g.LimitID == id && (g.MapKey == "" || g.MapKey == id)) {
					duplicate = true
				}
			}
			if !duplicate {
				scope := "unknown"
				if id == "codex" {
					scope = "account"
				}
				add(legacy, id, "", "", scope, true)
			}
		}
	} else if legacy := object(payload["rateLimits"]); legacy != nil {
		scope := "unknown"
		if textValue(legacy["limitId"]) == "codex" {
			scope = "account"
		}
		add(legacy, textValue(legacy["limitId"]), "", "", scope, true)
	} else {
		limits := object(payload["rate_limit"])
		s.Blocked = limits["limit_reached"] == true || limits["allowed"] == false
		add(limits, "codex", "", "", "account", false)
		for _, v := range array(payload["additional_rate_limits"]) {
			m := object(v)
			id := textValue(m["metered_feature"])
			if id == "" {
				id = textValue(m["limit_id"])
			}
			before := len(s.Limits)
			add(object(m["rate_limit"]), id, "", textValue(m["limit_name"]), "unknown", false)
			if len(s.Limits) > before {
				for _, k := range []string{"metered_feature", "normal_model_slug"} {
					if v, ok := m[k]; ok {
						s.Limits[before].Metadata[k] = v
					}
				}
			}
		}
	}
	if allowed, ok := payload["ordinaryUsageAllowed"].(bool); ok {
		s.Blocked = !allowed
	}
	credits := object(payload["credits"])
	if credits == nil {
		credits = object(object(payload["rateLimits"])["credits"])
	}
	if credits["unlimited"] == true {
		s.Balances = append(s.Balances, quota.Balance{Name: "credits", Unit: "USD", Unlimited: true})
	} else {
		balance(&s, "credits", "USD", numeric(credits["balance"]), nil)
	}
	if n := numeric(object(payload["rate_limit_reset_credits"])["available_count"]); n != nil && *n >= 0 {
		s.ResetCredits = n
	}
	if n := numeric(object(payload["rateLimitResetCredits"])["availableCount"]); n != nil && *n >= 0 {
		s.ResetCredits = n
	}
	reportedState := s.Blocked
	if _, ok := payload["ordinaryUsageAllowed"].(bool); ok {
		reportedState = true
	}
	for _, g := range s.Limits {
		if g.State == "available" || g.State == "exhausted" {
			reportedState = true
		}
	}
	if len(s.Windows) == 0 && len(s.Balances) == 0 && s.ResetCredits == nil && !reportedState {
		s.State = "unavailable"
		s.UnavailableReason = "no_reported_data"
		s.CollectedAt = time.Time{}
	}
	return s
}
func codexReset(v any) *time.Time {
	if n := numeric(v); n != nil && *n >= 0 && *n <= 253402300799 {
		t := time.Unix(int64(*n), 0).UTC()
		return &t
	}
	if raw := textValue(v); raw != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
