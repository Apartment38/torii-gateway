package config

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

const PricingCSVTemplate = "model_id,input_per_million,output_per_million\nprovider/model,0.25,1.00\n"
const MaxPricingCSVRows = 1000

func ValidateModelPricing(prices map[string]catalog.Pricing) error {
	for id, price := range prices {
		if len(id) > 512 {
			return errors.New("pricing model IDs must be at most 512 bytes")
		}
		if _, _, err := provider.ParseModelID(id); err != nil {
			return errors.New("model_pricing requires provider-qualified model IDs")
		}
		for _, rate := range []*float64{price.InputPerMillion, price.OutputPerMillion} {
			if rate != nil && (*rate < 0 || math.IsNaN(*rate) || math.IsInf(*rate, 0)) {
				return errors.New("model pricing requires finite nonnegative USD rates per million tokens")
			}
		}
	}
	return nil
}

// ParsePricingCSV returns replacements for the specified IDs. Blank rates restore
// automatic metadata for that field; both blank removes the manual override.
func ParsePricingCSV(reader io.Reader) (map[string]catalog.Pricing, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil || len(header) != 3 || strings.TrimPrefix(header[0], "\ufeff") != "model_id" || header[1] != "input_per_million" || header[2] != "output_per_million" {
		return nil, errors.New("CSV header must be model_id,input_per_million,output_per_million")
	}
	prices := map[string]catalog.Pricing{}
	for row := 2; ; row++ {
		values, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("pricing CSV row %d is malformed", row)
		}
		if len(prices) >= MaxPricingCSVRows {
			return nil, errors.New("pricing CSV supports at most 1000 rows")
		}
		id := strings.TrimSpace(values[0])
		if _, exists := prices[id]; exists {
			return nil, fmt.Errorf("pricing CSV row %d duplicates a model ID", row)
		}
		price := catalog.Pricing{}
		for i, dest := range []**float64{&price.InputPerMillion, &price.OutputPerMillion} {
			raw := strings.TrimSpace(values[i+1])
			if raw == "" {
				continue
			}
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, fmt.Errorf("pricing CSV row %d has an invalid rate", row)
			}
			*dest = &value
		}
		if err := ValidateModelPricing(map[string]catalog.Pricing{id: price}); err != nil {
			return nil, fmt.Errorf("pricing CSV row %d: %w", row, err)
		}
		prices[id] = price
	}
	if len(prices) == 0 {
		return nil, errors.New("pricing CSV contains no model rows")
	}
	return prices, nil
}
