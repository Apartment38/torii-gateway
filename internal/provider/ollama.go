package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ollamaDiscoveryTimeout = 10 * time.Second

// Ollama provides the model-discovery portion of an OpenAI-compatible Ollama
// endpoint. Inference requests use their caller's context and are deliberately
// not subject to the shorter discovery timeout.
type Ollama struct {
	baseURL          *url.URL
	client           *http.Client
	discoveryTimeout time.Duration
}

// NewOllama creates an Ollama model-discovery client.
func NewOllama(baseURL *url.URL, client *http.Client) (*Ollama, error) {
	if baseURL == nil {
		return nil, errors.New("ollama base URL is required")
	}
	if client == nil {
		return nil, errors.New("ollama HTTP client is required")
	}
	baseCopy := *baseURL
	return &Ollama{
		baseURL:          &baseCopy,
		client:           client,
		discoveryTimeout: ollamaDiscoveryTimeout,
	}, nil
}

// Models returns the raw model objects advertised by Ollama's OpenAI-compatible
// model catalog. Keeping each object raw avoids discarding provider extensions.
func (o *Ollama) Models(ctx context.Context) ([]json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, o.discoveryTimeout)
	defer cancel()

	endpoint := appendURLPath(o.baseURL, "/v1/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Ollama model request: %w", err)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("retrieve Ollama models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("retrieve Ollama models: upstream returned %s", resp.Status)
	}

	var catalog struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode Ollama model catalog: %w", err)
	}
	if len(catalog.Data) == 0 || bytes.Equal(bytes.TrimSpace(catalog.Data), []byte("null")) {
		return nil, errors.New("decode Ollama model catalog: data must be an array")
	}
	var models []json.RawMessage
	if err := json.Unmarshal(catalog.Data, &models); err != nil {
		return nil, fmt.Errorf("decode Ollama model catalog data: %w", err)
	}
	return models, nil
}

// appendURLPath appends an escaped path without allowing url.URL.Path and
// RawPath to disagree. In particular, an escaped terminal slash (%2F) is data,
// not the separator that should be trimmed before joining.
func appendURLPath(base *url.URL, suffix string) *url.URL {
	joined := *base
	suffix = "/" + strings.TrimLeft(suffix, "/")

	if joined.RawPath == "" {
		joined.Path = strings.TrimSuffix(joined.Path, "/") + suffix
		return &joined
	}

	if strings.HasSuffix(joined.RawPath, "/") {
		joined.RawPath = strings.TrimSuffix(joined.RawPath, "/")
		joined.Path = strings.TrimSuffix(joined.Path, "/")
	}
	joined.Path += suffix
	joined.RawPath += suffix
	return &joined
}
