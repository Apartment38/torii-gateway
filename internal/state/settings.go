package state

import (
	"database/sql"
	"errors"
	"strconv"
)

var ErrSettingsConflict = errors.New("configuration revision conflict")

type SubmittedSecret struct {
	ID, Service, Purpose string
	Value                []byte
}

// Settings returns the authoritative document and persistent revision.
func (d *Database) Settings() ([]byte, string, error) {
	var data []byte
	var revision int64
	err := d.db.QueryRow("SELECT document,revision FROM gateway_settings WHERE id=1").Scan(&data, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	return data, strconv.FormatInt(revision, 10), err
}

// CommitSettings atomically installs settings and encrypted credentials. Empty
// expected means first import/initialization; it never replaces established data.
func (d *Database) CommitSettings(data []byte, expected string, secrets []SubmittedSecret) (string, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var revision int64
	err = tx.QueryRow("SELECT revision FROM gateway_settings WHERE id=1").Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		if expected != "" {
			return "", ErrSettingsConflict
		}
	} else if err != nil {
		return "", err
	} else if expected != strconv.FormatInt(revision, 10) {
		return "", ErrSettingsConflict
	}
	for _, s := range secrets {
		if err = d.write(tx, s.ID, Owner, s.Service, s.Purpose, s.Value); err != nil {
			return "", err
		}
	}
	revision++
	_, err = tx.Exec("INSERT INTO gateway_settings(id,document,revision) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET document=excluded.document,revision=excluded.revision", data, revision)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return strconv.FormatInt(revision, 10), nil
}
