// Command gateway is the entry point for the Torii Gateway service.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
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

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/router"
	"github.com/Apartment38/torii-gateway/internal/state"
	"github.com/Apartment38/torii-gateway/internal/usage"
	"github.com/Apartment38/torii-gateway/internal/web"
	"gopkg.in/yaml.v3"
)

func main() {
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
		if _, err := config.Load(*configPath); err != nil {
			return err
		}
		_, err := fmt.Fprintln(output, "configuration is valid")
		return err
	}
	if len(args) > 0 && args[0] == "models" {
		flags := flag.NewFlagSet("models", flag.ContinueOnError)
		configPath := flags.String("config", "/config/config.yaml", "path to the YAML configuration")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: gateway models --config <path>")
		}
		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		for _, providerConfig := range cfg.Providers {
			upstream, buildErr := buildProviderWithState(providerConfig, cfg.State.Directory)
			if buildErr != nil {
				return fmt.Errorf("initialize provider %q: %w", providerConfig.ID, buildErr)
			}
			models, modelErr := upstream.Models(ctx)
			if modelErr != nil {
				return fmt.Errorf("discover provider %q: %w", providerConfig.ID, modelErr)
			}
			for _, model := range models {
				if _, err := fmt.Fprintln(output, model.ID); err != nil {
					return err
				}
			}
		}
		_, err = fmt.Fprintln(output, "smart")
		return err
	}
	if len(args) > 0 && args[0] == "auth" {
		return runAuth(args[1:], output)
	}
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: gateway <serve|auth> [options]")
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "/config/config.yaml", "path to the YAML configuration")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: gateway serve --config <path>")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	runtime := &handlerRuntime{configPath: *configPath}
	handler, err := buildHandlerWithRuntime(cfg, *configPath, runtime)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", cfg.Server.Listen, err)
	}
	defer listener.Close()

	runtime.handler = handler
	server := &http.Server{
		Handler:           runtime,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return serveHTTP(ctx, server, listener)
}

func runAuth(args []string, output io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: gateway auth <login|logout|status> <provider> --config <path>")
	}
	action, providerID := args[0], args[1]
	flags := flag.NewFlagSet("auth "+action, flag.ContinueOnError)
	configPath := flags.String("config", "/config/config.yaml", "path to the YAML configuration")
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
	cfg, err := config.Load(*configPath)
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
	store, err := state.NewFlatFileCredentialStore(cfg.State.Directory)
	if err != nil {
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
	mu         sync.RWMutex
	handler    http.Handler
	configPath string
	loginMu    sync.Mutex
	logins     map[string]providerLogin
	loginFlows map[string]providerLoginFlow
}

type providerLogin struct {
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
	VerificationURL string `json:"verification_url,omitempty"`
	UserCode        string `json:"user_code,omitempty"`
}

type providerLoginFlow struct {
	Authorization provider.PKCEAuthorization
	Provider      config.ProviderConfig
	StateDir      string
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
	r.mu.Lock()
	r.handler = handler
	r.mu.Unlock()
}

