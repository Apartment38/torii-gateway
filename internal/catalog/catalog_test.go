package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

type stubProvider struct {
	id     string
	models []provider.Model
	err    error
}

func (p *stubProvider) ID() string { return p.id }
func (p *stubProvider) Models(context.Context) ([]provider.Model, error) {
	return append([]provider.Model(nil), p.models...), p.err
}

func boolPointer(value bool) *bool        { return &value }
func intPointer(value int64) *int64       { return &value }
func floatPointer(value float64) *float64 { return &value }

func TestCatalogMergesDeterministicallyAndSorts(t *testing.T) {
	local := &stubProvider{id: "local", models: []provider.Model{{ID: "local/z", Created: 3}, {ID: "local/a", OwnedBy: "owner"}}}
	window := int64(8192)
	bundled := []catalog.Descriptor{{
		PublicID: "local/a", ContextWindow: &window,
		Capabilities: catalog.ModelCapabilities{Tools: catalog.Supported},
		Pricing:      catalog.Pricing{InputPerMillion: floatPointer(1)},
	}}
	overrides := []catalog.Descriptor{{
		PublicID: "local/a", ContextWindow: intPointer(16384),
		Capabilities: catalog.ModelCapabilities{Tools: catalog.Unsupported, Vision: catalog.Supported},
		Local:        boolPointer(true),
	}}
	c, err := catalog.New([]provider.Provider{local}, bundled, overrides)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	models := c.Models()
	if len(models) != 2 || models[0].PublicID != "local/a" || models[1].PublicID != "local/z" {
		t.Fatalf("models = %#v", models)
	}
	model := models[0]
	if *model.ContextWindow != 16384 || model.Capabilities.Tools != catalog.Unsupported || model.Capabilities.Vision != catalog.Supported || model.Capabilities.Reasoning != catalog.Unknown {
		t.Fatalf("merged model = %#v", model)
	}
	if model.Pricing.InputPerMillion == nil || *model.Pricing.InputPerMillion != 1 || model.Metadata.CapabilitySource != "config" {
		t.Fatalf("merged metadata = %#v", model)
	}
}

func TestBundledCatalogHasCurrentCoreModels(t *testing.T) {
	records, err := catalog.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("bundled catalog is empty")
	}
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if record.PublicID == "" || record.ProviderID == "" || record.UpstreamID == "" {
			t.Fatalf("record has incomplete identity: %#v", record)
		}
		if seen[record.PublicID] {
			t.Fatalf("duplicate bundled model %q", record.PublicID)
		}
		seen[record.PublicID] = true
		if record.Quality.General != nil || record.Quality.Tools != nil || record.Quality.Reasoning != nil {
			t.Fatalf("bundled model %q contains an unverified quality score", record.PublicID)
		}
	}
	for _, want := range []string{
		"openai-codex/gpt-5.3-codex",
		"anthropic/claude-opus-5-5",
		"gemini/gemini-3.8-flash",
		"xai/grok-4.7",
	} {
		if !seen[want] {
			t.Fatalf("bundled catalog is missing %q", want)
		}
	}
}

func TestCatalogPartialRefreshAndLastKnownGood(t *testing.T) {
	good := &stubProvider{id: "good", models: []provider.Model{{ID: "good/model"}}}
	flaky := &stubProvider{id: "flaky", models: []provider.Model{{ID: "flaky/model"}}}
	c, err := catalog.New([]provider.Provider{good, flaky}, []catalog.Descriptor{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	flaky.err = errors.New("offline")
	good.models = []provider.Model{{ID: "good/new"}}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	models := c.Models()
	if len(models) != 2 || models[0].PublicID != "flaky/model" || !models[0].Availability.Stale || models[1].PublicID != "good/new" {
		t.Fatalf("models after partial refresh = %#v", models)
	}
}

func TestCatalogUsesBundledFallback(t *testing.T) {
	offline := &stubProvider{id: "local", err: errors.New("offline")}
	bundled := []catalog.Descriptor{{PublicID: "local/known", ProviderID: "local", UpstreamID: "known"}}
	c, err := catalog.New([]provider.Provider{offline}, bundled, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	model, ok := c.Model("local/known")
	if !ok || model.Metadata.CatalogSource != "bundled" || !model.Availability.Stale {
		t.Fatalf("bundled fallback = %#v, %v", model, ok)
	}
}

func TestCatalogRejectsCollisionsAndZeroUsableProviders(t *testing.T) {
	t.Run("collision", func(t *testing.T) {
		duplicate := &stubProvider{id: "local", models: []provider.Model{{ID: "local/a"}, {ID: "local/a"}}}
		c, _ := catalog.New([]provider.Provider{duplicate}, []catalog.Descriptor{}, nil)
		if err := c.Refresh(context.Background()); err == nil {
			t.Fatal("Refresh() error = nil")
		}
	})
	t.Run("zero usable", func(t *testing.T) {
		c, _ := catalog.New([]provider.Provider{&stubProvider{id: "offline", err: errors.New("down")}}, []catalog.Descriptor{}, nil)
		if err := c.Refresh(context.Background()); err == nil {
			t.Fatal("Refresh() error = nil")
		}
	})
}

func TestCatalogSnapshotsAreImmutable(t *testing.T) {
	c, _ := catalog.New([]provider.Provider{&stubProvider{id: "local", models: []provider.Model{{ID: "local/a"}}}}, []catalog.Descriptor{}, nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := c.Models()
	first[0].PublicID = "changed"
	if model, ok := c.Model("local/a"); !ok || model.PublicID != "local/a" {
		t.Fatalf("catalog snapshot was mutated: %#v, %v", model, ok)
	}
}

func TestLivePricingPrecedesBundledButOperatorCanOverride(t *testing.T) {
	p := &stubProvider{id: "p", models: []provider.Model{{ID: "p/paid", ContextWindow: intPointer(2000), InputPerMillion: floatPointer(2), OutputPerMillion: floatPointer(4)}, {ID: "p/included", Subscription: true}}}
	bundled := []catalog.Descriptor{{PublicID: "p/paid", ContextWindow: intPointer(1000), Pricing: catalog.Pricing{InputPerMillion: floatPointer(9), OutputPerMillion: floatPointer(9)}}, {PublicID: "p/included", Pricing: catalog.Pricing{InputPerMillion: floatPointer(9), OutputPerMillion: floatPointer(9)}}}
	c, err := catalog.New([]provider.Provider{p}, bundled, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	paid, _ := c.Model("p/paid")
	included, _ := c.Model("p/included")
	if *paid.Pricing.InputPerMillion != 2 || *paid.Pricing.OutputPerMillion != 4 || *paid.ContextWindow != 2000 || !included.Subscription || *included.Pricing.InputPerMillion != 0 {
		t.Fatal("live metadata lost", paid, included)
	}
	c, err = catalog.New([]provider.Provider{p}, bundled, []catalog.Descriptor{{PublicID: "p/paid", Pricing: catalog.Pricing{InputPerMillion: floatPointer(3)}}})
	if err != nil {
		t.Fatal(err)
	}
	c.Refresh(context.Background())
	paid, _ = c.Model("p/paid")
	if *paid.Pricing.InputPerMillion != 3 || *paid.Pricing.OutputPerMillion != 4 {
		t.Fatal("operator precedence", paid)
	}
}
