// Package usage stores only aggregate counters needed by routing decisions.
package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const version = 1

type Aggregate struct {
	Requests      uint64    `json:"requests"`
	InputTokens   uint64    `json:"input_tokens"`
	OutputTokens  uint64    `json:"output_tokens"`
	EstimatedCost float64   `json:"estimated_cost"`
	LastRequestAt time.Time `json:"last_request_at,omitempty"`
}

type UsageDelta struct {
	Model                     string
	InputTokens, OutputTokens uint64
	EstimatedCost             float64
}
type UsageSummary = Aggregate

type file struct {
	Version int                  `json:"version"`
	Models  map[string]Aggregate `json:"models"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	models map[string]Aggregate
}

func ExtractResponseUsage(data []byte) (input, output uint64, ok bool) {
	var envelope struct {
		Usage struct {
			PromptTokens     uint64 `json:"prompt_tokens"`
			CompletionTokens uint64 `json:"completion_tokens"`
			InputTokens      uint64 `json:"input_tokens"`
			OutputTokens     uint64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return 0, 0, false
	}
	input, output = envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens
	if input == 0 {
		input = envelope.Usage.InputTokens
	}
	if output == 0 {
		output = envelope.Usage.OutputTokens
	}
	return input, output, input != 0 || output != 0
}

func Open(directory string) (*Store, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("usage directory is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create usage directory: %w", err)
	}
	path := filepath.Join(directory, "usage.json")
	s := &Store{path: path, models: map[string]Aggregate{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read usage: %w", err)
	}
	var f file
	if json.Unmarshal(data, &f) != nil || f.Version != version || f.Models == nil {
		return nil, errors.New("usage state is corrupt")
	}
	for model, aggregate := range f.Models {
		if strings.TrimSpace(model) == "" || aggregate.EstimatedCost < 0 {
			return nil, errors.New("usage state is corrupt")
		}
	}
	s.models = f.Models
	return s, nil
}

func (s *Store) Snapshot() map[string]Aggregate {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]Aggregate, len(s.models))
	for key, value := range s.models {
		result[key] = value
	}
	return result
}

func (s *Store) Get(model string) (Aggregate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.models[model]
	return value, ok
}

func (s *Store) AddDelta(delta UsageDelta) error {
	return s.Add(delta.Model, delta.InputTokens, delta.OutputTokens, delta.EstimatedCost)
}

func (s *Store) Add(model string, input, output uint64, cost float64) error {
	if s == nil || strings.TrimSpace(model) == "" {
		return errors.New("model is required")
	}
	if cost < 0 {
		return errors.New("cost must be non-negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.models[model]
	value.Requests++
	value.InputTokens += input
	value.OutputTokens += output
	value.EstimatedCost += cost
	value.LastRequestAt = time.Now().UTC()
	s.models[model] = value
	return s.persistLocked()
}

func (s *Store) persistLocked() error {
	data, err := json.Marshal(file{Version: version, Models: s.models})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".usage-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(append(data, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if err == nil {
		err = tmp.Close()
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpPath, s.path); err != nil {
		return err
	}
	keep = true
	return nil
}
