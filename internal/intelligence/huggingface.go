package intelligence

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sync"
)

type huggingFace struct {
	*remote
	mode string
}

func newHuggingFace(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
	cfg = WithSourceDefaults(cfg)
	r, err := newRemote(cfg, key, "https://datasets-server.huggingface.co/rows", options)
	return &huggingFace{remote: r, mode: "huggingface"}, err
}
func newArena(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
	cfg = WithSourceDefaults(cfg)
	r, err := newRemote(cfg, key, "https://datasets-server.huggingface.co/rows", options)
	return &huggingFace{remote: r, mode: "lmarena"}, err
}
func newLiveBench(cfg SourceSettings, key string, options FetchOptions) (Source, error) {
	cfg = WithSourceDefaults(cfg)
	r, err := newRemote(cfg, key, "https://datasets-server.huggingface.co/rows", options)
	return &huggingFace{remote: r, mode: "livebench"}, err
}

type hfPage struct {
	rows  []map[string]any
	total int
}

func parseHFPage(data []byte) (hfPage, error) {
	doc, err := decodeObject(data)
	if err != nil {
		return hfPage{}, err
	}
	rows, ok := doc["rows"].([]any)
	if !ok {
		return hfPage{}, &SourceError{Stage: "Hugging Face rows schema parsing"}
	}
	total, ok := number(doc["num_rows_total"])
	if !ok || total < 0 || total != math.Trunc(total) {
		return hfPage{}, &SourceError{Stage: "Hugging Face row count parsing"}
	}
	page := hfPage{total: int(total)}
	for _, entry := range rows {
		if truncated, ok := object(entry)["truncated_cells"].([]any); ok && len(truncated) > 0 {
			return hfPage{}, &SourceError{Stage: "truncated dataset cells"}
		}
		row := object(object(entry)["row"])
		if row == nil {
			return hfPage{}, &SourceError{Stage: "Hugging Face row parsing"}
		}
		page.rows = append(page.rows, row)
	}
	return page, nil
}
func (h *huggingFace) page(ctx context.Context, dataset Dataset, offset int) (hfPage, error) {
	u, _ := url.Parse(h.cfg.URL)
	q := u.Query()
	q.Set("dataset", dataset.ID)
	q.Set("config", dataset.Config)
	q.Set("split", dataset.Split)
	q.Set("offset", fmt.Sprint(offset))
	q.Set("length", "100")
	u.RawQuery = q.Encode()
	data, err := h.get(ctx, u.String(), "Authorization")
	if err != nil {
		return hfPage{}, err
	}
	return parseHFPage(data)
}
func (h *huggingFace) Fetch(ctx context.Context) (Result, error) {
	result := Result{Metadata: map[string]any{"datasets": h.cfg.Datasets, "transport": "Hugging Face dataset-viewer rows API"}}
	for _, dataset := range h.cfg.Datasets {
		first, err := h.page(ctx, dataset, 0)
		if err != nil {
			return Result{}, err
		}
		if first.total > h.cfg.MaxRows || first.total == 0 {
			return Result{}, &SourceError{Stage: "dataset row limit or empty dataset"}
		}
		count := (first.total + 99) / 100
		pages := make([]hfPage, count)
		pages[0] = first
		// Bounded parallel pages make the published LiveBench judgments usable
		// without requiring a parquet library, Python service, or hundreds of
		// sequential round trips. Any missing page rejects the entire update.
		jobs := make(chan int, count)
		for i := 1; i < count; i++ {
			jobs <- i
		}
		close(jobs)
		ctx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		var firstErr error
		var mu sync.Mutex
		for range min(8, count) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for index := range jobs {
					if ctx.Err() != nil {
						return
					}
					page, err := h.page(ctx, dataset, index*100)
					if err == nil && page.total != first.total {
						err = &SourceError{Stage: "dataset changed during pagination"}
					}
					if err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						mu.Unlock()
						cancel()
						return
					}
					pages[index] = page
				}
			}()
		}
		wg.Wait()
		cancel()
		if firstErr != nil {
			return Result{}, firstErr
		}
		seen := 0
		for _, page := range pages {
			seen += len(page.rows)
			for _, row := range page.rows {
				observations, err := h.parseRow(dataset, row)
				if err != nil {
					return Result{}, err
				}
				result.Observations = append(result.Observations, observations...)
			}
		}
		if seen != first.total {
			return Result{}, &SourceError{Stage: "incomplete dataset pagination"}
		}
	}
	return result, nil
}

func (h *huggingFace) parseRow(dataset Dataset, row map[string]any) ([]Observation, error) {
	field := dataset.ModelField
	if field == "" {
		field = "model"
	}
	model := text(pathValue(row, field))
	if model == "" {
		return nil, &SourceError{Stage: "dataset model identity parsing"}
	}
	date := parseDate(pathValue(row, dataset.DateField))
	metadata := map[string]any{"dataset": dataset.ID, "config": dataset.Config, "split": dataset.Split, "row": row}
	if question, ok := row["question_id"].(string); ok {
		metadata["question_id"] = question
	}
	if h.mode == "lmarena" {
		rating, ok := number(row["rating"])
		if !ok {
			rating, ok = number(row["score"])
		}
		if !ok {
			return nil, &SourceError{Stage: "Arena rating parsing"}
		}
		// Arena's relative ratings are preference, even for its coding/vision
		// subsets. Preserve the original scale and confidence intervals.
		o := Observation{ExternalModelID: model, Benchmark: "arena." + dataset.Config + "." + text(row["category"]), Category: Preference, Score: rating, Unit: "arena_rating", EvaluatedAt: date, Version: dataset.Version, Metadata: metadata}
		if _, ok := row["rating"]; ok {
			n := 1 / (1 + math.Pow(10, (1000-rating)/400))
			o.Normalized = &n
			o.Metadata["normalization"] = "logistic preference reference: 1000, scale: 400"
		}
		// IPS Agent Arena scores have no equivalent Elo scale; only an
		// explicitly declared min/max may normalize those values.
		if mapping, ok := dataset.Metrics["score"]; ok {
			n, err := Normalize(rating, mapping.Min, mapping.Max, mapping.LowerIsBetter)
			if err != nil {
				return nil, &SourceError{Stage: "Arena normalization"}
			}
			o.Normalized = n
		}
		return []Observation{o}, nil
	}
	if h.mode == "livebench" {
		score, ok := number(row["score"])
		if !ok {
			return nil, &SourceError{Stage: "LiveBench score parsing"}
		}
		category := map[string]string{"coding": Coding, "agentic_coding": Agentic, "reasoning": Reasoning, "math": Math, "instruction_following": Instruction, "data_analysis": Reasoning}[text(row["category"])]
		if category == "" {
			category = "unknown"
		}
		o, err := measurement(model, "livebench."+text(row["category"])+"."+text(row["task"]), category, score, &Mapping{Category: category, Max: 1}, date, dataset.Version, metadata)
		return []Observation{o}, err
	}
	var observations []Observation
	for _, metric := range sortedMappingKeys(dataset.Metrics) {
		mapping := dataset.Metrics[metric]
		value := pathValue(row, metric)
		if value == nil {
			continue
		}
		score, ok := number(value)
		if !ok {
			return nil, &SourceError{Stage: "dataset metric parsing"}
		}
		o, err := measurement(model, dataset.ID+"/"+metric, mapping.Category, score, &mapping, date, dataset.Version, metadata)
		if err != nil {
			return nil, err
		}
		observations = append(observations, o)
	}
	return observations, nil
}
func sortedMappingKeys(m map[string]Mapping) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}
