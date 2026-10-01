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
	"os"
	"os/signal"
	"strings"
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
	handler, err := buildHandler(cfg)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", cfg.Server.Listen, err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           handler,
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
	mux.Handle("/gateway/v1/", apiHandler)
	mux.Handle("/", web.Handler())
	return api.RequestIDMiddleware(nil, mux), nil
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
			return provider.CodexCredential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, Endpoint: credential.Endpoint, ExpiresAt: credential.ExpiresAt, AccountID: credential.AccountID}, nil
		}
		codex, err := provider.NewCodex(cfg.ID, cfg.BaseURL, cfg.Models, loader, &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
		if err != nil {
			return nil, err
		}
		saver := func(id string, value provider.CodexCredential) error {
			return credentialStore.Save(id, &state.Credential{AccessToken: value.AccessToken, RefreshToken: value.RefreshToken, ExpiresAt: value.ExpiresAt, Endpoint: value.Endpoint, AccountID: value.AccountID, TokenType: "Bearer"})
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
