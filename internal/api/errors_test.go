package api

import (
	"net/http"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/fault"
)

func TestPublicErrorMappings(t *testing.T) {
	tests := []struct {
		kind   fault.Kind
		status int
		typeID string
		code   string
	}{
		{fault.InvalidRequest, http.StatusBadRequest, "invalid_request_error", "invalid_request"},
		{fault.AuthenticationFailure, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key"},
		{fault.ModelNotFound, http.StatusNotFound, "invalid_request_error", "model_not_found"},
		{fault.UnsupportedCapability, http.StatusBadRequest, "invalid_request_error", "unsupported_capability"},
		{fault.ContextTooLarge, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large"},
		{fault.RateLimited, http.StatusTooManyRequests, "api_error", "rate_limited"},
		{fault.QuotaExhausted, http.StatusTooManyRequests, "api_error", "quota_exhausted"},
		{fault.ProviderUnavailable, http.StatusBadGateway, "api_error", "upstream_error"},
		{fault.StreamInterrupted, http.StatusBadGateway, "api_error", "stream_interrupted"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			status, body := publicError(fault.New(tt.kind, "model"))
			if status != tt.status || body.Type != tt.typeID || body.Code != tt.code || body.Param == nil || *body.Param != "model" {
				t.Fatalf("mapping = status %d, body %#v", status, body)
			}
		})
	}
}
