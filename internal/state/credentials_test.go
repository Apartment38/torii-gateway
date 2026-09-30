package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-router/internal/state"
)

func TestFlatFileCredentialStorePersistsAcrossRestart(t *testing.T) {
	directory := t.TempDir()
	store, err := state.NewFlatFileCredentialStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := &state.Credential{AccessToken: "access-secret", RefreshToken: "refresh-secret", TokenType: "Bearer", ExpiresAt: time.Unix(1_800_000_000, 0).UTC(), AccountID: "account", PrincipalID: "principal", Endpoint: "https://codex.example.test"}
	if err := store.Save("openai-codex", want); err != nil {
		t.Fatal(err)
	}
	restarted, err := state.NewFlatFileCredentialStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Load("openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	info, err := os.Stat(filepath.Join(directory, "auth", "openai-codex.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestFlatFileCredentialStoreIsolatesProviders(t *testing.T) {
	store, err := state.NewFlatFileCredentialStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for id, token := range map[string]string{"openai-codex": "codex-token", "other": "other-token"} {
		if err := store.Save(id, &state.Credential{AccessToken: token, Endpoint: "https://" + id + ".example.test"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Delete("openai-codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("openai-codex"); !errors.Is(err, state.ErrCredentialNotFound) {
		t.Fatalf("deleted Load()=%v", err)
	}
	other, err := store.Load("other")
	if err != nil || other.AccessToken != "other-token" {
		t.Fatalf("other credential=%#v, %v", other, err)
	}
}

func TestFlatFileCredentialStoreRejectsCorruptionWithoutLeakingSecrets(t *testing.T) {
	directory := t.TempDir()
	store, err := state.NewFlatFileCredentialStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	secret := "must-never-appear-in-error"
	path := filepath.Join(directory, "auth", "openai-codex.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"provider":"wrong","credential":{"access_token":"`+secret+`","endpoint":"https://example.test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Load("openai-codex")
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("Load()=%v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestFlatFileCredentialStoreRejectsUnsafeFilesAndIDs(t *testing.T) {
	directory := t.TempDir()
	store, err := state.NewFlatFileCredentialStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("../escape", &state.Credential{AccessToken: "x", Endpoint: "https://example.test"}); err == nil {
		t.Fatal("Save() accepted unsafe ID")
	}
	path := filepath.Join(directory, "auth", "provider.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if _, err := store.Load("provider"); err == nil || !strings.Contains(err.Error(), "permissions") {
			t.Fatalf("Load()=%v", err)
		}
	}
}

func TestFlatFileCredentialStoreRequiresEndpointBinding(t *testing.T) {
	store, err := state.NewFlatFileCredentialStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("provider", &state.Credential{AccessToken: "secret"}); err == nil || !strings.Contains(err.Error(), "endpoint binding") {
		t.Fatalf("Save()=%v", err)
	}
}
