package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Apartment38/torii-router/internal/provider"
	"github.com/Apartment38/torii-router/internal/transport"
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
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "Request body exceeds the maximum allowed size", "invalid_request_error", "request_too_large")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error", "invalid_json")
		return
	}
	fields, err := transport.InspectRequest(body)
	if err != nil {
		if errors.Is(err, transport.ErrInvalidStream) {
			writeAPIError(w, http.StatusBadRequest, "stream must be a boolean", "invalid_request_error", "invalid_stream")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "A valid model is required", "invalid_request_error", "invalid_model")
		return
	}
	if fields.Model == "" {
		writeAPIError(w, http.StatusBadRequest, "A valid model is required", "invalid_request_error", "invalid_model")
		return
	}
	providerID, modelID, err := provider.ParseModelID(fields.Model)
	upstream := h.byID[providerID]
	if err != nil || upstream == nil {
		writeAPIError(w, http.StatusNotFound, "The model does not exist", "invalid_request_error", "model_not_found")
		return
	}
	response, err := upstream.ChatCompletions(r.Context(), body, modelID)
	if err != nil {
		if errors.Is(err, transport.ErrInvalidRequest) {
			writeAPIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error", "invalid_request")
			return
		}
		writeAPIError(w, http.StatusBadGateway, "Upstream request failed", "api_error", "upstream_error")
		return
	}
	defer response.Body.Close()
	CopyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	if fields.Stream {
		_ = relayStream(w, response.Body)
		return
	}
	_, _ = io.Copy(w, response.Body)
}

// relayStream copies upstream bytes as they arrive and flushes every write.
// It deliberately does not parse SSE, preserving event data (including unknown
// fields and tool-call deltas) byte-for-byte and without buffering a response.
func relayStream(w http.ResponseWriter, src io.Reader) error {
	controller := http.NewResponseController(w)
	_, err := io.Copy(flushingWriter{writer: w, flush: controller.Flush}, src)
	return err
}

type flushingWriter struct {
	writer io.Writer
	flush  func() error
}

func (w flushingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err != nil {
		return n, err
	}
	if err := w.flush(); err != nil {
		return n, err
	}
	return n, nil
}

var hopByHopResponseHeaders = map[string]struct{}{
	"connection":          {},
	"content-length":      {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

// CopyResponseHeaders copies end-to-end upstream headers to a downstream
// response. Headers named by Connection are hop-by-hop as well as the standard
// and de facto static hop-by-hop names. Content-Length is omitted because the
// HTTP server computes it from the relayed response body.
func CopyResponseHeaders(dst, src http.Header) {
	excluded := make(map[string]struct{}, len(hopByHopResponseHeaders))
	for name := range hopByHopResponseHeaders {
		excluded[name] = struct{}{}
	}
	for _, connection := range src.Values("Connection") {
		for name := range strings.SplitSeq(connection, ",") {
			excluded[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
	}
	for name, values := range src {
		if _, skip := excluded[strings.ToLower(name)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
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
