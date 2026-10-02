package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/state"
	"github.com/Apartment38/torii-gateway/internal/usage"
)

func TestUsagePersistenceFailureDoesNotBlockInference(t *testing.T) {
	db, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	counters, err := usage.Open(db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer counters.Close(context.Background())
	pool, err := db.StateConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(`CREATE TRIGGER reject_usage BEFORE INSERT ON usage_aggregates BEGIN SELECT RAISE(ABORT,'test'); END`); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer upstream.Close()
	p, err := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: "test", BaseURL: upstream.URL, StaticModels: []string{"model"}})
	if err != nil {
		t.Fatal(err)
	}
	h, err := api.NewOpenAIHandler([]provider.OpenAIProvider{p})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RefreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.SetUsageStore(counters)
	for range 2 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test/model","messages":[{"role":"user","content":"hello"}]}`)))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "prompt_tokens") {
			t.Fatalf("inference=%d %s", w.Code, w.Body.String())
		}
	}
	if counters.Flush(context.Background()) == nil {
		t.Fatal("failure not reported")
	}
	if a, ok := counters.Get("test/model"); !ok || a.Requests != 2 || a.InputTokens != 20 {
		t.Fatal("memory counters lost")
	}
	if _, err := pool.Exec("DROP TRIGGER reject_usage"); err != nil {
		t.Fatal(err)
	}
	if err := counters.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	var requests string
	if err := pool.QueryRow("SELECT requests FROM usage_aggregates WHERE model_id='test/model'").Scan(&requests); err != nil || requests != "2" {
		t.Fatal("retry changed totals", err)
	}
}
