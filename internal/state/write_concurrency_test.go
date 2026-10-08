package state

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

// A deferred transaction could read credentials/settings, then lose its write
// upgrade to an activity/accounting writer. WAL returns BUSY_SNAPSHOT immediately
// in that case, bypassing busy_timeout. Acquire the writer before the first read.
func TestWriteTransactionLocksBeforeReadingAcrossPools(t *testing.T) {
	database, err := OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	other, err := database.StateConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := database.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM credentials").Scan(&count); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := other.Exec("UPDATE accounting_lock SET value=value+1 WHERE id=1")
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("competing pool wrote during credential transaction: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = database.write(tx, "fixture", Owner, "p", "token", []byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("competing writer did not resume after commit")
	}
}
