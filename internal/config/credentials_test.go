package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/state"
)

func TestCredentialMigrationPreservesYAMLAndOAuthAndIgnoresOldInputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("MIGRATE_KEY", "provider-secret-sentinel")
	text := "# retain comment\nserver:\n  listen: 127.0.0.1:8080\n  authentication:\n    api_key: {value: gateway-secret-sentinel}\nstate:\n  directory: " + dir + "\nproviders:\n  - id: upstream\n    type: openai_compatible\n    base_url: https://example.test\n    bearer_token: {env: MIGRATE_KEY}\n    headers:\n      X-Custom: {value: header-secret-sentinel}\n    discover_models: false\n    models: [model]\n"
	os.WriteFile(path, []byte(text), 0600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := state.NewFlatFileCredentialStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	old.Save("codex", &state.Credential{AccessToken: "oauth-sentinel", RefreshToken: "refresh-sentinel", Endpoint: "https://example.test"})
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = MigrateCredentials(cfg, path, db); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, s := range []string{"gateway-secret-sentinel", "header-secret-sentinel", "MIGRATE_KEY"} {
		if bytes.Contains(raw, []byte(s)) {
			t.Fatal("plaintext left in YAML")
		}
	}
	if !strings.Contains(string(raw), "retain comment") {
		t.Fatal("comment lost")
	}
	if _, err = os.Stat(filepath.Join(dir, "auth", "codex.json")); !os.IsNotExist(err) {
		t.Fatal("legacy OAuth file retained")
	}
	oauth, err := db.Load("codex")
	if err != nil || oauth.AccessToken != "oauth-sentinel" {
		t.Fatal("OAuth migration", err)
	}
	id := cfg.Providers[0].BearerToken.Credential
	if id == "" {
		t.Fatal("missing reference")
	}
	if err = db.Put(id, state.Owner, "upstream", "bearer_token", []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	// Simulate cleanup interrupted or read-only: old YAML reappears and env is gone.
	os.WriteFile(path, []byte(text), 0600)
	os.Unsetenv("MIGRATE_KEY")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = MigrateCredentials(cfg, path, db); err != nil {
		t.Fatal(err)
	}
	v, err := cfg.Providers[0].BearerToken.Resolve()
	if err != nil || v != "replacement" {
		t.Fatal("restart resolved legacy input", err)
	}
}
func TestFailedMigrationLeavesConfigAndCredentialsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte("server:\n  listen: 127.0.0.1:8080\n  authentication:\n    api_key: {value: first-secret}\nstate:\n  directory: " + dir + "\nproviders:\n  - id: p\n    type: openai_compatible\n    base_url: https://example.test\n    bearer_token: {env: MISSING_MIGRATION_ENV}\n")
	os.WriteFile(path, raw, 0600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = MigrateCredentials(cfg, path, db); err == nil {
		t.Fatal("missing input accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("config changed before successful import")
	}
	list, _ := db.List(state.Owner)
	if len(list) != 0 {
		t.Fatal("partial credentials imported")
	}
}

func TestConfigurationParsingErrorsNeverEchoCredentials(t *testing.T) {
	const sentinel = "credential-sentinel-never-log"
	for _, raw := range []string{"server:\n  authentication: " + sentinel + "\n", sentinel + ": true\n", "server:\n  listen: 127.0.0.1:8080\nstate:\n  directory: ./state\nproviders:\n  - id: p\n    type: openai_compatible\n    base_url: https://user:" + sentinel + "@example.test/%zz\n"} {
		_, err := Decode(strings.NewReader(raw))
		if err == nil || strings.Contains(err.Error(), sentinel) {
			t.Fatal("credential-bearing parser error")
		}
	}
}

func TestReadOnlyMigrationWarnsButUsesDatabaseAfterRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte("server:\n  listen: 127.0.0.1:8080\n  authentication:\n    api_key: {value: readonly-sentinel}\nstate:\n  directory: " + dir + "\nproviders:\n  - id: p\n    type: openai_compatible\n    base_url: https://example.test\n    discover_models: false\n    models: [model]\n")
	os.WriteFile(path, raw, 0400)
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateCredentials(cfg, path, db); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, raw) {
		t.Fatal("read-only original modified")
	}
	if cfg.Server.Authentication.APIKey.Value != "" {
		t.Fatal("plaintext retained in active configuration")
	}
	id := cfg.Server.Authentication.APIKey.Credential
	if err := db.Put(id, state.Owner, "gateway", "authentication", []byte("replaced")); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateCredentials(cfg, path, db); err != nil {
		t.Fatal(err)
	}
	v, err := cfg.Server.Authentication.APIKey.Resolve()
	if err != nil || v != "replaced" {
		t.Fatal("read-only restart used plaintext")
	}
}
