package intelligence

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
)

type Options struct {
	RefreshInterval time.Duration
	SourceTimeout   time.Duration
	StaleAfter      time.Duration
	MaxAge          time.Duration
	HalfLife        time.Duration
	Aggregation     Aggregation
	Identities      []Identity
	Sources         []SourceSettings
	Logger          *slog.Logger
}

func (o Options) Defaults() Options {
	if o.RefreshInterval <= 0 {
		o.RefreshInterval = 24 * time.Hour
	}
	if o.SourceTimeout <= 0 {
		o.SourceTimeout = 10 * time.Minute
	}
	if o.StaleAfter <= 0 {
		o.StaleAfter = 7 * 24 * time.Hour
	}
	if o.MaxAge <= 0 {
		o.MaxAge = 180 * 24 * time.Hour
	}
	if o.HalfLife <= 0 {
		o.HalfLife = 90 * 24 * time.Hour
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

type Manager struct {
	mu             sync.RWMutex
	refreshMu      sync.Mutex
	store          SnapshotStore
	sources        []Source
	options        Options
	models         []catalog.Descriptor
	snapshot       Snapshot
	physical       map[string]string
	now            func() time.Time
	lifecycle      context.Context
	closeLifecycle context.CancelFunc
}

func NewManager(store SnapshotStore, sources []Source, models []catalog.Descriptor, options Options) (*Manager, error) {
	options = options.Defaults()
	seen := map[string]bool{}
	for _, source := range sources {
		if source == nil || source.ID() == "" || seen[source.ID()] {
			return nil, errors.New("benchmark source IDs must be unique and nonempty")
		}
		seen[source.ID()] = true
	}
	snapshot, err := store.Load(context.Background())
	if err != nil {
		options.Logger.Warn("benchmark_cache_load_failed", "error", err)
		// A corrupt cache never prevents inference startup. It is not overwritten
		// until a refresh has produced a valid replacement.
		snapshot = Snapshot{Version: cacheVersion, Sources: map[string]SourceState{}}
	}
	m := &Manager{store: store, sources: sources, options: options, models: models, snapshot: snapshot, now: time.Now}
	m.lifecycle, m.closeLifecycle = context.WithCancel(context.Background())
	configured := map[string]SourceSettings{}
	for _, cfg := range options.Sources {
		configured[cfg.ID] = cfg
	}
	// Disabled/removed sources remain inspectable but cannot influence routing.
	for id, state := range m.snapshot.Sources {
		state.Enabled = false
		m.snapshot.Sources[id] = state
	}
	for id, cfg := range configured {
		state := m.snapshot.Sources[id]
		if state.Type != "" && cfg.Type != "" && state.Type != cfg.Type {
			state = SourceState{}
		}
		state.ID, state.Type, state.Enabled = id, cfg.Type, cfg.Enabled
		m.snapshot.Sources[id] = state
	}
	for _, source := range sources {
		if _, ok := configured[source.ID()]; !ok {
			state := m.snapshot.Sources[source.ID()]
			state.ID, state.Enabled = source.ID(), true
			m.snapshot.Sources[source.ID()] = state
		}
	}
	m.rebuildLocked()
	return m, nil
}

func ValidateObservations(observations []Observation) error {
	if len(observations) == 0 {
		return errors.New("source returned no observations")
	}
	for _, o := range observations {
		if o.ExternalModelID == "" || o.Benchmark == "" || !validCategory(o.Category) || math.IsNaN(o.Score) || math.IsInf(o.Score, 0) {
			return errors.New("source returned an invalid observation")
		}
		if o.Normalized != nil && (math.IsNaN(*o.Normalized) || math.IsInf(*o.Normalized, 0) || *o.Normalized < 0 || *o.Normalized > 1) {
			return errors.New("source returned an invalid normalized score")
		}
		if o.Category == Operational && o.Score < 0 {
			return errors.New("source returned a negative operational measurement")
		}
	}
	return nil
}

func (m *Manager) UpdateModels(models []catalog.Descriptor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models = models
	m.rebuildLocked()
}

// Refresh updates sources concurrently; network work never holds the read lock.
// Each failure retains that source's last successful dataset.
func (m *Manager) Refresh(ctx context.Context) error {
	return m.RefreshWithProgress(ctx, nil)
}

// RefreshWithProgress publishes each completed source independently, after
// durable storage succeeds. A slow or failed source cannot hide good updates.
// Progress callbacks run sequentially, outside the snapshot/routing read lock.
func (m *Manager) RefreshWithProgress(ctx context.Context, progress func(RefreshEvent)) error {
	emit := func(event RefreshEvent) {
		if progress != nil {
			progress(event)
		}
	}
	emit(RefreshEvent{Phase: "queued", Total: len(m.sources), Message: "Waiting for the benchmark refresh worker"})
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	ctx, cancelRefresh := context.WithCancel(ctx)
	defer cancelRefresh()
	stopCancel := context.AfterFunc(m.lifecycle, cancelRefresh)
	defer stopCancel()
	if m.lifecycle.Err() != nil || ctx.Err() != nil {
		return context.Canceled
	}
	type update struct {
		id      string
		result  Result
		err     error
		started bool
	}
	completed, failed := 0, 0
	notify := func(phase, id, message string, observations int) {
		emit(RefreshEvent{Phase: phase, Source: id, Completed: completed, Total: len(m.sources), Failed: failed, Observations: observations, Message: message})
	}
	notify("started", "", "Starting enabled benchmark sources", 0)
	updates := make(chan update, 2*len(m.sources))
	for _, source := range m.sources {
		go func(source Source) {
			updates <- update{id: source.ID(), started: true}
			m.options.Logger.Info("benchmark_refresh_started", "source", source.ID())
			ctx, cancel := context.WithTimeout(ctx, m.options.SourceTimeout)
			defer cancel()
			result, err := source.Fetch(ctx)
			if err == nil {
				err = ValidateObservations(result.Observations)
			}
			updates <- update{id: source.ID(), result: result, err: err}
		}(source)
	}
	var failures []error
	for completed < len(m.sources) {
		var update update
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update = <-updates:
		}
		if update.started {
			notify("fetching", update.id, "Fetching and parsing published benchmark data", 0)
			continue
		}
		m.mu.RLock()
		next := cloneJSON(m.snapshot)
		m.mu.RUnlock()
		now := m.now().UTC()
		state := next.Sources[update.id]
		state.LastAttempt = now
		if update.err != nil {
			// Do not expose arbitrary adapter errors (URLs can contain secrets).
			state.Error = "refresh failed; previous data retained"
			var sourceError *SourceError
			if errors.As(update.err, &sourceError) {
				state.Error = sourceError.Error() + "; previous data retained"
			}
			m.options.Logger.Warn("benchmark_refresh_failed", "source", update.id, "error", state.Error)
		} else {
			for i := range update.result.Observations {
				update.result.Observations[i].Source = update.id
				update.result.Observations[i].ModelID = "" // shared matching is authoritative
				update.result.Observations[i].RetrievedAt = now
				update.result.Observations[i].DateBasis = "evaluation"
				if update.result.Observations[i].EvaluatedAt.IsZero() {
					update.result.Observations[i].DateBasis = "retrieval_only"
				}
			}
			state.Observations, state.Metadata = update.result.Observations, update.result.Metadata
			state.LastSuccess, state.Error = now, ""
		}
		next.Sources[update.id] = state
		next.UpdatedAt = now
		next.Models, next.Unmatched = nil, nil
		notify("persisting", update.id, "Validating and atomically saving source results", len(state.Observations))
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := m.store.Save(ctx, next); err != nil {
			m.options.Logger.Error("benchmark_cache_save_failed", "message", "Could not publish benchmark source", "source", update.id)
			update.err = errors.New("could not save source; previous cached data retained")
		} else {
			m.mu.Lock()
			m.snapshot = next
			m.rebuildLocked()
			m.mu.Unlock()
		}
		completed++
		if update.err != nil {
			failed++
			failures = append(failures, errors.New(update.id+": refresh failed"))
			notify("failed", update.id, "Source update failed; previous cached data retained", len(state.Observations))
		} else {
			m.options.Logger.Info("benchmark_refresh_succeeded", "source", update.id, "observations", len(state.Observations))
			notify("published", update.id, "Saved source data, matched models and updated capability scores", len(state.Observations))
		}
	}
	if len(m.sources) == 0 {
		// Persist configured disabled sources as well, without fetching upstream.
		m.mu.RLock()
		next := cloneJSON(m.snapshot)
		m.mu.RUnlock()
		next.UpdatedAt = m.now().UTC()
		next.Models, next.Unmatched = nil, nil
		if err := m.store.Save(ctx, next); err != nil {
			return errors.New("cannot save benchmark status")
		}
		m.mu.Lock()
		m.snapshot = next
		m.rebuildLocked()
		m.mu.Unlock()
	}
	notify("complete", "", "Benchmark refresh finished", 0)
	return errors.Join(failures...)
}

// Close cancels all in-flight refreshes (including manual API refreshes) before
// another handler becomes the writer, then releases its benchmark connection.
func (m *Manager) Close() {
	m.closeLifecycle()
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if err := m.store.Close(); err != nil {
		m.options.Logger.Warn("benchmark_storage_close_failed", "message", "Could not close benchmark connection")
	}
}

// ReloadCache picks up the prior active handler's final successful update when
// this manager was built speculatively during a configuration reload.
func (m *Manager) ReloadCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	loaded, err := m.store.Load(context.Background())
	if err != nil {
		return
	}
	for id, state := range loaded.Sources {
		state.Enabled = false
		loaded.Sources[id] = state
	}
	for id, configured := range m.snapshot.Sources {
		state := loaded.Sources[id]
		if state.Type != "" && configured.Type != "" && state.Type != configured.Type {
			state = SourceState{}
		}
		state.ID, state.Type, state.Enabled = id, configured.Type, configured.Enabled
		loaded.Sources[id] = state
	}
	m.snapshot = loaded
	m.rebuildLocked()
}

