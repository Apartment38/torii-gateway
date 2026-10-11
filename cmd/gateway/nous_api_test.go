package main

import (
	"github.com/Apartment38/torii-gateway/internal/config"
	"testing"
)

func TestBuildNousAPIKeyProvider(t *testing.T) {
	_, err := buildProviderWithState(config.ProviderConfig{ID: "nous", Type: "nous", BaseURL: "https://inference-api.nousresearch.com/v1", BearerToken: &config.SecretRef{Value: "fixture-key"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
