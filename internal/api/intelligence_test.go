package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
	"github.com/Apartment38/torii-gateway/internal/state"
)

type codingClassifier struct{}

func (codingClassifier) ID() string { return "test/classifier" }
func (codingClassifier) Classify(context.Context, classifier.ClassificationRequest) (classifier.Classification, error) {
	return classifier.Classification{TaskType: classifier.TaskCoding, Complexity: classifier.ComplexityMedium, Confidence: 1}, nil
}
func benchmarkPtr[T any](value T) *T { return &value }

type consoleBenchmarkSource struct {
	id    string
	fetch func(context.Context) (intelligence.Result, error)
}

func (s consoleBenchmarkSource) ID() string { return s.id }
func (s consoleBenchmarkSource) Fetch(ctx context.Context) (intelligence.Result, error) {
	return s.fetch(ctx)
}
func consoleBenchmarkManager(t *testing.T, sources []intelligence.Source) (*api.OpenAIHandler, *intelligence.Manager) {
	t.Helper()
	handler, err := api.NewOpenAIHandler(nil)
	if err != nil {
		t.Fatal(err)
	}
	database, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	store, err := intelligence.OpenStore(context.Background(), database, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := intelligence.NewManager(store, sources, []catalog.Descriptor{{PublicID: "local/model-a", ProviderID: "local", UpstreamID: "model-a"}}, intelligence.Options{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	handler.SetIntelligence(m)
	// SetIntelligence normally takes identity from the handler's provider catalog.
	m.UpdateModels([]catalog.Descriptor{{PublicID: "local/model-a", ProviderID: "local", UpstreamID: "model-a"}})
	return handler, m
}
func consoleBenchmarkObservation(model string) intelligence.Observation {
	return intelligence.Observation{ExternalModelID: model, Benchmark: "code", Category: intelligence.Coding, Score: 90, Normalized: benchmarkPtr(.9), Metadata: map[string]any{"large_upstream_row": "not needed in table"}}
}

type configuredBenchmarkProvider struct{ models []string }

func (p *configuredBenchmarkProvider) ID() string { return "local" }
func (p *configuredBenchmarkProvider) Models(context.Context) ([]provider.Model, error) {
	var models []provider.Model
	for _, model := range p.models {
		models = append(models, provider.Model{ID: "local/" + model, Object: "model"})
	}
	return models, nil
}
func (p *configuredBenchmarkProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	return nil, nil
}

func TestConfiguredBenchmarkAPIUsesCurrentProvidersAndDoesNotChangeRouting(t *testing.T) {
	source := consoleBenchmarkSource{id: "source", fetch: func(context.Context) (intelligence.Result, error) {
		return intelligence.Result{Observations: []intelligence.Observation{consoleBenchmarkObservation("gemma-4-26b-128k-agent")}}, nil
	}}
	_, m := consoleBenchmarkManager(t, []intelligence.Source{source})
	p := &configuredBenchmarkProvider{models: []string{"gemma-4-26b"}}
	h, err := api.NewOpenAIHandler([]provider.OpenAIProvider{p})
	if err != nil {
		t.Fatal(err)
	}
	h.SetIntelligence(m)
	if err := h.RefreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	get := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/gateway/v1/intelligence/models"+query, nil))
		return w
	}
	w := get("?configured_only=true")
	var rows map[string]intelligence.TableModel
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || w.Code != 200 || len(rows) != 1 {
		t.Fatalf("%d %s %v", w.Code, w.Body.String(), err)
	}
	for _, row := range rows {
		if row.Match != "approximate" || len(row.ProviderIDs) != 1 || row.ProviderIDs[0] != "local/gemma-4-26b" {
			t.Fatal(row)
		}
	}
	if len(m.Snapshot().Models) != 0 {
		t.Fatal("display data became routing evidence")
	}
	if w := get("?configured_only=invalid"); w.Code != 400 {
		t.Fatal("invalid scope accepted")
	}
	if w := get(""); strings.TrimSpace(w.Body.String()) != "{}" {
		t.Fatal("legacy endpoint changed", w.Body.String())
	}
	p.models = []string{"gemma-4-27b"}
	if err := h.RefreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := get("?configured_only=true"); strings.TrimSpace(w.Body.String()) != "{}" {
		t.Fatal("provider refresh did not remove association", w.Body.String())
	}
}

func TestBenchmarkRefreshStreamsPublicationBeforeSlowSourceFinishes(t *testing.T) {
	release := make(chan struct{})
	fast := consoleBenchmarkSource{id: "fast", fetch: func(context.Context) (intelligence.Result, error) {
		return intelligence.Result{Observations: []intelligence.Observation{consoleBenchmarkObservation("model-a")}}, nil
	}}
	slow := consoleBenchmarkSource{id: "slow", fetch: func(ctx context.Context) (intelligence.Result, error) {
		select {
		case <-release:
			return intelligence.Result{}, fmt.Errorf("secret-sentinel")
		case <-ctx.Done():
			return intelligence.Result{}, ctx.Err()
		}
	}}
	handler, m := consoleBenchmarkManager(t, []intelligence.Source{fast, slow})
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/gateway/v1/intelligence/refresh", nil)
	request.Header.Set("Accept", "text/event-stream")
	reply, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer reply.Body.Close()
	if reply.StatusCode != 200 || !strings.Contains(reply.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("response=%d", reply.StatusCode)
	}
	scanner := bufio.NewScanner(reply.Body)
	var content strings.Builder
	published, result := false, false
	for scanner.Scan() {
		line := scanner.Text()
		content.WriteString(line)
		if strings.Contains(line, `"phase":"published"`) {
			published = true
			if _, ok := m.Lookup("local/model-a"); !ok {
				t.Fatal("stream event preceded memory publication")
			}
			close(release)
		}
		if line == "event: result" {
			result = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !published || !result || !strings.Contains(content.String(), `"partial_failure":true`) {
		t.Fatalf("events=%s", content.String())
	}
	if strings.Contains(content.String(), "secret-sentinel") {
		t.Fatal("upstream error leaked into stream")
	}
}

func TestBenchmarkConsoleJoinsBackgroundRefreshThroughRequestMiddleware(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var fetches atomic.Int64
	source := consoleBenchmarkSource{id: "source", fetch: func(ctx context.Context) (intelligence.Result, error) {
		fetches.Add(1)
		close(started)
		select {
		case <-release:
			return intelligence.Result{Observations: []intelligence.Observation{consoleBenchmarkObservation("unknown-model")}}, nil
		case <-ctx.Done():
			return intelligence.Result{}, ctx.Err()
		}
	}}
	handler, m := consoleBenchmarkManager(t, []intelligence.Source{source})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Refresh(ctx) }()
	<-started
	server := httptest.NewServer(api.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), handler))
	defer server.Close()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/gateway/v1/intelligence/refresh", nil)
	request.Header.Set("Accept", "text/event-stream")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	joined, completed := false, false
	for scanner.Scan() {
		line := scanner.Text()
		if !joined && strings.Contains(line, `"phase":"fetching"`) {
			joined = true
			close(release)
		}
		if line == "event: result" {
			completed = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !joined || !completed || fetches.Load() != 1 {
		t.Fatalf("background refresh was not followed: joined=%v completed=%v fetches=%d", joined, completed, fetches.Load())
	}
	reply, err := server.Client().Get(server.URL + "/gateway/v1/intelligence/observations?include_metadata=true&limit=25")
	if err != nil {
		t.Fatal(err)
	}
	defer reply.Body.Close()
	var rows struct {
		Total        int                        `json:"total"`
		Observations []intelligence.Observation `json:"observations"`
	}
	if err := json.NewDecoder(reply.Body).Decode(&rows); err != nil || rows.Total != 1 || len(rows.Observations) != 1 || rows.Observations[0].ModelID != "" {
		t.Fatalf("unmatched imported results unavailable to the table: %+v %v", rows, err)
	}
}

func TestBenchmarkObservationTablePaginationPreservesUnmatchedAndProvenance(t *testing.T) {
	source := consoleBenchmarkSource{id: "aa", fetch: func(context.Context) (intelligence.Result, error) {
		return intelligence.Result{Observations: []intelligence.Observation{consoleBenchmarkObservation("model-a"), consoleBenchmarkObservation("unknown")}}, nil
	}}
	handler, m := consoleBenchmarkManager(t, []intelligence.Source{source})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	w := get("/gateway/v1/intelligence/observations?offset=1&limit=1")
	var body struct {
		Total        int
		Observations []intelligence.Observation
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Total != 2 || len(body.Observations) != 1 || body.Observations[0].ExternalModelID != "unknown" || body.Observations[0].ModelID != "" {
		t.Fatalf("table=%s", w.Body.String())
	}
	if body.Observations[0].Metadata != nil || body.Observations[0].Score != 90 || body.Observations[0].RetrievedAt.IsZero() {
		t.Fatal("table provenance or metadata filtering incorrect")
	}
	w = get("/gateway/v1/intelligence/observations?include_metadata=true&limit=25")
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Observations[0].Metadata["large_upstream_row"] != "not needed in table" {
		t.Fatal("complete table omitted captured metadata")
	}
	w = get("/gateway/v1/intelligence/observations?q=MODEL-A")
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || body.Observations[0].ModelID != "local/model-a" {
		t.Fatalf("filter=%s", w.Body.String())
	}
	for _, query := range []string{"limit=0", "limit=501", "offset=-1", "limit=bad", "offset=bad", "include_metadata=invalid", "q=" + strings.Repeat("a", 257)} {
		if w := get("/gateway/v1/intelligence/observations?" + query); w.Code != 400 {
			t.Fatalf("query=%s status=%d", query, w.Code)
		}
	}
	for _, path := range []string{"/gateway/v1/intelligence/sources", "/gateway/v1/intelligence/models"} {
		w := get(path)
		if !strings.Contains(w.Body.String(), `"observation_count":`) {
			t.Fatalf("missing count: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), "large_upstream_row") {
			t.Fatal("table summary includes raw metadata")
		}
	}
	// Paging must not mutate the underlying provenance.
	if m.Snapshot().Sources["aa"].Observations[0].Metadata == nil {
		t.Fatal("pagination mutated stored data")
	}
}

func TestBenchmarkModelTableCombinesScoresAndKeepsLegacyRoutingAPIUnchanged(t *testing.T) {
	source := consoleBenchmarkSource{id: "aa", fetch: func(context.Context) (intelligence.Result, error) {
		coding := consoleBenchmarkObservation("claude-opus-5")
		agentic, knowledge := coding, coding
		agentic.Benchmark, agentic.Category, agentic.Normalized = "tools", intelligence.Agentic, benchmarkPtr(.8)
		knowledge.Benchmark, knowledge.Category, knowledge.Normalized = "knowledge", intelligence.Knowledge, benchmarkPtr(.95)
		return intelligence.Result{Observations: []intelligence.Observation{coding, agentic, knowledge, consoleBenchmarkObservation("model-a")}}, nil
	}}
	handler, m := consoleBenchmarkManager(t, []intelligence.Source{source})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	w := get("/gateway/v1/intelligence/models?include_unmatched=true")
	var rows map[string]intelligence.TableModel
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || w.Code != 200 || len(rows) != 2 {
		t.Fatalf("model table: %d %s %v", w.Code, w.Body.String(), err)
	}
	model := rows["external/claude-opus-5"]
	if model.Scores[intelligence.Coding] != .9 || model.Scores[intelligence.Agentic] != .8 || model.Scores[intelligence.Knowledge] != .95 || model.ObservationCount != 3 || model.Observations != nil {
		t.Fatalf("scores not combined: %+v", model)
	}
	if strings.Contains(w.Body.String(), "large_upstream_row") {
		t.Fatal("hidden metadata returned in summary")
	}
	w = get("/gateway/v1/intelligence/models")
	rows = nil
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("legacy models API changed: %s %v", w.Body.String(), err)
	}
	if w := get("/gateway/v1/intelligence/models/external/claude-opus-5"); w.Code != 404 {
		t.Fatal("unmatched summary exposed as a routable model")
	}
	if w := get("/gateway/v1/intelligence/models?include_unmatched=bad"); w.Code != 400 {
		t.Fatal("invalid summary scope accepted")
	}
	if len(m.Snapshot().Sources["aa"].Observations) != 4 {
		t.Fatal("table discarded the raw observations")
	}
}

func TestBenchmarkIngestionChangesSmartRoutingWithoutFetchingOnInference(t *testing.T) {
	var benchmarkFetches atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/benchmarks" {
			benchmarkFetches.Add(1)
			_, _ = io.WriteString(w, `{"data":[{"slug":"model-a","name":"Model A","evaluations":{"artificial_analysis_coding_index":20}},{"slug":"model-b","name":"Model B","evaluations":{"artificial_analysis_coding_index":95}}]}`)
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{"model": body["model"], "choices": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	physical, err := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "test", BaseURL: upstream.URL, APIPath: "/v1", StaticModels: []string{"model-a", "model-b"}})
	if err != nil {
		t.Fatal(err)
	}
	var overrides []catalog.Descriptor
	for _, id := range []string{"model-a", "model-b"} {
		price := 1.0
		if id == "model-b" {
			price = 10
		}
		overrides = append(overrides, catalog.Descriptor{PublicID: "test/" + id, ProviderID: "test", UpstreamID: id, Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Quality: catalog.Quality{General: benchmarkPtr(.9)}, Pricing: catalog.Pricing{InputPerMillion: &price, OutputPerMillion: &price}})
	}
	handler, err := api.NewOpenAIHandlerWithOverrides([]provider.OpenAIProvider{physical}, overrides)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.RefreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler.SetClassifier(classifier.Runner{Classifier: codingClassifier{}, OnFailure: classifier.FailureFail})
	policy := router.DefaultSelectionPolicy()
	policy.Intelligence = router.IntelligenceWeights{TaskWeights: map[string]map[string]float64{"coding": {intelligence.Coding: 1}}}
	handler.SetSelectionPolicy(policy)
	source, err := intelligence.Builtins().New(intelligence.SourceSettings{ID: "aa", Type: "artificial_analysis", URL: upstream.URL + "/benchmarks"}, "test-token", intelligence.FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	database, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store, err := intelligence.OpenStore(context.Background(), database, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := intelligence.NewManager(store, []intelligence.Source{source}, handler.CatalogModels(), intelligence.Options{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	defer manager.Close()
	handler.SetIntelligence(manager)
	check := func(session, model, want string) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Write some code"}]}`, model)))
		r.Header.Set("X-Session-ID", session)
		handler.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"model":"`+want+`"`) {
			t.Fatalf("route=%d %s want=%s", w.Code, w.Body.String(), want)
		}
	}
	check("before", "smart", "model-a")
	if benchmarkFetches.Load() != 0 {
		t.Fatal("inference fetched benchmark data")
	}
	if err := manager.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	check("after", "smart", "model-b")
	check("explicit", "test/model-a", "model-a")
	if benchmarkFetches.Load() != 1 {
		t.Fatal("normal inference fetched benchmarks")
	}
	handler.SetIntelligence(nil)
	check("unavailable", "smart", "model-a")
	handler.SetIntelligence(manager)
	policy.Intelligence = router.IntelligenceWeights{}
	handler.SetSelectionPolicy(policy)
	check("disabled", "smart", "model-a")
	for _, path := range []string{"/gateway/v1/intelligence", "/gateway/v1/intelligence/sources", "/gateway/v1/intelligence/models", "/gateway/v1/intelligence/models/test/model-b", "/gateway/v1/intelligence/unmatched"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 {
			t.Fatalf("inspection %s = %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/gateway/v1/intelligence/refresh", nil))
	if w.Code != 200 || benchmarkFetches.Load() != 2 {
		t.Fatal("manual refresh API did not retrieve data")
	}
	// Closing SQLite must not affect inference: routing uses the in-memory view.
	manager.Close()
	policy.Intelligence = router.IntelligenceWeights{TaskWeights: map[string]map[string]float64{"coding": {intelligence.Coding: 1}}}
	handler.SetSelectionPolicy(policy)
	check("closed-store", "smart", "model-b")
	store, err = intelligence.OpenStore(context.Background(), database, nil)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := intelligence.NewManager(store, []intelligence.Source{source}, handler.CatalogModels(), intelligence.Options{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	defer restarted.Close()
	handler.SetIntelligence(restarted)
	check("restarted-cache", "smart", "model-b")
	if benchmarkFetches.Load() != 2 {
		t.Fatal("startup or inference fetched benchmarks")
	}
}
