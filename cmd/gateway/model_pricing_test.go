package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
