package provider

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMixedDecisionCatalogAndRewrittenTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-key" {
			t.Error("provider credential missing")
		}
		switch r.URL.Path {
		case "/api/v1/models":
			calls++
			if r.URL.Query().Get("output_modalities") == "decisions" {
				io.WriteString(w, `{"data":[{"id":"typesafe/jev-1.13","architecture":{"output_modalities":["decisions"]},"context_length":64000,"pricing":{"prompt":"0.000000042","completion":"0"}},{"id":"other/decider","architecture":{"output_modalities":["decisions"]}}]}`)
			} else {
				io.WriteString(w, `{"data":[{"id":"typesafe/jev-router","architecture":{"output_modalities":["text"]}},{"id":"chat"}]}`)
			}
		case "/api/v1/systemone":
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{ "model": "typesafe/jev-1.13", "state": {"ticket":"hi"}, "questions":{}, "unknown": [1,2] }` {
				t.Errorf("body changed: %s", body)
			}
			io.WriteString(w, `{"answers":{},"extension":true}`)
		default:
			t.Errorf("unexpected path %s", r.URL)
		}
	}))
	defer server.Close()
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "renamed", BaseURL: server.URL, APIPath: "/api/v1", Headers: http.Header{"Authorization": {"Bearer provider-key"}}, DiscoverModels: true, DiscoverDecisions: true, ModelRewrites: map[string]string{"judge": "typesafe/jev-1.13"}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil || len(models) != 4 || calls != 2 {
		t.Fatalf("models=%+v calls=%d err=%v", models, calls, err)
	}
	if models[0].Decision || !models[2].Decision || !models[3].Decision || models[2].ID != "renamed/judge" {
		t.Fatalf("wrong decision capabilities: %+v", models)
	}
	if models[2].InputPerMillion == nil || math.Abs(*models[2].InputPerMillion-.042) > 1e-12 || models[2].OutputPerMillion == nil || *models[2].OutputPerMillion != 0 {
		t.Fatalf("decision pricing lost: %+v", models[2])
	}
	response, err := p.ForwardDecision(context.Background(), []byte(`{ "model": "judge", "state": {"ticket":"hi"}, "questions":{}, "unknown": [1,2] }`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestDecisionDetectionRequiresCatalogEvidence(t *testing.T) {
	for _, tc := range []struct {
		entry    string
		decision bool
	}{
		{`{"id":"typesafe/jev-router","architecture":{"output_modalities":["text"]}}`, false},
		{`{"id":"jev-latest"}`, false},
		{`{"id":"unrelated","architecture":{"output_modalities":["decisions"]}}`, true},
		{`{"id":"laya/english","endpoints":{"systemone":"supported"}}`, true},
		{`{"id":"custom","endpoints":{"systemone":false}}`, false},
	} {
		var m Model
		applyCatalogMetadata([]byte(tc.entry), &m)
		if m.Decision != tc.decision {
			t.Errorf("%s: decision=%v", tc.entry, m.Decision)
		}
	}
}

func TestDecisionDiscoveryDoesNotIgnoreFailures(t *testing.T) {
	for _, body := range []string{`{"data":null}`, `{"data":[{"id":"bad","architecture":{"output_modalities":null}}]} trailing`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery == "" {
				io.WriteString(w, `{"data":[{"id":"chat"}]}`)
			} else {
				io.WriteString(w, body)
			}
		}))
		p, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{ID: "p", BaseURL: server.URL, DiscoverModels: true, DiscoverDecisions: true})
		if _, err := p.Models(context.Background()); err == nil || !strings.Contains(err.Error(), "decision") {
			t.Errorf("malformed supplementary catalog accepted: %v", err)
		}
		server.Close()
	}
}
