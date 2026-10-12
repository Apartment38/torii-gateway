// Package activity publishes bounded, metadata-only request history.
package activity

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Apartment38/torii-gateway/internal/state"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	RetentionDays      int    `yaml:"retention_days,omitempty"`
	PromptLogging      bool   `yaml:"prompt_logging,omitempty"`
	JevDecisionLogging bool   `yaml:"jev_decision_logging,omitempty"`
	MaxBytes           int64  `yaml:"max_bytes,omitempty"`
	LogFile            string `yaml:"log_file,omitempty"`
	LogMaxBytes        int64  `yaml:"log_max_bytes,omitempty"`
	LogBackups         int    `yaml:"log_backups,omitempty"`
}

func (o Options) Defaults() Options {
	if o.RetentionDays == 0 {
		o.RetentionDays = 7
	}
	if o.MaxBytes == 0 {
		o.MaxBytes = 100 << 20
	}
	if o.LogMaxBytes == 0 {
		o.LogMaxBytes = 10 << 20
	}
	if o.LogBackups == 0 {
		o.LogBackups = 3
	}
	return o
}
func (o Options) Validate() error {
	if o.MaxBytes > 0 && o.MaxBytes < 8192 || o.LogMaxBytes > 0 && o.LogMaxBytes < 4096 {
		return errors.New("activity cap must be at least 8192 bytes; file log cap at least 4096 bytes")
	}
	if o.RetentionDays < 0 || o.RetentionDays > 3650 || o.MaxBytes < 0 || o.LogMaxBytes < 0 || o.LogBackups < 0 || o.LogBackups > 20 {
		return errors.New("invalid activity retention or log limits")
	}
	return nil
}

