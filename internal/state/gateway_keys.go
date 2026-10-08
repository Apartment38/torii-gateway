package state

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"strings"
	"unicode"
)

const gatewayClientKeyPrefix = "gateway/client-key/"

type GatewayKeyMetadata struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func IsGatewayAPIKeyID(id string) bool {
	return id == GatewayAPIKeyID || strings.HasPrefix(id, gatewayClientKeyPrefix)
}

// CreateGatewayAPIKey stores the secret through the existing encryption layer
// and an indexed digest for lookup. Only creation returns the new key.
func (d *Database) CreateGatewayAPIKey(name string) (GatewayKeyMetadata, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || strings.ContainsFunc(name, unicode.IsControl) {
		return GatewayKeyMetadata{}, "", errors.New("API key name must contain 1-128 bytes without control characters")
	}
	if _, err := d.Admin(); err != nil {
		return GatewayKeyMetadata{}, "", errors.New("create an administrator before generating client API keys")
	}
	key, err := generatedAPIKey()
	if err != nil {
		return GatewayKeyMetadata{}, "", err
	}
	id := gatewayClientKeyPrefix + NewSecretID()
	digest := sha256.Sum256([]byte(key))
	tx, err := d.db.Begin()
	if err != nil {
		return GatewayKeyMetadata{}, "", errors.New("cannot create API key")
	}
	defer tx.Rollback()
	if err := d.write(tx, id, Owner, "gateway", "client_authentication", []byte(key)); err != nil {
		return GatewayKeyMetadata{}, "", errors.New("cannot encrypt API key")
	}
	if _, err := tx.Exec("INSERT INTO gateway_api_keys(credential_id,name,key_hash) VALUES(?,?,?)", id, name, digest[:]); err != nil {
		return GatewayKeyMetadata{}, "", errors.New("cannot store API key metadata")
	}
	m := GatewayKeyMetadata{ID: id, Name: name}
	if err := tx.QueryRow("SELECT created_at FROM credentials WHERE id=?", id).Scan(&m.CreatedAt); err != nil {
		return GatewayKeyMetadata{}, "", errors.New("cannot read API key metadata")
	}
	if err := tx.Commit(); err != nil {
		return GatewayKeyMetadata{}, "", errors.New("cannot commit API key")
	}
	return m, key, nil
}

func (d *Database) ListGatewayAPIKeys() ([]GatewayKeyMetadata, error) {
	rows, err := d.db.Query(`SELECT c.id,k.name,c.created_at FROM gateway_api_keys k
 JOIN credentials c ON c.id=k.credential_id WHERE c.owner=? AND c.deleted=0
 UNION ALL SELECT id,'Default gateway key',created_at FROM credentials WHERE id=? AND owner=? AND deleted=0
 ORDER BY created_at,id`, Owner, GatewayAPIKeyID, Owner)
	if err != nil {
		return nil, errors.New("cannot list API keys")
	}
	defer rows.Close()
	keys := []GatewayKeyMetadata{}
	for rows.Next() {
		var m GatewayKeyMetadata
		if err := rows.Scan(&m.ID, &m.Name, &m.CreatedAt); err != nil {
			return nil, errors.New("cannot read API key metadata")
		}
		keys = append(keys, m)
	}
	return keys, rows.Err()
}

// VerifyGatewayAPIKey keeps no plaintext cache and rechecks the encrypted
// credential after indexed lookup. Deletions take effect on the next request.
func (d *Database) VerifyGatewayAPIKey(key string) bool {
	_, err := d.ResolveGatewayAPIKey(key)
	return err == nil
}
func (d *Database) ResolveGatewayAPIKey(key string) (GatewayKeyMetadata, error) {
	if key == "" || len(key) > 1024 || strings.ContainsAny(key, " \t\r\n") {
		return GatewayKeyMetadata{}, ErrCredentialNotFound
	}
	digest := sha256.Sum256([]byte(key))
	var id string
	err := d.db.QueryRow(`SELECT k.credential_id FROM gateway_api_keys k JOIN credentials c ON c.id=k.credential_id
 WHERE k.key_hash=? AND c.owner=? AND c.service='gateway' AND c.purpose='client_authentication' AND c.deleted=0`, digest[:], Owner).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id = GatewayAPIKeyID
	} else if err != nil {
		return GatewayKeyMetadata{}, ErrCredentialNotFound
	}
	stored, err := d.Get(id, Owner)
	if err != nil {
		return GatewayKeyMetadata{}, ErrCredentialNotFound
	}
	want := sha256.Sum256(stored)
	if subtle.ConstantTimeCompare(digest[:], want[:]) != 1 {
		return GatewayKeyMetadata{}, ErrCredentialNotFound
	}
	name := "Default gateway key"
	if id != GatewayAPIKeyID {
		if err = d.db.QueryRow("SELECT name FROM gateway_api_keys WHERE credential_id=?", id).Scan(&name); err != nil {
			return GatewayKeyMetadata{}, ErrCredentialNotFound
		}
	}
	return GatewayKeyMetadata{ID: id, Name: name}, nil
}

// DeleteGatewayAPIKey physically removes both the encrypted record and its
// metadata (via foreign-key cascade), rather than retaining a soft-deleted key.
func (d *Database) DeleteGatewayAPIKey(id string) error {
	if !IsGatewayAPIKeyID(id) {
		return ErrCredentialNotFound
	}
	result, err := d.db.Exec(`DELETE FROM credentials WHERE id=? AND owner=? AND service='gateway'
 AND (id=? OR (purpose='client_authentication' AND EXISTS
 (SELECT 1 FROM gateway_api_keys WHERE credential_id=credentials.id)))`, id, Owner, GatewayAPIKeyID)
	if err != nil {
		return errors.New("cannot delete API key")
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return ErrCredentialNotFound
	}
	return nil
}
