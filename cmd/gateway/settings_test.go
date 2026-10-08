package main

import (
	"bytes"
	"context"
	"github.com/Apartment38/torii-gateway/internal/state"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportPreservesCredentialsAndRefusesReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.yaml")
	database := filepath.Join(dir, "torii.db")
	t.Setenv("TORII_DB_KEY", strings.Repeat("41", 32))
	t.Setenv("TORII_DB_KEY_FILE", "")
	db, err := state.OpenDatabase(database, bytes.Repeat([]byte{0x41}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Put("existing", state.Owner, "p", "bearer_token", []byte("existing-value")); err != nil {
		t.Fatal(err)
	}
	db.Close()
	raw := `server: {listen: "127.0.0.1:8080"}
providers:
 - id: p
   type: openai_compatible
   base_url: https://example.test
   discover_models: false
   models: [original/model]
   bearer_token: {credential: existing}
 - id: other
   type: openai_compatible
   base_url: https://example.test
   discover_models: false
   models: [original]
   bearer_token: {value: imported-secret}
`
	if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runWithIO(context.Background(), []string{"config", "import", "--config", path, "--database", database}, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	db, err = openSettingsDatabase(database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg, err := loadDatabaseSettings(db)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].Models[0] != "original/model" {
		t.Fatal("model ID changed")
	}
	data, revision, _ := db.Settings()
	if revision != "1" || strings.Contains(string(data), "imported-secret") {
		t.Fatal("unsafe persisted settings")
	}
	if value, _ := db.Get("existing", state.Owner); string(value) != "existing-value" {
		t.Fatal("original credential changed")
	}
	if err = runConfigImport([]string{"--config", path, "--database", database}, new(bytes.Buffer)); err == nil {
		t.Fatal("overwrite accepted")
	}
}
func TestRejectedRuntimeDoesNotStoreSubmittedCredentials(t *testing.T) {
	r, db := credentialRuntime(t)
	before, revision, _ := db.Settings()
	secrets, _ := db.List(state.Owner)
	old := r.handler
	body := `server: {listen: "127.0.0.1:8080"}
providers:
 - id: p
   type: openai_compatible
   base_url: https://example.test
   discover_models: false
   models: [model]
   bearer_token: {value: rejected-value}
   tls: {ca_file: /does/not/exist}
`
	w := httptest.NewRecorder()
	r.putConfig(w, configSaveRequest(t, r, body))
	after, newRevision, _ := db.Settings()
	now, _ := db.List(state.Owner)
	if w.Code != 400 || !bytes.Equal(before, after) || revision != newRevision || len(now) != len(secrets) || r.handler != old {
		t.Fatal("rejected save mutated state", w.Code, w.Body.String())
	}
}
