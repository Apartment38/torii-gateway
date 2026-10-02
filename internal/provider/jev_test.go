package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJevProviderDiscoversModelCatalogFormats(t *testing.T) {
	for _, catalog := range []string{`{"models":[{"name":"jev-latest"},{"name":"jev-preview"}]}`, `{"data":[{"id":"jev-latest"},{"id":"jev-preview"}]}`, `{"models":["jev-latest","jev-preview","jev-latest","unavailable"]}`} {
		t.Run(catalog[:16], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer scoped-key" {
					t.Errorf("discovery = %s, auth=%q", r.URL, r.Header.Get("Authorization"))
				}
				_, _ = io.WriteString(w, catalog)
			}))
			defer server.Close()
			p, err := NewJevProvider(OpenAICompatibleConfig{ID: "jev", BaseURL: server.URL + "/v1", Headers: http.Header{"Authorization": {"Bearer scoped-key"}}, DiscoverModels: true, StaticModels: []string{"not-live"}}, false)
			if err != nil {
				t.Fatal(err)
			}
			models, err := p.Models(context.Background())
			if err != nil || len(models) != 2 || models[0].ID != "jev/jev-latest" || models[1].ID != "jev/jev-preview" {
				t.Fatalf("catalog = %+v, %v", models, err)
			}
		})
	}
}

func TestJevCompatibleDiscoversCurrentLayaHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/v1/models":
			http.NotFound(w, r)
		case "/prefix/health":
			_, _ = io.WriteString(w, `{"status":"ok","loaded":["english","multilingual","typed-decisions"]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p, err := NewJevProvider(OpenAICompatibleConfig{ID: "laya", BaseURL: server.URL + "/prefix", DiscoverModels: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil || len(models) != 3 || models[0].ID != "laya/english" {
		t.Fatalf("Laya health discovery = %+v, %v", models, err)
	}
}

func TestJevDiscoveryDoesNotHideAuthenticationOrMalformedResponses(t *testing.T) {
	for _, testcase := range []struct {
		status int
		body   string
	}{
		{http.StatusUnauthorized, "private error"}, {http.StatusForbidden, "private error"},
		{http.StatusOK, `{"models":[]}`}, {http.StatusOK, `{}`}, {http.StatusOK, `{"models":["unavailable"]}`},
		{http.StatusOK, strings.Repeat("x", (1<<20)+1)},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				t.Error("authentication/catalog failure incorrectly triggered health fallback")
			}
			w.WriteHeader(testcase.status)
			_, _ = io.WriteString(w, testcase.body)
		}))
		p, err := NewJevProvider(OpenAICompatibleConfig{ID: "laya", BaseURL: server.URL, DiscoverModels: true, StaticModels: []string{"fake-success"}}, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Models(context.Background()); err == nil || strings.Contains(err.Error(), "private error") {
			t.Fatalf("discovery error = %v", err)
		}
		server.Close()
	}
}

func TestJevProviderUsesTypedDecisionTransportOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/systemone" || r.Method != http.MethodPost || r.Header.Get("X-Tenant") != "tenant" {
			t.Errorf("decision request = %s %s", r.Method, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"english"`) || !strings.Contains(string(body), `"questions"`) {
			t.Errorf("decision body = %s", body)
		}
		_, _ = io.WriteString(w, `{"model":"english","answers":{}}`)
	}))
	defer server.Close()
	p, err := NewJevProvider(OpenAICompatibleConfig{ID: "laya", BaseURL: server.URL, APIPath: "/custom", Headers: http.Header{"X-Tenant": {"tenant"}}, StaticModels: []string{"english"}, ModelRewrites: map[string]string{"friendly": "english"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil || models[0].ID != "laya/friendly" {
		t.Fatalf("static classifier alias = %+v, %v", models, err)
	}
	response, err := p.ForwardDecision(context.Background(), []byte(`{"model":"friendly","state":"hello","questions":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !p.ClassifierOnly() {
		t.Fatal("provider was not marked classifier-only")
	}
	if _, err := p.Forward(context.Background(), "/v1/chat/completions", nil, "friendly"); err == nil {
		t.Fatal("classifier served an inference request")
	}
}
