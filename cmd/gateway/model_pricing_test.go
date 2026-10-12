package main

import (
	"encoding/json"
	"fmt"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
)

func TestManualModelPricesPersistWithRevisionChecks(t *testing.T) {
	r, db := credentialRuntime(t)
	revision := currentConfigRevision(t, r)
	update := func(body, revision string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PUT", "/gateway/v1/models", strings.NewReader(body))
		req.Header.Set("If-Match", revision)
		w := httptest.NewRecorder()
		r.setModel(w, req)
		return w
	}
	w := update(`{"id":"p/model","pricing":{"input_per_million":0.25,"output_per_million":1.5}}`, revision)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cfg, err := loadDatabaseSettings(db)
	if err != nil || *cfg.ModelPricing["p/model"].OutputPerMillion != 1.5 {
		t.Fatalf("price lost in durable settings: %+v %v", cfg, err)
	}
	if w := update(`{"id":"p/model","pricing":{}}`, revision); w.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale pricing edit accepted: %d", w.Code)
	}
	if w := update(`{"id":"p/model","pricing":{"input_per_million":-1}}`, currentConfigRevision(t, r)); w.Code != 400 {
		t.Fatal("negative rate accepted")
	}
	cfg, err = r.loadConfig()
	if err != nil || *cfg.ModelPricing["p/model"].InputPerMillion != .25 {
		t.Fatal("invalid edit changed saved rates")
	}
	if w := update(`{"id":"p/model","pricing":{}}`, currentConfigRevision(t, r)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cfg, err = loadDatabaseSettings(db)
	if err != nil || len(cfg.ModelPricing) != 0 {
		t.Fatal("automatic pricing not restored")
	}
}
func TestPricingCSVParseAndTemplateRequireAdmin(t *testing.T) {
	r, _ := credentialRuntime(t)
	revision := currentConfigRevision(t, r)
	for _, path := range []string{"/gateway/v1/models/pricing/template.csv", "/gateway/v1/models/pricing/parse"} {
		method := "GET"
		if strings.HasSuffix(path, "parse") {
			method = "POST"
		}
		req := httptest.NewRequest(method, path, strings.NewReader(config.PricingCSVTemplate))
		req.Header.Set("Authorization", "Bearer admin-sentinel")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("inference key allowed pricing management: %s %d", path, w.Code)
		}
		req = httptest.NewRequest(method, path, strings.NewReader(config.PricingCSVTemplate))
		req.SetBasicAuth("chosen-admin", "administrator-password")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("administrator could not use %s: %d %s", path, w.Code, w.Body.String())
		}
		if method == "GET" {
			if w.Body.String() != config.PricingCSVTemplate {
				t.Fatal("wrong pricing template")
			}
		} else {
			var result map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["model_pricing"] == nil {
				t.Fatal("parsed rates missing")
			}
		}
	}
	if currentConfigRevision(t, r) != revision {
		t.Fatal("CSV validation changed configuration before saving")
	}
}

func TestPricingRefreshRequiresAdminAndDoesNotChangeSettings(t *testing.T) {
	r, _ := credentialRuntime(t)
	revision := currentConfigRevision(t, r)
	const path = "/gateway/v1/models/pricing/refresh"
	req := httptest.NewRequest("POST", path, nil)
	req.Header.Set("Authorization", "Bearer admin-sentinel")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("inference key refreshed pricing: %d", w.Code)
	}
	req = httptest.NewRequest("POST", path, nil)
	req.SetBasicAuth("chosen-admin", "administrator-password")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result struct {
		Models []json.RawMessage `json:"models"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Models) != 1 {
		t.Fatalf("refresh failed: %d %s", w.Code, w.Body.String())
	}
	if currentConfigRevision(t, r) != revision {
		t.Fatal("refresh modified settings")
	}
}

func TestPricingRefreshPullsLatestProviderRatesAndRetainsManualOverrides(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		count := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"id":"model","pricing":{"prompt":"0.00000%d","completion":"0.000002"}},{"id":"manual","pricing":{"prompt":"0.000003","completion":"0.000004"}}]}`, count)
	}))
	defer upstream.Close()
	r, _ := credentialRuntime(t)
	cfg, err := r.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	discover := true
	cfg.Providers[0].DiscoverModels = &discover
	cfg.Providers[0].BaseURL = upstream.URL
	manual := 9.0
	cfg.ModelPricing = map[string]catalog.Pricing{"p/manual": {InputPerMillion: &manual}}
	handler, err := buildHandlerWithRuntime(cfg, r.configPath, r)
	if err != nil {
		t.Fatal(err)
	}
	r.handler = handler
	before := calls.Load()
	req := httptest.NewRequest("POST", "/gateway/v1/models/pricing/refresh", nil)
	req.SetBasicAuth("chosen-admin", "administrator-password")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result struct {
		Models []struct {
			ID      string          `json:"id"`
			Pricing catalog.Pricing `json:"pricing"`
		} `json:"models"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("refresh failed: %d %s", w.Code, w.Body.String())
	}
	if calls.Load() <= before {
		t.Fatal("refresh reused cached catalog")
	}
	for _, model := range result.Models {
		if model.ID == "p/model" && (model.Pricing.UpdatedAt == nil || *model.Pricing.InputPerMillion != float64(calls.Load())) {
			t.Fatalf("latest provider pricing not returned: %+v", model.Pricing)
		}
		if model.ID == "p/manual" && (*model.Pricing.InputPerMillion != 9 || model.Pricing.Source != "operator") {
			t.Fatal("refresh replaced manual pricing")
		}
	}
}
