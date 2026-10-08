package accounting

import (
	"bytes"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/state"
	"path/filepath"
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
	}{{false, `{"usage":{"prompt_tokens":2,"completion_tokens":3}}`, 2, 3}, {true, "data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\r\n\r\ndata: [DONE]\n\n", 2, 3}, {true, "event: response.completed\ndata: {\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":5}}}\n\n", 4, 5}, {true, "data: {\"message\":{\"usage\":{\"input_tokens\":7}}}\n\ndata: {\"usage\":{\"output_tokens\":8}}\n\n", 7, 8}, {false, `{"usage":{"prompt_tokens":0,"completion_tokens":0}}`, 0, 0}, {false, `{"usage":{"total_tokens":9}}`, 9, 0}} {
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
	amount, supported = Estimate([]byte(`{"max_output_tokens":10}`), "/v1/responses", model)
	if !supported || amount.Output != 10 {
		t.Fatal("explicit output limit ignored")
	}
	if _, supported = Estimate([]byte(`{}`), "/v1/chat/completions", catalog.Descriptor{}); supported {
		t.Fatal("unknown capacity invented")
	}
}
