package intelligence

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const cacheVersion = 1
const maxCacheBytes = 128 << 20

type Store struct{ path string }

func OpenStore(directory string) (*Store, error) {
	// Keep this separate from credentials and usage, inside existing state.directory.
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(directory, "benchmark-intelligence.json")}, nil
}

func (s *Store) Load() (Snapshot, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{Version: cacheVersion, Sources: map[string]SourceState{}}, nil
	}
	if err != nil {
		return Snapshot{}, errors.New("cannot open benchmark cache")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxCacheBytes+1))
	if err != nil || len(b) > maxCacheBytes {
		return Snapshot{}, errors.New("benchmark cache unreadable or too large")
	}
	var snapshot Snapshot
	if json.Unmarshal(b, &snapshot) != nil || snapshot.Version != cacheVersion || snapshot.Sources == nil {
		return Snapshot{}, errors.New("benchmark cache corrupt or unsupported schema version")
	}
	for _, source := range snapshot.Sources {
		if len(source.Observations) == 0 {
			continue
		}
		if err := ValidateObservations(source.Observations); err != nil {
			return Snapshot{}, errors.New("benchmark cache contains invalid observations")
		}
	}
	return snapshot, nil
}

func (s *Store) Save(snapshot Snapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(data) > maxCacheBytes {
		return errors.New("benchmark cache exceeds 128 MiB")
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".benchmark-*")
	if err != nil {
		return errors.New("cannot create benchmark cache replacement")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		return errors.New("cannot atomically replace benchmark cache")
	}
	return nil
}
