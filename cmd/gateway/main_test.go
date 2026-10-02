package main

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/Apartment38/torii-gateway/internal/config"
)

func TestInitializeConfigPathBootstrapsConsoleConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := initializeConfigPath(path, "", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("bootstrap config is invalid: %v", err)
	}
	if cfg.Server.Listen != "0.0.0.0:8080" || len(cfg.Providers) != 1 || cfg.Providers[0].ID != "setup" {
		t.Fatalf("unexpected bootstrap config: %+v", cfg)
	}
}

func TestInitializeConfigPathDoesNotCreateWithoutBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := initializeConfigPath(path, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config exists or stat failed: %v", err)
	}
}

func TestSmartRoutingUsesConfiguredJevCompatibleProvider(t *testing.T) {
	var decisions, chats int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/systemone":
			decisions++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "english" || body["state"] != "hello" || r.Header.Get("Authorization") != "Bearer laya-key" {
				t.Errorf("wrong decision request: %+v", body)
			}
			_, _ = io.WriteString(w, `{"answers":{"task_type":{"type":"choice","choice":"general","confidence":0.9},"complexity":{"type":"choice","choice":"low","confidence":0.9}}}`)
		case "/v1/chat/completions":
			chats++
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello back"}}]}`)
		default:
			t.Errorf("unexpected upstream request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	discover := false
	cfg := &config.Config{Server: config.ServerConfig{Listen: "127.0.0.1:8080"}, State: config.StateConfig{Directory: t.TempDir()}, Providers: []config.ProviderConfig{
		{ID: "chat", Type: "openai_compatible", BaseURL: server.URL, DiscoverModels: &discover, Models: []string{"chat-model"}},
		{ID: "laya", Type: "jev_compatible", BaseURL: server.URL, DiscoverModels: &discover, Models: []string{"english"}, BearerToken: &config.SecretRef{Value: "laya-key"}},
	}, Router: config.RouterConfig{Classifier: config.ClassifierConfig{Type: "jev", Transport: "provider", Provider: "laya", Model: "english", OnFailure: "fail"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := buildHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"smart","messages":[{"role":"user","content":"hello"}]}`)))
	if w.Code != http.StatusOK || decisions != 1 || chats != 1 {
		t.Fatalf("smart did not use decision provider: HTTP %d %s, decisions=%d chats=%d", w.Code, w.Body.String(), decisions, chats)
	}
}

func TestProviderConnectionReportsClassifierCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("discovery path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"models":[{"name":"english"}]}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := initializeConfigPath(path, "", true); err != nil {
		t.Fatal(err)
	}
	r := &handlerRuntime{configPath: path}
	w := httptest.NewRecorder()
	request := fmt.Sprintf(`{"provider":{"id":"laya","type":"jev_compatible","base_url":%q,"api_path":"/v1"}}`, server.URL)
	r.testProvider(w, httptest.NewRequest(http.MethodPost, "/gateway/v1/providers/test", strings.NewReader(request)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"classifier_only":true`) || !strings.Contains(w.Body.String(), `"id":"laya/english"`) {
		t.Fatalf("classifier connection = %d %s", w.Code, w.Body.String())
	}
}

func TestConsoleDirectProviderKeyPersistsReloadsAndAuthenticates(t *testing.T) {
	t.Setenv("TORII_API_KEY", "")
	t.Setenv("TORII_DB_KEY", strings.Repeat("17", 32))
	t.Setenv("TORII_DB_PATH", filepath.Join(t.TempDir(), "torii.db"))
	const key = "test-console-provider-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/models" || req.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("provider discovery used incorrect path or authorization")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"name":"english"}]}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := initializeConfigPath(path, "", true); err != nil {
		t.Fatal(err)
	}
	r := &handlerRuntime{configPath: path}
	initial, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openCredentials(initial, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r.credentials = db
	updated := fmt.Sprintf(`{"server":{"listen":"127.0.0.1:8080"},"state":{"directory":%q},"providers":[{"id":"laya","type":"jev_compatible","base_url":%q,"api_path":"/v1","bearer_token":{"value":%q}}],"router":{"classifier":{"type":"disabled"}}}`, filepath.Dir(path), server.URL, key)
	put := httptest.NewRecorder()
	r.putConfig(put, httptest.NewRequest(http.MethodPut, "/gateway/v1/config", strings.NewReader(updated)))
	if put.Code != http.StatusOK || !strings.Contains(put.Body.String(), `"reloaded":true`) {
		t.Fatalf("save direct provider key = HTTP %d", put.Code)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := cfg.Providers[0].BearerToken
	if secret == nil || secret.Value != "" || secret.Credential == "" || secret.Env != "" || secret.File != "" {
		t.Fatal("saved provider key was not replaced by an encrypted credential reference")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("configuration permissions = %o, want 600", info.Mode().Perm())
	}
	get := httptest.NewRecorder()
	r.getConfig(get, httptest.NewRequest(http.MethodGet, "/gateway/v1/config", nil))
	var document struct {
		Providers []struct {
			BearerToken struct {
				Value string `json:"value"`
			} `json:"bearer_token"`
		} `json:"providers"`
	}
	if get.Code != http.StatusOK || json.Unmarshal(get.Body.Bytes(), &document) != nil || len(document.Providers) != 1 || document.Providers[0].BearerToken.Value != "" || strings.Contains(get.Body.String(), key) {
		t.Fatal("console config reload disclosed the saved provider key")
	}
	connection := httptest.NewRecorder()
	request := fmt.Sprintf(`{"provider":{"id":"laya","type":"jev_compatible","base_url":%q,"api_path":"/v1","bearer_token":{"value":%q}}}`, server.URL, key)
	r.testProvider(connection, httptest.NewRequest(http.MethodPost, "/gateway/v1/providers/test", strings.NewReader(request)))
	if connection.Code != http.StatusOK || !strings.Contains(connection.Body.String(), `"id":"laya/english"`) {
		t.Fatalf("connection test with direct key = HTTP %d", connection.Code)
	}
	if strings.Contains(connection.Body.String(), key) {
		t.Fatal("connection status disclosed the provider API key")
	}
}

func TestCodexConnectionTestDoesNotTreatFallbackAsSuccess(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	if err := initializeConfigPath(path, "", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.State.Directory = directory
	if err := persistConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	r := &handlerRuntime{configPath: path}
	w := httptest.NewRecorder()
	r.testProvider(w, httptest.NewRequest(http.MethodPost, "/gateway/v1/providers/test", strings.NewReader(`{"provider":{"id":"openai-codex","type":"openai-codex","base_url":"https://chatgpt.com/backend-api/codex"}}`)))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "log in") {
		t.Fatalf("missing credential falsely reported success: %d %s", w.Code, w.Body.String())
	}
}

func TestLoginRefreshUsesOnlyActiveHandler(t *testing.T) {
	var active, discarded int
	r := &handlerRuntime{}
	r.swap(&catalogHandler{refresh: func(context.Context) error { active++; return nil }})
	// Building a replacement is not enough; only successfully saved handlers
	// are swapped into the runtime.
	replacement := &catalogHandler{refresh: func(context.Context) error { discarded++; return nil }}
	if err := r.refreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if active != 1 || discarded != 0 {
		t.Fatalf("refreshed an inactive handler: active=%d discarded=%d", active, discarded)
	}
	r.swap(replacement)
	if err := r.refreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if active != 1 || discarded != 1 {
		t.Fatalf("refresh did not follow saved handler: active=%d discarded=%d", active, discarded)
	}
}

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
	command.Env = append(os.Environ(), "TEST_GATEWAY_API_KEY=process-secret", "TORII_DB_KEY="+strings.Repeat("18", 32), "TORII_DB_PATH="+filepath.Join(directory, "torii.db"))
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

func TestBuildHandlerRejectsAPIKeyContainingWhitespace(t *testing.T) {
	t.Setenv("TEST_GATEWAY_API_KEY", " unusable-key")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	configuration := `server:
  listen: 127.0.0.1:0
  authentication:
    api_key:
      env: TEST_GATEWAY_API_KEY
state:
  directory: /tmp/state
providers:
  - id: local
    type: ollama
    base_url: http://127.0.0.1:11434
`
	if err := os.WriteFile(cfgPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildHandler(cfg); err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("buildHandler() error = %v, want whitespace validation error", err)
	}
}

func TestRuntimeConfigEndpointPersistsAndReloads(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	initial := `server:
  listen: 127.0.0.1:8080
  authentication:
    api_key: {value: test-management-key}
state:
  directory: STATE_DIR
providers:
  - id: local
    type: openai_compatible
    base_url: https://example.test
    discover_models: false
    models: [model]
`
	initial = strings.Replace(initial, "STATE_DIR", filepath.Dir(configPath), 1)
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &handlerRuntime{configPath: configPath}
	handler, err := buildHandlerWithRuntime(cfg, configPath, runtime)
	if err != nil {
		t.Fatal(err)
	}
	runtime.handler = handler

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/gateway/v1/config", nil)
	getReq.Header.Set("Authorization", "Bearer test-management-key")
	handler.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"listen":"127.0.0.1:8080"`) {
		t.Fatalf("GET config = %d %s", get.Code, get.Body.String())
	}

	updated := []byte(`{"server":{"listen":"127.0.0.1:9090","authentication":{"api_key":{"value":"test-management-key"}}},"state":{"directory":"` + filepath.Dir(configPath) + `"},"providers":[{"id":"local","type":"openai_compatible","base_url":"https://example.test","discover_models":false,"models":["model"]}]}`)
	put := httptest.NewRecorder()
	putReq := httptest.NewRequest(http.MethodPut, "/gateway/v1/config", bytes.NewReader(updated))
	putReq.Header.Set("Authorization", "Bearer test-management-key")
	handler.ServeHTTP(put, putReq)
	if put.Code != http.StatusOK || !strings.Contains(put.Body.String(), `"reloaded":true`) {
		t.Fatalf("PUT config = %d %s", put.Code, put.Body.String())
	}
	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Server.Listen != "127.0.0.1:9090" {
		t.Fatalf("reloaded listen = %q", reloaded.Server.Listen)
	}
}

func TestRemoteCodexLoginSessionImportsOnce(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	configuration := fmt.Sprintf(`server:
  listen: 127.0.0.1:8080
  authentication:
    api_key: {value: test-management-key}
state:
  directory: %s
providers:
  - id: openai-codex
    type: openai-codex
    base_url: https://chatgpt.com/backend-api/codex
    discover_models: false
    models: [gpt-5.5]
`, directory)
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &handlerRuntime{configPath: configPath}
	handler, err := buildHandlerWithRuntime(cfg, configPath, runtime)
	if err != nil {
		t.Fatal(err)
	}
	runtime.handler = handler

	start := httptest.NewRequest(http.MethodPost, "/gateway/v1/providers/login", strings.NewReader(`{"provider":{"id":"openai-codex","type":"openai-codex","base_url":"https://chatgpt.com/backend-api/codex","discover_models":false,"models":["gpt-5.5"]}}`))
	start.Host = "gateway.example.test:8398"
	start.Header.Set("Content-Type", "application/json")
	start.Header.Set("Authorization", "Bearer test-management-key")
	started := httptest.NewRecorder()
	handler.ServeHTTP(started, start)
	if started.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", started.Code, started.Body.String())
	}
	var login providerLogin
	if err := json.Unmarshal(started.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.HelperURL == "" || login.HelperToken == "" || login.ExpiresAt.IsZero() {
		t.Fatalf("remote login response = %#v", login)
	}

	metadataRequest := httptest.NewRequest(http.MethodGet, "/gateway/v1/providers/login/session", nil)
	metadataRequest.Header.Set("Authorization", "Bearer "+login.HelperToken)
	metadata := httptest.NewRecorder()
	handler.ServeHTTP(metadata, metadataRequest)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"host_id":"urn:uuid:`) {
		t.Fatalf("session metadata = %d %s", metadata.Code, metadata.Body.String())
	}

	importRequest := httptest.NewRequest(http.MethodPost, "/gateway/v1/providers/login/import", strings.NewReader(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_at":"2099-01-01T00:00:00Z","client_id":"oaiapp_test","id_token":"id-token","issuer":"https://auth.openai.com","host_id":"`+extractJSONField(metadata.Body.String(), "host_id")+`","scopes":["openid","chatgpt.tokens.use.direct"]}`))
	importRequest.Header.Set("Authorization", "Bearer "+login.HelperToken)
	importRequest.Header.Set("Content-Type", "application/json")
	imported := httptest.NewRecorder()
	handler.ServeHTTP(imported, importRequest)
	if imported.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", imported.Code, imported.Body.String())
	}

	replayed := httptest.NewRecorder()
	handler.ServeHTTP(replayed, importRequest)
	if replayed.Code != http.StatusUnauthorized {
		t.Fatalf("replayed import status = %d, body = %s", replayed.Code, replayed.Body.String())
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/gateway/v1/providers/login?provider=openai-codex", nil)
	statusRequest.Header.Set("Authorization", "Bearer test-management-key")
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, statusRequest)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"success"`) || strings.Contains(status.Body.String(), login.HelperToken) {
		t.Fatalf("login status = %d %s", status.Code, status.Body.String())
	}
}

func extractJSONField(data, field string) string {
	var values map[string]string
	if json.Unmarshal([]byte(data), &values) != nil {
		return ""
	}
	return values[field]
}

func TestBuildCompatibleProviderResolvesAndIsolatesCredentials(t *testing.T) {
	t.Setenv("UPSTREAM_TOKEN", "upstream-secret")
	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		_, _ = io.WriteString(w, `{"data":[{"id":"model"}]}`)
	}))
	defer upstream.Close()
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("redirect target received credentials: %#v", r.Header)
	}))
	defer redirectTarget.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", redirectTarget.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirecting.Close()

	secret := config.SecretRef{Env: "UPSTREAM_TOKEN"}
	p, err := buildProvider(config.ProviderConfig{ID: "generic", Type: "openai_compatible", BaseURL: upstream.URL, BearerToken: &secret, Headers: map[string]config.SecretRef{"X-Provider-Key": secret}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	headers := <-received
	if headers.Get("Authorization") != "Bearer upstream-secret" || headers.Get("X-Provider-Key") != "upstream-secret" {
		t.Fatalf("headers = %#v", headers)
	}

	p, err = buildProvider(config.ProviderConfig{ID: "redirect", Type: "openai_compatible", BaseURL: redirecting.URL, BearerToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Models(context.Background()); err == nil || !strings.Contains(err.Error(), "307") {
		t.Fatalf("redirect discovery error = %v", err)
	}
}

func TestAuthCLIImportStatusAndLogoutNeverPrintTokens(t *testing.T) {
	t.Setenv("TORII_DB_KEY", strings.Repeat("19", 32))
	t.Setenv("TORII_DB_PATH", filepath.Join(t.TempDir(), "torii.db"))
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	configuration := fmt.Sprintf("server:\n  listen: 127.0.0.1:0\nstate:\n  directory: %s\nproviders:\n  - id: openai\n    type: openai_compatible\n    base_url: https://api.example.test\n    discover_models: false\n    models: [model]\n", directory)
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLI_ACCESS_TOKEN", "access-secret-never-print")
	t.Setenv("CLI_REFRESH_TOKEN", "refresh-secret-never-print")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openCredentials(cfg, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateAdmin("cli-admin", "cli-admin-password"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var output strings.Builder
	passwordStdin(t, "cli-admin-password")
	if err := runWithIO(context.Background(), []string{"auth", "login", "openai", "--config", configPath, "--access-token-env", "CLI_ACCESS_TOKEN", "--refresh-token-env", "CLI_REFRESH_TOKEN", "--expires-at", "2099-01-01T00:00:00Z"}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "secret") || !strings.Contains(output.String(), "logged in") {
		t.Fatalf("login output=%q", output.String())
	}
	output.Reset()
	passwordStdin(t, "cli-admin-password")
	if err := runWithIO(context.Background(), []string{"auth", "status", "openai", "--config", configPath}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "provider openai: logged in\n" {
		t.Fatalf("status output=%q", output.String())
	}
	output.Reset()
	passwordStdin(t, "cli-admin-password")
	if err := runWithIO(context.Background(), []string{"auth", "logout", "openai", "--config", configPath}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	passwordStdin(t, "cli-admin-password")
	if err := runWithIO(context.Background(), []string{"auth", "status", "openai", "--config", configPath}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "provider openai: logged out\n" {
		t.Fatalf("status output=%q", output.String())
	}
}

func TestAuthCLIValidatesConfigurationAndProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("unknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runWithIO(context.Background(), []string{"auth", "status", "missing", "--config", path}, io.Discard); err == nil {
		t.Fatal("invalid configuration accepted")
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