func buildHandlerWithRuntime(cfg *config.Config, configPath string, runtime *handlerRuntime) (http.Handler, error) {
	var authenticator api.ClientAuthenticator = api.NoAuthAuthenticator{}
	if cfg.Server.Authentication != nil {
		apiKey, err := cfg.Server.Authentication.APIKey.Resolve()
		if err != nil {
			return nil, fmt.Errorf("resolve server API key: %w", err)
		}
		configured, err := api.NewStaticBearerAuthenticator(apiKey)
		if err != nil {
			return nil, err
		}
		authenticator = configured
	}

	providers := make([]provider.OpenAIProvider, 0, len(cfg.Providers))
	for _, providerConfig := range cfg.Providers {
		upstream, err := buildProviderWithState(providerConfig, cfg.State.Directory)
		if err != nil {
			return nil, fmt.Errorf("initialize provider %q: %w", providerConfig.ID, err)
		}
		providers = append(providers, upstream)
	}
	overrides, err := loadCatalogOverrides(cfg.Router.CatalogFile)
	if err != nil {
		return nil, err
	}
	openAI, err := api.NewOpenAIHandlerWithOverrides(providers, overrides)
	if err != nil {
		return nil, err
	}
	if err := openAI.RefreshCatalog(context.Background()); err != nil {
		return nil, fmt.Errorf("initialize model catalog: %w", err)
	}
	var configuredClassifier classifier.Classifier
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
			configuredClassifier, err = classifier.NewJevTransport(decisionProvider, cfg.Router.Classifier.Model)
			if err != nil {
				return nil, fmt.Errorf("initialize Jev classifier: %w", err)
			}
		} else {
			apiKey, resolveErr := cfg.Router.Classifier.APIKey.Resolve()
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve Jev API key: %w", resolveErr)
			}
			configuredClassifier, err = classifier.NewJev(cfg.Router.Classifier.BaseURL, apiKey, cfg.Router.Classifier.Model, nil)
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
		openAI.SetClassifier(classifier.Runner{Classifier: configuredClassifier, Timeout: 10 * time.Second, OnFailure: failure})
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
	usageStore, usageErr := usage.Open(cfg.State.Directory)
	if usageErr != nil {
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
		mux.HandleFunc("GET /gateway/v1/config", runtime.getConfig)
		mux.HandleFunc("PUT /gateway/v1/config", runtime.putConfig)
		mux.HandleFunc("POST /gateway/v1/config", runtime.putConfig)
		mux.HandleFunc("POST /gateway/v1/providers/test", runtime.testProvider)
		mux.HandleFunc("POST /gateway/v1/providers/login", runtime.startProviderLogin)
		mux.HandleFunc("GET /gateway/v1/providers/login", runtime.providerLoginStatus)
		mux.HandleFunc("GET /gateway/v1/providers/login/callback", runtime.providerLoginCallback)
		mux.HandleFunc("GET /auth/callback", runtime.providerLoginCallback)
	}
	mux.Handle("/gateway/v1/", apiHandler)
	mux.Handle("/", web.Handler())
	return api.RequestIDMiddleware(nil, mux), nil
}

func (r *handlerRuntime) getConfig(w http.ResponseWriter, _ *http.Request) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.handler == nil {
		http.Error(w, "gateway is starting", http.StatusServiceUnavailable)
		return
	}
	cfg, err := config.Load(r.configPath)
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	if cfg.Server.Authentication != nil {
		if secret, resolveErr := cfg.Server.Authentication.APIKey.Resolve(); resolveErr == nil {
			cfg.Server.Authentication.APIKey = config.SecretRef{Value: secret}
		}
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(document)
}

