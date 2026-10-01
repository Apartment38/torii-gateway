package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/fault"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/requestid"
	"github.com/Apartment38/torii-gateway/internal/router"
	"github.com/Apartment38/torii-gateway/internal/session"
	"github.com/Apartment38/torii-gateway/internal/transport"
	"github.com/Apartment38/torii-gateway/internal/usage"
)

const maxRequestBody = 32 << 20

// OpenAIHandler serves the currently implemented OpenAI-compatible endpoints.
type OpenAIHandler struct {
	byID             map[string]provider.OpenAIProvider
	catalog          *catalog.Catalog
	refreshMu        sync.Mutex
	ready            bool
	refreshedAt      time.Time
	sessions         *session.Manager
	classifier       classifier.Runner
	policy           router.SelectionPolicy
	usage            *usage.Store
	quota            *quota.Cache
	latencyMu        sync.Mutex
	latency          map[string]time.Duration
	rerouteOnFailure bool
}

func NewOpenAIHandler(providers []provider.OpenAIProvider) (*OpenAIHandler, error) {
	return NewOpenAIHandlerWithOverrides(providers, nil)
}

// NewOpenAIHandlerWithOverrides constructs a handler with optional catalog
// metadata supplied by the operator. Overrides are applied after bundled and
// live provider metadata during catalog refresh.
func NewOpenAIHandlerWithOverrides(providers []provider.OpenAIProvider, overrides []catalog.Descriptor) (*OpenAIHandler, error) {
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
	modelCatalog, err := catalog.New(discovery, nil, overrides)
	if err != nil {
		return nil, err
	}
	return &OpenAIHandler{byID: registered, catalog: modelCatalog, sessions: session.NewManager(30*time.Minute, session.MaxSessions), policy: router.DefaultSelectionPolicy(), latency: make(map[string]time.Duration), rerouteOnFailure: true}, nil
}

// SetClassifier configures the bounded semantic classifier used by smart.
// Leaving it unset selects using objective requirements and deterministic
// capability fallback.
func (h *OpenAIHandler) SetClassifier(runner classifier.Runner) { h.classifier = runner }

func (h *OpenAIHandler) SetSelectionPolicy(policy router.SelectionPolicy) { h.policy = policy }

func (h *OpenAIHandler) SetUsageStore(store *usage.Store) { h.usage = store }

func (h *OpenAIHandler) SetQuotaCache(cache *quota.Cache) { h.quota = cache }

func (h *OpenAIHandler) SetSessionTimeout(timeout time.Duration) {
	h.sessions = session.NewManager(timeout, session.MaxSessions)
}

func (h *OpenAIHandler) SetModelFailurePolicy(policy string) { h.rerouteOnFailure = policy != "fail" }

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
	h.refreshedAt = time.Now()
	return nil
}

func (h *OpenAIHandler) ensureCatalog(ctx context.Context) error {
	h.refreshMu.Lock()
	defer h.refreshMu.Unlock()
	// Do not pin a startup fallback forever after credentials or the account
	// catalog change. Refresh at most once per minute, serialized across callers.
	if h.ready && time.Since(h.refreshedAt) < time.Minute {
		return nil
	}
	if err := h.catalog.Refresh(ctx); err != nil {
		return err
	}
	h.ready = true
	h.refreshedAt = time.Now()
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
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/gateway/v1/sessions/"):
		h.deleteSession(w, r, strings.TrimPrefix(r.URL.Path, "/gateway/v1/sessions/"))
	default:
		writeAPIError(w, http.StatusNotFound, "Not found", "invalid_request_error", "not_found")
	}
}

func (h *OpenAIHandler) deleteSession(w http.ResponseWriter, _ *http.Request, id string) {
	if id == "" || strings.Contains(id, "/") || len(id) > session.MaxIDLength {
		writeAPIError(w, http.StatusBadRequest, "A valid session ID is required", "invalid_request_error", "invalid_session")
		return
	}
	h.sessions.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *OpenAIHandler) listModels(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureCatalog(r.Context()); err != nil {
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "discover models", err))
		return
	}
	descriptors := h.catalog.Models()
	models := make([]provider.Model, 0, len(descriptors)+1)
	for _, descriptor := range descriptors {
		models = append(models, provider.Model{ID: descriptor.PublicID, Object: "model", Created: descriptor.Created, OwnedBy: descriptor.OwnedBy})
	}
	models = append(models, provider.Model{ID: "smart", Object: "model", OwnedBy: "torii"})
	writeJSON(w, http.StatusOK, struct {
		Object string           `json:"object"`
		Data   []provider.Model `json:"data"`
	}{Object: "list", Data: models})
}

