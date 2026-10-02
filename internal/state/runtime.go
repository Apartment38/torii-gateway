package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func migrateRuntimeState(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
CREATE TABLE usage_aggregates (
 model_id TEXT PRIMARY KEY, requests TEXT NOT NULL, input_tokens TEXT NOT NULL,
 output_tokens TEXT NOT NULL, estimated_cost REAL NOT NULL CHECK(estimated_cost>=0),
 last_request_at TEXT NOT NULL);
CREATE TABLE application_settings (
 key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);
PRAGMA user_version=3;`)
	if err != nil {
		return errors.New("cannot migrate runtime state schema; preserve the database and restore a pre-upgrade backup if needed")
	}
	return tx.Commit()
}

// LegacyFile reads a bounded migration input without creating directories or
// following a symlink. An omitted directory never searches the working directory.
func LegacyFile(directory, relative string, limit int64) ([]byte, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(directory, relative)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("legacy state input is not a regular file or exceeds its size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("cannot read bounded legacy state input")
	}
	return b, nil
}

// InitializeCodexHostID imports the old public identity once. Files are retained
// unchanged; an established database identity always wins, including after logout.
func (d *Database) InitializeCodexHostID(ctx context.Context, directory string) error {
	var value string
	err := d.db.QueryRowContext(ctx, "SELECT value FROM application_settings WHERE key='codex_host_id'").Scan(&value)
	if err == nil {
		if strings.TrimSpace(value) == "" {
			return errors.New("stored Codex host identity is invalid")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return errors.New("cannot inspect Codex host identity")
	}
	b, err := LegacyFile(directory, filepath.Join("auth", "codex-host-id"), 1024)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot import Codex host identity; check the legacy auth/codex-host-id file and its permissions")
	}
	if err == nil {
		value = strings.TrimSpace(string(b))
		if value == "" {
			return errors.New("legacy Codex host identity is empty; restore the original auth/codex-host-id file")
		}
	} else {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return errors.New("cannot generate Codex host identity")
		}
		// RFC 4122 version 4 UUID; public identity, not an authentication secret.
		raw[6] = (raw[6] & 0x0f) | 0x40
		raw[8] = (raw[8] & 0x3f) | 0x80
		value = fmt.Sprintf("urn:uuid:%08x-%04x-%04x-%04x-%012x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cannot import Codex host identity")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT INTO application_settings(key,value,updated_at) VALUES('codex_host_id',?,?) ON CONFLICT(key) DO NOTHING", value, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errors.New("cannot persist Codex host identity")
	}
	written, err := result.RowsAffected()
	if err != nil {
		return errors.New("cannot verify Codex host identity import")
	}
	var actual string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM application_settings WHERE key='codex_host_id'").Scan(&actual); err != nil || strings.TrimSpace(actual) == "" || (written == 1 && actual != value) {
		return errors.New("Codex host identity import verification failed")
	}
	return tx.Commit()
}

func (d *Database) CodexHostID(ctx context.Context) (string, error) {
	var value string
	if err := d.db.QueryRowContext(ctx, "SELECT value FROM application_settings WHERE key='codex_host_id'").Scan(&value); err != nil || strings.TrimSpace(value) == "" {
		return "", errors.New("Codex host identity unavailable; restart the gateway to initialize database state")
	}
	return value, nil
}