func (r *handlerRuntime) putConfig(w http.ResponseWriter, req *http.Request) {
	if req.ContentLength > 2<<20 {
		writeConfigError(w, http.StatusRequestEntityTooLarge, errors.New("configuration exceeds 2 MiB"))
		return
	}
	decoder := yaml.NewDecoder(io.LimitReader(req.Body, 2<<20))
	decoder.KnownFields(true)
	var next config.Config
	if err := decoder.Decode(&next); err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("decode configuration: %w", err))
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			writeConfigError(w, http.StatusBadRequest, errors.New("multiple configuration documents are not allowed"))
		} else {
			writeConfigError(w, http.StatusBadRequest, fmt.Errorf("decode configuration: %w", err))
		}
		return
	}
	if err := next.Validate(); err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("validate configuration: %w", err))
		return
	}
	newHandler, err := buildHandlerWithRuntime(&next, r.configPath, r)
	if err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("initialize configuration: %w", err))
		return
	}
	if err := persistConfig(r.configPath, &next); err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	r.swap(newHandler)
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"status":"saved","reloaded":true}`+"\n")
}

func (r *handlerRuntime) testProvider(w http.ResponseWriter, req *http.Request) {
	decoder := yaml.NewDecoder(io.LimitReader(req.Body, 1<<20))
	decoder.KnownFields(true)
	var request struct {
		Provider config.ProviderConfig `yaml:"provider"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("decode provider: %w", err))
		return
	}
	if request.Provider.ID == "" {
		writeConfigError(w, http.StatusBadRequest, errors.New("provider.id is required"))
		return
	}
	current, err := config.Load(r.configPath)
	if err != nil {
		writeConfigError(w, http.StatusInternalServerError, fmt.Errorf("load current configuration: %w", err))
		return
	}
	if request.Provider.Type != "ollama" {
		discover := true
		request.Provider.DiscoverModels = &discover
	}
	upstream, err := buildProviderWithState(request.Provider, current.State.Directory)
	if err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("initialize provider: %w", err))
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()
	models, err := upstream.Models(ctx)
	if err != nil {
		writeConfigError(w, http.StatusBadGateway, fmt.Errorf("test provider connection: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "models": models})
}

func (r *handlerRuntime) startProviderLogin(w http.ResponseWriter, req *http.Request) {
	var request struct {
		Provider   yaml.Node `json:"provider" yaml:"provider"`
		ProviderID string    `json:"provider_id" yaml:"provider_id"`
	}
	if err := yaml.NewDecoder(io.LimitReader(req.Body, 64<<10)).Decode(&request); err != nil {
		writeConfigError(w, http.StatusBadRequest, fmt.Errorf("decode login request: %w", err))
		return
	}
	var providerConfig config.ProviderConfig
	providerID := request.ProviderID
	if request.Provider.Kind == yaml.ScalarNode {
		providerID = request.Provider.Value
	} else if request.Provider.Kind != 0 {
		if err := request.Provider.Decode(&providerConfig); err != nil {
			writeConfigError(w, http.StatusBadRequest, fmt.Errorf("decode provider: %w", err))
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
	current, err := config.Load(r.configPath)
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
		if providerConfig.BaseURL == "" {
			providerConfig.BaseURL = "https://chatgpt.com/backend-api/codex"
		}
		if len(providerConfig.Models) == 0 {
			providerConfig.Models = []string{"gpt-5.5"}
		}
	}
	if providerConfig.Type != "openai-codex" && providerConfig.Type != "nous" {
		writeConfigError(w, http.StatusBadRequest, errors.New("browser login is supported only for openai-codex and nous providers"))
		return
	}
	if providerConfig.Type == "openai-codex" {
		authorization, startErr := provider.StartPKCE("https://auth.openai.com", providerLoginRedirectURL(req))
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
		r.loginFlows[authorization.State] = providerLoginFlow{Authorization: authorization, Provider: providerConfig, StateDir: current.State.Directory}
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
	go r.completeProviderLogin(providerConfig, current.State.Directory)
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
		store, storeErr := state.NewFlatFileCredentialStore(flow.StateDir)
		if storeErr != nil {
			err = storeErr
		} else {
			err = store.Save(flow.Provider.ID, &state.Credential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt, Endpoint: flow.Provider.BaseURL, AccountID: credential.AccountID, TokenType: "Bearer"})
		}
	}
	if err != nil {
		r.setProviderLogin(flow.Provider.ID, providerLogin{Status: "failure", Message: err.Error()})
		http.Error(w, "Authorization failed. You may close this window.", http.StatusBadGateway)
		return
	}
	r.setProviderLogin(flow.Provider.ID, providerLogin{Status: "success", Message: "Provider login complete"})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<html><body><h1>Torii Gateway login complete</h1><p>You may close this window and return to the console.</p></body></html>")
}

func (r *handlerRuntime) completeProviderLogin(providerConfig config.ProviderConfig, stateDirectory string) {
	store, err := state.NewFlatFileCredentialStore(stateDirectory)
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
	r.setProviderLogin(providerConfig.ID, providerLogin{Status: "success", Message: "Provider login complete"})
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
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

type providerQuotaFetcher struct{ providers map[string]quota.Fetcher }

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
	return buildProviderWithState(cfg, "")
}

func buildProviderWithState(cfg config.ProviderConfig, stateDirectory string) (provider.OpenAIProvider, error) {
	if cfg.Type == "ollama" {
		return provider.NewOllama(cfg.ID, cfg.BaseURL, &http.Client{})
	}
	if cfg.Type == "nous" {
		if stateDirectory == "" {
			return nil, errors.New("state directory is required for nous")
		}
		credentialStore, err := state.NewFlatFileCredentialStore(stateDirectory)
		if err != nil {
			return nil, err
		}
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
		return provider.NewNous(cfg.ID, cfg.BaseURL, cfg.PortalURL, cfg.ClientID, cfg.Models, discover, loader, saver, &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
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
		return provider.NewAnthropic(cfg.ID, cfg.BaseURL, key, cfg.Models, discover, &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
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
		if stateDirectory == "" {
			return nil, errors.New("state directory is required for openai-codex")
		}
		credentialStore, err := state.NewFlatFileCredentialStore(stateDirectory)
		if err != nil {
			return nil, err
		}
		loader := func(id string) (provider.CodexCredential, error) {
			credential, err := credentialStore.Load(id)
			if err != nil {
				return provider.CodexCredential{}, err
			}
			return provider.CodexCredential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, Endpoint: credential.Endpoint, ExpiresAt: credential.ExpiresAt, AccountID: credential.AccountID, ClientID: credential.ClientID, IDToken: credential.IDToken, Issuer: credential.Issuer, Subject: credential.Subject, Email: credential.Email, HostID: credential.HostID, Scopes: credential.Scopes}, nil
		}
		codex, err := provider.NewCodex(cfg.ID, cfg.BaseURL, cfg.Models, loader, &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
		if err != nil {
			return nil, err
		}
		saver := func(id string, value provider.CodexCredential) error {
			return credentialStore.Save(id, &state.Credential{AccessToken: value.AccessToken, RefreshToken: value.RefreshToken, ExpiresAt: value.ExpiresAt, Endpoint: value.Endpoint, AccountID: value.AccountID, ClientID: value.ClientID, IDToken: value.IDToken, Issuer: value.Issuer, Subject: value.Subject, Email: value.Email, HostID: value.HostID, Scopes: value.Scopes, TokenType: "Bearer"})
		}
		codex.ConfigureRefresh("https://auth.openai.com", saver)
		return codex, nil
	}
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
	client := &http.Client{Transport: httpTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
		default:
			apiPath = "/v1"
		}
	}
	var quotaURL string
	quotaHeaders := make(http.Header)
	if cfg.Type == "openrouter" && cfg.ManagementToken != nil {
		managementToken, err := cfg.ManagementToken.Resolve()
		if err != nil {
			return nil, fmt.Errorf("resolve OpenRouter management token: %w", err)
		}
		if managementToken == "" || strings.ContainsAny(managementToken, "\r\n") {
			return nil, errors.New("resolved OpenRouter management token is empty or contains a newline")
		}
		quotaURL = strings.TrimRight(cfg.BaseURL, "/") + strings.TrimRight(apiPath, "/") + "/credits"
		quotaHeaders.Set("Authorization", "Bearer "+managementToken)
	}
	return provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: cfg.ID, BaseURL: cfg.BaseURL, APIPath: apiPath, Client: client, Headers: headers, QuotaURL: quotaURL, QuotaHeaders: quotaHeaders, DiscoverModels: discover, StaticModels: cfg.Models, ModelRewrites: cfg.ModelRewrites, DiscoveryTimeout: discoveryTimeout, RequestTimeout: requestTimeout})
}
