package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCodexBrowserLoginNeedsNoHelperAndPersistsCredential(t *testing.T) {
	for _, host := range []string{"gateway.example.test:8398", "localhost:8398"} {
		t.Run(host, func(t *testing.T) {
			r, db := credentialRuntime(t)
			previous := http.DefaultTransport
			var starts atomic.Int32
			release := make(chan struct{})
			finished := make(chan struct{})
			var releaseOnce sync.Once
			releasePoll := func() { releaseOnce.Do(func() { close(release) }) }
			workerStarted := false
			idToken := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user","email":"user@example.test","https://api.openai.com/auth":{"chatgpt_account_id":"account"}}`)) + ".signature"
			http.DefaultTransport = codexLifecycleTransport(func(req *http.Request) (*http.Response, error) {
				body := `{"data":[{"id":"model"}]}`
				switch req.URL.Path {
				case "/api/accounts/deviceauth/usercode":
					starts.Add(1)
					body = `{"device_auth_id":"device-secret","user_code":"ONE-TIME","interval":"1"}`
				case "/api/accounts/deviceauth/token":
					<-release
					body = `{"authorization_code":"code-secret","code_verifier":"verifier-secret"}`
				case "/oauth/token":
					body = `{"access_token":"access-secret","refresh_token":"refresh-secret","id_token":"` + idToken + `","expires_in":3600,"scope":"openid offline_access"}`
				}
				return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			defer func() {
				releasePoll()
				if workerStarted {
					select {
					case <-finished:
					case <-time.After(5 * time.Second):
						t.Error("login worker did not finish")
					}
				}
				http.DefaultTransport = previous
			}()
			call := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Host = host
				if authenticated {
					req.SetBasicAuth("chosen-admin", "administrator-password")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				return w
			}
			method := ""
			if strings.HasPrefix(host, "localhost") {
				method = `"login_method":"device",`
			}
			document := `{` + method + `"provider":{"id":"codex","type":"openai-codex","base_url":"https://chatgpt.com/backend-api/codex"}}`
			if denied := call("POST", "/gateway/v1/providers/login", document, false); denied.Code != 401 {
				t.Fatal("unauthenticated login allowed")
			}
			start := call("POST", "/gateway/v1/providers/login", document, true)
			if start.Code != 202 || strings.Contains(start.Body.String(), "helper_") {
				t.Fatalf("not helper-free: %d %s", start.Code, start.Body.String())
			}
			waitStatus := func(expected string) providerLogin {
				t.Helper()
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					w := call("GET", "/gateway/v1/providers/login?provider=codex", "", true)
					for _, secret := range []string{"device-secret", "code-secret", "verifier-secret", "access-secret", "refresh-secret", idToken} {
						if strings.Contains(w.Body.String(), secret) {
							t.Error("login status exposed OAuth material")
						}
					}
					if w.Header().Get("Cache-Control") != "no-store" {
						t.Error("login status can be cached")
					}
					var status providerLogin
					_ = json.Unmarshal(w.Body.Bytes(), &status)
					if expected == "code" && status.UserCode != "" || status.Status == expected {
						return status
					}
					time.Sleep(5 * time.Millisecond)
				}
				t.Fatal("login status did not arrive")
				return providerLogin{}
			}
			// Wait for the terminal status before restoring the mock transport.
			workerStarted = true
			go func() {
				defer close(finished)
				for {
					r.loginMu.Lock()
					status := r.logins["codex"]
					r.loginMu.Unlock()
					if status.Status == "success" || status.Status == "failure" {
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
			}()
			func() {
				defer releasePoll()
				status := waitStatus("code")
				if status.UserCode != "ONE-TIME" || status.VerificationURL != "https://auth.openai.com/codex/device" {
					t.Error("missing browser instructions")
				}
				again := call("POST", "/gateway/v1/providers/login", document, true)
				if again.Code != 202 || !strings.Contains(again.Body.String(), "ONE-TIME") || starts.Load() != 1 {
					t.Error("duplicate login did not reuse pending authorization")
				}
			}()
			waitStatus("success")
			saved, err := db.Load("codex")
			if err != nil || saved.AccessToken != "access-secret" || saved.RefreshToken != "refresh-secret" || saved.AccountID != "account" || saved.Subject != "user" || saved.Email != "user@example.test" || saved.ClientID == "" || saved.IDToken != idToken || saved.Issuer != "https://auth.openai.com" || len(saved.Scopes) != 2 || saved.Endpoint != "https://chatgpt.com/backend-api/codex" {
				t.Fatal("OAuth credential context was not persisted")
			}
			if retry := call("POST", "/gateway/v1/providers/login", `{"login_method":"unknown","provider":{"id":"codex","type":"openai-codex"}}`, true); retry.Code != 400 {
				t.Error("unsupported login method accepted")
			}
		})
	}
}
