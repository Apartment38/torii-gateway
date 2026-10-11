package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/activity"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/fault"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
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
	namedRouters        map[string]*OpenAIHandler
	routerName          string
	allowedProviders    map[string]bool
	allowedModels       map[string]bool
	activity            *activity.Store
	accounting          *accounting.Store
	accountingOverrides []catalog.Descriptor
	budgets             accounting.Settings
	modelEnabled        map[string]bool
	freeProviders       map[string]bool
	byID                map[string]provider.OpenAIProvider
	catalog             *catalog.Catalog
	refreshMu           sync.Mutex
	ready               bool
	refreshedAt         time.Time
	sessions            *session.Manager
	classifier          classifier.Runner
	policy              router.SelectionPolicy
	usage               *usage.Store
	quota               *quota.Cache
	intelligence        *intelligence.Manager
	latencyMu           sync.Mutex
	latency             map[string]time.Duration
	rerouteOnFailure    bool
}

func NewOpenAIHandler(providers []provider.OpenAIProvider) (*OpenAIHandler, error) {
	return NewOpenAIHandlerWithOverrides(providers, nil)
}

// NewOpenAIHandlerWithOverrides constructs a handler with optional catalog
// metadata supplied by the operator. Overrides are applied after bundled and
// live provider metadata during catalog refresh.
func NewOpenAIHandlerWithOverrides(providers []provider.OpenAIProvider, overrides []catalog.Descriptor) (*OpenAIHandler, error) {
	registered := make(map[string]provider.OpenAIProvider, len(providers))
	discovery := make([]provider.Provider, 0, len(providers))
	for _, p := range providers {
		if p == nil {
			return nil, errors.New("register provider: nil provider")
		}
		if classifier, ok := p.(interface{ ClassifierOnly() bool }); ok && classifier.ClassifierOnly() {
			continue
		}
		if _, exists := registered[p.ID()]; exists {
			return nil, fmt.Errorf("register provider: duplicate ID %q", p.ID())
		}
		registered[p.ID()] = p
		discovery = append(discovery, p)
	}
	modelCatalog, err := catalog.New(discovery, nil, overrides)
	if err != nil {
		return nil, err
	}
	return &OpenAIHandler{byID: registered, catalog: modelCatalog, sessions: session.NewManager(30*time.Minute, session.MaxSessions), policy: router.DefaultSelectionPolicy(), latency: make(map[string]time.Duration), rerouteOnFailure: true}, nil
}

// SetClassifier configures the bounded semantic classifier used by smart.
// Leaving it unset selects using objective requirements and deterministic
// selection within the configured safety constraints.
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
	if h.intelligence != nil {
		h.intelligence.UpdateModels(h.CatalogModels())
	}
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
	if h.intelligence != nil {
		h.intelligence.UpdateModels(h.CatalogModels())
	}
	return nil
}

func (h *OpenAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/gateway/v1/intelligence"):
		h.intelligenceAPI(w, r)
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
	for _, child := range h.namedRouters {
		child.sessions.Delete(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *OpenAIHandler) listModels(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureCatalog(r.Context()); err != nil {
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "discover models", err))
		return
	}
	descriptors := h.CatalogModels()
	snapshots := h.prefetchQuotas(r.Context(), descriptors)
	models := make([]clientModel, 0, len(descriptors)+1)
	modelUsage, usageAvailable := h.modelUsage(r, false)
	// Clients may show only the first page of a provider's catalog. Keep the
	// gateway routing choice visible before the sorted physical models.
	for _, name := range h.RouterNames() {
		models = append(models, clientModel{Model: provider.Model{ID: name, Object: "model", OwnedBy: "torii"}})
	}
	for _, descriptor := range descriptors {
		if !h.enabled(descriptor.PublicID) {
			continue
		}
		models = append(models, clientMetadataWithQuota(descriptor, modelUsage[descriptor.PublicID], usageAvailable, snapshots[descriptor.PublicID]))
	}
	writeJSON(w, http.StatusOK, struct {
		Object string        `json:"object"`
		Data   []clientModel `json:"data"`
	}{Object: "list", Data: models})
}

