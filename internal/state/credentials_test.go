package state

import (
	"errors"
	"testing"
)

func TestOAuthDatabasePersistenceAndProviderIsolation(t *testing.T) {
	d, path, key := testDB(t)
	credential := &Credential{AccessToken: "oauth-access", RefreshToken: "oauth-refresh", Endpoint: "https://example.test"}
	if err := d.Save("codex", credential); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Load("other"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatal("provider isolation", err)
	}
	d.Close()
	restarted, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got, err := restarted.Load("codex")
	if err != nil || got.AccessToken != credential.AccessToken || got.RefreshToken != credential.RefreshToken {
		t.Fatal("persistence", err)
	}
	if err := restarted.Delete("codex"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Delete("codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Load("codex"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatal("deleted OAuth returned", err)
	}
}
