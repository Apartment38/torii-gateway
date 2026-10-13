package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

type mappingProvider struct {
	calls            int
	reply            string
	confidence       float64
	sizes            []int
	chooseEveryBatch bool
}

func (p *mappingProvider) ID() string { return "connected" }
func (p *mappingProvider) Models(context.Context) ([]provider.Model, error) {
	return []provider.Model{
		{ID: "connected/gemma4:e2b"}, {ID: "connected/no-candidates"}, {ID: "connected/decider", Decision: true, Discovery: &provider.ModelDiscovery{Endpoints: map[string]bool{"systemone": true}}},
	}, nil
}
func (p *mappingProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	return nil, fmt.Errorf("chat should not be called")
}
func (p *mappingProvider) ForwardDecision(_ context.Context, body []byte) (*http.Response, error) {
	p.calls++
	var request struct {
		Model     string
		Questions map[string]struct{ Criteria map[string]string }
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	if request.Model != "decider" {
		return nil, fmt.Errorf("wrong upstream model %s", request.Model)
	}
	criteria := request.Questions["match"].Criteria
	p.sizes = append(p.sizes, len(criteria))
	choice := "none"
	if p.chooseEveryBatch {
		choice = "candidate_0"
	}
	for key, id := range criteria {
		if !strings.HasPrefix(strings.ToLower(id), "gemma") && key != "none" {
			return nil, fmt.Errorf("unrelated option %s", id)
		}
		if id == p.reply {
			choice = key
		}
	}
	probabilities := map[string]float64{}
	for key := range criteria {
		probabilities[key] = 0
	}
	probabilities[choice] = 1
	data, _ := json.Marshal(map[string]any{"answers": map[string]any{"match": map[string]any{"type": "choice", "choice": choice, "confidence": p.confidence, "probabilities": probabilities}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
}
func TestSmartMappingShortlistsAndValidates(t *testing.T) {
	for _, scenario := range []struct {
		name, reply string
		confidence  float64
		want        int
	}{{"match", "gemma-4-e2b", .99, 1}, {"none", "unrelated", 1, 0}, {"uncertain", "gemma-4-e2b", .4, 0}, {"invalid", "gemma-4-e2b", 2, 0}} {
		t.Run(scenario.name, func(t *testing.T) {
			p := &mappingProvider{reply: scenario.reply, confidence: scenario.confidence}
			h, err := api.NewOpenAIHandler([]provider.OpenAIProvider{p})
			if err != nil {
				t.Fatal(err)
			}
			_, manager := consoleBenchmarkManager(t, []intelligence.Source{consoleBenchmarkSource{id: "fixture", fetch: func(context.Context) (intelligence.Result, error) {
				return intelligence.Result{Observations: []intelligence.Observation{consoleBenchmarkObservation("gemma-4-e2b"), consoleBenchmarkObservation("gemma-3-27b"), consoleBenchmarkObservation("qwen-3-4b")}}, nil
			}}})
			h.SetIntelligence(manager)
			if err := h.RefreshCatalog(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := manager.Refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/gateway/v1/intelligence/smart-mapping", strings.NewReader(`{"model":"connected/decider"}`)))
			var result struct {
				Mappings                  []intelligence.Identity
				Requests, Checked, Failed int
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if len(result.Mappings) != scenario.want || p.calls != 1 || result.Requests != 1 || result.Checked != 2 {
				t.Fatalf("result %+v calls %d", result, p.calls)
			}
			if scenario.want > 0 && result.Mappings[0].SourceIDs["fixture"][0] != "gemma-4-e2b" {
				t.Fatal(result.Mappings)
			}
			if len(manager.Snapshot().Unmatched) != 3 {
				t.Fatal("proposal endpoint changed mappings before configuration save")
			}
			for _, model := range []string{"connected/gemma4:e2b", "missing/decider"} {
				w = httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("POST", "/gateway/v1/intelligence/smart-mapping", strings.NewReader(fmt.Sprintf(`{"model":%q}`, model))))
				if w.Code != 400 || p.calls != 1 {
					t.Fatalf("invalid model made request: %d %d", w.Code, p.calls)
				}
			}
			h.SetModelControls(map[string]bool{"connected/decider": false}, nil)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/gateway/v1/intelligence/smart-mapping", strings.NewReader(`{"model":"connected/decider"}`)))
			if w.Code != 400 || p.calls != 1 {
				t.Fatal("disabled decision model accepted")
			}
		})
	}
}

func TestSmartMappingBatchesAndRejectsConflictingMatches(t *testing.T) {
	p := &mappingProvider{confidence: 1, chooseEveryBatch: true}
	h, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{p})
	observations := []intelligence.Observation{}
	for i := 0; i < 401; i++ {
		observations = append(observations, consoleBenchmarkObservation(fmt.Sprintf("gemma-variant-%03d", i)))
	}
	_, manager := consoleBenchmarkManager(t, []intelligence.Source{consoleBenchmarkSource{id: "fixture", fetch: func(context.Context) (intelligence.Result, error) {
		return intelligence.Result{Observations: observations}, nil
	}}})
	h.SetIntelligence(manager)
	h.RefreshCatalog(context.Background())
	manager.Refresh(context.Background())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/gateway/v1/intelligence/smart-mapping", strings.NewReader(`{"model":"connected/decider"}`)))
	var result struct {
		Mappings            []intelligence.Identity
		Requests, Ambiguous int
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if result.Requests != 3 || result.Ambiguous != 1 || len(result.Mappings) != 0 {
		t.Fatalf("%+v", result)
	}
	for _, size := range p.sizes {
		if size > 201 {
			t.Fatal("unbounded request", size)
		}
	}
}
