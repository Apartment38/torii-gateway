package state

import (
	"database/sql"
	"errors"
	"fmt"
)

// BaselineSchemaVersion is the first database-only release. Older installations
// must be recreated once; all subsequent upgrades preserve their database.
const BaselineSchemaVersion = 4
const SchemaVersion = 9

type schemaMigration struct {
	Version int
	SQL     string
}

// Register one immutable step per future version, and increment SchemaVersion.
var schemaMigrations = []schemaMigration{{Version: 5, SQL: `
CREATE TABLE gateway_api_keys (
 credential_id TEXT PRIMARY KEY REFERENCES credentials(id) ON DELETE CASCADE,
 name TEXT NOT NULL, key_hash BLOB NOT NULL UNIQUE CHECK(length(key_hash)=32));
`}, {Version: 6, SQL: `
CREATE TABLE gateway_settings (id INTEGER PRIMARY KEY CHECK(id=1), document BLOB NOT NULL, revision INTEGER NOT NULL);
`}, {Version: 7, SQL: `
CREATE TABLE admin_sessions (token_hash BLOB PRIMARY KEY CHECK(length(token_hash)=32), csrf TEXT NOT NULL, username TEXT NOT NULL, expires_at INTEGER NOT NULL, admin_fingerprint BLOB NOT NULL);
`}, {Version: 8, SQL: `
CREATE TABLE accounting_lock(id INTEGER PRIMARY KEY,value INTEGER NOT NULL); INSERT INTO accounting_lock VALUES(1,0);
CREATE TABLE accounting_totals(key_id TEXT NOT NULL,key_name TEXT NOT NULL,period TEXT NOT NULL,provider TEXT NOT NULL,model TEXT NOT NULL,requests INTEGER NOT NULL,input_tokens INTEGER NOT NULL,output_tokens INTEGER NOT NULL,cost REAL NOT NULL,unknown_cost INTEGER NOT NULL,estimated_tokens INTEGER NOT NULL,PRIMARY KEY(key_id,period,provider,model));
CREATE TABLE accounting_reservations(id TEXT PRIMARY KEY,key_id TEXT NOT NULL,key_name TEXT NOT NULL,provider TEXT NOT NULL,model TEXT NOT NULL,day TEXT NOT NULL,month TEXT NOT NULL,requests INTEGER NOT NULL,input_tokens INTEGER NOT NULL,output_tokens INTEGER NOT NULL,cost REAL NOT NULL,unknown_cost INTEGER NOT NULL,estimated_tokens INTEGER NOT NULL,status TEXT NOT NULL);
`}, {Version: 9, SQL: `
CREATE TABLE activity_storage(id INTEGER PRIMARY KEY CHECK(id=1),bytes INTEGER NOT NULL); INSERT INTO activity_storage VALUES(1,0);
CREATE TABLE activity_events(id INTEGER PRIMARY KEY AUTOINCREMENT,at INTEGER NOT NULL,document BLOB NOT NULL,size INTEGER NOT NULL);
CREATE INDEX activity_time ON activity_events(at,id);
CREATE TRIGGER activity_insert AFTER INSERT ON activity_events BEGIN UPDATE activity_storage SET bytes=bytes+NEW.size WHERE id=1; END;
CREATE TRIGGER activity_delete AFTER DELETE ON activity_events BEGIN UPDATE activity_storage SET bytes=bytes-OLD.size WHERE id=1; END;
`}}

const credentialSchema = `
CREATE TABLE credentials (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL, service TEXT NOT NULL, purpose TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, key_version INTEGER NOT NULL,
 nonce BLOB NOT NULL, ciphertext BLOB NOT NULL, tag BLOB NOT NULL,
 deleted INTEGER NOT NULL DEFAULT 0, UNIQUE(key_version,nonce));
`

func checkSchema(version, tables, target int) error {
	if version == 0 && tables == 0 {
		return nil
	}
	if version < BaselineSchemaVersion {
		return errors.New("database predates the database-only baseline: stop Torii, archive the database and configuration, then provision a fresh database and configuration; see docs/guides/backup-and-upgrade.md for the one-time reset")
	}
	if version > target {
		return errors.New("database schema is newer than this gateway; upgrade the gateway or restore its matching pre-upgrade backup")
	}
	return nil
}

func (d *Database) initializeSchema() error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(credentialSchema + benchmarkSchema + runtimeSchema); err != nil {
		return errors.New("cannot initialize database schema; check the database volume")
	}
	if err = d.write(tx, "__key_check", Owner, "gateway", "key_check", []byte("torii-key-check-v1")); err != nil {
		return err
	}
	if _, err = tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", BaselineSchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema(db *sql.DB, from, target int, migrations []schemaMigration) error {
	if from < BaselineSchemaVersion || target < from {
		return errors.New("unsupported database schema upgrade")
	}
	// Validate the entire path before allowing any write.
	steps := make(map[int]string)
	for _, m := range migrations {
		if m.Version <= BaselineSchemaVersion || m.SQL == "" {
			return errors.New("invalid database schema migration")
		}
		if _, exists := steps[m.Version]; exists {
			return errors.New("duplicate database schema migration")
		}
		steps[m.Version] = m.SQL
	}
	for version := from + 1; version <= target; version++ {
		if _, ok := steps[version]; !ok {
			return errors.New("missing database schema migration; install a complete gateway release")
		}
	}
	for version := from + 1; version <= target; version++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(steps[version]); err == nil {
			_, err = tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", version))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("database schema upgrade to %d failed; preserve the database and restore its pre-upgrade backup if needed", version)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("cannot commit database schema upgrade to %d", version)
		}
	}
	return nil
}
