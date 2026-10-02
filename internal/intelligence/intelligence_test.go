package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/state"
)

type fakeSource struct {
	id    string
	fetch func(context.Context) (Result, error)
}

func (s fakeSource) ID() string { return s.id }
func (s fakeSource) Fetch(ctx context.Context) (Result, error) {
	if s.fetch == nil {
		return Result{}, errors.New("unavailable")
	}
	return s.fetch(ctx)
}
func modelCatalog() []catalog.Descriptor {
	return []catalog.Descriptor{{PublicID: "local/model-a", ProviderID: "local", UpstreamID: "model-a"}, {PublicID: "cloud/model-b", ProviderID: "cloud", UpstreamID: "model-b"}}
}
func observation(source, benchmark, category, model string, score float64, at time.Time) Observation {
	return Observation{Source: source, Benchmark: benchmark, Category: category, ExternalModelID: model, Score: score, Normalized: &score, EvaluatedAt: at, RetrievedAt: at}
}
func options() Options { return Options{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))} }
func manager(t *testing.T, sources []Source, opts Options) (*Manager, *Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := OpenStore(context.Background(), db, dir, opts.Defaults().Logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	m, err := NewManager(store, sources, modelCatalog(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return m, store
}

func TestIdentityMatchesScopedAliasesAndNeverMergesAmbiguousModels(t *testing.T) {
	models := append(modelCatalog(), catalog.Descriptor{PublicID: "remote/model-a", ProviderID: "remote", UpstreamID: "model-a"})
	m := NewMatcher(models, nil)
	if id, candidates := m.Match("aa", "MODEL_A"); id != "" || len(candidates) != 2 {
		t.Fatalf("ambiguous upstream matched: %s %v", id, candidates)
	}
	if id, _ := m.Match("aa", "local/model-a"); id != "local/model-a" {
		t.Fatalf("canonical match=%s", id)
	}
	if id, _ := m.Match("aa", "model-a-2026-09-01"); id != "" {
		t.Fatal("dated model matched to undated base")
	}
	if id, _ := m.Match("aa", "model-a (high)"); id != "" {
		t.Fatal("effort variant matched to base")
	}
	identities := []Identity{{CanonicalID: "model-a", ProviderIDs: []string{"local/model-a", "remote/model-a"}, Aliases: []string{"Model A"}, SourceIDs: map[string][]string{"aa": {"source-uuid"}}}}
	m = NewMatcher(models, identities)
	for _, external := range []string{"Model A", "source-uuid", "model-a", "local/model-a"} {
		if id, _ := m.Match("aa", external); id != "model-a" {
			t.Fatalf("alias %s matched %s", external, id)
		}
	}
	if id, _ := m.Match("other", "source-uuid"); id != "" {
		t.Fatal("source-scoped alias leaked to another source")
	}
	conflicting := []Identity{{CanonicalID: "a", ProviderIDs: []string{"local/model-a"}, Aliases: []string{"shared"}}, {CanonicalID: "b", ProviderIDs: []string{"cloud/model-b"}, Aliases: []string{"shared"}}}
	m = NewMatcher(models, conflicting)
	if id, candidates := m.Match("aa", "shared"); id != "" || len(candidates) != 2 {
		t.Fatal("conflicting explicit alias silently merged")
	}
}

func TestAggregationSourceBenchmarkWeightsPreferenceAndRawRetention(t *testing.T) {
	now := time.Now().UTC()
	obs := []Observation{observation("s1", "code", Coding, "a", .9, now), observation("s2", "code", Coding, "a", .3, now), observation("arena", "rating", Preference, "a", .99, now)}
	cfg := Aggregation{SourceWeights: map[string]float64{"s1": 3, "s2": 1}}
	m := Aggregate(obs, cfg, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores[Coding], .75)
	near(t, m.Scores["overall"], .75)
	near(t, m.Scores[Preference], .99)
	if len(m.Observations) != 3 {
		t.Fatal("raw observations discarded")
	}
	cfg.OverallWeights = map[string]float64{Coding: 1, Preference: 1}
	m = Aggregate(obs, cfg, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores["overall"], .87)
	obs = append(obs, observation("s1", "another-code", Coding, "a", .1, now))
	cfg.BenchmarkWeights = map[string]float64{"s1/another-code": 0}
	m = Aggregate(obs, cfg, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores[Coding], .75)
	cfg.SourceWeights["s1"] = 0
	m = Aggregate(obs, cfg, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores[Coding], .3)
}

func TestAggregationFreshnessStaleDataDedupAndQuestionBalancing(t *testing.T) {
	now := time.Now().UTC()
	newer := observation("new", "code", Coding, "a", .9, now)
	older := observation("old", "code", Coding, "a", .3, now.Add(-time.Hour))
	older.RetrievedAt = now
	m := Aggregate([]Observation{newer, older}, Aggregation{}, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores[Coding], .7)
	stale := newer
	stale.RetrievedAt = now.Add(-2 * time.Hour)
	m = Aggregate([]Observation{stale}, Aggregation{}, now, time.Hour, 24*time.Hour, time.Hour)
	if !m.Stale || len(m.Scores) > 0 || len(m.Observations) != 1 {
		t.Fatal("stale evidence used or discarded")
	}
	ancient := newer
	ancient.EvaluatedAt = now.Add(-48 * time.Hour)
	m = Aggregate([]Observation{ancient}, Aggregation{}, now, time.Hour, 24*time.Hour, time.Hour)
	if len(m.Scores) > 0 {
		t.Fatal("old evaluation refreshed by retrieval timestamp")
	}
	oldVersion := newer
	oldVersion.Score = .1
	v := .1
	oldVersion.Normalized = &v
	oldVersion.EvaluatedAt = now.Add(-time.Minute)
	m = Aggregate([]Observation{newer, oldVersion}, Aggregation{}, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores[Coding], .9)
	if len(m.Observations) != 2 {
		t.Fatal("previous raw version discarded")
	}
	var questions []Observation
	for i := range 100 {
		o := observation("questions", "task", Coding, "a", 1, now)
		o.Metadata = map[string]any{"question_id": string(rune(i + 32))}
		questions = append(questions, o)
	}
	questions = append(questions, observation("other", "task", Coding, "a", 0, now))
	m = Aggregate(questions, Aggregation{}, now, time.Hour, 24*time.Hour, time.Hour)
	near(t, m.Scores[Coding], .5)
}

func TestRefreshPartialFailureRetainsGoodCacheAndInspection(t *testing.T) {
	var fail atomic.Bool
	good := fakeSource{id: "good", fetch: func(context.Context) (Result, error) {
		if fail.Load() {
			return Result{}, errors.New("private test token must not appear")
		}
		return Result{Observations: []Observation{observation("", "coding", Coding, "model-a", .9, time.Now()), observation("", "coding", Coding, "not-installed", .5, time.Now())}}, nil
	}}
	bad := fakeSource{id: "bad", fetch: func(context.Context) (Result, error) {
		return Result{}, &SourceError{Stage: "upstream request", Status: 401}
	}}
	m, store := manager(t, []Source{good, bad}, options())
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("partial failure unreported")
	}
	if model, ok := m.Lookup("local/model-a"); !ok {
		t.Fatal("successful source blocked by another failure")
	} else {
		near(t, model.Scores[Coding], .9)
	}
	snapshot := m.Snapshot()
	if len(snapshot.Unmatched) != 1 || snapshot.Unmatched[0].ExternalID != "not-installed" {
		t.Fatal("unmatched identifier missing")
	}
	if snapshot.Sources["bad"].LastAttempt.IsZero() || !snapshot.Sources["bad"].LastSuccess.IsZero() || snapshot.Sources["bad"].Error == "" {
		t.Fatal("source failure timestamps/status incorrect")
	}
	success := snapshot.Sources["good"].LastSuccess
	fail.Store(true)
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("source error unreported")
	}
	snapshot = m.Snapshot()
	if !snapshot.Sources["good"].LastSuccess.Equal(success) || len(snapshot.Sources["good"].Observations) != 2 {
		t.Fatal("failed update destroyed good dataset")
	}
	if snapshot.Sources["good"].Error == "private test token must not appear" {
		t.Fatal("unsafe upstream error disclosed")
	}
	reloaded, err := NewManager(store, []Source{good, bad}, modelCatalog(), options())
	if err != nil {
		t.Fatal(err)
	}
	if model, ok := reloaded.Lookup("local/model-a"); !ok {
		t.Fatal("startup did not use persistent cache")
	} else {
		near(t, model.Scores[Coding], .9)
	}
	var sequence int
	var name, path string
	if err := store.db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("cache permissions are not owner-only")
	}
	// Snapshots and routing views are isolated from callers.
	snapshot.Models["local/model-a"].Scores[Coding] = 0
	model, _ := m.Lookup("local/model-a")
	near(t, model.Scores[Coding], .9)
}

