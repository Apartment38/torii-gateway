package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func TestGatewayClientKeyManagementAndImmediateRevocation(t *testing.T) {
	r, db := credentialRuntime(t)
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(oldLogger)
	call := func(method, path, body, key string, basic bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if basic {
			req.SetBasicAuth("chosen-admin", "administrator-password")
		} else if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	const endpoint = "/gateway/v1/admin/api-keys"
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if w := call(method, endpoint, `{"name":"client"}`, "", false); w.Code != 401 {
			t.Fatal("anonymous key management", method, w.Code)
		}
		if w := call(method, endpoint, `{"name":"client"}`, "admin-sentinel", false); w.Code != 401 {
			t.Fatal("inference key grants management", method, w.Code)
		}
	}
	for _, input := range []string{`{"name":""}`, `{"name":"client","value":"unsafe-input"}`, `{"name":"client"} {}`} {
		if w := call("POST", endpoint, input, "", true); w.Code != 400 || strings.Contains(w.Body.String(), "unsafe-input") {
			t.Fatal("invalid key request", w.Code)
		}
	}
	w := call("POST", endpoint, `{"name":"Hermes agent"}`, "", true)
	var first struct {
		Key     state.GatewayKeyMetadata `json:"key"`
		APIKey  string                   `json:"api_key"`
		Warning string                   `json:"warning"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &first) != nil || first.APIKey == "" || first.Key.Name != "Hermes agent" || !strings.Contains(first.Warning, "only once") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("key creation or once-only warning failed", w.Code)
	}
	second, otherKey, err := db.CreateGatewayAPIKey("Other client")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{first.APIKey, otherKey, "admin-sentinel"} {
		if w := call("GET", "/v1/models", "", key, false); w.Code != 200 {
			t.Fatal("active client key rejected", w.Code)
		}
	}
	if w := call("GET", endpoint, "", first.APIKey, false); w.Code != 401 {
		t.Fatal("client can manage keys")
	}
	w = call("GET", endpoint, "", "", true)
	var listed struct {
		Keys []state.GatewayKeyMetadata `json:"api_keys"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listed) != nil || len(listed.Keys) != 3 || strings.Contains(w.Body.String(), first.APIKey) || strings.Contains(w.Body.String(), otherKey) {
		t.Fatal("unsafe key list")
	}
	if w := call("GET", "/gateway/v1/config", "", "", true); strings.Contains(w.Body.String(), first.APIKey) {
		t.Fatal("config exported client key")
	}
	for _, method := range []string{"PUT", "DELETE"} {
		if w := call(method, "/gateway/v1/credentials/"+url.PathEscape(first.Key.ID), `{"value":"replacement","service":"gateway","purpose":"client_authentication"}`, "", true); w.Code != 400 {
			t.Fatal("generic credential API mutated client key", w.Code)
		}
	}
	cfg := &config.Config{Providers: []config.ProviderConfig{{BearerToken: &config.SecretRef{Credential: first.Key.ID}}}}
	if err := cfg.BindSecrets(db); err == nil {
		t.Fatal("client key used as provider credential")
	}
	request := httptest.NewRequest("DELETE", endpoint+"/"+url.PathEscape(first.Key.ID), nil)
	request.SetBasicAuth("chosen-admin", "administrator-password")
	request.Header.Set("Origin", "https://attacker.test")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code != 403 {
		t.Fatal("cross-origin key deletion")
	}
	if w := call("DELETE", endpoint+"/"+url.PathEscape(first.Key.ID), "", "", true); w.Code != 200 {
		t.Fatal("key deletion", w.Code)
	}
	if w := call("GET", "/v1/models", "", first.APIKey, false); w.Code != 401 {
		t.Fatal("deleted key still authenticates")
	}
	if w := call("GET", "/v1/models", "", otherKey, false); w.Code != 200 {
		t.Fatal("deletion invalidated another client")
	}
	if _, err := db.Get(first.Key.ID, state.Owner); err == nil {
		t.Fatal("encrypted key not removed")
	}
	if w := call("DELETE", endpoint+"/"+url.PathEscape(state.GatewayAPIKeyID), "", "", true); w.Code != 200 {
		t.Fatal("default key deletion")
	}
	if w := call("GET", "/v1/models", "", "admin-sentinel", false); w.Code != 401 {
		t.Fatal("default key still active")
	}
	if w := call("GET", endpoint, "", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), second.Name) || strings.Contains(w.Body.String(), first.Key.Name) {
		t.Fatal("key list or admin login after deletion")
	}
	if strings.Contains(logs.String(), first.APIKey) || strings.Contains(logs.String(), otherKey) {
		t.Fatal("API keys logged")
	}
}
