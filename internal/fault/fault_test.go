package fault_test

import (
	"errors"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/fault"
)

func TestTypedFaultWrapsCause(t *testing.T) {
	cause := errors.New("provider detail")
	err := fault.Wrap(fault.ProviderUnavailable, "call provider", cause)
	if kind, ok := fault.KindOf(err); !ok || kind != fault.ProviderUnavailable {
		t.Fatalf("KindOf() = %q, %v", kind, ok)
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not retained")
	}
}
