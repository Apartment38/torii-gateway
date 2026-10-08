package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// SecretRef identifies a secret stored outside the configuration file, or an
// explicitly configured inline value for console-managed local deployments.
// Exactly one source must be configured.
type SecretRef struct {
	Credential string `yaml:"credential,omitempty"`
	store      SecretStore
	Env        string `yaml:"env,omitempty"`
	File       string `yaml:"file,omitempty"`
	Value      string `yaml:"value,omitempty"`
}

type SecretStore interface {
	Get(string, string) ([]byte, error)
}

func (r *SecretRef) Bind(store SecretStore) { r.store = store }
func (r *SecretRef) BindDefault(store SecretStore) {
	if r.store == nil {
		r.store = store
	}
}

// Validate checks that the reference selects exactly one non-blank source.
func (r SecretRef) Validate() error {
	hasEnv := r.Env != ""
	hasFile := r.File != ""
	hasValue := r.Value != ""
	if (btoi(hasEnv) + btoi(hasFile) + btoi(hasValue) + btoi(r.Credential != "")) != 1 {
		return errors.New("exactly one of env, file, value, or credential must be configured")
	}
	if hasEnv && strings.TrimSpace(r.Env) == "" {
		return errors.New("env must not be blank")
	}
	if hasFile && strings.TrimSpace(r.File) == "" {
		return errors.New("file must not be blank")
	}
	if hasValue && strings.TrimSpace(r.Value) == "" {
		return errors.New("value must not be blank")
	}
	return nil
}

// Resolve loads the referenced secret. File-backed values have surrounding
// whitespace removed; environment values are returned unchanged.
func (r SecretRef) Resolve() (string, error) {
	if err := r.Validate(); err != nil {
		return "", fmt.Errorf("validate secret reference: %w", err)
	}
	if r.Credential != "" {
		if r.store == nil {
			return "", errors.New("credential store not initialized")
		}
		b, err := r.store.Get(r.Credential, "default")
		return string(b), err
	}
	if r.Env != "" {
		value, ok := os.LookupEnv(r.Env)
		if !ok {
			return "", fmt.Errorf("resolve secret: environment variable %q is not set", r.Env)
		}
		return value, nil
	}
	if r.Value != "" {
		return r.Value, nil
	}

	value, err := os.ReadFile(r.File)
	if err != nil {
		return "", fmt.Errorf("resolve secret from file %q: %w", r.File, err)
	}
	return strings.TrimSpace(string(value)), nil
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}
