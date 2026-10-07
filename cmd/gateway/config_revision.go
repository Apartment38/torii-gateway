package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/Apartment38/torii-gateway/internal/config"
	"gopkg.in/yaml.v3"
)

// configRevision requires configMu. A runtime-specific key keeps revisions
// opaque and invalidates old tabs after restart. Canonical configuration bytes
// detect edits made outside the API; the version also advances on no-op saves.
func (r *handlerRuntime) configRevision(cfg *config.Config) (string, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	if r.configKey == nil {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return "", err
		}
		r.configKey = key
	}
	mac := hmac.New(sha256.New, r.configKey)
	fmt.Fprintf(mac, "%d\n", r.configVersion)
	mac.Write(data)
	return `"` + hex.EncodeToString(mac.Sum(nil)) + `"`, nil
}
