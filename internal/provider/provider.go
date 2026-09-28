package provider

import (
	"context"
	"net/http"
)

// Model is the OpenAI-compatible metadata exposed for a physical model.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// Provider discovers models from one configured upstream.
type Provider interface {
	ID() string
	Models(context.Context) ([]Model, error)
}

// OpenAIProvider supports the transparent OpenAI-compatible inference
// endpoints exposed by the gateway.
type OpenAIProvider interface {
	Provider
	Forward(context.Context, string, []byte, string) (*http.Response, error)
}
