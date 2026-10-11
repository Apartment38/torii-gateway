// Package state persists the minimal durable gateway state.
package state

import (
	"encoding/json"
	"errors"
	"sync"
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

// CredentialRefreshLock survives handler replacement and serializes refresh for
// one configured account. Login/logout writes are protected by SaveRotated's check.
func (d *Database) CredentialRefreshLock(id string) *sync.Mutex {
	lock, _ := d.credentialLocks.LoadOrStore(id, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// SaveRotated refuses to replace a login that changed during an OAuth exchange.
func (d *Database) SaveRotated(id string, previous, next *Credential) error {
	tx, err := d.db.Begin()
	if err != nil {
		return errors.New("cannot begin credential rotation")
	}
	defer tx.Rollback()
	var service, purpose string
	var version int
	var nonce, ciphertext, tag []byte
	key := "oauth/" + id
	err = tx.QueryRow("SELECT service,purpose,key_version,nonce,ciphertext,tag FROM credentials WHERE id=? AND owner=? AND deleted=0", key, Owner).Scan(&service, &purpose, &version, &nonce, &ciphertext, &tag)
	if err != nil {
		return errors.New("credential changed during refresh")
	}
	raw, err := d.decrypt(key, Owner, service, purpose, version, nonce, ciphertext, tag)
	if err != nil {
		return err
	}
	var current Credential
	if json.Unmarshal(raw, &current) != nil || current.AccessToken != previous.AccessToken || current.RefreshToken != previous.RefreshToken || current.AccountID != previous.AccountID || current.Endpoint != previous.Endpoint {
		return errors.New("credential changed during refresh")
	}
	raw, err = json.Marshal(next)
	if err != nil {
		return errors.New("invalid rotated credential")
	}
	if err = d.write(tx, key, Owner, service, purpose, raw); err != nil {
		return errors.New("cannot persist rotated credential")
	}
	return tx.Commit()
}
