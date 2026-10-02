package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func migrateBenchmarks(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
 CREATE TABLE benchmark_datasets (
   id TEXT PRIMARY KEY, source_id TEXT NOT NULL, observation_count INTEGER NOT NULL,
   content_hash TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(id,source_id));
 CREATE TABLE benchmark_sources (
   source_id TEXT PRIMARY KEY, type TEXT NOT NULL, last_attempt TEXT, last_success TEXT,
   error TEXT NOT NULL, metadata BLOB NOT NULL, current_dataset TEXT,
   FOREIGN KEY(current_dataset,source_id) REFERENCES benchmark_datasets(id,source_id));
 CREATE INDEX benchmark_source_dataset ON benchmark_sources(current_dataset);
 CREATE TABLE benchmark_observations (
   dataset_id TEXT NOT NULL REFERENCES benchmark_datasets(id), ordinal INTEGER NOT NULL,
   external_model_id TEXT NOT NULL, benchmark TEXT NOT NULL, category TEXT NOT NULL,
   original_score REAL NOT NULL, normalized_score REAL, unit TEXT NOT NULL, version TEXT NOT NULL,
   evaluated_at TEXT, retrieved_at TEXT, date_basis TEXT NOT NULL, metadata BLOB NOT NULL,
   PRIMARY KEY(dataset_id,ordinal));
 CREATE INDEX benchmark_model ON benchmark_observations(external_model_id,dataset_id);
 CREATE INDEX benchmark_category ON benchmark_observations(category,benchmark,dataset_id);
 CREATE TABLE benchmark_storage (
   id INTEGER PRIMARY KEY CHECK(id=1), updated_at TEXT, published INTEGER NOT NULL DEFAULT 0,
   legacy_import_done INTEGER NOT NULL DEFAULT 0);
 INSERT INTO benchmark_storage(id) VALUES(1);
 PRAGMA user_version=2;`)
	if err != nil {
		return errors.New("cannot migrate benchmark database schema; preserve the database and restore a pre-upgrade backup if necessary")
	}
	return tx.Commit()
}

// BenchmarkConnection uses a separate pool so WAL credential readers do not
// queue behind benchmark staging. The caller owns and closes this pool.
func (d *Database) BenchmarkConnection() (*sql.DB, error) {
	return d.StateConnection()
}

// StateConnection opens an independently owned pool for public durable state.
func (d *Database) StateConnection() (*sql.DB, error) {
	db, err := sql.Open("sqlite", d.dsn)
	if err != nil {
		return nil, errors.New("cannot open state database connection")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("cannot connect to state database")
	}
	return db, nil
}

// LockBenchmarkWriter coordinates active/speculative handlers in one gateway.
// Each persistent database volume belongs to one gateway process.
func (d *Database) LockBenchmarkWriter(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case d.benchmarkGate <- struct{}{}:
		return func() { <-d.benchmarkGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
