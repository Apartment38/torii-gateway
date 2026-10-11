package api_test

import (
	"encoding/json"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNamedRoutersRestrictModelsAndKeepIndependentPolicies(t *testing.T) {
	p := &criticProvider{models: []string{"cheap", "expensive"}}
	cheap, expensive := criticModel("cheap"), criticModel("expensive")
	cheap.Pricing = catalog.Pricing{InputPerMillion: criticPtr(0.), OutputPerMillion: criticPtr(0.)}
	expensive.Pricing = catalog.Pricing{InputPerMillion: criticPtr(10.), OutputPerMillion: criticPtr(10.)}
	h := criticHandler(t, p, cheap, expensive)
	child, err := h.NewRouter([]provider.OpenAIProvider{p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	child.SetRouterScope("paid-only", []string{"critic"}, []string{"critic/expensive"})
	policy := router.DefaultSelectionPolicy()
	policy.AllowFreeModels = criticPtr(false)
	child.SetSelectionPolicy(policy)
	h.AddRouter("paid-only", child)
	for _, name := range []string{"smart", "paid-only", "smart"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+name+`","messages":[{"role":"user","content":"hi"}]}`)))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if strings.Join(p.calls, ",") != "cheap,expensive,cheap" {
		t.Fatalf("routing crossed policies: %v", p.calls)
	}
	child.SetRouterScope("paid-only", []string{"excluded"}, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"paid-only","messages":[{"role":"user","content":"hi"}]}`)))
	if w.Code == 200 || len(p.calls) != 3 {
		t.Fatal("escaped restricted provider pool")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	var result struct {
		Data []provider.Model `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) < 2 || result.Data[0].ID != "smart" || result.Data[1].ID != "paid-only" {
		t.Fatalf("named catalog: %s", w.Body.String())
	}
}
