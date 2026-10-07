package intelligence

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
)

func TestConfiguredTableMatchesSuffixesMultipleProvidersAndRefreshWithoutRoutingEvidence(t *testing.T) {
	at := time.Now().UTC()
	m, _ := manager(t, nil, options())
	m.UpdateModels([]catalog.Descriptor{{PublicID: "one/gemma-4-26b", UpstreamID: "gemma-4-26b"}, {PublicID: "two/google/gemma_4_26b:latest", UpstreamID: "google/gemma_4_26b:latest"}})
	snapshot := Snapshot{Sources: map[string]SourceState{"livebench": {Enabled: true, Observations: []Observation{
		observation("livebench", "code", Coding, "gemma-4-26b-128k-agent", .8, at),
		observation("livebench", "code", Coding, "gemma-4-26b-64k", .7, at),
		observation("livebench", "code", Coding, "gemma-3-26b", .9, at),
		observation("livebench", "code", Coding, "gemma-4-27b", .9, at),
	}}}}
	rows := m.ConfiguredTableModels(snapshot)
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row.Match != "approximate" || !reflect.DeepEqual(row.ProviderIDs, []string{"one/gemma-4-26b", "two/google/gemma_4_26b:latest"}) {
			t.Fatal(row)
		}
	}
	if len(m.Snapshot().Models) != 0 {
		t.Fatal("display associations entered routing")
	}
	m.UpdateModels([]catalog.Descriptor{{PublicID: "one/gemma-4-27b", UpstreamID: "gemma-4-27b"}})
	rows = m.ConfiguredTableModels(snapshot)
	if len(rows) != 1 || rows["external/gemma-4-27b"].ProviderIDs[0] != "one/gemma-4-27b" {
		t.Fatal("catalog refresh not reflected", rows)
	}
}

func TestDisplayIdentityTokenBoundariesAndExactAliasPrecedence(t *testing.T) {
	for _, item := range []struct {
		left, right string
		want        bool
	}{
		{"gemma-4-26b", "gemma-4-26b-128k-agent", true},
		{"gemma-4-26b-128k-agent", "gemma-4-26b", true},
		{"Google/GEMMA_4_26b:latest", "gemma-4-26b", true},
		{"gemma-4-26b", "gemma-3-26b", false},
		{"gemma-4-26b", "gemma-4-27b", false},
		{"gemma-4-26b", "gemma-4-260b", false},
		{"gpt-5.2", "gpt-5.3", false},
		{"model", "modeling", false}, {"", "model", false},
	} {
		if DisplayIdentityMatch(item.left, item.right) != item.want {
			t.Fatal(item)
		}
	}
	opts := options()
	opts.Identities = []Identity{{CanonicalID: "specific", ProviderIDs: []string{"one/exact"}, Aliases: []string{"gemma-4-26b"}}}
	m, _ := manager(t, nil, opts)
	m.UpdateModels([]catalog.Descriptor{{PublicID: "one/exact", UpstreamID: "exact"}, {PublicID: "two/gemma-4-26b-agent", UpstreamID: "gemma-4-26b-agent"}})
	rows := m.ConfiguredTableModels(Snapshot{Sources: map[string]SourceState{"source": {Enabled: true, Observations: []Observation{observation("source", "code", Coding, "gemma-4-26b", .8, time.Now())}}}})
	for _, row := range rows {
		if !reflect.DeepEqual(row.ProviderIDs, []string{"one/exact"}) {
			t.Fatal("approximate match overrode explicit alias", row)
		}
		if row.Match != "matched" {
			t.Fatal("exact alias labelled incorrectly", row)
		}
	}
	stale := Snapshot{Models: map[string]Model{"removed": {ID: "removed", ProviderIDs: []string{"old/removed"}}}}
	if rows := m.ConfiguredTableModels(stale); len(rows) != 0 {
		t.Fatal("removed provider association retained", rows)
	}
}

