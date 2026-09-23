package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Apartment38/torii-router/internal/transport"
)

const modelDiscoveryTimeout = 10 * time.Second

// Ollama implements model discovery and non-streaming inference for an Ollama
// server. Inference uses Ollama's OpenAI-compatible API.
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
	compatible, err := transport.NewOpenAICompatible(baseURL, client)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(baseURL)
	if client == nil {
		client = http.DefaultClient
	}
	return &Ollama{id: id, baseURL: u, client: client, transport: compatible}, nil
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
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode Ollama models: %w", err)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, errors.New("decode Ollama models: data must be an array")
	}
	var catalog []Model
	if err := json.Unmarshal(envelope.Data, &catalog); err != nil || catalog == nil {
		return nil, errors.New("decode Ollama models: data must be an array")
	}
	models := make([]Model, 0, len(catalog))
	for _, model := range catalog {
		if model.ID == "" {
			return nil, errors.New("decode Ollama models: model has empty ID")
		}
		model.ID = o.id + "/" + model.ID
		if model.Object == "" {
			model.Object = "model"
		}
		if model.OwnedBy == "" {
			model.OwnedBy = o.id
		}
		models = append(models, model)
	}
	return models, nil
}

func (o *Ollama) ChatCompletions(ctx context.Context, body []byte, model string) (*http.Response, error) {
	return o.transport.Do(ctx, "/v1/chat/completions", body, model)
}
