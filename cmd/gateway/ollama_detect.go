package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

// Probe only local/host endpoints and configured Ollama ports, never a LAN scan.
func (r *handlerRuntime) detectOllama(w http.ResponseWriter, req *http.Request) {
	var input struct {
		Port int `json:"port"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Port < 0 || input.Port > 65535 {
		writeConfigError(w, 400, errors.New("port must be between 1 and 65535, or omitted for detection"))
		return
	}
	ports := []int{}
	if input.Port > 0 {
		ports = append(ports, input.Port)
	} else {
		if cfg, err := r.loadConfig(); err == nil {
			for _, p := range cfg.Providers {
				if p.Type == "ollama" {
					u, e := url.Parse(p.BaseURL)
					if e == nil {
						if port, e := strconv.Atoi(u.Port()); e == nil {
							ports = append(ports, port)
						}
					}
				}
			}
		}
		if u, e := url.Parse(os.Getenv("OLLAMA_HOST")); e == nil {
			if port, e := strconv.Atoi(u.Port()); e == nil {
				ports = append(ports, port)
			}
		}
		ports = append(ports, 11434)
	}
	hosts := []string{"127.0.0.1", "host.containers.internal", "host.docker.internal"}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		hosts = []string{"host.containers.internal", "host.docker.internal", "127.0.0.1"}
	}
	ctx, cancel := context.WithTimeout(req.Context(), 8*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for _, port := range ports {
		for _, host := range hosts {
			base := "http://" + net.JoinHostPort(host, strconv.Itoa(port))
			if seen[base] {
				continue
			}
			seen[base] = true
			upstream, err := provider.NewOllama("ollama", base, &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
			if err != nil {
				continue
			}
			models, err := upstream.Models(ctx)
			if err == nil {
				writeJSON(w, 200, map[string]any{"base_url": base, "port": port, "model_count": len(models)})
				return
			}
		}
	}
	writeConfigError(w, 422, errors.New("Ollama was not found on the gateway host. Enter its listening port, or configure a remote API URL in advanced options."))
}
