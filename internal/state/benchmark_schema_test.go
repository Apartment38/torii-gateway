package state

import (
	"bytes"
	"context"
	"testing"
)

func legacySchema(t *testing.T, d *Database) {
	t.Helper()
	_, err := d.db.Exec(`DROP TABLE usage_aggregates; DROP TABLE application_settings; DROP TABLE benchmark_observations; DROP TABLE benchmark_sources; DROP TABLE benchmark_datasets; DROP TABLE benchmark_storage; PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBenchmarkSchemaUpgradePreservesEncryptedCredentials(t *testing.T) {
	d, path, key := testDB(t)
	if err := d.Put("legacy-key", Owner, "p", "api_key", []byte("original-secret")); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id='legacy-key'").Scan(&before); err != nil {
		t.Fatal(err)
	}
	legacySchema(t, d)
	d.Close()
	// Key verification precedes schema writes.
	if wrong, err := OpenDatabase(path, bytes.Repeat([]byte{9}, 32)); err == nil {
		wrong.Close()
		t.Fatal("wrong key accepted")
	}
	upgraded, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version int
	if err := upgraded.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatalf("version=%d: %v", version, err)
	}
	var after []byte
	if err := upgraded.db.QueryRow("SELECT ciphertext FROM credentials WHERE id='legacy-key'").Scan(&after); err != nil || !bytes.Equal(before, after) {
		t.Fatal("credential rewritten", err)
	}
	got, err := upgraded.Get("legacy-key", Owner)
	if err != nil || string(got) != "original-secret" {
		t.Fatal("credential lost", err)
	}
	pool, err := upgraded.BenchmarkConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for query, want := range map[string]string{"PRAGMA journal_mode": "wal", "PRAGMA busy_timeout": "5000", "PRAGMA foreign_keys": "1", "PRAGMA synchronous": "2"} {
		var got string
		if err := pool.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s=%s: %v", query, got, err)
		}
	}
	upgraded.Close()
	if err := pool.PingContext(context.Background()); err != nil {
		t.Fatal("benchmark pool is not independently owned", err)
	}
}

func TestBenchmarkSchemaFailureRollsBackAndWrongKeyDoesNotMigrate(t *testing.T) {
	d, path, key := testDB(t)
	legacySchema(t, d)
	if err := d.Put("key", Owner, "p", "api_key", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec("CREATE TABLE benchmark_sources(conflicting TEXT)"); err != nil {
		t.Fatal(err)
	}
	if wrong, err := OpenDatabase(path, bytes.Repeat([]byte{9}, 32)); err == nil {
		wrong.Close()
		t.Fatal("wrong key accepted")
	}
	var version, tables int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatal("wrong key mutated schema", err)
	}
	if failed, err := OpenDatabase(path, key); err == nil {
		failed.Close()
		t.Fatal("conflicting schema accepted")
	}
	if err := d.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='benchmark_datasets'").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("partial migration committed", err)
	}
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatal("failed migration changed version", err)
	}
	if got, err := d.Get("key", Owner); err != nil || string(got) != "secret" {
		t.Fatal("credential affected by failed migration", err)
	}
	if _, err := d.db.Exec("DROP TABLE benchmark_sources"); err != nil {
		t.Fatal(err)
	}
	retried, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal("migration retry failed", err)
	}
	defer retried.Close()
}

func TestFutureSchemaRejectedWithoutModification(t *testing.T) {
	d, path, key := testDB(t)
	if _, err := d.db.Exec("PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	if future, err := OpenDatabase(path, key); err == nil {
		future.Close()
		t.Fatal("future schema accepted")
	}
	var version int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		t.Fatal("future schema modified", err)
	}
}
