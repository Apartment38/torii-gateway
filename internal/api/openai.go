package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Apartment38/torii-router/internal/provider"
)

const maxRequestBody = 32 << 20

// OpenAIHandler serves the currently implemented OpenAI-compatible endpoints.
type OpenAIHandler struct {
	providers []provider.ChatCompletionsProvider
	byID      map[string]provider.ChatCompletionsProvider
}

func NewOpenAIHandler(providers []provider.ChatCompletionsProvider) (*OpenAIHandler, error) {
	registered := make(map[string]provider.ChatCompletionsProvider, len(providers))
	for _, p := range providers {
		if p == nil {
			return nil, errors.New("register provider: nil provider")
		}
		if _, exists := registered[p.ID()]; exists {
			return nil, fmt.Errorf("register provider: duplicate ID %q", p.ID())
		}
		registered[p.ID()] = p
	}
	return &OpenAIHandler{providers: append([]provider.ChatCompletionsProvider(nil), providers...), byID: registered}, nil
}

func (h *OpenAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		h.listModels(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/models/"):
		h.getModel(w, r, strings.TrimPrefix(r.URL.Path, "/v1/models/"))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		h.chatCompletions(w, r)
	default:
		writeAPIError(w, http.StatusNotFound, "Not found", "invalid_request_error", "not_found")
	}
}

func (h *OpenAIHandler) listModels(w http.ResponseWriter, r *http.Request) {
	models := make([]provider.Model, 0)
	for _, p := range h.providers {
		discovered, err := p.Models(r.Context())
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "Failed to discover upstream models", "api_error", "upstream_error")
			return
		}
		models = append(models, discovered...)
	}
	writeJSON(w, http.StatusOK, struct {
		Object string           `json:"object"`
		Data   []provider.Model `json:"data"`
	}{Object: "list", Data: models})
}

func (h *OpenAIHandler) getModel(w http.ResponseWriter, r *http.Request, publicID string) {
	providerID, _, err := provider.ParseModelID(publicID)
	if err != nil || h.byID[providerID] == nil {
		writeAPIError(w, http.StatusNotFound, "The model does not exist", "invalid_request_error", "model_not_found")
		return
	}
	models, err := h.byID[providerID].Models(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "Failed to discover upstream models", "api_error", "upstream_error")
		return
	}
	for _, model := range models {
		if model.ID == publicID {
			writeJSON(w, http.StatusOK, model)
			return
		}
	}
	writeAPIError(w, http.StatusNotFound, "The model does not exist", "invalid_request_error", "model_not_found")
}

func (h *OpenAIHandler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error", "invalid_json")
		return
	}
	var fields struct {
		Model  string          `json:"model"`
		Stream json.RawMessage `json:"stream"`
	}
	if err := json.Unmarshal(body, &fields); err != nil || fields.Model == "" {
		writeAPIError(w, http.StatusBadRequest, "A valid model is required", "invalid_request_error", "invalid_model")
		return
	}
	if len(fields.Stream) != 0 {
		var stream bool
		if err := json.Unmarshal(fields.Stream, &stream); err != nil {
			writeAPIError(w, http.StatusBadRequest, "stream must be a boolean", "invalid_request_error", "invalid_stream")
			return
		}
		if stream {
			writeAPIError(w, http.StatusBadRequest, "Streaming is not supported yet", "invalid_request_error", "unsupported_streaming")
			return
		}
	}
	providerID, modelID, err := provider.ParseModelID(fields.Model)
	upstream := h.byID[providerID]
	if err != nil || upstream == nil {
		writeAPIError(w, http.StatusNotFound, "The model does not exist", "invalid_request_error", "model_not_found")
		return
	}
	response, err := upstream.ChatCompletions(r.Context(), body, modelID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "Upstream request failed", "api_error", "upstream_error")
		return
	}
	defer response.Body.Close()
	copyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		switch http.CanonicalHeaderKey(key) {
		case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Content-Length":
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message, errorType, code string) {
	writeJSON(w, status, struct {
		Error struct {
			Message string  `json:"message"`
			Type    string  `json:"type"`
			Param   *string `json:"param"`
			Code    string  `json:"code"`
		} `json:"error"`
	}{Error: struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Param   *string `json:"param"`
		Code    string  `json:"code"`
	}{Message: message, Type: errorType, Code: code}})
}
