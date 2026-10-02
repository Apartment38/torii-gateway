package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func benchmarkConfig(t *testing.T, url string) *config.Config {
	t.Helper()
	db, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.AdoptGatewayAPIKey("test-gateway-key"); err != nil {
		t.Fatal(err)
	}
	discover := false
	return &config.Config{Credentials: db, Server: config.ServerConfig{Listen: "127.0.0.1:8080", Authentication: &config.ServerAuthenticationConfig{APIKey: config.SecretRef{Value: "test-gateway-key"}}}, State: config.StateConfig{Directory: t.TempDir()}, Providers: []config.ProviderConfig{{ID: "local", Type: "openai_compatible", BaseURL: "https://example.test", DiscoverModels: &discover, Models: []string{"model"}}}, Intelligence: config.IntelligenceConfig{Enabled: true, Sources: []config.BenchmarkSourceConfig{{SourceSettings: intelligence.SourceSettings{ID: "bench", Type: "benchlm", Enabled: true, URL: url}}}}}
}

func TestBenchmarkManagementAuthenticationCLIAndSourceFailureIsolation(t *testing.T) {
	t.Setenv("TORII_DB_KEY", strings.Repeat("24", 32))
	t.Setenv("TORII_DB_PATH", filepath.Join(t.TempDir(), "torii.db"))
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"models":[{"model":"model","overallScore":80,"evidenceStatus":"supported"}]}`)
	}))
	defer upstream.Close()
	cfg := benchmarkConfig(t, upstream.URL)
	cfg.Intelligence.Sources = append(cfg.Intelligence.Sources, config.BenchmarkSourceConfig{SourceSettings: intelligence.SourceSettings{ID: "missing-key", Type: "artificial_analysis", Enabled: true}, APIKey: &config.SecretRef{Env: "TORII_TEST_NONEXISTENT_BENCHMARK_KEY"}})
	path := filepath.Join(cfg.State.Directory, "config.yaml")
	if err := persistConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	runtime := &handlerRuntime{configPath: path}
	defer runtime.closeUsage()
	handler, err := buildHandlerWithRuntime(cfg, path, runtime)
	if err != nil {
		t.Fatalf("one missing source key prevented startup: %v", err)
	}
	runtime.swap(handler)
	defer runtime.stopIntelligence()
	if calls.Load() != 0 {
		t.Fatal("handler construction fetched benchmarks")
	}
	w := httptest.NewRecorder()
	runtime.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gateway/v1/intelligence/sources", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal("management inspection bypassed configured authentication")
	}
	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/gateway/v1/intelligence/refresh", nil)
	r.Header.Set("Authorization", "Bearer test-gateway-key")
	runtime.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"partial_failure":true`) || calls.Load() != 1 {
		t.Fatalf("partial refresh=%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "credentials or initialization") || strings.Contains(w.Body.String(), "test-gateway-key") {
		t.Fatal("source error status unsafe or uninformative")
	}
	server := httptest.NewServer(runtime)
	defer server.Close()
	// CLI imports its authentication key through the database; the running
	// handler's missing source still demonstrates independent refresh failure.
	cfg.Intelligence.Sources = cfg.Intelligence.Sources[:1]
	if err := persistConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runWithIO(context.Background(), []string{"benchmarks", "refresh", "--config", path, "--url", server.URL}, &output); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || !strings.Contains(output.String(), `"partial_failure":true`) {
		t.Fatal("CLI did not refresh running gateway")
	}
}

func TestBenchmarkWorkerOnlyStartsForActiveHandlerAndStopsOnReload(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"models":[{"model":"model","overallScore":80,"evidenceStatus":"supported"}]}`)
	}))
	defer upstream.Close()
	cfg := benchmarkConfig(t, upstream.URL)
	cfg.Intelligence.RefreshInterval = "10ms"
	path := filepath.Join(cfg.State.Directory, "config.yaml")
	if err := persistConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &handlerRuntime{configPath: path, background: ctx}
	defer runtime.closeUsage()
	handler, err := buildHandlerWithRuntime(cfg, path, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("discardable config build started a worker")
	}
	runtime.swap(handler)
	defer runtime.stopIntelligence()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for calls.Load() < 2 {
		select {
		case <-deadline.C:
			t.Fatal("active handler did not refresh")
		case <-tick.C:
		}
	}
	cfg.Intelligence.Enabled = false
	replacement, err := buildHandlerWithRuntime(cfg, path, runtime)
	if err != nil {
		t.Fatal(err)
	}
	runtime.swap(replacement)
	before := calls.Load()
	time.Sleep(25 * time.Millisecond)
	if calls.Load() != before {
		t.Fatal("discarded handler still refreshes sources")
	}
}
