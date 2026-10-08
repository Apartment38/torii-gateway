package api

import (
	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/activity"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/requestid"
	"net/http"
	"time"
)

func (h *OpenAIHandler) SetActivity(store *activity.Store) { h.activity = store }
func (h *OpenAIHandler) activityEvent(r *http.Request, kind, requested, selected string) activity.Event {
	identity, _ := ClientIdentityFromContext(r.Context())
	id, _ := requestid.FromContext(r.Context())
	e := activity.Event{Kind: kind, RequestID: id, RequestedModel: requested, SelectedModel: selected}
	if identity != nil {
		e.KeyID = identity.ID
		e.KeyName = identity.Name
	}
	if selected != "" {
		e.Provider, _, _ = provider.ParseModelID(selected)
	}
	return e
}
func (h *OpenAIHandler) completeActivity(r *http.Request, requested, selected string, started time.Time, status int, interrupted bool) {
	if h.activity == nil {
		return
	}
	e := h.activityEvent(r, "request_complete", requested, selected)
	e.Status = status
	e.DurationMS = time.Since(started).Milliseconds()
	e.Outcome = "success"
	if status >= 400 {
		e.Outcome = "error"
	}
	if interrupted || r.Context().Err() != nil {
		e.Outcome = "interrupted"
	}
	amount := accounting.Amount{}
	if subject := accounting.SubjectFrom(r.Context()); subject != nil {
		amount, e.AccountingFailure = subject.Snapshot()
	}
	e.InputTokens = amount.Input
	e.OutputTokens = amount.Output
	e.EstimatedTokens = amount.Estimated
	e.UnknownCost = amount.Unknown > 0
	if !e.UnknownCost {
		e.EstimatedCost = &amount.Cost
	}
	h.activity.Emit(e)
}
