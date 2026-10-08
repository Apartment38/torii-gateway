package api

import (
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"net/http"
)

func (h *OpenAIHandler) SetModelControls(enabled map[string]bool, free map[string]bool) {
	h.modelEnabled = enabled
	h.freeProviders = free
}
func (h *OpenAIHandler) enabled(id string) bool {
	value, exists := h.modelEnabled[id]
	return !exists || value
}
func (h *OpenAIHandler) CatalogModels() []catalog.Descriptor {
	models := h.catalog.Models()
	for i := range models {
		if h.freeProviders[models[i].ProviderID] {
			zero := float64(0)
			models[i].Pricing = catalog.Pricing{InputPerMillion: &zero, OutputPerMillion: &zero}
		}
	}
	return models
}

type ManagedModel struct {
	catalog.Descriptor
	Enabled          bool                `json:"enabled"`
	Benchmark        *intelligence.Model `json:"benchmark,omitempty"`
	BenchmarkMissing bool                `json:"benchmark_missing"`
}

func (h *OpenAIHandler) ManagementModels(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureCatalog(r.Context()); err != nil {
		writeAPIError(w, 503, "Model discovery unavailable", "server_error", "catalog_unavailable")
		return
	}
	models := []ManagedModel{}
	for _, d := range h.CatalogModels() {
		m := ManagedModel{Descriptor: d, Enabled: h.enabled(d.PublicID), BenchmarkMissing: true}
		if h.intelligence != nil {
			if benchmark, ok := h.intelligence.Lookup(d.PublicID); ok {
				benchmark.Observations = nil
				m.Benchmark = &benchmark
				m.BenchmarkMissing = len(benchmark.Scores) == 0
			}
		}
		models = append(models, m)
	}
	writeJSON(w, 200, map[string]any{"models": models})
}
