// Command torii-auth performs one-shot local OAuth for a remote Torii Gateway.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/buildinfo"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

type loginSession struct {
	ProviderID  string `json:"provider_id"`
	Issuer      string `json:"issuer"`
	ClientID    string `json:"client_id"`
	HostID      string `json:"host_id"`
	IDTokenHint string `json:"id_token_hint"`
}

type importedCredential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	AccountID    string    `json:"account_id,omitempty"`
	ClientID     string    `json:"client_id"`
	IDToken      string    `json:"id_token"`
	Issuer       string    `json:"issuer"`
	Subject      string    `json:"subject,omitempty"`
	Email        string    `json:"email,omitempty"`
	HostID       string    `json:"host_id"`
	Scopes       []string  `json:"scopes"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "torii-auth: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 1 && args[0] == "version" {
		fmt.Printf("Torii Auth %s\nCommit: %s\nBuilt: %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)
		return nil
	}
	flags := flag.NewFlagSet("torii-auth", flag.ContinueOnError)
	sessionURL := flags.String("session-url", "", "remote Torii login session URL")
	pairingToken := flags.String("pairing-token", "", "one-time remote Torii pairing token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *sessionURL == "" || *pairingToken == "" || flags.NArg() != 0 {
		return errors.New("usage: torii-auth --session-url <url> --pairing-token <token>")
	}
	return login(ctx, *sessionURL, *pairingToken, http.DefaultClient)
}

func login(ctx context.Context, sessionURL, pairingToken string, client *http.Client) error {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionURL, nil)
	if err != nil {
		return fmt.Errorf("create session request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+pairingToken)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("connect to Torii login session: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Torii login session returned %s", response.Status)
	}
	var session loginSession
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&session); err != nil {
		return fmt.Errorf("decode Torii login session: %w", err)
	}
	if session.ProviderID == "" || session.Issuer == "" || session.ClientID == "" || session.HostID == "" {
		return errors.New("Torii login session is incomplete")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start local OAuth callback: %w", err)
	}
	defer listener.Close()
	redirectURI := "http://" + listener.Addr().String() + "/auth/callback"
	authorization, err := provider.StartPKCEForHost(session.Issuer, redirectURI, session.ClientID, session.HostID, session.IDTokenHint)
	if err != nil {
		return fmt.Errorf("start Codex OAuth: %w", err)
	}

	codeChannel := make(chan string, 1)
	errorChannel := make(chan error, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, callback *http.Request) {
		if callback.URL.Path != "/auth/callback" {
			http.NotFound(w, callback)
			return
		}
		if callback.URL.Query().Get("state") != authorization.State {
			errorChannel <- errors.New("OAuth state validation failed")
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		if oauthError := callback.URL.Query().Get("error"); oauthError != "" {
			errorChannel <- fmt.Errorf("Codex authorization failed: %s", oauthError)
			_, _ = io.WriteString(w, "Authorization failed. You may close this window.")
			return
		}
		code := callback.URL.Query().Get("code")
		if code == "" {
			errorChannel <- errors.New("OAuth callback did not include a code")
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		if issuedClientID := callback.URL.Query().Get("client_id"); issuedClientID != "" {
			authorization.ClientID = issuedClientID
		}
		codeChannel <- code
		_, _ = io.WriteString(w, "Torii authorization complete. You may close this window.")
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	fmt.Printf("Opening Codex authorization in your browser...\n")
	if err := openBrowser(authorization.AuthorizationURL); err != nil {
		fmt.Printf("Open this URL in your browser:\n%s\n", authorization.AuthorizationURL)
	}
	var code string
	select {
	case code = <-codeChannel:
	case err := <-errorChannel:
		return err
	case <-time.After(15 * time.Minute):
		return errors.New("Codex authorization timed out")
	case <-ctx.Done():
		return ctx.Err()
	}
	credential, err := provider.ExchangePKCE(ctx, session.Issuer, code, authorization, client)
	if err != nil {
		return fmt.Errorf("exchange Codex authorization: %w", err)
	}
	identity, err := provider.ValidateCodexIDToken(ctx, credential.IDToken, session.Issuer, credential.ClientID, authorization.Nonce, client)
	if err != nil {
		return fmt.Errorf("validate Codex identity: %w", err)
	}
	credential.HostID = session.HostID
	credential.Subject = identity.Subject
	credential.Email = identity.Email
	credential.Issuer = identity.Issuer
	if credential.ClientID == "" {
		credential.ClientID = authorization.ClientID
	}
	if credential.Issuer == "" {
		credential.Issuer = session.Issuer
	}
	if credential.IDToken == "" {
		return errors.New("Codex token response did not include an ID token")
	}
	if !containsScope(credential.Scopes, "chatgpt.tokens.use.direct") {
		return errors.New("Codex authorization did not grant ChatGPT plan usage")
	}

	importURL, err := importEndpoint(sessionURL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(importedCredential{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, TokenType: "Bearer", ExpiresAt: credential.ExpiresAt, AccountID: credential.AccountID, ClientID: credential.ClientID, IDToken: credential.IDToken, Issuer: credential.Issuer, Subject: credential.Subject, Email: credential.Email, HostID: credential.HostID, Scopes: credential.Scopes})
	if err != nil {
		return fmt.Errorf("encode imported credential: %w", err)
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, importURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create credential import request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+pairingToken)
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		return fmt.Errorf("import credential into Torii: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("Torii credential import returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	fmt.Printf("Codex login imported into Torii successfully. This helper can now exit.\n")
	return nil
}

func importEndpoint(sessionURL string) (string, error) {
	parsed, err := url.Parse(sessionURL)
	if err != nil {
		return "", fmt.Errorf("parse session URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || !strings.HasSuffix(parsed.Path, "/session") {
		return "", errors.New("session URL must end in /gateway/v1/providers/login/session")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/session") + "/import"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func containsScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func openBrowser(target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		command, args = "xdg-open", []string{target}
	}
	return exec.Command(command, args...).Start()
}
