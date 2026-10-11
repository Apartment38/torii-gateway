package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
)

// Missing source credentials fail only that source, not gateway startup or
// other adapters. Secret references remain the existing configuration type.
type unavailableBenchmarkSource struct{ id string }

func (s unavailableBenchmarkSource) ID() string { return s.id }
func (s unavailableBenchmarkSource) Fetch(context.Context) (intelligence.Result, error) {
	return intelligence.Result{}, &intelligence.SourceError{Stage: "source credentials or initialization"}
}

func (h *catalogHandler) startIntelligence(ctx context.Context) {
	h.workerMu.Lock()
	defer h.workerMu.Unlock()
	if h.intelligence == nil && h.pollQuotas == nil || h.workerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	h.workerCancel = cancel
	h.workerDone = make(chan struct{})
	go func() {
		defer close(h.workerDone)
		var wg sync.WaitGroup
		if h.intelligence != nil {
			wg.Add(1)
			go func() { defer wg.Done(); h.intelligence.Run(ctx) }()
		}
		if h.pollQuotas != nil {
			interval := h.quotaInterval
			if interval <= 0 {
				interval = time.Minute
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				h.pollQuotas(ctx)
				select {
				case <-ctx.Done():
					wg.Wait()
					return
				case <-ticker.C:
				}
			}
		}
		wg.Wait()
	}()
}
func (h *catalogHandler) stopIntelligence() {
	h.workerMu.Lock()
	defer h.workerMu.Unlock()
	if h.workerCancel != nil {
		h.workerCancel()
	}
	if h.intelligence != nil {
		h.intelligence.Close()
	}
	if h.workerCancel != nil {
		<-h.workerDone
		h.workerCancel = nil
	}
}
func (r *handlerRuntime) stopIntelligence() {
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()
	if h, ok := handler.(*catalogHandler); ok {
		h.stopIntelligence()
	}
}

func buildIntelligence(cfg *config.Config, models []catalog.Descriptor) (*intelligence.Manager, error) {
	if !cfg.Intelligence.Enabled {
		return nil, nil
	}
	store, err := intelligence.OpenStore(context.Background(), cfg.Credentials, nil)
	if err != nil {
		return nil, err
	}
	timeout, _ := time.ParseDuration(cfg.Intelligence.HTTPTimeout)
	requestInterval, _ := time.ParseDuration(cfg.Intelligence.RequestInterval)
	retries := 2
	if cfg.Intelligence.Retries != nil {
		retries = *cfg.Intelligence.Retries
	}
	registry := intelligence.Builtins()
	var sources []intelligence.Source
	for _, settings := range cfg.Intelligence.Sources {
		if !settings.Enabled {
			continue
		}
		key := ""
		var resolveKey func() (string, error)
		if settings.APIKey != nil {
			value, err := settings.APIKey.Resolve()
			if err != nil {
				sources = append(sources, unavailableBenchmarkSource{id: settings.ID})
				continue
			}
			key = value
			if cfg.Credentials != nil {
				key = "database-managed"
				resolveKey = settings.APIKey.Resolve
			}
		}
		source, err := registry.New(settings.SourceSettings, key, intelligence.FetchOptions{Timeout: timeout, Retries: retries, ResolveKey: resolveKey, RequestInterval: requestInterval})
		if err != nil {
			sources = append(sources, unavailableBenchmarkSource{id: settings.ID})
			continue
		}
		sources = append(sources, source)
	}
	manager, err := intelligence.NewManager(store, sources, models, cfg.Intelligence.Options())
	if err != nil {
		store.Close()
	}
	return manager, err
}

// The CLI refreshes the running gateway rather than running a second writer
// against its cache. This shares authentication and refresh serialization.
func runBenchmarkCommand(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 || args[0] != "refresh" {
		return errors.New("usage: gateway benchmarks refresh --database <path> [--url <gateway-origin>]")
	}
	flags := flag.NewFlagSet("benchmarks refresh", flag.ContinueOnError)
	path := flags.String("database", "", "SQLite database path")
	origin := flags.String("url", "", "running gateway origin")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected benchmark command arguments")
	}
	db, err := openSettingsDatabase(*path)
	if err != nil {
		return err
	}
	cfg, err := loadDatabaseSettings(db)
	if err != nil {
		db.Close()
		return err
	}
	_ = cfg
	defer db.Close()
	if *origin == "" {
		host, port, _ := net.SplitHostPort(cfg.Server.Listen)
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		*origin = "http://" + net.JoinHostPort(host, port)
	}
	if err := intelligence.ValidateURL(*origin); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(*origin, "/")+"/gateway/v1/intelligence/refresh", nil)
	if err != nil {
		return err
	}
	key, err := db.GatewayAPIKey()
	if err != nil {
		return errors.New("create an administrator to initialize the gateway API key")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := http.Client{Timeout: cfg.Intelligence.Options().SourceTimeout + time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("benchmark refresh request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("benchmark refresh rejected by gateway; check URL, authentication and whether intelligence is enabled")
	}
	_, err = io.Copy(output, io.LimitReader(resp.Body, 4<<20))
	return err
}
