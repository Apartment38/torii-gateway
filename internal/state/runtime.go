package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const runtimeSchema = `
CREATE TABLE usage_aggregates (
 model_id TEXT PRIMARY KEY, requests TEXT NOT NULL, input_tokens TEXT NOT NULL,
 output_tokens TEXT NOT NULL, estimated_cost REAL NOT NULL CHECK(estimated_cost>=0),
 last_request_at TEXT NOT NULL);
CREATE TABLE application_settings (
 key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);
`

// InitializeCodexHostID creates a database-owned public identity once.
// An established identity survives restarts and provider logout.
func (d *Database) InitializeCodexHostID(ctx context.Context) error {
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
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return errors.New("cannot generate Codex host identity")
	}
	// RFC 4122 version 4 UUID; public identity, not an authentication secret.
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	value = fmt.Sprintf("urn:uuid:%08x-%04x-%04x-%04x-%012x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cannot initialize Codex host identity")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT INTO application_settings(key,value,updated_at) VALUES('codex_host_id',?,?) ON CONFLICT(key) DO NOTHING", value, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errors.New("cannot persist Codex host identity")
	}
	written, err := result.RowsAffected()
	if err != nil {
		return errors.New("cannot verify Codex host identity initialization")
	}
	var actual string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM application_settings WHERE key='codex_host_id'").Scan(&actual); err != nil || strings.TrimSpace(actual) == "" || (written == 1 && actual != value) {
		return errors.New("Codex host identity initialization verification failed")
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
