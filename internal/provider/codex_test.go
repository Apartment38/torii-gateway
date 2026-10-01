package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCodexTranslatesChatAndResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer access" {
			t.Errorf("request path/auth = %s/%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"input"`) || strings.Contains(string(body), `"messages"`) {
			t.Errorf("body was not translated: %s", body)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","content":[{"text":"hello"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`)
	}))
	defer server.Close()
	cred := func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "access", Endpoint: server.URL}, nil
	}
	codex, err := NewCodex("openai-codex", server.URL, []string{"gpt-test"}, cred, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := codex.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`), "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(data), `"chat.completion"`) || !strings.Contains(string(data), `"hello"`) {
		t.Fatalf("translated response = %s", data)
	}
}

func TestCodexUsesDefaultModelForBootstrapPlaceholder(t *testing.T) {
	codex, err := NewCodex("openai-codex", "https://chatgpt.com/backend-api/codex", []string{"setup"}, func(string) (CodexCredential, error) {
		return CodexCredential{}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	models, err := codex.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "openai-codex/gpt-5.3-codex" {
		t.Fatalf("models = %+v, want default Codex model", models)
	}
}

func TestCodexDiscoversAuthenticatedSubscriptionModels(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.URL.Query().Get("client_version") != codexClientVersion || r.Header.Get("Authorization") != "Bearer access" || r.Header.Get("ChatGPT-Account-ID") != "account" {
			t.Errorf("discovery request = %s auth=%q account=%q", r.URL.String(), r.Header.Get("Authorization"), r.Header.Get("ChatGPT-Account-ID"))
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"gpt-a","visibility":"list","supported_in_api":true},{"slug":"hidden","visibility":"hide","supported_in_api":true},{"slug":"unsupported","visibility":"list","supported_in_api":false}]}`)
	}))
	defer server.Close()
	codex, err := NewCodex("openai-codex", server.URL, []string{"fallback"}, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "access", AccountID: "account", Endpoint: server.URL}, nil
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := codex.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "openai-codex/gpt-a" || models[1].ID != "openai-codex/unsupported" {
		t.Fatalf("models = %+v, want all visible subscription models regardless of API-key support", models)
	}
}

type codexTestTransport func(*http.Request) (*http.Response, error)

func (f codexTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCodexDirectTokenUsesPublicSubscriptionAPI(t *testing.T) {
	credential := CodexCredential{AccessToken: "access", Endpoint: DefaultCodexBaseURL, Scopes: []string{"chatgpt.tokens.use.direct"}}
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/models" || r.Header.Get("Authorization") != "Bearer access" {
			t.Errorf("unexpected direct-token request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":" gpt-new ","visibility":"list","context_window":400000},{"slug":"gpt-new","visibility":"list"},{"slug":"subscription-only","visibility":"list","supported_in_api":false},{"slug":"unmarked"},{"slug":"private","visibility":"hidden"},{"slug":"internal","visibility":"hide"},{"slug":"bad model"}]}`))}, nil
	})}
	codex, err := NewCodex("openai-codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) { return credential, nil }, client)
	if err != nil {
		t.Fatal(err)
	}
	models, err := codex.DiscoverModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 || models[0].ID != "openai-codex/gpt-new" || models[1].ID != "openai-codex/subscription-only" || models[2].ID != "openai-codex/unmarked" {
		t.Fatalf("public subscription catalog = %+v", models)
	}
	if models[0].ContextWindow == nil || *models[0].ContextWindow != 400000 || !models[0].Subscription {
		t.Fatalf("metadata missing: %+v", models[0])
	}
}

func TestCodexLegacyCatalogVersionFallbackAndRoutingHints(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"jwt-account","chatgpt_compute_residency":"us"}}`))
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		version := codexClientVersion
		if call == 2 {
			version = "0.0.0"
		}
		if r.URL.Query().Get("client_version") != version || r.Header.Get("ChatGPT-Account-ID") != "jwt-account" || r.Header.Get("x-openai-internal-codex-residency") != "us" {
			t.Errorf("legacy routing headers or version missing")
		}
		if call == 1 {
			_, _ = io.WriteString(w, `{"models":[]}`)
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"gpt-legacy","visibility":"list"}]}`)
	}))
	defer server.Close()
	codex, err := NewCodex("openai-codex", server.URL, nil, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "header." + payload + ".signature", Endpoint: server.URL}, nil
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := codex.DiscoverModels(context.Background())
	if err != nil || calls.Load() != 2 || len(models) != 1 || models[0].ID != "openai-codex/gpt-legacy" {
		t.Fatalf("fallback = %+v, %v; calls = %d", models, err, calls.Load())
	}
}

func TestCodexRetainsLastGoodCatalogAndReportsLiveFailure(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "secret-upstream-error")
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"gpt-live","visibility":"list"}]}`)
	}))
	defer server.Close()
	codex, err := NewCodex("openai-codex", server.URL, []string{"fallback"}, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "access", Endpoint: server.URL}, nil
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if _, err := codex.DiscoverModels(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("strict error = %v", err)
	}
	models, err := codex.Models(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "openai-codex/gpt-live" {
		t.Fatalf("lost last-good catalog: %+v, %v", models, err)
	}
}

