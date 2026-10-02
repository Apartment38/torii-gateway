package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/config"
)

func TestExampleConfigurationIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open example configuration: %v", err)
	}
	defer file.Close()

	cfg, err := config.Decode(file)
	if err != nil {
		t.Fatalf("decode example configuration: %v", err)
	}
	if cfg.Server.Authentication != nil {
		t.Fatal("example must use database-managed administrator setup, not an environment API key")
	}
	if got, want := len(cfg.Providers), 1; got != want {
		t.Fatalf("example provider count = %d, want %d", got, want)
	}
}
