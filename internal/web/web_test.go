package web_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/web"
)

func TestConsoleProviderStatusBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js unavailable; run node --test internal/web/app_test.mjs separately")
	}
	if output, err := exec.Command(node, "--test", "app_test.mjs", "auth_test.mjs").CombinedOutput(); err != nil {
		t.Fatalf("console behavior tests failed: %v\n%s", err, output)
	}
}

func TestHandlerServesConsoleAndBrandAssets(t *testing.T) {
	handler := web.Handler()
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/", want: "Torii Gateway Console"},
		{path: "/web/app.js", want: "testProvider"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
			t.Fatalf("GET %s = %d %q; want %d containing %q", test.path, response.Code, response.Body.String(), http.StatusOK, test.want)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/web/torii-gateway-icon.png", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" || response.Body.Len() == 0 {
		t.Fatalf("asset response = %d content-type=%q bytes=%d", response.Code, response.Header().Get("Content-Type"), response.Body.Len())
	}
}

func TestResetPageIsPublicAndDoesNotCacheOrLeakTokens(t *testing.T) {
	for _, path := range []string{"/reset-password", "/reset-password.html", "/web/reset-password.html"} {
		w := httptest.NewRecorder()
		web.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "id=\"reset-password\"") {
			t.Fatal("reset page unavailable", path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("missing reset page protections", path, w.Header())
		}
	}
}
