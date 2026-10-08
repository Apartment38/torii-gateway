package accounting

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/catalog"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestClassifierTransportPreservesLargePayload(t *testing.T) {
	store, _, _ := testStore(t)
	payload := []byte(`{"model":"decision","input":"` + strings.Repeat("x", 3<<20) + `"}`)
	request, err := http.NewRequest("POST", "https://example.test/v1/systemone", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request = request.WithContext(WithSubject(request.Context(), &Subject{Key: "key", Name: "test"}))
	transport := Transport{Store: store, Describe: func(string) catalog.Descriptor { return catalog.Descriptor{} }, Base: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		forwarded, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(forwarded, payload) {
			t.Fatal("classifier body changed", err, len(forwarded))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":3,"output_tokens":0}}`))}, nil
	})}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	total, _ := SubjectFrom(request.Context()).Snapshot()
	if total.Input != 3 || total.Estimated != 0 {
		t.Fatalf("classifier reconciliation: %+v", total)
	}
}

func TestAmbiguousTotalRetainsExpensiveOutputBudget(t *testing.T) {
	store, _, _ := testStore(t)
	input, output := float64(0), float64(1000)
	meter := Metered{id: "p", Store: store, Settings: Settings{Gateway: Budgets{Daily: Budget{EstimatedCost: .015}}}, Describe: func(string) catalog.Descriptor {
		return catalog.Descriptor{Pricing: catalog.Pricing{InputPerMillion: &input, OutputPerMillion: &output}}
	}}
	request, _ := http.NewRequest("POST", "https://example.test/v1/chat/completions", nil)
	ctx := WithSubject(request.Context(), &Subject{Key: "key", Name: "test"})
	calls := 0
	dispatch := func() (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"usage":{"total_tokens":5}}`))}, nil
	}
	response, err := meter.forward(ctx, "/v1/chat/completions", []byte(`{"max_tokens":10}`), "model", dispatch)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	amount, _ := SubjectFrom(ctx).Snapshot()
	if amount.Output != 10 || amount.Estimated == 0 || amount.Cost != .01 {
		t.Fatalf("ambiguous split understated cost: %+v", amount)
	}
	if _, err = meter.forward(ctx, "/v1/chat/completions", []byte(`{"max_tokens":10}`), "model", dispatch); err == nil || calls != 1 {
		t.Fatal("ambiguous usage released monetary budget", err, calls)
	}
}