func (h *OpenAIHandler) getModel(w http.ResponseWriter, r *http.Request, publicID string) {
	if publicID == "smart" {
		writeJSON(w, http.StatusOK, provider.Model{ID: "smart", Object: "model", OwnedBy: "torii"})
		return
	}
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
	requestedModel := fields.Model
	if requestedModel == "smart" {
		var resolveErr error
		requestedModel, resolveErr = h.resolveSmartModel(r, body)
		if resolveErr != nil {
			writeError(w, fault.Wrap(fault.ClassifierFailure, "resolve smart model", resolveErr))
			return
		}
		if requestedModel == "" {
			writeError(w, fault.New(fault.ModelUnavailable, "smart"))
			return
		}
	}
	providerID, modelID, err := provider.ParseModelID(requestedModel)
	upstream := h.byID[providerID]
	if err != nil || upstream == nil {
		writeError(w, fault.New(fault.ModelNotFound, "model"))
		return
	}
	started := time.Now()
	response, err := upstream.Forward(r.Context(), r.URL.Path, body, modelID)
	if fields.Model == "smart" && h.rerouteOnFailure && (err != nil || (response != nil && response.StatusCode >= 500)) {
		if response != nil {
			_ = response.Body.Close()
		}
		if alternate := h.nextSmartModel(r, body, requestedModel); alternate != "" {
			if alternateProvider, alternateModel, parseErr := provider.ParseModelID(alternate); parseErr == nil && h.byID[alternateProvider] != nil {
				requestedModel, providerID, modelID = alternate, alternateProvider, alternateModel
				upstream = h.byID[providerID]
				response, err = upstream.Forward(r.Context(), r.URL.Path, body, modelID)
			}
		}
	}
	h.recordLatency(requestedModel, time.Since(started))
	if err != nil {
		if errors.Is(err, transport.ErrInvalidRequest) {
			writeAPIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error", "invalid_request")
			return
		}
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "forward request", err))
		return
	}
	h.recordQuotaHeaders(requestedModel, response.Header)
	if fields.Model == "smart" {
		if identity := smartIdentityInfo(r, body, r.URL.Path); identity.ID != "" {
			_ = h.sessions.Put(session.Record{ID: identity.ID, Model: requestedModel, ContextSignature: identity.Signature})
		}
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
	if h.usage != nil {
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<20))
		if readErr == nil {
			if input, output, ok := usage.ExtractResponseUsage(data); ok {
				_ = h.usage.Add(requestedModel, input, output, h.actualCost(requestedModel, input, output))
			}
			_, _ = w.Write(data)
			return
		}
	}
	_, _ = io.Copy(w, response.Body)
}

func (h *OpenAIHandler) actualCost(model string, input, output uint64) float64 {
	descriptor, ok := h.catalog.Model(model)
	if !ok || (descriptor.Local != nil && *descriptor.Local) {
		return 0
	}
	if descriptor.Pricing.InputPerMillion == nil || descriptor.Pricing.OutputPerMillion == nil {
		return 0
	}
	return (float64(input)*(*descriptor.Pricing.InputPerMillion) + float64(output)*(*descriptor.Pricing.OutputPerMillion)) / 1_000_000
}

func (h *OpenAIHandler) recordQuotaHeaders(model string, headers http.Header) {
	if h.quota == nil {
		return
	}
	limit := firstHeaderNumber(headers, "X-RateLimit-Limit-Requests", "X-RateLimit-Limit-Tokens")
	remaining := firstHeaderNumber(headers, "X-RateLimit-Remaining-Requests", "X-RateLimit-Remaining-Tokens")
	if limit == nil || remaining == nil || *limit < 0 || *remaining < 0 {
		return
	}
	if *remaining > *limit {
		*remaining = *limit
	}
	_ = h.quota.SetLocal(model, *limit, *limit-*remaining)
}

func firstHeaderNumber(headers http.Header, names ...string) *float64 {
	for _, name := range names {
		raw := strings.TrimSpace(strings.Split(headers.Get(name), ",")[0])
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return &value
		}
	}
	return nil
}

