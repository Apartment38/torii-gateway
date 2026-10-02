package state

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
)

const GatewayAPIKeyID = "gateway/api-key"
const adminRecordID = "__admin"
const passwordIterations = 600000

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

type Admin struct {
	Username   string `json:"username"`
	Salt       []byte `json:"salt"`
	Hash       []byte `json:"hash"`
	Iterations int    `json:"iterations"`
	Algorithm  string `json:"algorithm"`
}

func (d *Database) Admin() (*Admin, error) {
	b, err := d.Get(adminRecordID, Owner)
	if err != nil {
		return nil, err
	}
	var a Admin
	if json.Unmarshal(b, &a) != nil || !usernamePattern.MatchString(a.Username) || len(a.Salt) != 32 || len(a.Hash) != 32 || a.Iterations != passwordIterations || a.Algorithm != "pbkdf2-hmac-sha256" {
		return nil, errors.New("admin password record is invalid or unsupported")
	}
	return &a, nil
}
func (d *Database) VerifyAdmin(username, password string) bool {
	a, err := d.Admin()
	if err != nil || len(password) > 1024 {
		return false
	}
	hash, err := pbkdf2.Key(sha256.New, password, a.Salt, a.Iterations, 32)
	if err != nil {
		return false
	}
	actual, want := sha256.Sum256([]byte(username)), sha256.Sum256([]byte(a.Username))
	return subtle.ConstantTimeCompare(actual[:], want[:])&subtle.ConstantTimeCompare(hash, a.Hash) == 1
}
func generatedAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("secure randomness unavailable")
	}
	return "torii_" + hex.EncodeToString(b), nil
}

// CreateAdmin atomically claims the singleton admin account. Existing imported
// API keys are retained and never revealed. Only newly generated keys return.
func (d *Database) CreateAdmin(username, password string) (string, error) {
	if !usernamePattern.MatchString(username) {
		return "", errors.New("username must contain 1-64 letters, digits, dots, underscores or hyphens")
	}
	if len(password) < 12 || len(password) > 1024 {
		return "", errors.New("password must contain 12-1024 bytes")
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(Admin{Username: username, Salt: salt, Hash: hash, Iterations: passwordIterations, Algorithm: "pbkdf2-hmac-sha256"})
	if err != nil {
		return "", err
	}
	tx, err := d.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM credentials WHERE id=? AND deleted=0", adminRecordID).Scan(&count); err != nil {
		return "", err
	}
	if count != 0 {
		return "", errors.New("admin account already exists; sign in instead")
	}
	if err = d.write(tx, adminRecordID, Owner, "gateway", "administrator", b); err != nil {
		return "", err
	}
	if err = tx.QueryRow("SELECT count(*) FROM credentials WHERE id=? AND deleted=0", GatewayAPIKeyID).Scan(&count); err != nil {
		return "", err
	}
	key := ""
	if count == 0 {
		key, err = generatedAPIKey()
		if err != nil {
			return "", err
		}
		if err = d.write(tx, GatewayAPIKeyID, Owner, "gateway", "authentication", []byte(key)); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return key, nil
}
func (d *Database) GatewayAPIKey() (string, error) {
	b, err := d.Get(GatewayAPIKeyID, Owner)
	return string(b), err
}
func (d *Database) RotateAPIKey() (string, error) {
	if _, err := d.Admin(); err != nil {
		return "", errors.New("create an administrator before rotating the API key")
	}
	key, err := generatedAPIKey()
	if err != nil {
		return "", err
	}
	if err = d.Put(GatewayAPIKeyID, Owner, "gateway", "authentication", []byte(key)); err != nil {
		return "", err
	}
	return key, nil
}
func (d *Database) AdminRequired() (bool, error) {
	_, err := d.Admin()
	if errors.Is(err, ErrCredentialNotFound) || errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return false, err
}
