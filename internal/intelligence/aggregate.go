package intelligence

import (
	"math"
	"sort"
	"time"
)

type average struct{ sum, weight, count float64 }

func (a *average) add(value, weight float64) { a.sum += value * weight; a.weight += weight; a.count++ }
func weight(weights map[string]float64, key string) float64 {
	if v, ok := weights[key]; ok {
		return v
	}
	return 1
}
func evidenceTime(o Observation) time.Time {
	if !o.EvaluatedAt.IsZero() {
		return o.EvaluatedAt
	}
	return o.RetrievedAt
}

// Aggregate is pure and source-agnostic. Each benchmark is averaged first,
// then each source/category, then sources. Question-level datasets cannot
// overwhelm a category simply by providing more rows than another source.
func Aggregate(observations []Observation, cfg Aggregation, now time.Time, halfLife, maxAge, staleAfter time.Duration) Model {
	defaults := (Options{HalfLife: halfLife, MaxAge: maxAge, StaleAfter: staleAfter}).Defaults()
	halfLife, maxAge, staleAfter = defaults.HalfLife, defaults.MaxAge, defaults.StaleAfter
	m := Model{Scores: map[string]float64{}, Operations: map[string]float64{}, Observations: observations}
	latest := map[string]Observation{}
	for _, o := range observations {
		at := evidenceTime(o)
		if at.After(m.LatestEvaluation) {
			m.LatestEvaluation = at
		}
		if o.RetrievedAt.After(m.LatestRetrieval) {
			m.LatestRetrieval = o.RetrievedAt
		}
		key := o.Source + "\x00" + o.Benchmark + "\x00" + o.Category
		// Retain per-question measurements while deduplicating reruns.
		if id, ok := o.Metadata["question_id"].(string); ok {
			key += "\x00" + id
		}
		if previous, ok := latest[key]; !ok || evidenceTime(previous).Before(at) || (evidenceTime(previous).Equal(at) && previous.RetrievedAt.Before(o.RetrievedAt)) {
			latest[key] = o
		}
	}
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	benchmarks := map[string]*average{}
	benchmarkInfo := map[string]Observation{}
	for _, key := range keys {
		o := latest[key]
		age := now.Sub(evidenceTime(o))
		if age > maxAge || now.Sub(o.RetrievedAt) > staleAfter || weight(cfg.SourceWeights, o.Source) == 0 || weight(cfg.BenchmarkWeights, o.Source+"/"+o.Benchmark) == 0 {
			continue
		}
		if o.Category == Operational || o.Normalized != nil {
			until := o.RetrievedAt.Add(staleAfter)
			if at := evidenceTime(o).Add(maxAge); at.Before(until) {
				until = at
			}
			if m.ValidUntil.IsZero() || until.Before(m.ValidUntil) {
				m.ValidUntil = until
			}
		}
		if o.Category == Operational {
			previous, exists := benchmarkInfo["op\x00"+o.Benchmark]
			if !exists || evidenceTime(previous).Before(evidenceTime(o)) {
				m.Operations[o.Benchmark] = o.Score
				benchmarkInfo["op\x00"+o.Benchmark] = o
			}
			continue
		}
		if o.Normalized == nil {
			continue
		}
		key = o.Source + "\x00" + o.Category + "\x00" + o.Benchmark
		if benchmarks[key] == nil {
			benchmarks[key] = &average{}
		}
		freshness := math.Exp2(-math.Max(0, age.Seconds()) / halfLife.Seconds())
		benchmarks[key].add(*o.Normalized, freshness)
		benchmarkInfo[key] = o
	}
	sourceCategories := map[string]*average{}
	sourceRawWeights := map[string]float64{}
	keys = keys[:0]
	for key := range benchmarks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		a, o := benchmarks[key], benchmarkInfo[key]
		categoryKey := o.Source + "\x00" + o.Category
		if sourceCategories[categoryKey] == nil {
			sourceCategories[categoryKey] = &average{}
		}
		w := weight(cfg.BenchmarkWeights, o.Source+"/"+o.Benchmark)
		// The mean freshness weight preserves ageing across sources, rather
		// than cancelling it out in a single-source mean.
		freshness := a.weight / a.count
		if a.weight > 0 {
			sourceCategories[categoryKey].add(a.sum/a.weight, w*freshness)
			sourceRawWeights[categoryKey] += w
		}
	}
	categoryTotals := map[string]*average{}
	keys = keys[:0]
	for key := range sourceCategories {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		a := sourceCategories[key]
		if a.weight <= 0 {
			continue
		}
		// Source/category info is independent of source-specific schemas.
		var source, category string
		for i, ch := range key {
			if ch == 0 {
				source, category = key[:i], key[i+1:]
				break
			}
		}
		if categoryTotals[category] == nil {
			categoryTotals[category] = &average{}
		}
		// Use average benchmark freshness; row count never changes source weight.
		if sourceRawWeights[key] > 0 {
			categoryTotals[category].add(a.sum/a.weight, weight(cfg.SourceWeights, source)*a.weight/sourceRawWeights[key])
		}
	}
	overall := average{}
	for _, category := range Categories {
		if a := categoryTotals[category]; a != nil && a.weight > 0 {
			m.Scores[category] = a.sum / a.weight
			w := weight(cfg.OverallWeights, category)
			if category == Preference {
				w = cfg.OverallWeights[Preference]
			} // explicit opt-in only
			overall.add(m.Scores[category], w)
		}
	}
	if overall.weight > 0 {
		m.Scores["overall"] = overall.sum / overall.weight
	}
	m.AgeSeconds = math.Max(0, now.Sub(m.LatestEvaluation).Seconds())
	m.Stale = m.LatestRetrieval.IsZero() || now.Sub(m.LatestRetrieval) > staleAfter || m.AgeSeconds > maxAge.Seconds()
	return m
}