func (h *OpenAIHandler) resolveSmartModel(r *http.Request, body []byte) (string, error) {
	if err := h.ensureCatalog(r.Context()); err != nil {
		return "", err
	}
	identity := smartIdentityInfo(r, body, r.URL.Path)
	if identity.ID != "" {
		if pinned, ok := h.sessions.GetWithSignature(identity.ID, identity.Signature); ok && pinned.Model != "" {
			if _, exists := h.catalog.Model(pinned.Model); exists {
				return pinned.Model, nil
			}
		}
	}
	models := h.catalog.Models()
	if len(models) == 0 {
		return "", errors.New("smart model has no physical candidates")
	}
	endpoint := router.EndpointChat
	switch r.URL.Path {
	case "/v1/completions":
		endpoint = router.EndpointCompletions
	case "/v1/responses":
		endpoint = router.EndpointResponses
	case "/v1/embeddings":
		endpoint = router.EndpointEmbeddings
	}
	requirements, err := router.ExtractRequirements(endpoint, body)
	if err == nil {
		complexity := "medium"
		if h.classifier.Classifier != nil {
			classification, classifyErr := h.classifier.Classify(r.Context(), classifier.ClassificationRequest{Model: "smart", Text: smartText(body), Requirements: requirements})
			if classifyErr != nil {
				return "", classifyErr
			}
			complexity = string(classification.Complexity)
			requirements.Tools = classification.Requirements.Tools
			requirements.Vision = classification.Requirements.Vision
			requirements.Reasoning = classification.Requirements.Reasoning
			requirements.StructuredOutput = classification.Requirements.StructuredOutput
		}
		candidates := make([]router.Candidate, 0, len(models))
		for _, model := range models {
			candidate := router.Candidate{Model: model, LatencyEWMA: h.getLatency(model.PublicID)}
			if h.quota != nil {
				if status, quotaErr := h.quota.Get(r.Context(), model.PublicID); quotaErr == nil {
					candidate.QuotaKnown, candidate.QuotaRemaining, candidate.QuotaExhausted = status.Known, status.FractionRemaining, status.Exhausted
				}
			}
			candidates = append(candidates, candidate)
		}
		policy := h.policy
		if policy.MinimumQuality == nil {
			policy = router.DefaultSelectionPolicy()
		}
		if decision, selectErr := router.Select(candidates, requirements, complexity, policy); selectErr == nil {
			return decision.Model.PublicID, nil
		}
		if requirements.Tools || requirements.Vision || requirements.StructuredOutput || requirements.Reasoning || requirements.Streaming {
			return "", errors.New("no candidate has verified support for the requested capabilities")
		}
	}
	// Unknown provider metadata is intentionally not treated as verified
	// capability support. The deterministic fallback still gives smart a useful
	// route when no metadata-backed candidate is available.
	for _, model := range models {
		if model.Availability.Available {
			return model.PublicID, nil
		}
	}
	return models[0].PublicID, nil
}

func (h *OpenAIHandler) recordLatency(model string, value time.Duration) {
	h.latencyMu.Lock()
	defer h.latencyMu.Unlock()
	previous := h.latency[model]
	if previous == 0 {
		h.latency[model] = value
	} else {
		h.latency[model] = (previous*4 + value) / 5
	}
}
func (h *OpenAIHandler) getLatency(model string) time.Duration {
	h.latencyMu.Lock()
	defer h.latencyMu.Unlock()
	return h.latency[model]
}

func (h *OpenAIHandler) nextSmartModel(r *http.Request, body []byte, excluded string) string {
	if err := h.ensureCatalog(r.Context()); err != nil {
		return ""
	}
	endpoint := router.EndpointChat
	switch r.URL.Path {
	case "/v1/completions":
		endpoint = router.EndpointCompletions
	case "/v1/responses":
		endpoint = router.EndpointResponses
	case "/v1/embeddings":
		endpoint = router.EndpointEmbeddings
	}
	if requirements, extractErr := router.ExtractRequirements(endpoint, body); extractErr == nil {
		candidates := make([]router.Candidate, 0)
		for _, model := range h.catalog.Models() {
			if model.PublicID == excluded {
				continue
			}
			candidate := router.Candidate{Model: model, LatencyEWMA: h.getLatency(model.PublicID)}
			if h.quota != nil {
				if status, quotaErr := h.quota.Get(r.Context(), model.PublicID); quotaErr == nil {
					candidate.QuotaKnown, candidate.QuotaRemaining, candidate.QuotaExhausted = status.Known, status.FractionRemaining, status.Exhausted
				}
			}
			candidates = append(candidates, candidate)
		}
		policy := h.policy
		if policy.MinimumQuality == nil {
			policy = router.DefaultSelectionPolicy()
		}
		if decision, selectErr := router.Select(candidates, requirements, "medium", policy); selectErr == nil {
			return decision.Model.PublicID
		}
		if requirements.Tools || requirements.Vision || requirements.StructuredOutput || requirements.Reasoning || requirements.Streaming {
			return ""
		}
	}
	for _, model := range h.catalog.Models() {
		if model.PublicID == excluded || !model.Availability.Available {
			continue
		}
		return model.PublicID
	}
	_ = body
	return ""
}

func smartText(body []byte) string {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	if input, ok := root["input"].(string); ok {
		return input
	}
	if messages, ok := root["messages"].([]any); ok {
		for _, raw := range messages {
			message, _ := raw.(map[string]any)
			role, _ := message["role"].(string)
			if role == "user" {
				if content, ok := message["content"].(string); ok {
					return content
				}
			}
		}
	}
	return ""
}

func smartIdentity(r *http.Request, body []byte, endpoint string) string {
	return smartIdentityInfo(r, body, endpoint).ID
}

func smartIdentityInfo(r *http.Request, body []byte, endpoint string) session.Identity {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return session.Identity{}
	}
	metadata, _ := root["metadata"].(map[string]any)
	user, _ := root["user"].(string)
	var system, first string
	if messages, ok := root["messages"].([]any); ok {
		for _, raw := range messages {
			message, _ := raw.(map[string]any)
			role, _ := message["role"].(string)
			content, _ := message["content"].(string)
			if role == "system" || role == "developer" {
				system += content
			}
			if role == "user" && first == "" {
				first = content
			}
		}
	}
	headers := map[string]string{"X-Session-ID": r.Header.Get("X-Session-ID"), "X-Conversation-ID": r.Header.Get("X-Conversation-ID")}
	return session.Identify(headers, metadata, user, endpoint, system, first)
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
