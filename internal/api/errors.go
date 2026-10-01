package api

import (
	"errors"
	"net/http"

	"github.com/Apartment38/torii-gateway/internal/fault"
)

type errorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	var typed *fault.Error
	if !errors.As(err, &typed) {
		typed = fault.New(fault.ProviderUnavailable, "")
	}
	status, body := publicError(typed)
	writeJSON(w, status, errorEnvelope{Error: body})
}

func publicError(err *fault.Error) (int, errorBody) {
	var param *string
	if err.Param != "" {
		value := err.Param
		param = &value
	}
	result := errorBody{Type: "api_error", Param: param, Code: string(err.Kind)}
	switch err.Kind {
	case fault.AuthenticationFailure:
		result.Message, result.Type, result.Code = "Invalid authentication credentials", "invalid_request_error", "invalid_api_key"
		return http.StatusUnauthorized, result
	case fault.InvalidRequest:
		result.Message, result.Type = "Invalid request", "invalid_request_error"
		return http.StatusBadRequest, result
	case fault.ModelNotFound:
		result.Message, result.Type = "The model does not exist", "invalid_request_error"
		return http.StatusNotFound, result
	case fault.UnsupportedEndpoint, fault.UnsupportedCapability:
		result.Message, result.Type = "The requested feature is not supported", "invalid_request_error"
		return http.StatusBadRequest, result
	case fault.ContextTooLarge:
		result.Message, result.Type, result.Code = "Request body exceeds the maximum allowed size", "invalid_request_error", "request_too_large"
		return http.StatusRequestEntityTooLarge, result
	case fault.ProviderAuthenticationFailure:
		result.Message = "Upstream authentication failed"
		return http.StatusBadGateway, result
	case fault.RateLimited:
		result.Message = "Upstream rate limit exceeded"
		return http.StatusTooManyRequests, result
	case fault.QuotaExhausted:
		result.Message = "Upstream quota exhausted"
		return http.StatusTooManyRequests, result
	case fault.ModelUnavailable:
		result.Message = "The model is unavailable"
		return http.StatusServiceUnavailable, result
	case fault.ClassifierFailure:
		result.Message = "Request classification failed"
		return http.StatusBadGateway, result
	case fault.StreamInterrupted:
		result.Message = "Upstream stream was interrupted"
		return http.StatusBadGateway, result
	default:
		result.Message, result.Code = "Upstream request failed", "upstream_error"
		return http.StatusBadGateway, result
	}
}
