package api_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/router"
)

func TestCustomRouterDiscoveryScopeAndRetry(t *testing.T) {
	p := &safetyProvider{models: []string{"a", "b"}, statuses: map[string]int{"b": 500}}
	h := safetyHandler(t, p, safetyModel("a", false), safetyModel("b", false))
	h.SetCustomRouters([]router.CustomRouter{{Name: "smart-team", Models: []string{"safety/b"}}})
	for _, path := range []string{"/v1/models", "/v1/models/smart-team"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"smart-team"`) {
			t.Fatalf("discovery: %d %s", w.Code, w.Body.String())
		}
	}
	w := safetyPost(h, strings.Replace(safetyGreeting, `"smart"`, `"smart-team"`, 1))
	if w.Code == 200 || len(p.calls) != 1 || p.calls[0] != "b" {
		t.Fatalf("restricted retry escaped pool: status=%d calls=%v", w.Code, p.calls)
	}
}

func TestCustomRouterRechecksSessionPinAndDisabledModels(t *testing.T) {
	p := &safetyProvider{models: []string{"a", "b"}}
	h := safetyHandler(t, p, safetyModel("a", false), safetyModel("b", false))
	if w := safetyPost(h, safetyGreeting); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	h.SetCustomRouters([]router.CustomRouter{{Name: "smart-team", Models: []string{"safety/b"}}})
	w := safetyPost(h, strings.Replace(safetyToolOutput, `"smart"`, `"smart-team"`, 1))
	if w.Code == 200 || len(p.calls) != 1 {
		t.Fatalf("tool pin escaped pool: status=%d calls=%v", w.Code, p.calls)
	}
	w = safetyPost(h, strings.Replace(safetyGreeting, `"smart"`, `"smart-team"`, 1))
	if w.Code != 200 || p.calls[len(p.calls)-1] != "b" {
		t.Fatalf("ordinary pin escaped pool: status=%d calls=%v", w.Code, p.calls)
	}
	h.SetModelControls(map[string]bool{"safety/b": false}, nil)
	w = safetyPost(h, strings.Replace(safetyGreeting, `"smart"`, `"smart-team"`, 1))
	if w.Code == 200 || len(p.calls) != 2 {
		t.Fatalf("disabled restricted model dispatched: status=%d calls=%v", w.Code, p.calls)
	}
}

func TestCustomRouterPricingGates(t *testing.T) {
	p := &safetyProvider{models: []string{"a"}}
	h := safetyHandler(t, p, safetyModel("a", false))
	h.SetCustomRouters([]router.CustomRouter{{Name: "smart-team", Providers: []string{"safety"}, AllowFreeModels: safetyPtr(false)}})
	w := safetyPost(h, strings.Replace(safetyGreeting, `"smart"`, `"smart-team"`, 1))
	if w.Code == 200 || len(p.calls) != 0 {
		t.Fatal("free pricing gate ignored")
	}
}
