package intelligence

import "context"

type benchLM struct{ *remote }

func newBenchLM(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
	cfg = WithSourceDefaults(cfg)
	r, err := newRemote(cfg, key, "https://benchlm.ai/data/models.json", options)
	return &benchLM{r}, err
}
func (b *benchLM) Fetch(ctx context.Context) (Result, error) {
	data, err := b.get(ctx, b.cfg.URL, "Authorization")
	if err != nil {
		return Result{}, err
	}
	return parseBenchLM(data, b.cfg)
}

var benchCategories = map[string]string{"agentic": Agentic, "coding": Coding, "reasoning": Reasoning, "math": Math, "knowledge": Knowledge, "instructionFollowing": Instruction, "multimodalGrounded": Multimodal}

func parseBenchLM(data []byte, cfg SourceSettings) (Result, error) {
	doc, err := decodeObject(data)
	if err != nil {
		return Result{}, err
	}
	rows, ok := doc["items"].([]any)
	if !ok {
		rows, ok = doc["models"].([]any)
	}
	if !ok {
		return Result{}, &SourceError{Stage: "BenchLM schema parsing"}
	}
	result := Result{Metadata: map[string]any{}}
	for _, key := range []string{"license", "licenseUrl", "attribution", "commercialUse", "schemaVersion", "methodologyVersion", "generatedAt", "lastUpdated", "sourceAttributions", "uncertaintyPolicy"} {
		if value, ok := doc[key]; ok {
			result.Metadata[key] = value
		}
	}
	for _, raw := range rows {
		row := object(raw)
		model := text(row["canonicalModelKey"])
		if model == "" {
			model = text(row["slug"])
		}
		if model == "" {
			model = text(row["model"])
		}
		if model == "" {
			return Result{}, &SourceError{Stage: "BenchLM identity parsing"}
		}
		// Build/retrieval dates are not evaluation dates. Never renew old
		// benchmark evidence merely because the website was rebuilt today.
		date := parseDate(row["evaluatedAt"])
		version := text(row["methodologyVersion"])
		if version == "" {
			version = text(doc["methodologyVersion"])
		}
		metadata := map[string]any{"model_name": row["model"], "creator": row["creator"], "evidence_status": row["evidenceStatus"], "interval90": row["interval90"], "benchmark_sources": row["benchmarkSources"], "release_date": row["releaseDate"]}
		trusted := text(row["evidenceStatus"]) == "supported" || cfg.IncludeEstimated
		appendScore := func(name, category string, value any, scale *Mapping) error {
			score, ok := number(value)
			if !ok {
				return nil
			}
			if rule, ok := cfg.Metrics[name]; ok {
				scale = &rule
			}
			o, err := measurement(model, name, category, score, scale, date, version, metadata)
			if err != nil {
				return err
			}
			if !trusted {
				o.Normalized = nil
			}
			result.Observations = append(result.Observations, o)
			return nil
		}
		value := row["overallScore"]
		if value == nil {
			value = row["displayScore"]
		}
		if err := appendScore("benchalign.overall", General, value, &Mapping{Category: General, Max: 100}); err != nil {
			return Result{}, err
		}
		categories := object(row["categoryScores"])
		if categories == nil {
			categories = object(object(row["scores"])["verifiedDisplayCategoryScores"])
		}
		for _, key := range sortedKeys(categories) {
			category := benchCategories[key]
			if category == "" {
				category = "unknown"
			}
			if err := appendScore("benchalign."+key, category, categories[key], &Mapping{Category: category, Max: 100}); err != nil {
				return Result{}, err
			}
		}
		// Unknown raw benchmark scales are deliberately not inferred from their
		// numeric magnitude. Configure metrics to normalize those measurements.
		benchmarks := object(row["benchmarks"])
		for _, group := range sortedKeys(benchmarks) {
			category := benchCategories[group]
			if category == "" {
				category = "unknown"
			}
			for _, name := range sortedKeys(object(benchmarks[group])) {
				if err := appendScore(group+"."+name, category, object(benchmarks[group])[name], nil); err != nil {
					return Result{}, err
				}
			}
		}
		for _, field := range []string{"inputPrice", "outputPrice", "contextWindowTokens", "tokensPerSecond", "latencySeconds"} {
			if score, ok := number(row[field]); ok && (score > 0 || field == "inputPrice" || field == "outputPrice") {
				metric := map[string]string{"inputPrice": "input_price", "outputPrice": "output_price", "contextWindowTokens": "context_length", "tokensPerSecond": "throughput", "latencySeconds": "latency"}[field]
				result.Observations = append(result.Observations, Observation{ExternalModelID: model, Benchmark: metric, Category: Operational, Score: score, Unit: operationUnit(metric), EvaluatedAt: date, Version: version, Metadata: map[string]any{"upstream_field": field}})
			}
		}
	}
	return result, nil
}
