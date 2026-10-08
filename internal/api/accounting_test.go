package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/state"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type accountingFixtureProvider struct {
	calls   int
	stream  bool
	missing bool
	broken  bool
	retry   bool
}

func (*accountingFixtureProvider) ID() string { return "p" }
func (*accountingFixtureProvider) Models(context.Context) ([]provider.Model, error) {
	window := int64(1024)
	return []provider.Model{{ID: "p/model", ContextWindow: &window}, {ID: "p/alternate", ContextWindow: &window}}, nil
}

type interruptedBody struct{ io.Reader }

func (b interruptedBody) Read(p []byte) (int, error) {
	n, _ := b.Reader.Read(p)
	return n, errors.New("cancelled")
}
func (b interruptedBody) Close() error { return nil }
func (p *accountingFixtureProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	p.calls++
	status := 200
	body := `{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3}}`
	headers := http.Header{}
	if p.stream {
		headers.Set("Content-Type", "text/event-stream")
		body = "data: {\"choices\":[{\"delta\":{\"content\":\"secret response\"}}]}\n\ndata: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n"
	}
	if p.missing {
		body = `{"choices":[]}`
	}
	if p.retry && p.calls == 1 {
		status = 500
	}
	response := &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
	if p.broken {
		response.Body = interruptedBody{strings.NewReader(body)}
	}
	return response, nil
}
func accountingHandler(t *testing.T, p *accountingFixtureProvider, settings accounting.Settings) (*OpenAIHandler, *accounting.Store) {
	t.Helper()
	db, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := accounting.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	zero := float64(0)
	h, err := NewOpenAIHandlerWithOverrides([]provider.OpenAIProvider{p}, []catalog.Descriptor{{PublicID: "p/alternate", ProviderID: "p", UpstreamID: "alternate", Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Pricing: catalog.Pricing{InputPerMillion: &zero, OutputPerMillion: &zero}}, {PublicID: "p/model", ProviderID: "p", UpstreamID: "model", Endpoints: catalog.EndpointCapabilities{ChatCompletions: catalog.Supported}, Pricing: catalog.Pricing{InputPerMillion: &zero, OutputPerMillion: &zero}}})
	if err != nil {
		t.Fatal(err)
	}
	h.SetModelControls(nil, map[string]bool{"p": true})
	h.RefreshCatalog(context.Background())
	h.ConfigureAccounting(store, settings, []provider.OpenAIProvider{p})
	return h, store
}
func TestAccountingJSONSSEMissingUsageCancellationAndRetries(t *testing.T) {
	for index, tc := range []struct{ stream, missing, broken, retry bool }{{}, {true, false, false, false}, {false, true, false, false}, {false, false, true, false}, {false, false, false, true}} {
		t.Run(fmt.Sprintf("case%d", index), func(t *testing.T) {
			p := &accountingFixtureProvider{stream: tc.stream, missing: tc.missing, broken: tc.broken, retry: tc.retry}
			h, store := accountingHandler(t, p, accounting.Settings{})
			model := "p/model"
			if tc.retry {
				model = "smart" // retry requires another eligible physical model; first failure is conservatively retained
			}
			body := `{"model":"` + model + `","max_tokens":10,"messages":[{"role":"user","content":"private prompt"}]`
			if tc.stream {
				body += `,"stream":true`
			}
			body += `}`
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), clientIdentityContextKey{}, &ClientIdentity{ID: "key-a", Name: "Client A"}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			summaries, _, err := store.Summary(accounting.Settings{})
			if err != nil {
				t.Fatal(err)
			}
			requests, input, output, estimated := int64(0), int64(0), int64(0), int64(0)
			period := time.Now().UTC().Format("2006-01-02")
			for _, row := range summaries {
				if row.Key == "key-a" && row.Period == period {
					requests += row.Requests
					input += row.Input
					output += row.Output
					estimated += row.Estimated
				}
			}
			if requests != 1 {
				t.Fatal("client request counted incorrectly", requests)
			}
			if tc.missing || tc.broken || tc.retry {
				if estimated == 0 || input+output < 10 {
					t.Fatal("conservative reservation lost", summaries)
				}
			} else if input != 2 || output != 3 || estimated != 0 {
				t.Fatal("reported usage not reconciled", summaries)
			}
			if tc.stream && !strings.HasSuffix(w.Body.String(), "data: [DONE]\n\n") {
				t.Fatal("SSE payload changed")
			}
		})
	}
}
func TestQuotaEnforcesRequestTokenCostAndStorageBeforeDispatch(t *testing.T) {
	settings := accounting.Settings{Keys: map[string]accounting.Budgets{"anonymous": {Daily: accounting.Budget{Requests: 1}}}}
	p := &accountingFixtureProvider{}
	h, store := accountingHandler(t, p, settings)
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"p/model","max_tokens":10,"messages":[]}`)))
		return w
	}
	if call().Code != 200 {
		t.Fatal("initial request failed")
	}
	w := call()
	if w.Code != 429 || p.calls != 1 || w.Header().Get("X-Quota-Reset") == "" {
		t.Fatal("request budget bypass", w.Code, p.calls)
	}
	store.Close()
	w = call()
	if w.Code != 503 || p.calls != 1 {
		t.Fatal("quota storage failed open", w.Code)
	}
	p = &accountingFixtureProvider{}
	settings = accounting.Settings{Gateway: accounting.Budgets{Daily: accounting.Budget{Tokens: 1}}}
	h, _ = accountingHandler(t, p, settings)
	if call().Code != 429 || p.calls != 0 {
		t.Fatal("token budget bypass")
	}
	h.budgets = accounting.Settings{Gateway: accounting.Budgets{Daily: accounting.Budget{EstimatedCost: 1}}}
	h.ConfigureAccounting(h.accounting, h.budgets, []provider.OpenAIProvider{p})
	h.freeProviders = nil // remove reliable catalog pricing for this fixture
	h.catalog, _ = catalog.New([]provider.Provider{p}, []catalog.Descriptor{}, nil)
	h.RefreshCatalog(context.Background())
	if call().Code != 429 || p.calls != 0 {
		t.Fatal("unknown pricing admitted")
	}
}
