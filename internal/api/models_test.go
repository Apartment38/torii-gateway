package api

import (
	"context"
	"encoding/json"
	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/router"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestModelDiscoveryTrackingIsScopedAndConsistent(t *testing.T) {
	p := &accountingFixtureProvider{}
	h, store := accountingHandler(t, p, accounting.Settings{})
	for key, tokens := range map[string]int64{"client": 7, "other": 99} {
		r, err := store.Reserve(key, "Private "+key, "p", "p/model", accounting.Settings{}, accounting.Amount{Input: tokens, Cost: float64(tokens) / 100})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Settle(r, nil); err != nil {
			t.Fatal(err)
		}
	}
	c := quota.NewCache(time.Minute, nil)
	reset := time.Now().UTC().Add(time.Hour)
	if err := c.Set("p/model", quota.Status{Windows: []quota.Window{{Name: "weekly", Unit: "percent", Limit: 100, Used: 25, ResetAt: &reset}}, Source: "fixture"}); err != nil {
		t.Fatal(err)
	}
	h.SetQuotaCache(c)
	for _, path := range []string{"/v1/models", "/v1/models/p/model"} {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(context.WithValue(req.Context(), clientIdentityContextKey{}, &ClientIdentity{ID: "client"}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var model clientModel
		if path == "/v1/models" {
			var list struct {
				Data []clientModel `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			for _, m := range list.Data {
				if m.ID == "p/model" {
					model = m
				}
			}
		} else {
			json.Unmarshal(w.Body.Bytes(), &model)
		}
		if model.Pricing == nil || *model.Pricing.InputPerMillion != 0 || model.Quota == nil || model.Quota.Remaining != 75 || model.Quota.ResetAt == nil || !model.UsageAvailable || len(model.Usage) != 2 {
			t.Fatalf("model=%+v", model)
		}
		for _, row := range model.Usage {
			if row.Input != 7 {
				t.Fatal("usage scope leak", row)
			}
		}
		if strings.Contains(w.Body.String(), "Private") || strings.Contains(w.Body.String(), "other") {
			t.Fatal("private metadata leaked")
		}
	}
	w := httptest.NewRecorder()
	h.ManagementModels(w, httptest.NewRequest("GET", "/gateway/v1/models", nil))
	var models struct {
		Models []ManagedModel `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &models); err != nil {
		t.Fatal(err)
	}
	for _, m := range models.Models {
		if m.PublicID == "p/model" {
			if len(m.Usage) != 2 || m.Usage[0].Input != 106 || m.Quota.Remaining != 75 {
				t.Fatal("management totals", m)
			}
		}
	}
}

func TestModelDiscoveryUnknownTrackingRemainsExplicit(t *testing.T) {
	h, _ := NewOpenAIHandler([]provider.OpenAIProvider{&controlledProvider{}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models/p/model", nil))
	var m clientModel
	json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 200 || m.Pricing == nil || m.Pricing.InputPerMillion != nil || m.Quota == nil || m.Quota.Known || m.Quota.Exhausted || m.UsageAvailable {
		t.Fatalf("unknown model=%+v body=%s", m, w.Body.String())
	}
}
