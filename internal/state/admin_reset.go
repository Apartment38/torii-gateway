package state

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"
)

var ErrInvalidReset = errors.New("invalid or expired reset link")

// Empty email preserves existing installations; those accounts cannot request resets.
func NormalizeAdminEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", nil
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return "", errors.New("enter a valid email address")
	}
	return strings.ToLower(email), nil
}

func (d *Database) adminInTransaction(tx *sql.Tx) (*Admin, error) {
	var service, purpose string
	var version int
	var nonce, ciphertext, tag []byte
	if err := tx.QueryRow("SELECT service,purpose,key_version,nonce,ciphertext,tag FROM credentials WHERE id=? AND owner=? AND deleted=0", adminRecordID, Owner).Scan(&service, &purpose, &version, &nonce, &ciphertext, &tag); err != nil {
		return nil, err
	}
	b, err := d.decrypt(adminRecordID, Owner, service, purpose, version, nonce, ciphertext, tag)
	if err != nil {
		return nil, err
	}
	var a Admin
	if err = json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	return &a, nil
}
func adminFingerprint(a *Admin) [32]byte { b, _ := json.Marshal(a); return sha256.Sum256(b) }
func (d *Database) saveAdminChange(tx *sql.Tx, a *Admin) error {
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err = d.write(tx, adminRecordID, Owner, "gateway", "administrator", b); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM admin_sessions"); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM admin_password_reset"); err != nil {
		return err
	}
	return tx.Commit()
}

// Reauthentication and mutation share the same write transaction to prevent stale updates.
func (d *Database) UpdateAdmin(username, email, password, currentPassword string) error {
	if len(currentPassword) > 1024 {
		return errors.New("administrator authentication failed")
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a, err := d.adminInTransaction(tx)
	if err != nil {
		return err
	}
	hash, err := pbkdf2.Key(sha256.New, currentPassword, a.Salt, a.Iterations, 32)
	if err != nil || subtle.ConstantTimeCompare(hash, a.Hash) != 1 {
		return errors.New("administrator authentication failed")
	}
	if password == "" {
		password = currentPassword
	}
	a, err = newAdmin(username, email, password)
	if err != nil {
		return err
	}
	return d.saveAdminChange(tx, a)
}

// Only a hash is stored. A new link replaces the old one, at most once per minute.
func (d *Database) IssueAdminPasswordReset(email string) (string, string, error) {
	email, err := NormalizeAdminEmail(email)
	if err != nil || email == "" {
		return "", "", ErrInvalidReset
	}
	tx, err := d.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	a, err := d.adminInTransaction(tx)
	if err != nil {
		return "", "", err
	}
	if a.Email == "" || !strings.EqualFold(a.Email, email) {
		return "", "", ErrInvalidReset
	}
	var requested int64
	err = tx.QueryRow("SELECT requested_at FROM admin_password_reset WHERE id=1").Scan(&requested)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	now := time.Now().Unix()
	if requested > now-60 {
		return "", "", ErrInvalidReset
	}
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(token))
	fingerprint := adminFingerprint(a)
	_, err = tx.Exec("INSERT INTO admin_password_reset(id,token_hash,admin_fingerprint,expires_at,requested_at) VALUES(1,?,?,?,?) ON CONFLICT(id) DO UPDATE SET token_hash=excluded.token_hash,admin_fingerprint=excluded.admin_fingerprint,expires_at=excluded.expires_at,requested_at=excluded.requested_at", digest[:], fingerprint[:], now+1800, now)
	if err != nil {
		return "", "", err
	}
	return token, a.Email, tx.Commit()
}
func (d *Database) ResetAdminPassword(token, password string) error {
	if len(token) != 64 {
		return ErrInvalidReset
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var saved, fingerprint []byte
	var expires int64
	if err = tx.QueryRow("SELECT token_hash,admin_fingerprint,expires_at FROM admin_password_reset WHERE id=1").Scan(&saved, &fingerprint, &expires); err != nil {
		return ErrInvalidReset
	}
	digest := sha256.Sum256([]byte(token))
	a, err := d.adminInTransaction(tx)
	if err != nil {
		return err
	}
	current := adminFingerprint(a)
	if expires <= time.Now().Unix() || subtle.ConstantTimeCompare(saved, digest[:]) != 1 || subtle.ConstantTimeCompare(fingerprint, current[:]) != 1 {
		return ErrInvalidReset
	}
	a, err = newAdmin(a.Username, a.Email, password)
	if err != nil {
		return err
	}
	return d.saveAdminChange(tx, a)
}
