package provider

import (
	"github.com/Apartment38/torii-gateway/internal/quota"
	"time"
)

func parseNousAccount(payload map[string]any, now time.Time) quota.Status {
	s := quota.Status{Source: "nous_account"}
	if payload["error"] != nil {
		s.UnavailableReason = "account_unavailable"
		return s
	}
	sub := object(payload["subscription"])
	access := object(payload["paid_service_access"])
	s.Plan = firstText(sub, "plan", "plan_name")
	reset := firstReset(sub, now, "current_period_end", "renews_at")
	remaining := firstNumber(access, "subscription_credits_remaining")
	if remaining == nil {
		remaining = numeric(sub["credits_remaining"])
	}
	purchased := numeric(access["purchased_credits_remaining"])
	if purchased == nil {
		purchased = numeric(payload["purchased_credits_remaining"])
	}
	total := numeric(access["total_usable_credits"])
	if total == nil {
		total = numeric(payload["total_usable_credits"])
	}
	if total == nil && remaining != nil && purchased != nil {
		v := max(0, *remaining) + max(0, *purchased)
		total = &v
	}
	balance(&s, "total_usable", "USD", total, nil)
	balance(&s, "subscription", "USD", remaining, reset)
	balance(&s, "purchased", "USD", purchased, nil)
	balance(&s, "rollover", "USD", numeric(sub["rollover_credits"]), nil)
	if cap := numeric(sub["monthly_credits"]); cap != nil && *cap > 0 && remaining != nil && *remaining <= *cap {
		used := max(0, *cap-*remaining)
		appendWindow(&s, "subscription", "USD", cap, &used, reset, 0)
		// This plan gauge does not govern free models or purchased-credit use.
		// Observed inference rate limits remain enforcing windows.
		s.Windows[len(s.Windows)-1].Informational = true
	}
	spend := numeric(payload["member_spend_usd"])
	if spend == nil {
		spend = numeric(access["member_spend_usd"])
	}
	if spend != nil && *spend >= 0 {
		s.Usage = &quota.Spend{Monthly: spend}
		cap := numeric(payload["member_spend_cap_usd"])
		if cap == nil {
			cap = numeric(access["member_spend_cap_usd"])
		}
		appendWindow(&s, "member_spend", "USD", cap, spend, reset, 0)
		if cap != nil {
			for i := range s.Windows {
				if s.Windows[i].Name == "member_spend" {
					s.Windows[i].Informational = true
				}
			}
		}
	}
	if len(s.Balances) == 0 && len(s.Windows) == 0 && s.Usage == nil {
		s.UnavailableReason = "no_reported_data"
	}
	return s
}
