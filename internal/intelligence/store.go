package intelligence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/Apartment38/torii-gateway/internal/state"
)

const cacheVersion = 1
const maxCacheBytes = 128 << 20
const observationBatch = 250

type SnapshotStore interface {
	Load(context.Context) (Snapshot, error)
	Save(context.Context, Snapshot) error
	Close() error
}
type Store struct {
	db       *sql.DB
	database *state.Database
}
type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type preparedSource struct {
	status              SourceState
	hash                string
	metadata            []byte
	observationMetadata [][]byte
}

func OpenStore(ctx context.Context, database *state.Database, logger *slog.Logger) (*Store, error) {
	if database == nil {
		return nil, errors.New("benchmark storage requires the application database")
	}
	db, err := database.BenchmarkConnection()
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, database: database}
	if logger == nil {
		logger = slog.Default()
	}
	unlock, err := database.LockBenchmarkWriter(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	defer unlock()
	if err = s.cleanup(ctx); err != nil {
		logger.Warn("benchmark_cleanup_failed", "error", err)
	}
	if err = ctx.Err(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func emptySnapshot() Snapshot {
	return Snapshot{Version: cacheVersion, Sources: map[string]SourceState{}}
}
func stamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
func parseStamp(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return time.Time{}, errors.New("invalid benchmark timestamp")
	}
	return t, nil
}
func metadata(value map[string]any) (map[string]any, []byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, nil, errors.New("benchmark metadata is not serializable")
	}
	var normalized map[string]any
	if json.Unmarshal(b, &normalized) != nil {
		return nil, nil, errors.New("benchmark metadata is invalid")
	}
	b, err = json.Marshal(normalized)
	return normalized, b, err
}
func observationHash(observations []Observation) string {
	b, _ := json.Marshal(observations)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func prepare(snapshot Snapshot) (map[string]preparedSource, error) {
	if snapshot.Version != cacheVersion || snapshot.Sources == nil {
		return nil, errors.New("unsupported benchmark snapshot")
	}
	canonical := emptySnapshot()
	canonical.UpdatedAt = snapshot.UpdatedAt.UTC()
	prepared := map[string]preparedSource{}
	for id, source := range snapshot.Sources {
		if id == "" {
			return nil, errors.New("benchmark source identifier is empty")
		}
		source.ID = id
		source.AgeSeconds = 0
		source.Stale = false
		source.LastAttempt = source.LastAttempt.UTC()
		source.LastSuccess = source.LastSuccess.UTC()
		normalized, b, err := metadata(source.Metadata)
		if err != nil {
			return nil, err
		}
		source.Metadata = normalized
		p := preparedSource{metadata: b}
		if len(source.Observations) > 0 {
			if err := ValidateObservations(source.Observations); err != nil {
				return nil, err
			}
			observations := make([]Observation, len(source.Observations))
			for i, o := range source.Observations {
				o.Source = id
				o.ModelID = ""
				o.EvaluatedAt = o.EvaluatedAt.UTC()
				o.RetrievedAt = o.RetrievedAt.UTC()
				if o.Score == 0 {
					o.Score = 0
				}
				if o.Normalized != nil {
					v := *o.Normalized
					if v == 0 {
						v = 0
					}
					o.Normalized = &v
				}
				o.Metadata, b, err = metadata(o.Metadata)
				if err != nil {
					return nil, err
				}
				p.observationMetadata = append(p.observationMetadata, b)
				observations[i] = o
			}
			source.Observations = observations
			p.hash = observationHash(observations)
		}
		p.status = source
		prepared[id] = p
		canonical.Sources[id] = source
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return nil, errors.New("invalid benchmark snapshot")
	}
	if len(data) > maxCacheBytes {
		return nil, errors.New("benchmark cache exceeds 128 MiB")
	}
	return prepared, nil
}

func (s *Store) Load(ctx context.Context) (result Snapshot, loadErr error) {
	defer func() {
		if loadErr != nil && ctx.Err() != nil {
			loadErr = ctx.Err()
		}
	}()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, errors.New("cannot read benchmark storage")
	}
	defer tx.Rollback()
	snapshot := emptySnapshot()
	var updated sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT updated_at FROM benchmark_storage WHERE id=1").Scan(&updated); err != nil {
		return Snapshot{}, errors.New("cannot read benchmark storage metadata")
	}
	snapshot.UpdatedAt, err = parseStamp(updated)
	if err != nil {
		return Snapshot{}, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT source_id,type,last_attempt,last_success,error,metadata,current_dataset FROM benchmark_sources ORDER BY source_id")
	if err != nil {
		return Snapshot{}, errors.New("cannot read benchmark sources")
	}
	datasets := map[string]string{}
	for rows.Next() {
		var source SourceState
		var attempt, success, dataset sql.NullString
		var b []byte
		if err = rows.Scan(&source.ID, &source.Type, &attempt, &success, &source.Error, &b, &dataset); err != nil {
			rows.Close()
			return Snapshot{}, errors.New("invalid benchmark source record")
		}
		source.LastAttempt, err = parseStamp(attempt)
		if err == nil {
			source.LastSuccess, err = parseStamp(success)
		}
		if err != nil || json.Unmarshal(b, &source.Metadata) != nil {
			rows.Close()
			return Snapshot{}, errors.New("invalid benchmark source metadata")
		}
		snapshot.Sources[source.ID] = source
		if dataset.Valid {
			datasets[source.ID] = dataset.String
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Snapshot{}, errors.New("cannot read benchmark sources")
	}
	for sourceID, dataset := range datasets {
		source := snapshot.Sources[sourceID]
		source.Observations, err = s.readObservations(ctx, tx, dataset, sourceID)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Sources[sourceID] = source
	}
	if _, err = prepare(snapshot); err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, errors.New("cannot finish benchmark read")
	}
	return snapshot, nil
}

