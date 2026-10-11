package api

import (
	"context"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"net/http"
	"testing"
	"time"
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

func TestCodexInferenceQuotaHeadersSharedAcrossModelPickerAndRouters(t *testing.T) {
	h, _ := NewOpenAIHandler(nil)
	h.SetQuotaCache(quota.NewCache(time.Minute, nil))
	headers := http.Header{}
	headers.Set("X-Codex-Primary-Used-Percent", "25")
	headers.Set("X-Codex-Primary-Window-Minutes", "300")
	headers.Set("X-Codex-Primary-Reset-At", "1900000000")
	headers.Set("X-Codex-Secondary-Used-Percent", "90")
	headers.Set("X-Codex-Secondary-Reset-At", "1900100000")
	h.recordQuotaHeaders("codex/model-a", headers)
	child, err := h.NewRouter(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child.SetQuotaCache(quota.NewCache(time.Minute, nil))
	for _, handler := range []*OpenAIHandler{h, child} {
		s, err := handler.ModelQuota(context.Background(), "codex/model-b")
		if err != nil || !s.Known || s.FractionRemaining != .1 || len(s.Windows) != 2 || s.ResetAt.Unix() != 1900100000 {
			t.Fatalf("shared inference quota %+v %v", s, err)
		}
	}
}
