package session_test

import (
	"github.com/Apartment38/torii-gateway/internal/session"
	"testing"
	"time"
)

func TestIdentifyPrecedenceAndHashing(t *testing.T) {
	a := session.Identify(map[string]string{"X-Session-ID": "abc"}, nil, "user", "chat", "system", "first")
	b := session.Identify(map[string]string{"X-Session-ID": "abc"}, nil, "user", "chat", "system", "changed")
	if !a.Explicit || a.ID != "abc" || a.Signature == b.Signature {
		t.Fatalf("identities=%#v %#v", a, b)
	}
	if got := session.Identify(nil, nil, "", "", "", ""); got.ID != "" {
		t.Fatal("empty context created affinity")
	}
}

func TestManagerExpiresAndBounds(t *testing.T) {
	m := session.NewManager(time.Millisecond, 1)
	if err := m.Put(session.Record{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Put(session.Record{ID: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("a"); ok {
		t.Fatal("oldest session was not evicted")
	}
	time.Sleep(2 * time.Millisecond)
	if _, ok := m.Get("b"); ok {
		t.Fatal("expired session returned")
	}
}
