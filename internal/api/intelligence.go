package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
)

func (h *OpenAIHandler) SetIntelligence(manager *intelligence.Manager) {
	h.intelligence = manager
	if manager != nil {
		manager.UpdateModels(h.catalog.Models())
	}
}
func (h *OpenAIHandler) CatalogModels() []catalog.Descriptor { return h.catalog.Models() }

func (h *OpenAIHandler) modelIntelligence(publicID string) *intelligence.Model {
	if h.intelligence == nil || !h.policy.Intelligence.Enabled() {
		return nil
	}
	model, ok := h.intelligence.Lookup(publicID)
	if !ok || (len(model.Scores) == 0 && len(model.Operations) == 0) {
		return nil
	}
	return &model
}

func (h *OpenAIHandler) intelligenceAPI(w http.ResponseWriter, r *http.Request) {
	if h.intelligence == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "Benchmark intelligence is disabled", "invalid_request_error", "intelligence_disabled")
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/gateway/v1/intelligence/refresh" {
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			h.streamBenchmarkRefresh(w, r)
			return
		}
		err := h.intelligence.Refresh(r.Context())
		// Partial failure is inspectable and successful source updates still persist.
		snapshot := h.intelligence.Snapshot()
		for id, source := range snapshot.Sources {
			source.Observations = nil
			snapshot.Sources[id] = source
		}
		response := map[string]any{"updated": err == nil, "partial_failure": err != nil, "sources": snapshot.Sources, "models": len(snapshot.Models), "unmatched": len(snapshot.Unmatched)}
		if err != nil {
			response["refresh_error"] = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	snapshot := h.intelligence.Snapshot()
	var response any
	switch {
	case r.URL.Path == "/gateway/v1/intelligence":
		response = snapshot
	case r.URL.Path == "/gateway/v1/intelligence/sources":
		for id, source := range snapshot.Sources {
			source.Observations = nil
			snapshot.Sources[id] = source
		}
		response = snapshot.Sources
	case r.URL.Path == "/gateway/v1/intelligence/unmatched":
		response = snapshot.Unmatched
	case r.URL.Path == "/gateway/v1/intelligence/models":
		if raw := r.URL.Query().Get("configured_only"); raw != "" {
			configured, err := strconv.ParseBool(raw)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "Invalid benchmark model scope", "invalid_request_error", "invalid_scope")
				return
			}
			if configured {
				if err := h.ensureCatalog(r.Context()); err != nil && len(h.byID) > 0 {
					writeAPIError(w, http.StatusServiceUnavailable, "Model catalog unavailable", "invalid_request_error", "catalog_unavailable")
					return
				}
				response = h.intelligence.ConfiguredTableModels(h.intelligence.Snapshot())
				break
			}
		}
		if raw := r.URL.Query().Get("include_unmatched"); raw != "" {
			includeUnmatched, err := strconv.ParseBool(raw)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "Invalid benchmark model scope", "invalid_request_error", "invalid_scope")
				return
			}
			if includeUnmatched {
				response = h.intelligence.TableModels(snapshot)
				break
			}
		}
		models := map[string]intelligence.Model{}
		for id, model := range snapshot.Models {
			model.Observations = nil
			models[id] = model
		}
		response = models
	case r.URL.Path == "/gateway/v1/intelligence/observations":
		limit, offset := 100, 0
		var err error
		includeMetadata := false
		if raw := r.URL.Query().Get("include_metadata"); raw != "" {
			includeMetadata, err = strconv.ParseBool(raw)
		}
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if err == nil {
				limit, err = strconv.Atoi(raw)
			}
		}
		if err == nil && r.URL.Query().Get("offset") != "" {
			offset, err = strconv.Atoi(r.URL.Query().Get("offset"))
		}
		query := strings.ToLower(r.URL.Query().Get("q"))
		if err != nil || limit < 1 || limit > 500 || offset < 0 || len(query) > 256 {
			writeAPIError(w, 400, "Invalid benchmark pagination", "invalid_request_error", "invalid_pagination")
			return
		}
		ids := make([]string, 0, len(snapshot.Sources))
		for id := range snapshot.Sources {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		observations := []intelligence.Observation{}
		total := 0
		for _, id := range ids {
			for _, observation := range snapshot.Sources[id].Observations {
				if query != "" && !strings.Contains(strings.ToLower(id+" "+observation.ModelID+" "+observation.ExternalModelID+" "+observation.Benchmark+" "+observation.Category), query) {
					continue
				}
				if total >= offset && len(observations) < limit {
					if !includeMetadata {
						observation.Metadata = nil
					}
					observations = append(observations, observation)
				}
				total++
			}
		}
		response = map[string]any{"observations": observations, "total": total, "offset": offset, "limit": limit}
	case strings.HasPrefix(r.URL.Path, "/gateway/v1/intelligence/models/"):
		id := strings.TrimPrefix(r.URL.Path, "/gateway/v1/intelligence/models/")
		model, ok := snapshot.Models[id]
		if !ok {
			for _, candidate := range snapshot.Models {
				for _, physical := range candidate.ProviderIDs {
					if physical == id {
						model, ok = candidate, true
						break
					}
				}
			}
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, "No benchmark data for this model", "invalid_request_error", "not_found")
			return
		}
		response = model
	default:
		writeAPIError(w, http.StatusNotFound, "Not found", "invalid_request_error", "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// Streaming is opt-in; existing CLI/API callers retain the synchronous JSON
// response. Only this handler writes the SSE connection, including heartbeats.
func (h *OpenAIHandler) streamBenchmarkRefresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	write := func(event string, value any) bool {
		data, err := json.Marshal(value)
		if err != nil {
			return false
		}
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	events := make(chan intelligence.RefreshEvent, 64)
	done := make(chan error, 1)
	go func() {
		err := h.intelligence.RefreshWithProgress(ctx, func(event intelligence.RefreshEvent) {
			select {
			case events <- event:
			case <-ctx.Done():
			}
		})
		done <- err
		close(events)
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !write("heartbeat", map[string]string{"message": "Refresh is still running"}) {
				return
			}
		case event, ok := <-events:
			if !ok {
				err := <-done
				snapshot := h.intelligence.Snapshot()
				for id, source := range snapshot.Sources {
					source.Observations = nil
					snapshot.Sources[id] = source
				}
				write("result", map[string]any{"updated": err == nil, "partial_failure": err != nil, "sources": snapshot.Sources, "models": len(snapshot.Models), "unmatched": len(snapshot.Unmatched)})
				return
			}
			if !write("progress", event) {
				return
			}
		}
	}
}
