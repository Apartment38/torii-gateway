package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestBasicAdminAuthenticationAndFailureLimit(t *testing.T) {
	calls := 0
	a := NewBasicAdminAuthenticator(func(u, p string) bool { calls++; return u == "chosen" && p == "password" })
	now := time.Now()
	a.clock = func() time.Time { return now }
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer password")
	if _, err := a.Authenticate(req); err == nil {
		t.Fatal("API key accepted as administrator")
	}
	req.SetBasicAuth("chosen", "password")
	req.Header.Add("Authorization", "Basic duplicate")
	if _, err := a.Authenticate(req); err == nil {
		t.Fatal("duplicate authorization accepted")
	}
	req.SetBasicAuth("chosen", "password")
	id, err := a.Authenticate(req)
	if err != nil || id.ID != "default" {
		t.Fatal("valid login rejected")
	}
	req.SetBasicAuth("chosen", "wrong")
	for i := 0; i < 5; i++ {
		if _, err = a.Authenticate(req); err == nil {
			t.Fatal("wrong password accepted")
		}
	}
	before := calls
	req.SetBasicAuth("chosen", "password")
	if _, err = a.Authenticate(req); err == nil || calls != before {
		t.Fatal("failed-attempt limit bypassed")
	}
	now = now.Add(61 * time.Second)
	if _, err = a.Authenticate(req); err != nil {
		t.Fatal("rate limit did not expire")
	}
}
