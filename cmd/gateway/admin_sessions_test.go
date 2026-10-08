package main

import (
	"crypto/tls"
	"encoding/json"
	"github.com/Apartment38/torii-gateway/internal/state"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCookieSessionsCSRFRestartLogoutAndOrigins(t *testing.T) {
	r, db := credentialRuntime(t)
	login := httptest.NewRequest("POST", "http://example.com/gateway/v1/session", strings.NewReader(`{"username":"chosen-admin","password":"administrator-password"}`))
	login.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, login)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var session state.AdminSession
	json.Unmarshal(w.Body.Bytes(), &session)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Secure {
		t.Fatal("unsafe HTTP session cookie")
	}
	if session.CSRF == "" || session.Username != "chosen-admin" {
		t.Fatal("incomplete session")
	}
	call := func(method, path, csrf, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://example.com"+path, nil)
		req.AddCookie(cookies[0])
		req.Header.Set("X-CSRF-Token", csrf)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if call("GET", "/gateway/v1/config", "", "").Code != 200 {
		t.Fatal("cookie auth denied")
	}
	if call("POST", "/gateway/v1/admin/api-key", "", "").Code == 200 {
		t.Fatal("CSRF bypass")
	}
	if call("POST", "/gateway/v1/admin/api-key", session.CSRF, "http://evil.test").Code != 403 {
		t.Fatal("origin bypass")
	}
	cfg, err := r.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	restarted := &handlerRuntime{credentials: db, configPath: "database"}
	handler, err := buildHandlerWithRuntime(cfg, "database", restarted)
	if err != nil {
		t.Fatal(err)
	}
	restarted.handler = handler
	defer restarted.closeUsage()
	r = restarted
	if call("GET", "/gateway/v1/session", "", "").Code != 200 {
		t.Fatal("restart lost session")
	}
	if call("DELETE", "/gateway/v1/session", "", "").Code != 403 {
		t.Fatal("logout CSRF bypass")
	}
	if call("DELETE", "/gateway/v1/session", session.CSRF, "http://example.com").Code != 204 {
		t.Fatal("logout failed")
	}
	if call("GET", "/gateway/v1/config", "", "").Code != 401 {
		t.Fatal("logout did not revoke")
	}
}
func TestCookieSecureRequiresHTTPSOrTrustedProxy(t *testing.T) {
	r, _ := credentialRuntime(t)
	for _, tc := range []struct{ tls, proxy, trusted bool }{{true, false, false}, {false, true, false}, {false, true, true}} {
		cfg, _ := r.loadConfig()
		if tc.trusted {
			cfg.Server.TrustedProxies = []string{"192.0.2.0/24"}
		}
		req := httptest.NewRequest("POST", "http://example.com/gateway/v1/session", strings.NewReader(`{"username":"chosen-admin","password":"administrator-password"}`))
		req.RemoteAddr = "192.0.2.1:1234"
		if tc.tls {
			req.TLS = &tls.ConnectionState{}
		}
		if tc.proxy {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		w := httptest.NewRecorder()
		r.adminSessionAPI(cfg.Server)(w, req)
		if w.Code != 201 || w.Result().Cookies()[0].Secure != (tc.tls || tc.trusted) {
			t.Fatal("incorrect secure cookie", tc, w.Code)
		}
	}
}
