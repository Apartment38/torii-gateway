package main

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func TestTrackingCredentialsAreEncryptedAndIsolatedFromInference(t *testing.T) {
	db := runtimeStateDB(t)
	var billing, inference int
	expectedBilling := "management-synthetic"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			inference++
			if r.Header.Get("Authorization") != "Bearer inference-synthetic" {
				t.Error("inference received wrong credential")
			}
			io.WriteString(w, `{}`)
		case "/v1/billing/teams/team/prepaid/balance":
			billing++
			if r.Header.Get("Authorization") != "Bearer "+expectedBilling {
				t.Error("billing received wrong credential")
			}
			io.WriteString(w, `{"total":{"val":"-1250"}}`)
		case "/v1/billing/teams/team/usage":
			w.WriteHeader(403)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	no := false
	cfg := config.Config{Providers: []config.ProviderConfig{{ID: "xai", Type: "xai", BaseURL: server.URL, BearerToken: &config.SecretRef{Value: "inference-synthetic"}, ManagementToken: &config.SecretRef{Value: "management-synthetic"}, Models: []string{"m"}, DiscoverModels: &no, Tracking: &config.TrackingConfig{BaseURL: server.URL, TeamID: "team"}}}}
	if err := config.StoreSubmittedCredentials(&cfg, db); err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].ManagementToken.Value != "" || cfg.Providers[0].BearerToken.Value != "" {
		t.Fatal("plaintext credential retained in config")
	}
	p, err := buildProviderWithState(cfg.Providers[0], db)
	if err != nil {
		t.Fatal(err)
	}
	s, err := quota.NewCache(time.Minute, p.(quota.Fetcher)).Get(context.Background(), "xai/m")
	if err != nil || len(s.Balances) != 1 || *s.Balances[0].Remaining != 12.5 || !s.Partial {
		t.Fatalf("status %+v %v", s, err)
	}
	resp, err := p.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"m"}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if billing != 1 || inference != 1 {
		t.Fatalf("request counts %d %d", billing, inference)
	}
	// References are resolved on every outbound read, including replacement.
	ref := cfg.Providers[0].ManagementToken.Credential
	expectedBilling = "rotated-management-synthetic"
	if err := db.Put(ref, state.Owner, "xai", "management_token", []byte(expectedBilling)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.(quota.Fetcher).Quota(context.Background(), "xai/m"); err != nil {
		t.Fatal(err)
	}
}

func TestSeparateTrackingTokenAndOriginValidation(t *testing.T) {
	no := false
	cfg := config.ProviderConfig{ID: "ds", Type: "deepseek", BaseURL: "https://api.deepseek.com", BearerToken: &config.SecretRef{Value: "synthetic-inference"}, Models: []string{"m"}, DiscoverModels: &no, Tracking: &config.TrackingConfig{UsageURL: "https://other.example/balance"}}
	if _, err := buildProviderWithState(cfg); err == nil || !strings.Contains(err.Error(), "separate tracking token") {
		t.Fatalf("cross-origin inference credential accepted: %v", err)
	}
	cfg.Tracking.Token = &config.SecretRef{Value: "synthetic-billing"}
	if _, err := buildProviderWithState(cfg); err != nil {
		t.Fatal(err)
	}
	full := config.Config{Providers: []config.ProviderConfig{cfg}}
	seen := false
	if err := full.VisitSecrets(func(path, service, purpose string, r *config.SecretRef) error {
		if path == "providers/0/tracking/token" && purpose == "tracking_token" {
			seen = true
		}
		return nil
	}); err != nil || !seen {
		t.Fatalf("tracking secret not inventoried %v", err)
	}
}

func TestNewProviderTypesValidateAndBuild(t *testing.T) {
	for _, kind := range []string{"opencode-go", "opencode-zen", "standard-compute", "zai", "kimi-coding", "minimax", "deepseek"} {
		t.Run(kind, func(t *testing.T) {
			raw := "server:\n  listen: 127.0.0.1:8080\nstate:\n  database_path: /data/torii.db\nproviders:\n  - id: provider\n    type: " + kind + "\n    base_url: https://example.com\n    bearer_token: {value: synthetic}\n    models: [m]\n    discover_models: false\n"
			cfg, err := config.Decode(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			p, err := buildProviderWithState(cfg.Providers[0])
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := p.(quota.Fetcher); !ok {
				t.Fatal("provider lacks quota fetcher")
			}
		})
	}
}

func TestTrackingUsesConfiguredTLSRoots(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("incorrect tracking request")
		}
		io.WriteString(w, `{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"9.25"}]}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	no := false
	cfg := config.ProviderConfig{ID: "ds", Type: "deepseek", BaseURL: server.URL, BearerToken: &config.SecretRef{Value: "synthetic"}, Models: []string{"m"}, DiscoverModels: &no}
	cfg.TLS = &config.TLSConfig{CAFile: path}
	p, err := buildProviderWithState(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s, err := p.(quota.Fetcher).Quota(context.Background(), "ds/m")
	if err != nil || len(s.Balances) != 1 || *s.Balances[0].Remaining != 9.25 {
		t.Fatalf("TLS tracking: %+v, %v", s, err)
	}
}