func TestTableModelsCombinesAllCapabilitiesWithoutAddingUnmatchedRoutingModels(t *testing.T) {
	at := time.Now().UTC()
	first := fakeSource{id: "first", fetch: func(context.Context) (Result, error) {
		return Result{Observations: []Observation{
			observation("", "code", Coding, "claude-opus-5", .8, at),
			observation("", "tools", Agentic, "claude-opus-5", .75, at),
			observation("", "code", Coding, "model-a", .9, at),
		}}, nil
	}}
	second := fakeSource{id: "second", fetch: func(context.Context) (Result, error) {
		return Result{Observations: []Observation{
			observation("", "knowledge", Knowledge, "claude-opus-5", .95, at),
			observation("", "code", Coding, "CLAUDE-OPUS-5", .9, at),
			observation("", "preference", Preference, "claude-opus-5", 1, at),
			observation("", "code", Coding, "claude-opus-5-high", .7, at),
		}}, nil
	}}
	m, _ := manager(t, []Source{first, second}, options())
	m.now = func() time.Time { return at }
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := m.Snapshot()
	rows := m.TableModels(snapshot)
	if len(rows) != 3 {
		t.Fatalf("one row per model: %+v", rows)
	}
	model := rows["external/claude-opus-5"]
	near(t, model.Scores[Coding], .85)
	near(t, model.Scores[Agentic], .75)
	near(t, model.Scores[Knowledge], .95)
	near(t, model.Scores[Preference], 1)
	near(t, model.Scores["overall"], .85) // Preference is not silently included.
	if model.DisplayName != "claude-opus-5" || model.Match != "unmatched" || len(model.Sources) != 2 || len(model.ExternalIDs) != 2 || model.ObservationCount != 5 || model.Observations != nil {
		t.Fatalf("incorrect model summary: %+v", model)
	}
	if _, ok := m.Lookup("external/claude-opus-5"); ok || len(m.Snapshot().Models) != 1 {
		t.Fatal("display-only model became eligible for routing")
	}
	if len(snapshot.Sources["first"].Observations) != 3 || len(snapshot.Models["local/model-a"].Observations) != 1 {
		t.Fatal("summarizing discarded underlying results")
	}
}

func TestTableModelsUsesCanonicalAliasesAndPreservesAmbiguity(t *testing.T) {
	at := time.Now().UTC()
	opts := options()
	opts.Identities = []Identity{{CanonicalID: "model-a", ProviderIDs: []string{"local/model-a"}, Aliases: []string{"Model A alias"}}}
	source := fakeSource{id: "source", fetch: func(context.Context) (Result, error) {
		return Result{Observations: []Observation{
			observation("", "code", Coding, "model-a", .9, at),
			observation("", "tools", Agentic, "Model A alias", .8, at),
			observation("", "code", Coding, "ambiguous", .7, at),
		}}, nil
	}}
	m, _ := manager(t, []Source{source}, opts)
	m.UpdateModels(append(modelCatalog(), catalog.Descriptor{PublicID: "one/ambiguous", UpstreamID: "ambiguous"}, catalog.Descriptor{PublicID: "two/ambiguous", UpstreamID: "ambiguous"}))
	m.now = func() time.Time { return at }
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows := m.TableModels(m.Snapshot())
	if len(rows) != 2 || rows["model-a"].Match != "matched" || rows["external/ambiguous"].Match != "ambiguous" {
		t.Fatalf("identity grouping: %+v", rows)
	}
	near(t, rows["model-a"].Scores[Agentic], .8)
	if _, ok := m.Lookup("one/ambiguous"); ok {
		t.Fatal("ambiguous table row resolved a routing identity")
	}
}

func TestTableModelsRespectsWeightsFreshnessAndDisabledSources(t *testing.T) {
	at := time.Now().UTC()
	m, _ := manager(t, nil, options())
	m.now = func() time.Time { return at }
	snapshot := Snapshot{Sources: map[string]SourceState{
		"fresh":    {Enabled: true, Observations: []Observation{observation("fresh", "code", Coding, "model", .8, at)}},
		"stale":    {Enabled: true, Observations: []Observation{observation("stale", "math", Math, "model", .9, at.Add(-365*24*time.Hour))}},
		"disabled": {Enabled: false, Observations: []Observation{observation("disabled", "knowledge", Knowledge, "hidden-model", 1, at)}},
	}}
	rows := m.TableModels(snapshot)
	if len(rows) != 1 {
		t.Fatal("disabled-only data entered the table")
	}
	model := rows["external/model"]
	near(t, model.Scores[Coding], .8)
	if _, ok := model.Scores[Math]; ok {
		t.Fatal("obsolete observations filled capability scores")
	}
	m.options.Aggregation.SourceWeights = map[string]float64{"fresh": 0}
	if len(m.TableModels(snapshot)["external/model"].Scores) != 0 {
		t.Fatal("table ignored configured aggregation weights")
	}
}
