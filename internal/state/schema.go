package state

import (
	"database/sql"
	"errors"
	"fmt"
)

// BaselineSchemaVersion is the first database-only release. Older installations
// must be recreated once; all subsequent upgrades preserve their database.
const BaselineSchemaVersion = 4
const SchemaVersion = 6

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
