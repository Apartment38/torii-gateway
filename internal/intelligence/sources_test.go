package intelligence

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func find(t *testing.T, observations []Observation, name string) Observation {
	t.Helper()
	for _, o := range observations {
		if o.Benchmark == name {
			return o
		}
	}
	t.Fatalf("observation %s missing", name)
	return Observation{}
}
func near(t *testing.T, actual, want float64) {
	t.Helper()
	if math.Abs(actual-want) > 1e-9 {
		t.Fatalf("score=%v want=%v", actual, want)
	}
}

func TestNormalizeDeclaredScales(t *testing.T) {
	for _, tc := range []struct {
		score, min, max, want float64
		lower                 bool
	}{{75, 0, 100, .75, false}, {.8, 0, 1, .8, false}, {1, 0, 4, .75, true}, {0, -1, 1, .5, false}} {
		score, err := Normalize(tc.score, tc.min, tc.max, tc.lower)
		if err != nil {
			t.Fatal(err)
		}
		near(t, *score, tc.want)
	}
	for _, score := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		if _, err := Normalize(score, 0, 100, false); err == nil {
			t.Fatal("invalid score accepted")
		}
	}
	if _, err := Normalize(1, 1, 1, false); err == nil {
		t.Fatal("invalid scale accepted")
	}
	for _, bounds := range [][2]float64{{math.NaN(), 100}, {0, math.NaN()}, {math.Inf(-1), 100}, {0, math.Inf(1)}} {
		if _, err := Normalize(1, bounds[0], bounds[1], false); err == nil {
			t.Fatal("non-finite scale accepted")
		}
	}
}

