// Command gateway is the entry point for the Torii Gateway service.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/activity"
	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/router"
	"github.com/Apartment38/torii-gateway/internal/state"
	"github.com/Apartment38/torii-gateway/internal/usage"
	"github.com/Apartment38/torii-gateway/internal/web"
	"gopkg.in/yaml.v3"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runWithIO(ctx, os.Args[1:], os.Stdout); err != nil {
		log.Printf("gateway: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	return runWithIO(ctx, args, io.Discard)
}

func runWithIO(ctx context.Context, args []string, output io.Writer) error {
	if len(args) > 0 && args[0] == "version" {
		return printVersion(args[1:], output)
	}
	if len(args) > 0 && args[0] == "admin" {
		return runAdmin(ctx, args[1:], output)
	}
	if len(args) > 0 && args[0] == "credentials" {
		return runCredentials(ctx, args[1:], output)
	}
	if len(args) > 0 && args[0] == "benchmarks" {
		return runBenchmarkCommand(ctx, args[1:], output)
	}
	if len(args) > 0 && args[0] == "healthcheck" {
		flags := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
		urlFlag := flags.String("url", "http://127.0.0.1:8080/healthz", "health URL")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: gateway healthcheck [--url <url>]")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, *urlFlag, nil)
		if err != nil {
			return err
		}
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("health check returned %s", response.Status)
		}
		return nil
	}
	if len(args) > 0 && args[0] == "config" {
		if len(args) > 1 && args[1] == "import" {
			return runConfigImport(args[2:], output)
		}
		if len(args) < 2 || args[1] != "validate" {
			return errors.New("usage: gateway config validate --config <path>")
		}
		flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
		configPath := flags.String("config", "/config/config.yaml", "path to the YAML configuration")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: gateway config validate --config <path>")
		}
		data, err := os.ReadFile(*configPath)
		if err != nil {
			return err
		}
		if _, err := config.Decode(bytes.NewReader(data)); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "configuration is valid")
		return err
	}
	if len(args) > 0 && args[0] == "models" {
		flags := flag.NewFlagSet("models", flag.ContinueOnError)
		databasePath := flags.String("database", "", "SQLite database path")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: gateway models --database <path>")
		}
		db, err := openSettingsDatabase(*databasePath)
		if err != nil {
			return err
		}
		cfg, err := loadDatabaseSettings(db)
		if err != nil {
			db.Close()
			return err
		}
		defer db.Close()
		if _, err := fmt.Fprintln(output, "smart"); err != nil {
			return err
		}
		for _, providerConfig := range cfg.Providers {
			if providerConfig.Enabled != nil && !*providerConfig.Enabled {
				continue
			}
			upstream, buildErr := buildProviderWithState(providerConfig, db)
			if buildErr != nil {
				return fmt.Errorf("initialize provider %q: %w", providerConfig.ID, buildErr)
			}
			models, modelErr := upstream.Models(ctx)
			if modelErr != nil {
				return fmt.Errorf("discover provider %q: %w", providerConfig.ID, modelErr)
			}
			for _, model := range models {
				if enabled, remembered := cfg.ModelEnabled[model.ID]; remembered && !enabled {
					continue
				}
				if _, err := fmt.Fprintln(output, model.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if len(args) > 0 && args[0] == "auth" {
		return runAuth(args[1:], output)
	}
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: gateway <serve|admin|credentials|auth|models|benchmarks|healthcheck> [options]")
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	databasePath := flags.String("database", "", "SQLite database path (or TORII_DB_PATH)")
	listen := flags.String("listen", os.Getenv("TORII_LISTEN"), "listener override; requires restart")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: gateway serve [--database PATH] [--listen ADDRESS]")
	}
	db, err := openSettingsDatabase(*databasePath)
	if err != nil {
		return err
	}
	cfg, err := loadDatabaseSettings(db)
	if err != nil {
		db.Close()
		return err
	}
	if cfg.Activity.LogFile != "" {
		options := cfg.Activity.Defaults()
		writer, err := activity.NewRotatingWriter(options.LogFile, options.LogMaxBytes, options.LogBackups)
		if err != nil {
			db.Close()
			return errors.New("cannot open activity log file")
		}
		defer writer.Close()
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, writer), nil)))
	}
	if *listen != "" {
		cfg.Server.Listen = *listen
	}
	if err = cfg.Validate(); err != nil {
		db.Close()
		return err
	}
	defer db.Close()
	runtime := &handlerRuntime{configPath: "database", background: ctx, credentials: db}
	// Install cleanup before construction so startup/listen failures also release
	// shared state. Normal shutdown drains HTTP before the final usage flush.
	defer runtime.closeUsage()
	if err := runtime.initializeAdminPrompt(output); err != nil {
		return err
	}
	handler, err := buildHandlerWithRuntime(cfg, "database", runtime)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		if owned, ok := handler.(*catalogHandler); ok {
			owned.Close()
		}
		return fmt.Errorf("listen on %q: %w", cfg.Server.Listen, err)
	}
	defer listener.Close()

	runtime.swap(handler)
	defer runtime.stopIntelligence()
	server := &http.Server{
		Handler:           runtime,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := serveHTTP(ctx, server, listener)
	return errors.Join(serveErr, runtime.closeUsage())
}

const bootstrapConfig = `server:
  listen: 0.0.0.0:8080
providers: []
`

// initializeConfigPath seeds a missing writable configuration from a supplied
// template or the minimal console bootstrap when explicitly enabled.
func initializeConfigPath(path, template string, bootstrap bool) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect configuration: %w", err)
	}
	if template == "" && !bootstrap {
		return nil
	}
	data := []byte(bootstrapConfig)
	if template != "" {
		var err error
		data, err = os.ReadFile(template)
		if err != nil {
			return fmt.Errorf("read configuration template: %w", err)
		}
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".torii-gateway-config-*")
	if err != nil {
		return fmt.Errorf("create initial configuration: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect initial configuration: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write initial configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close initial configuration: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("install initial configuration: %w", err)
	}
	return nil
}