func (h *OpenAIHandler) getModel(w http.ResponseWriter, r *http.Request, publicID string) {
	if publicID == h.smartName() || h.namedRouters[publicID] != nil {
		writeJSON(w, http.StatusOK, provider.Model{ID: publicID, Object: "model", OwnedBy: "torii"})
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
	for _, descriptor := range h.CatalogModels() {
		if descriptor.PublicID == publicID && h.enabled(publicID) {
			modelUsage, usageAvailable := h.modelUsage(r, false)
			writeJSON(w, http.StatusOK, h.clientMetadata(r.Context(), descriptor, modelUsage[publicID], usageAvailable))
			return
		}
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
	if child := h.namedRouters[fields.Model]; child != nil {
		clone := r.Clone(r.Context())
		clone.Body = io.NopCloser(bytes.NewReader(body))
		child.forward(w, clone)
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
	activityStarted := time.Now()
	activitySelected := ""
	activityInterrupted := false
	tracked := &statusWriter{ResponseWriter: w, status: 200}
	w = tracked
	if h.activity != nil {
		h.activity.Emit(h.activityEvent(r, "request_start", fields.Model, ""))
	}
	defer func() {
		h.completeActivity(r, fields.Model, activitySelected, activityStarted, tracked.status, activityInterrupted)
	}()
	r, err = h.beginAccounting(r)
	if err != nil {
		writeBudgetError(w, err)
		return
	}
	requestedModel := fields.Model
	var routing *smartRouting
	if requestedModel == h.smartName() {
		routing, err = h.prepareSmartRouting(r, body)
		defer routing.logEvidence(r.Context())
		if subject := accounting.SubjectFrom(r.Context()); subject != nil && subject.BudgetFailure() != nil {
			err = subject.BudgetFailure()
		}
		if err == nil {
			requestedModel, err = h.selectSmartModel(r.Context(), routing, "")
		}
		if err != nil {
			routing.errorKind, _ = fault.KindOf(err)
			routing.writeHeaders(w.Header(), "")
			if !writeBudgetError(w, err) {
				writeError(w, err)
			}
			return
		}
	}
	if !h.enabled(requestedModel) {
		writeError(w, fault.New(fault.ModelUnavailable, "model"))
		return
	}
	providerID, modelID, err := provider.ParseModelID(requestedModel)
	upstream := h.byID[providerID]
	if err != nil || upstream == nil {
		writeError(w, fault.New(fault.ModelNotFound, "model"))
		return
	}
	activitySelected = requestedModel
	if h.activity != nil {
		h.activity.Emit(h.activityEvent(r, "routing", fields.Model, requestedModel))
	}
	started := time.Now()
	response, err := upstream.Forward(r.Context(), r.URL.Path, body, modelID)
	h.recordLatency(requestedModel, time.Since(started))
	routing.recordAttempt(requestedModel, response, err, time.Since(started))
	if routing != nil && retryableResponse(response, err) {
		h.sessions.Delete(routing.identity.ID)
		// Retry at most once, before any upstream bytes have reached the client.
		// Tool continuations keep their provider's opaque call IDs and fail in
		// place rather than silently crossing to a different physical model.
		if h.rerouteOnFailure && !routing.toolContinuation && r.Context().Err() == nil {
			if alternate, selectErr := h.selectSmartModel(r.Context(), routing, requestedModel); selectErr == nil {
				if response != nil && response.Body != nil {
					_ = response.Body.Close()
				}
				providerID, modelID, _ = provider.ParseModelID(alternate)
				requestedModel, upstream = alternate, h.byID[providerID]
				activitySelected = requestedModel
				if h.activity != nil {
					h.activity.Emit(h.activityEvent(r, "routing", fields.Model, requestedModel))
				}
				started = time.Now()
				response, err = upstream.Forward(r.Context(), r.URL.Path, body, modelID)
				h.recordLatency(requestedModel, time.Since(started))
				routing.recordAttempt(requestedModel, response, err, time.Since(started))
			}
		}
	}
	if err == nil && (response == nil || response.Body == nil) {
		err = errors.New("upstream returned no response body")
	}
	if err != nil {
		if writeBudgetError(w, err) {
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			return
		}
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if routing != nil {
			h.sessions.Delete(routing.identity.ID)
			routing.errorKind = fault.ProviderUnavailable
			routing.writeHeaders(w.Header(), requestedModel)
		}
		if errors.Is(err, transport.ErrInvalidRequest) {
			writeAPIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error", "invalid_request")
			return
		}
		writeError(w, fault.Wrap(fault.ProviderUnavailable, "forward request", err))
		return
	}
	h.recordQuotaHeaders(requestedModel, response.Header)
	defer response.Body.Close()
	CopyResponseHeaders(w.Header(), response.Header)
	routing.writeHeaders(w.Header(), requestedModel)
	if id, ok := requestid.FromContext(r.Context()); ok {
		w.Header().Set(requestIDHeader, id)
	}
	w.WriteHeader(response.StatusCode)
	if fields.Stream {
		observer := accounting.NewObserver(true)
		relayErr := relayStream(w, io.TeeReader(response.Body, observer))
		observer.Finish()
		activityInterrupted = relayErr != nil
		if relayErr == nil && observer.Reported && h.usage != nil {
			h.usage.Add(requestedModel, uint64(observer.Input), uint64(observer.Output), h.actualCost(requestedModel, uint64(observer.Input), uint64(observer.Output)))
		}
		h.finishSmartRouting(routing, requestedModel, response.StatusCode, relayErr)
		return
	}
	observer := accounting.NewObserver(false)
	_, relayErr := io.Copy(w, io.TeeReader(response.Body, observer))
	observer.Finish()
	activityInterrupted = relayErr != nil
	if relayErr == nil && observer.Reported && h.usage != nil {
		h.usage.Add(requestedModel, uint64(observer.Input), uint64(observer.Output), h.actualCost(requestedModel, uint64(observer.Input), uint64(observer.Output)))
	}
	h.finishSmartRouting(routing, requestedModel, response.StatusCode, relayErr)

}

func (h *OpenAIHandler) actualCost(model string, input, output uint64) float64 {
	providerID, _, _ := provider.ParseModelID(model)
	if h.freeProviders[providerID] {
		return 0
	}
	descriptor, ok := h.catalog.Model(model)
	if !ok {
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
	if status, ok := quota.FromHeaders(headers, time.Now().UTC()); ok {
		for i := range status.Windows {
			if strings.HasPrefix(status.Windows[i].Name, "anthropic_") {
				_, upstream, err := provider.ParseModelID(model)
				if err == nil {
					status.Windows[i].Model = upstream
				}
			}
		}
		_ = h.quota.Observe(model, status)
	}
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

func (h *OpenAIHandler) smartName() string {
	if h.routerName != "" {
		return h.routerName
	}
	return "smart"
}
func (h *OpenAIHandler) RouterNames() []string {
	names := []string{}
	for name := range h.namedRouters {
		names = append(names, name)
	}
	sort.Strings(names)
	return append([]string{h.smartName()}, names...)
}
func (h *OpenAIHandler) AddRouter(name string, child *OpenAIHandler) {
	if h.namedRouters == nil {
		h.namedRouters = map[string]*OpenAIHandler{}
	}
	h.namedRouters[name] = child
}
func (h *OpenAIHandler) SetRouterScope(name string, providers, models []string) {
	h.routerName = name
	h.allowedProviders = map[string]bool{}
	h.allowedModels = map[string]bool{}
	for _, id := range providers {
		h.allowedProviders[id] = true
	}
	for _, id := range models {
		h.allowedModels[id] = true
	}
}
func (h *OpenAIHandler) NewRouter(providers []provider.OpenAIProvider, overrides []catalog.Descriptor) (*OpenAIHandler, error) {
	child, err := NewOpenAIHandlerWithOverrides(providers, overrides)
	if err != nil {
		return nil, err
	}
	if len(overrides) == 0 {
		child.catalog = h.catalog
	}
	child.accounting = h.accounting
	child.accountingOverrides = h.accountingOverrides
	child.budgets = h.budgets
	child.activity = h.activity
	child.intelligence = h.intelligence
	child.usage = h.usage
	child.modelEnabled = h.modelEnabled
	child.freeProviders = h.freeProviders
	return child, nil
}
