package provider

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed tracking_prices.json
var trackingPricesJSON []byte

var trackingPrices = func() struct {
	UpdatedAt string                                                `json:"updated_at"`
	Models    map[string]map[string]struct{ Input, Output float64 } `json:"models"`
} {
	var data struct {
		UpdatedAt string                                                `json:"updated_at"`
		Models    map[string]map[string]struct{ Input, Output float64 } `json:"models"`
	}
	if err := json.Unmarshal(trackingPricesJSON, &data); err != nil {
		panic("invalid bundled tracking prices")
	}
	return data
}()

func applyTrackingPrice(kind string, model *Model) {
	if model.InputPerMillion != nil && model.OutputPerMillion != nil {
		return
	}
	id := strings.TrimPrefix(model.ID, strings.SplitN(model.ID, "/", 2)[0]+"/")
	price, ok := trackingPrices.Models[kind][id]
	if !ok {
		return
	}
	if model.InputPerMillion == nil {
		v := price.Input
		model.InputPerMillion = &v
	}
	if model.OutputPerMillion == nil {
		v := price.Output
		model.OutputPerMillion = &v
	}
	model.PricingSource = "models.dev snapshot " + trackingPrices.UpdatedAt + "; base token rates"
}
