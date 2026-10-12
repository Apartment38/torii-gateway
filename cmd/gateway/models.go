package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
)

func (r *handlerRuntime) setModel(w http.ResponseWriter, req *http.Request) {
	var input struct {
		ID      string           `json:"id"`
		Enabled *bool            `json:"enabled"`
		Pricing *catalog.Pricing `json:"pricing"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || (input.Enabled == nil && input.Pricing == nil) {
		writeConfigError(w, 400, errors.New("provide model id and enabled or pricing"))
		return
	}
	if _, _, err := provider.ParseModelID(input.ID); err != nil {
		writeConfigError(w, 400, errors.New("provider-qualified model id required"))
		return
	}
	cfg, err := r.loadConfig()
	if err != nil {
		writeConfigError(w, 503, errors.New("configuration unavailable"))
		return
	}
	if cfg.ModelEnabled == nil {
		cfg.ModelEnabled = map[string]bool{}
	}
	if input.Enabled != nil {
		cfg.ModelEnabled[input.ID] = *input.Enabled
	}
	if input.Pricing != nil {
		if cfg.ModelPricing == nil {
			cfg.ModelPricing = map[string]catalog.Pricing{}
		}
		if input.Pricing.InputPerMillion == nil && input.Pricing.OutputPerMillion == nil {
			delete(cfg.ModelPricing, input.ID)
		} else {
			cfg.ModelPricing[input.ID] = *input.Pricing
		}
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		writeConfigError(w, 500, err)
		return
	}
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(data))
	clone.ContentLength = int64(len(data))
	r.putConfig(w, clone)
}
func (r *handlerRuntime) setBudgets(w http.ResponseWriter, req *http.Request) {
	var input accounting.Settings
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Validate() != nil {
		writeConfigError(w, 400, errors.New("invalid budgets"))
		return
	}
	cfg, err := r.loadConfig()
	if err != nil {
		writeConfigError(w, 503, errors.New("configuration unavailable"))
		return
	}
	cfg.Budgets = input
	data, err := yaml.Marshal(cfg)
	if err != nil {
		writeConfigError(w, 500, err)
		return
	}
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(data))
	clone.ContentLength = int64(len(data))
	r.putConfig(w, clone)
}

func (r *handlerRuntime) pricingCSVTemplate(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=model-pricing-template.csv")
	_, _ = io.WriteString(w, config.PricingCSVTemplate)
}
func (r *handlerRuntime) parsePricingCSV(w http.ResponseWriter, req *http.Request) {
	prices, err := config.ParsePricingCSV(http.MaxBytesReader(w, req.Body, 1<<20))
	if err != nil {
		writeConfigError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"model_pricing": prices})
}
