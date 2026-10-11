package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

func (c *Codex) QuotaKey(string) string { return c.id + "/@account" }

// Quota uses the same account-scoped usage read as the upstream Codex client.
// Public API plan-scoped credentials have no verified quota endpoint here.
func (c *Codex) Quota(ctx context.Context, _ string) (quota.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	credential, err := c.credential(ctx)
	if err != nil {
		return quota.Status{}, errors.New("Codex quota requires login")
	}
	base := c.inferenceBase(credential)
	if base.Host == "api.openai.com" {
		return quota.Status{Source: "codex_quota_unavailable"}, nil
	}
	base.Path = strings.TrimSuffix(strings.TrimRight(base.Path, "/"), "/codex") + "/wham/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return quota.Status{}, err
	}
	c.authenticate(req, credential)
	resp, err := c.client.Do(req)
	if err != nil {
		return quota.Status{}, errors.New("Codex quota request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return quota.Status{}, fmt.Errorf("Codex quota returned HTTP %d", resp.StatusCode)
	}
	type window struct {
		UsedPercent *float64 `json:"used_percent"`
		Seconds     int64    `json:"limit_window_seconds"`
		ResetAt     int64    `json:"reset_at"`
	}
	var payload struct {
		RateLimit *struct {
			Allowed   *bool   `json:"allowed"`
			Reached   bool    `json:"limit_reached"`
			Primary   *window `json:"primary_window"`
			Secondary *window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) != nil {
		return quota.Status{}, errors.New("invalid Codex quota response")
	}
	status := quota.Status{Source: "codex_subscription"}
	if payload.RateLimit == nil {
		return status, nil
	}
	limits := payload.RateLimit
	status.Blocked = limits.Reached || limits.Allowed != nil && !*limits.Allowed
	for _, entry := range []struct {
		name  string
		value *window
	}{{"primary", limits.Primary}, {"secondary", limits.Secondary}} {
		w := entry.value
		if w == nil || w.UsedPercent == nil {
			continue
		}
		if *w.UsedPercent < 0 || *w.UsedPercent > 100 || w.Seconds < 0 || w.ResetAt > 253402300799 {
			return quota.Status{}, errors.New("invalid Codex quota window")
		}
		var reset *time.Time
		if w.ResetAt > 0 {
			t := time.Unix(w.ResetAt, 0).UTC()
			reset = &t
		}
		status.Windows = append(status.Windows, quota.Window{Name: entry.name, Unit: "percent", Limit: 100, Used: *w.UsedPercent, WindowSeconds: w.Seconds, ResetAt: reset})
	}
	return status, nil
}
