package router_test

import (
	"testing"

	"github.com/Apartment38/torii-gateway/internal/router"
)

func TestCustomRouterScope(t *testing.T) {
	custom := router.CustomRouter{Name: "smart-team", Providers: []string{"local"}, Models: []string{"cloud/vendor/model"}}
	if err := custom.Validate(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"local/new": true, "cloud/vendor/model": true, "cloud/other": false, "other/new": false, "smart": false} {
		if got := custom.Includes(id); got != want {
			t.Errorf("Includes(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIndependentPricingGates(t *testing.T) {
	free, paid, unknown := model("p/free"), model("p/paid"), model("p/unknown")
	free.Pricing.InputPerMillion, free.Pricing.OutputPerMillion = ptr(0.0), ptr(0.0)
	unknown.Pricing.InputPerMillion, unknown.Pricing.OutputPerMillion = nil, nil
	for _, tc := range []struct {
		paid, free bool
		want string
	}{
		{true, true, "p/free"},
		{false, true, "p/free"},
		{true, false, "p/paid"},
		{false, false, ""},
	} {
		policy := router.DefaultSelectionPolicy()
		policy.AllowPaidModels, policy.AllowFreeModels = tc.paid, tc.free
		decision, err := router.Select([]router.Candidate{{Model: free}, {Model: paid}, {Model: unknown}}, router.RequestRequirements{Endpoint: router.EndpointChat}, "low", policy)
		if tc.want == "" {
			if err == nil {
				t.Fatal("both gates disabled admitted a model")
			}
		} else if err != nil || decision.Model.PublicID != tc.want {
			t.Fatalf("paid=%v free=%v: model=%s err=%v", tc.paid, tc.free, decision.Model.PublicID, err)
		}
	}
}
