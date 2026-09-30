// Package fault defines errors that can cross gateway component boundaries.
package fault

import (
	"errors"
	"fmt"
)

// Kind identifies a failure without requiring callers to inspect error text.
type Kind string

const (
	InvalidRequest                Kind = "invalid_request"
	AuthenticationFailure         Kind = "authentication_failure"
	ProviderAuthenticationFailure Kind = "provider_authentication_failure"
	RateLimited                   Kind = "rate_limited"
	QuotaExhausted                Kind = "quota_exhausted"
	ProviderUnavailable           Kind = "provider_unavailable"
	ModelUnavailable              Kind = "model_unavailable"
	ModelNotFound                 Kind = "model_not_found"
	UnsupportedEndpoint           Kind = "unsupported_endpoint"
	UnsupportedCapability         Kind = "unsupported_capability"
	ContextTooLarge               Kind = "context_too_large"
	ClassifierFailure             Kind = "classifier_failure"
	StreamInterrupted             Kind = "stream_interrupted"
)

// Error is a typed internal failure. Detail and Cause are for internal use;
// public handlers map Kind to a fixed, non-sensitive response.
type Error struct {
	Kind   Kind
	Param  string
	Detail string
	Cause  error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Kind, e.Detail)
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error { return e.Cause }

// New constructs a typed failure with an optional parameter name.
func New(kind Kind, param string) *Error { return &Error{Kind: kind, Param: param} }

// Wrap adds a typed boundary while retaining the cause for errors.Is/As.
func Wrap(kind Kind, detail string, cause error) *Error {
	return &Error{Kind: kind, Detail: detail, Cause: cause}
}

// KindOf returns the first typed kind in err's chain.
func KindOf(err error) (Kind, bool) {
	var typed *Error
	if !errors.As(err, &typed) {
		return "", false
	}
	return typed.Kind, true
}
