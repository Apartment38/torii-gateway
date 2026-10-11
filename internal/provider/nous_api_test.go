package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNousAPIKeyWithoutOAuthSaver(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "ToriiGateway" {
			t.Error("Nous requires an application user agent")
		}
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("API key not sent")
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"hermes","context_length":32768,"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
			return
		}
		w.WriteHeader(401)
	}))
	defer server.Close()
	p, err := NewNous("nous", server.URL+"/v1", "", "", nil, true, func(string) (NousCredential, error) {
		return NousCredential{AccessToken: "fixture-key", Endpoint: server.URL + "/v1"}, nil
	}, nil, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(context.Background())
	if err != nil || len(models) != 1 || *models[0].InputPerMillion != 1 || *models[0].OutputPerMillion != 2 {
		t.Fatalf("catalog: %+v %v", models, err)
	}
	response, err := p.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"hermes","messages":[]}`), "hermes")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("API key failure must be returned without OAuth refresh")
	}
}
func TestNousRejectsEmptyAPIKey(t *testing.T) {
	p, _ := NewNous("nous", DefaultNousInferenceURL, "", "", nil, true, func(string) (NousCredential, error) { return NousCredential{Endpoint: DefaultNousInferenceURL}, nil }, nil, nil)
	_, err := p.Models(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatal(err)
	}
}
