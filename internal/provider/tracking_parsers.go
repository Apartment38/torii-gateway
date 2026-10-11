package provider

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

func parseOpenCode(s *quota.Status, m map[string]any, now time.Time, depth int) {
	if depth > 4 {
		return
	}
	for _, entry := range []struct {
		name    string
		keys    []string
		seconds int64
	}{
		{"rolling", []string{"rolling", "rollingUsage", "fiveHour", "five_hour"}, 18000},
		{"weekly", []string{"weekly", "weeklyUsage"}, 604800},
		{"monthly", []string{"monthly", "monthlyUsage"}, 0},
	} {
		var w map[string]any
		for _, k := range entry.keys {
			if w = object(m[k]); w != nil {
				break
			}
		}
		if w == nil {
			continue
		}
		percent := firstNumber(w, "percent", "usagePercent", "usedPercent", "utilization")
		reset := firstReset(w, now, "resetsAt", "resetAt", "reset_at")
		if reset == nil {
			if n := firstNumber(w, "resetInSec", "resetInSeconds"); n != nil && *n >= 0 && *n <= 31536000 {
				t := now.Add(time.Duration(*n * float64(time.Second)))
				reset = &t
			}
		}
		if percent != nil {
			appendPercent(s, entry.name, percent, reset, entry.seconds)
		} else {
			appendWindow(s, entry.name, "units", firstNumber(w, "limit", "total"), firstNumber(w, "used", "usage"), reset, entry.seconds)
		}
	}
	// Only explicitly denominated credits are accepted; opaque integer wallet
	// fields must not be interpreted as dollars (Zen uses internal nano-units).
	balance(s, "wallet", "USD", firstNumber(m, "balance_usd", "remaining_usd"), nil)
	if n := numeric(m["remaining_credits"]); n != nil {
		unit := firstText(m, "currency", "credit_unit")
		if unit == "" {
			unit = "credits"
		}
		balance(s, "wallet", unit, n, nil)
	}
	if m["remaining_credits"] == nil {
		if total, used := numeric(m["total_credits"]), numeric(m["total_usage"]); total != nil && used != nil && *total >= 0 && *used >= 0 {
			unit := firstText(m, "currency", "credit_unit")
			if unit == "" {
				unit = "credits"
			}
			left := *total - *used
			balance(s, "wallet", unit, &left, nil)
		}
	}
	for _, k := range []string{"data", "result", "usage", "credits"} {
		if nested := object(m[k]); nested != nil {
			parseOpenCode(s, nested, now, depth+1)
		}
	}
}

func parseZAI(s *quota.Status, payload map[string]any, now time.Time) {
	if payload["success"] == false {
		s.UnavailableReason = "no_subscription"
		return
	}
	for _, v := range array(object(payload["data"])["limits"]) {
		w := object(v)
		kind := textValue(w["type"])
		name, seconds, unit := kind, int64(0), "tokens"
		bucket := numeric(w["unit"])
		if bucket != nil {
			switch *bucket {
			case 3:
				name = "session"
				seconds = 18000
			case 6:
				name = "weekly"
				seconds = 604800
			}
		}
		if kind == "TIME_LIMIT" {
			name = "monthly_web_tools"
			unit = "requests"
		} else if kind == "CREDIT_LIMIT" {
			unit = "credits"
		} else if kind != "TOKENS_LIMIT" {
			continue
		}
		reset := resetTime(w["nextResetTime"], now)
		limit, used := numeric(w["usage"]), numeric(w["currentValue"])
		before := len(s.Windows)
		if limit != nil && used != nil {
			appendWindow(s, name, unit, limit, used, reset, seconds)
		} else {
			appendPercent(s, name, numeric(w["percentage"]), reset, seconds)
		}
		if kind == "TIME_LIMIT" && len(s.Windows) > before {
			s.Windows[len(s.Windows)-1].Informational = true
		}
	}
}

