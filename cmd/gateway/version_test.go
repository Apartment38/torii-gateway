package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/buildinfo"
)

func TestVersionRequiresNoConfigurationOrMasterKey(t *testing.T) {
	t.Setenv("TORII_DB_KEY_FILE", "/missing-secret")
	t.Setenv("TORII_DB_KEY", "invalid")
	old := buildinfo.Version
	buildinfo.Version = "v2.0.0-test"
	defer func() { buildinfo.Version = old }()
	var output bytes.Buffer
	if err := runWithIO(context.Background(), []string{"version"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"v2.0.0-test", "Database schema: 4", "supported baseline: 4", "Encryption key version: 1"} {
		if !strings.Contains(output.String(), want) {
			t.Fatal("missing version metadata", want)
		}
	}
	if err := runWithIO(context.Background(), []string{"version", "extra"}, &output); err == nil {
		t.Fatal("extra arguments accepted")
	}
}
