package provider

import (
	"context"
	"github.com/Apartment38/torii-gateway/internal/quota"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCodexQuotaWindowsAndAuthentication(t *testing.T) {
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://chatgpt.com/backend-api/wham/usage" || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("ChatGPT-Account-ID") != "test-account" {
			t.Fatalf("wrong quota request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":18000,"reset_at":1900000000},"secondary_window":{"used_percent":100,"limit_window_seconds":604800,"reset_at":1900100000}}}`))}, nil
	})}
	c, err := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
		return CodexCredential{AccessToken: "test-token", AccountID: "test-account", Endpoint: DefaultCodexBaseURL}, nil
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	cache := quota.NewCache(time.Minute, c)
	s, err := cache.Get(context.Background(), "codex/model")
	if err != nil || !s.Known || !s.Exhausted || len(s.Windows) != 2 || s.ResetAt.Unix() != 1900100000 {
		t.Fatalf("quota: %+v %v", s, err)
	}
}
func TestCodexQuotaUnknownAndErrors(t *testing.T) {
	for _, body := range []string{`{}`, `{"rate_limit":{"primary_window":{"used_percent":-1}}}`} {
		c, _ := NewCodex("codex", DefaultCodexBaseURL, nil, func(string) (CodexCredential, error) {
			return CodexCredential{AccessToken: "test-token", Endpoint: DefaultCodexBaseURL}, nil
		}, &http.Client{Transport: codexTestTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})})
		s, err := quota.NewCache(time.Minute, c).Get(context.Background(), "model")
		if body == `{}` {
			if err != nil || s.Known {
				t.Fatalf("unknown: %+v %v", s, err)
			}
		} else if err == nil && s.Known {
			t.Fatal("invalid percentage became known quota")
		}
	}
}
