package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func TestAdminAccountBoundaryReauthenticationAndUpdate(t *testing.T) {
	r, db := credentialRuntime(t)
	call := func(method, body, csrf, origin string, cookie *http.Cookie, basic bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://example.com/gateway/v1/admin/account", strings.NewReader(body))
		req.Header.Set("X-CSRF-Token", csrf)
		req.Header.Set("Origin", origin)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if basic {
			req.SetBasicAuth("chosen-admin", "administrator-password")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := call("GET", "", "", "", nil, false); w.Code != 401 {
		t.Fatal("anonymous account access", w.Code)
	}
	w := call("GET", "", "", "", nil, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "hash") || strings.Contains(w.Body.String(), "salt") {
		t.Fatal(w.Body.String())
	}
	token, session, err := db.NewAdminSession(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: adminCookie, Value: token}
	body := `{"username":"renamed","email":"admin@example.com","password":"new-password-123","current_password":"administrator-password"}`
	if w = call("PUT", body, "", "", cookie, false); w.Code != 401 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w = call("PUT", body, session.CSRF, "https://evil.example", cookie, false); w.Code != 403 {
		t.Fatal("origin bypass", w.Code)
	}
	if w = call("PUT", strings.Replace(body, "administrator-password", "wrong-password", 1), session.CSRF, "", cookie, false); w.Code != 401 {
		t.Fatal("current password bypass", w.Code)
	}
	if w = call("PUT", body, session.CSRF, "", cookie, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !db.VerifyAdmin("renamed", "new-password-123") {
		t.Fatal("account not changed")
	}
	if w = call("GET", "", "", "", cookie, false); w.Code != 401 {
		t.Fatal("session survived account update")
	}
}
func TestPasswordResetRequestDeliveryGenericResponsesAndConfirmation(t *testing.T) {
	r, db := credentialRuntime(t)
	if err := db.UpdateAdmin("chosen-admin", "admin@example.com", "", "administrator-password"); err != nil {
		t.Fatal(err)
	}
	delivered := make(chan string, 1)
	r.resetSender = func(email, token string) error {
		if email != "admin@example.com" {
			t.Errorf("wrong recipient %s", email)
		}
		delivered <- token
		return nil
	}
	handler := r.passwordResetAPI(config.ServerConfig{})
	call := func(path, body, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://example.com"+path, strings.NewReader(body))
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler(w, req)
		return w
	}
	first := call("/gateway/v1/password-reset", `{"email":"admin@example.com"}`, "")
	if first.Code != 202 || first.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(first.Code)
	}
	var token string
	select {
	case token = <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("email not delivered")
	}
	unknown := call("/gateway/v1/password-reset", `{"email":"unknown@example.com"}`, "")
	if unknown.Code != first.Code || unknown.Body.String() != first.Body.String() {
		t.Fatal("account enumeration")
	}
	if w := call("/gateway/v1/password-reset", `{"email":"admin@example.com"}`, "https://evil.example"); w.Code != 403 {
		t.Fatal("reset origin bypass")
	}
	browser, _, _ := db.NewAdminSession(time.Hour)
	body, _ := json.Marshal(map[string]string{"token": token, "password": "new-password-123"})
	// Check consecutive attempts before password hashing can outlast the
	// cooldown on slower machines or under the race detector.
	if w := call("/gateway/v1/password-reset/confirm", `{"token":"invalid","password":"new-password-123"}`, ""); w.Code != 400 {
		t.Fatal("invalid reset token accepted", w.Code)
	}
	if w := call("/gateway/v1/password-reset/confirm", string(body), ""); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("reset attempts not limited", w.Code)
	}
	r.resetMu.Lock()
	r.resetAttemptAfter = time.Time{}
	r.resetMu.Unlock()
	if w := call("/gateway/v1/password-reset/confirm", string(body), ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !db.VerifyAdmin("chosen-admin", "new-password-123") {
		t.Fatal("reset failed")
	}
	if _, err := db.AdminSession(browser); err == nil {
		t.Fatal("session survived reset")
	}
	r.resetMu.Lock()
	r.resetAttemptAfter = time.Time{}
	r.resetMu.Unlock()
	if w := call("/gateway/v1/password-reset/confirm", string(body), ""); w.Code != 400 || !strings.Contains(w.Body.String(), state.ErrInvalidReset.Error()) {
		t.Fatal("link replay accepted")
	}
}
func TestResetMailRequiresHTTPSAndTLSConfiguration(t *testing.T) {
	t.Setenv("TORII_PUBLIC_URL", "https://gateway.example")
	t.Setenv("TORII_SMTP_ADDR", "smtp.example:587")
	t.Setenv("TORII_SMTP_FROM", "torii@example.com")
	t.Setenv("TORII_SMTP_TLS", "starttls")
	t.Setenv("TORII_SMTP_USERNAME", "")
	t.Setenv("TORII_SMTP_PASSWORD", "")
	t.Setenv("TORII_SMTP_PASSWORD_FILE", "")
	if _, err := passwordResetSenderFromEnv(); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"http://gateway.example", "https://user:password@gateway.example", "https://gateway.example/path", "https://gateway.example?query=1"} {
		t.Setenv("TORII_PUBLIC_URL", base)
		if _, err := passwordResetSenderFromEnv(); err == nil {
			t.Fatal("unsafe reset origin", base)
		}
	}
	t.Setenv("TORII_PUBLIC_URL", "https://gateway.example")
	t.Setenv("TORII_SMTP_TLS", "none")
	if _, err := passwordResetSenderFromEnv(); err == nil {
		t.Fatal("plaintext SMTP allowed")
	}
}
