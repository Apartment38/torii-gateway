package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
)

type codingClassifier struct{}

func (codingClassifier) ID() string { return "test/classifier" }
func (codingClassifier) Classify(context.Context, classifier.ClassificationRequest) (classifier.Classification, error) {
	return classifier.Classification{TaskType: classifier.TaskCoding, Complexity: classifier.ComplexityMedium, Confidence: 1}, nil
}
func benchmarkPtr[T any](value T) *T { return &value }

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
	store, err := intelligence.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := intelligence.NewManager(store, []intelligence.Source{source}, handler.CatalogModels(), intelligence.Options{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
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
}
