package intelligence

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const CustomSource = "custom"
const MaxCustomRows = 10000
const CSVTemplate = "external_model_id,benchmark,category,original_score,normalized_score,unit,version,evaluated_at\nprovider/model,my-coding-eval,coding,85,0.85,percent,v1,2026-10-01T00:00:00Z\n"

var csvFields = []string{"external_model_id", "benchmark", "category", "original_score", "normalized_score", "unit", "version", "evaluated_at"}

// ParseCSV validates the entire upload before any durable mutation.
func ParseCSV(reader io.Reader, now time.Time) ([]Observation, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, errors.New("CSV header is required")
	}
	if len(header) != len(csvFields) {
		return nil, errors.New("CSV columns must match the template")
	}
	for i, field := range csvFields {
		if strings.TrimPrefix(header[i], "\ufeff") != field {
			return nil, errors.New("CSV columns must match the template in order")
		}
	}
	observations := []Observation{}
	seen := map[string]bool{}
	for row := 2; ; row++ {
		values, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d is malformed", row)
		}
		if len(observations) >= MaxCustomRows {
			return nil, errors.New("CSV supports at most 10000 rows")
		}
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
			if len(values[i]) > 512 {
				return nil, fmt.Errorf("CSV row %d field is too long", row)
			}
		}
		o := Observation{Source: CustomSource, ExternalModelID: values[0], Benchmark: values[1], Category: values[2], Unit: values[5], Version: values[6], RetrievedAt: now.UTC(), DateBasis: "retrieved"}
		o.Score, err = strconv.ParseFloat(values[3], 64)
		if err != nil {
			return nil, fmt.Errorf("CSV row %d has an invalid original_score", row)
		}
		if values[4] != "" {
			n, e := strconv.ParseFloat(values[4], 64)
			if e != nil {
				return nil, fmt.Errorf("CSV row %d has an invalid normalized_score", row)
			}
			o.Normalized = &n
		}
		if values[7] != "" {
			o.EvaluatedAt, err = time.Parse(time.RFC3339, values[7])
			if err != nil {
				return nil, fmt.Errorf("CSV row %d evaluated_at must be RFC3339", row)
			}
			o.DateBasis = "evaluated"
		}
		if err := ValidateObservations([]Observation{o}); err != nil {
			return nil, fmt.Errorf("CSV row %d: %w", row, err)
		}
		key := observationKey(o)
		if seen[key] {
			return nil, fmt.Errorf("CSV row %d duplicates a model/benchmark/category/version", row)
		}
		seen[key] = true
		observations = append(observations, o)
	}
	if len(observations) == 0 {
		return nil, errors.New("CSV contains no benchmark rows")
	}
	return observations, nil
}
func observationKey(o Observation) string {
	return o.ExternalModelID + "\x00" + o.Benchmark + "\x00" + o.Category + "\x00" + o.Version
}

// mutate serializes imports/edits with refreshes and publishes only after Save.
func (m *Manager) mutate(ctx context.Context, change func(*Snapshot) error) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if m.lifecycle.Err() != nil {
		return context.Canceled
	}
	m.mu.RLock()
	next := cloneJSON(m.snapshot)
	m.mu.RUnlock()
	if err := change(&next); err != nil {
		return err
	}
	next.UpdatedAt = m.now().UTC()
	next.Models = nil
	next.Unmatched = nil
	if err := m.store.Save(ctx, next); err != nil {
		return errors.New("could not save benchmarks; previous data retained")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot = next
	m.rebuildLocked()
	return nil
}
func (m *Manager) Import(ctx context.Context, observations []Observation) error {
	if len(observations) > MaxCustomRows {
		return errors.New("at most 10000 rows may be imported")
	}
	if err := ValidateObservations(observations); err != nil {
		return err
	}
	return m.mutate(ctx, func(next *Snapshot) error {
		state := next.Sources[CustomSource]
		if state.Type != "" && state.Type != "custom" {
			return errors.New("source ID custom is reserved for uploaded data")
		}
		positions := map[string]int{}
		for i, o := range state.Observations {
			positions[observationKey(o)] = i
		}
		for _, o := range observations {
			o.Source = CustomSource
			o.ModelID = ""
			o.RetrievedAt = m.now().UTC()
			if i, ok := positions[observationKey(o)]; ok {
				state.Observations[i] = o
			} else {
				positions[observationKey(o)] = len(state.Observations)
				state.Observations = append(state.Observations, o)
			}
		}
		if len(state.Observations) > 100000 {
			return errors.New("custom benchmark data supports at most 100000 stored rows")
		}
		state.ID = CustomSource
		state.Type = "custom"
		state.Enabled = true
		state.LastSuccess = m.now().UTC()
		state.LastAttempt = state.LastSuccess
		state.Error = ""
		next.Sources[CustomSource] = state
		return nil
	})
}

var ErrEditConflict = errors.New("benchmark data changed; reload before editing")

func DatasetRevision(observations []Observation) string { return observationHash(observations) }
func (m *Manager) Edit(ctx context.Context, source string, index int, revision string, observation Observation) error {
	if err := ValidateObservations([]Observation{observation}); err != nil {
		return err
	}
	for _, value := range []string{observation.ExternalModelID, observation.Benchmark, observation.Category, observation.Unit, observation.Version} {
		if len(value) > 512 {
			return errors.New("benchmark fields must be at most 512 bytes")
		}
	}
	return m.mutate(ctx, func(next *Snapshot) error {
		state, ok := next.Sources[source]
		if !ok || index < 0 || index >= len(state.Observations) || revision != DatasetRevision(state.Observations) {
			return ErrEditConflict
		}
		old := state.Observations[index]
		observation.Source = source
		observation.ModelID = ""
		observation.RetrievedAt = m.now().UTC()
		observation.Metadata = old.Metadata
		if observation.Metadata == nil {
			observation.Metadata = map[string]any{}
		}
		observation.Metadata["manually_edited"] = true
		if observation.EvaluatedAt.IsZero() {
			observation.DateBasis = "retrieved"
		} else {
			observation.DateBasis = "evaluated"
		}
		state.Observations[index] = observation
		next.Sources[source] = state
		return nil
	})
}
