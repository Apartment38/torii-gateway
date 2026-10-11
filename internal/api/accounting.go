package api

import (
	"errors"
	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"github.com/Apartment38/torii-gateway/internal/requestid"
	"net/http"
	"strconv"
	"time"
)

func (h *OpenAIHandler) ConfigureAccounting(store *accounting.Store, settings accounting.Settings, providers []provider.OpenAIProvider) []provider.OpenAIProvider {
	h.accounting = store
	h.budgets = settings
	wrapped := make([]provider.OpenAIProvider, 0, len(providers))
	for _, p := range providers {
		w := accounting.Wrap(p, store, settings, h.accountingDescriptor)
		if h.byID[p.ID()] != nil {
			h.byID[p.ID()] = w
		}
		wrapped = append(wrapped, w)
	}
	return wrapped
}
func (h *OpenAIHandler) SetAccountingOverrides(overrides []catalog.Descriptor) {
	h.accountingOverrides = overrides
}
func (h *OpenAIHandler) AccountingDescriptor(id string) catalog.Descriptor {
	return h.accountingDescriptor(id)
}
func (h *OpenAIHandler) accountingDescriptor(id string) catalog.Descriptor {
	for _, d := range h.CatalogModels() {
		if d.PublicID == id {
			return d
		}
	}
	providerID, modelID, _ := provider.ParseModelID(id)
	d := catalog.Descriptor{PublicID: id, ProviderID: providerID, UpstreamID: modelID}
	for _, override := range h.accountingOverrides {
		if override.PublicID == id {
			d = override
			break
		}
	}
	if h.freeProviders[providerID] {
		zero := float64(0)
		d.Pricing = catalog.Pricing{InputPerMillion: &zero, OutputPerMillion: &zero}
	}
	return d
}
func (h *OpenAIHandler) beginAccounting(r *http.Request) (*http.Request, error) {
	if h.accounting == nil {
		return r, nil
	}
	identity, _ := ClientIdentityFromContext(r.Context())
	key, name := "anonymous", "anonymous"
	if identity != nil {
		key, name = identity.ID, identity.Name
	}
	id, _ := requestid.FromContext(r.Context())
	subject := &accounting.Subject{Key: key, Name: name, RequestID: id}
	reservation, err := h.accounting.Reserve(key, name, "", "", h.budgets, accounting.Amount{Requests: 1})
	if err != nil && h.budgets.Limited(key) {
		return r, err
	}
	if err == nil {
		h.accounting.Settle(reservation, nil)
	}
	return r.WithContext(accounting.WithSubject(r.Context(), subject)), nil
}
func writeBudgetError(w http.ResponseWriter, err error) bool {
	var limit *accounting.LimitError
	if errors.As(err, &limit) {
		reset := limit.Reset
		if reset.IsZero() {
			reset = time.Now().UTC().Add(time.Minute)
		}
		w.Header().Set("Retry-After", strconv.FormatInt(max(1, int64(time.Until(reset).Seconds())), 10))
		w.Header().Set("X-Quota-Reset", reset.UTC().Format(time.RFC3339))
		writeAPIError(w, 429, limit.Error(), "insufficient_quota", "quota_exceeded")
		return true
	}
	if errors.Is(err, accounting.ErrStorage) {
		writeAPIError(w, 503, "Operator quota storage unavailable", "server_error", "quota_storage_unavailable")
		return true
	}
	return false
}
func (h *OpenAIHandler) UsageAPI(w http.ResponseWriter, r *http.Request) {
	if h.accounting == nil {
		writeAPIError(w, 503, "Accounting unavailable", "server_error", "accounting_unavailable")
		return
	}
	summaries, budgets, err := h.accounting.Summary(h.budgets)
	if err != nil {
		writeBudgetError(w, err)
		return
	}
	providerQuotas := map[string]quota.Status{}
	h.prefetchQuotas(r.Context(), h.CatalogModels())
	for _, d := range h.CatalogModels() {
		providerQuotas[d.PublicID] = h.modelQuota(r.Context(), d.PublicID)
	}
	writeJSON(w, 200, map[string]any{"usage": summaries, "budgets": budgets, "provider_quotas": providerQuotas, "qualification": "Operator accounting; monetary values are estimates, not provider billing. Unknown costs remain unknown; estimated tokens include unresolved reservations. Provider quotas and usage_usd are upstream snapshots, independent of operator budgets.", "row_limit": 10000})
}
