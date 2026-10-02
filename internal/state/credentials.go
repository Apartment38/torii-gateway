// Package state persists the minimal durable gateway state.
package state

import (
	"errors"
	"time"
)

var ErrCredentialNotFound = errors.New("credential not found")

type Credential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	PrincipalID  string    `json:"principal_id,omitempty"`
	Endpoint     string    `json:"endpoint"`
	ClientID     string    `json:"client_id,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	Issuer       string    `json:"issuer,omitempty"`
	Subject      string    `json:"subject,omitempty"`
	Email        string    `json:"email,omitempty"`
	HostID       string    `json:"host_id,omitempty"`
}

type CredentialStore interface {
	Load(providerID string) (*Credential, error)
	Save(providerID string, credential *Credential) error
	Delete(providerID string) error
}