func TestCodexRefreshesBeforeCatalogDiscovery(t *testing.T) {
	for _, clientID := range []string{CodexOAuthClientID, "dynamic-client"} {
		t.Run(clientID, func(t *testing.T) {
			credential := CodexCredential{AccessToken: "expired", RefreshToken: "refresh", ClientID: clientID, ExpiresAt: time.Now().Add(-time.Minute)}
			var saved atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/models" {
					if r.Header.Get("Authorization") != "Bearer refreshed" || !saved.Load() {
						t.Errorf("discovered before persisting refreshed credential")
					}
					_, _ = io.WriteString(w, `{"models":[{"slug":"gpt-refreshed"}]}`)
					return
				}
				path := "/oauth/token"
				if clientID != CodexOAuthClientID {
					path = "/api/accounts/oauth/token"
				}
				if r.URL.Path != path {
					t.Errorf("refresh path = %s, want %s", r.URL.Path, path)
				}
				_ = r.ParseForm()
				if r.Form.Get("refresh_token") != "refresh" || r.Form.Get("client_id") != clientID {
					t.Errorf("invalid refresh request")
				}
				_, _ = io.WriteString(w, `{"access_token":"refreshed","refresh_token":"rotated","expires_in":3600}`)
			}))
			defer server.Close()
			credential.Endpoint = server.URL
			codex, err := NewCodex("openai-codex", server.URL, nil, func(string) (CodexCredential, error) { return credential, nil }, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			codex.ConfigureRefresh(server.URL, func(_ string, value CodexCredential) error { credential = value; saved.Store(true); return nil })
			models, err := codex.DiscoverModels(context.Background())
			if err != nil || len(models) != 1 || credential.RefreshToken != "rotated" {
				t.Fatalf("refresh/discovery = %+v, %v", models, err)
			}
		})
	}
}

func TestCodexDirectInferenceForcesUpstreamStream(t *testing.T) {
	for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(endpoint+"/"+map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
				client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if r.URL.String() != "https://api.openai.com/v1/responses" || body["stream"] != true || body["store"] != false || r.Header.Get("Accept") != "text/event-stream" {
						t.Errorf("incorrect plan-usage request: %s, %+v", r.URL, body)
					}
					data := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"message\",\"content\":[{\"text\":\"hello\"}]}]}}\n\n"
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(data))}, nil
				})}
				codex, err := NewCodex("openai-codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
					return CodexCredential{AccessToken: "access", Endpoint: DefaultCodexBaseURL, Scopes: []string{"chatgpt.tokens.use.direct"}}, nil
				}, client)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}, "input": "hi", "stream": stream})
				response, err := codex.Forward(context.Background(), endpoint, body, "gpt-live")
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				data, err := io.ReadAll(response.Body)
				if err != nil || !strings.Contains(string(data), "hello") {
					t.Fatalf("response = %s, %v", data, err)
				}
				if stream && endpoint == "/v1/responses" && !strings.Contains(string(data), "response.completed") {
					t.Fatalf("Responses stream was translated to chat: %s", data)
				}
				if !stream && (!json.Valid(data) || response.Header.Get("Content-Type") != "application/json") {
					t.Fatalf("invalid non-streaming response: %s", data)
				}
			})
		}
	}
}

