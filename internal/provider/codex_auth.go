package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const CodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

// PKCELogin starts a browser authorization-code flow and waits for the
// localhost callback. It prints/returns only the authorization URL; tokens are
// returned to the caller for secure persistence.
func PKCELogin(ctx context.Context, issuer string, client *http.Client, notify func(url string)) (CodexCredential, error) {
	if client == nil {
		client = http.DefaultClient
	}
	issuer = strings.TrimRight(issuer, "/")
	verifier, err := randomURLValue(32)
	if err != nil {
		return CodexCredential{}, err
	}
	state, err := randomURLValue(24)
	if err != nil {
		return CodexCredential{}, err
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return CodexCredential{}, fmt.Errorf("start PKCE callback: %w", err)
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	query := url.Values{"response_type": {"code"}, "client_id": {CodexOAuthClientID}, "redirect_uri": {redirect}, "scope": {"openid profile email offline_access"}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	authorizationURL := issuer + "/oauth/authorize?" + query.Encode()
	if notify != nil {
		notify(authorizationURL)
	}
	callback := make(chan string, 1)
	callbackErr := make(chan error, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != state {
			callbackErr <- errors.New("PKCE state validation failed")
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			callbackErr <- fmt.Errorf("Codex authorization failed: %s", oauthErr)
			_, _ = io.WriteString(w, "Authorization failed")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			callbackErr <- errors.New("PKCE callback did not contain a code")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		callback <- code
		_, _ = io.WriteString(w, "Authorization complete. You may close this window.")
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	var code string
	select {
	case code = <-callback:
	case err := <-callbackErr:
		return CodexCredential{}, err
	case <-ctx.Done():
		return CodexCredential{}, ctx.Err()
	}
	requestBody, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "client_id": CodexOAuthClientID, "code": code, "redirect_uri": redirect, "code_verifier": verifier})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer+"/oauth/token", bytes.NewReader(requestBody))
	if err != nil {
		return CodexCredential{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return CodexCredential{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return CodexCredential{}, fmt.Errorf("Codex authorization token exchange returned %s", response.Status)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		AccountID    string `json:"account_id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return CodexCredential{}, errors.New("Codex authorization token response is invalid")
	}
	expires := time.Time{}
	if token.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	return CodexCredential{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresAt: expires, Endpoint: issuer, AccountID: token.AccountID}, nil
}

func randomURLValue(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// DeviceCodeLogin performs the non-interactive portion of the Codex device
// flow. The callback receives display-safe verification instructions; tokens
// are returned only to the caller for secure persistence.
func DeviceCodeLogin(ctx context.Context, issuer string, client *http.Client, notify func(url, code string)) (CodexCredential, error) {
	if client == nil {
		client = http.DefaultClient
	}
	issuer = strings.TrimRight(issuer, "/")
	body, _ := json.Marshal(map[string]string{"client_id": CodexOAuthClientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer+"/api/accounts/deviceauth/usercode", bytes.NewReader(body))
	if err != nil {
		return CodexCredential{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return CodexCredential{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return CodexCredential{}, fmt.Errorf("Codex device-code request returned %s", response.Status)
	}
	var code struct {
		DeviceAuthID    string `json:"device_auth_id"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(response.Body).Decode(&code); err != nil || code.DeviceAuthID == "" || code.UserCode == "" || code.VerificationURL == "" {
		return CodexCredential{}, errors.New("Codex device-code response is invalid")
	}
	if notify != nil {
		notify(code.VerificationURL, code.UserCode)
	}
	interval := time.Duration(code.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return CodexCredential{}, ctx.Err()
		case <-deadline.C:
			return CodexCredential{}, errors.New("Codex device authorization expired")
		case <-time.After(interval):
		}
		pollBody, _ := json.Marshal(map[string]string{"device_auth_id": code.DeviceAuthID, "user_code": code.UserCode, "client_id": CodexOAuthClientID})
		poll, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer+"/api/accounts/deviceauth/token", bytes.NewReader(pollBody))
		if err != nil {
			return CodexCredential{}, err
		}
		poll.Header.Set("Content-Type", "application/json")
		result, err := client.Do(poll)
		if err != nil {
			return CodexCredential{}, err
		}
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			status := result.Status
			_ = result.Body.Close()
			return CodexCredential{}, fmt.Errorf("Codex device token request returned %s", status)
		}
		var payload struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			AccountID    string `json:"account_id"`
			Error        string `json:"error"`
		}
		decodeErr := json.NewDecoder(result.Body).Decode(&payload)
		_ = result.Body.Close()
		if decodeErr != nil {
			return CodexCredential{}, errors.New("Codex device token response is invalid")
		}
		if payload.AccessToken != "" {
			expiry := time.Time{}
			if payload.ExpiresIn > 0 {
				expiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
			}
			return CodexCredential{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, ExpiresAt: expiry, Endpoint: issuer, AccountID: payload.AccountID}, nil
		}
		if payload.Error == "authorization_pending" || payload.Error == "slow_down" {
			if payload.Error == "slow_down" {
				interval += time.Second
			}
			continue
		}
		if payload.Error == "access_denied" {
			return CodexCredential{}, errors.New("Codex device authorization was denied")
		}
		return CodexCredential{}, fmt.Errorf("Codex device authorization failed: %s", payload.Error)
	}
}