func parseKimi(s *quota.Status, payload map[string]any, now time.Time) {
	usages := object(payload["usages"])
	keys := make([]string, 0, len(usages))
	for k := range usages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		w := object(usages[key])
		n := numeric(w["used_ratio"])
		if n != nil {
			v := *n * 100
			seconds := int64(0)
			if key == "limit_5h" {
				seconds = 18000
			}
			appendPercent(s, key, &v, resetTime(w["reset_time"], now), seconds)
		}
	}
	parse := func(w map[string]any, name string) {
		if w == nil {
			return
		}
		if scope := firstText(w, "scope", "name"); scope != "" {
			name = scope
		}
		detail := object(w["detail"])
		limit, used, remaining := numeric(w["limit"]), numeric(w["used"]), numeric(w["remaining"])
		if limit == nil {
			limit = numeric(detail["limit"])
		}
		if used == nil {
			used = numeric(detail["used"])
		}
		if remaining == nil {
			remaining = numeric(detail["remaining"])
		}
		if used == nil && limit != nil && remaining != nil {
			v := max(0, *limit-*remaining)
			used = &v
		}
		reset := firstReset(w, now, "resetTime", "reset_time")
		if reset == nil {
			reset = resetTime(detail["resetTime"], now)
		}
		if limit != nil && used != nil {
			appendWindow(s, name, "units", limit, used, reset, 0)
		} else {
			appendPercent(s, name, numeric(w["percentage"]), reset, 0)
		}
	}
	parse(object(payload["usage"]), "weekly")
	for i, v := range array(payload["limits"]) {
		w := object(v)
		name := firstText(w, "scope", "name")
		if name == "" {
			name = "limit_" + strconv.Itoa(i+1)
		}
		parse(w, name)
	}
	parse(object(payload["limits"]), "limit")
}

func parseMiniMax(s *quota.Status, payload map[string]any, now time.Time) {
	if code := numeric(object(payload["base_resp"])["status_code"]); code != nil && *code != 0 {
		s.UnavailableReason = "no_subscription"
		return
	}
	for _, v := range array(payload["model_remains"]) {
		m := object(v)
		model := textValue(m["model_name"])
		if model == "" {
			continue
		}
		for _, w := range []struct {
			name, prefix, reset string
			seconds             int64
		}{{"session", "current_interval_", "end_time", 18000}, {"weekly", "current_weekly_", "weekly_end_time", 604800}} {
			reset := resetTime(m[w.reset], now)
			name := model + "_" + w.name
			before := len(s.Windows)
			if remaining := numeric(m[w.prefix+"remaining_percent"]); remaining != nil {
				v := 100 - *remaining
				appendPercent(s, name, &v, reset, w.seconds)
			} else {
				appendWindow(s, name, "tokens", numeric(m[w.prefix+"total_count"]), numeric(m[w.prefix+"usage_count"]), reset, w.seconds)
			}
			if len(s.Windows) > before && model != "general" {
				s.Windows[len(s.Windows)-1].Model = model
				s.Windows[len(s.Windows)-1].Informational = true
			}
		}
	}
}

func parseAnthropic(s *quota.Status, payload map[string]any, now time.Time) {
	for _, name := range []string{"five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet"} {
		w := object(payload[name])
		seconds := int64(604800)
		if name == "five_hour" {
			seconds = 18000
		}
		before := len(s.Windows)
		appendPercent(s, name, numeric(w["utilization"]), resetTime(w["resets_at"], now), seconds)
		if len(s.Windows) > before && strings.HasPrefix(name, "seven_day_") {
			s.Windows[len(s.Windows)-1].Model = strings.TrimPrefix(name, "seven_day_")
			s.Windows[len(s.Windows)-1].Informational = true
		}
	}
	for _, v := range array(payload["limits"]) {
		w := object(v)
		kind := textValue(w["kind"])
		if kind != "session" && kind != "weekly_all" && kind != "weekly_scoped" {
			continue
		}
		name := kind
		model := object(object(w["scope"])["model"])
		if kind == "weekly_scoped" {
			name = firstText(model, "id", "display_name") + "_weekly"
		}
		before := len(s.Windows)
		appendPercent(s, name, numeric(w["percent"]), resetTime(w["resets_at"], now), 0)
		if len(s.Windows) > before && kind == "weekly_scoped" {
			s.Windows[len(s.Windows)-1].Model = firstText(model, "id", "display_name")
			s.Windows[len(s.Windows)-1].Informational = true
		}
	}
	extra := object(payload["extra_usage"])
	if extra["is_enabled"] == true {
		unit := firstText(extra, "currency")
		if unit == "" {
			unit = "USD"
		}
		appendWindow(s, "extra_usage", unit, numeric(extra["monthly_limit"]), numeric(extra["used_credits"]), nil, 0)
	}
}
