package api

import (
	"net/http"
	"testing"
)

func TestFirstHeaderNumber(t *testing.T) {
	headers := http.Header{"X-Ratelimit-Limit-Requests": []string{"100"}, "X-Ratelimit-Remaining-Requests": []string{"42, 41"}}
	if value := firstHeaderNumber(headers, "X-RateLimit-Limit-Requests"); value == nil || *value != 100 {
		t.Fatalf("limit = %v", value)
	}
	if value := firstHeaderNumber(headers, "X-RateLimit-Remaining-Requests"); value == nil || *value != 42 {
		t.Fatalf("remaining = %v", value)
	}
}
