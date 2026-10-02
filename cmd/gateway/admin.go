package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
	"golang.org/x/term"
)

// Existing benchmark CLI callers may use the inference key. Configuration and
// credential routes are registered separately and accept administrator Basic only.
type benchmarkAuthenticator struct{ basic, bearer api.ClientAuthenticator }

func (a benchmarkAuthenticator) Authenticate(r *http.Request) (*api.ClientIdentity, error) {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
		return a.basic.Authenticate(r)
	}
	return a.bearer.Authenticate(r)
}

func (r *handlerRuntime) initializeAdminPrompt(output io.Writer) error {
	required, err := r.credentials.AdminRequired()
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		u, p, err := promptAdminCredentials("", true, output)
		if err != nil {
			return err
		}
		key, err := r.credentials.CreateAdmin(u, p)
		if err != nil {
			return err
		}
		displayNewAPIKey(output, key)
		return nil
	}
	code := state.NewSecretID() + state.NewSecretID()
	r.setupDigest = sha256.Sum256([]byte(code))
	r.setupExpires = time.Now().Add(30 * time.Minute)
	_, err = fmt.Fprintf(output, "Admin setup required. Open the web console and enter this one-time setup code (expires in 30 minutes): %s\nAlternatively run /gateway admin create --config %s in an interactive terminal.\n", code, r.configPath)
	return err
}
func (r *handlerRuntime) setupStatus(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	required, err := r.credentials.AdminRequired()
	if err != nil {
		writeConfigError(w, 503, errors.New("administrator state unavailable"))
		return
	}
	writeJSON(w, 200, map[string]any{"setup_required": required})
}
func (r *handlerRuntime) setupAdmin(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !sameOrigin(req) {
		http.Error(w, "cross-origin setup denied", 403)
		return
	}
	required, err := r.credentials.AdminRequired()
	if err != nil || !required {
		writeConfigError(w, 409, errors.New("administrator already configured"))
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"setup_code"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeConfigError(w, 400, errors.New("invalid setup request"))
		return
	}
	digest := sha256.Sum256([]byte(input.Code))
	if !time.Now().Before(r.setupExpires) || subtle.ConstantTimeCompare(digest[:], r.setupDigest[:]) != 1 {
		writeConfigError(w, 403, errors.New("invalid or expired setup code; inspect terminal output or use admin create locally"))
		return
	}
	key, err := r.credentials.CreateAdmin(input.Username, input.Password)
	if err != nil {
		writeConfigError(w, 400, err)
		return
	}
	writeJSON(w, 201, map[string]any{"username": input.Username, "api_key": key, "api_key_generated": key != "", "warning": "Save a newly generated API key now. It is displayed only once; existing imported keys are never revealed."})
}
func (r *handlerRuntime) rotateAPIKey(w http.ResponseWriter, req *http.Request) {
	key, err := r.credentials.RotateAPIKey()
	if err != nil {
		writeConfigError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]string{"api_key": key, "warning": "Save this API key now. It is displayed only once; the previous key no longer works."})
}

// Named client keys are distinct from provider secrets. Management is protected
// by the same administrator Basic boundary as configuration and rotation.
func (r *handlerRuntime) gatewayKeysAPI(w http.ResponseWriter, req *http.Request) {
	if r.credentials == nil {
		writeConfigError(w, 503, errors.New("credential database unavailable"))
		return
	}
	id := strings.TrimPrefix(strings.TrimPrefix(req.URL.Path, "/gateway/v1/admin/api-keys"), "/")
	switch req.Method {
	case http.MethodGet:
		if id != "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		keys, err := r.credentials.ListGatewayAPIKeys()
		if err != nil {
			writeConfigError(w, 500, errors.New("cannot list API keys"))
			return
		}
		writeJSON(w, 200, map[string]any{"api_keys": keys})
	case http.MethodPost:
		if id != "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
		decoder.DisallowUnknownFields()
		var extra any
		if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
			writeConfigError(w, 400, errors.New("provide an API key name"))
			return
		}
		metadata, key, err := r.credentials.CreateGatewayAPIKey(input.Name)
		if err != nil {
			writeConfigError(w, 400, err)
			return
		}
		writeJSON(w, 201, map[string]any{"key": metadata, "api_key": key, "warning": "This API key is displayed only once. Save it securely before closing this dialog."})
	case http.MethodDelete:
		if id == "" {
			writeConfigError(w, 400, errors.New("API key ID required"))
			return
		}
		if err := r.credentials.DeleteGatewayAPIKey(id); err != nil {
			writeConfigError(w, 404, errors.New("API key not found"))
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
func promptAdminCredentials(username string, confirm bool, output io.Writer) (string, string, error) {
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	if username == "" {
		if !interactive {
			return "", "", errors.New("supply --username when reading a password from stdin")
		}
		fmt.Fprint(output, "Choose admin username: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return "", "", err
		}
		username = strings.TrimSpace(line)
	}
	if !interactive {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1026))
		if err != nil || len(b) > 1025 {
			return "", "", errors.New("cannot read password from stdin")
		}
		return username, strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r"), nil
	}
	fmt.Fprint(output, "Admin password (hidden): ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(output)
	if err != nil {
		return "", "", errors.New("cannot read password from terminal")
	}
	if confirm {
		fmt.Fprint(output, "Confirm password (hidden): ")
		again, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(output)
		if err != nil || string(again) != string(b) {
			return "", "", errors.New("passwords do not match")
		}
	}
	return username, string(b), nil
}
func displayNewAPIKey(output io.Writer, key string) {
	if key == "" {
		fmt.Fprintln(output, "Administrator created. Your existing imported API key was preserved and is not displayed. Use admin api-key to generate a replacement.")
		return
	}
	fmt.Fprintln(output, "WARNING: This API key is displayed only once. Save it securely.\n"+key)
}
func authenticateLocalAdmin(db *state.Database, output io.Writer) error {
	a, err := db.Admin()
	if err != nil {
		return errors.New("create an administrator first using admin create")
	}
	u, p, err := promptAdminCredentials(a.Username, false, output)
	if err != nil {
		return err
	}
	if !db.VerifyAdmin(u, p) {
		return errors.New("administrator authentication failed")
	}
	return nil
}
func runAdmin(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: gateway admin <create|api-key> --config PATH [--username NAME]; passwords are prompted securely or read from stdin")
	}
	f := flag.NewFlagSet("admin", flag.ContinueOnError)
	path := f.String("config", "/config/config.yaml", "configuration path")
	username := f.String("username", "", "administrator username")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	db, err := openCredentials(cfg, *path)
	if err != nil {
		return err
	}
	defer db.Close()
	if args[0] == "create" {
		required, err := db.AdminRequired()
		if err != nil {
			return err
		}
		if !required {
			return errors.New("administrator already configured")
		}
		u, p, err := promptAdminCredentials(*username, true, output)
		if err != nil {
			return err
		}
		key, err := db.CreateAdmin(u, p)
		if err != nil {
			return err
		}
		displayNewAPIKey(output, key)
		return nil
	}
	if args[0] == "api-key" {
		a, err := db.Admin()
		if err != nil {
			return errors.New("create an administrator first using admin create")
		}
		u := *username
		if u == "" {
			u = a.Username
		}
		u, p, err := promptAdminCredentials(u, false, output)
		if err != nil {
			return err
		}
		if !db.VerifyAdmin(u, p) {
			return errors.New("administrator authentication failed")
		}
		key, err := db.RotateAPIKey()
		if err != nil {
			return err
		}
		displayNewAPIKey(output, key)
		return nil
	}
	return errors.New("unknown admin command")
}
