package api

import "net/http"

// CredentialBearerAuthenticator keeps no plaintext credential between requests.
type CredentialBearerAuthenticator struct{ Resolve func() (string, error) }

func (a CredentialBearerAuthenticator) Authenticate(r *http.Request) (*ClientIdentity, error) {
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
