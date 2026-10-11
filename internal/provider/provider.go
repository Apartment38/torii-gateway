package provider

import (
	"context"
	"net/http"
)

// Model is the OpenAI-compatible metadata exposed for a physical model.
type Model struct {
	ID               string          `json:"id"`
	Object           string          `json:"object"`
	Created          int64           `json:"created"`
	OwnedBy          string          `json:"owned_by"`
	ContextWindow    *int64          `json:"context_window,omitempty"`
	InputPerMillion  *float64        `json:"input_per_million,omitempty"`
	OutputPerMillion *float64        `json:"output_per_million,omitempty"`
	Subscription     bool            `json:"subscription,omitempty"`
	PricingSource    string          `json:"-"`
	Discovery        *ModelDiscovery `json:"-"`
}

// ModelDiscovery carries verified adapter facts into the catalog without
// changing the OpenAI model-list wire format. Missing keys remain unknown.
type ModelDiscovery struct {
	Endpoints    map[string]bool
	Capabilities map[string]bool
	Local        *bool
	Source       string
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

// DecisionProvider exposes a typed-decision transport such as Jev's
// System One endpoint without pretending that endpoint is chat-compatible.
type DecisionProvider interface {
	ForwardDecision(context.Context, []byte) (*http.Response, error)
}