func (s *Store) readObservations(ctx context.Context, q queryer, dataset, sourceID string) ([]Observation, error) {
	var count int
	var hash string
	if err := q.QueryRowContext(ctx, "SELECT observation_count,content_hash FROM benchmark_datasets WHERE id=? AND source_id=?", dataset, sourceID).Scan(&count, &hash); err != nil {
		return nil, errors.New("invalid benchmark dataset reference")
	}
	rows, err := q.QueryContext(ctx, `SELECT ordinal,external_model_id,benchmark,category,original_score,normalized_score,unit,version,evaluated_at,retrieved_at,date_basis,metadata FROM benchmark_observations WHERE dataset_id=? ORDER BY ordinal`, dataset)
	if err != nil {
		return nil, errors.New("cannot read benchmark observations")
	}
	defer rows.Close()
	var observations []Observation
	for rows.Next() {
		var o Observation
		var ordinal int
		var normalized sql.NullFloat64
		var evaluated, retrieved sql.NullString
		var b []byte
		if err = rows.Scan(&ordinal, &o.ExternalModelID, &o.Benchmark, &o.Category, &o.Score, &normalized, &o.Unit, &o.Version, &evaluated, &retrieved, &o.DateBasis, &b); err != nil {
			return nil, errors.New("invalid benchmark observation record")
		}
		if ordinal != len(observations) {
			return nil, errors.New("benchmark dataset is incomplete")
		}
		o.Source = sourceID
		if normalized.Valid {
			v := normalized.Float64
			o.Normalized = &v
		}
		o.EvaluatedAt, err = parseStamp(evaluated)
		if err == nil {
			o.RetrievedAt, err = parseStamp(retrieved)
		}
		if err != nil || json.Unmarshal(b, &o.Metadata) != nil {
			return nil, errors.New("invalid benchmark observation metadata")
		}
		observations = append(observations, o)
	}
	if rows.Err() != nil {
		return nil, errors.New("cannot read benchmark observations")
	}
	if len(observations) != count || count == 0 || observationHash(observations) != hash {
		return nil, errors.New("benchmark dataset verification failed")
	}
	if err = ValidateObservations(observations); err != nil {
		return nil, err
	}
	return observations, nil
}

