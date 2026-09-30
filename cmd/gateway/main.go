// Command gateway is the entry point for the Torii Router gateway.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

	"github.com/Apartment38/torii-router/internal/api"
	"github.com/Apartment38/torii-router/internal/config"
	"github.com/Apartment38/torii-router/internal/provider"
	"github.com/Apartment38/torii-router/internal/state"
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
	store, err := state.NewFlatFileCredentialStore(cfg.State.Directory)
	if err != nil {
		return err
	}
	switch action {
	case "login":
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
	if cfg.Server.Authentication == nil {
		return nil, errors.New("server.authentication is required to serve the API")
	}
	apiKey, err := cfg.Server.Authentication.APIKey.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve server API key: %w", err)
	}
	authenticator, err := api.NewStaticBearerAuthenticator(apiKey)
	if err != nil {
		return nil, err
	}

	providers := make([]provider.OpenAIProvider, 0, len(cfg.Providers))
	for _, providerConfig := range cfg.Providers {
		upstream, err := buildProvider(providerConfig)
		if err != nil {
			return nil, fmt.Errorf("initialize provider %q: %w", providerConfig.ID, err)
		}
		providers = append(providers, upstream)
	}
	openAI, err := api.NewOpenAIHandler(providers)
	if err != nil {
		return nil, err
	}
	if err := openAI.RefreshCatalog(context.Background()); err != nil {
		return nil, fmt.Errorf("initialize model catalog: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.Handle("/", api.AuthenticateMiddleware(authenticator, openAI))
	return api.RequestIDMiddleware(nil, mux), nil
}

func buildProvider(cfg config.ProviderConfig) (provider.OpenAIProvider, error) {
	if cfg.Type == "ollama" {
		return provider.NewOllama(cfg.ID, cfg.BaseURL, &http.Client{})
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
	return provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{ID: cfg.ID, BaseURL: cfg.BaseURL, Client: client, Headers: headers, DiscoverModels: discover, StaticModels: cfg.Models, ModelRewrites: cfg.ModelRewrites, DiscoveryTimeout: discoveryTimeout, RequestTimeout: requestTimeout})
}
