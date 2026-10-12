package api_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

func TestManualPricingFeedsRoutingNamedRoutersAndAccounting(t *testing.T) {
	p := &criticProvider{models: []string{"a", "b"}}
	a, b := criticModel("a"), criticModel("b")
	b.Pricing = catalog.Pricing{InputPerMillion: criticPtr(1.), OutputPerMillion: criticPtr(1.)}
	h := criticHandler(t, p, a, b)
	w := criticPost(h, "/v1/chat/completions", criticGreeting)
	if w.Code != 200 || p.calls[0] != "b" {
		t.Fatal("baseline known pricing did not win")
	}
	h.SetModelPricing(map[string]catalog.Pricing{"critic/a": {InputPerMillion: criticPtr(.01), OutputPerMillion: criticPtr(.02)}})
	// A new conversation avoids deterministic affinity from the baseline request.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("DELETE", "/gateway/v1/sessions/critic-session", nil))
	w = criticPost(h, "/v1/chat/completions", criticGreeting)
	if w.Code != 200 || p.calls[1] != "a" {
		t.Fatalf("manual price did not inform routing: %d %s calls=%v", w.Code, w.Body.String(), p.calls)
	}
	d := h.AccountingDescriptor("critic/a")
	if *d.Pricing.InputPerMillion != .01 || *d.Pricing.OutputPerMillion != .02 {
		t.Fatal("accounting ignored manual prices")
	}
	// Manual prices apply even when the named router has a separate catalog.
	a.Pricing = catalog.Pricing{InputPerMillion: criticPtr(100.), OutputPerMillion: criticPtr(100.)}
	child, err := h.NewRouter([]provider.OpenAIProvider{p}, []catalog.Descriptor{a, b})
	if err != nil {
		t.Fatal(err)
	}
	child.SetRouterScope("named", nil, nil)
	h.AddRouter("named", child)
	w = criticPost(h, "/v1/chat/completions", `{"model":"named","messages":[{"role":"user","content":"hi"}]}`)
	if w.Code != 200 || p.calls[2] != "a" {
		t.Fatalf("named router ignored global prices: %d %s", w.Code, w.Body.String())
	}
	models := h.CatalogModels()
	*models[0].Pricing.InputPerMillion = 999
	if *h.CatalogModels()[0].Pricing.InputPerMillion != .01 {
		t.Fatal("manual rates exposed mutable pointers")
	}
	h.SetModelControls(nil, map[string]bool{"critic": true})
	if *h.CatalogModels()[0].Pricing.InputPerMillion != 0 || h.CatalogModels()[0].Pricing.Source != "operator" {
		t.Fatal("manual prices bypassed explicit free-provider policy")
	}
}
func TestPartialManualPricingPreservesAutomaticRates(t *testing.T) {
	p := &criticProvider{models: []string{"a"}}
	a := criticModel("a")
	a.Pricing.InputPerMillion = criticPtr(2.)
	h := criticHandler(t, p, a)
	h.SetModelPricing(map[string]catalog.Pricing{"critic/a": {OutputPerMillion: criticPtr(3.)}})
	if err := h.RefreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := h.AccountingDescriptor("critic/a")
	if *d.Pricing.InputPerMillion != 2 || *d.Pricing.OutputPerMillion != 3 || d.Pricing.Source != "operator" {
		t.Fatal("partial override lost automatic price")
	}
	h.SetModelPricing(nil)
	d = h.AccountingDescriptor("critic/a")
	if *d.Pricing.InputPerMillion != 2 || d.Pricing.OutputPerMillion != nil {
		t.Fatal("clearing rates did not restore automatic metadata")
	}
	// Classifier models outside the inference catalog also use manual prices.
	h.SetModelPricing(map[string]catalog.Pricing{"jev/checkpoint": {InputPerMillion: criticPtr(.3), OutputPerMillion: criticPtr(.6)}})
	d = h.AccountingDescriptor("jev/checkpoint")
	if *d.Pricing.OutputPerMillion != .6 {
		t.Fatal("classifier accounting ignored manual prices")
	}
}
