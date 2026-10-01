package usage_test

import (
	"github.com/Apartment38/torii-gateway/internal/usage"
	"testing"
)

func TestStorePersistsAggregate(t *testing.T) {
	dir := t.TempDir()
	store, err := usage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add("local/model", 10, 5, .25); err != nil {
		t.Fatal(err)
	}
	restarted, err := usage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := restarted.Snapshot()["local/model"]
	if got.Requests != 1 || got.InputTokens != 10 || got.OutputTokens != 5 || got.EstimatedCost != .25 {
		t.Fatalf("aggregate=%#v", got)
	}
}
