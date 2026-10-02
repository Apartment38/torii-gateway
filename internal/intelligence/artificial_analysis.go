package intelligence

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type artificialAnalysis struct{ *remote }

func newArtificialAnalysis(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
	cfg = WithSourceDefaults(cfg)
	r, err := newRemote(cfg, key, "https://artificialanalysis.ai/api/v2/data/llms/models", options)
	return &artificialAnalysis{r}, err
}

var aaMetrics = map[string]Mapping{
	"artificial_analysis_intelligence_index": {Category: General, Max: 100},
	"artificial_analysis_coding_index":       {Category: Coding, Max: 100},
	"artificial_analysis_agentic_index":      {Category: Agentic, Max: 100},
	"artificial_analysis_math_index":         {Category: Math, Max: 100},
	"mmlu_pro":                               {Category: Knowledge, Max: 1}, "gpqa": {Category: Reasoning, Max: 1}, "gpqa_diamond": {Category: Reasoning, Max: 1},
	"hle": {Category: Knowledge, Max: 1}, "livecodebench": {Category: Coding, Max: 1}, "scicode": {Category: Coding, Max: 1},
	"math_500": {Category: Math, Max: 1}, "aime": {Category: Math, Max: 1}, "aime_25": {Category: Math, Max: 1},
	"ifbench": {Category: Instruction, Max: 1}, "mmmu_pro": {Category: Multimodal, Max: 1},
	"artificial_analysis_reasoning_index": {Category: Reasoning, Max: 100}, "artificial_analysis_knowledge_index": {Category: Knowledge, Max: 100},
	"artificial_analysis_instruction_following_index": {Category: Instruction, Max: 100}, "artificial_analysis_multimodal_index": {Category: Multimodal, Max: 100},
}

func (a *artificialAnalysis) Fetch(ctx context.Context) (Result, error) {
	result := Result{}
	for page := 1; page <= 100; page++ {
		u, _ := url.Parse(a.cfg.URL)
		if page > 1 {
			query := u.Query()
			query.Set("page", fmt.Sprint(page))
			u.RawQuery = query.Encode()
		}
		data, err := a.get(ctx, u.String(), "x-api-key")
		if err != nil {
			return Result{}, err
		}
		parsed, more, err := parseArtificialAnalysis(data, a.cfg.Metrics)
		if err != nil {
			return Result{}, err
		}
		result.Observations = append(result.Observations, parsed.Observations...)
		result.Metadata = parsed.Metadata
		if len(result.Observations) > a.cfg.MaxRows*32 {
			return Result{}, &SourceError{Stage: "source observation limit"}
		}
		if !more {
			return result, nil
		}
	}
	return Result{}, &SourceError{Stage: "pagination limit"}
}

func parseArtificialAnalysis(data []byte, overrides map[string]Mapping) (Result, bool, error) {
	document, err := decodeObject(data)
	if err != nil {
		return Result{}, false, err
	}
	rows, ok := document["data"].([]any)
	if !ok {
		return Result{}, false, &SourceError{Stage: "Artificial Analysis schema parsing"}
	}
	result := Result{Metadata: map[string]any{"intelligence_index_version": document["intelligence_index_version"], "tier": document["tier"]}}
	version := fmt.Sprint(document["intelligence_index_version"])
	if document["intelligence_index_version"] == nil {
		version = ""
	}
	for _, raw := range rows {
		row := object(raw)
		model := text(row["slug"])
		if model == "" {
			model = text(row["name"])
		}
		// A source slug can omit reasoning effort. Do not silently attach a
		// high/max-effort result to an unqualified base model.
		if name := text(row["name"]); strings.Contains(name, "(") {
			model = name
		}
		if model == "" {
			return Result{}, false, &SourceError{Stage: "Artificial Analysis model identity parsing"}
		}
		date := parseDate(row["evaluated_at"])
		for _, metric := range sortedKeys(object(row["evaluations"])) {
			score, ok := number(object(row["evaluations"])[metric])
			if !ok {
				continue
			}
			mapping, known := aaMetrics[metric]
			if override, ok := overrides[metric]; ok {
				mapping, known = override, true
			}
			category := "unknown"
			var rule *Mapping
			if known {
				category = mapping.Category
				rule = &mapping
			}
			o, err := measurement(model, metric, category, score, rule, date, version, map[string]any{"model_name": row["name"], "source_slug": row["slug"], "creator": row["model_creator"], "release_date": row["release_date"]})
			if err != nil {
				return Result{}, false, err
			}
			result.Observations = append(result.Observations, o)
		}
		operations := map[string]string{"pricing.price_1m_input_tokens": "input_price", "pricing.price_1m_output_tokens": "output_price", "context_window": "context_length", "context_window_tokens": "context_length", "performance.median_output_tokens_per_second": "throughput", "performance.median_time_to_first_token_seconds": "ttft", "performance.median_end_to_end_response_time_seconds": "latency", "median_output_tokens_per_second": "throughput", "median_time_to_first_token_seconds": "ttft"}
		for _, path := range sortedStringMapKeys(operations) {
			if score, ok := number(pathValue(row, path)); ok {
				metric := operations[path]
				unit := "seconds"
				if metric == "input_price" || metric == "output_price" {
					unit = "USD_per_million_tokens"
				}
				if metric == "throughput" {
					unit = "tokens_per_second"
				}
				if metric == "context_length" {
					unit = "tokens"
				}
				result.Observations = append(result.Observations, Observation{ExternalModelID: model, Benchmark: metric, Category: Operational, Score: score, Unit: unit, EvaluatedAt: date, Version: version, Metadata: map[string]any{"upstream_field": path}})
			}
		}
	}
	more, _ := object(document["pagination"])["has_more"].(bool)
	return result, more, nil
}
func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}
