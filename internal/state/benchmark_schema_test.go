package state

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshBaselineAndNoImportMetadata(t *testing.T) {
	d, _, _ := testDB(t)
	var version, count int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatal(version, err)
	}
	if err := d.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='imports'").Scan(&count); err != nil || count != 0 {
		t.Fatal("legacy imports table", err)
	}
	if _, err := d.db.Exec("SELECT published,legacy_import_done FROM benchmark_storage"); err == nil {
		t.Fatal("legacy columns retained")
	}
}
func TestUnversionedLookalikeInternalTableIsNotAnEmptyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE sqlitex_preserved(value TEXT); INSERT INTO sqlitex_preserved VALUES('original')"); err != nil {
		t.Fatal(err)
	}
	if accepted, err := OpenDatabase(path, bytes.Repeat([]byte{7}, 32)); err == nil {
		accepted.Close()
		t.Fatal("nonempty unversioned database accepted")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='credentials'").Scan(&count); err != nil || count != 0 {
		t.Fatal("baseline initialized over existing database", err)
	}
}
func TestPreBaselineAndNewerSchemasRejectedUnchanged(t *testing.T) {
	for _, version := range []int{0, 1, 2, 3, SchemaVersion + 1} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "existing.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('original')"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("PRAGMA user_version=" + string(rune('0'+version))); err != nil {
				t.Fatal(err)
			}
			bad, err := OpenDatabase(path, bytes.Repeat([]byte{7}, 32))
			if err == nil {
				bad.Close()
				t.Fatal("unsupported database accepted")
			}
			var value string
			var got int
			if err := db.QueryRow("SELECT value FROM preserved").Scan(&value); err != nil || value != "original" {
				t.Fatal("old data changed", err)
			}
			if err := db.QueryRow("PRAGMA user_version").Scan(&got); err != nil || got != version {
				t.Fatal("old schema changed", err)
			}
			var journal string
			if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "delete" {
				t.Fatal("rejected database changed journal mode", err)
			}
		})
	}
}
func TestFutureSchemaMigrationPreservesDataAndRetries(t *testing.T) {
	d, _, _ := testDB(t)
	if err := d.Put("key", Owner, "provider", "api_key", []byte("keep-secret")); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id='key'").Scan(&before); err != nil {
		t.Fatal(err)
	}
	steps := []schemaMigration{{6, "CREATE TABLE future_state(value TEXT); INSERT INTO future_state VALUES('new')"}}
	if err := migrateSchema(d.db, 5, 6, steps); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema(d.db, 6, 6, steps); err != nil {
		t.Fatal(err)
	}
	var after []byte
	if err := d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id='key'").Scan(&after); err != nil || !bytes.Equal(before, after) {
		t.Fatal("credential changed", err)
	}
	if err := migrateSchema(d.db, 6, 7, append(steps, schemaMigration{7, "CREATE TABLE partial(value TEXT); INSERT INTO missing VALUES(1)"})); err == nil {
		t.Fatal("failure accepted")
	}
	var version, count int
	d.db.QueryRow("PRAGMA user_version").Scan(&version)
	d.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='partial'").Scan(&count)
	if version != 6 || count != 0 {
		t.Fatal("partial upgrade committed")
	}
	if err := migrateSchema(d.db, 6, 7, append(steps, schemaMigration{7, "CREATE TABLE partial(value TEXT)"})); err != nil {
		t.Fatal(err)
	}
}
func TestMissingMigrationPathMakesNoWrites(t *testing.T) {
	d, _, _ := testDB(t)
	if err := migrateSchema(d.db, 4, 6, []schemaMigration{{5, "CREATE TABLE unwanted(value TEXT)"}}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal(err)
	}
	var count int
	d.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='unwanted'").Scan(&count)
	if count != 0 {
		t.Fatal("incomplete path made writes")
	}
}

func TestUpgradeAuthenticatesBeforeMigrationAndFreshUsesSamePath(t *testing.T) {
	d, path, key := testDB(t)
	if err := d.Put("key", Owner, "provider", "api_key", []byte("preserved")); err != nil {
		t.Fatal(err)
	}
	d.Close()
	steps := append(append([]schemaMigration{}, schemaMigrations...), schemaMigration{6, "CREATE TABLE future_state(value TEXT)"})
	if bad, err := openDatabase(path, bytes.Repeat([]byte{9}, 32), 6, steps); err == nil {
		bad.Close()
		t.Fatal("wrong key accepted")
	}
	original, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal("wrong key changed schema", err)
	}
	var count int
	original.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='future_state'").Scan(&count)
	if count != 0 {
		t.Fatal("migration ran before authentication")
	}
	original.Close()
	upgraded, err := openDatabase(path, key, 6, steps)
	if err != nil {
		t.Fatal(err)
	}
	value, err := upgraded.Get("key", Owner)
	if err != nil || string(value) != "preserved" {
		t.Fatal("upgrade lost credential", err)
	}
	upgraded.Close()
	restarted, err := openDatabase(path, key, 6, steps)
	if err != nil {
		t.Fatal("upgrade restart", err)
	}
	restarted.Close()
	fresh, err := openDatabase(filepath.Join(t.TempDir(), "fresh.db"), key, 6, steps)
	if err != nil {
		t.Fatal("fresh upgrade", err)
	}
	defer fresh.Close()
	var version int
	if err := fresh.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 6 {
		t.Fatal("fresh skipped migration", err)
	}
	if _, err := fresh.db.Exec("SELECT value FROM future_state"); err != nil {
		t.Fatal(err)
	}
}
