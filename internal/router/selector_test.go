package router_test

import (
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/router"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func model(id string) catalog.Descriptor {
	return catalog.Descriptor{PublicID: id, Availability: catalog.Availability{Available: true}, Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Capabilities: catalog.ModelCapabilities{Streaming: catalog.Supported, Tools: catalog.Supported, Vision: catalog.Supported, StructuredOutput: catalog.Supported, Reasoning: catalog.Supported}, Quality: catalog.Quality{General: ptr(.9), Tools: ptr(.9), Reasoning: ptr(.9)}, ContextWindow: ptr(int64(10000)), Pricing: catalog.Pricing{InputPerMillion: ptr(1.0), OutputPerMillion: ptr(2.0)}}
}
func TestSelectFiltersHardRequirements(t *testing.T) {
	good := model("b/good")
	badEndpoint := model("a/endpoint")
	badEndpoint.Endpoints.ChatCompletions = catalog.Unsupported
	badTools := model("a/tools")
	badTools.Capabilities.Tools = catalog.Unknown
	small := model("a/small")
	small.ContextWindow = ptr(int64(100))
	exhausted := model("a/quota")
	decision, err := router.Select([]router.Candidate{{Model: badEndpoint}, {Model: badTools}, {Model: small}, {Model: exhausted, QuotaExhausted: true}, {Model: good}}, router.RequestRequirements{Endpoint: router.EndpointChat, Tools: true, EstimatedInputTokens: 1000, RequestedOutputTokens: 100}, "high", router.DefaultSelectionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Model.PublicID != "b/good" || len(decision.Reasons) != 5 {
		t.Fatalf("decision=%#v", decision)
	}
}
func TestSelectUsesUnknownQualityOnlyAsFallback(t *testing.T) {
	known := model("z/known")
	unknown := model("a/unknown")
	unknown.Quality.General = nil
	unknown.Pricing.InputPerMillion = ptr(0.0)
	unknown.Pricing.OutputPerMillion = ptr(0.0)
	req := router.RequestRequirements{Endpoint: router.EndpointChat}
	got, err := router.Select([]router.Candidate{{Model: unknown}, {Model: known}}, req, "low", router.DefaultSelectionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.PublicID != "z/known" {
		t.Fatalf("selected %q", got.Model.PublicID)
	}
	known.Quality.General = ptr(.1)
	got, err = router.Select([]router.Candidate{{Model: unknown}, {Model: known}}, req, "low", router.DefaultSelectionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.PublicID != "a/unknown" {
		t.Fatalf("fallback selected %q", got.Model.PublicID)
	}
}
func TestSelectFreeOnlyExcludesUnknownPricing(t *testing.T) {
	unknown := model("a/unknown")
	unknown.Pricing = catalog.Pricing{}
	local := model("z/local")
	local.Local = ptr(true)
	policy := router.DefaultSelectionPolicy()
	policy.AllowPaidModels = false
	got, err := router.Select([]router.Candidate{{Model: unknown}, {Model: local}}, router.RequestRequirements{Endpoint: router.EndpointChat}, "low", policy)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.PublicID != "z/local" || got.EstimatedCost == nil || *got.EstimatedCost != 0 {
		t.Fatalf("decision=%#v", got)
	}
}
func TestSelectDeterministicOrdering(t *testing.T) {
	cheap := model("z/cheap")
	*cheap.Pricing.InputPerMillion = .1
	expensive := model("a/expensive")
	req := router.RequestRequirements{Endpoint: router.EndpointChat, EstimatedInputTokens: 1000, RequestedOutputTokens: 100}
	got, err := router.Select([]router.Candidate{{Model: expensive, QuotaKnown: true, QuotaRemaining: 1, LatencyEWMA: time.Millisecond}, {Model: cheap, QuotaKnown: true, QuotaRemaining: .1, LatencyEWMA: time.Second}}, req, "low", router.DefaultSelectionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.PublicID != "z/cheap" {
		t.Fatalf("selected %q", got.Model.PublicID)
	}
	a, b := model("b/model"), model("a/model")
	a.Pricing = b.Pricing
	got, err = router.Select([]router.Candidate{{Model: a}, {Model: b}}, req, "low", router.DefaultSelectionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.PublicID != "a/model" {
		t.Fatalf("lexical selected %q", got.Model.PublicID)
	}
}
func FuzzSelectOrderIndependent(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(42))
	f.Fuzz(func(t *testing.T, seed int64) {
		candidates := []router.Candidate{{Model: model("c/model")}, {Model: model("a/model")}, {Model: model("b/model")}}
		first, err := router.Select(candidates, router.RequestRequirements{Endpoint: router.EndpointChat}, "low", router.DefaultSelectionPolicy())
		if err != nil {
			t.Fatal(err)
		}
		rand.New(rand.NewSource(seed)).Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		second, err := router.Select(candidates, router.RequestRequirements{Endpoint: router.EndpointChat}, "low", router.DefaultSelectionPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if first.Model.PublicID != second.Model.PublicID || !reflect.DeepEqual(first.EstimatedCost, second.EstimatedCost) {
			t.Fatalf("selection changed: %#v %#v", first, second)
		}
	})
}

func TestSelectReservesDefaultOutputAndAvoidsTokenOverflow(t *testing.T) {
	small := model("a/small")
	small.ContextWindow = ptr(int64(100))
	large := model("b/large")
	for _, req := range []router.RequestRequirements{
		{Endpoint: router.EndpointChat},
		{Endpoint: router.EndpointChat, EstimatedInputTokens: 1<<63 - 2, RequestedOutputTokens: 100},
	} {
		decision, err := router.Select([]router.Candidate{{Model: small}, {Model: large}}, req, "medium", router.DefaultSelectionPolicy())
		if req.EstimatedInputTokens == 0 {
			if err != nil || decision.Model.PublicID != "b/large" {
				t.Fatalf("default output ignored: %#v %v", decision, err)
			}
		} else if err == nil {
			t.Fatal("token arithmetic overflow bypassed context check")
		}
	}
}

func TestSelectLatencyPreferenceStaysWithinHardConstraints(t *testing.T) {
	cheap, quick, unsupported := model("a/cheap"), model("b/quick"), model("c/unsupported")
	cheap.Pricing = catalog.Pricing{InputPerMillion: ptr(0.0), OutputPerMillion: ptr(0.0)}
	unsupported.Capabilities.Tools = catalog.Unsupported
	candidates := []router.Candidate{{Model: cheap, LatencyEWMA: time.Second}, {Model: quick, LatencyEWMA: 100 * time.Millisecond}, {Model: unsupported, LatencyEWMA: time.Millisecond}}
	for _, sensitive := range []bool{false, true} {
		req := router.RequestRequirements{Endpoint: router.EndpointChat, Tools: true, LatencySensitive: sensitive, RequestedOutputTokens: 100}
		decision, err := router.Select(candidates, req, "medium", router.DefaultSelectionPolicy())
		want := "a/cheap"
		if sensitive {
			want = "b/quick"
		}
		if err != nil || decision.Model.PublicID != want {
			t.Fatalf("sensitive=%v decision=%#v err=%v", sensitive, decision, err)
		}
	}
}
