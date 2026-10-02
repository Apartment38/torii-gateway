package usage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/state"
)

func database(t *testing.T) (*state.Database, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dir
}
func store(t *testing.T, db *state.Database, dir string, worker bool) *Store {
	t.Helper()
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if !worker {
		s.cancel()
		<-s.done
	}
	t.Cleanup(func() { s.Close(context.Background()) })
	return s
}
func add(t *testing.T, s *Store, model string) {
	t.Helper()
	if err := s.Add(model, 10, 5, .25); err != nil {
		t.Fatal(err)
	}
}
func persisted(t *testing.T, s *Store) map[string]Aggregate {
	t.Helper()
	values, err := load(s.db)
	if err != nil {
		t.Fatal(err)
	}
	return values
}
func TestStoreBatchesPersistsAndRestarts(t *testing.T) {
	db, dir := database(t)
	s := store(t, db, "", false)
	add(t, s, "local/model")
	if len(persisted(t, s)) != 0 {
		t.Fatal("Add performed synchronous persistence")
	}
	if got, _ := s.Get("local/model"); got.Requests != 1 {
		t.Fatal("in-memory counters not immediate")
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	add(t, s, "local/model")
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	db.Close()
	restarted, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got := store(t, restarted, "", false).Snapshot()["local/model"]
	if got.Requests != 2 || got.InputTokens != 20 || got.OutputTokens != 10 || got.EstimatedCost != .5 {
		t.Fatalf("aggregate=%+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "usage.json")); !os.IsNotExist(err) {
		t.Fatal("plaintext usage file created")
	}
}
func TestWorkerFlushesAndRejectsOverflow(t *testing.T) {
	db, _ := database(t)
	s := store(t, db, "", true)
	add(t, s, "model")
	deadline := time.Now().Add(5 * time.Second)
	for len(persisted(t, s)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("scheduled flush did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, cost := range []float64{-1, math.NaN(), math.Inf(1)} {
		if s.Add("model", 0, 0, cost) == nil {
			t.Fatal("invalid cost accepted")
		}
	}
	s.mu.Lock()
	s.models["overflow"] = Aggregate{Requests: math.MaxUint64}
	s.models["tokens"] = Aggregate{InputTokens: math.MaxUint64}
	s.models["cost"] = Aggregate{EstimatedCost: math.MaxFloat64}
	s.mu.Unlock()
	if s.Add("overflow", 0, 0, 0) == nil || s.Add("tokens", 1, 0, 0) == nil || s.Add("cost", 0, 0, math.MaxFloat64) == nil {
		t.Fatal("overflow accepted")
	}
}
func TestFailedBatchesRetryAbsoluteTotalsWithoutDoubleCounting(t *testing.T) {
	db, _ := database(t)
	s := store(t, db, "", false)
	for i := 0; i < 600; i++ {
		add(t, s, fmt.Sprintf("model-%03d", i))
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_batch BEFORE INSERT ON usage_aggregates WHEN NEW.model_id='model-300' BEGIN SELECT RAISE(ABORT,'test'); END`); err != nil {
		t.Fatal(err)
	}
	if s.Flush(context.Background()) == nil {
		t.Fatal("failed batch accepted")
	}
	if len(persisted(t, s)) != 250 || len(s.Snapshot()) != 600 {
		t.Fatal("partial batch handling incorrect")
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_batch"); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	values := persisted(t, s)
	if len(values) != 600 {
		t.Fatal("pending totals lost")
	}
	for _, a := range values {
		if a.Requests != 1 {
			t.Fatal("retry double counted")
		}
	}
	if len(s.dirty) != 0 {
		t.Fatal("persisted totals still dirty")
	}
}
func TestAddDuringBlockedFlushRemainsDirtyAndCredentialReadsContinue(t *testing.T) {
	db, _ := database(t)
	s := store(t, db, "", false)
	add(t, s, "model")
	if err := db.Put("key", state.Owner, "p", "api_key", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.StateConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	tx, err := blocker.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO application_settings VALUES('writer-lock','test','date')"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Flush(context.Background()) }()
	deadline := time.Now().Add(time.Second)
	for s.db.Stats().InUse == 0 {
		if time.Now().After(deadline) {
			t.Fatal("flush did not start")
		}
		time.Sleep(time.Millisecond)
	}
	add(t, s, "model")
	if got, err := db.Get("key", state.Owner); err != nil || string(got) != "secret" {
		t.Fatal("credential reader blocked or failed", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if persisted(t, s)["model"].Requests != 1 || !s.dirty["model"] {
		t.Fatal("concurrent Add was cleared by older flush")
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if persisted(t, s)["model"].Requests != 2 {
		t.Fatal("concurrent Add lost")
	}
}
func TestConcurrentAddsAndCancelledShutdown(t *testing.T) {
	db, _ := database(t)
	s := store(t, db, "", false)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if err := s.Add("model", 1, 1, 0); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if persisted(t, s)["model"].Requests != 1000 {
		t.Fatal("concurrent counters lost")
	}
	add(t, s, "model")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown=%v", err)
	}
	if s.Add("model", 0, 0, 0) == nil {
		t.Fatal("Add after close accepted")
	}
	reopened := store(t, db, "", false)
	if got, _ := reopened.Get("model"); got.Requests != 1000 {
		t.Fatal("failed final flush altered saved totals")
	}
}
func TestDatabaseUsageIgnoresLegacyFiles(t *testing.T) {
	db, dir := database(t)
	path := filepath.Join(dir, "usage.json")
	original := []byte("invalid JSON; never read")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	s := store(t, db, dir, false)
	if len(s.Snapshot()) != 0 {
		t.Fatal("imported legacy usage")
	}
	add(t, s, "model")
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted := store(t, db, dir, false)
	if got, _ := restarted.Get("model"); got.Requests != 1 {
		t.Fatal("usage not persisted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatal("legacy file changed", err)
	}
}
