package config

import (
	"bytes"
	"github.com/Apartment38/torii-gateway/internal/state"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const credentialsConfig = "server:\n  listen: 127.0.0.1:8080\nproviders:\n  - id: p\n    type: openai_compatible\n    base_url: https://example.test\n    discover_models: false\n    models: [model]\n"

func TestPersistedCredentialsRejectLegacyInputsWithoutChangingFiles(t *testing.T) {
	for _, input := range []string{"value: credential-sentinel", "env: credential-sentinel", "file: credential-sentinel"} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			raw := []byte(credentialsConfig + "    bearer_token: {" + input + "}\n")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || strings.Contains(err.Error(), "credential-sentinel") {
				t.Fatal("unsafe persisted input accepted or echoed", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("legacy YAML modified", err)
			}
		})
	}
}
func TestSubmittedCredentialsEncryptedAndExportedAsReferences(t *testing.T) {
	db, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg, err := Decode(strings.NewReader(credentialsConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers[0].BearerToken = &SecretRef{Value: "credential-sentinel"}
	if err := StoreSubmittedCredentials(cfg, db); err != nil {
		t.Fatal(err)
	}
	id := cfg.Providers[0].BearerToken.Credential
	if id == "" || cfg.Providers[0].BearerToken.Value != "" {
		t.Fatal("direct input retained")
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil || bytes.Contains(raw, []byte("credential-sentinel")) {
		t.Fatal("plaintext export", err)
	}
	if err := cfg.BindSecrets(db); err != nil {
		t.Fatal(err)
	}
	got, err := cfg.Providers[0].BearerToken.Resolve()
	if err != nil || got != "credential-sentinel" {
		t.Fatal("stored reference", err)
	}
	cfg.Providers[0].BearerToken = &SecretRef{Credential: "__admin"}
	if err := cfg.BindSecrets(db); err == nil {
		t.Fatal("system reference accepted")
	}
}
func TestConfigurationParsingErrorsNeverEchoCredentials(t *testing.T) {
	for _, raw := range []string{"server:\n  authentication: credential-sentinel\n", "credential-sentinel: true\n", strings.Replace(credentialsConfig, "https://example.test", "https://user:credential-sentinel@example.test/%zz", 1)} {
		_, err := Decode(strings.NewReader(raw))
		if err == nil || strings.Contains(err.Error(), "credential-sentinel") {
			t.Fatal("credential-bearing parser error")
		}
	}
}