func TestSourceTypeChangesDoNotReuseAnotherAdaptersCache(t *testing.T) {
	source := fakeSource{id: "shared", fetch: func(context.Context) (Result, error) {
		return Result{Observations: []Observation{observation("", "code", Coding, "model-a", .9, time.Now())}}, nil
	}}
	opts := options()
	opts.Sources = []SourceSettings{{ID: "shared", Type: "benchlm", Enabled: true}}
	m, store := manager(t, []Source{source}, opts)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	opts.Sources[0].Type = "artificial_analysis"
	reloaded, err := NewManager(store, []Source{source}, modelCatalog(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Lookup("local/model-a"); ok {
		t.Fatal("source type change reused incompatible cache")
	}
	if len(reloaded.Snapshot().Sources["shared"].Observations) != 0 {
		t.Fatal("old adapter observations retained under new source type")
	}
}

func TestRefreshDoesNotBlockCachedReadsAndDoesNotFetchOnLookup(t *testing.T) {
	var calls atomic.Int64
	entered := make(chan struct{})
	release := make(chan struct{})
	source := fakeSource{id: "source", fetch: func(ctx context.Context) (Result, error) {
		if calls.Add(1) > 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		return Result{Observations: []Observation{observation("", "code", Coding, "model-a", .8, time.Now())}}, nil
	}}
	m, _ := manager(t, []Source{source}, options())
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- m.Refresh(context.Background()) }()
	<-entered
	for range 20 {
		if _, ok := m.Lookup("local/model-a"); !ok {
			t.Fatal("cached reads unavailable during refresh")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("lookup fetched benchmark data")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCacheAtomicFailureDisabledSourcesStalenessAndCorruption(t *testing.T) {
	source := fakeSource{id: "source", fetch: func(context.Context) (Result, error) {
		return Result{Observations: []Observation{observation("", "code", Coding, "model-a", .8, time.Now())}}, nil
	}}
	m, store := manager(t, []Source{source}, options())
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, err := store.Load(context.Background())
	before, _ := json.Marshal(beforeSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	bad := m.Snapshot()
	state := bad.Sources["source"]
	state.Metadata = map[string]any{"unserializable": func() {}}
	bad.Sources["source"] = state
	if err := store.Save(context.Background(), bad); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	afterSnapshot, _ := store.Load(context.Background())
	after, _ := json.Marshal(afterSnapshot)
	if string(before) != string(after) {
		t.Fatal("failed atomic save replaced good cache")
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_publication BEFORE UPDATE ON benchmark_storage BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("failed persistence unreported")
	}
	if _, err := store.db.Exec("DROP TRIGGER reject_publication"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Lookup("local/model-a"); !ok {
		t.Fatal("failed persistence destroyed memory snapshot")
	}
	disabled := options()
	disabled.Sources = []SourceSettings{{ID: "source", Type: "benchlm", Enabled: false}}
	reloaded, err := NewManager(store, nil, modelCatalog(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Lookup("local/model-a"); ok {
		t.Fatal("disabled source influenced routing")
	}
	if len(reloaded.Snapshot().Sources["source"].Observations) == 0 {
		t.Fatal("disabled source results discarded")
	}
	m.now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if _, ok := m.Lookup("local/model-a"); ok {
		t.Fatal("stale cache consumed by routing")
	}
	if _, err := store.db.Exec("UPDATE benchmark_observations SET metadata='invalid json'"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("corrupt database cache accepted")
	}
	if _, err := NewManager(store, nil, modelCatalog(), options()); err != nil {
		t.Fatal("corrupt cache prevented startup")
	}
}

func TestScheduledRefreshStopsWithLifecycleAndUsesSourceTimeout(t *testing.T) {
	var calls atomic.Int64
	source := fakeSource{id: "source", fetch: func(context.Context) (Result, error) {
		calls.Add(1)
		return Result{Observations: []Observation{observation("", "code", Coding, "model-a", .8, time.Now())}}, nil
	}}
	opts := options()
	opts.RefreshInterval = 10 * time.Millisecond
	m, _ := manager(t, []Source{source}, opts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for calls.Load() < 2 {
		select {
		case <-deadline.C:
			cancel()
			t.Fatal("scheduled refresh missing")
		case <-tick.C:
		}
	}
	cancel()
	<-done
	before := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != before {
		t.Fatal("worker remained after lifecycle cancellation")
	}
	opts.SourceTimeout = 10 * time.Millisecond
	m, _ = manager(t, []Source{fakeSource{id: "slow", fetch: func(ctx context.Context) (Result, error) { <-ctx.Done(); return Result{}, ctx.Err() }}}, opts)
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("source timeout unreported")
	}
}

func TestConcurrentRefreshSnapshotAndLookup(t *testing.T) {
	source := fakeSource{id: "source", fetch: func(context.Context) (Result, error) {
		return Result{Observations: []Observation{observation("", "code", Coding, "model-a", .8, time.Now())}}, nil
	}}
	m, _ := manager(t, []Source{source}, options())
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				_ = m.Refresh(context.Background())
				_ = m.Snapshot()
				_, _ = m.Lookup("local/model-a")
			}
		}()
	}
	wg.Wait()
}