func runAuth(args []string, output io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: gateway auth <login|logout|status> <provider> --database <path>")
	}
	action, providerID := args[0], args[1]
	flags := flag.NewFlagSet("auth "+action, flag.ContinueOnError)
	databasePath := flags.String("database", "", "SQLite database path")
	accessEnv := flags.String("access-token-env", "", "environment variable containing the access token")
	accessFile := flags.String("access-token-file", "", "file containing the access token")
	refreshEnv := flags.String("refresh-token-env", "", "environment variable containing the refresh token")
	refreshFile := flags.String("refresh-token-file", "", "file containing the refresh token")
	expiresAt := flags.String("expires-at", "", "access-token expiry in RFC3339 format")
	accountID := flags.String("account-id", "", "upstream account identifier")
	principalID := flags.String("principal-id", "", "upstream principal identifier")
	browser := flags.Bool("browser", false, "use browser authorization-code + PKCE for Codex login")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected authentication arguments")
	}
	store, err := openSettingsDatabase(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	cfg, err := loadDatabaseSettings(store)
	if err != nil {
		return err
	}
	var providerConfig *config.ProviderConfig
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == providerID {
			providerConfig = &cfg.Providers[i]
			break
		}
	}
	if providerConfig == nil {
		return fmt.Errorf("provider %q is not configured", providerID)
	}
	if *browser && providerConfig.Type != "openai-codex" {
		return errors.New("--browser is supported only for openai-codex")
	}
	if err := authenticateLocalAdmin(store, output); err != nil {
		return err
	}
	switch action {
	case "login":
		if providerConfig.Type == "nous" && *accessEnv == "" && *accessFile == "" && *refreshEnv == "" && *refreshFile == "" {
			portalURL := providerConfig.PortalURL
			if portalURL == "" {
				portalURL = provider.DefaultNousPortalURL
			}
			clientID := providerConfig.ClientID
			if clientID == "" {
				clientID = provider.DefaultNousClientID
			}
			credential, loginErr := provider.NousDeviceCodeLogin(context.Background(), portalURL, clientID, "inference:invoke", &http.Client{}, func(url, code string) {
				_, _ = fmt.Fprintf(output, "open %s and enter code %s\n", url, code)
			})
			if loginErr != nil {
				return loginErr
			}
			credential.Endpoint = strings.TrimRight(providerConfig.BaseURL, "/")
			if err := store.Save(providerID, &state.Credential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt, Endpoint: credential.Endpoint, TokenType: "Bearer"}); err != nil {
				return err
			}
			_, err := fmt.Fprintf(output, "provider %s: logged in\n", providerID)
			return err
		}
		if providerConfig.Type == "openai-codex" && *accessEnv == "" && *accessFile == "" && *refreshEnv == "" && *refreshFile == "" {
			var credential provider.CodexCredential
			var loginErr error
			if *browser {
				credential, loginErr = provider.PKCELogin(context.Background(), "https://auth.openai.com", &http.Client{}, func(url string) { _, _ = fmt.Fprintf(output, "open %s\n", url) })
			} else {
				credential, loginErr = provider.DeviceCodeLogin(context.Background(), "https://auth.openai.com", &http.Client{}, func(url, code string) { _, _ = fmt.Fprintf(output, "open %s and enter code %s\n", url, code) })
			}
			if loginErr != nil {
				return loginErr
			}
			credential.Endpoint = providerConfig.BaseURL
			if err := store.Save(providerID, &state.Credential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt, Endpoint: credential.Endpoint, AccountID: credential.AccountID}); err != nil {
				return err
			}
			_, err := fmt.Fprintf(output, "provider %s: logged in\n", providerID)
			return err
		}
		access, err := resolveCLISecret(*accessEnv, *accessFile, true)
		if err != nil {
			return fmt.Errorf("access token: %w", err)
		}
		refresh, err := resolveCLISecret(*refreshEnv, *refreshFile, false)
		if err != nil {
			return fmt.Errorf("refresh token: %w", err)
		}
		var expiry time.Time
		if *expiresAt != "" {
			expiry, err = time.Parse(time.RFC3339, *expiresAt)
			if err != nil {
				return errors.New("expires-at must be RFC3339")
			}
		}
		if err := store.Save(providerID, &state.Credential{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresAt: expiry, AccountID: *accountID, PrincipalID: *principalID, Endpoint: providerConfig.BaseURL}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "provider %s: logged in\n", providerID)
		return err
	case "logout":
		if *accessEnv != "" || *accessFile != "" || *refreshEnv != "" || *refreshFile != "" || *expiresAt != "" || *accountID != "" || *principalID != "" {
			return errors.New("login-only flags cannot be used with logout")
		}
		if err := store.Delete(providerID); err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "provider %s: logged out\n", providerID)
		return err
	case "status":
		if *accessEnv != "" || *accessFile != "" || *refreshEnv != "" || *refreshFile != "" || *expiresAt != "" || *accountID != "" || *principalID != "" {
			return errors.New("login-only flags cannot be used with status")
		}
		credential, loadErr := store.Load(providerID)
		status := "logged out"
		if loadErr == nil {
			status = "logged in"
			if !credential.ExpiresAt.IsZero() && !credential.ExpiresAt.After(time.Now()) {
				status = "expired"
			}
		} else if !errors.Is(loadErr, state.ErrCredentialNotFound) {
			return loadErr
		}
		_, err = fmt.Fprintf(output, "provider %s: %s\n", providerID, status)
		return err
	default:
		return fmt.Errorf("unsupported auth action %q", action)
	}
}

func resolveCLISecret(envName, fileName string, required bool) (string, error) {
	if (envName == "") == (fileName == "") {
		if !required && envName == "" {
			return "", nil
		}
		return "", errors.New("configure exactly one environment variable or file")
	}
	value, err := (config.SecretRef{Env: envName, File: fileName}).Resolve()
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", errors.New("resolved value is empty")
	}
	return value, nil
}

func serveHTTP(ctx context.Context, server *http.Server, listener net.Listener) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		return normalizeServeError(err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	serveError := normalizeServeError(<-serveErr)
	if serveError != nil {
		return serveError
	}
	if shutdownErr != nil {
		return fmt.Errorf("shut down HTTP server: %w", shutdownErr)
	}
	return nil
}

