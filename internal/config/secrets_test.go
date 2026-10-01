package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
)

func TestSecretRefValidateRequiresExactlyOneSource(t *testing.T) {
	tests := []struct {
		name string
		ref  config.SecretRef
	}{
		{name: "neither source"},
		{name: "both sources", ref: config.SecretRef{Env: "API_KEY", File: "/secret"}},
		{name: "blank environment variable", ref: config.SecretRef{Env: "  "}},
		{name: "blank file", ref: config.SecretRef{File: "\t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ref.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}

func TestSecretRefResolveEnvironment(t *testing.T) {
	t.Setenv("TEST_GATEWAY_SECRET", "  environment value  ")
	got, err := (config.SecretRef{Env: "TEST_GATEWAY_SECRET"}).Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := "  environment value  "; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestSecretRefResolveMissingEnvironment(t *testing.T) {
	const name = "TEST_GATEWAY_MISSING_SECRET"
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	_, err := (config.SecretRef{Env: name}).Resolve()
	if err == nil || !strings.Contains(err.Error(), "is not set") {
		t.Fatalf("Resolve() error = %v, want missing-variable error", err)
	}
}

func TestSecretRefResolveFileTrimsWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(" \tfile value\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := (config.SecretRef{File: path}).Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := "file value"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}
