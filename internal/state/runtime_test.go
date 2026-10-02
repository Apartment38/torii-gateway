package state

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeSchemaUpgradeFromV2PreservesBenchmarksAndCredentials(t *testing.T) {
	d, path, key := testDB(t)
	if err := d.Put("key", Owner, "p", "api_key", []byte("original")); err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	if err := d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id='key'").Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec(`INSERT INTO benchmark_sources(source_id,type,error,metadata) VALUES('existing','benchlm','','{}');DROP TABLE usage_aggregates;DROP TABLE application_settings;PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	if wrong, err := OpenDatabase(path, bytes.Repeat([]byte{9}, 32)); err == nil {
		wrong.Close()
		t.Fatal("wrong key accepted")
	}
	var version int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatal("wrong key migrated schema", err)
	}
	d.Close()
	upgraded, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if err := upgraded.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatal("wrong schema", err)
	}
	var after []byte
	if err := upgraded.db.QueryRow("SELECT ciphertext FROM credentials WHERE id='key'").Scan(&after); err != nil || !bytes.Equal(after, ciphertext) {
		t.Fatal("credential changed", err)
	}
	var count int
	if err := upgraded.db.QueryRow("SELECT count(*) FROM benchmark_sources WHERE source_id='existing'").Scan(&count); err != nil || count != 1 {
		t.Fatal("benchmark lost", err)
	}
}
func TestRuntimeSchemaFailureRollsBack(t *testing.T) {
	d, path, key := testDB(t)
	if _, err := d.db.Exec(`DROP TABLE usage_aggregates;DROP TABLE application_settings;CREATE TABLE application_settings(conflict TEXT);PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	if bad, err := OpenDatabase(path, key); err == nil {
		bad.Close()
		t.Fatal("conflict accepted")
	}
	var version, count int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatal("failed migration changed version", err)
	}
	if err := d.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='usage_aggregates'").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial migration committed", err)
	}
	if _, err := d.db.Exec("DROP TABLE application_settings"); err != nil {
		t.Fatal(err)
	}
	retry, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Close()
}
func TestCodexIdentityImportRestartLogoutAndDatabaseAuthority(t *testing.T) {
	d, path, key := testDB(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "auth"), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "auth", "codex-host-id")
	original := []byte("urn:uuid:original-host\n")
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.InitializeCodexHostID(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	got, err := d.CodexHostID(context.Background())
	if err != nil || got != "urn:uuid:original-host" {
		t.Fatal("identity not imported", err)
	}
	file, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(file, original) {
		t.Fatal("legacy file changed", err)
	}
	if err := d.Save("codex", &Credential{AccessToken: "access", Endpoint: "https://example.test", HostID: got}); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete("codex"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("changed-host"), 0600); err != nil {
		t.Fatal(err)
	}
	d.Close()
	restarted, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.InitializeCodexHostID(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if after, err := restarted.CodexHostID(context.Background()); err != nil || after != got {
		t.Fatal("restart/logout/legacy changed identity", err)
	}
}
func TestCodexIdentityInvalidInputRollbackAndOmittedDirectory(t *testing.T) {
	d, _, _ := testDB(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "auth"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auth", "codex-host-id")
	if err := os.WriteFile(path, []byte(" \n"), 0600); err != nil {
		t.Fatal(err)
	}
	if d.InitializeCodexHostID(context.Background(), dir) == nil {
		t.Fatal("empty legacy identity silently replaced")
	}
	if _, err := d.CodexHostID(context.Background()); err == nil {
		t.Fatal("failed import published identity")
	}
	if err := os.WriteFile(path, []byte("restored-host"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec(`CREATE TRIGGER reject_identity BEFORE INSERT ON application_settings WHEN NEW.key='codex_host_id' BEGIN SELECT RAISE(ABORT,'test'); END`); err != nil {
		t.Fatal(err)
	}
	if d.InitializeCodexHostID(context.Background(), dir) == nil {
		t.Fatal("failed write accepted")
	}
	if _, err := d.db.Exec("DROP TRIGGER reject_identity"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec(`CREATE TRIGGER modify_identity AFTER INSERT ON application_settings WHEN NEW.key='codex_host_id' BEGIN UPDATE application_settings SET value='modified' WHERE key='codex_host_id'; END`); err != nil {
		t.Fatal(err)
	}
	if d.InitializeCodexHostID(context.Background(), dir) == nil {
		t.Fatal("modified import accepted")
	}
	if _, err := d.CodexHostID(context.Background()); err == nil {
		t.Fatal("verification failure committed identity")
	}
	if _, err := d.db.Exec("DROP TRIGGER modify_identity"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := d.InitializeCodexHostID(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	got, err := d.CodexHostID(context.Background())
	if err != nil || got == "restored-host" {
		t.Fatal("omitted directory searched CWD", err)
	}
	if len(got) != 45 {
		t.Fatalf("invalid generated UUID: %s", got)
	}
}
