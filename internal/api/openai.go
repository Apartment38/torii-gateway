package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Apartment38/torii-router/internal/catalog"
	"github.com/Apartment38/torii-router/internal/fault"
	"github.com/Apartment38/torii-router/internal/provider"
	"github.com/Apartment38/torii-router/internal/requestid"
	"github.com/Apartment38/torii-router/internal/transport"
)

const maxRequestBody = 32 << 20

// OpenAIHandler serves the currently implemented OpenAI-compatible endpoints.
type OpenAIHandler struct {
	byID      map[string]provider.OpenAIProvider
	catalog   *catalog.Catalog
	refreshMu sync.Mutex
	ready     bool
}

func NewOpenAIHandler(providers []provider.OpenAIProvider) (*OpenAIHandler, error) {
	registered := make(map[string]provider.OpenAIProvider, len(providers))
	for _, p := range providers {
		if p == nil {
			return nil, errors.New("register provider: nil provider")
		}
		if _, exists := registered[p.ID()]; exists {
			return nil, fmt.Errorf("register provider: duplicate ID %q", p.ID())
		}
		registered[p.ID()] = p
	}
	discovery := make([]provider.Provider, len(providers))
	for i := range providers {
		discovery[i] = providers[i]
	}
	modelCatalog, err := catalog.New(discovery, nil, nil)
	if err != nil {
		return nil, err
	}
	return &OpenAIHandler{byID: registered, catalog: modelCatalog}, nil
}

// RefreshCatalog discovers all providers and atomically publishes a new model
// snapshot. It is called during startup; lazy refresh keeps standalone handler
// construction useful in tests and embedded deployments.
func (h *OpenAIHandler) RefreshCatalog(ctx context.Context) error {
	h.refreshMu.Lock()
	defer h.refreshMu.Unlock()
	if err := h.catalog.Refresh(ctx); err != nil {
		return err
	}
	h.ready = true
	return nil
}

func (h *OpenAIHandler) ensureCatalog(ctx context.Context) error {
	h.refreshMu.Lock()
	defer h.refreshMu.Unlock()
	if h.ready {
		return nil
	}
	if err := h.catalog.Refresh(ctx); err != nil {
		return err
	}
	h.ready = true
	return nil
}

func (h *OpenAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		h.listModels(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/models/"):
		h.getModel(w, r, strings.TrimPrefix(r.URL.Path, "/v1/models/"))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		h.forward(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/completions":
		h.forward(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/embeddings":
		h.forward(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		h.forward(w, r)
	default:
		writeAPIError(w, http.StatusNotFound, "Not found", "invalid_request_error", "not_found")
	}
}

func (h *OpenAIHandler) listModels(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureCatalog(r.Context()); err != nil {
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "discover models", err))
		return
	}
	descriptors := h.catalog.Models()
	models := make([]provider.Model, len(descriptors))
	for i, descriptor := range descriptors {
		models[i] = provider.Model{ID: descriptor.PublicID, Object: "model", Created: descriptor.Created, OwnedBy: descriptor.OwnedBy}
	}
	writeJSON(w, http.StatusOK, struct {
		Object string           `json:"object"`
		Data   []provider.Model `json:"data"`
	}{Object: "list", Data: models})
}

func (h *OpenAIHandler) getModel(w http.ResponseWriter, r *http.Request, publicID string) {
	providerID, _, err := provider.ParseModelID(publicID)
	if err != nil || h.byID[providerID] == nil {
		writeError(w, fault.New(fault.ModelNotFound, "model"))
		return
	}
	if err := h.ensureCatalog(r.Context()); err != nil {
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "discover models", err))
		return
	}
	descriptor, ok := h.catalog.Model(publicID)
	if ok {
		writeJSON(w, http.StatusOK, provider.Model{ID: descriptor.PublicID, Object: "model", Created: descriptor.Created, OwnedBy: descriptor.OwnedBy})
		return
	}
	writeError(w, fault.New(fault.ModelNotFound, "model"))
}

func (h *OpenAIHandler) forward(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, fault.New(fault.ContextTooLarge, ""))
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
	if r.URL.Path == "/v1/responses" {
		if fields.PreviousResponseID {
			writeUnsupportedParameter(w, "previous_response_id")
			return
		}
		if fields.Conversation {
			writeUnsupportedParameter(w, "conversation")
			return
		}
	}
	providerID, modelID, err := provider.ParseModelID(fields.Model)
	upstream := h.byID[providerID]
	if err != nil || upstream == nil {
		writeError(w, fault.New(fault.ModelNotFound, "model"))
		return
	}
	response, err := upstream.Forward(r.Context(), r.URL.Path, body, modelID)
	if err != nil {
		if errors.Is(err, transport.ErrInvalidRequest) {
			writeAPIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error", "invalid_request")
			return
		}
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "forward request", err))
		return
	}
	defer response.Body.Close()
	CopyResponseHeaders(w.Header(), response.Header)
	if id, ok := requestid.FromContext(r.Context()); ok {
		w.Header().Set(requestIDHeader, id)
	}
	w.WriteHeader(response.StatusCode)
	if fields.Stream {
		_ = relayStream(w, response.Body)
		return
	}
	_, _ = io.Copy(w, response.Body)
}

func writeUnsupportedParameter(w http.ResponseWriter, parameter string) {
	writeAPIErrorWithParam(w, http.StatusBadRequest, fmt.Sprintf("The %s parameter is not supported; Responses requests must be stateless", parameter), "invalid_request_error", "unsupported_parameter", &parameter)
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
	writeAPIErrorWithParam(w, status, message, errorType, code, nil)
}

func writeAPIErrorWithParam(w http.ResponseWriter, status int, message, errorType, code string, param *string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{
		Message: message,
		Type:    errorType,
		Param:   param,
		Code:    code,
	}})
}
