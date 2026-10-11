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
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/transport"
)

const modelDiscoveryTimeout = 10 * time.Second

// DefaultOllamaRequestTimeout allows cold model loading and response generation,
// including consuming a streaming response. Discovery has its own shorter limit.
const DefaultOllamaRequestTimeout = 5 * time.Minute

const maxModelCatalogSize = 16 << 20

// Ollama implements model discovery and inference for an Ollama server.
// Inference uses Ollama's OpenAI-compatible API, including SSE streaming.
type Ollama struct {
	id        string
	baseURL   *url.URL
	client    *http.Client
	transport *transport.OpenAICompatible
}

func NewOllama(id, baseURL string, client *http.Client) (*Ollama, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	// Do not change a caller's shared client when applying the inference default.
	if client.Timeout == 0 {
		copy := *client
		copy.Timeout = DefaultOllamaRequestTimeout
		client = &copy
	}
	compatible, err := transport.NewOpenAICompatible(baseURL, client)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(baseURL)
	// Discovery uses its own context deadline, even if inference is configured
	// with a shorter timeout. Both clients share the same connection transport.
	discoveryClient := *client
	discoveryClient.Timeout = 0
	return &Ollama{id: id, baseURL: u, client: &discoveryClient, transport: compatible}, nil
}

func (o *Ollama) ID() string { return o.id }

func (o *Ollama) Models(ctx context.Context) ([]Model, error) {
	if o == nil || o.baseURL == nil || o.client == nil {
		return nil, errors.New("Ollama provider is not initialized")
	}
	u := *o.baseURL
	escapedPath := strings.TrimRight(u.EscapedPath(), "/") + "/v1/models"
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		return nil, fmt.Errorf("construct Ollama model URL: %w", err)
	}
	u.Path, u.RawPath = path, escapedPath
	discoveryCtx, cancel := context.WithTimeout(ctx, modelDiscoveryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(discoveryCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Ollama model request: %w", err)
	}
	response, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover Ollama models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("discover Ollama models: upstream returned %s", response.Status)
	}
	limitedBody := &io.LimitedReader{R: response.Body, N: maxModelCatalogSize + 1}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(limitedBody)
	if err := decoder.Decode(&envelope); err != nil {
		if limitedBody.N == 0 {
			return nil, fmt.Errorf("decode Ollama models: response exceeds %d bytes", maxModelCatalogSize)
		}
		return nil, fmt.Errorf("decode Ollama models: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if limitedBody.N == 0 {
			return nil, fmt.Errorf("decode Ollama models: response exceeds %d bytes", maxModelCatalogSize)
		}
		if err == nil {
			return nil, errors.New("decode Ollama models: response must contain one JSON value")
		}
		return nil, fmt.Errorf("decode Ollama models: trailing data: %w", err)
	}
	if limitedBody.N == 0 {
		return nil, fmt.Errorf("decode Ollama models: response exceeds %d bytes", maxModelCatalogSize)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, errors.New("decode Ollama models: data must be an array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(envelope.Data, &entries); err != nil || entries == nil {
		return nil, errors.New("decode Ollama models: data must be an array")
	}
	models := make([]Model, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		trimmed := bytes.TrimSpace(entry)
		if len(trimmed) < 2 || trimmed[0] != '{' {
			return nil, errors.New("decode Ollama models: each model must be an object")
		}
		var model Model
		if err := json.Unmarshal(entry, &model); err != nil {
			return nil, fmt.Errorf("decode Ollama models: invalid model object: %w", err)
		}
		if model.ID == "" {
			return nil, errors.New("decode Ollama models: model has empty ID")
		}
		model.ID = o.id + "/" + model.ID
		// Ollama can list a loaded model and its installed tag separately.
		// They share the same inference ID and must occupy one catalog entry.
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		if model.Object == "" {
			model.Object = "model"
		}
		if model.OwnedBy == "" {
			model.OwnedBy = o.id
		}
		models = append(models, model)
	}
	// /v1/models supplies names only. Native show metadata verifies each
	// model's actual capabilities, including custom names and cloud aliases.
	// Failure enriches nothing; it never promotes an unknown capability.
	var wg sync.WaitGroup
	jobs := make(chan int)
	for range min(4, len(models)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				o.enrichModel(discoveryCtx, &models[i])
			}
		}()
	}
discover:
	for i := range models {
		select {
		case jobs <- i:
		case <-discoveryCtx.Done():
			break discover
		}
	}
	close(jobs)
	wg.Wait()
	return models, nil
}

func (o *Ollama) Forward(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	switch endpoint {
	case "/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/responses":
		return o.transport.Do(ctx, endpoint, body, model)
	default:
		return nil, fmt.Errorf("unsupported Ollama endpoint %q", endpoint)
	}
}