type Event struct {
	ID                int64     `json:"id"`
	At                time.Time `json:"at"`
	Kind              string    `json:"kind"`
	RequestID         string    `json:"request_id,omitempty"`
	KeyID             string    `json:"key_id,omitempty"`
	KeyName           string    `json:"key_name,omitempty"`
	RequestedModel    string    `json:"requested_model,omitempty"`
	SelectedModel     string    `json:"selected_model,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	Outcome           string    `json:"outcome,omitempty"`
	Status            int       `json:"status,omitempty"`
	DurationMS        int64     `json:"duration_ms,omitempty"`
	InputTokens       int64     `json:"input_tokens,omitempty"`
	OutputTokens      int64     `json:"output_tokens,omitempty"`
	EstimatedTokens   int64     `json:"estimated_tokens,omitempty"`
	EstimatedCost     *float64  `json:"estimated_cost,omitempty"`
	UnknownCost       bool      `json:"unknown_cost"`
	AccountingFailure bool      `json:"accounting_failure,omitempty"`
	GapCount          uint64    `json:"gap_count,omitempty"`
	Prompt            string    `json:"prompt,omitempty"`
	JevRequest        string    `json:"jev_request,omitempty"`
	JevResponse       string    `json:"jev_response,omitempty"`
	DebugTruncated    bool      `json:"debug_truncated,omitempty"`
}
type Store struct {
	db          *sql.DB
	queue       chan Event
	done        chan struct{}
	mu          sync.RWMutex
	options     Options
	subscribers map[chan Event]bool
	closed      bool
	dropped     atomic.Uint64
	closeOnce   sync.Once
}

func Open(database *state.Database, options Options) (*Store, error) {
	db, err := database.StateConnection()
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, options: options.Defaults(), queue: make(chan Event, 1024), done: make(chan struct{}), subscribers: map[chan Event]bool{}}
	if err = s.prune(); err != nil {
		db.Close()
		return nil, err
	}
	go s.run()
	return s, nil
}
func (s *Store) Configure(options Options) {
	s.mu.Lock()
	s.options = options.Defaults()
	s.mu.Unlock()
	if err := s.prune(); err != nil {
		slog.Warn("activity_retention_unavailable")
	}
}
func (s *Store) Options() Options {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.options
}

const MaxDebugBytes = 64 << 10

func DebugText(value string) (string, bool) {
	if len(value) <= MaxDebugBytes {
		return value, false
	}
	return strings.ToValidUTF8(value[:MaxDebugBytes], ""), true
}

func (s *Store) Emit(e Event) {
	// Only this explicit field inventory crosses the persistence/log boundary.
	e.ID = 0
	e.At = time.Now().UTC()
	e.RequestID = bounded(e.RequestID)
	e.KeyID = bounded(e.KeyID)
	e.KeyName = bounded(e.KeyName)
	e.RequestedModel = bounded(e.RequestedModel)
	e.SelectedModel = bounded(e.SelectedModel)
	e.Provider = bounded(e.Provider)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return
	}
	if !s.options.PromptLogging {
		e.Prompt = ""
	}
	if !s.options.JevDecisionLogging {
		e.JevRequest = ""
		e.JevResponse = ""
	}
	var truncated bool
	e.Prompt, truncated = DebugText(e.Prompt)
	e.DebugTruncated = e.DebugTruncated || truncated
	e.JevRequest, truncated = DebugText(e.JevRequest)
	e.DebugTruncated = e.DebugTruncated || truncated
	e.JevResponse, truncated = DebugText(e.JevResponse)
	e.DebugTruncated = e.DebugTruncated || truncated
	select {
	case s.queue <- e:
	default:
		s.dropped.Add(1)
	}
}
func bounded(value string) string {
	if len(value) > 512 {
		return strings.Clone(value[:512])
	}
	return value
}
func (s *Store) Subscribe() (chan Event, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan Event, 64)
	if s.closed || len(s.subscribers) >= 128 {
		close(ch)
		return ch, func() {}
	}
	s.subscribers[ch] = true
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.subscribers[ch] {
			delete(s.subscribers, ch)
			close(ch)
		}
	}
}
func (s *Store) publish(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- e:
		default:
			delete(s.subscribers, ch)
			close(ch)
		}
	}
}
func (s *Store) run() {
	defer close(s.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case e, ok := <-s.queue:
			if !ok {
				return
			}
			if count := s.dropped.Swap(0); count > 0 {
				s.persist(Event{Kind: "gap", At: time.Now().UTC(), GapCount: count})
			}
			s.persist(e)
		case <-ticker.C:
			if err := s.prune(); err != nil {
				slog.Warn("activity_retention_unavailable")
			}
		}
	}
}
func (s *Store) persist(e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	// Structured logs carry the same safe metadata as history, even if storage fails.
	defer func() {
		metadata := e
		metadata.Prompt = ""
		metadata.JevRequest = ""
		metadata.JevResponse = ""
		metadata.DebugTruncated = false
		slog.Info("gateway_activity", "event", metadata)
	}()
	tx, err := s.db.Begin()
	if err != nil {
		s.publish(Event{Kind: "gap", GapCount: 1})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec("INSERT INTO activity_events(at,document,size) VALUES(?,?,?)", e.At.Unix(), data, len(data)+256)
	if err != nil {
		s.publish(Event{Kind: "gap", GapCount: 1})
		return
	}
	e.ID, err = result.LastInsertId()
	if err != nil {
		return
	}
	removed, err := s.trim(tx)
	if err != nil {
		s.publish(Event{Kind: "gap", GapCount: 1})
		return
	}
	if err = tx.Commit(); err != nil {
		s.publish(Event{Kind: "gap", GapCount: 1})
		return
	}
	s.publish(e)
	if removed {
		s.publish(Event{Kind: "history_reset"})
	}
}
func (s *Store) trim(tx *sql.Tx) (bool, error) {
	s.mu.RLock()
	options := s.options
	s.mu.RUnlock()
	result, err := tx.Exec("DELETE FROM activity_events WHERE at<?", time.Now().UTC().AddDate(0, 0, -options.RetentionDays).Unix())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	removed := count > 0
	for {
		var size int64
		if err := tx.QueryRow("SELECT bytes FROM activity_storage WHERE id=1").Scan(&size); err != nil {
			return removed, err
		}
		if size <= options.MaxBytes {
			return removed, nil
		}
		if _, err := tx.Exec("DELETE FROM activity_events WHERE id IN (SELECT id FROM activity_events ORDER BY id LIMIT 128)"); err != nil {
			return removed, err
		}
		removed = true
	}
}
func (s *Store) prune() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	removed, err := s.trim(tx)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if removed {
		s.publish(Event{Kind: "history_reset"})
	}
	return nil
}
func (s *Store) History(after int64, limit int) ([]Event, bool, error) {
	if limit < 1 || limit > 1000 {
		return nil, false, errors.New("history limit must be 1-1000")
	}
	var minimum, maximum int64
	err := s.db.QueryRow("SELECT coalesce(min(id),0),coalesce(max(id),0) FROM activity_events").Scan(&minimum, &maximum)
	if err != nil {
		return nil, false, err
	}
	gap := after > maximum || after > 0 && after < minimum-1
	if after > maximum {
		after = 0
	}
	rows, err := s.db.Query("SELECT id,document FROM activity_events WHERE id>? ORDER BY id DESC LIMIT ?", after, limit)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var id int64
		var data []byte
		if err = rows.Scan(&id, &data); err != nil {
			return nil, false, err
		}
		var e Event
		if json.Unmarshal(data, &e) != nil {
			return nil, false, errors.New("invalid activity metadata")
		}
		e.ID = id
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
	if after > 0 && len(events) > 0 && events[0].ID > after+1 {
		gap = true
	}
	return events, gap, nil
}
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.queue)
		s.mu.Unlock()
		<-s.done
		s.mu.Lock()
		for ch := range s.subscribers {
			close(ch)
			delete(s.subscribers, ch)
		}
		s.mu.Unlock()
	})
	return s.db.Close()
}
