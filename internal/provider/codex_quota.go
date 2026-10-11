package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

func (c *Codex) QuotaKey(string) string { return c.id + "/@account" }

func (c *Codex) Quota(ctx context.Context, _ string) (quota.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	credential, err := c.credential(ctx)
	if err != nil {
		return quota.Status{}, errors.New("Codex quota requires login")
	}
	base := c.inferenceBase(credential)
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
	c.authenticate(req, credential)
	cfg := TrackingConfig{Client: c.client, Headers: req.Header}
	payload, err := trackingJSON(ctx, cfg, http.MethodGet, base.String(), nil)
	if err != nil {
		return quota.Status{Source: "codex_subscription", UnavailableReason: trackingReason(err)}, nil
	}
	return parseCodexUsage(payload, time.Now().UTC()), nil
}

func parseCodexUsage(payload map[string]any, now time.Time) quota.Status {
	s := quota.Status{Source: "codex_subscription", Plan: textValue(payload["plan_type"])}
	limits := object(payload["rate_limit"])
	s.Blocked = limits["limit_reached"] == true || limits["allowed"] == false
	parse := func(m map[string]any, prefix, model string) {
		for _, name := range []string{"primary", "secondary"} {
			w := object(m[name+"_window"])
			seconds := int64(0)
			if n := numeric(w["limit_window_seconds"]); n != nil && *n >= 0 && *n <= 31536000 {
				seconds = int64(*n)
			}
			before := len(s.Windows)
			appendPercent(&s, prefix+name, numeric(w["used_percent"]), resetTime(w["reset_at"], now), seconds)
			if model != "" && len(s.Windows) > before {
				s.Windows[len(s.Windows)-1].Model = model
				s.Windows[len(s.Windows)-1].Informational = true
			}
		}
	}
	parse(limits, "", "")
	for _, v := range array(payload["additional_rate_limits"]) {
		m := object(v)
		model := textValue(m["limit_name"])
		if model != "" {
			parse(object(m["rate_limit"]), model+"_", model)
		}
	}
	credits := object(payload["credits"])
	if credits["unlimited"] == true {
		s.Balances = append(s.Balances, quota.Balance{Name: "credits", Unit: "USD", Unlimited: true})
	} else {
		balance(&s, "credits", "USD", numeric(credits["balance"]), nil)
	}
	if n := numeric(object(payload["rate_limit_reset_credits"])["available_count"]); n != nil && *n >= 0 {
		s.ResetCredits = n
	}
	if len(s.Windows) == 0 && len(s.Balances) == 0 && s.ResetCredits == nil {
		s.UnavailableReason = "no_reported_data"
	}
	return s
}
