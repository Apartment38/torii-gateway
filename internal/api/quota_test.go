package api

import (
	"context"
	"encoding/json"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"net/http"
	"net/http/httptest"
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

func TestSharedQuotaHeadersPreserveBillingInModelDiscovery(t *testing.T) {
	h, _ := NewOpenAIHandler([]provider.OpenAIProvider{&controlledProvider{}})
	h.SetQuotaCache(quota.NewCache(time.Minute, nil))
	left, spend, resets := 8.75, 2.0, 3.0
	deadline := time.Now().Add(10 * time.Second)
	if err := h.quota.Set("p/model", quota.Status{Balances: []quota.Balance{{Name: "credits", Unit: "USD", Remaining: &left, ResetAt: &deadline}}, Usage: &quota.Spend{Monthly: &spend}, Plan: "test-plan", ResetCredits: &resets}); err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set("X-Codex-Primary-Used-Percent", "25")
	headers.Set("X-Codex-Primary-Reset-At", "1900000000")
	headers.Set("X-Codex-Credits-Balance", "9")
	h.recordQuotaHeaders("p/new", headers)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	var catalog struct {
		Data []clientModel `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	for _, model := range catalog.Data {
		if model.ID != "p/model" {
			continue
		}
		s := model.Quota
		if s == nil || !s.Known || len(s.Windows) != 1 || len(s.Balances) != 1 || *s.Balances[0].Remaining != left || s.Usage == nil || *s.Usage.Monthly != spend || s.Plan != "test-plan" || s.ResetCredits == nil || *s.ResetCredits != resets || s.ExpiresAt.After(deadline) {
			t.Fatalf("discovery dropped billing or shared windows: %+v", s)
		}
		return
	}
	t.Fatal("model missing from discovery")
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
