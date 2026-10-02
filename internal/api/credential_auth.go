package api

import (
	"net/http"
	"strings"
)

// CredentialBearerAuthenticator keeps no plaintext credential between requests.
type CredentialBearerAuthenticator struct {
	Resolve func() (string, error)
	Verify  func(string) bool
}

func (a CredentialBearerAuthenticator) Authenticate(r *http.Request) (*ClientIdentity, error) {
	if r == nil {
		return nil, ErrUnauthorized
	}
	if a.Verify != nil {
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			return nil, ErrUnauthorized
		}
		scheme, token, ok := strings.Cut(values[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") || !a.Verify(token) {
			return nil, ErrUnauthorized
		}
		return &ClientIdentity{ID: "default"}, nil
	}
	if a.Resolve == nil {
		return nil, ErrUnauthorized
	}
	key, err := a.Resolve()
	if err != nil {
		return nil, ErrUnauthorized
	}
	auth, err := NewStaticBearerAuthenticator(key)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return auth.Authenticate(r)
}
