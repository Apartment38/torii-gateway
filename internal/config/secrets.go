package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// SecretRef identifies a secret stored outside the configuration file.
// Exactly one of Env and File must be configured.
type SecretRef struct {
	Env  string `yaml:"env,omitempty"`
	File string `yaml:"file,omitempty"`
}

// Validate checks that the reference selects exactly one non-blank source.
func (r SecretRef) Validate() error {
	hasEnv := r.Env != ""
	hasFile := r.File != ""
	if hasEnv == hasFile {
		return errors.New("exactly one of env or file must be configured")
	}
	if hasEnv && strings.TrimSpace(r.Env) == "" {
		return errors.New("env must not be blank")
	}
	if hasFile && strings.TrimSpace(r.File) == "" {
		return errors.New("file must not be blank")
	}
	return nil
}

// Resolve loads the referenced secret. File-backed values have surrounding
// whitespace removed; environment values are returned unchanged.
func (r SecretRef) Resolve() (string, error) {
	if err := r.Validate(); err != nil {
		return "", fmt.Errorf("validate secret reference: %w", err)
	}
	if r.Env != "" {
		value, ok := os.LookupEnv(r.Env)
		if !ok {
			return "", fmt.Errorf("resolve secret: environment variable %q is not set", r.Env)
		}
		return value, nil
	}

	value, err := os.ReadFile(r.File)
	if err != nil {
		return "", fmt.Errorf("resolve secret from file %q: %w", r.File, err)
	}
	return strings.TrimSpace(string(value)), nil
}
