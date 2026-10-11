package config_test

import (
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/router"
	"gopkg.in/yaml.v3"
)

func TestCustomRouterSettingsRoundTrip(t *testing.T) {
	var cfg config.RouterConfig
	if err := yaml.Unmarshal([]byte("allow_free_models: false\ncustom_routers:\n  - name: smart-team\n    models: [cloud/vendor/model]\n    allow_paid_models: false\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var next config.RouterConfig
	if err := yaml.Unmarshal(encoded, &next); err != nil {
		t.Fatal(err)
	}
	if next.AllowFreeModels == nil || *next.AllowFreeModels || next.CustomRouters[0].AllowPaidModels == nil || *next.CustomRouters[0].AllowPaidModels || next.CustomRouters[0].AllowFreeModels != nil {
		t.Fatalf("pricing defaults lost: %#v", next)
	}
}

func TestCustomRouterSettingsRejectInvalidPools(t *testing.T) {
	for _, custom := range []router.CustomRouter{
		{Name: "smart", Providers: []string{"p"}},
		{Name: "p/router", Providers: []string{"p"}},
		{Name: "team"},
		{Name: "team", Models: []string{"unqualified"}},
		{Name: "team", Providers: []string{"Bad Provider"}},
	} {
		if err := (config.RouterConfig{CustomRouters: []router.CustomRouter{custom}}).Validate(); err == nil {
			t.Fatalf("accepted invalid router: %#v", custom)
		}
	}
	custom := router.CustomRouter{Name: "team", Providers: []string{"p"}}
	if err := (config.RouterConfig{CustomRouters: []router.CustomRouter{custom, custom}}).Validate(); err == nil {
		t.Fatal("accepted duplicate names")
	}
}
