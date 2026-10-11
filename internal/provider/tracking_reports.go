package provider

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/quota"
)

func (p *trackedProvider) xaiSpend(ctx context.Context, s *quota.Status) {
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	body := map[string]any{"analyticsRequest": map[string]any{"timeRange": map[string]any{"startTime": day.AddDate(0, 0, -30).Format("2006-01-02 15:04:05"), "endTime": now.Format("2006-01-02 15:04:05"), "timezone": "Etc/GMT"}, "timeUnit": "TIME_UNIT_DAY", "values": []any{map[string]any{"name": "usd", "aggregation": "AGGREGATION_SUM"}}, "groupBy": []any{}, "filters": []any{}}}
	endpoint := strings.TrimRight(p.tracking.BaseURL, "/") + "/v1/billing/teams/" + url.PathEscape(p.tracking.TeamID) + "/usage"
	payload, err := trackingJSON(ctx, p.tracking, http.MethodPost, endpoint, body)
	if err != nil || payload["limitReached"] == true {
		s.Partial = true
		s.UnavailableReason = "spend_unavailable"
		return
	}
	totals := map[string]float64{"daily": 0, "weekly": 0, "monthly": 0}
	valid := false
	for _, v := range array(payload["timeSeries"]) {
		for _, row := range array(object(v)["dataPoints"]) {
			m := object(row)
			t := resetTime(m["timestamp"], now)
			values := array(m["values"])
			if t == nil || len(values) == 0 {
				continue
			}
			n := numeric(values[0])
			if n == nil || *n < 0 || t.After(now) {
				continue
			}
			valid = true
			for period, start := range periodStarts(now) {
				if !t.Before(start) {
					totals[period] += *n
				}
			}
		}
	}
	if valid {
		s.Usage = &quota.Spend{Daily: floatPointer(totals["daily"]), Weekly: floatPointer(totals["weekly"]), Monthly: floatPointer(totals["monthly"])}
	} else {
		s.Partial = true
		s.UnavailableReason = "spend_unavailable"
	}
}
func floatPointer(v float64) *float64 { return &v }
func periodStarts(now time.Time) map[string]time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return map[string]time.Time{"daily": day, "weekly": day.AddDate(0, 0, -(int(day.Weekday())+6)%7), "monthly": time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)}
}

func (p *trackedProvider) anthropicReports(ctx context.Context, s quota.Status) (quota.Status, error) {
	if p.tracking.Headers.Get("X-Api-Key") == "" {
		s.UnavailableReason = "admin_credentials_required"
		return s, nil
	}
	now := time.Now().UTC()
	starts := periodStarts(now)
	for _, report := range []struct {
		path, unit string
		cost       bool
	}{{"cost_report", "USD", true}, {"usage_report/messages", "tokens", false}} {
		start := starts["monthly"]
		if starts["weekly"].Before(start) {
			start = starts["weekly"]
		}
		query := url.Values{"starting_at": {start.Format(time.RFC3339)}, "ending_at": {now.Format(time.RFC3339)}, "bucket_width": {"1d"}, "limit": {"31"}}
		totals := map[string]float64{"daily": 0, "weekly": 0, "monthly": 0}
		complete := false
		for page := 0; page < 4; page++ {
			endpoint := strings.TrimRight(p.tracking.BaseURL, "/") + "/v1/organizations/" + report.path + "?" + query.Encode()
			payload, err := trackingJSON(ctx, p.tracking, http.MethodGet, endpoint, nil)
			if err != nil {
				s.Partial = true
				s.UnavailableReason = trackingReason(err)
				break
			}
			rows, ok := payload["data"].([]any)
			if !ok {
				s.Partial = true
				s.UnavailableReason = "invalid_response"
				break
			}
			for _, row := range rows {
				bucket := object(row)
				stamp := resetTime(bucket["starting_at"], now)
				if stamp == nil {
					continue
				}
				for _, v := range array(bucket["results"]) {
					m := object(v)
					amount := 0.0
					if report.cost {
						n := numeric(m["amount"])
						if n == nil || *n < 0 {
							continue
						}
						currency := textValue(m["currency"])
						if currency != "" && !strings.EqualFold(currency, "USD") {
							continue
						}
						amount = *n / 100
					} else {
						for _, field := range []string{"input_tokens", "uncached_input_tokens", "cache_read_input_tokens", "output_tokens"} {
							if n := numeric(m[field]); n != nil && *n >= 0 {
								amount += *n
							}
						}
						for _, v := range object(m["cache_creation"]) {
							if n := numeric(v); n != nil && *n >= 0 {
								amount += *n
							}
						}
					}
					for period, start := range starts {
						if !stamp.Before(start) && !stamp.After(now) {
							totals[period] += amount
						}
					}
				}
			}
			if payload["has_more"] != true {
				complete = true
				break
			}
			next := textValue(payload["next_page"])
			if next == "" || next == query.Get("page") {
				break
			}
			query.Set("page", next)
		}
		if !complete {
			s.Partial = true
			if s.UnavailableReason == "" {
				s.UnavailableReason = "report_incomplete"
			}
			continue
		}
		for _, period := range []string{"daily", "weekly", "monthly"} {
			s.ReportedUsage = append(s.ReportedUsage, quota.ReportedUsage{Period: period, Unit: report.unit, Used: totals[period], Scope: "organization"})
		}
		if report.cost {
			s.Usage = &quota.Spend{Daily: floatPointer(totals["daily"]), Weekly: floatPointer(totals["weekly"]), Monthly: floatPointer(totals["monthly"])}
		}
	}
	return s, nil
}