func TestArtificialAnalysisParserPreservesScoresAndOperationalData(t *testing.T) {
	result, more, err := parseArtificialAnalysis(fixture(t, "artificial_analysis"), nil)
	if err != nil || more {
		t.Fatalf("parse=%v more=%v", err, more)
	}
	coding := find(t, result.Observations, "artificial_analysis_coding_index")
	near(t, *coding.Normalized, .88)
	if coding.Category != Coding || coding.Version != "4.1" || !coding.EvaluatedAt.IsZero() {
		t.Fatalf("incorrect category/version or release date used as evaluation date: %+v", coding)
	}
	unknown := find(t, result.Observations, "new_unknown_benchmark")
	if unknown.Normalized != nil || unknown.Score != 1200 {
		t.Fatal("unknown scale discarded or guessed")
	}
	for metric, expected := range map[string]float64{"input_price": 1.2, "output_price": 4.8, "throughput": 100, "ttft": .8, "latency": 4, "context_length": 128000} {
		o := find(t, result.Observations, metric)
		if o.Category != Operational || o.Score != expected {
			t.Fatalf("incorrect operation %s", metric)
		}
	}
	data := strings.Replace(string(fixture(t, "artificial_analysis")), `"Model A"`, `"Model A (high)"`, 1)
	result, _, err = parseArtificialAnalysis([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Observations[0].ExternalModelID != "Model A (high)" {
		t.Fatal("effort variant silently collapsed")
	}
	if _, _, err := parseArtificialAnalysis([]byte(`{"data":"changed"}`), nil); err == nil {
		t.Fatal("schema change accepted")
	}
}

func TestBenchLMRawResultsScalesEstimatesAndLicensing(t *testing.T) {
	result, err := parseBenchLM(fixture(t, "benchlm"), SourceSettings{Metrics: map[string]Mapping{"coding.customAccuracy": {Category: Coding, Max: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	near(t, *find(t, result.Observations, "coding.customAccuracy").Normalized, .85)
	if find(t, result.Observations, "coding.customFraction").Normalized != nil {
		t.Fatal("ambiguous raw scale guessed")
	}
	if find(t, result.Observations, "agentic.toolUseElo").Score != 1400 {
		t.Fatal("raw rating discarded")
	}
	if result.Metadata["license"] != "CC BY-NC 4.0" {
		t.Fatal("licensing metadata lost")
	}
	for _, o := range result.Observations {
		if o.ExternalModelID == "model-estimated" && o.Normalized != nil {
			t.Fatal("estimated evidence silently treated as measured")
		}
	}
	result, err = parseBenchLM(fixture(t, "benchlm"), SourceSettings{IncludeEstimated: true})
	if err != nil {
		t.Fatal(err)
	}
	var estimated bool
	for _, o := range result.Observations {
		if o.ExternalModelID == "model-estimated" && o.Normalized != nil {
			estimated = true
		}
	}
	if !estimated {
		t.Fatal("explicit estimated opt-in ignored")
	}
}

func TestArenaAndLiveBenchUsePublishedStructuredRows(t *testing.T) {
	for _, sourceType := range []string{"lmarena", "livebench"} {
		t.Run(sourceType, func(t *testing.T) {
			name := "arena"
			if sourceType == "livebench" {
				name = "livebench"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("length") != "100" {
					t.Error("incorrect dataset page size")
				}
				_, _ = w.Write(fixture(t, name))
			}))
			defer server.Close()
			source, err := Builtins().New(SourceSettings{ID: sourceType, Type: sourceType, URL: server.URL}, "", FetchOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := source.Fetch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if sourceType == "lmarena" {
				for _, o := range result.Observations {
					if o.Category != Preference {
						t.Fatal("Arena coding rating treated as objective coding")
					}
				}
				near(t, *result.Observations[0].Normalized, 10.0/11)
				if result.Observations[0].Metadata["row"].(map[string]any)["rating_lower"] != float64(1380) {
					t.Fatal("confidence bounds lost")
				}
			} else {
				if result.Observations[0].Category != Coding || result.Observations[0].EvaluatedAt.Year() != 2026 || result.Observations[2].Category != Math {
					t.Fatal("LiveBench provenance/category lost")
				}
			}
		})
	}
}

func TestHuggingFaceConfigurableMappingsAndCompletePagination(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-hf-token" {
			t.Error("missing dataset token")
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		n := min(100, 205-offset)
		rows := make([]any, 0, n)
		for i := range n {
			rows = append(rows, map[string]any{"row": map[string]any{"id": "model-a", "acc": float64((i + offset) % 101), "date": "2026-09-01"}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows, "num_rows_total": 205})
	}))
	defer server.Close()
	source, err := Builtins().New(SourceSettings{ID: "hf", Type: "huggingface", URL: server.URL, Datasets: []Dataset{{ID: "org/leaderboard", Config: "default", Split: "train", ModelField: "id", DateField: "date", Metrics: map[string]Mapping{"acc": {Category: Knowledge, Max: 100}}}}}, "test-hf-token", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 205 || calls.Load() != 3 {
		t.Fatalf("incomplete pagination: %d calls=%d", len(result.Observations), calls.Load())
	}
	if result.Observations[204].Benchmark != "org/leaderboard/acc" || result.Observations[204].Category != Knowledge {
		t.Fatal("dataset mapping ignored")
	}
}

func TestRetriesCancellationNoRedirectSecretsAndPaginationFailure(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporary", 503)
			return
		}
		if r.Header.Get("x-api-key") != "test-aa-token" {
			t.Error("missing AA token")
		}
		_, _ = w.Write(fixture(t, "artificial_analysis"))
	}))
	defer server.Close()
	source, err := Builtins().New(SourceSettings{ID: "aa", Type: "artificial_analysis", URL: server.URL}, "test-aa-token", FetchOptions{Retries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Fetch(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatalf("retry failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Fetch(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	source, _ = Builtins().New(SourceSettings{ID: "aa", Type: "artificial_analysis", URL: redirect.URL}, "test-secret", FetchOptions{})
	if _, err := source.Fetch(context.Background()); err == nil || leaked.Load() {
		t.Fatal("credentials forwarded through redirect")
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "0" {
			_, _ = io.WriteString(w, `{"num_rows_total":101,"rows":[{"row":{"model_name":"model-a","rating":1000}}]}`)
		} else {
			http.Error(w, "missing page", 500)
		}
	}))
	defer broken.Close()
	source, _ = Builtins().New(SourceSettings{ID: "arena", Type: "lmarena", URL: broken.URL}, "", FetchOptions{})
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("incomplete pagination published")
	}
}

func TestHTTPTimeoutAndAuthenticationIsNotRetried(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		http.Error(w, "test-secret reflected by upstream", 401)
	}))
	defer server.Close()
	source, _ := Builtins().New(SourceSettings{ID: "aa", Type: "artificial_analysis", URL: server.URL}, "test-secret", FetchOptions{Retries: 3})
	if _, err := source.Fetch(context.Background()); err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatal("error missing or disclosed secret")
	}
	if calls.Load() != 1 {
		t.Fatal("authentication failure retried")
	}
	source, _ = Builtins().New(SourceSettings{ID: "aa", Type: "artificial_analysis", URL: server.URL + "/slow"}, "", FetchOptions{Timeout: 10 * time.Millisecond})
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("timeout ignored")
	}
}

func TestArenaIPSRemainsUnnormalizedWithoutExplicitScale(t *testing.T) {
	h := huggingFace{mode: "lmarena"}
	row := map[string]any{"model_name": "model-a", "score": -.1, "category": "overall"}
	observations, err := h.parseRow(Dataset{Config: "agent", ModelField: "model_name"}, row)
	if err != nil {
		t.Fatal(err)
	}
	if observations[0].Normalized != nil || observations[0].Category != Preference {
		t.Fatal("IPS score normalized using an Elo assumption")
	}
}

func TestRegistryExtensibleWithoutAggregationChanges(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("test", func(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
		return fakeSource{id: cfg.ID}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.New(SourceSettings{ID: "new", Type: "test"}, "", FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("test", nil); err == nil {
		t.Fatal("duplicate registration accepted")
	}
}
