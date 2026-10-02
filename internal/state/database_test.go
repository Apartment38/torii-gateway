package state

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDB(t *testing.T) (*Database, string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "torii.db")
	key := bytes.Repeat([]byte{7}, 32)
	d, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, path, key
}
func TestEncryptedDatabaseCRUDPersistenceIsolationAndPragmas(t *testing.T) {
	d, path, key := testDB(t)
	secret := []byte("sensitive-sentinel-never-plaintext")
	for _, p := range []struct{ sql, want string }{{"PRAGMA journal_mode", "wal"}, {"PRAGMA busy_timeout", "5000"}, {"PRAGMA foreign_keys", "1"}} {
		var got string
		if err := d.db.QueryRow(p.sql).Scan(&got); err != nil || got != p.want {
			t.Fatalf("%s = %q %v", p.sql, got, err)
		}
	}
	if err := d.Put("key", Owner, "provider", "api_key", secret); err != nil {
		t.Fatal(err)
	}
	b, err := d.Get("key", Owner)
	if err != nil || !bytes.Equal(b, secret) {
		t.Fatal("round trip", err)
	}
	if _, err = d.Get("key", "other-user"); err == nil {
		t.Fatal("cross-owner read")
	}
	if err = d.Put("key", "other-user", "provider", "api_key", []byte("changed")); err == nil {
		t.Fatal("cross-owner replace")
	}
	if err = d.DeleteSecret("key", "other-user"); err == nil {
		t.Fatal("cross-owner delete")
	}
	list, err := d.List(Owner)
	if err != nil || len(list) != 1 {
		t.Fatalf("metadata list: %+v %v", list, err)
	}
	var before []byte
	d.db.QueryRow("SELECT nonce FROM credentials WHERE id='key'").Scan(&before)
	if err = d.Put("key", Owner, "provider", "api_key", secret); err != nil {
		t.Fatal(err)
	}
	var after []byte
	d.db.QueryRow("SELECT nonce FROM credentials WHERE id='key'").Scan(&after)
	if bytes.Equal(before, after) {
		t.Fatal("nonce reused")
	}
	for _, file := range []string{path, path + "-wal", path + "-shm"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, secret) {
			t.Fatal("plaintext written to SQLite")
		}
		info, _ := os.Stat(file)
		if info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe permissions on %s", file)
		}
	}
	d.Close()
	restarted, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err = restarted.Get("key", Owner); err != nil {
		t.Fatal("restart", err)
	}
	if err = restarted.DeleteSecret("key", Owner); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Get("key", Owner); err == nil {
		t.Fatal("deleted credential returned")
	}
}
func TestWrongKeyAndTamperingFailSecurely(t *testing.T) {
	for _, column := range []string{"ciphertext", "tag", "nonce", "purpose"} {
		t.Run(column, func(t *testing.T) {
			d, path, key := testDB(t)
			if err := d.Put("key", Owner, "provider", "api_key", []byte("secret")); err != nil {
				t.Fatal(err)
			}
			if column == "purpose" {
				d.db.Exec("UPDATE credentials SET purpose='tampered' WHERE id='key'")
			} else {
				d.db.Exec("UPDATE credentials SET " + column + "=zeroblob(length(" + column + ")) WHERE id='key'")
			}
			if _, err := d.Get("key", Owner); err == nil {
				t.Fatal("tampering undetected")
			}
			d.Close()
			if db, err := OpenDatabase(path, key); err == nil {
				db.Close()
				t.Fatal("tampered database accepted at startup")
			}
		})
	}
	d, path, _ := testDB(t)
	d.Close()
	if db, err := OpenDatabase(path, bytes.Repeat([]byte{8}, 32)); err == nil {
		db.Close()
		t.Fatal("wrong key accepted")
	}
}
func TestMasterKeyValidationPrecedence(t *testing.T) {
	t.Setenv("TORII_DB_KEY_FILE", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("TORII_DB_KEY", strings.Repeat("11", 32))
	if _, err := MasterKey(); err == nil {
		t.Fatal("explicit missing file fell back")
	}
	file := filepath.Join(t.TempDir(), "key")
	t.Setenv("TORII_DB_KEY_FILE", file)
	for _, value := range []string{"", "abc", strings.Repeat("xx", 32), strings.Repeat("11", 31), strings.Repeat("11", 33)} {
		os.WriteFile(file, []byte(value), 0600)
		if _, err := MasterKey(); err == nil {
			t.Fatal("invalid master key accepted")
		}
	}
	os.WriteFile(file, []byte(strings.Repeat("22", 32)+"\n"), 0600)
	key, err := MasterKey()
	if err != nil || key[0] != 0x22 {
		t.Fatal("file precedence", err)
	}
	t.Setenv("TORII_DB_KEY_FILE", "")
	t.Setenv("TORII_DB_KEY", "")
	if _, err = MasterKey(); err == nil {
		t.Fatal("missing key accepted")
	}
	t.Setenv("TORII_DB_KEY", strings.Repeat("33", 32))
	key, err = MasterKey()
	if err != nil || key[0] != 0x33 {
		t.Fatal("environment fallback", err)
	}
}
