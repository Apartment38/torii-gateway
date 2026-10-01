package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

type changingCatalogProvider struct{ calls int }

func (p *changingCatalogProvider) ID() string { return "subscription" }
func (p *changingCatalogProvider) Models(context.Context) ([]provider.Model, error) {
	p.calls++
	id := "subscription/fallback"
	if p.calls > 1 {
		id = "subscription/live"
	}
	return []provider.Model{{ID: id, Object: "model"}}, nil
}
func (p *changingCatalogProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	return nil, nil
}

func TestCatalogRetriesAfterCachedFallbackExpires(t *testing.T) {
	p := &changingCatalogProvider{}
	h, err := NewOpenAIHandler([]provider.OpenAIProvider{p})
	if err != nil {
		t.Fatal(err)
	}
	request := func() string {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("models = %d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if !strings.Contains(request(), "subscription/fallback") || p.calls != 1 {
		t.Fatal("initial catalog was not populated")
	}
	_ = request()
	if p.calls != 1 {
		t.Fatal("catalog refreshed before the cache expired")
	}
	h.refreshedAt = time.Now().Add(-2 * time.Minute)
	if body := request(); !strings.Contains(body, "subscription/live") || strings.Contains(body, "subscription/fallback") || p.calls != 2 {
		t.Fatalf("catalog remained stuck on fallback: %s", body)
	}
}
