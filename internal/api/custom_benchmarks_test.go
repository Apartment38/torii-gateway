package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/intelligence"
)

func TestCustomBenchmarkAPI(t *testing.T) {
	h, m := consoleBenchmarkManager(t, nil)
	send := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	template := send(http.MethodGet, "/gateway/v1/intelligence/template.csv", "")
	if template.Code != 200 || !strings.Contains(template.Body.String(), "normalized_score") {
		t.Fatal("template unavailable")
	}
	csv := strings.Replace(intelligence.CSVTemplate, "provider/model", "local/model-a", 1)
	if w := send(http.MethodPost, "/gateway/v1/intelligence/import", csv); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if len(m.Snapshot().Models) != 1 {
		t.Fatal("uploaded score was not matched")
	}
	records := send(http.MethodGet, "/gateway/v1/intelligence/records?q=local/model-a", "")
	var page struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal(records.Body.Bytes(), &page); err != nil || len(page.Records) != 1 {
		t.Fatalf("records: %s %v", records.Body.String(), err)
	}
	record := page.Records[0]
	record["observation"].(map[string]any)["normalized_score"] = .99
	body, _ := json.Marshal(record)
	if w := send(http.MethodPut, "/gateway/v1/intelligence/observations/edit", string(body)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := send(http.MethodPut, "/gateway/v1/intelligence/observations/edit", string(body)); w.Code != 409 {
		t.Fatalf("stale edit: %d %s", w.Code, w.Body.String())
	}
	before := m.Snapshot().Sources[intelligence.CustomSource].Observations[0].Score
	if w := send(http.MethodPost, "/gateway/v1/intelligence/import", strings.Replace(csv, ",0.85,", ",10,", 1)); w.Code != 400 {
		t.Fatal("invalid CSV accepted")
	}
	if m.Snapshot().Sources[intelligence.CustomSource].Observations[0].Score != before {
		t.Fatal("invalid CSV mutated stored data")
	}
}