func (m *Manager) rebuildLocked() {
	previousUnmatched := map[string]bool{}
	for _, previous := range m.snapshot.Unmatched {
		previousUnmatched[previous.Source+"\x00"+previous.ExternalID+"\x00"+previous.Reason] = true
	}
	matcher := NewMatcher(m.models, m.options.Identities)
	m.physical = matcher.physical
	models := map[string][]Observation{}
	unmatched := map[string]Unmatched{}
	now := m.now().UTC()
	ids := make([]string, 0, len(m.snapshot.Sources))
	for id := range m.snapshot.Sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state := m.snapshot.Sources[id]
		state.ObservationCount = len(state.Observations)
		state.AgeSeconds = 0
		if !state.LastSuccess.IsZero() {
			state.AgeSeconds = math.Max(0, now.Sub(state.LastSuccess).Seconds())
		}
		state.Stale = state.LastSuccess.IsZero() || now.Sub(state.LastSuccess) > m.options.StaleAfter
		for i := range state.Observations {
			o := state.Observations[i]
			canonical, candidates := matcher.Match(id, o.ExternalModelID)
			o.ModelID = canonical
			state.Observations[i].ModelID = canonical
			if !state.Enabled {
				continue
			}
			if canonical == "" {
				reason := "unmatched"
				if len(candidates) > 1 {
					reason = "ambiguous"
				}
				unmatched[id+"\x00"+o.ExternalModelID] = Unmatched{Source: id, ExternalID: o.ExternalModelID, Reason: reason, Candidates: candidates}
			} else {
				models[canonical] = append(models[canonical], o)
			}
		}
		m.snapshot.Sources[id] = state
	}
	m.snapshot.Models = map[string]Model{}
	for id, observations := range models {
		model := Aggregate(observations, m.options.Aggregation, now, m.options.HalfLife, m.options.MaxAge, m.options.StaleAfter)
		model.ID, model.ProviderIDs = id, matcher.providers[id]
		model.ObservationCount = len(observations)
		seen := map[string]bool{}
		for _, observation := range observations {
			seen[observation.Source] = true
		}
		for source := range seen {
			model.Sources = append(model.Sources, source)
		}
		sort.Strings(model.Sources)
		m.snapshot.Models[id] = model
	}
	m.snapshot.Unmatched = nil
	keys := make([]string, 0, len(unmatched))
	for key := range unmatched {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		m.snapshot.Unmatched = append(m.snapshot.Unmatched, unmatched[key])
		entry := unmatched[key]
		if !previousUnmatched[entry.Source+"\x00"+entry.ExternalID+"\x00"+entry.Reason] {
			m.options.Logger.Warn("benchmark_model_match_failed", "source", entry.Source, "external_id", entry.ExternalID, "reason", entry.Reason, "candidates", entry.Candidates)
		}
	}
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebuildLocked()
	return cloneJSON(m.snapshot)
}

// Lookup is cache-only and bounded: no HTTP, disk I/O or aggregation on inference.
func (m *Manager) Lookup(publicID string) (Model, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	canonical := m.physical[publicID]
	model, ok := m.snapshot.Models[canonical]
	if !ok || (!model.ValidUntil.IsZero() && m.now().After(model.ValidUntil)) || m.now().Sub(model.LatestRetrieval) > m.options.StaleAfter || m.now().Sub(model.LatestEvaluation) > m.options.MaxAge {
		return Model{}, false
	}
	// Routing needs only the small derived view, not the raw dataset.
	model.Observations = nil
	return cloneJSON(model), !model.Stale
}

// Run owns one background worker and stops when its lifecycle context ends.
// Reaggregation ages cached evidence even between scheduled network refreshes.
func (m *Manager) Run(ctx context.Context) {
	_ = m.Refresh(ctx)
	interval := min(time.Minute, m.options.RefreshInterval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastRefresh := m.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.now().Sub(lastRefresh) >= m.options.RefreshInterval {
				_ = m.Refresh(ctx)
				lastRefresh = m.now()
			} else {
				m.mu.Lock()
				m.rebuildLocked()
				m.mu.Unlock()
			}
		}
	}
}
