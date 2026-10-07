package main

import (
	"bytes"
	"github.com/Apartment38/torii-gateway/internal/state"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func currentConfigRevision(t *testing.T, r *handlerRuntime) string {
	t.Helper()
	r.configMu.Lock()
	defer r.configMu.Unlock()
	cfg, err := r.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	tag, err := r.configRevision(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

func configSaveRequest(t *testing.T, r *handlerRuntime, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/gateway/v1/config", strings.NewReader(body))
	req.Header.Set("If-Match", currentConfigRevision(t, r))
	return req
}

func TestConfigRevisionRejectsBeforeSideEffects(t *testing.T) {
	r, db := credentialRuntime(t)
	view := httptest.NewRecorder()
	r.getConfig(view, httptest.NewRequest(http.MethodGet, "/gateway/v1/config", nil))
	if view.Code != 200 || view.Header().Get("ETag") == "" {
		t.Fatal("missing revision", view.Code)
	}
	tag := view.Header().Get("ETag")
	before, err := os.ReadFile(r.configPath)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := db.List(state.Owner)
	if err != nil {
		t.Fatal(err)
	}
	handler := r.handler
	// A valid document with a new plaintext secret would mutate the database
	// if the revision check happened after credential storage.
	body := strings.Replace(view.Body.String(), `"models":["model"]`, `"models":["model"],"bearer_token":{"value":"rejected-secret-sentinel"}`, 1)
	if body == view.Body.String() {
		t.Fatal("credential fixture not injected")
	}
	for _, method := range []string{http.MethodPut, http.MethodPost} {
		for _, header := range []string{"", `"stale"`, "*", "W/" + tag, tag + ", " + tag} {
			req := httptest.NewRequest(method, "/gateway/v1/config", strings.NewReader(body))
			if header != "" {
				req.Header.Set("If-Match", header)
			}
			w := httptest.NewRecorder()
			r.putConfig(w, req)
			want := 412
			if header == "" {
				want = 428
			}
			if w.Code != want {
				t.Fatalf("%s %q: %d %s", method, header, w.Code, w.Body.String())
			}
		}
	}
	after, err := os.ReadFile(r.configPath)
	if err != nil {
		t.Fatal(err)
	}
	now, err := db.List(state.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || r.handler != handler || len(now) != len(secrets) {
		t.Fatal("rejected saves changed durable or runtime state")
	}
}

func TestConfigRevisionConcurrentSavesAndReload(t *testing.T) {
	r, _ := credentialRuntime(t)
	view := httptest.NewRecorder()
	r.getConfig(view, httptest.NewRequest(http.MethodGet, "/gateway/v1/config", nil))
	tag := view.Header().Get("ETag")
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPut, "/gateway/v1/config", strings.NewReader(view.Body.String()))
			req.Header.Set("If-Match", tag)
			w := httptest.NewRecorder()
			r.putConfig(w, req)
			codes <- w.Code
		}()
	}
	wg.Wait()
	a, b := <-codes, <-codes
	if !((a == 200 && b == 412) || (a == 412 && b == 200)) {
		t.Fatalf("concurrent save statuses: %d %d", a, b)
	}
	if currentConfigRevision(t, r) == tag {
		t.Fatal("no-op save did not advance revision")
	}
	w := httptest.NewRecorder()
	req := configSaveRequest(t, r, view.Body.String())
	req.Method = http.MethodPost
	r.putConfig(w, req)
	if w.Code != 200 {
		t.Fatal("fresh POST save rejected", w.Body.String())
	}
}

func TestConfigRevisionExternalEditAndRestart(t *testing.T) {
	r, _ := credentialRuntime(t)
	tag := currentConfigRevision(t, r)
	data, err := os.ReadFile(r.configPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(data, []byte("models: [model]"), []byte("models: [repaired]"), 1)
	if bytes.Equal(data, edited) {
		t.Fatal("fixture edit missed")
	}
	if err := os.WriteFile(r.configPath, edited, 0600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/gateway/v1/config", bytes.NewReader(data))
	req.Header.Set("If-Match", tag)
	w := httptest.NewRecorder()
	r.putConfig(w, req)
	if w.Code != 412 {
		t.Fatal("external edit overwritten", w.Code)
	}
	current := currentConfigRevision(t, r)
	restarted := &handlerRuntime{configPath: r.configPath, credentials: r.credentials}
	req.Header.Set("If-Match", current)
	w = httptest.NewRecorder()
	restarted.putConfig(w, req)
	if w.Code != 412 {
		t.Fatal("pre-restart revision accepted", w.Code)
	}
}
