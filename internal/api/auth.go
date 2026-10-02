package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/fault"
)

// ErrUnauthorized indicates that a request did not contain valid client
// credentials. It intentionally does not distinguish missing and invalid keys.
var ErrUnauthorized = errors.New("unauthorized")

// ClientIdentity identifies an authenticated gateway client.
type ClientIdentity struct {
	ID string
}

// ClientAuthenticator authenticates a request and returns its client identity.
type ClientAuthenticator interface {
	Authenticate(*http.Request) (*ClientIdentity, error)
}

// NoAuthAuthenticator is the intentionally opt-in development authenticator.
// Keep the request boundary behind ClientAuthenticator so Basic Auth and
// Authentik/OIDC adapters can replace it without changing API handlers.
type NoAuthAuthenticator struct{}

func (NoAuthAuthenticator) Authenticate(*http.Request) (*ClientIdentity, error) {
	return &ClientIdentity{ID: "anonymous"}, nil
}

// StaticBearerAuthenticator authenticates the single gateway API key used in
// v1. The stored digest also ensures comparisons have a fixed length.
type StaticBearerAuthenticator struct {
	keyDigest [sha256.Size]byte
}

// NewStaticBearerAuthenticator creates an authenticator for apiKey.
func NewStaticBearerAuthenticator(apiKey string) (*StaticBearerAuthenticator, error) {
	if apiKey == "" || strings.ContainsAny(apiKey, " \t\r\n") {
		return nil, errors.New("gateway API key must be non-empty and contain no whitespace")
	}
	return &StaticBearerAuthenticator{keyDigest: sha256.Sum256([]byte(apiKey))}, nil
}

// Authenticate validates an Authorization: Bearer header. Authentication
// failures share one error so callers cannot use the response as a key oracle.
func (a *StaticBearerAuthenticator) Authenticate(r *http.Request) (*ClientIdentity, error) {
	if a == nil || r == nil {
		return nil, ErrUnauthorized
	}

	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return nil, ErrUnauthorized
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return nil, ErrUnauthorized
	}

	presentedDigest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(presentedDigest[:], a.keyDigest[:]) != 1 {
		return nil, ErrUnauthorized
	}
	return &ClientIdentity{ID: "default"}, nil
}

type clientIdentityContextKey struct{}

// ClientIdentityFromContext returns the identity set by AuthenticateMiddleware.
func ClientIdentityFromContext(ctx context.Context) (*ClientIdentity, bool) {
	identity, ok := ctx.Value(clientIdentityContextKey{}).(*ClientIdentity)
	return identity, ok
}

// AuthenticateMiddleware requires valid client credentials before invoking
// next. Unauthorized responses use the error envelope expected by OpenAI
// clients.
func AuthenticateMiddleware(authenticator ClientAuthenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authenticator == nil {
			writeUnauthorized(w)
			return
		}
		identity, err := authenticator.Authenticate(r)
		if err != nil || identity == nil {
			if challenge, ok := authenticator.(interface{ Challenge() string }); ok {
				writeUnauthorized(w, challenge.Challenge())
			} else {
				writeUnauthorized(w)
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIdentityContextKey{}, identity)))
	})
}

func writeUnauthorized(w http.ResponseWriter, challenges ...string) {
	challenge := "Bearer"
	if len(challenges) > 0 {
		challenge = challenges[0]
	}
	w.Header().Set("WWW-Authenticate", challenge)
	writeError(w, fault.New(fault.AuthenticationFailure, ""))
}
