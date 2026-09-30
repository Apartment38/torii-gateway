// Package state persists the minimal durable gateway state.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Apartment38/torii-router/internal/provider"
)

const (
	credentialVersion = 1
	maxCredentialSize = 1 << 20
)

var ErrCredentialNotFound = errors.New("credential not found")

type Credential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	PrincipalID  string    `json:"principal_id,omitempty"`
	Endpoint     string    `json:"endpoint"`
}

type CredentialStore interface {
	Load(providerID string) (*Credential, error)
	Save(providerID string, credential *Credential) error
	Delete(providerID string) error
}

type FlatFileCredentialStore struct{ directory string }

func NewFlatFileCredentialStore(stateDirectory string) (*FlatFileCredentialStore, error) {
	if stateDirectory == "" {
		return nil, errors.New("state directory is required")
	}
	directory := filepath.Join(stateDirectory, "auth")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create credential directory: %w", err)
	}
	if err := rejectSymlink(directory); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure credential directory: %w", err)
	}
	return &FlatFileCredentialStore{directory: directory}, nil
}

func (s *FlatFileCredentialStore) Load(providerID string) (*Credential, error) {
	path, err := s.path(providerID)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspect credential for provider %q: %w", providerID, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("credential for provider %q is not a regular file", providerID)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("credential for provider %q has unsafe permissions", providerID)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open credential for provider %q: %w", providerID, err)
	}
	defer f.Close()
	limited := &io.LimitedReader{R: f, N: maxCredentialSize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read credential for provider %q: %w", providerID, err)
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("credential for provider %q exceeds size limit", providerID)
	}
	var record credentialRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("credential for provider %q is corrupt", providerID)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("credential for provider %q is corrupt", providerID)
	}
	if record.Version != credentialVersion || record.Provider != providerID || validateCredential(&record.Credential) != nil {
		return nil, fmt.Errorf("credential for provider %q is corrupt", providerID)
	}
	credential := record.Credential
	return &credential, nil
}

func (s *FlatFileCredentialStore) Save(providerID string, credential *Credential) error {
	path, err := s.path(providerID)
	if err != nil {
		return err
	}
	if err := validateCredential(credential); err != nil {
		return fmt.Errorf("credential for provider %q: %w", providerID, err)
	}
	record := credentialRecord{Version: credentialVersion, Provider: providerID, Credential: *credential}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode credential for provider %q", providerID)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(s.directory, ".credential-*")
	if err != nil {
		return fmt.Errorf("create credential replacement for provider %q: %w", providerID, err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure credential replacement for provider %q: %w", providerID, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write credential replacement for provider %q: %w", providerID, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync credential replacement for provider %q: %w", providerID, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close credential replacement for provider %q: %w", providerID, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace credential for provider %q: %w", providerID, err)
	}
	keep = true
	if err := syncDirectory(s.directory); err != nil {
		return fmt.Errorf("sync credential directory: %w", err)
	}
	return nil
}

func (s *FlatFileCredentialStore) Delete(providerID string) error {
	path, err := s.path(providerID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete credential for provider %q: %w", providerID, err)
	}
	return syncDirectory(s.directory)
}

type credentialRecord struct {
	Version    int        `json:"version"`
	Provider   string     `json:"provider"`
	Credential Credential `json:"credential"`
}

func (s *FlatFileCredentialStore) path(providerID string) (string, error) {
	if s == nil || s.directory == "" {
		return "", errors.New("credential store is not initialized")
	}
	if err := provider.ValidateID(providerID); err != nil {
		return "", fmt.Errorf("invalid credential provider: %w", err)
	}
	return filepath.Join(s.directory, providerID+".json"), nil
}

func validateCredential(c *Credential) error {
	if c == nil {
		return errors.New("is required")
	}
	if c.AccessToken == "" {
		return errors.New("access token is required")
	}
	if c.Endpoint == "" {
		return errors.New("endpoint binding is required")
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect credential directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("credential directory is not a real directory")
	}
	return nil
}
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
