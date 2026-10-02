package intelligence

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuiltInPresetsAreIndependentAndKeepExplicitDatasets(t *testing.T) {
	presets := SourcePresets()
	if len(presets) != 5 {
		t.Fatal("missing presets")
	}
	for _, preset := range presets {
		if !Builtins().Has(preset.Settings.Type) || ValidateURL(preset.Settings.URL) != nil {
			t.Fatalf("invalid preset: %s", preset.Name)
		}
	}
	cfg := WithSourceDefaults(SourceSettings{Type: "huggingface"})
	if len(cfg.Datasets) != 1 || cfg.Datasets[0].ID != "open-llm-leaderboard/contents" || len(cfg.Datasets[0].Metrics) != 6 {
		t.Fatal("Hugging Face defaults missing")
	}
	cfg.Datasets[0].Metrics["IFEval Raw"] = Mapping{Category: Math, Max: 99}
	if WithSourceDefaults(SourceSettings{Type: "huggingface"}).Datasets[0].Metrics["IFEval Raw"].Max != 1 {
		t.Fatal("defaults share mutable data")
	}
	custom := WithSourceDefaults(SourceSettings{Type: "huggingface", URL: "https://custom.example/rows", Datasets: []Dataset{{ID: "org/results", Config: "custom", Split: "eval"}}})
	if custom.URL != "https://custom.example/rows" || custom.Datasets[0].Config != "custom" {
		t.Fatal("operator configuration overwritten")
	}
	if WithSourceDefaults(SourceSettings{Type: "artificial_analysis", URL: "https://artificialanalysis.ai/api/v2/language/models/free"}).URL != "https://artificialanalysis.ai/api/v2/data/llms/models" {
		t.Fatal("legacy built-in API URL not updated")
	}
}

func TestHuggingFacePresetImportsPublishedRawColumnsWithoutManualMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataset") != "open-llm-leaderboard/contents" || r.URL.Query().Get("config") != "default" || r.URL.Query().Get("split") != "train" {
			t.Error("incorrect preset query")
		}
		fmt.Fprint(w, `{"num_rows_total":1,"rows":[{"row":{"fullname":"org/model","Model":"<a>not an identity</a>","IFEval Raw":0.9,"BBH Raw":0.8,"MATH Lvl 5 Raw":0.7,"GPQA Raw":0.6,"MUSR Raw":0.5,"MMLU-PRO Raw":0.4}}]}`)
	}))
	defer server.Close()
	source, err := Builtins().New(SourceSettings{ID: "hf", Type: "huggingface", URL: server.URL}, "", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := source.Fetch(context.Background())
	if err != nil || len(result.Observations) != 6 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, o := range result.Observations {
		if o.ExternalModelID != "org/model" || o.Normalized == nil || o.Version != "Open LLM Leaderboard v2" || !o.EvaluatedAt.IsZero() {
			t.Fatalf("incorrect provenance: %+v", o)
		}
	}
}
