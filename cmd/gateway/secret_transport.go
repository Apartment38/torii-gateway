package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/config"
)

// secretTransport materializes credentials only on the outbound request. The
// provider and connection pool retain references, never decrypted header values.
type secretTransport struct {
	base       http.RoundTripper
	headers    map[string]config.SecretRef
	bearer     bool
	management *config.SecretRef
}

func (t *secretTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header = req.Header.Clone()
	for name, ref := range t.headers {
		if name == "Authorization" && t.management != nil && strings.HasSuffix(r.URL.Path, "/credits") {
			ref = *t.management
		}
		v, err := ref.Resolve()
		if err != nil || v == "" || strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("upstream credential is unavailable or invalid")
		}
		if strings.EqualFold(name, "Authorization") && (t.bearer || (t.management != nil && strings.HasSuffix(r.URL.Path, "/credits"))) {
			v = "Bearer " + v
		}
		r.Header.Set(name, v)
	}
	response, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, errors.New("upstream HTTP request failed")
	}
	// Upstream errors can echo request headers. Do not forward their bodies or
	// credential-bearing response headers to clients or tracing/error handlers.
	if response.StatusCode >= 400 {
		response.Body.Close()
		response.Body = http.NoBody
		response.ContentLength = 0
	}
	response.Header.Del("Authorization")
	response.Header.Del("X-Api-Key")
	response.Header.Del("Set-Cookie")
	if response.Request != nil {
		response.Request.Header = make(http.Header)
	}
	return response, nil
}
