package provider

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const CodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

const codexDynamicClientID = "dynamic_agent_client"

// PKCEAuthorization contains the browser URL and verifier state for a web
// OAuth flow. The verifier must remain private until the callback exchange.
type PKCEAuthorization struct {
	AuthorizationURL string
	State            string
	Verifier         string
	RedirectURI      string
	ClientID         string
	HostID           string
	Nonce            string
}

// CodexIdentity contains the validated identity claims from an OpenAI ID token.
type CodexIdentity struct {
	Issuer  string
	Subject string
	Email   string
}

// StartPKCE creates an authorization URL for a caller-managed callback.
func StartPKCE(issuer, redirectURI string) (PKCEAuthorization, error) {
	return StartPKCEForHost(issuer, redirectURI, codexDynamicClientID, "urn:uuid:torii-gateway", "")
}

// StartPKCEForHost creates an OpenAI Sign in with ChatGPT authorization URL.
// clientID is dynamic_agent_client for first registration and the issued
// client ID for subsequent authorization. hostID must remain stable for the
// lifetime of the remote Torii installation.
func StartPKCEForHost(issuer, redirectURI, clientID, hostID, idTokenHint string) (PKCEAuthorization, error) {
	issuer = strings.TrimRight(issuer, "/")
	if issuer == "" || redirectURI == "" {
		return PKCEAuthorization{}, errors.New("Codex OAuth issuer and redirect URI are required")
	}
	if clientID == "" {
		clientID = codexDynamicClientID
	}
	if hostID == "" {
		return PKCEAuthorization{}, errors.New("Codex OAuth host ID is required")
	}
	verifier, err := randomURLValue(32)
	if err != nil {
		return PKCEAuthorization{}, err
	}
	state, err := randomURLValue(24)
	if err != nil {
		return PKCEAuthorization{}, err
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	nonce, err := randomURLValue(24)
	if err != nil {
		return PKCEAuthorization{}, err
	}
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {codexDynamicClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"},
		"resource":              {"https://api.openai.com/v1"},
		"state":                 {state},
		"nonce":                 {nonce},
		"agent_name_hint":       {"Torii Gateway"},
		"ext_agent_host_id":     {"torii-gateway"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	query.Set("client_id", clientID)
	query.Set("ext_agent_host_id", hostID)
	if clientID != codexDynamicClientID {
		query.Del("agent_name_hint")
	}
	if idTokenHint != "" {
		query.Set("id_token_hint", idTokenHint)
	}
	return PKCEAuthorization{AuthorizationURL: issuer + "/api/accounts/authorize?" + query.Encode(), State: state, Verifier: verifier, RedirectURI: redirectURI, ClientID: clientID, HostID: hostID, Nonce: nonce}, nil
}

// ExchangePKCE exchanges a callback code for Codex credentials.
func ExchangePKCE(ctx context.Context, issuer, code string, authorization PKCEAuthorization, client *http.Client) (CodexCredential, error) {
	if client == nil {
		client = http.DefaultClient
	}
	issuer = strings.TrimRight(issuer, "/")
	clientID := authorization.ClientID
	if clientID == "" {
		clientID = codexDynamicClientID
	}
	requestBody := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {authorization.RedirectURI},
		"code_verifier": {authorization.Verifier},
		"resource":      {"https://api.openai.com/v1"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer+"/api/accounts/oauth/token", strings.NewReader(requestBody.Encode()))
	if err != nil {
		return CodexCredential{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
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
		ClientID     string `json:"client_id"`
		IDToken      string `json:"id_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return CodexCredential{}, errors.New("Codex authorization token response is invalid")
	}
	expires := time.Time{}
	if token.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	if token.ClientID == "" {
		token.ClientID = clientID
	}
	return CodexCredential{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresAt: expires, Endpoint: issuer, AccountID: token.AccountID, ClientID: token.ClientID, IDToken: token.IDToken, Issuer: issuer, HostID: authorization.HostID, Scopes: strings.Fields(token.Scope)}, nil
}

// ValidateCodexIDToken validates the OpenID Connect token returned by OpenAI.
// The discovery and JWKS documents are fetched for each login; the helper is a
// short-lived process, so a persistent key cache is unnecessary here.
func ValidateCodexIDToken(ctx context.Context, token, issuer, clientID, nonce string, client *http.Client) (CodexIdentity, error) {
	if client == nil {
		client = http.DefaultClient
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return CodexIdentity{}, errors.New("Codex ID token is not a JWT")
	}
	decode := func(value string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(value) }
	headerBytes, err := decode(parts[0])
	if err != nil {
		return CodexIdentity{}, errors.New("Codex ID token header is invalid")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return CodexIdentity{}, errors.New("Codex ID token signing header is invalid")
	}
	payloadBytes, err := decode(parts[1])
	if err != nil {
		return CodexIdentity{}, errors.New("Codex ID token claims are invalid")
	}
	var claims struct {
		Issuer    string          `json:"iss"`
		Subject   string          `json:"sub"`
		Email     string          `json:"email"`
		Audience  json.RawMessage `json:"aud"`
		ExpiresAt int64           `json:"exp"`
		Nonce     string          `json:"nonce"`
	}
	if json.Unmarshal(payloadBytes, &claims) != nil || claims.Issuer == "" || claims.Subject == "" || claims.ExpiresAt <= time.Now().Unix() || claims.Nonce != nonce {
		return CodexIdentity{}, errors.New("Codex ID token claims failed validation")
	}
	if strings.TrimRight(claims.Issuer, "/") != strings.TrimRight(issuer, "/") {
		return CodexIdentity{}, errors.New("Codex ID token issuer is invalid")
	}
	if !audienceContains(claims.Audience, clientID) {
		return CodexIdentity{}, errors.New("Codex ID token audience is invalid")
	}

	discovery, err := fetchOpenAIDiscovery(ctx, issuer, client)
	if err != nil {
		return CodexIdentity{}, err
	}
	key, err := fetchOpenAIKey(ctx, discovery.JWKSURI, header.KeyID, client)
	if err != nil {
		return CodexIdentity{}, err
	}
	signature, err := decode(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err != nil || rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return CodexIdentity{}, errors.New("Codex ID token signature is invalid")
	}
	return CodexIdentity{Issuer: claims.Issuer, Subject: claims.Subject, Email: claims.Email}, nil
}

func audienceContains(raw json.RawMessage, wanted string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == wanted
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, value := range multiple {
		if value == wanted {
			return true
		}
	}
	return false
}

type openAIDiscovery struct {
	JWKSURI string `json:"jwks_uri"`
}

func fetchOpenAIDiscovery(ctx context.Context, issuer string, client *http.Client) (openAIDiscovery, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return openAIDiscovery{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return openAIDiscovery{}, fmt.Errorf("fetch OpenAI discovery: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return openAIDiscovery{}, fmt.Errorf("fetch OpenAI discovery returned %s", response.Status)
	}
	var discovery openAIDiscovery
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&discovery); err != nil || discovery.JWKSURI == "" {
		return openAIDiscovery{}, errors.New("OpenAI discovery document is invalid")
	}
	return discovery, nil
}

func fetchOpenAIKey(ctx context.Context, jwksURI, keyID string, client *http.Client) (*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch OpenAI JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch OpenAI JWKS returned %s", response.Status)
	}
	var document struct {
		Keys []struct {
			KeyID string `json:"kid"`
			N     string `json:"n"`
			E     string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&document); err != nil {
		return nil, errors.New("OpenAI JWKS document is invalid")
	}
	for _, candidate := range document.Keys {
		if candidate.KeyID != keyID {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(candidate.N)
		if err != nil {
			return nil, errors.New("OpenAI JWKS modulus is invalid")
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(candidate.E)
		if err != nil || len(exponentBytes) > 4 {
			return nil, errors.New("OpenAI JWKS exponent is invalid")
		}
		var exponent uint32
		for _, value := range exponentBytes {
			exponent = exponent<<8 | uint32(value)
		}
		if exponent == 0 {
			return nil, errors.New("OpenAI JWKS exponent is invalid")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(exponent)}, nil
	}
	return nil, fmt.Errorf("OpenAI JWKS key %q was not found", keyID)
}

// CodexOAuthPort extracts the externally reachable port from the request host.
// OpenAI's Codex sign-in flow requires a loopback redirect, so the browser must
// return to the same published gateway port while using 127.0.0.1 as the host.
func CodexOAuthPort(host string) string {
	if _, port, err := net.SplitHostPort(host); err == nil && port != "" {
		if n, parseErr := strconv.Atoi(port); parseErr == nil && n > 0 && n <= 65535 {
			return port
		}
	}
	return "8080"
}

// LoadOrCreateCodexHostID returns the stable host identifier used by OpenAI's
// dynamic registration flow. It is stored separately from bearer credentials
// so a logout does not accidentally create a new agent host.
func LoadOrCreateCodexHostID(stateDirectory string) (string, error) {
	if stateDirectory == "" {
		return "", errors.New("Codex state directory is required")
	}
	authDirectory := filepath.Join(stateDirectory, "auth")
	if err := os.MkdirAll(authDirectory, 0o700); err != nil {
		return "", fmt.Errorf("create Codex auth directory: %w", err)
	}
	path := filepath.Join(authDirectory, "codex-host-id")
	if data, err := os.ReadFile(path); err == nil {
		if hostID := strings.TrimSpace(string(data)); hostID != "" {
			return hostID, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read Codex host ID: %w", err)
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Codex host ID: %w", err)
	}
	hostID := fmt.Sprintf("urn:uuid:%08x-%04x-%04x-%04x-%012x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
	if err := os.WriteFile(path, []byte(hostID+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write Codex host ID: %w", err)
	}
	return hostID, nil
}

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
