// Command gateway is the entry point for the Torii Router gateway.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Apartment38/torii-router/internal/api"
	"github.com/Apartment38/torii-router/internal/config"
	"github.com/Apartment38/torii-router/internal/provider"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Printf("gateway: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: gateway serve --config <path>")
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

	providers := make([]provider.ChatCompletionsProvider, 0, len(cfg.Providers))
	for _, providerConfig := range cfg.Providers {
		upstream, err := provider.NewOllama(providerConfig.ID, providerConfig.BaseURL, &http.Client{})
		if err != nil {
			return nil, fmt.Errorf("initialize provider %q: %w", providerConfig.ID, err)
		}
		providers = append(providers, upstream)
	}
	openAI, err := api.NewOpenAIHandler(providers)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.Handle("/", api.AuthenticateMiddleware(authenticator, openAI))
	return mux, nil
}
