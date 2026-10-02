package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexIdentityDatabaseRestartLogoutAndLegacyIgnored(t *testing.T) {
	d, path, key := testDB(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "auth"), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "auth", "codex-host-id")
	if err := os.WriteFile(legacy, []byte("old-file-host"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := d.InitializeCodexHostID(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := d.CodexHostID(context.Background())
	if err != nil || len(got) != 45 || got == "old-file-host" {
		t.Fatal("invalid identity", err)
	}
	if err := d.Save("codex", &Credential{AccessToken: "access", Endpoint: "https://example.test", HostID: got}); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete("codex"); err != nil {
		t.Fatal(err)
	}
	d.Close()
	restarted, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.InitializeCodexHostID(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after, err := restarted.CodexHostID(context.Background()); err != nil || after != got {
		t.Fatal("identity lost", err)
	}
	after, err := os.ReadFile(legacy)
	if err != nil || string(after) != "old-file-host" {
		t.Fatal("legacy file changed", err)
	}
}

func TestCodexIdentityFailedWritesRollBack(t *testing.T) {
	for _, trigger := range []string{
		"CREATE TRIGGER fail BEFORE INSERT ON application_settings BEGIN SELECT RAISE(ABORT,'test'); END",
		"CREATE TRIGGER fail AFTER INSERT ON application_settings BEGIN UPDATE application_settings SET value='' WHERE key='codex_host_id'; END",
	} {
		t.Run(trigger, func(t *testing.T) {
			d, _, _ := testDB(t)
			if _, err := d.db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			if err := d.InitializeCodexHostID(context.Background()); err == nil {
				t.Fatal("invalid write accepted")
			}
			if _, err := d.CodexHostID(context.Background()); err == nil {
				t.Fatal("partial identity published")
			}
			if _, err := d.db.Exec("DROP TRIGGER fail"); err != nil {
				t.Fatal(err)
			}
			if err := d.InitializeCodexHostID(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
