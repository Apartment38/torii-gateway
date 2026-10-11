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

func (p *Compatible) QuotaKey(model string) string {
	if p.keyQuotaURL != "" || p.quotaURL != "" {
		if strings.HasSuffix(model, ":free") {
			return p.id + "/@free"
		}
		return p.id + "/@account"
	}
	return model
}

// Quota combines key caps with optional account credits. Inference credentials
// are used only for /key; management credentials remain isolated to /credits.
func (p *Compatible) Quota(ctx context.Context, model string) (quota.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, p.discoveryTO)
	defer cancel()
	account, accountErr := p.accountQuota(ctx)
	if p.keyQuotaURL == "" {
		return account, accountErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.keyQuotaURL, nil)
	if err != nil {
		return quota.Status{}, err
	}
	req.Header = p.headers.Clone()
	resp, err := p.client.Do(req)
	if err != nil {
		if account.Known {
			return account, nil
		}
		return quota.Status{}, errors.New("OpenRouter key usage unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if account.Known {
			return account, nil
		}
		return quota.Status{}, fmt.Errorf("OpenRouter key usage returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data *struct {
			Limit        *float64 `json:"limit"`
			Remaining    *float64 `json:"limit_remaining"`
			Reset        string   `json:"limit_reset"`
			Usage        *float64 `json:"usage"`
			Daily        *float64 `json:"usage_daily"`
			Weekly       *float64 `json:"usage_weekly"`
			Monthly      *float64 `json:"usage_monthly"`
			FreeRequests *struct {
				Limit     *float64 `json:"limit"`
				Remaining *float64 `json:"remaining"`
			} `json:"free_model_daily_requests"`
			Effective *struct {
				Limit     *float64   `json:"limit_usd"`
				Remaining *float64   `json:"remaining_usd"`
				ResetAt   *time.Time `json:"resets_at"`
			} `json:"effective_budget"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) != nil || payload.Data == nil {
		return quota.Status{}, errors.New("invalid OpenRouter key usage")
	}
	d := payload.Data
	status := quota.Status{Source: "openrouter_key", Usage: &quota.Spend{Total: d.Usage, Daily: d.Daily, Weekly: d.Weekly, Monthly: d.Monthly}}
	if strings.HasSuffix(model, ":free") && d.FreeRequests != nil && d.FreeRequests.Limit != nil && d.FreeRequests.Remaining != nil {
		status.Windows = append(status.Windows, quota.Window{Name: "free_daily_requests", Unit: "requests", Limit: *d.FreeRequests.Limit, Used: max(0, *d.FreeRequests.Limit-*d.FreeRequests.Remaining), ResetAt: openRouterReset("daily", time.Now().UTC())})
	}
	if d.Limit != nil && d.Remaining != nil {
		status.Windows = append(status.Windows, quota.Window{Name: "key_spend", Unit: "USD", Limit: *d.Limit, Used: max(0, *d.Limit-*d.Remaining), ResetAt: openRouterReset(d.Reset, time.Now().UTC())})
	}
	if d.Effective != nil && d.Effective.Limit != nil && d.Effective.Remaining != nil {
		status.Windows = append(status.Windows, quota.Window{Name: "effective_budget", Unit: "USD", Limit: *d.Effective.Limit, Used: max(0, *d.Effective.Limit-*d.Effective.Remaining), ResetAt: d.Effective.ResetAt})
	}
	if account.Known {
		status.Windows = append(status.Windows, quota.Window{Name: "account_credits", Unit: "USD", Limit: account.Limit, Used: account.Used})
		status.Balances = account.Balances
	}
	if accountErr != nil {
		status.Partial = true
		status.UnavailableReason = "balance_unavailable"
	}
	return status, nil
}

func openRouterReset(interval string, now time.Time) *time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var reset time.Time
	switch interval {
	case "daily":
		reset = day.AddDate(0, 0, 1)
	case "weekly":
		reset = day.AddDate(0, 0, 7-(int(day.Weekday())+6)%7)
	case "monthly":
		reset = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	default:
		return nil
	}
	return &reset
}
