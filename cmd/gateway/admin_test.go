package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func TestWebAdminSetupAuthorizationRotationAndOneTimeDisplay(t *testing.T) {
	r, db := credentialRuntime(t, false)
	if err := db.DeleteSecret(state.GatewayAPIKeyID, state.Owner); err != nil {
		t.Fatal(err)
	}
	r.setupDigest = sha256.Sum256([]byte("terminal-setup-code"))
	r.setupExpires = time.Now().Add(time.Minute)
	call := func(method, path, body, authorization, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", authorization)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	setup := `{"username":"selected-user","password":"selected-admin-password","setup_code":"terminal-setup-code"}`
	if w := call("GET", "/gateway/v1/setup", "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "true") {
		t.Fatal("setup status")
	}
	if w := call("POST", "/gateway/v1/setup", setup, "", "https://attacker.test"); w.Code != 403 {
		t.Fatal("cross-origin setup")
	}
	if w := call("POST", "/gateway/v1/setup", strings.ReplaceAll(setup, "terminal-setup-code", "wrong-code"), "", ""); w.Code != 403 {
		t.Fatal("unguarded setup")
	}
	w := call("POST", "/gateway/v1/setup", setup, "", "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.APIKey == "" {
		t.Fatal("new key missing")
	}
	oldKey := result.APIKey
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API key response cached")
	}
	if w = call("POST", "/gateway/v1/setup", setup, "", ""); w.Code != 409 || strings.Contains(w.Body.String(), oldKey) {
		t.Fatal("setup replay")
	}
	if w = call("GET", "/gateway/v1/config", "", "Bearer "+oldKey, ""); w.Code != 401 {
		t.Fatal("API key grants credential management")
	}
	basicReq := httptest.NewRequest("GET", "/", nil)
	basicReq.SetBasicAuth("selected-user", "selected-admin-password")
	basic := basicReq.Header.Get("Authorization")
	for _, path := range []string{"/gateway/v1/credentials/__admin", "/gateway/v1/credentials/__key_check", "/gateway/v1/credentials/gateway/api-key"} {
		if w = call("DELETE", path, "", basic, ""); w.Code != 400 {
			t.Fatal("system credential deletion", path, w.Code)
		}
	}
	w = call("POST", "/gateway/v1/admin/api-key", "", basic, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	newKey := result.APIKey
	if newKey == oldKey || newKey == "" {
		t.Fatal("rotation failed")
	}
	if w = call("GET", "/v1/models", "", "Bearer "+oldKey, ""); w.Code != 401 {
		t.Fatal("old key still valid")
	}
	if w = call("GET", "/v1/models", "", "Bearer "+newKey, ""); w.Code != 200 {
		t.Fatal("new key rejected", w.Code)
	}
	if w = call("GET", "/gateway/v1/config", "", basic, ""); w.Code != 200 || strings.Contains(w.Body.String(), newKey) || strings.Contains(w.Body.String(), "selected-admin-password") {
		t.Fatal("config leaked auth")
	}
	if w = call("GET", "/gateway/v1/credentials", "", basic, ""); w.Code != 200 || strings.Contains(w.Body.String(), newKey) || strings.Contains(w.Body.String(), "__admin") {
		t.Fatal("metadata leaked auth")
	}
}
func TestExpiredSetupCodeAndImportedKeyPreservation(t *testing.T) {
	r, db := credentialRuntime(t, false)
	r.setupDigest = sha256.Sum256([]byte("code"))
	r.setupExpires = time.Now().Add(-time.Second)
	body := `{"username":"chosen-user","password":"long-enough-password","setup_code":"code"}`
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/gateway/v1/setup", strings.NewReader(body)))
		return w
	}
	if w := request(); w.Code != 403 {
		t.Fatal("expired setup code accepted")
	}
	r.setupExpires = time.Now().Add(time.Minute)
	w := request()
	if w.Code != 201 || strings.Contains(w.Body.String(), "admin-sentinel") {
		t.Fatal("preservation", w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["api_key"] != "" || result["api_key_generated"] != false {
		t.Fatal("imported key displayed")
	}
	key, _ := db.GatewayAPIKey()
	if key != "admin-sentinel" {
		t.Fatal("existing key lost")
	}
}
func passwordStdin(t *testing.T, password string) {
	t.Helper()
	prior := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString(password + "\n")
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prior; r.Close() })
}
func TestAdminCLIAndLegacyEnvironmentImportOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := initializeConfigPath(path, "", true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TORII_DB_KEY", strings.Repeat("ab", 32))
	t.Setenv("TORII_DB_KEY_FILE", "")
	t.Setenv("TORII_DB_PATH", filepath.Join(dir, "torii.db"))
	t.Setenv("TORII_API_KEY", "legacy-env-key")
	passwordStdin(t, "chosen-admin-password")
	var output bytes.Buffer
	if err := runAdmin(context.Background(), []string{"create", "--config", path, "--username", "custom-name"}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "legacy-env-key") {
		t.Fatal("imported key disclosed")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Authentication != nil {
		t.Fatal("legacy authentication retained in YAML")
	}
	t.Setenv("TORII_API_KEY", "changed-env-key")
	db, err := openCredentials(cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := db.GatewayAPIKey()
	if key != "legacy-env-key" || !db.VerifyAdmin("custom-name", "chosen-admin-password") {
		t.Fatal("migration changed credentials")
	}
	db.Close()
	// A pipe is consumed once; reset it for each command.
	passwordStdin(t, "wrong-password")
	if err := runAdmin(context.Background(), []string{"api-key", "--config", path}, io.Discard); err == nil {
		t.Fatal("unauthorized terminal rotation")
	}
	passwordStdin(t, "chosen-admin-password")
	output.Reset()
	if err := runAdmin(context.Background(), []string{"api-key", "--config", path}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "displayed only once") || !strings.Contains(output.String(), "torii_") {
		t.Fatal("new key warning missing")
	}
	db, err = openCredentials(cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key, _ = db.GatewayAPIKey()
	if key == "legacy-env-key" || key == "changed-env-key" {
		t.Fatal("rotation overridden by environment")
	}
}
