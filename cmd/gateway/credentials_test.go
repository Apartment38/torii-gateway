package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func credentialRuntime(t *testing.T, setup ...bool) (*handlerRuntime, *state.Database) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := "server:\n  listen: 127.0.0.1:8080\nproviders:\n  - id: p\n    type: openai_compatible\n    base_url: https://example.test\n    discover_models: false\n    models: [model]\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenDatabase(filepath.Join(dir, "torii.db"), bytes.Repeat([]byte{6}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := cfg.BindSecrets(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Put(state.GatewayAPIKeyID, state.Owner, "gateway", "authentication", []byte("admin-sentinel")); err != nil {
		t.Fatal(err)
	}
	if len(setup) == 0 || setup[0] {
		if _, err := db.CreateAdmin("chosen-admin", "administrator-password"); err != nil {
			t.Fatal(err)
		}
	}
	r := &handlerRuntime{configPath: path, credentials: db}
	h, err := buildHandlerWithRuntime(cfg, path, r)
	if err != nil {
		t.Fatal(err)
	}
	r.handler = h
	t.Cleanup(func() { r.stopIntelligence(); r.closeUsage() })
	return r, db
}
func TestCredentialManagementAuthenticationCRUDAndNoDisclosure(t *testing.T) {
	r, db := credentialRuntime(t)
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(old)
	call := func(method, path, body, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if key != "" {
			req.SetBasicAuth("chosen-admin", "administrator-password")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, path := range []string{"/gateway/v1/config", "/gateway/v1/credentials", "/gateway/v1/intelligence/sources", "/gateway/v1/providers/login"} {
		if w := call("GET", path, "", ""); w.Code != 401 {
			t.Fatalf("unprotected %s: %d", path, w.Code)
		}
	}
	const secret = "provider-sentinel-that-must-not-leak"
	w := call("POST", "/gateway/v1/credentials", `{"service":"p","purpose":"api_key","value":"`+secret+`"}`, "admin-sentinel")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("create leaked secret")
	}
	var reply map[string]string
	json.Unmarshal(w.Body.Bytes(), &reply)
	id := reply["credential"]
	if w = call("GET", "/gateway/v1/credentials", "", "admin-sentinel"); w.Code != 200 || strings.Contains(w.Body.String(), secret) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe metadata response")
	}
	if w = call("GET", "/gateway/v1/config", "", "admin-sentinel"); w.Code != 200 || strings.Contains(w.Body.String(), "admin-sentinel") {
		t.Fatal("unsafe config response")
	}
	if err := db.Put("foreign", "another-user", "p", "api_key", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if w = call("PUT", "/gateway/v1/credentials/foreign", `{"value":"replacement","service":"p","purpose":"api_key"}`, "admin-sentinel"); w.Code != 404 {
		t.Fatal("cross-owner replacement allowed")
	}
	if w = call("PUT", "/gateway/v1/credentials/"+id, `{"value":"updated","service":"p","purpose":"api_key"}`, "admin-sentinel"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	value, _ := db.Get(id, state.Owner)
	if string(value) != "updated" {
		t.Fatal("replacement failed")
	}
	req := httptest.NewRequest("DELETE", "/gateway/v1/credentials/"+id, nil)
	req.SetBasicAuth("chosen-admin", "administrator-password")
	req.Header.Set("Origin", "https://evil.test")
	blocked := httptest.NewRecorder()
	r.ServeHTTP(blocked, req)
	if blocked.Code != 403 {
		t.Fatal("cross-origin mutation allowed")
	}
	if w = call("DELETE", "/gateway/v1/credentials/"+id, "", "admin-sentinel"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := db.Get(id, state.Owner); err == nil {
		t.Fatal("delete failed")
	}
	// Decoder errors must not quote secret-bearing input.
	w = call("PUT", "/gateway/v1/config", `{"providers":"`+secret+`"}`, "admin-sentinel")
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("parser echoed credential")
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "admin-sentinel") {
		t.Fatal("credential logged")
	}
}
func TestProviderUsesCurrentEncryptedCredentialAndSuppressesUpstreamErrorEcho(t *testing.T) {
	db, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Put("key", state.Owner, "p", "bearer_token", []byte("first")); err != nil {
		t.Fatal(err)
	}
	var expected = "first"
	var echo bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+expected {
			t.Errorf("stale or missing outbound credential: %q", r.Header.Get("Authorization"))
		}
		if echo {
			w.WriteHeader(401)
			io.WriteString(w, "rejected secret "+expected)
			return
		}
		io.WriteString(w, `{"data":[{"id":"model"}]}`)
	}))
	defer upstream.Close()
	ref := config.SecretRef{Credential: "key"}
	ref.Bind(db)
	p, err := buildProviderWithState(config.ProviderConfig{ID: "p", Type: "openai_compatible", BaseURL: upstream.URL, BearerToken: &ref}, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Models(t.Context()); err != nil {
		t.Fatal(err)
	}
	expected = "replacement"
	if err = db.Put("key", state.Owner, "p", "bearer_token", []byte(expected)); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Models(t.Context()); err != nil {
		t.Fatal(err)
	}
	echo = true
	_, err = p.Models(t.Context())
	if err == nil || strings.Contains(err.Error(), expected) {
		t.Fatal("upstream error echoed credential")
	}
}
