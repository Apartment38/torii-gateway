package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/state"
)

func ptr(value float64) *float64 { return &value }

func storageDatabase(t *testing.T) (*state.Database, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dir
}
func storageOpen(t *testing.T, db *state.Database, dir string) *Store {
	t.Helper()
	s, err := OpenStore(context.Background(), db, options().Logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func storageSnapshot(count int) Snapshot {
	at := time.Date(2026, 10, 2, 12, 3, 4, 123456789, time.UTC)
	observations := make([]Observation, count)
	for i := range observations {
		o := observation("a", "coding", Coding, "model-a", .8123456789012345, at)
		o.Version = "version-42"
		o.DateBasis = "published"
		o.Metadata = map[string]any{"row": i, "nested": map[string]any{"original": "upstream"}}
		observations[i] = o
	}
	return Snapshot{Version: cacheVersion, UpdatedAt: at, Sources: map[string]SourceState{"a": {ID: "a", Type: "benchlm", LastAttempt: at, LastSuccess: at, Metadata: map[string]any{"version": "42"}, Observations: observations}}}
}
func mustSave(t *testing.T, s *Store, snapshot Snapshot) {
	t.Helper()
	if err := s.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
}
func mustLoad(t *testing.T, s *Store) Snapshot {
	t.Helper()
	v, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func sourceDataset(t *testing.T, s *Store, id string) string {
	t.Helper()
	var dataset string
	if err := s.db.QueryRow("SELECT current_dataset FROM benchmark_sources WHERE source_id=?", id).Scan(&dataset); err != nil {
		t.Fatal(err)
	}
	return dataset
}
func sqlCount(t *testing.T, s *Store, table string) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
func sameSnapshot(t *testing.T, a, b Snapshot) {
	t.Helper()
	x, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := json.Marshal(b)
	if err != nil || !bytes.Equal(x, y) {
		t.Fatalf("snapshot changed: %v", err)
	}
}

func TestSQLiteSnapshotRoundTripLatestOnlyAndRestart(t *testing.T) {
	db, dir := storageDatabase(t)
	s := storageOpen(t, db, dir)
	v := storageSnapshot(600)
	a := v.Sources["a"]
	a.Observations[0].Normalized = nil
	a.Observations[1].Normalized = ptr(0.0)
	a.Observations[2].ExternalModelID = "unmatched-external"
	a.Observations[3] = a.Observations[2] // Duplicates and order are evidence, not storage keys.
	a.Observations[4].Category = Operational
	a.Observations[4].Benchmark = "throughput"
	a.Observations[4].Score = 150
	a.Observations[4].Normalized = nil
	a.Observations[4].Unit = "tokens/s"
	a.Observations[5].EvaluatedAt = time.Time{}
	v.Sources["a"] = a
	mustSave(t, s, v)
	first := mustLoad(t, s)
	if first.Sources["a"].Observations[0].Normalized != nil || first.Sources["a"].Observations[1].Normalized == nil || *first.Sources["a"].Observations[1].Normalized != 0 {
		t.Fatal("NULL and zero conflated")
	}
	want, err := prepare(v)
	if err != nil {
		t.Fatal(err)
	}
	canonical := emptySnapshot()
	canonical.UpdatedAt = v.UpdatedAt
	canonical.Sources["a"] = want["a"].status
	sameSnapshot(t, first, canonical)
	old := sourceDataset(t, s, "a")
	a.LastAttempt = a.LastAttempt.Add(time.Minute)
	a.Error = "upstream unavailable"
	v.Sources["a"] = a
	mustSave(t, s, v)
	if sourceDataset(t, s, "a") != old {
		t.Fatal("unchanged evidence rewritten")
	}
	a.Observations = append([]Observation(nil), a.Observations...)
	a.Observations[0].Score = .5
	v.Sources["a"] = a
	mustSave(t, s, v)
	if sourceDataset(t, s, "a") == old || sqlCount(t, s, "benchmark_datasets") != 1 || sqlCount(t, s, "benchmark_observations") != 600 {
		t.Fatal("old generations retained or new generation not published")
	}
	// Source type changes cannot reuse a dataset even with identical observations.
	old = sourceDataset(t, s, "a")
	a.Type = "livebench"
	v.Sources["a"] = a
	mustSave(t, s, v)
	if sourceDataset(t, s, "a") == old {
		t.Fatal("source type change reused dataset")
	}
	before := mustLoad(t, s)
	s.Close()
	db.Close()
	restarted, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	sameSnapshot(t, before, mustLoad(t, storageOpen(t, restarted, dir)))
}

func TestSQLiteStagingAndPublicationFailuresPreserveAllActiveSources(t *testing.T) {
	for _, trigger := range []string{
		`CREATE TRIGGER fail_staging BEFORE INSERT ON benchmark_observations WHEN NEW.ordinal=250 BEGIN SELECT RAISE(ABORT,'test'); END`,
		`CREATE TRIGGER fail_publication BEFORE UPDATE ON benchmark_storage BEGIN SELECT RAISE(ABORT,'test'); END`,
	} {
		t.Run(trigger, func(t *testing.T) {
			db, dir := storageDatabase(t)
			s := storageOpen(t, db, dir)
			v := storageSnapshot(2)
			b := v.Sources["a"]
			b.ID = "b"
			v.Sources["b"] = b
			mustSave(t, s, v)
			before := mustLoad(t, s)
			if _, err := s.db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			next := storageSnapshot(600)
			next.UpdatedAt = next.UpdatedAt.Add(time.Hour)
			next.Sources["b"] = b
			if err := s.Save(context.Background(), next); err == nil {
				t.Fatal("injected failure not returned")
			}
			sameSnapshot(t, before, mustLoad(t, s))
			if sqlCount(t, s, "benchmark_datasets") <= 2 {
				t.Fatal("no staged dataset to exercise recovery")
			}
			if _, err := s.db.Exec("DROP TRIGGER IF EXISTS fail_staging; DROP TRIGGER IF EXISTS fail_publication"); err != nil {
				t.Fatal(err)
			}
			s.Close()
			s = storageOpen(t, db, dir)
			if sqlCount(t, s, "benchmark_datasets") != 2 || sqlCount(t, s, "benchmark_observations") != 4 {
				t.Fatal("restart cleanup removed active data or retained abandoned batches")
			}
			mustSave(t, s, next)
			if sqlCount(t, s, "benchmark_datasets") != 2 || len(mustLoad(t, s).Sources["a"].Observations) != 600 {
				t.Fatal("retry failed")
			}
		})
	}
}

func TestSQLiteSerializationCancellationForeignKeysAndCredentialReaders(t *testing.T) {
	db, dir := storageDatabase(t)
	s := storageOpen(t, db, dir)
	v := storageSnapshot(2)
	mustSave(t, s, v)
	before := mustLoad(t, s)
	a := v.Sources["a"]
	a.Metadata = map[string]any{"bad": func() {}}
	v.Sources["a"] = a
	if s.Save(context.Background(), v) == nil || sqlCount(t, s, "benchmark_datasets") != 1 {
		t.Fatal("serialization mutated storage")
	}
	sameSnapshot(t, before, mustLoad(t, s))
	unlock, err := db.LockBenchmarkWriter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Save(ctx, storageSnapshot(3)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled save: %v", err)
	}
	unlock()
	if _, err := s.db.Exec("UPDATE benchmark_sources SET source_id='other' WHERE source_id='a'"); err == nil {
		t.Fatal("cross-source dataset reference accepted")
	}
	if err := db.Put("credential", state.Owner, "p", "api_key", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO benchmark_datasets VALUES('staging-reader-test','a',0,'hash','date')"); err != nil {
		t.Fatal(err)
	}
	// The benchmark writer holds its pool and SQLite write lock. Credential reads
	// must still complete through their independent WAL connection.
	result := make(chan error, 1)
	go func() {
		value, err := db.Get("credential", state.Owner)
		if err == nil && string(value) != "secret" {
			err = errors.New("wrong credential")
		}
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("credential read blocked by benchmark writer")
	}
	reader, err := db.BenchmarkConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	readStore := &Store{db: reader, database: db}
	sameSnapshot(t, before, mustLoad(t, readStore))
}

func TestDatabaseStorageIgnoresLegacyFiles(t *testing.T) {
	db, dir := storageDatabase(t)
	path := filepath.Join(dir, "benchmark-intelligence.json")
	original := []byte("not valid JSON; never read")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	s := storageOpen(t, db, dir)
	if len(mustLoad(t, s).Sources) != 0 {
		t.Fatal("imported a legacy cache")
	}
	mustSave(t, s, storageSnapshot(3))
	s.Close()
	s = storageOpen(t, db, dir)
	if len(mustLoad(t, s).Sources["a"].Observations) != 3 {
		t.Fatal("database persistence failed")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatal("legacy file changed", err)
	}
}

type blockingSnapshotStore struct {
	SnapshotStore
	entered chan struct{}
	release chan struct{}
}

func (s *blockingSnapshotStore) Save(ctx context.Context, v Snapshot) error {
	close(s.entered)
	select {
	case <-s.release:
		return s.SnapshotStore.Save(ctx, v)
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestRefreshStagingDoesNotBlockCachedRoutingAndCloseOwnsStorage(t *testing.T) {
	db, dir := storageDatabase(t)
	s := storageOpen(t, db, dir)
	mustSave(t, s, storageSnapshot(2))
	blocked := &blockingSnapshotStore{SnapshotStore: s, entered: make(chan struct{}), release: make(chan struct{})}
	m, err := NewManager(blocked, []Source{fakeSource{id: "a", fetch: func(context.Context) (Result, error) {
		return Result{Observations: storageSnapshot(3).Sources["a"].Observations}, nil
	}}}, modelCatalog(), options())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// Keep freshness assertions independent of the wall clock and CI run date.
	m.now = func() time.Time { return storageSnapshot(0).UpdatedAt }
	m.UpdateModels(modelCatalog())
	done := make(chan error, 1)
	go func() { done <- m.Refresh(context.Background()) }()
	<-blocked.entered
	lookup := make(chan bool, 1)
	go func() { _, ok := m.Lookup("local/model-a"); lookup <- ok }()
	select {
	case ok := <-lookup:
		if !ok {
			t.Error("cached routing unavailable during staging")
		}
	case <-time.After(time.Second):
		t.Error("cached routing blocked during staging")
	}
	close(blocked.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	m.Close()
	if _, err := s.Load(context.Background()); err == nil {
		t.Fatal("manager did not close its pool")
	}
	if _, ok := m.Lookup("local/model-a"); !ok {
		t.Fatal("routing unexpectedly reads closed SQLite")
	}
}
