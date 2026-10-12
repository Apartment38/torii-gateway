package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

func TestDecisionPassthroughPreservesPayloadAndProviderIsolation(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(map[bool]string{true: "Laya", false: "mixed"}[native], func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer upstream-key" {
					t.Error("wrong upstream credential")
				}
				switch r.URL.Path {
				case "/prefix/v1/models":
					io.WriteString(w, `{"data":[{"id":"decision","architecture":{"output_modalities":["decisions"]}},{"id":"chat"}]}`)
				case "/prefix/v1/systemone":
					calls++
					body, _ := io.ReadAll(r.Body)
					if string(body) != `{ "model": "decision", "state": {"text":"hello"}, "questions":{}, "unknown":{"preserve":true} }` {
						t.Errorf("changed payload %s", body)
					}
					w.Header().Set("X-Upstream", "decision")
					w.WriteHeader(422)
					io.WriteString(w, `{"custom_error":"unaltered"}`)
				default:
					t.Errorf("unexpected upstream endpoint %s", r.URL)
				}
			}))
			defer upstream.Close()
			cfg := provider.OpenAICompatibleConfig{ID: "p", BaseURL: upstream.URL + "/prefix", Headers: http.Header{"Authorization": {"Bearer upstream-key"}}, DiscoverModels: !native, StaticModels: []string{"decision"}}
			var p provider.OpenAIProvider
			var err error
			if native {
				p, err = provider.NewJevProvider(cfg, true)
			} else {
				p, err = provider.NewOpenAICompatibleProvider(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			h, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{p})
			if err = h.RefreshCatalog(context.Background()); err != nil {
				t.Fatal(err)
			}
			listed := httptest.NewRecorder()
			h.ServeHTTP(listed, httptest.NewRequest("GET", "/v1/models", nil))
			if !strings.Contains(listed.Body.String(), `"models":[{"name":"p/decision"`) || !strings.Contains(listed.Body.String(), `"decision":true`) {
				t.Fatalf("decision discovery missing: %s", listed.Body.String())
			}
			for _, path := range []string{"/v1/systemone", "/v1/systemone/"} {
				req := httptest.NewRequest("POST", path, strings.NewReader(`{ "model": "p/decision", "state": {"text":"hello"}, "questions":{}, "unknown":{"preserve":true} }`))
				req.Header.Set("Authorization", "Bearer client-key")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				if w.Code != 422 || w.Header().Get("X-Upstream") != "decision" || w.Body.String() != `{"custom_error":"unaltered"}` {
					t.Fatalf("response changed %d %s", w.Code, w.Body.String())
				}
			}
			for _, model := range []string{"smart", "p/chat", "p/missing"} {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/systemone", strings.NewReader(`{"model":"`+model+`","state":"hello","questions":{}}`)))
				if w.Code != 400 {
					t.Errorf("invalid decision model %s accepted: %d", model, w.Code)
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"p/decision","messages":[]}`)))
			if w.Code != 400 || calls != 2 {
				t.Errorf("decision model reached chat or unexpected dispatch: status=%d calls=%d", w.Code, calls)
			}
			h.SetModelControls(map[string]bool{"p/decision": false}, nil)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/systemone", strings.NewReader(`{"model":"p/decision","state":"hello","questions":{}}`)))
			if w.Code == 422 || calls != 2 {
				t.Error("disabled decision model forwarded")
			}
		})
	}
}
