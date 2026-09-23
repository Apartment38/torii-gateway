package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGatewayProcessServesAuthenticatedModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"llama3","object":"model"}]}`)
	}))
	defer upstream.Close()

	address := unusedAddress(t)
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	configuration := fmt.Sprintf(`server:
  listen: %s
  authentication:
    api_key:
      env: TEST_GATEWAY_API_KEY
state:
  directory: %s
providers:
  - id: local
    type: ollama
    base_url: %s
`, address, directory, upstream.URL)
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(directory, "gateway")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build gateway: %v\n%s", err, output)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(ctx, binary, "serve", "--config", configPath)
	command.Env = append(os.Environ(), "TEST_GATEWAY_API_KEY=process-secret")
	output := new(strings.Builder)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = command.Wait()
	}()

	baseURL := "http://" + address
	waitForHealthy(t, baseURL+"/healthz", output)
	unauthorized, err := http.Get(baseURL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /v1/models status = %d, want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer process-secret")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"id":"local/llama3"`) {
		t.Fatalf("GET /v1/models = %d %s; process output: %s", response.StatusCode, body, output)
	}
}

func TestServeHTTPWaitsForActiveRequestDuringShutdown(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusNoContent)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveHTTP(ctx, server, listener)
	}()

	responseDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
		responseDone <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach handler")
	}

	cancel()
	select {
	case err := <-serveDone:
		t.Fatalf("serveHTTP returned before active request completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRequest)
	if err := <-responseDone; err != nil {
		t.Fatalf("active request failed during shutdown: %v", err)
	}
	if err := <-serveDone; err != nil {
		t.Fatalf("serveHTTP returned an error: %v", err)
	}
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitForHealthy(t *testing.T, url string, processOutput *strings.Builder) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("gateway did not become healthy; process output: %s", processOutput)
}
