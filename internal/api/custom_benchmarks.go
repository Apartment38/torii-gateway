package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Apartment38/torii-gateway/internal/intelligence"
)

func (h *OpenAIHandler) customBenchmarkMutation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	var err error
	if r.URL.Path == "/gateway/v1/intelligence/import" && r.Method == http.MethodPost {
		var observations []intelligence.Observation
		observations, err = intelligence.ParseCSV(r.Body, time.Now())
		if err == nil {
			err = h.intelligence.Import(r.Context(), observations)
		}
	} else if r.URL.Path == "/gateway/v1/intelligence/observations/edit" && r.Method == http.MethodPut {
		var request struct {
			Source      string                   `json:"source"`
			Index       int                      `json:"index"`
			Revision    string                   `json:"revision"`
			Observation intelligence.Observation `json:"observation"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&request)
		if err == nil {
			err = h.intelligence.Edit(r.Context(), request.Source, request.Index, request.Revision, request.Observation)
		}
	} else {
		writeAPIError(w, 405, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, intelligence.ErrEditConflict) {
			status = http.StatusConflict
		}
		writeAPIError(w, status, err.Error(), "invalid_request_error", "benchmark_update_failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"updated": true})
}
