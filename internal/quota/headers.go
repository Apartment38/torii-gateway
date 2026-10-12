package quota

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FromHeaders keeps request, token and subscription windows separate. A missing
// limit/remaining pair never becomes an invented balance.
func FromHeaders(headers http.Header, now time.Time) (Status, bool) {
	s := Status{Source: "response_headers"}
	var codexWindows []Window
	for _, unit := range []string{"requests", "tokens"} {
		if headers.Get("Anthropic-RateLimit-"+unit+"-Limit") != "" {
			limit, lOK := number(headers.Get("Anthropic-RateLimit-" + unit + "-Limit"))
			remaining, rOK := number(headers.Get("Anthropic-RateLimit-" + unit + "-Remaining"))
			if lOK && rOK && limit >= 0 && remaining >= 0 {
				s.Windows = append(s.Windows, Window{Name: "anthropic_" + unit, Unit: unit, Limit: limit, Used: math.Max(0, limit-remaining), ResetAt: ParseReset(headers.Get("Anthropic-RateLimit-"+unit+"-Reset"), now)})
			}
		}
		prefix := "X-RateLimit-"
		limit, lOK := number(headers.Get(prefix + "Limit-" + unit))
		remaining, rOK := number(headers.Get(prefix + "Remaining-" + unit))
		if !lOK || !rOK || limit < 0 || remaining < 0 {
			continue
		}
		s.Windows = append(s.Windows, Window{Name: unit, Unit: unit, Limit: limit, Used: math.Max(0, limit-remaining), ResetAt: ParseReset(headers.Get(prefix+"Reset-"+unit), now)})
	}
	for _, name := range []string{"primary", "secondary"} {
		prefix := "X-Codex-" + name + "-"
		used, ok := number(headers.Get(prefix + "Used-Percent"))
		if !ok || used < 0 {
			continue
		}
		var reset *time.Time
		reset = ParseReset(headers.Get(prefix+"Reset-At"), now)
		if seconds, ok := number(headers.Get(prefix + "Reset-After-Seconds")); reset == nil && ok && seconds >= 0 && seconds <= 31536000 {
			t := now.Add(time.Duration(seconds) * time.Second)
			reset = &t
		}
		window := Window{Name: name, Unit: "percent", Limit: 100, Used: used, ResetAt: reset}
		if minutes, ok := number(headers.Get(prefix + "Window-Minutes")); ok && minutes > 0 && minutes*60 < float64(math.MaxInt64) {
			window.WindowSeconds = int64(minutes * 60)
		}
		window.WindowType = WindowType(window.WindowSeconds)
		window.Slot = name
		window.State = "available"
		window.Source = s.Source
		window.LastUpdated = now
		s.Windows = append(s.Windows, window)
		codexWindows = append(codexWindows, window)
	}
	if len(codexWindows) > 0 {
		s.Provider = "openai-codex"
		s.State = "available"
		s.CollectedAt = now
		groupState := "available"
		for _, w := range codexWindows {
			if w.Used >= 100 {
				groupState = "exhausted"
			}
		}
		s.Limits = []LimitGroup{{Scope: "unknown", State: groupState, Windows: codexWindows}}
	}
	if reset := ParseReset(headers.Get("X-Sc-Budget-Renews-At"), now); reset != nil {
		s.Source = "standard_compute_headers"
		s.ResetAt = reset
	}
	if headers.Get("X-Codex-Credits-Unlimited") == "true" {
		s.Balances = append(s.Balances, Balance{Name: "credits", Unit: "USD", Unlimited: true})
	} else if n, ok := number(headers.Get("X-Codex-Credits-Balance")); ok {
		s.Balances = append(s.Balances, Balance{Name: "credits", Unit: "USD", Remaining: &n})
	}
	return s, len(s.Windows) > 0 || s.ResetAt != nil || len(s.Balances) > 0
}

func number(raw string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.Split(raw, ",")[0]), 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// ParseReset accepts RFC3339, HTTP dates, relative Go durations, and Unix seconds
// or milliseconds. Small numeric values are relative seconds.
func ParseReset(raw string, now time.Time) *time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339Nano, http.TimeFormat} {
		if t, err := time.Parse(layout, raw); err == nil {
			t = t.UTC()
			return &t
		}
	}
	if duration, err := time.ParseDuration(raw); err == nil && duration >= 0 {
		t := now.Add(duration).UTC()
		return &t
	}
	if n, ok := number(raw); ok && n >= 0 && n <= 253402300799000 {
		var t time.Time
		switch {
		case n >= 1e12:
			t = time.UnixMilli(int64(n))
		case n >= 1e9 && n <= 253402300799:
			t = time.Unix(int64(n), 0)
		case n <= 31536000:
			t = now.Add(time.Duration(n) * time.Second)
		default:
			return nil
		}
		t = t.UTC()
		return &t
	}
	return nil
}
