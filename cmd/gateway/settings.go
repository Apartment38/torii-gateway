package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
	"gopkg.in/yaml.v3"
)

type pendingSecrets struct {
	mu     sync.RWMutex
	db     *state.Database
	values map[string][]byte
}

func (p *pendingSecrets) Get(id, owner string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if b, ok := p.values[id]; ok && owner == state.Owner {
		return b, nil
	}
	return p.db.Get(id, owner)
}
func (p *pendingSecrets) committed() { p.mu.Lock(); defer p.mu.Unlock(); clear(p.values) }

// Prepare against an overlay: construction can resolve submitted secrets, but
// rejected configurations never write credentials or modify the active runtime.
func prepareSettings(cfg *config.Config, db *state.Database, importing bool) (*pendingSecrets, []state.SubmittedSecret, error) {
	overlay := &pendingSecrets{db: db, values: map[string][]byte{}}
	secrets := []state.SubmittedSecret{}
	cfg.Credentials = db
	err := cfg.VisitSecrets(func(_, service, purpose string, ref *config.SecretRef) error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Credential == "" {
			if !importing && (ref.Env != "" || ref.File != "") {
				return errors.New("submit a credential value or database reference")
			}
			value, err := ref.Resolve()
			if err != nil {
				return errors.New("cannot resolve imported credential")
			}
			id := state.NewSecretID()
			overlay.values[id] = []byte(value)
			secrets = append(secrets, state.SubmittedSecret{ID: id, Service: service, Purpose: purpose, Value: []byte(value)})
			*ref = config.SecretRef{Credential: id}
		}
		if state.IsGatewayAPIKeyID(ref.Credential) || len(ref.Credential) >= 2 && ref.Credential[:2] == "__" {
			return errors.New("system credentials cannot be used as provider credentials")
		}
		if _, err := overlay.Get(ref.Credential, state.Owner); err != nil {
			return errors.New("credential reference unavailable")
		}
		ref.Bind(overlay)
		return nil
	})
	return overlay, secrets, err
}
func loadDatabaseSettings(db *state.Database) (*config.Config, error) {
	data, _, err := db.Settings()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		cfg, err := config.Decode(stringsReader(bootstrapConfig))
		if err != nil {
			return nil, err
		}
		data, err = yaml.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		if _, err = db.CommitSettings(data, "", nil); err != nil {
			return nil, err
		}
	}
	cfg, err := config.Decode(stringsReader(string(data)))
	if err != nil {
		return nil, err
	}
	err = cfg.BindSecrets(db)
	return cfg, err
}
func stringsReader(s string) io.Reader { return strings.NewReader(s) }
func openSettingsDatabase(path string) (*state.Database, error) {
	if path == "" {
		path = os.Getenv("TORII_DB_PATH")
	}
	if path == "" {
		path = "/data/torii.db"
	}
	key, err := state.MasterKey()
	if err != nil {
		return nil, err
	}
	db, err := state.OpenDatabase(path, key)
	if err != nil {
		return nil, err
	}
	if err = db.InitializeCodexHostID(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func runConfigImport(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("config import", flag.ContinueOnError)
	path := flags.String("config", "", "legacy YAML configuration")
	database := flags.String("database", "", "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return errors.New("usage: gateway config import --config PATH --database PATH")
	}
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, err := config.Decode(f)
	if err != nil {
		return err
	}
	if *database == "" && os.Getenv("TORII_DB_PATH") == "" {
		*database = cfg.State.DatabasePath
	}
	db, err := openSettingsDatabase(*database)
	if err != nil {
		return err
	}
	defer db.Close()
	if data, _, err := db.Settings(); err != nil {
		return err
	} else if len(data) != 0 {
		return errors.New("database configuration already established; import refused")
	}
	if cfg.Router.CatalogFile != "" {
		cfg.Router.CatalogOverrides, err = loadCatalogOverrides(cfg.Router.CatalogFile)
		if err != nil {
			return err
		}
		cfg.Router.CatalogFile = ""
	}
	overlay, secrets, err := prepareSettings(cfg, db, true)
	if err != nil {
		return err
	}
	handler, err := buildHandlerWithRuntime(cfg, "", nil)
	if err != nil {
		return err
	}
	defer handler.(*catalogHandler).Close()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if _, err = db.CommitSettings(data, "", secrets); err != nil {
		return err
	}
	overlay.committed()
	_, err = fmt.Fprintln(output, "configuration imported into database")
	return err
}
