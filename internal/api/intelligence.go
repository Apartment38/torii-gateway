package api

import (
	"encoding/json"
	"net/http"
	"strings"

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
		models := map[string]intelligence.Model{}
		for id, model := range snapshot.Models {
			model.Observations = nil
			models[id] = model
		}
		response = models
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
