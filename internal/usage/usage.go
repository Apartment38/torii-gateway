// Package usage maintains memory-only routing counters with batched persistence.
package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/state"
)

const flushInterval = time.Second
const batchSize = 250

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

type Store struct {
	mu       sync.Mutex
	flushMu  sync.Mutex
	closeMu  sync.Mutex
	db       *sql.DB
	models   map[string]Aggregate
	dirty    map[string]bool
	closed   bool
	cancel   context.CancelFunc
	done     chan struct{}
	closeErr error
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

// Open owns an independent pool and one worker. The gateway shares this store
// across handler reloads. Add accepts a memory update; Flush/Close ensure durability.
func Open(database *state.Database) (*Store, error) {
	if database == nil {
		return nil, errors.New("usage storage requires the application database")
	}
	db, err := database.StateConnection()
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, models: map[string]Aggregate{}, dirty: map[string]bool{}}
	s.models, err = load(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.run(ctx)
	return s, nil
}
func validAggregate(model string, a Aggregate) bool {
	return strings.TrimSpace(model) != "" && a.EstimatedCost >= 0 && !math.IsNaN(a.EstimatedCost) && !math.IsInf(a.EstimatedCost, 0)
}
func load(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) (map[string]Aggregate, error) {
	rows, err := q.Query("SELECT model_id,requests,input_tokens,output_tokens,estimated_cost,last_request_at FROM usage_aggregates ORDER BY model_id")
	if err != nil {
		return nil, errors.New("cannot read usage aggregates")
	}
	defer rows.Close()
	models := map[string]Aggregate{}
	for rows.Next() {
		var model, requests, input, output, at string
		var a Aggregate
		if err := rows.Scan(&model, &requests, &input, &output, &a.EstimatedCost, &at); err != nil {
			return nil, errors.New("invalid stored usage aggregate")
		}
		a.Requests, err = strconv.ParseUint(requests, 10, 64)
		if err == nil {
			a.InputTokens, err = strconv.ParseUint(input, 10, 64)
		}
		if err == nil {
			a.OutputTokens, err = strconv.ParseUint(output, 10, 64)
		}
		if err == nil {
			a.LastRequestAt, err = time.Parse(time.RFC3339Nano, at)
		}
		if err != nil || !validAggregate(model, a) {
			return nil, errors.New("stored usage is corrupt; restore a database backup")
		}
		models[model] = a
	}
	if rows.Err() != nil {
		return nil, errors.New("cannot read usage aggregates")
	}
	return models, nil
}

const upsert = `INSERT INTO usage_aggregates(model_id,requests,input_tokens,output_tokens,estimated_cost,last_request_at) VALUES(?,?,?,?,?,?) ON CONFLICT(model_id) DO UPDATE SET requests=excluded.requests,input_tokens=excluded.input_tokens,output_tokens=excluded.output_tokens,estimated_cost=excluded.estimated_cost,last_request_at=excluded.last_request_at`

func write(ctx context.Context, tx *sql.Tx, model string, a Aggregate) error {
	_, err := tx.ExecContext(ctx, upsert, model, strconv.FormatUint(a.Requests, 10), strconv.FormatUint(a.InputTokens, 10), strconv.FormatUint(a.OutputTokens, 10), a.EstimatedCost, a.LastRequestAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errors.New("cannot persist usage aggregate")
	}
	return nil
}

func (s *Store) Snapshot() map[string]Aggregate {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]Aggregate, len(s.models))
	for model, a := range s.models {
		result[model] = a
	}
	return result
}
func (s *Store) Get(model string) (Aggregate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.models[model]
	return a, ok
}
func (s *Store) AddDelta(d UsageDelta) error {
	return s.Add(d.Model, d.InputTokens, d.OutputTokens, d.EstimatedCost)
}
func (s *Store) Add(model string, input, output uint64, cost float64) error {
	if s == nil || strings.TrimSpace(model) == "" {
		return errors.New("model is required")
	}
	if cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return errors.New("cost must be finite and non-negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("usage store is closed")
	}
	a := s.models[model]
	if a.Requests == math.MaxUint64 || input > math.MaxUint64-a.InputTokens || output > math.MaxUint64-a.OutputTokens || math.IsInf(a.EstimatedCost+cost, 0) {
		return errors.New("usage aggregate overflow")
	}
	a.Requests++
	a.InputTokens += input
	a.OutputTokens += output
	a.EstimatedCost += cost
	a.LastRequestAt = time.Now().UTC()
	s.models[model] = a
	s.dirty[model] = true
	return nil
}

// Flush uses absolute totals so retries after uncertain commits never double count.
func (s *Store) Flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	snapshot := map[string]Aggregate{}
	ids := make([]string, 0, len(s.dirty))
	for model := range s.dirty {
		snapshot[model] = s.models[model]
		ids = append(ids, model)
	}
	s.mu.Unlock()
	sort.Strings(ids)
	for start := 0; start < len(ids); start += batchSize {
		end := min(start+batchSize, len(ids))
		if err := s.flushBatch(ctx, ids[start:end], snapshot); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		s.mu.Lock()
		for _, model := range ids[start:end] {
			if s.models[model] == snapshot[model] {
				delete(s.dirty, model)
			}
		}
		s.mu.Unlock()
	}
	return nil
}
func (s *Store) flushBatch(ctx context.Context, ids []string, values map[string]Aggregate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cannot begin usage batch")
	}
	defer tx.Rollback()
	for _, model := range ids {
		if err := write(ctx, tx, model, values[model]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.New("cannot commit usage batch")
	}
	return nil
}
func (s *Store) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	var lastWarning time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Flush(ctx); err != nil && ctx.Err() == nil && (lastWarning.IsZero() || time.Since(lastWarning) >= 30*time.Second) {
				slog.Warn("usage_flush_failed", "error", err, "message", "Pending usage retained in memory; retrying every second; crash loss window is growing")
				lastWarning = time.Now()
			}
		}
	}
}

// Close must follow HTTP draining. It stops the worker and flushes with the
// caller's independent shutdown deadline before releasing the connection pool.
func (s *Store) Close(ctx context.Context) error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.closeErr
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	<-s.done
	s.closeErr = s.Flush(ctx)
	if s.closeErr != nil {
		slog.Error("usage_shutdown_flush_failed", "error", s.closeErr, "message", "Unflushed usage may be lost; database totals preserved")
	}
	s.closeErr = errors.Join(s.closeErr, s.db.Close())
	return s.closeErr
}
