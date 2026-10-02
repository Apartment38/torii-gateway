package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/state"
	"gopkg.in/yaml.v3"
)

// VisitSecrets is the complete inventory of configuration credential fields.
func (c *Config) VisitSecrets(fn func(path, service, purpose string, ref *SecretRef) error) error {
	if c.Server.Authentication != nil {
		if err := fn("server/authentication/api_key", "gateway", "authentication", &c.Server.Authentication.APIKey); err != nil {
			return err
		}
	}
	for i := range c.Providers {
		p := &c.Providers[i]
		base := "providers/" + strconv.Itoa(i) + "/"
		for _, v := range []struct {
			name string
			ref  *SecretRef
		}{{"bearer_token", p.BearerToken}, {"management_token", p.ManagementToken}} {
			if v.ref != nil {
				if err := fn(base+v.name, p.ID, v.name, v.ref); err != nil {
					return err
				}
			}
		}
		for name, ref := range p.Headers {
			if err := fn(base+"headers/"+name, p.ID, "header:"+name, &ref); err != nil {
				return err
			}
			p.Headers[name] = ref
		}
	}
	if c.Router.Classifier.APIKey != nil {
		if err := fn("router/classifier/api_key", "classifier", "api_key", c.Router.Classifier.APIKey); err != nil {
			return err
		}
	}
	for i := range c.Intelligence.Sources {
		s := &c.Intelligence.Sources[i]
		if s.APIKey != nil {
			if err := fn("intelligence/sources/"+strconv.Itoa(i)+"/api_key", "benchmark:"+s.ID, "api_key", s.APIKey); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *Config) BindSecrets(db *state.Database) error {
	c.Credentials = db
	return c.VisitSecrets(func(_, _, _ string, r *SecretRef) error {
		if r.Credential == "" {
			return errors.New("legacy credential requires startup migration")
		}
		r.Bind(db)
		return nil
	})
}

// AttachStoredSecrets sanitizes read-only legacy configuration without consulting
// its old values. Only committed migration mappings may be used.
func (c *Config) AttachStoredSecrets(db *state.Database) error {
	c.Credentials = db
	return c.VisitSecrets(func(_, service, purpose string, r *SecretRef) error {
		if strings.HasPrefix(r.Credential, "__") || (r.Credential == state.GatewayAPIKeyID && service != "gateway") {
			return errors.New("system credentials cannot be used as provider credentials")
		}
		if r.Credential == "" {
			id := importID(service, purpose)
			ok, err := db.Imported(id)
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("configuration contains unmigrated credentials; restart to migrate")
			}
			*r = SecretRef{Credential: id}
		}
		r.Bind(db)
		return nil
	})
}
func importID(service, purpose string) string {
	sum := sha256.Sum256([]byte(service + "\x00" + purpose))
	return "legacy/" + hex.EncodeToString(sum[:])
}

// MigrateCredentials commits verified ciphertext before touching any legacy file.
// Already imported locations never resolve legacy inputs again.
func MigrateCredentials(c *Config, path string, db *state.Database) error {
	c.Credentials = db
	var imports []state.Import
	replacements := map[string]string{}
	err := c.VisitSecrets(func(location, service, purpose string, r *SecretRef) error {
		if r.Credential != "" {
			r.Bind(db)
			return nil
		}
		id := importID(service, purpose)
		exists, err := db.Imported(id)
		if err != nil {
			return err
		}
		if !exists {
			value, err := r.Resolve()
			if err != nil {
				return fmt.Errorf("cannot migrate %s credential: ensure its environment variable or file is available", service)
			}
			if value == "" {
				return fmt.Errorf("cannot migrate empty credential for %s", service)
			}
			imports = append(imports, state.Import{ID: id, Service: service, Purpose: purpose, Value: []byte(value)})
		}
		replacements[location] = id
		return nil
	})
	if err != nil {
		return err
	}
	// Include all old OAuth files, even providers currently absent from YAML.
	var oauthFiles []string
	files, err := filepath.Glob(filepath.Join(c.State.Directory, "auth", "*.json"))
	if err != nil {
		return err
	}
	for _, file := range files {
		provider := strings.TrimSuffix(filepath.Base(file), ".json")
		id := "oauth/" + provider
		exists, err := db.Imported(id)
		if err != nil {
			return err
		}
		if !exists {
			legacy, err := state.NewFlatFileCredentialStore(c.State.Directory)
			if err != nil {
				return err
			}
			credential, err := legacy.Load(provider)
			if err != nil {
				return fmt.Errorf("cannot migrate OAuth credentials for %s", provider)
			}
			b, err := json.Marshal(credential)
			if err != nil {
				return err
			}
			imports = append(imports, state.Import{ID: id, Service: provider, Purpose: "oauth", Value: b})
		}
		oauthFiles = append(oauthFiles, file)
	}
	if err := db.ImportOnce(imports); err != nil {
		return err
	}
	if len(imports) > 0 {
		slog.Info("credentials_imported", "count", len(imports), "message", "Legacy external secret inputs are no longer used; remove unused provider secret files and environment variables after verifying the installation. Keep the database master key.")
	}
	if err := c.VisitSecrets(func(location, _, _ string, r *SecretRef) error {
		if id, ok := replacements[location]; ok {
			*r = SecretRef{Credential: id}
		}
		r.Bind(db)
		return nil
	}); err != nil {
		return err
	}
	if len(replacements) > 0 {
		b, err := os.ReadFile(path)
		var node yaml.Node
		if err == nil {
			err = yaml.Unmarshal(b, &node)
		}
		if err == nil {
			for location, id := range replacements {
				if err = replaceSecretNode(&node, strings.Split(location, "/"), id); err != nil {
					break
				}
			}
		}
		if err == nil {
			b, err = yaml.Marshal(&node)
		}
		if err == nil {
			err = atomicConfig(path, b)
		}
		if err != nil {
			slog.Warn("credential_yaml_cleanup_required", "message", "Credentials are encrypted and legacy values are ignored. Remove value/env/file fields from the host YAML and replace them with database references; use authenticated GET /gateway/v1/config to obtain sanitized configuration.")
		}
	}
	for _, file := range oauthFiles {
		if err := os.Remove(file); err != nil {
			slog.Warn("credential_oauth_cleanup_required", "message", "Encrypted import succeeded; remove legacy state/auth credential JSON files on the host")
		}
	}
	return nil
}
func replaceSecretNode(n *yaml.Node, path []string, id string) error {
	if n.Kind == yaml.DocumentNode {
		return replaceSecretNode(n.Content[0], path, id)
	}
	if n.Kind == yaml.AliasNode {
		return errors.New("aliased YAML requires manual credential cleanup")
	}
	if len(path) == 0 {
		comment := n.HeadComment
		*n = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", HeadComment: comment, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "credential"}, {Kind: yaml.ScalarNode, Tag: "!!str", Value: id}}}
		return nil
	}
	if n.Kind == yaml.SequenceNode {
		i, err := strconv.Atoi(path[0])
		if err != nil || i < 0 || i >= len(n.Content) {
			return errors.New("invalid YAML credential path")
		}
		return replaceSecretNode(n.Content[i], path[1:], id)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == path[0] {
			return replaceSecretNode(n.Content[i+1], path[1:], id)
		}
	}
	return errors.New("missing YAML credential field")
}
func atomicConfig(path string, b []byte) error {
	if info, err := os.Lstat(path); err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0200 == 0 {
		return errors.New("legacy configuration requires manual cleanup")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".torii-secrets-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	return err
}

// Gateway authentication settings migrate to SQLite and are no longer needed
// in ordinary configuration. Read-only originals remain ignored after import.
func RemoveLegacyAuthentication(c *Config, path string) {
	c.Server.Authentication = nil
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var n yaml.Node
	if yaml.Unmarshal(b, &n) != nil || len(n.Content) == 0 {
		return
	}
	root := n.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "server" {
			server := root.Content[i+1]
			for j := 0; j+1 < len(server.Content); j += 2 {
				if server.Content[j].Value == "authentication" {
					server.Content = append(server.Content[:j], server.Content[j+2:]...)
					b, err = yaml.Marshal(&n)
					if err == nil {
						err = atomicConfig(path, b)
					}
					if err != nil {
						slog.Warn("legacy_authentication_cleanup_required", "message", "Gateway authentication is now managed in SQLite; remove server.authentication from the host YAML")
					}
					return
				}
			}
		}
	}
}

// StoreSubmittedCredentials is used only behind authenticated management APIs.
// Existing references are ownership-checked; direct input is never serialized.
func StoreSubmittedCredentials(c *Config, db *state.Database) error {
	c.Credentials = db
	return c.VisitSecrets(func(_, service, purpose string, r *SecretRef) error {
		if strings.HasPrefix(r.Credential, "__") || r.Credential == state.GatewayAPIKeyID {
			return errors.New("system credentials cannot be used as provider credentials")
		}
		if r.Credential == "" {
			if r.Env != "" || r.File != "" {
				return errors.New("use stored credential references or a directly submitted value; external references are migration-only")
			}
			if err := r.Validate(); err != nil {
				return err
			}
			id := state.NewSecretID()
			if err := db.Put(id, state.Owner, service, purpose, []byte(r.Value)); err != nil {
				return err
			}
			*r = SecretRef{Credential: id}
		}
		if _, err := db.Get(r.Credential, state.Owner); err != nil {
			return errors.New("credential reference is unavailable or not owned by this administrator")
		}
		r.Bind(db)
		return nil
	})
}
