package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
)

func (r *handlerRuntime) setModel(w http.ResponseWriter, req *http.Request) {
	var input struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Enabled == nil {
		writeConfigError(w, 400, errors.New("provide model id and enabled"))
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
	cfg.ModelEnabled[input.ID] = *input.Enabled
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
