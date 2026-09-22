package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apartment38/torii-router/internal/api"
)

type nilIdentityAuthenticator struct{}

func (nilIdentityAuthenticator) Authenticate(*http.Request) (*api.ClientIdentity, error) {
	return nil, nil
}

func TestAuthenticateMiddlewareAcceptsBearerKey(t *testing.T) {
	authenticator, err := api.NewStaticBearerAuthenticator("gateway-secret")
	if err != nil {
		t.Fatal(err)
	}

	handler := api.AuthenticateMiddleware(authenticator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := api.ClientIdentityFromContext(r.Context())
		if !ok || identity.ID != "default" {
			t.Fatalf("identity = %#v, %v; want default identity", identity, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer gateway-secret")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestAuthenticateMiddlewareRejectsUnauthorizedRequests(t *testing.T) {
	authenticator, err := api.NewStaticBearerAuthenticator("gateway-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.AuthenticateMiddleware(authenticator, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	}))

	tests := []struct {
		name   string
		header []string
	}{
		{name: "missing"},
		{name: "wrong key same length", header: []string{"Bearer gateway-secrex"}},
		{name: "wrong key different length", header: []string{"Bearer short"}},
		{name: "wrong scheme", header: []string{"Basic gateway-secret"}},
		{name: "missing token", header: []string{"Bearer "}},
		{name: "extra whitespace", header: []string{"Bearer  gateway-secret"}},
		{name: "multiple headers", header: []string{"Bearer gateway-secret", "Bearer gateway-secret"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			for _, value := range tt.header {
				req.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, req)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if got := response.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			var envelope struct {
				Error struct {
					Message string  `json:"message"`
					Type    string  `json:"type"`
					Param   *string `json:"param"`
					Code    string  `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if envelope.Error.Message == "" || envelope.Error.Type != "invalid_request_error" || envelope.Error.Param != nil || envelope.Error.Code != "invalid_api_key" {
				t.Errorf("unexpected error envelope: %#v", envelope.Error)
			}
		})
	}
}

func TestAuthenticateMiddlewareRejectsNilAuthenticator(t *testing.T) {
	handler := api.AuthenticateMiddleware(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAuthenticateMiddlewareRejectsNilIdentity(t *testing.T) {
	handler := api.AuthenticateMiddleware(nilIdentityAuthenticator{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestStaticBearerAuthenticatorRejectsEmptyKey(t *testing.T) {
	if _, err := api.NewStaticBearerAuthenticator(""); err == nil {
		t.Fatal("NewStaticBearerAuthenticator() error = nil, want error")
	}
}
