package router

import (
	"fmt"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

// CustomRouter exposes a restricted candidate pool as a virtual model.
// Providers and Models are additive: a provider includes all its models.
type CustomRouter struct {
	Name            string   `yaml:"name"`
	Providers       []string `yaml:"providers,omitempty"`
	Models          []string `yaml:"models,omitempty"`
	AllowPaidModels *bool    `yaml:"allow_paid_models,omitempty"`
	AllowFreeModels *bool    `yaml:"allow_free_models,omitempty"`
}

func (r CustomRouter) Validate() error {
	if r.Name == "smart" {
		return fmt.Errorf("name smart is reserved")
	}
	if err := provider.ValidateID(r.Name); err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if len(r.Providers)+len(r.Models) == 0 {
		return fmt.Errorf("select at least one provider or model")
	}
	for _, id := range r.Providers {
		if err := provider.ValidateID(id); err != nil {
			return err
		}
	}
	for _, id := range r.Models {
		if _, _, err := provider.ParseModelID(id); err != nil {
			return err
		}
	}
	return nil
}

func (r CustomRouter) Includes(publicID string) bool {
	providerID, _, err := provider.ParseModelID(publicID)
	if err != nil {
		return false
	}
	for _, id := range r.Providers {
		if id == providerID {
			return true
		}
	}
	for _, id := range r.Models {
		if id == publicID {
			return true
		}
	}
	return false
}
