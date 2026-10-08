package accounting

import (
	"bytes"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/state"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, *state.Database, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "torii.db")
	db, err := state.OpenDatabase(path, bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); db.Close() })
	return s, db, path
}
func TestConcurrentReservationsSettlementAndRestart(t *testing.T) {
	s, db, path := testStore(t)
	settings := Settings{Keys: map[string]Budgets{"key": {Daily: Budget{Tokens: 100}}}}
	var wg sync.WaitGroup
	results := make(chan Reservation, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Reserve("key", "name", "p", "p/model", settings, Amount{Input: 10, Output: 10, Estimated: 20})
			if err == nil {
				results <- r
			}
		}()
	}
	wg.Wait()
	close(results)
	reservations := []Reservation{}
	for r := range results {
		reservations = append(reservations, r)
	}
	if len(reservations) != 5 {
		t.Fatal("atomic quota exceeded", len(reservations))
	}
	if err := s.Settle(reservations[0], &Amount{Input: 2, Output: 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.Settle(reservations[0], &Amount{Input: 2, Output: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve("key", "name", "p", "p/model", settings, Amount{Input: 15}); err != nil {
		t.Fatal("settlement did not release unused budget", err)
	}
	s.Close()
	db.Close()
	restored, err := state.OpenDatabase(path, bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restarted, err := Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err := restarted.Reserve("key", "name", "p", "p/model", settings, Amount{Input: 1}); err == nil {
		t.Fatal("restart reset reserved quota")
	}
}
func TestUTCRolloverGatewayAndUnknownCost(t *testing.T) {
	s, _, _ := testStore(t)
	now := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	s.clock = func() time.Time { return now }
	settings := Settings{Gateway: Budgets{Daily: Budget{Requests: 1}, Monthly: Budget{Requests: 1}}}
	if _, err := s.Reserve("a", "a", "", "", settings, Amount{Requests: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve("b", "b", "", "", settings, Amount{Requests: 1}); err == nil {
		t.Fatal("gateway budget bypass")
	}
	now = now.Add(time.Second)
	if _, err := s.Reserve("b", "b", "", "", settings, Amount{Requests: 1}); err != nil {
		t.Fatal("UTC rollover failed", err)
	}
	settings = Settings{Keys: map[string]Budgets{"priced": {Daily: Budget{EstimatedCost: 1}}}}
	if _, err := s.Reserve("priced", "priced", "p", "p/model", settings, Amount{Unknown: 1}); err == nil {
		t.Fatal("unknown cost admitted")
	}
	s.Close()
	if _, err := s.Reserve("a", "a", "p", "p/model", settings, Amount{}); err != ErrStorage {
		t.Fatal("storage failed open", err)
	}
}
func TestObserverFragmentedJSONSSEZeroUsageAndBounds(t *testing.T) {
	for _, tc := range []struct {
		stream        bool
		body          string
		input, output int64
	}{{false, `{"usage":{"prompt_tokens":2,"completion_tokens":3}}`, 2, 3}, {true, "data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\r\n\r\ndata: [DONE]\n\n", 2, 3}, {true, "event: response.completed\ndata: {\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":5}}}\n\n", 4, 5}, {true, "data: {\"message\":{\"usage\":{\"input_tokens\":7}}}\n\ndata: {\"usage\":{\"output_tokens\":8}}\n\ndata: [DONE]\n\n", 7, 8}, {false, `{"usage":{"prompt_tokens":0,"completion_tokens":0}}`, 0, 0}, {false, `{"usage":{"prompt_tokens":2,"total_tokens":5}}`, 2, 3}} {
		o := NewObserver(tc.stream)
		for i := range len(tc.body) {
			o.Write([]byte(tc.body[i : i+1]))
		}
		o.Finish()
		if !o.Reported || o.Input != tc.input || o.Output != tc.output {
			t.Fatal("usage framing", tc, o)
		}
	}
	o := NewObserver(true)
	o.Write(bytes.Repeat([]byte{'x'}, observationLimit*2))
	if len(o.buffer) > observationLimit {
		t.Fatal("unbounded observation")
	}
	o.Finish()
	if o.Reported {
		t.Fatal("invented usage")
	}
}
func TestEstimatesReserveSupportedMaximumWithoutOutputLimit(t *testing.T) {
	window, maxOutput := int64(8192), int64(1024)
	zero := float64(0)
	model := catalog.Descriptor{ContextWindow: &window, MaxOutputTokens: &maxOutput, Pricing: catalog.Pricing{InputPerMillion: &zero, OutputPerMillion: &zero}}
	amount, supported := Estimate([]byte(`{"messages":[]}`), "/v1/chat/completions", model)
	if !supported || amount.Output != 1024 {
		t.Fatal("omitted output limit underreserved", amount)
	}
	classifierAmount, classifierSupported := Estimate([]byte(`{}`), "/v1/systemone", model)
	if !classifierSupported || classifierAmount.Output != 1024 {
		t.Fatal("classifier output underreserved", classifierAmount)
	}
	amount, supported = Estimate([]byte(`{"max_output_tokens":10}`), "/v1/responses", model)
	if !supported || amount.Output != 10 {
		t.Fatal("explicit output limit ignored")
	}
	if _, supported = Estimate([]byte(`{}`), "/v1/chat/completions", catalog.Descriptor{}); supported {
		t.Fatal("unknown capacity invented")
	}
}

func TestUsageTotalsRespectKnownOutputAndUnknownSplits(t *testing.T) {
	for _, tc := range []struct {
		name, body                  string
		embedding, stream, reported bool
		input, output               int64
	}{
		{"embedding", `{"usage":{"prompt_tokens":5,"total_tokens":5}}`, true, false, true, 5, 0},
		{"embedding total", `{"usage":{"total_tokens":9}}`, true, false, true, 9, 0},
		{"embedding input", `{"usage":{"input_tokens":5}}`, true, false, true, 5, 0},
		{"ambiguous generation", `{"usage":{"total_tokens":9}}`, false, false, false, 0, 0},
		{"contradictory totals", `{"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":9}}`, false, false, false, 2, 3},
		{"output and total", `{"usage":{"completion_tokens":3,"total_tokens":5}}`, false, false, true, 2, 3},
		{"cumulative SSE", "data: {\"usage\":{\"input_tokens\":2,\"total_tokens\":2}}\n\ndata: {\"usage\":{\"total_tokens\":10}}\n\ndata: [DONE]\n\n", false, true, true, 2, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := NewObserver(tc.stream)
			o.noOutput = tc.embedding
			for _, b := range []byte(tc.body) {
				o.Write([]byte{b})
			}
			o.Finish()
			if o.Reported != tc.reported || o.Input != tc.input || o.Output != tc.output {
				t.Fatalf("usage interpretation: %+v", o)
			}
		})
	}
}

func TestStreamingIncompleteAndStaleUsageKeepsEstimates(t *testing.T) {
	for _, body := range []string{
		"data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":0}}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"new output\"}}]}\n\ndata: [DONE]\n\n",
		"data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":0,\"total_tokens\":2}}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"new output\"}}]}\n\ndata: [DONE]\n\n",
		"data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\n",
	} {
		o := NewObserver(true)
		for _, b := range []byte(body) {
			o.Write([]byte{b})
		}
		o.Finish()
		if o.Reported {
			t.Fatal("incomplete or stale streaming usage treated as final")
		}
	}
}

func TestUsageEnvelopeDepthIsBounded(t *testing.T) {
	payload := strings.Repeat(`{"response":`, 100) + `{"usage":{"input_tokens":2,"output_tokens":3}}` + strings.Repeat(`}`, 100)
	observer := NewObserver(false)
	observer.Write([]byte(payload))
	observer.Finish()
	if observer.Reported {
		t.Fatal("unsupported deep envelope trusted as reported usage")
	}
}
