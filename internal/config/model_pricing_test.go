package config_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/config"
)

func TestModelPricingCSV(t *testing.T) {
	prices, err := config.ParsePricingCSV(strings.NewReader(config.PricingCSVTemplate))
	if err != nil || *prices["provider/model"].InputPerMillion != .25 || *prices["provider/model"].OutputPerMillion != 1 {
		t.Fatalf("template: %+v %v", prices, err)
	}
	prices, err = config.ParsePricingCSV(strings.NewReader("\ufeffmodel_id,input_per_million,output_per_million\ncloud/model,,0\ncloud/automatic,,\n"))
	if err != nil || prices["cloud/model"].InputPerMillion != nil || *prices["cloud/model"].OutputPerMillion != 0 || prices["cloud/automatic"].OutputPerMillion != nil {
		t.Fatalf("blank and zero semantics: %+v %v", prices, err)
	}
	for _, input := range []string{
		"id,input,output\nx,1,2\n",
		"model_id,input_per_million,output_per_million\n",
		strings.Replace(config.PricingCSVTemplate, "0.25", "-1", 1),
		strings.Replace(config.PricingCSVTemplate, "0.25", "NaN", 1),
		strings.Replace(config.PricingCSVTemplate, "0.25", "Inf", 1),
		strings.Replace(config.PricingCSVTemplate, "provider/model", "not-qualified", 1),
		config.PricingCSVTemplate + "provider/model,1,2\n",
		strings.Replace(config.PricingCSVTemplate, "0.25,1.00", "0.25", 1),
	} {
		if _, err := config.ParsePricingCSV(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid CSV accepted: %s", input)
		}
	}
	var large strings.Builder
	large.WriteString("model_id,input_per_million,output_per_million\n")
	for i := 0; i <= config.MaxPricingCSVRows; i++ {
		large.WriteString(fmt.Sprintf("p/model-%d,1,2\n", i))
	}
	if _, err := config.ParsePricingCSV(strings.NewReader(large.String())); err == nil {
		t.Fatal("oversized CSV accepted")
	}
}
func TestModelPricingValidation(t *testing.T) {
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1)} {
		if err := config.ValidateModelPricing(map[string]catalog.Pricing{"p/model": {InputPerMillion: &rate}}); err == nil {
			t.Fatal("invalid price accepted")
		}
	}
	cfg, err := config.Decode(strings.NewReader(validConfig + "model_pricing:\n  p/model:\n    input_per_million: 0\n    output_per_million: 1.5\n"))
	if err != nil || *cfg.ModelPricing["p/model"].OutputPerMillion != 1.5 {
		t.Fatalf("pricing config: %+v %v", cfg, err)
	}
}
