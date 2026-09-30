package provider

import (
	"context"
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

func TestCodexRejectsCredentialBoundToOtherOrigin(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
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
