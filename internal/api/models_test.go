package api

import (
	"context"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type controlledProvider struct{ calls int }

func (*controlledProvider) ID() string { return "p" }
func (*controlledProvider) Models(context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "p/model"}, {ID: "p/new"}}, nil
}
func (p *controlledProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	p.calls++
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"choices":[]}`))}, nil
}
func TestDisabledModelsExcludeDiscoverySmartAndDirect(t *testing.T) {
	p := &controlledProvider{}
	h, err := NewOpenAIHandler([]provider.OpenAIProvider{p})
	if err != nil {
		t.Fatal(err)
	}
	h.SetModelControls(map[string]bool{"p/model": false, "p/new": false}, nil)
	for _, body := range []string{`{"model":"p/model","messages":[]}`, `{"model":"smart","messages":[]}`} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body)))
		if w.Code == 200 || p.calls != 0 {
			t.Fatal("disabled upstream dispatched", w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	if strings.Contains(w.Body.String(), "p/model") || !strings.Contains(w.Body.String(), "smart") {
		t.Fatal("disabled discovery", w.Body.String())
	}
	h.SetModelControls(map[string]bool{"p/model": false}, nil)
	h.RefreshCatalog(context.Background())
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	if !strings.Contains(w.Body.String(), "p/new") {
		t.Fatal("new models not enabled")
	}
}
func TestFreePricingOverridesPreservePaidMetadataWhenOff(t *testing.T) {
	input, output, local := float64(2), float64(4), true
	p := &controlledProvider{}
	h, err := NewOpenAIHandlerWithOverrides([]provider.OpenAIProvider{p}, []catalog.Descriptor{{PublicID: "p/model", Local: &local, Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Pricing: catalog.Pricing{InputPerMillion: &input, OutputPerMillion: &output}}})
	if err != nil {
		t.Fatal(err)
	}
	h.RefreshCatalog(context.Background())
	h.SetModelControls(nil, map[string]bool{"p": true})
	policy := router.DefaultSelectionPolicy()
	policy.AllowPaidModels = false
	selectControlled := func() error {
		for _, m := range h.CatalogModels() {
			if m.PublicID == "p/model" {
				_, err := router.Select([]router.Candidate{{Model: m}}, router.RequestRequirements{Endpoint: router.EndpointChat}, "low", policy)
				return err
			}
		}
		t.Fatal("missing controlled model")
		return nil
	}
	if err := selectControlled(); err != nil {
		t.Fatalf("free provider ineligible: %v", err)
	}
	if *h.CatalogModels()[0].Pricing.InputPerMillion != 0 {
		t.Fatal("free provider not zero")
	}
	if cost := h.actualCost("p/model", 1000000, 1000000); cost != 0 {
		t.Fatalf("free cost=%v", cost)
	}
	h.SetModelControls(nil, nil)
	if err := selectControlled(); err == nil {
		t.Fatal("local paid model eligible with free flag off")
	}
	if cost := h.actualCost("p/model", 1000000, 1000000); cost != 6 {
		t.Fatalf("local pricing ignored: %v", cost)
	}
	for _, m := range h.CatalogModels() {
		if m.PublicID == "p/model" && *m.Pricing.InputPerMillion != 2 {
			t.Fatal("free flag destroyed override")
		}
	}
}
