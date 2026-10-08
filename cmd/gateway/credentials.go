package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/term"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func openCredentials(cfg *config.Config, path string) (*state.Database, error) {
	key, err := state.MasterKey()
	if err != nil {
		return nil, err
	}
	dbPath := os.Getenv("TORII_DB_PATH")
	if dbPath == "" {
		dbPath = cfg.State.DatabasePath
	}
	if dbPath == "" {
		dbPath = "/data/torii.db"
	}
	db, err := state.OpenDatabase(dbPath, key)
	if err != nil {
		return nil, err
	}
	if err := cfg.BindSecrets(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.InitializeCodexHostID(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func (r *handlerRuntime) loadConfig() (*config.Config, error) {
	return loadDatabaseSettings(r.credentials)
}
func (r *handlerRuntime) oauthStore() (state.CredentialStore, error) {
	if r.credentials != nil {
		return r.credentials, nil
	}
	return nil, errors.New("OAuth credentials require the application database")
}

// managementBoundary never accepts NoAuth for administrative operations.
func managementBoundary(auth api.ClientAuthenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := http.NewRequest(http.MethodGet, origin, nil)
			if err != nil || u.URL.Host != r.Host {
				http.Error(w, "cross-origin management request denied", http.StatusForbidden)
				return
			}
		}
		api.AuthenticateMiddleware(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := api.ClientIdentityFromContext(r.Context())
			if id == nil || id.ID == "anonymous" {
				http.Error(w, "administrator authentication required", http.StatusForbidden)
				return
			}
			if id.ID != state.Owner {
				http.Error(w, "administrator authorization required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := http.NewRequest(http.MethodGet, origin, nil)
	return err == nil && u.URL.Host == r.Host
}
func (r *handlerRuntime) credentialAPI(w http.ResponseWriter, req *http.Request) {
	if r.credentials == nil {
		writeConfigError(w, 503, errors.New("credential database unavailable"))
		return
	}
	id := strings.TrimPrefix(req.URL.Path, "/gateway/v1/credentials/")
	if req.URL.Path == "/gateway/v1/credentials" {
		id = ""
	}
	if strings.HasPrefix(id, "__") || (state.IsGatewayAPIKeyID(id) && req.Method != http.MethodGet) {
		writeConfigError(w, 400, errors.New("protected system credential; use admin API key generation"))
		return
	}
	switch req.Method {
	case http.MethodGet:
		list, err := r.credentials.List(state.Owner)
		if err != nil {
			writeConfigError(w, 500, errors.New("cannot list credentials"))
			return
		}
		writeJSON(w, 200, map[string]any{"credentials": list})
	case http.MethodPost, http.MethodPut:
		var input struct {
			Value   string `json:"value"`
			Service string `json:"service"`
			Purpose string `json:"purpose"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.Value == "" || input.Service == "" || input.Purpose == "" {
			writeConfigError(w, 400, errors.New("provide value, service, and purpose"))
			return
		}
		if id == "" {
			id = state.NewSecretID()
		} else {
			list, err := r.credentials.List(state.Owner)
			if err != nil {
				writeConfigError(w, 500, errors.New("cannot inspect credential"))
				return
			}
			found := false
			for _, m := range list {
				if m.ID == id {
					found = true
					input.Service = m.Service
					input.Purpose = m.Purpose
				}
			}
			if !found {
				writeConfigError(w, 404, errors.New("credential not found"))
				return
			}
		}
		if id == state.GatewayAPIKeyID {
			writeConfigError(w, 400, errors.New("use Generate new API key on the Server page"))
			return
		}
		if input.Purpose == "oauth" {
			writeConfigError(w, 400, errors.New("use provider login to replace OAuth credentials"))
			return
		}
		if input.Service == "gateway" && input.Purpose == "authentication" {
			if _, err := api.NewStaticBearerAuthenticator(input.Value); err != nil {
				writeConfigError(w, 400, err)
				return
			}
		}
		if err := r.credentials.Put(id, state.Owner, input.Service, input.Purpose, []byte(input.Value)); err != nil {
			writeConfigError(w, 400, errors.New("cannot store credential"))
			return
		}
		writeJSON(w, 200, map[string]string{"credential": id, "status": "configured"})
	case http.MethodDelete:
		if id == "" {
			writeConfigError(w, 400, errors.New("credential ID required"))
			return
		}
		if err := r.credentials.DeleteSecret(id, state.Owner); err != nil {
			writeConfigError(w, 404, errors.New("credential not found"))
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
func runCredentials(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: gateway credentials <replace|delete> --database PATH --id ID [--secret-file PATH]; administrator authentication required")
	}
	f := flag.NewFlagSet("credentials", flag.ContinueOnError)
	path := f.String("database", "", "SQLite database path")
	id := f.String("id", "", "credential ID")
	secretFile := f.String("secret-file", "", "read replacement from a protected file instead of prompting")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	db, err := openSettingsDatabase(*path)
	if err != nil {
		return err
	}
	cfg, err := loadDatabaseSettings(db)
	if err != nil {
		db.Close()
		return err
	}
	_ = cfg
	defer db.Close()
	if strings.HasPrefix(*id, "__") || state.IsGatewayAPIKeyID(*id) {
		return errors.New("protected system credential; use admin api-key")
	}
	if args[0] == "bootstrap" {
		return errors.New("use gateway admin create to choose an administrator and generate an API key")
	}
	if err := authenticateLocalAdmin(db, output); err != nil {
		return err
	}
	switch args[0] {
	case "bootstrap":
		return errors.New("use gateway admin create to choose an administrator and generate an API key")
	case "replace":
		m, err := db.Metadata(*id, state.Owner)
		if err != nil {
			return err
		}
		var b []byte
		if *secretFile != "" {
			b, err = os.ReadFile(*secretFile)
		} else if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(output, "Replacement credential (hidden): ")
			b, err = term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(output)
		} else {
			return errors.New("supply --secret-file; stdin is reserved for the administrator password")
		}
		if err != nil || len(b) > 1<<20 {
			return errors.New("cannot read replacement from stdin")
		}
		value := strings.TrimSpace(string(b))
		if m.Purpose == "oauth" {
			return errors.New("use auth login to replace OAuth credentials")
		}
		if m.Service == "gateway" && m.Purpose == "authentication" {
			if _, err := api.NewStaticBearerAuthenticator(value); err != nil {
				return err
			}
		}
		return db.Put(m.ID, state.Owner, m.Service, m.Purpose, []byte(value))
	case "delete":
		return db.DeleteSecret(*id, state.Owner)
	default:
		return errors.New("unknown credential command")
	}
}
