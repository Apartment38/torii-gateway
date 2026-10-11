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

func TestRotatedCredentialCannotOverwriteNewLogin(t *testing.T) {
	d, _, _ := testDB(t)
	defer d.Close()
	old := &Credential{AccessToken: "old", RefreshToken: "refresh", AccountID: "first", Endpoint: "https://example.test"}
	if err := d.Save("codex", old); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveRotated("codex", old, &Credential{AccessToken: "rotated", RefreshToken: "next", AccountID: "first", Endpoint: old.Endpoint}); err != nil {
		t.Fatal(err)
	}
	current, _ := d.Load("codex")
	if current.AccessToken != "rotated" || current.RefreshToken != "next" {
		t.Fatal("rotation not persisted")
	}
	d.Save("codex", &Credential{AccessToken: "new-login", AccountID: "second", Endpoint: old.Endpoint})
	if err := d.SaveRotated("codex", old, &Credential{AccessToken: "bad"}); err == nil {
		t.Fatal("overwrote newer login")
	}
	current, _ = d.Load("codex")
	if current.AccessToken != "new-login" {
		t.Fatal("failed rotation changed valid credentials")
	}
	if d.CredentialRefreshLock("codex") != d.CredentialRefreshLock("codex") || d.CredentialRefreshLock("codex") == d.CredentialRefreshLock("other") {
		t.Fatal("refresh lock selection")
	}
}
