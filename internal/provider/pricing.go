package provider

import (
	"encoding/json"
	"math"
	"strconv"
)

// Prices in OpenRouter/Portal catalogs are USD per token. Invalid or sentinel
// values remain unknown, including negative automatic-routing prices.
func tokenPrice(raw json.RawMessage) *float64 {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		text = string(raw)
	}
	v, err := strconv.ParseFloat(text, 64)
	v *= 1e6
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func applyCatalogMetadata(raw json.RawMessage, model *Model) {
	var entry struct {
		ContextLength *int64                                       `json:"context_length"`
		BillingMode   string                                       `json:"billing_mode"`
		Pricing       struct{ Prompt, Completion json.RawMessage } `json:"pricing"`
	}
	if json.Unmarshal(raw, &entry) != nil {
		return
	}
	if entry.ContextLength != nil && *entry.ContextLength > 0 {
		model.ContextWindow = entry.ContextLength
	}
	if input := tokenPrice(entry.Pricing.Prompt); input != nil {
		model.InputPerMillion = input
		model.PricingSource = "provider_model_catalog"
	}
	if output := tokenPrice(entry.Pricing.Completion); output != nil {
		model.OutputPerMillion = output
		model.PricingSource = "provider_model_catalog"
	}
	model.Subscription = model.Subscription || entry.BillingMode == "subscription"
	if model.Subscription {
		zero := float64(0)
		model.InputPerMillion, model.OutputPerMillion = &zero, &zero
	}
}