func normalizeServeError(err error) error {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

func buildHandler(cfg *config.Config) (http.Handler, error) {
	return buildHandlerWithRuntime(cfg, "", nil)
}

type handlerRuntime struct {
	adminBasic      *api.BasicAdminAuthenticator
	configMu        sync.Mutex
	configKey       []byte
	configVersion   uint64
	usageMu         sync.Mutex
	usageStore      *usage.Store
	accountingStore *accounting.Store
	activityStore   *activity.Store
	setupDigest     [32]byte
	setupExpires    time.Time
	credentials     *state.Database
	background      context.Context
	swapMu          sync.Mutex
	mu              sync.RWMutex
	handler         http.Handler
	configPath      string
	loginMu         sync.Mutex
	logins          map[string]providerLogin
	loginFlows      map[string]providerLoginFlow
	loginSessions   map[string]providerLoginSession
}

type providerLogin struct {
	Status          string    `json:"status"`
	Message         string    `json:"message,omitempty"`
	VerificationURL string    `json:"verification_url,omitempty"`
	UserCode        string    `json:"user_code,omitempty"`
	HelperURL       string    `json:"helper_url,omitempty"`
	HelperToken     string    `json:"helper_token,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
}

type providerLoginFlow struct {
	Authorization provider.PKCEAuthorization
	Provider      config.ProviderConfig
}

type providerLoginSession struct {
	TokenHash string
	Provider  config.ProviderConfig
	HostID    string
	ClientID  string
	IDToken   string
	ExpiresAt time.Time
	Used      bool
}

func (r *handlerRuntime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()
	if handler == nil {
		http.Error(w, "gateway is starting", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, req)
}

func (r *handlerRuntime) swap(handler http.Handler) {
	r.swapMu.Lock()
	defer r.swapMu.Unlock()
	r.mu.RLock()
	previous := r.handler
	r.mu.RUnlock()
	if previous == handler {
		return
	}
	if old, ok := previous.(*catalogHandler); ok {
		old.stopIntelligence()
	}
	if next, ok := handler.(*catalogHandler); ok && next.intelligence != nil {
		next.intelligence.ReloadCache()
	}
	r.mu.Lock()
	r.handler = handler
	r.mu.Unlock()
	if next, ok := handler.(*catalogHandler); ok && next.onActivate != nil {
		next.onActivate()
	}
	if next, ok := handler.(*catalogHandler); ok && r.background != nil {
		next.startIntelligence(r.background)
	}
}

// Keep the catalog refresh callback attached to the active handler, so a
// rejected configuration save cannot redirect login refreshes to a discarded one.
type catalogHandler struct {
	http.Handler
	refresh         func(context.Context) error
	intelligence    *intelligence.Manager
	ownedUsage      *usage.Store
	ownedAccounting *accounting.Store
	onActivate      func()
	workerMu        sync.Mutex
	workerCancel    context.CancelFunc
	workerDone      chan struct{}
}

func (h *catalogHandler) RefreshCatalog(ctx context.Context) error {
	return h.refresh(ctx)
}

func (r *handlerRuntime) refreshCatalog(ctx context.Context) error {
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()
	if catalog, ok := handler.(interface{ RefreshCatalog(context.Context) error }); ok {
		return catalog.RefreshCatalog(ctx)
	}
	return nil
}

func (r *handlerRuntime) setLoginSuccess(providerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := r.refreshCatalog(ctx)
	cancel()
	message := "Provider login complete"
	if err != nil {
		message += "; model discovery will retry on the next refresh"
	}
	r.setProviderLogin(providerID, providerLogin{Status: "success", Message: message})
}

func buildHandlerWithRuntime(cfg *config.Config, configPath string, runtime *handlerRuntime) (http.Handler, error) {
	database := cfg.Credentials
	if database == nil && runtime != nil {
		database = runtime.credentials
	}
	var authenticator api.ClientAuthenticator = api.NoAuthAuthenticator{}
	var managementAuth api.ClientAuthenticator = authenticator
	if database != nil {
		authenticator = api.CredentialBearerAuthenticator{Identify: func(token string) (*api.ClientIdentity, error) {
			key, err := database.ResolveGatewayAPIKey(token)
			if err != nil {
				return nil, api.ErrUnauthorized
			}
			return &api.ClientIdentity{ID: key.ID, Name: key.Name}, nil
		}}
		basic := api.NewBasicAdminAuthenticator(database.VerifyAdmin)
		if runtime != nil {
			if runtime.adminBasic == nil {
				runtime.adminBasic = basic
			}
			basic = runtime.adminBasic
		}
		managementAuth = sessionAuthenticator{db: database, basic: basic, server: cfg.Server}
	}

	providers := make([]provider.OpenAIProvider, 0, len(cfg.Providers))
	for _, providerConfig := range cfg.Providers {
		if providerConfig.Enabled != nil && !*providerConfig.Enabled {
			continue
		}
		upstream, err := buildProviderWithState(providerConfig, database)
		if err != nil {
			return nil, fmt.Errorf("initialize provider %q: %w", providerConfig.ID, err)
		}
		providers = append(providers, upstream)
	}
	overrides := cfg.Router.CatalogOverrides
	var err error
	if cfg.Router.CatalogFile != "" {
		overrides, err = loadCatalogOverrides(cfg.Router.CatalogFile)
	}
	if err != nil {
		return nil, err
	}
	openAI, err := api.NewOpenAIHandlerWithOverrides(providers, overrides)
	if err != nil {
		return nil, err
	}
	freeProviders := map[string]bool{}
	for _, p := range cfg.Providers {
		freeProviders[p.ID] = p.Type == "ollama"
		if p.TreatModelsAsFree != nil {
			freeProviders[p.ID] = *p.TreatModelsAsFree
		}
	}
	openAI.SetModelControls(cfg.ModelEnabled, freeProviders)
	if err := openAI.RefreshCatalog(context.Background()); err != nil {
		return nil, fmt.Errorf("initialize model catalog: %w", err)
	}
	var accountingStore *accounting.Store
	if database != nil {
		if runtime != nil {
			if runtime.accountingStore == nil {
				runtime.accountingStore, err = accounting.Open(database)
			}
			accountingStore = runtime.accountingStore
		} else {
			accountingStore, err = accounting.Open(database)
		}
		if err != nil {
			return nil, fmt.Errorf("initialize accounting: %w", err)
		}
		openAI.SetAccountingOverrides(cfg.Router.CatalogOverrides)
		providers = openAI.ConfigureAccounting(accountingStore, cfg.Budgets, providers)
	}
	if runtime != nil && database != nil {
		if runtime.activityStore == nil {
			runtime.activityStore, err = activity.Open(database, cfg.Activity)
			if err != nil {
				return nil, fmt.Errorf("initialize activity: %w", err)
			}
		}
		openAI.SetActivity(runtime.activityStore)
	}
	var configuredClassifier classifier.Classifier
	decisionOptions := classifier.DecisionOptions{Questions: cfg.Router.Classifier.Questions, NoulThresholds: cfg.Router.Classifier.NoulThresholds}
	if cfg.Router.Classifier.Type == "jev" {
		if cfg.Router.Classifier.Transport == "provider" {
			var selected provider.OpenAIProvider
			for _, candidate := range providers {
				if candidate.ID() == cfg.Router.Classifier.Provider {
					selected = candidate
					break
				}
			}
			decisionProvider, ok := selected.(provider.DecisionProvider)
			if !ok {
				return nil, fmt.Errorf("initialize Jev classifier: provider %q does not support decision transport", cfg.Router.Classifier.Provider)
			}
			configuredClassifier, err = classifier.NewJevTransportWithOptions(decisionProvider, cfg.Router.Classifier.Model, decisionOptions)
			if err != nil {
				return nil, fmt.Errorf("initialize Jev classifier: %w", err)
			}
		} else {
			apiKey, resolveErr := cfg.Router.Classifier.APIKey.Resolve()
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve Jev API key: %w", resolveErr)
			}
			client := &http.Client{}
			if cfg.Credentials != nil {
				apiKey = "database-managed"
				client.Transport = &secretTransport{base: http.DefaultTransport, headers: map[string]config.SecretRef{"Authorization": *cfg.Router.Classifier.APIKey}, bearer: true}
			}
			if accountingStore != nil {
				base := client.Transport
				if base == nil {
					base = http.DefaultTransport
				}
				client.Transport = &accounting.Transport{Base: base, Store: accountingStore, Settings: cfg.Budgets, Describe: openAI.AccountingDescriptor}
			}
			configuredClassifier, err = classifier.NewJevWithOptions(cfg.Router.Classifier.BaseURL, apiKey, cfg.Router.Classifier.Model, client, decisionOptions)
			if err != nil {
				return nil, fmt.Errorf("initialize Jev classifier: %w", err)
			}
		}
	}
	if classifierModel := cfg.Router.Classifier.Model; classifierModel != "" && cfg.Router.Classifier.Type != "embedding" && cfg.Router.Classifier.Type != "jev" {
		connected, classifierErr := classifier.NewConnectedModel(classifierModel, providers)
		if classifierErr != nil {
			return nil, fmt.Errorf("initialize classifier: %w", classifierErr)
		}
		configuredClassifier = connected
	}
	if cfg.Router.Classifier.Type == "embedding" {
		var embeddingProvider provider.OpenAIProvider
		for _, candidate := range providers {
			if candidate.ID() == cfg.Router.Classifier.Provider {
				embeddingProvider = candidate
				break
			}
		}
		if embeddingProvider == nil {
			return nil, fmt.Errorf("initialize embedding classifier: provider %q is unavailable", cfg.Router.Classifier.Provider)
		}
		embedder := classifier.ProviderEmbedder{Provider: embeddingProvider, Model: cfg.Router.Classifier.Model}
		prototype, classifierErr := classifier.NewPrototypeClassifier(context.Background(), embedder)
		if classifierErr != nil {
			return nil, fmt.Errorf("initialize embedding classifier: %w", classifierErr)
		}
		configuredClassifier = prototype
	}
	if configuredClassifier != nil {
		failure := classifier.FailureCapabilityFallback
		if cfg.Router.Classifier.OnFailure == string(classifier.FailureFail) {
			failure = classifier.FailureFail
		}
		timeout := 10 * time.Second
		if configured := cfg.Router.Classifier.Timeout; configured != "" {
			timeout, err = time.ParseDuration(configured)
			if err != nil {
				return nil, fmt.Errorf("initialize classifier timeout: %w", err)
			}
		}
		openAI.SetClassifier(classifier.Runner{Classifier: configuredClassifier, Timeout: timeout, MinimumConfidence: cfg.Router.Classifier.MinimumConfidence, OnFailure: failure})
	}
	selection := router.DefaultSelectionPolicy()
	if cfg.Router.AllowPaidModels != nil {
		selection.AllowPaidModels = *cfg.Router.AllowPaidModels
	}
	if value := cfg.Router.Selection.ContextUtilizationLimit; value > 0 {
		selection.MaxContextFraction = value
	}
	if value := cfg.Router.Selection.DefaultExpectedOutputTokens; value > 0 {
		selection.DefaultExpectedOutputTokens = value
	}
	if value := cfg.Router.Selection.LocationPreference; value != "" {
		selection.LocationPreference = router.LocationPreference(value)
	}
	if cfg.Router.Selection.MinimumQuality != nil {
		selection.MinimumQuality = cfg.Router.Selection.MinimumQuality
	}
	openAI.SetSelectionPolicy(selection)
	benchmarkManager, err := buildIntelligence(cfg, openAI.CatalogModels())
	if err != nil {
		return nil, fmt.Errorf("initialize benchmark intelligence: %w", err)
	}
	if benchmarkManager != nil {
		selection.Intelligence = cfg.Intelligence.Routing
		openAI.SetSelectionPolicy(selection)
		openAI.SetIntelligence(benchmarkManager)
	}
	if raw := cfg.Router.Sessions.InactivityTimeout; raw != "" {
		if timeout, parseErr := time.ParseDuration(raw); parseErr == nil {
			openAI.SetSessionTimeout(timeout)
		}
	}
	if cfg.Router.Sessions.OnModelFailure != "" {
		openAI.SetModelFailurePolicy(cfg.Router.Sessions.OnModelFailure)
	}
	quotaTTL := time.Minute
	if raw := cfg.Router.Quota.CacheTTL; raw != "" {
		if parsed, parseErr := time.ParseDuration(raw); parseErr == nil {
			quotaTTL = parsed
		}
	}
	quotaFetchers := make(map[string]quota.Fetcher)
	for _, upstream := range providers {
		if fetcher, ok := upstream.(quota.Fetcher); ok {
			quotaFetchers[upstream.ID()] = fetcher
		}
	}
	var quotaFetcher quota.Fetcher
	if len(quotaFetchers) > 0 {
		quotaFetcher = providerQuotaFetcher{providers: quotaFetchers}
	}
	quotaCache := quota.NewCache(quotaTTL, quotaFetcher)
	openAI.SetQuotaCache(quotaCache)
	var usageStore *usage.Store
	var usageErr error
	if runtime != nil {
		runtime.usageMu.Lock()
		if runtime.usageStore == nil {
			runtime.usageStore, usageErr = usage.Open(database)
		}
		usageStore = runtime.usageStore
		runtime.usageMu.Unlock()
	} else {
		usageStore, usageErr = usage.Open(database)
	}
	if usageErr != nil {
		if benchmarkManager != nil {
			benchmarkManager.Close()
		}
		return nil, fmt.Errorf("initialize usage state: %w", usageErr)
	}
	openAI.SetUsageStore(usageStore)
	for model, limit := range cfg.Router.Quota.Allowances {
		used := float64(0)
		if summary, exists := usageStore.Get(model); exists {
			used = summary.EstimatedCost
		}
		_ = quotaCache.SetLocal(model, limit, used)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	apiHandler := api.AuthenticateMiddleware(authenticator, openAI)
	mux.Handle("/v1/", apiHandler)
	if runtime != nil && configPath != "" {
		protect := func(pattern string, handler http.HandlerFunc) {
			mux.Handle(pattern, managementBoundary(managementAuth, handler))
		}
		if runtime.credentials != nil {
			mux.Handle("/gateway/v1/session", runtime.adminSessionAPI(cfg.Server))
			mux.HandleFunc("GET /gateway/v1/setup", runtime.setupStatus)
			mux.HandleFunc("POST /gateway/v1/setup", runtime.setupAdmin)
			protect("POST /gateway/v1/admin/api-key", runtime.rotateAPIKey)
			protect("/gateway/v1/admin/api-keys", runtime.gatewayKeysAPI)
			protect("/gateway/v1/admin/api-keys/", runtime.gatewayKeysAPI)
		}
		protect("GET /gateway/v1/activity", runtime.activityAPI)
		protect("GET /gateway/v1/activity/events", runtime.activityAPI)
		protect("GET /gateway/v1/usage", openAI.UsageAPI)
		protect("GET /gateway/v1/quotas", openAI.UsageAPI)
		protect("PUT /gateway/v1/quotas", runtime.setBudgets)
		protect("GET /gateway/v1/models", openAI.ManagementModels)
		protect("PUT /gateway/v1/models", runtime.setModel)
		protect("GET /gateway/v1/config", runtime.getConfig)
		protect("PUT /gateway/v1/config", runtime.putConfig)
		protect("POST /gateway/v1/config", runtime.putConfig)
		protect("POST /gateway/v1/providers/test", runtime.testProvider)
		protect("POST /gateway/v1/providers/login", runtime.startProviderLogin)
		protect("GET /gateway/v1/providers/login", runtime.providerLoginStatus)
		protect("/gateway/v1/credentials", runtime.credentialAPI)
		protect("/gateway/v1/credentials/", runtime.credentialAPI)
		mux.HandleFunc("GET /gateway/v1/providers/login/session", runtime.providerLoginSession)
		mux.HandleFunc("POST /gateway/v1/providers/login/import", runtime.importProviderLogin)
		protect("GET /gateway/v1/providers/login/helper/{asset}", runtime.downloadLoginHelper)
		mux.HandleFunc("GET /gateway/v1/providers/login/callback", runtime.providerLoginCallback)
		mux.HandleFunc("GET /auth/callback", runtime.providerLoginCallback)
	}
	benchmarkAuth := authenticator
	if cfg.Credentials != nil {
		benchmarkAuth = benchmarkAuthenticator{basic: managementAuth, bearer: authenticator}
	}
	mux.Handle("/gateway/v1/", managementBoundary(benchmarkAuth, openAI))
	mux.Handle("/", web.Handler())
	handler := &catalogHandler{Handler: api.RequestIDMiddleware(nil, mux), refresh: openAI.RefreshCatalog, intelligence: benchmarkManager}
	if runtime != nil && runtime.activityStore != nil {
		handler.onActivate = func() { runtime.activityStore.Configure(cfg.Activity) }
	}
	if runtime == nil {
		handler.ownedAccounting = accountingStore
		handler.ownedUsage = usageStore
	}
	if runtime != nil && database != nil {
		data, _, err := database.Settings()
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			data, err = yaml.Marshal(cfg)
			if err != nil {
				return nil, err
			}
			if _, err = database.CommitSettings(data, "", nil); err != nil {
				return nil, err
			}
		}
	}
	return handler, nil
}

func (r *handlerRuntime) closeUsage() error {
	r.usageMu.Lock()
	defer r.usageMu.Unlock()
	if r.activityStore != nil {
		defer r.activityStore.Close()
	}
	if r.accountingStore != nil {
		defer r.accountingStore.Close()
	}
	if r.usageStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return r.usageStore.Close(ctx)
}

// Close is used by standalone handler owners; runtime handlers share usage.
func (h *catalogHandler) Close() error {
	h.stopIntelligence()
	if h.ownedAccounting != nil {
		defer h.ownedAccounting.Close()
	}
	if h.ownedUsage == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.ownedUsage.Close(ctx)
}

func (r *handlerRuntime) getConfig(w http.ResponseWriter, _ *http.Request) {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.handler == nil {
		http.Error(w, "gateway is starting", http.StatusServiceUnavailable)
		return
	}
	cfg, err := r.loadConfig()
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	revision, err := r.configRevision(cfg)
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, errors.New("cannot create configuration revision"))
		return
	}
	w.Header().Set("ETag", revision)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(document)
}

func (r *handlerRuntime) putConfig(w http.ResponseWriter, req *http.Request) {
	// Hold this through publication: two callers with the same revision must
	// never both store credentials or publish replacement configurations.
	r.configMu.Lock()
	defer r.configMu.Unlock()
	if len(req.Header.Values("If-Match")) == 0 {
		writeConfigError(w, http.StatusPreconditionRequired, errors.New("configuration revision required; read configuration and send its ETag in If-Match"))
		return
	}
	current, err := r.loadConfig()
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, errors.New("cannot load active configuration"))
		return
	}
	revision, err := r.configRevision(current)
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, errors.New("cannot create configuration revision"))
		return
	}
	// Require one exact strong revision; wildcards, weak tags and lists would
	// let callers bypass the protection against outdated full documents.
	if len(req.Header.Values("If-Match")) != 1 || req.Header.Get("If-Match") != revision {
		writeConfigError(w, http.StatusPreconditionFailed, errors.New("configuration changed; reload and reconcile your edits before saving"))
		return
	}
	if req.ContentLength > 2<<20 {
		writeConfigError(w, http.StatusRequestEntityTooLarge, errors.New("configuration exceeds 2 MiB"))
		return
	}
	decoder := yaml.NewDecoder(io.LimitReader(req.Body, 2<<20))
	decoder.KnownFields(true)
	var next config.Config
	if err := decoder.Decode(&next); err != nil {
		writeConfigError(w, http.StatusBadRequest, errors.New("invalid configuration document (input omitted for credential safety)"))
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			writeConfigError(w, http.StatusBadRequest, errors.New("multiple configuration documents are not allowed"))
		} else {
			writeConfigError(w, http.StatusBadRequest, errors.New("invalid configuration document (input omitted for credential safety)"))
		}
		return
	}
	if err := next.Validate(); err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("validate configuration: %w", err))
		return
	}
	if current.State.DatabasePath != next.State.DatabasePath {
		writeConfigError(w, 400, errors.New("database path is a bootstrap option; restart with --database"))
		return
	}
	if next.Router.CatalogFile != "" {
		writeConfigError(w, 400, errors.New("use catalog_overrides in database settings"))
		return
	}
	overlay, secrets, err := prepareSettings(&next, r.credentials, false)
	if err != nil {
		writeConfigError(w, 400, err)
		return
	}
	newHandler, err := buildHandlerWithRuntime(&next, r.configPath, r)
	if err != nil {
		writeConfigError(w, 400, fmt.Errorf("initialize configuration: %w", err))
		return
	}
	data, err := yaml.Marshal(&next)
	if err != nil {
		writeConfigError(w, 500, err)
		return
	}
	expected := strings.Trim(revision, `"`)
	_, _, err = r.credentials.Settings()
	if err == nil {
		_, err = r.credentials.CommitSettings(data, expected, secrets)
	}
	if err != nil {
		newHandler.(*catalogHandler).stopIntelligence()
		if errors.Is(err, state.ErrSettingsConflict) {
			writeConfigError(w, http.StatusPreconditionFailed, err)
			return
		}
		writeConfigError(w, 500, errors.New("cannot commit configuration"))
		return
	}
	overlay.committed()
	r.swap(newHandler)
	r.configVersion++
	committed, _ := r.configRevision(&next)
	w.Header().Set("ETag", committed)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, 200, map[string]any{"status": "saved", "reloaded": true, "restart_required": current.Server.Listen != next.Server.Listen || current.Activity.LogFile != next.Activity.LogFile || current.Activity.LogMaxBytes != next.Activity.LogMaxBytes || current.Activity.LogBackups != next.Activity.LogBackups})
}

