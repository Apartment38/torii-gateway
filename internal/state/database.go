package state

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const Owner = "default"

// Database stores only encrypted payloads; metadata is deliberately non-secret.
type Database struct {
	db   *sql.DB
	aead cipher.AEAD
}
type SecretMetadata struct {
	ID         string `json:"id"`
	Owner      string `json:"owner"`
	Service    string `json:"service"`
	Purpose    string `json:"purpose"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	KeyVersion int    `json:"key_version"`
}
type Import struct {
	ID, Service, Purpose string
	Value                []byte
}

func MasterKey() ([]byte, error) {
	path := os.Getenv("TORII_DB_KEY_FILE")
	explicit := path != ""
	if !explicit {
		path = "/run/secrets/torii_db_key"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if explicit || !errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("cannot read TORII_DB_KEY_FILE; mount a readable master-key secret")
		}
		b = []byte(os.Getenv("TORII_DB_KEY"))
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil, errors.New("master encryption key required: mount /run/secrets/torii_db_key or set TORII_DB_KEY to 64 hexadecimal characters")
	}
	k, err := hex.DecodeString(s)
	if err != nil || len(s) != 64 || len(k) != 32 {
		return nil, errors.New("master encryption key must be exactly 64 hexadecimal characters (32 random bytes)")
	}
	return k, nil
}

func OpenDatabase(path string, key []byte) (*Database, error) {
	if path == "" || strings.ContainsAny(path, "?#\x00") || strings.HasPrefix(path, "file:") || path == ":memory:" {
		return nil, errors.New("database path must be a filesystem path, not a SQLite URI")
	}
	if len(key) != 32 {
		return nil, errors.New("AES-256 requires a 32-byte master key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, errors.New("cannot create database directory; mount /data writable by the container user or set TORII_DB_PATH")
	}
	for _, target := range []string{filepath.Dir(path), path, path + "-wal", path + "-shm"} {
		if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("database paths must not be symbolic links")
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("database is not writable; check the persistent volume owner")
	}
	f.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("invalid database path")
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute), RawQuery: "_pragma=busy_timeout%285000%29&_pragma=foreign_keys%281%29"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, errors.New("cannot open credential database")
	}
	db.SetMaxOpenConns(1)
	d := &Database{db: db, aead: aead}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	for _, pragma := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL"} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, errors.New("cannot configure credential database")
		}
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, err
	}
	if version > 1 {
		return nil, errors.New("credential database schema is newer than this gateway")
	}
	if version == 0 {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		_, err = tx.Exec(`CREATE TABLE credentials (id TEXT PRIMARY KEY, owner TEXT NOT NULL, service TEXT NOT NULL, purpose TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, key_version INTEGER NOT NULL, nonce BLOB NOT NULL, ciphertext BLOB NOT NULL, tag BLOB NOT NULL, deleted INTEGER NOT NULL DEFAULT 0, UNIQUE(key_version,nonce)); CREATE TABLE imports (id TEXT PRIMARY KEY REFERENCES credentials(id)); PRAGMA user_version=1;`)
		if err != nil {
			return nil, errors.New("cannot migrate credential database schema")
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM credentials").Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		if err := d.Put("__key_check", Owner, "gateway", "key_check", []byte("torii-key-check-v1")); err != nil {
			return nil, err
		}
	}
	rows, err := db.Query("SELECT id,owner,service,purpose,key_version,nonce,ciphertext,tag FROM credentials WHERE deleted=0")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, owner, service, purpose string
		var v int
		var n, c, t []byte
		if err := rows.Scan(&id, &owner, &service, &purpose, &v, &n, &c, &t); err != nil {
			rows.Close()
			return nil, err
		}
		if _, err := d.decrypt(id, owner, service, purpose, v, n, c, t); err != nil {
			rows.Close()
			return nil, errors.New("cannot decrypt credential database: restore the original master key and check database integrity; do not generate a replacement key")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, p := range []string{path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	ok = true
	return d, nil
}
func (d *Database) Close() error { return d.db.Close() }
func aad(id, owner, service, purpose string, version int) []byte {
	b, _ := json.Marshal([]any{id, owner, service, purpose, version})
	return b
}
func (d *Database) decrypt(id, owner, service, purpose string, v int, n, c, t []byte) ([]byte, error) {
	if v != 1 || len(n) != d.aead.NonceSize() || len(t) != d.aead.Overhead() {
		return nil, errors.New("credential encryption metadata is invalid or unsupported")
	}
	b, err := d.aead.Open(nil, n, append(append([]byte{}, c...), t...), aad(id, owner, service, purpose, v))
	if err != nil {
		return nil, errors.New("credential authentication failed")
	}
	return b, nil
}
func (d *Database) write(tx *sql.Tx, id, owner, service, purpose string, value []byte) error {
	n := make([]byte, d.aead.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return err
	}
	sealed := d.aead.Seal(nil, n, value, aad(id, owner, service, purpose, 1))
	split := len(sealed) - d.aead.Overhead()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := tx.Exec(`INSERT INTO credentials(id,owner,service,purpose,created_at,updated_at,key_version,nonce,ciphertext,tag) VALUES(?,?,?,?,?,?,1,?,?,?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at,key_version=excluded.key_version,nonce=excluded.nonce,ciphertext=excluded.ciphertext,tag=excluded.tag,deleted=0 WHERE credentials.owner=excluded.owner AND credentials.service=excluded.service AND credentials.purpose=excluded.purpose`, id, owner, service, purpose, now, now, n, sealed[:split], sealed[split:])
	return err
}
func (d *Database) Put(id, owner, service, purpose string, value []byte) error {
	if id == "" || owner == "" || service == "" || purpose == "" || len(value) == 0 || len(value) > 1<<20 {
		return errors.New("invalid credential input")
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing string
	var oldService, oldPurpose string
	err = tx.QueryRow("SELECT owner,service,purpose FROM credentials WHERE id=?", id).Scan(&existing, &oldService, &oldPurpose)
	if err == nil && (existing != owner || service != oldService || purpose != oldPurpose) {
		return ErrCredentialNotFound
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err = d.write(tx, id, owner, service, purpose, value); err != nil {
		return errors.New("cannot store encrypted credential")
	}
	return tx.Commit()
}
func (d *Database) Get(id, owner string) ([]byte, error) {
	var service, purpose string
	var v int
	var n, c, t []byte
	err := d.db.QueryRow("SELECT service,purpose,key_version,nonce,ciphertext,tag FROM credentials WHERE id=? AND owner=? AND deleted=0", id, owner).Scan(&service, &purpose, &v, &n, &c, &t)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		return nil, errors.New("cannot read credential")
	}
	return d.decrypt(id, owner, service, purpose, v, n, c, t)
}
func (d *Database) DeleteSecret(id, owner string) error {
	if strings.HasPrefix(id, "__") {
		return errors.New("system credential cannot be deleted")
	}
	r, err := d.db.Exec("UPDATE credentials SET deleted=1 WHERE id=? AND owner=? AND deleted=0", id, owner)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrCredentialNotFound
	}
	return nil
}
func (d *Database) List(owner string) ([]SecretMetadata, error) {
	r, err := d.db.Query("SELECT id,owner,service,purpose,created_at,updated_at,key_version FROM credentials WHERE owner=? AND deleted=0 ORDER BY id", owner)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var list = []SecretMetadata{}
	for r.Next() {
		var m SecretMetadata
		if err := r.Scan(&m.ID, &m.Owner, &m.Service, &m.Purpose, &m.CreatedAt, &m.UpdatedAt, &m.KeyVersion); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(m.ID, "__") {
			list = append(list, m)
		}
	}
	return list, r.Err()
}
func (d *Database) Metadata(id, owner string) (SecretMetadata, error) {
	var m SecretMetadata
	err := d.db.QueryRow("SELECT id,owner,service,purpose,created_at,updated_at,key_version FROM credentials WHERE id=? AND owner=?", id, owner).Scan(&m.ID, &m.Owner, &m.Service, &m.Purpose, &m.CreatedAt, &m.UpdatedAt, &m.KeyVersion)
	if errors.Is(err, sql.ErrNoRows) || strings.HasPrefix(id, "__") {
		return m, ErrCredentialNotFound
	}
	return m, err
}

// ImportOnce never overwrites records, including tombstones. All values are
// encrypted and decrypted for verification before a single transaction commits.
func (d *Database) ImportOnce(values []Import) error {
	tx, err := d.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, v := range values {
		inserted := false
		var found int
		err := tx.QueryRow("SELECT 1 FROM imports WHERE id=?", v.ID).Scan(&found)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = tx.QueryRow("SELECT 1 FROM credentials WHERE id=?", v.ID).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			if err = d.write(tx, v.ID, Owner, v.Service, v.Purpose, v.Value); err != nil {
				return errors.New("credential import failed")
			}
			inserted = true
		} else if err != nil {
			return err
		}
		var owner, service, purpose string
		var version int
		var n, c, t []byte
		if err = tx.QueryRow("SELECT owner,service,purpose,key_version,nonce,ciphertext,tag FROM credentials WHERE id=?", v.ID).Scan(&owner, &service, &purpose, &version, &n, &c, &t); err != nil {
			return err
		}
		if owner != Owner || service != v.Service || purpose != v.Purpose {
			return errors.New("credential import ownership or purpose mismatch")
		}
		plaintext, decryptErr := d.decrypt(v.ID, owner, service, purpose, version, n, c, t)
		if decryptErr != nil {
			return decryptErr
		}
		if inserted && !bytes.Equal(plaintext, v.Value) {
			return errors.New("credential import verification failed")
		}
		if _, err = tx.Exec("INSERT INTO imports(id) VALUES(?)", v.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *Database) Imported(id string) (bool, error) {
	var n int
	err := d.db.QueryRow("SELECT 1 FROM imports WHERE id=?", id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
func (d *Database) Load(providerID string) (*Credential, error) {
	b, err := d.Get("oauth/"+providerID, Owner)
	if err != nil {
		return nil, err
	}
	var c Credential
	if json.Unmarshal(b, &c) != nil {
		return nil, errors.New("invalid encrypted OAuth record")
	}
	return &c, nil
}
func (d *Database) Save(providerID string, c *Credential) error {
	if c == nil {
		return errors.New("credential required")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return d.Put("oauth/"+providerID, Owner, providerID, "oauth", b)
}
func (d *Database) Delete(providerID string) error {
	err := d.DeleteSecret("oauth/"+providerID, Owner)
	if errors.Is(err, ErrCredentialNotFound) {
		return nil
	}
	return err
}
func NewSecretID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("secure randomness unavailable: %w", err))
	}
	return hex.EncodeToString(b)
}