func TestCompletedCodexResponseRejectsTruncatedAndFailedStreams(t *testing.T) {
	for _, input := range []string{"", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", "data: {\"type\":\"response.failed\"}\n\n", "data: {\"type\":\"response.completed\"}\n\n", "data: invalid\n\n"} {
		if _, err := completedCodexResponse(strings.NewReader(input)); err == nil {
			t.Errorf("accepted incomplete stream %q", input)
		}
	}
}

func TestCodexRejectsCredentialBoundToOtherOrigin(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	codex, err := NewCodex("openai-codex", server.URL, []string{"gpt"}, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "secret", Endpoint: "https://other.example"}, nil
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Forward(context.Background(), "/v1/responses", []byte(`{"model":"gpt","input":"hi"}`), "gpt"); err == nil {
		t.Fatal("accepted an endpoint-mismatched credential")
	}
	if _, err := codex.DiscoverModels(context.Background()); err == nil {
		t.Fatal("discovered models with an endpoint-mismatched credential")
	}
	if requests.Load() != 0 {
		t.Fatal("sent a credential to a mismatched endpoint")
	}
}

func TestCodexDirectScopePreservesCustomEndpointBinding(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected custom discovery path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"custom-model"}]}`)
	}))
	defer server.Close()
	codex, err := NewCodex("custom", server.URL, nil, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "access", Endpoint: server.URL, Scopes: []string{"chatgpt.tokens.use.direct"}}, nil
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := codex.DiscoverModels(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "custom/custom-model" {
		t.Fatalf("custom endpoint was overridden: %+v, %v", models, err)
	}
}

func TestCodexRejectsInvalidCatalogs(t *testing.T) {
	codex := &Codex{id: "openai-codex"}
	for _, body := range []string{`invalid`, `{}`, `{"data":[{"id":"wrong-api"}]}`, `{"models":[]}`, `{"models":[{"slug":"hidden","visibility":"hide"}]}`, `{"models":[{"slug":""}]}`, strings.Repeat("x", (8<<20)+1)} {
		response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
		if _, err := codex.decodeModels(response); err == nil {
			t.Error("invalid catalog accepted")
		}
		_ = response.Body.Close()
	}
}

func TestPKCELoginValidatesStateAndExchangesCode(t *testing.T) {
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"account_id":"acct"}`)
	}))
	defer issuer.Close()
	credential, err := PKCELogin(context.Background(), issuer.URL, issuer.Client(), func(raw string) {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			t.Error(parseErr)
			return
		}
		if parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("state") == "" {
			t.Error("PKCE parameters missing")
		}
		callback, _ := url.Parse(parsed.Query().Get("redirect_uri"))
		query := callback.Query()
		query.Set("code", "auth-code")
		query.Set("state", parsed.Query().Get("state"))
		callback.RawQuery = query.Encode()
		go func() { time.Sleep(20 * time.Millisecond); _, _ = http.Get(callback.String()) }()
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "access" || credential.RefreshToken != "refresh" || credential.AccountID != "acct" {
		t.Fatalf("credential=%#v", credential)
	}
}

func TestStartAndExchangePKCEUsesCurrentCodexOAuthFlow(t *testing.T) {
	var receivedForm url.Values
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/oauth/token" {
			t.Fatalf("token path = %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		receivedForm = r.Form
		_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"account_id":"acct"}`)
	}))
	defer issuer.Close()

	authorization, err := StartPKCE(issuer.URL, "http://127.0.0.1:8398/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorization.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/api/accounts/authorize" {
		t.Fatalf("authorization path = %s", parsed.Path)
	}
	query := parsed.Query()
	if query.Get("client_id") != codexDynamicClientID || query.Get("redirect_uri") != authorization.RedirectURI || query.Get("resource") != "https://api.openai.com/v1" {
		t.Fatalf("authorization query = %#v", query)
	}
	if query.Get("scope") != "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct" {
		t.Fatalf("scope = %q", query.Get("scope"))
	}

	credential, err := ExchangePKCE(context.Background(), issuer.URL, "auth-code", authorization, issuer.Client())
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "access" || credential.AccountID != "acct" {
		t.Fatalf("credential = %#v", credential)
	}
	if receivedForm.Get("client_id") != codexDynamicClientID || receivedForm.Get("code") != "auth-code" || receivedForm.Get("resource") != "https://api.openai.com/v1" {
		t.Fatalf("token form = %#v", receivedForm)
	}
}

func TestCodexToolTranslation(t *testing.T) {
	body, stream, err := codexRequest("/v1/chat/completions", []byte(`{"model":"gpt","messages":[{"role":"user","content":"call it"},{"role":"tool","tool_call_id":"call-1","content":"result"}],"tools":[{"type":"function","function":{"name":"lookup","description":"find a value","parameters":{"type":"object"}}}]}`), "gpt")
	if err != nil || stream {
		t.Fatalf("request translation = %s, %v, %v", body, err, stream)
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	input, _ := request["input"].([]any)
	if len(input) != 2 || input[1].(map[string]any)["type"] != "function_call_output" {
		t.Fatalf("input = %#v", request["input"])
	}
	tools, _ := request["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "lookup" {
		t.Fatalf("tools = %#v", request["tools"])
	}
}

func TestCodexRejectsUnsupportedMessageRoleAndMalformedImage(t *testing.T) {
	if _, _, err := codexRequest("/v1/chat/completions", []byte(`{"model":"gpt","messages":[{"role":"alien","content":"hi"}]}`), "gpt"); err == nil {
		t.Fatal("accepted unsupported message role")
	}
	if _, _, err := codexRequest("/v1/chat/completions", []byte(`{"model":"gpt","messages":[{"role":"user","content":[{"type":"image_url"}]}]}`), "gpt"); err == nil {
		t.Fatal("accepted malformed image content")
	}
}

func TestDeviceCodeLoginPollsPendingThenSucceeds(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			_, _ = io.WriteString(w, `{"device_auth_id":"device","user_code":"code","verification_url":"https://example.test/verify","interval":1}`)
		case "/api/accounts/deviceauth/token":
			if polls.Add(1) == 1 {
				_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
			} else {
				_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"account_id":"acct"}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	credential, err := DeviceCodeLogin(context.Background(), server.URL, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "access" || polls.Load() != 2 {
		t.Fatalf("credential=%#v polls=%d", credential, polls.Load())
	}
}