func (s *Store) Save(ctx context.Context, snapshot Snapshot) (saveErr error) {
	defer func() {
		if saveErr != nil && ctx.Err() != nil {
			saveErr = ctx.Err()
		}
	}()
	prepared, err := prepare(snapshot)
	if err != nil {
		return err
	}
	unlock, err := s.database.LockBenchmarkWriter(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.savePrepared(ctx, snapshot.UpdatedAt, prepared)
}

func (s *Store) savePrepared(ctx context.Context, updated time.Time, prepared map[string]preparedSource) error {
	if err := s.cleanup(ctx); err != nil {
		return err
	}
	ids := make([]string, 0, len(prepared))
	for id := range prepared {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	datasets := map[string]any{}
	for _, id := range ids {
		p := prepared[id]
		if len(p.status.Observations) == 0 {
			datasets[id] = nil
			continue
		}
		var previous, hash, sourceType sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT s.current_dataset,d.content_hash,s.type FROM benchmark_sources s LEFT JOIN benchmark_datasets d ON d.id=s.current_dataset WHERE s.source_id=?`, id).Scan(&previous, &hash, &sourceType)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.New("cannot inspect benchmark dataset")
		}
		if previous.Valid && hash.String == p.hash && sourceType.String == p.status.Type {
			if _, err := s.readObservations(ctx, s.db, previous.String, id); err == nil {
				datasets[id] = previous.String
				continue
			}
		}
		dataset := state.NewSecretID()
		datasets[id] = dataset
		if _, err = s.db.ExecContext(ctx, "INSERT INTO benchmark_datasets(id,source_id,observation_count,content_hash,created_at) VALUES(?,?,?,?,?)", dataset, id, len(p.status.Observations), p.hash, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return errors.New("cannot stage benchmark dataset")
		}
		for start := 0; start < len(p.status.Observations); start += observationBatch {
			if err = s.stageBatch(ctx, dataset, p, start, min(start+observationBatch, len(p.status.Observations))); err != nil {
				return err
			}
		}
		if _, err = s.readObservations(ctx, s.db, dataset, id); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cannot publish benchmark dataset")
	}
	defer tx.Rollback()
	for _, id := range ids {
		p := prepared[id]
		_, err = tx.ExecContext(ctx, `INSERT INTO benchmark_sources(source_id,type,last_attempt,last_success,error,metadata,current_dataset) VALUES(?,?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET type=excluded.type,last_attempt=excluded.last_attempt,last_success=excluded.last_success,error=excluded.error,metadata=excluded.metadata,current_dataset=excluded.current_dataset`, id, p.status.Type, stamp(p.status.LastAttempt), stamp(p.status.LastSuccess), p.status.Error, p.metadata, datasets[id])
		if err != nil {
			return errors.New("cannot publish benchmark source status")
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE benchmark_storage SET updated_at=? WHERE id=1", stamp(updated))
	if err != nil {
		return errors.New("cannot publish benchmark storage metadata")
	}
	if err = tx.Commit(); err != nil {
		return errors.New("cannot commit benchmark publication")
	}
	// Publication already succeeded; cleanup failure must not reject the snapshot.
	if err = s.cleanup(ctx); err != nil {
		slog.Warn("benchmark_cleanup_failed", "message", "Published data preserved; abandoned data will be cleaned on next open or refresh")
	}
	return nil
}
func (s *Store) stageBatch(ctx context.Context, dataset string, p preparedSource, start, end int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cannot stage benchmark observations")
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO benchmark_observations(dataset_id,ordinal,external_model_id,benchmark,category,original_score,normalized_score,unit,version,evaluated_at,retrieved_at,date_basis,metadata) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return errors.New("cannot prepare benchmark observations")
	}
	defer statement.Close()
	for i := start; i < end; i++ {
		o := p.status.Observations[i]
		if _, err = statement.ExecContext(ctx, dataset, i, o.ExternalModelID, o.Benchmark, o.Category, o.Score, o.Normalized, o.Unit, o.Version, stamp(o.EvaluatedAt), stamp(o.RetrievedAt), o.DateBasis, p.observationMetadata[i]); err != nil {
			return errors.New("cannot stage benchmark observations")
		}
	}
	if err = tx.Commit(); err != nil {
		return errors.New("cannot commit benchmark staging batch")
	}
	return nil
}
func (s *Store) cleanup(ctx context.Context) error {
	for {
		result, err := s.db.ExecContext(ctx, `DELETE FROM benchmark_observations WHERE rowid IN (SELECT o.rowid FROM benchmark_observations o LEFT JOIN benchmark_sources s ON s.current_dataset=o.dataset_id WHERE s.source_id IS NULL LIMIT 250)`)
		if err != nil {
			return errors.New("cannot clean unpublished benchmark observations")
		}
		n, err := result.RowsAffected()
		if err != nil {
			return errors.New("cannot inspect benchmark cleanup")
		}
		if n == 0 {
			break
		}
	}
	for {
		result, err := s.db.ExecContext(ctx, `DELETE FROM benchmark_datasets WHERE id IN (SELECT d.id FROM benchmark_datasets d LEFT JOIN benchmark_sources s ON s.current_dataset=d.id WHERE s.source_id IS NULL AND NOT EXISTS(SELECT 1 FROM benchmark_observations o WHERE o.dataset_id=d.id) LIMIT 250)`)
		if err != nil {
			return errors.New("cannot clean unpublished benchmark datasets")
		}
		n, err := result.RowsAffected()
		if err != nil {
			return errors.New("cannot inspect benchmark cleanup")
		}
		if n == 0 {
			return nil
		}
	}
}
