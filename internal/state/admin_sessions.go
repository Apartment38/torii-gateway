package state

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

type AdminSession struct {
	Username  string    `json:"username"`
	CSRF      string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (d *Database) NewAdminSession(ttl time.Duration) (string, AdminSession, error) {
	if ttl <= 0 || ttl > 30*24*time.Hour {
		return "", AdminSession{}, errors.New("invalid session lifetime")
	}
	a, err := d.Admin()
	if err != nil {
		return "", AdminSession{}, err
	}
	token, err := randomToken()
	if err != nil {
		return "", AdminSession{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", AdminSession{}, err
	}
	digest := sha256.Sum256([]byte(token))
	fingerprint := sha256.Sum256(a.Hash)
	session := AdminSession{Username: a.Username, CSRF: csrf, ExpiresAt: time.Now().UTC().Add(ttl)}
	tx, err := d.db.Begin()
	if err != nil {
		return "", AdminSession{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM admin_sessions WHERE expires_at<=?", time.Now().Unix()); err != nil {
		return "", AdminSession{}, err
	}
	// Bound the administrator's active browser sessions, evicting oldest first.
	if _, err = tx.Exec("DELETE FROM admin_sessions WHERE token_hash IN (SELECT token_hash FROM admin_sessions ORDER BY expires_at DESC LIMIT -1 OFFSET 127)"); err != nil {
		return "", AdminSession{}, err
	}
	_, err = tx.Exec("INSERT INTO admin_sessions(token_hash,csrf,username,expires_at,admin_fingerprint) VALUES(?,?,?,?,?)", digest[:], csrf, a.Username, session.ExpiresAt.Unix(), fingerprint[:])
	if err != nil {
		return "", AdminSession{}, err
	}
	return token, session, tx.Commit()
}
func (d *Database) AdminSession(token string) (AdminSession, error) {
	if len(token) != 64 {
		return AdminSession{}, ErrCredentialNotFound
	}
	digest := sha256.Sum256([]byte(token))
	var s AdminSession
	var expires int64
	var saved []byte
	err := d.db.QueryRow("SELECT csrf,username,expires_at,admin_fingerprint FROM admin_sessions WHERE token_hash=?", digest[:]).Scan(&s.CSRF, &s.Username, &expires, &saved)
	if err != nil {
		return s, ErrCredentialNotFound
	}
	a, err := d.Admin()
	if err != nil {
		return s, ErrCredentialNotFound
	}
	fingerprint := sha256.Sum256(a.Hash)
	if expires <= time.Now().Unix() || subtle.ConstantTimeCompare(saved, fingerprint[:]) != 1 || s.Username != a.Username {
		d.DeleteAdminSession(token)
		return AdminSession{}, ErrCredentialNotFound
	}
	s.ExpiresAt = time.Unix(expires, 0).UTC()
	return s, nil
}
func (d *Database) DeleteAdminSession(token string) error {
	digest := sha256.Sum256([]byte(token))
	_, err := d.db.Exec("DELETE FROM admin_sessions WHERE token_hash=?", digest[:])
	return err
}
