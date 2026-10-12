package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexAppServerHelper(t *testing.T) {
	if os.Getenv("TORII_TEST_APP_SERVER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"userAgent": "synthetic"}
		case "initialized":
			continue
		case "account/read":
			result = map[string]any{"account": map[string]any{"type": "chatgpt", "email": "synthetic@example.test", "planType": "plus"}}
		case "account/rateLimits/read":
			fmt.Println(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"limitId":"wrong","primary":{"usedPercent":99}}}}`)
			account := os.Getenv("TORII_TEST_APP_ACCOUNT")
			result = map[string]any{"accountId": account, "rateLimitsByLimitId": map[string]any{"codex": map[string]any{"limitId": "codex", "primary": map[string]any{"usedPercent": 35, "windowDurationMins": 300}}}}
		default:
			os.Exit(3)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"id": request.ID, "result": result})
	}
	os.Exit(0)
}

func TestCodexAppServerIdentityAndFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test launcher uses a POSIX shell; production uses exec.CommandContext")
	}
	t.Setenv("TORII_TEST_APP_SERVER", "1")
	script := filepath.Join(t.TempDir(), "codex")
	quoted := "'" + strings.ReplaceAll(os.Args[0], "'", "'\"'\"'") + "'"
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec "+quoted+" -test.run=^TestCodexAppServerHelper$ -- \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, path, account, email string
		direct                     bool
	}{{"matching", script, "account", "synthetic@example.test", false}, {"wrong workspace", script, "other", "synthetic@example.test", true}, {"missing workspace", script, "", "synthetic@example.test", true}, {"wrong email", script, "account", "other@example.test", true}, {"missing Torii user identity", script, "account", "", true}, {"missing executable", script + "-missing", "account", "synthetic@example.test", true}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TORII_TEST_APP_ACCOUNT", tt.account)
			var direct int
			c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
				return CodexCredential{AccessToken: "synthetic", AccountID: "account", Email: tt.email, Endpoint: DefaultCodexBaseURL}, nil
			}, &http.Client{Transport: codexTestTransport(func(*http.Request) (*http.Response, error) {
				direct++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":604800}}}`))}, nil
			})})
			c.ConfigureAppServer(CodexAppServerConfig{Path: tt.path})
			s, err := c.Quota(context.Background(), "m")
			if err != nil || s.State != "available" {
				t.Fatalf("state=%s error=%v", s.State, err)
			}
			if (direct == 1) != tt.direct {
				t.Fatalf("direct calls=%d", direct)
			}
			if !tt.direct && (s.Source != "codex_app_server" || s.Limits[0].Windows[0].Used != 35) {
				t.Fatal("notification replaced authenticated read")
			}
		})
	}
}

func TestCodexAppServerTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher")
	}
	script := filepath.Join(t.TempDir(), "codex")
	// The child leaves a descendant holding stdout; cancellation must still bound reads.
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 1 &\nwait\n"), 0700)
	c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
		return CodexCredential{AccountID: "account", Email: "synthetic@example.test"}, nil
	}, nil)
	c.ConfigureAppServer(CodexAppServerConfig{Path: script})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.appServerQuota(ctx, CodexCredential{AccountID: "account", Email: "synthetic@example.test"})
	if err == nil || time.Since(start) > 500*time.Millisecond {
		t.Fatal("subprocess deadline was not enforced")
	}
}
