package api

import (
	"context"
	"errors"
	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/intelligence"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"net/http"
	"sync"
	"time"
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
	Quota            quota.Status        `json:"quota"`
	Usage            []ModelUsage        `json:"usage"`
	UsageAvailable   bool                `json:"usage_available"`
}

// ModelUsage omits key identifiers and names from discovery metadata. Client
// discovery is scoped to its authenticated key; management uses gateway totals.
type ModelUsage struct {
	Period string `json:"period"`
	accounting.Amount
}

type clientModel struct {
	provider.Model
	Pricing        *catalog.Pricing `json:"pricing,omitempty"`
	Quota          *quota.Status    `json:"quota,omitempty"`
	Usage          []ModelUsage     `json:"usage,omitempty"`
	UsageAvailable bool             `json:"usage_available"`
}

func (h *OpenAIHandler) modelUsage(r *http.Request, management bool) (map[string][]ModelUsage, bool) {
	result := map[string][]ModelUsage{}
	if h.accounting == nil {
		return result, false
	}
	rows, _, err := h.accounting.Summary(h.budgets)
	if err != nil {
		return result, false
	}
	key := "anonymous"
	if identity, ok := ClientIdentityFromContext(r.Context()); ok && identity != nil {
		key = identity.ID
	}
	if management {
		key = "*"
	}
	for _, row := range rows {
		if row.Key == key && row.Model != "" {
			result[row.Model] = append(result[row.Model], ModelUsage{Period: row.Period, Amount: row.Amount})
		}
	}
	return result, true
}

func (h *OpenAIHandler) modelQuota(ctx context.Context, id string) quota.Status {
	if h.quota != nil {
		status, _ := h.ModelQuota(ctx, id)
		return status
	}
	return quota.Status{}
}

// Warm each provider account concurrently within one picker request deadline.
// A slow billing service cannot multiply the wait by the number of models.
func (h *OpenAIHandler) prefetchQuotas(ctx context.Context, models []catalog.Descriptor) map[string]quota.Status {
	results := make(map[string]quota.Status, len(models))
	if h.quota == nil {
		return results
	}
	var mu sync.Mutex
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	jobs := make(chan string, len(models))
	for _, m := range models {
		jobs <- m.PublicID
	}
	close(jobs)
	var wg sync.WaitGroup
	for i := 0; i < min(8, len(models)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				status := h.modelQuota(ctx, id)
				mu.Lock()
				results[id] = status
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results
}

func (h *OpenAIHandler) clientMetadata(ctx context.Context, d catalog.Descriptor, usage []ModelUsage, available bool) clientModel {
	return clientMetadataWithQuota(d, usage, available, h.modelQuota(ctx, d.PublicID))
}

func clientMetadataWithQuota(d catalog.Descriptor, usage []ModelUsage, available bool, status quota.Status) clientModel {
	return clientModel{Model: provider.Model{ID: d.PublicID, Object: "model", Created: d.Created, OwnedBy: d.OwnedBy, ContextWindow: d.ContextWindow, InputPerMillion: d.Pricing.InputPerMillion, OutputPerMillion: d.Pricing.OutputPerMillion, Subscription: d.Subscription}, Pricing: &d.Pricing, Quota: &status, Usage: usage, UsageAvailable: available}
}

func (h *OpenAIHandler) ManagementModels(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureCatalog(r.Context()); err != nil {
		writeAPIError(w, 503, "Model discovery unavailable", "server_error", "catalog_unavailable")
		return
	}
	models := []ManagedModel{}
	snapshots := h.prefetchQuotas(r.Context(), h.CatalogModels())
	usage, usageAvailable := h.modelUsage(r, true)
	for _, d := range h.CatalogModels() {
		m := ManagedModel{Descriptor: d, Enabled: h.enabled(d.PublicID), BenchmarkMissing: true, Quota: snapshots[d.PublicID], Usage: usage[d.PublicID], UsageAvailable: usageAvailable}
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

func (h *OpenAIHandler) ModelQuota(ctx context.Context, id string) (quota.Status, error) {
	if h.quota == nil {
		return quota.Status{}, nil
	}
	status, err := h.quota.Get(ctx, id)
	if !status.Known && h.observedQuotas != nil {
		if providerID, _, parseErr := provider.ParseModelID(id); parseErr == nil {
			if observed, observeErr := h.observedQuotas.Get(ctx, providerID); observeErr == nil && observed.Known {
				return observed, nil
			}
		}
	}
	return status, err
}

func quotaErrorMessage(err error) string {
	var remote *quota.HTTPError
	if errors.As(err, &remote) {
		return remote.Error()
	}
	return "Provider quota unavailable"
}