func (r *handlerRuntime) testProvider(w http.ResponseWriter, req *http.Request) {
	decoder := yaml.NewDecoder(io.LimitReader(req.Body, 1<<20))
	decoder.KnownFields(true)
	var request struct {
		Provider config.ProviderConfig `yaml:"provider"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeConfigError(w, http.StatusBadRequest, errors.New("invalid provider document (input omitted for credential safety)"))
		return
	}
	if request.Provider.ID == "" {
		writeConfigError(w, http.StatusBadRequest, errors.New("provider.id is required"))
		return
	}
	var err error
	if request.Provider.Type != "ollama" {
		discover := true
		request.Provider.DiscoverModels = &discover
	}
	if r.credentials != nil {
		draft := config.Config{Providers: []config.ProviderConfig{request.Provider}}
		if _, _, err := prepareSettings(&draft, r.credentials, false); err != nil {
			writeConfigError(w, 400, err)
			return
		}
		request.Provider = draft.Providers[0]
	}
	upstream, err := buildProviderWithState(request.Provider, r.credentials)
	if err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("initialize provider: %w", err))
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()
	var models []provider.Model
	if live, ok := upstream.(interface {
		DiscoverModels(context.Context) ([]provider.Model, error)
	}); ok {
		models, err = live.DiscoverModels(ctx)
	} else {
		models, err = upstream.Models(ctx)
	}
	if err != nil {
		writeConfigError(w, http.StatusBadGateway, fmt.Errorf("test provider connection: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	classifierOnly := false
	if classifier, ok := upstream.(interface{ ClassifierOnly() bool }); ok {
		classifierOnly = classifier.ClassifierOnly()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "models": models, "classifier_only": classifierOnly})
}

func (r *handlerRuntime) startProviderLogin(w http.ResponseWriter, req *http.Request) {
	var request struct {
		Provider   yaml.Node `json:"provider" yaml:"provider"`
		ProviderID string    `json:"provider_id" yaml:"provider_id"`
	}
	if err := yaml.NewDecoder(io.LimitReader(req.Body, 64<<10)).Decode(&request); err != nil {
		writeConfigError(w, http.StatusBadRequest, errors.New("invalid login document (input omitted for credential safety)"))
		return
	}
	var providerConfig config.ProviderConfig
	providerID := request.ProviderID
	if request.Provider.Kind == yaml.ScalarNode {
		providerID = request.Provider.Value
	} else if request.Provider.Kind != 0 {
		if err := request.Provider.Decode(&providerConfig); err != nil {
			writeConfigError(w, http.StatusBadRequest, errors.New("invalid provider document (input omitted for credential safety)"))
			return
		}
		if providerID == "" {
			providerID = providerConfig.ID
		}
	}
	if providerID == "" {
		writeConfigError(w, http.StatusBadRequest, errors.New("provider is required"))
		return
	}
	current, err := r.loadConfig()
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("load current configuration: %w", err))
		return
	}
	if providerConfig.ID == "" {
		for i := range current.Providers {
			if current.Providers[i].ID == providerID {
				providerConfig = current.Providers[i]
				break
			}
		}
	}
	if providerConfig.ID == "" {
		writeConfigError(w, http.StatusNotFound, fmt.Errorf("provider %q is not configured", providerID))
		return
	}
	if providerConfig.Type == "openai-codex" {
		if providerConfig.BaseURL == "" || providerConfig.BaseURL == "https://example.invalid" {
			providerConfig.BaseURL = provider.DefaultCodexBaseURL
		}
	}
	if providerConfig.Type != "openai-codex" && providerConfig.Type != "nous" {
		writeConfigError(w, http.StatusBadRequest, errors.New("browser login is supported only for openai-codex and nous providers"))
		return
	}
	if providerConfig.Type == "openai-codex" {
		if r.credentials == nil {
			writeConfigError(w, http.StatusServiceUnavailable, errors.New("Codex login requires the application database"))
			return
		}
		if !isLoopbackRequest(req) {
			r.startRemoteCodexLogin(w, req, providerConfig)
			return
		}
		hostID, hostErr := r.credentials.CodexHostID(req.Context())
		if hostErr != nil {
			writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("prepare Codex host identity: %w", hostErr))
			return
		}
		clientID, idToken := "dynamic_agent_client", ""
		authorization, startErr := provider.StartPKCEForHost("https://auth.openai.com", providerLoginRedirectURL(req), clientID, hostID, idToken)
		if startErr != nil {
			writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("start Codex OAuth: %w", startErr))
			return
		}
		r.loginMu.Lock()
		if r.logins == nil {
			r.logins = make(map[string]providerLogin)
		}
		if r.loginFlows == nil {
			r.loginFlows = make(map[string]providerLoginFlow)
		}
		if existing, ok := r.logins[providerConfig.ID]; ok && existing.Status == "pending" {
			r.loginMu.Unlock()
			writeJSON(w, http.StatusOK, existing)
			return
		}
		r.loginFlows[authorization.State] = providerLoginFlow{Authorization: authorization, Provider: providerConfig}
		r.logins[providerConfig.ID] = providerLogin{Status: "pending", Message: "Waiting for authorization", VerificationURL: authorization.AuthorizationURL}
		r.loginMu.Unlock()
		writeJSON(w, http.StatusAccepted, providerLogin{Status: "pending", Message: "Open the OpenAI authorization page", VerificationURL: authorization.AuthorizationURL})
		return
	}
	r.loginMu.Lock()
	if r.logins == nil {
		r.logins = make(map[string]providerLogin)
	}
	if existing, ok := r.logins[providerConfig.ID]; ok && existing.Status == "pending" {
		r.loginMu.Unlock()
		writeJSON(w, http.StatusOK, existing)
		return
	}
	r.logins[providerConfig.ID] = providerLogin{Status: "pending", Message: "Waiting for authorization"}
	r.loginMu.Unlock()
	writeJSON(w, http.StatusAccepted, providerLogin{Status: "pending", Message: "Waiting for authorization"})
	go r.completeProviderLogin(providerConfig)
}

func (r *handlerRuntime) startRemoteCodexLogin(w http.ResponseWriter, req *http.Request, providerConfig config.ProviderConfig) {
	hostID, err := r.credentials.CodexHostID(req.Context())
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("prepare Codex host identity: %w", err))
		return
	}
	clientID, idToken := "dynamic_agent_client", ""
	token, tokenHash, err := newLoginPairingToken()
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("create Codex login session: %w", err))
		return
	}
	expiresAt := time.Now().Add(10 * time.Minute)
	r.loginMu.Lock()
	if r.logins == nil {
		r.logins = make(map[string]providerLogin)
	}
	if r.loginSessions == nil {
		r.loginSessions = make(map[string]providerLoginSession)
	}
	if existing, ok := r.logins[providerConfig.ID]; ok && existing.Status == "pending" && existing.HelperURL != "" && existing.ExpiresAt.After(time.Now()) {
		r.loginMu.Unlock()
		writeJSON(w, http.StatusAccepted, existing)
		return
	}
	r.loginSessions[tokenHash] = providerLoginSession{TokenHash: tokenHash, Provider: providerConfig, HostID: hostID, ClientID: clientID, IDToken: idToken, ExpiresAt: expiresAt}
	status := providerLogin{Status: "pending", Message: "Run the Torii Auth Helper on the computer with your browser", HelperURL: providerLoginSessionURL(req), HelperToken: token, ExpiresAt: expiresAt}
	r.logins[providerConfig.ID] = status
	r.loginMu.Unlock()
	writeJSON(w, http.StatusAccepted, status)
}

func (r *handlerRuntime) providerLoginSession(w http.ResponseWriter, req *http.Request) {
	session, ok := r.findLoginSession(req)
	if !ok {
		writeConfigError(w, http.StatusUnauthorized, errors.New("Codex login session is invalid or expired"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"provider_id": session.Provider.ID, "issuer": "https://auth.openai.com", "client_id": session.ClientID, "host_id": session.HostID})
}

func (r *handlerRuntime) downloadLoginHelper(w http.ResponseWriter, req *http.Request) {
	asset := req.PathValue("asset")
	allowed := map[string]string{
		"torii-auth-linux-amd64":       "torii-auth-linux-amd64",
		"torii-auth-linux-arm64":       "torii-auth-linux-arm64",
		"torii-auth-darwin-amd64":      "torii-auth-darwin-amd64",
		"torii-auth-darwin-arm64":      "torii-auth-darwin-arm64",
		"torii-auth-windows-amd64.exe": "torii-auth-windows-amd64.exe",
	}
	filename, ok := allowed[asset]
	if !ok {
		http.NotFound(w, req)
		return
	}
	path := filepath.Join("/helpers", filename)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "This container image does not include the login helper; rebuild the image with the current Dockerfile.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Unable to read the login helper.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (r *handlerRuntime) importProviderLogin(w http.ResponseWriter, req *http.Request) {
	session, ok := r.findLoginSession(req)
	if !ok {
		writeConfigError(w, http.StatusUnauthorized, errors.New("Codex login session is invalid or expired"))
		return
	}
	var imported struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		TokenType    string    `json:"token_type"`
		ExpiresAt    time.Time `json:"expires_at"`
		AccountID    string    `json:"account_id"`
		ClientID     string    `json:"client_id"`
		IDToken      string    `json:"id_token"`
		Issuer       string    `json:"issuer"`
		Subject      string    `json:"subject"`
		Email        string    `json:"email"`
		HostID       string    `json:"host_id"`
		Scopes       []string  `json:"scopes"`
	}
	if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&imported); err != nil {
		writeConfigError(w, http.StatusBadRequest, errors.New("invalid Codex credential document (input omitted for credential safety)"))
		return
	}
	if imported.AccessToken == "" || imported.RefreshToken == "" || imported.ClientID == "" || imported.HostID == "" {
		writeConfigError(w, http.StatusBadRequest, errors.New("Codex credential is missing required fields"))
		return
	}
	if !containsScope(imported.Scopes, "chatgpt.tokens.use.direct") {
		writeConfigError(w, http.StatusBadRequest, errors.New("Codex credential does not include ChatGPT plan usage permission"))
		return
	}
	if imported.HostID != session.HostID || (session.ClientID != "dynamic_agent_client" && imported.ClientID != session.ClientID) {
		writeConfigError(w, http.StatusBadRequest, errors.New("Codex credential does not match this login session"))
		return
	}
	store, err := r.oauthStore()
	if err == nil {
		err = store.Save(session.Provider.ID, &state.Credential{AccessToken: imported.AccessToken, RefreshToken: imported.RefreshToken, TokenType: imported.TokenType, ExpiresAt: imported.ExpiresAt, AccountID: imported.AccountID, Endpoint: session.Provider.BaseURL, ClientID: imported.ClientID, IDToken: imported.IDToken, Issuer: imported.Issuer, Subject: imported.Subject, Email: imported.Email, HostID: imported.HostID, Scopes: imported.Scopes})
	}
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("save Codex credential: %w", err))
		return
	}
	r.loginMu.Lock()
	if current, exists := r.loginSessions[session.TokenHash]; exists {
		current.Used = true
		r.loginSessions[session.TokenHash] = current
	}
	r.loginMu.Unlock()
	r.setLoginSuccess(session.Provider.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "imported", "provider_id": session.Provider.ID})
}

func (r *handlerRuntime) findLoginSession(req *http.Request) (providerLoginSession, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		return providerLoginSession{}, false
	}
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	r.loginMu.Lock()
	defer r.loginMu.Unlock()
	session, ok := r.loginSessions[tokenHash]
	if !ok || session.Used || !session.ExpiresAt.After(time.Now()) {
		return providerLoginSession{}, false
	}
	return session, true
}

func newLoginPairingToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func containsScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func providerLoginSessionURL(req *http.Request) string {
	scheme := req.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if req.TLS != nil {
			scheme = "https"
		}
	}
	return (&url.URL{Scheme: scheme, Host: req.Host, Path: "/gateway/v1/providers/login/session"}).String()
}

func isLoopbackRequest(req *http.Request) bool {
	host := req.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (r *handlerRuntime) providerLoginStatus(w http.ResponseWriter, req *http.Request) {
	providerID := req.URL.Query().Get("provider")
	r.loginMu.Lock()
	status, ok := r.logins[providerID]
	r.loginMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_started"})
		return
	}
	status.HelperToken = ""
	writeJSON(w, http.StatusOK, status)
}

func providerLoginRedirectURL(req *http.Request) string {
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", provider.CodexOAuthPort(req.Host)), Path: "/auth/callback"}).String()
}

func (r *handlerRuntime) providerLoginCallback(w http.ResponseWriter, req *http.Request) {
	oauthState := req.URL.Query().Get("state")
	r.loginMu.Lock()
	flow, ok := r.loginFlows[oauthState]
	if ok {
		delete(r.loginFlows, oauthState)
	}
	r.loginMu.Unlock()
	if !ok || oauthState == "" {
		http.Error(w, "This OAuth login is invalid or expired.", http.StatusBadRequest)
		return
	}
	if oauthError := req.URL.Query().Get("error"); oauthError != "" {
		r.setProviderLogin(flow.Provider.ID, providerLogin{Status: "failure", Message: "Codex authorization failed: " + oauthError})
		http.Error(w, "Authorization failed. You may close this window.", http.StatusBadRequest)
		return
	}
	code := req.URL.Query().Get("code")
	if code == "" {
		r.setProviderLogin(flow.Provider.ID, providerLogin{Status: "failure", Message: "Codex authorization callback did not include a code"})
		http.Error(w, "Authorization failed. You may close this window.", http.StatusBadRequest)
		return
	}
	if clientID := req.URL.Query().Get("client_id"); clientID != "" {
		flow.Authorization.ClientID = clientID
	}
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()
	credential, err := provider.ExchangePKCE(ctx, "https://auth.openai.com", code, flow.Authorization, &http.Client{})
	if err == nil {
		identity, validateErr := provider.ValidateCodexIDToken(ctx, credential.IDToken, "https://auth.openai.com", credential.ClientID, flow.Authorization.Nonce, &http.Client{})
		if validateErr != nil {
			err = validateErr
		} else if !containsScope(credential.Scopes, "chatgpt.tokens.use.direct") {
			err = errors.New("Codex authorization did not grant ChatGPT plan usage")
		} else {
			credential.Issuer = identity.Issuer
			credential.Subject = identity.Subject
			credential.Email = identity.Email
		}
	}
	if err == nil {
		store, storeErr := r.oauthStore()
		if storeErr != nil {
			err = storeErr
		} else {
			err = store.Save(flow.Provider.ID, &state.Credential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt, Endpoint: flow.Provider.BaseURL, AccountID: credential.AccountID, ClientID: credential.ClientID, IDToken: credential.IDToken, Issuer: credential.Issuer, Subject: credential.Subject, Email: credential.Email, HostID: credential.HostID, Scopes: credential.Scopes, TokenType: "Bearer"})
		}
	}
	if err != nil {
		r.setProviderLogin(flow.Provider.ID, providerLogin{Status: "failure", Message: err.Error()})
		http.Error(w, "Authorization failed. You may close this window.", http.StatusBadGateway)
		return
	}
	r.setLoginSuccess(flow.Provider.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<html><body><h1>Torii Gateway login complete</h1><p>You may close this window and return to the console.</p></body></html>")
}

func (r *handlerRuntime) completeProviderLogin(providerConfig config.ProviderConfig) {
	store, err := r.oauthStore()
	if err != nil {
		r.setProviderLogin(providerConfig.ID, providerLogin{Status: "failure", Message: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if providerConfig.Type == "openai-codex" {
		credential, loginErr := provider.DeviceCodeLogin(ctx, "https://auth.openai.com", &http.Client{}, func(url, code string) {
			r.setProviderLogin(providerConfig.ID, providerLogin{Status: "pending", Message: "Open the verification URL and enter the code", VerificationURL: url, UserCode: code})
		})
		if loginErr == nil {
			loginErr = store.Save(providerConfig.ID, &state.Credential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt, Endpoint: providerConfig.BaseURL, AccountID: credential.AccountID, TokenType: "Bearer"})
		}
		if loginErr != nil {
			r.setProviderLogin(providerConfig.ID, providerLogin{Status: "failure", Message: loginErr.Error()})
			return
		}
	} else {
		portalURL := providerConfig.PortalURL
		if portalURL == "" {
			portalURL = provider.DefaultNousPortalURL
		}
		clientID := providerConfig.ClientID
		if clientID == "" {
			clientID = provider.DefaultNousClientID
		}
		credential, loginErr := provider.NousDeviceCodeLogin(ctx, portalURL, clientID, "inference:invoke", &http.Client{}, func(url, code string) {
			r.setProviderLogin(providerConfig.ID, providerLogin{Status: "pending", Message: "Open the verification URL and enter the code", VerificationURL: url, UserCode: code})
		})
		if loginErr == nil {
			loginErr = store.Save(providerConfig.ID, &state.Credential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt, Endpoint: strings.TrimRight(providerConfig.BaseURL, "/"), TokenType: "Bearer"})
		}
		if loginErr != nil {
			r.setProviderLogin(providerConfig.ID, providerLogin{Status: "failure", Message: loginErr.Error()})
			return
		}
	}
	r.setLoginSuccess(providerConfig.ID)
}

func (r *handlerRuntime) setProviderLogin(providerID string, status providerLogin) {
	r.loginMu.Lock()
	if r.logins == nil {
		r.logins = make(map[string]providerLogin)
	}
	r.logins[providerID] = status
	r.loginMu.Unlock()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func persistConfig(path string, cfg *config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".torii-gateway-config-*")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary configuration: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace configuration: %w", err)
	}
	return nil
}

func writeConfigError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "decode") || strings.Contains(strings.ToLower(message), "yaml") || strings.Contains(strings.ToLower(message), "parse") {
		message = "invalid request format; check field names and types"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

type providerQuotaFetcher struct{ providers map[string]quota.Fetcher }

func (f providerQuotaFetcher) QuotaKey(model string) string {
	id, _, err := provider.ParseModelID(model)
	if err != nil {
		return model
	}
	if keyer, ok := f.providers[id].(quota.Keyer); ok {
		return keyer.QuotaKey(model)
	}
	return model
}

func (f providerQuotaFetcher) Quota(ctx context.Context, model string) (quota.Status, error) {
	providerID, _, err := provider.ParseModelID(model)
	if err != nil {
		return quota.Status{Known: false}, nil
	}
	upstream := f.providers[providerID]
	if upstream == nil {
		return quota.Status{Known: false}, nil
	}
	return upstream.Quota(ctx, model)
}

func loadCatalogOverrides(path string) ([]catalog.Descriptor, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read router.catalog_file: %w", err)
	}
	if len(data) > 8<<20 {
		return nil, errors.New("router.catalog_file exceeds 8 MiB")
	}
	var records []catalog.Descriptor
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode router.catalog_file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, errors.New("router.catalog_file must contain one JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode router.catalog_file: %w", err)
	}
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.PublicID == "" || record.ProviderID == "" || record.UpstreamID == "" {
			return nil, errors.New("router.catalog_file records require id, provider_id, and upstream_id")
		}
		if _, exists := seen[record.PublicID]; exists {
			return nil, fmt.Errorf("router.catalog_file duplicates model %q", record.PublicID)
		}
		seen[record.PublicID] = struct{}{}
	}
	return records, nil
}

func buildProvider(cfg config.ProviderConfig) (provider.OpenAIProvider, error) {
	return buildProviderWithState(cfg)
}

func buildInferenceProviderWithState(cfg config.ProviderConfig, databases ...*state.Database) (provider.OpenAIProvider, error) {
	var db *state.Database
	if len(databases) > 0 {
		db = databases[0]
	}
	var secretHeaders map[string]config.SecretRef
	bearerAuth := cfg.BearerToken != nil
	if db != nil {
		secretHeaders = map[string]config.SecretRef{}
		if cfg.BearerToken != nil {
			ref := *cfg.BearerToken
			ref.BindDefault(db)
			name := "Authorization"
			if cfg.Type == "anthropic" {
				name = "x-api-key"
			}
			secretHeaders[name] = ref
			cfg.BearerToken = &config.SecretRef{Value: "database-managed"}
		}
		for name, ref := range cfg.Headers {
			if strings.EqualFold(name, "Authorization") {
				bearerAuth = false
			}
			ref.BindDefault(db)
			secretHeaders[name] = ref
		}
		cfg.Headers = nil
	}
	if cfg.Type == "ollama" {
		timeout := provider.DefaultOllamaRequestTimeout
		if cfg.RequestTimeout != "" {
			var err error
			timeout, err = time.ParseDuration(cfg.RequestTimeout)
			if err != nil || timeout <= 0 {
				return nil, errors.New("Ollama request_timeout must be a positive duration")
			}
		}
		return provider.NewOllama(cfg.ID, cfg.BaseURL, &http.Client{Timeout: timeout})
	}
	if cfg.Type == "nous" {
		if db == nil {
			return nil, errors.New("application database is required for nous")
		}
		var credentialStore state.CredentialStore = db
		loader := func(id string) (provider.NousCredential, error) {
			credential, loadErr := credentialStore.Load(id)
			if loadErr != nil {
				return provider.NousCredential{}, loadErr
			}
			return provider.NousCredential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, Endpoint: credential.Endpoint, ExpiresAt: credential.ExpiresAt}, nil
		}
		saver := func(id string, value provider.NousCredential) error {
			return credentialStore.Save(id, &state.Credential{AccessToken: value.AccessToken, RefreshToken: value.RefreshToken, ExpiresAt: value.ExpiresAt, Endpoint: value.Endpoint, TokenType: "Bearer"})
		}
		discover := true
		if cfg.DiscoverModels != nil {
			discover = *cfg.DiscoverModels
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if db != nil {
			client.Transport = &secretTransport{base: http.DefaultTransport}
		}
		return provider.NewNous(cfg.ID, cfg.BaseURL, cfg.PortalURL, cfg.ClientID, cfg.Models, discover, loader, saver, client)
	}
	if cfg.Type == "anthropic" {
		key, err := cfg.BearerToken.Resolve()
		if err != nil {
			return nil, fmt.Errorf("resolve Anthropic API key: %w", err)
		}
		discover := true
		if cfg.DiscoverModels != nil {
			discover = *cfg.DiscoverModels
		}
		requestTimeout, _ := time.ParseDuration(cfg.RequestTimeout)
		client := &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if db != nil {
			client.Transport = &secretTransport{base: http.DefaultTransport, headers: secretHeaders}
		}
		return provider.NewAnthropic(cfg.ID, cfg.BaseURL, key, cfg.Models, discover, client)
	}
	headers := make(http.Header, len(cfg.Headers)+1)
	if cfg.BearerToken != nil {
		token, err := cfg.BearerToken.Resolve()
		if err != nil {
			return nil, fmt.Errorf("resolve bearer token: %w", err)
		}
		if token == "" || strings.ContainsAny(token, "\r\n") {
			return nil, errors.New("resolved bearer token is empty or contains a newline")
		}
		headers.Set("Authorization", "Bearer "+token)
	}
	for name, ref := range cfg.Headers {
		value, err := ref.Resolve()
		if err != nil {
			return nil, fmt.Errorf("resolve header %q: %w", name, err)
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("resolved header %q contains a newline", name)
		}
		headers.Set(name, value)
	}
	if cfg.Type == "openai" {
		if cfg.Organization != "" {
			headers.Set("OpenAI-Organization", cfg.Organization)
		}
		if cfg.Project != "" {
			headers.Set("OpenAI-Project", cfg.Project)
		}
	}
	if cfg.Type == "openai-codex" {
		if cfg.BaseURL == "" || cfg.BaseURL == "https://example.invalid" {
			cfg.BaseURL = provider.DefaultCodexBaseURL
		}
		if db == nil {
			return nil, errors.New("application database is required for openai-codex")
		}
		var credentialStore state.CredentialStore = db
		loader := func(id string) (provider.CodexCredential, error) {
			credential, err := credentialStore.Load(id)
			if err != nil {
				return provider.CodexCredential{}, err
			}
			return provider.CodexCredential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, Endpoint: credential.Endpoint, ExpiresAt: credential.ExpiresAt, AccountID: credential.AccountID, ClientID: credential.ClientID, IDToken: credential.IDToken, Issuer: credential.Issuer, Subject: credential.Subject, Email: credential.Email, HostID: credential.HostID, Scopes: credential.Scopes}, nil
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if db != nil {
			client.Transport = &secretTransport{base: http.DefaultTransport}
		}
		codex, err := provider.NewCodex(cfg.ID, cfg.BaseURL, cfg.Models, loader, client)
		if err != nil {
			return nil, err
		}
		saver := func(id string, value provider.CodexCredential) error {
			return credentialStore.Save(id, &state.Credential{AccessToken: value.AccessToken, RefreshToken: value.RefreshToken, ExpiresAt: value.ExpiresAt, Endpoint: value.Endpoint, AccountID: value.AccountID, ClientID: value.ClientID, IDToken: value.IDToken, Issuer: value.Issuer, Subject: value.Subject, Email: value.Email, HostID: value.HostID, Scopes: value.Scopes, TokenType: "Bearer"})
		}
		codex.ConfigureRefresh("https://auth.openai.com", saver)
		return codex, nil
	}
	httpTransport, err := compatibleHTTPTransport(cfg)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: httpTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if db != nil {
		client.Transport = &secretTransport{base: httpTransport, headers: secretHeaders, bearer: bearerAuth, management: cfg.ManagementToken}
	}
	discover := true
	if cfg.DiscoverModels != nil {
		discover = *cfg.DiscoverModels
	}
	discoveryTimeout, _ := time.ParseDuration(cfg.DiscoveryTimeout)
	requestTimeout, _ := time.ParseDuration(cfg.RequestTimeout)
	apiPath := cfg.APIPath
	if apiPath == "" {
		switch cfg.Type {
		case "gemini":
			apiPath = "/v1beta/openai"
		case "opencode-go":
			apiPath = "/zen/go/v1"
		case "opencode-zen":
			apiPath = "/zen/v1"
		case "zai":
			apiPath = "/api/coding/paas/v4"
		case "kimi-coding":
			apiPath = "/coding/v1"
		default:
			apiPath = "/v1"
		}
	}
	var quotaURL string
	if cfg.Type == "jev" || cfg.Type == "jev_compatible" {
		return provider.NewJevProvider(provider.OpenAICompatibleConfig{ID: cfg.ID, BaseURL: cfg.BaseURL, APIPath: apiPath, Client: client, Headers: headers, DiscoverModels: discover, StaticModels: cfg.Models, ModelRewrites: cfg.ModelRewrites, DiscoveryTimeout: discoveryTimeout, RequestTimeout: requestTimeout}, cfg.Type == "jev_compatible")
	}
	quotaHeaders := make(http.Header)
	var keyQuotaURL string
	if cfg.Type == "openrouter" {
		keyQuotaURL = strings.TrimRight(cfg.BaseURL, "/") + strings.TrimRight(apiPath, "/") + "/key"
		quotaURL = strings.TrimRight(cfg.BaseURL, "/") + strings.TrimRight(apiPath, "/") + "/credits"
		quotaHeaders = headers.Clone()
	}
	if cfg.Type == "openrouter" && cfg.ManagementToken != nil {
		managementToken := "database-managed"
		var err error
		if db == nil {
			managementToken, err = cfg.ManagementToken.Resolve()
		}
		if err != nil {
			return nil, fmt.Errorf("resolve OpenRouter management token: %w", err)
		}
		if managementToken == "" || strings.ContainsAny(managementToken, "\r\n") {
			return nil, errors.New("resolved OpenRouter management token is empty or contains a newline")
		}
		quotaURL = strings.TrimRight(cfg.BaseURL, "/") + strings.TrimRight(apiPath, "/") + "/credits"
		quotaHeaders.Set("Authorization", "Bearer "+managementToken)
	}
	return provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: cfg.ID, BaseURL: cfg.BaseURL, APIPath: apiPath, Client: client, Headers: headers, QuotaURL: quotaURL, QuotaHeaders: quotaHeaders, KeyQuotaURL: keyQuotaURL, DiscoverModels: discover, StaticModels: cfg.Models, ModelRewrites: cfg.ModelRewrites, DiscoveryTimeout: discoveryTimeout, RequestTimeout: requestTimeout})
}

func compatibleHTTPTransport(cfg config.ProviderConfig) (*http.Transport, error) {
	connectTimeout, _ := time.ParseDuration(cfg.ConnectTimeout)
	dialer := &net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.DialContext = dialer.DialContext
	if cfg.TLS != nil {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.TLS.ServerName}
		if cfg.TLS.CAFile != "" {
			pem, err := os.ReadFile(cfg.TLS.CAFile)
			if err != nil {
				return nil, fmt.Errorf("read TLS CA file: %w", err)
			}
			roots, err := x509.SystemCertPool()
			if err != nil {
				roots = x509.NewCertPool()
			}
			if !roots.AppendCertsFromPEM(pem) {
				return nil, errors.New("TLS CA file contains no certificates")
			}
			tlsConfig.RootCAs = roots
		}
		httpTransport.TLSClientConfig = tlsConfig
	}
	return httpTransport, nil
}
