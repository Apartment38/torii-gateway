package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

type Subject struct {
	Key, Name, RequestID string
	mu                   sync.Mutex
	Total                Amount
	Failure              error
	StorageFailure       bool
}
type subjectKey struct{}

func WithSubject(ctx context.Context, s *Subject) context.Context {
	return context.WithValue(ctx, subjectKey{}, s)
}
func SubjectFrom(ctx context.Context) *Subject { s, _ := ctx.Value(subjectKey{}).(*Subject); return s }
func (s *Subject) add(a Amount, failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Total.Input += a.Input
	s.Total.Output += a.Output
	s.Total.Cost += a.Cost
	s.Total.Unknown += a.Unknown
	s.Total.Estimated += a.Estimated
	s.StorageFailure = s.StorageFailure || failed
}
func (s *Subject) Fail(err error)       { s.mu.Lock(); defer s.mu.Unlock(); s.Failure = err }
func (s *Subject) BudgetFailure() error { s.mu.Lock(); defer s.mu.Unlock(); return s.Failure }
func (s *Subject) Snapshot() (Amount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Total, s.StorageFailure
}
func Estimate(body []byte, path string, model catalog.Descriptor) (Amount, bool) {
	input := int64(len(body))
	output := int64(0)
	supported := true
	// Typed classifiers can report output usage too; only embeddings have a
	// protocol-defined zero generation bound.
	if path != "/v1/embeddings" {
		output = 65536
		supported = false
		if model.ContextWindow != nil && *model.ContextWindow > 0 {
			output = *model.ContextWindow
			supported = true
		}
		if model.MaxOutputTokens != nil && *model.MaxOutputTokens > 0 {
			output = *model.MaxOutputTokens
			supported = true
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) == nil {
			for _, key := range []string{"max_completion_tokens", "max_output_tokens", "max_tokens"} {
				if raw, ok := fields[key]; ok {
					var v int64
					if json.Unmarshal(raw, &v) == nil && v > 0 && v <= 1e12 {
						output = v
						supported = true
					} else {
						supported = false
					}
					break
				}
			}
		}
	}
	if bytesContainsImage(body) {
		if model.ContextWindow != nil {
			input = *model.ContextWindow
		} else {
			supported = false
		}
	}
	a := Amount{Input: input, Output: output, Estimated: input + output}
	if model.Pricing.InputPerMillion == nil || model.Pricing.OutputPerMillion == nil {
		a.Unknown = 1
	} else {
		a.Cost = (float64(input)**model.Pricing.InputPerMillion + float64(output)**model.Pricing.OutputPerMillion) / 1e6
	}
	return a, supported
}
func bytesContainsImage(body []byte) bool {
	text := string(body)
	return strings.Contains(text, `"image_url"`) || strings.Contains(text, `"input_image"`) || strings.Contains(text, `"audio"`)
}

type Metered struct {
	id       string
	base     provider.OpenAIProvider
	Store    *Store
	Settings Settings
	Describe func(string) catalog.Descriptor
}
type meteredDecision struct {
	*Metered
	decision provider.DecisionProvider
}

func Wrap(base provider.OpenAIProvider, store *Store, settings Settings, describe func(string) catalog.Descriptor) provider.OpenAIProvider {
	m := &Metered{id: base.ID(), base: base, Store: store, Settings: settings, Describe: describe}
	if p, ok := base.(provider.DecisionProvider); ok {
		return &meteredDecision{Metered: m, decision: p}
	}
	return m
}
func (m *Metered) ID() string                                           { return m.id }
func (m *Metered) Models(ctx context.Context) ([]provider.Model, error) { return m.base.Models(ctx) }
func (m *Metered) ClassifierOnly() bool {
	if p, ok := m.base.(interface{ ClassifierOnly() bool }); ok {
		return p.ClassifierOnly()
	}
	return false
}
func (m *Metered) Quota(ctx context.Context, model string) (quota.Status, error) {
	if q, ok := m.base.(quota.Fetcher); ok {
		return q.Quota(ctx, model)
	}
	return quota.Status{}, errors.New("provider quota unknown")
}
func (m *Metered) Forward(ctx context.Context, path string, body []byte, model string) (*http.Response, error) {
	return m.forward(ctx, path, body, model, func() (*http.Response, error) { return m.base.Forward(ctx, path, body, model) })
}
func (m *meteredDecision) ForwardDecision(ctx context.Context, body []byte) (*http.Response, error) {
	var fields struct {
		Model string `json:"model"`
	}
	json.Unmarshal(body, &fields)
	return m.forward(ctx, "/v1/systemone", body, fields.Model, func() (*http.Response, error) { return m.decision.ForwardDecision(ctx, body) })
}
func (m *Metered) forward(ctx context.Context, path string, body []byte, model string, dispatch func() (*http.Response, error)) (*http.Response, error) {
	subject := SubjectFrom(ctx)
	if subject == nil || m.Store == nil {
		return dispatch()
	}
	id := m.ID() + "/" + model
	descriptor := m.Describe(id)
	estimate, supported := Estimate(body, path, descriptor)
	if !supported && (m.Settings.TokenLimited(subject.Key) || m.Settings.CostLimited(subject.Key)) {
		err := UnknownCapacity()
		subject.Fail(err)
		return nil, err
	}
	reservation, err := m.Store.Reserve(subject.Key, subject.Name, m.ID(), id, m.Settings, estimate)
	if err != nil {
		if m.Settings.Limited(subject.Key) {
			subject.Fail(err)
			return nil, err
		}
		slog.Warn("operator_accounting_unavailable")
		subject.add(estimate, true)
		return dispatch()
	}
	response, err := dispatch()
	if err != nil || response == nil || response.Body == nil {
		m.Store.Settle(reservation, nil)
		subject.add(estimate, false)
		return response, err
	}
	stream := strings.Contains(response.Header.Get("Content-Type"), "text/event-stream")
	var fields struct {
		Stream bool `json:"stream"`
	}
	json.Unmarshal(body, &fields)
	stream = stream || fields.Stream
	observer := NewObserver(stream)
	observer.noOutput = path == "/v1/embeddings"
	response.Body = &observedBody{ReadCloser: response.Body, observer: observer, complete: func(o *Observer, eof bool) {
		amount := estimate
		var actual *Amount
		if eof && o.Reported {
			amount = Amount{Input: o.Input, Output: o.Output, Unknown: estimate.Unknown}
			if amount.Unknown == 0 {
				amount.Cost = (float64(o.Input)**descriptor.Pricing.InputPerMillion + float64(o.Output)**descriptor.Pricing.OutputPerMillion) / 1e6
			}
			actual = &amount
		}
		err := m.Store.Settle(reservation, actual)
		subject.add(amount, err != nil)
		if err != nil {
			slog.Warn("operator_settlement_unavailable")
		}
	}}
	return response, nil
}
