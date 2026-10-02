package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// BasicAdminAuthenticator shares the application's singleton administrator.
// Failed attempts are bounded per client; only hashes live in the database.
type BasicAdminAuthenticator struct {
	Verify   func(string, string) bool
	mu       sync.Mutex
	attempts map[string]authAttempts
	clock    func() time.Time
	workers  chan struct{}
	pending  chan struct{}
}
type authAttempts struct {
	count int
	until time.Time
}

func NewBasicAdminAuthenticator(verify func(string, string) bool) *BasicAdminAuthenticator {
	return &BasicAdminAuthenticator{Verify: verify, attempts: map[string]authAttempts{}, clock: time.Now, workers: make(chan struct{}, 4), pending: make(chan struct{}, 32)}
}
func (*BasicAdminAuthenticator) Challenge() string {
	return `Basic realm="Torii Gateway", charset="UTF-8"`
}
func (a *BasicAdminAuthenticator) Authenticate(r *http.Request) (*ClientIdentity, error) {
	if a == nil || r == nil || a.Verify == nil || len(r.Header.Values("Authorization")) != 1 {
		return nil, ErrUnauthorized
	}
	u, p, ok := r.BasicAuth()
	if !ok || len(u) > 64 || len(p) > 1024 {
		return nil, ErrUnauthorized
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	now := a.clock()
	select {
	case a.pending <- struct{}{}:
	default:
		return nil, ErrUnauthorized
	}
	defer func() { <-a.pending }()
	select {
	case a.workers <- struct{}{}:
	case <-r.Context().Done():
		return nil, ErrUnauthorized
	}
	defer func() { <-a.workers }()
	a.mu.Lock()
	for key, v := range a.attempts {
		if !now.Before(v.until) {
			delete(a.attempts, key)
		}
	}
	v := a.attempts[ip]
	if v.count >= 5 || len(a.attempts) >= 1024 {
		a.mu.Unlock()
		return nil, ErrUnauthorized
	}
	a.mu.Unlock()
	if !a.Verify(u, p) {
		a.mu.Lock()
		v = a.attempts[ip]
		v.count++
		if v.until.IsZero() {
			v.until = now.Add(time.Minute)
		}
		a.attempts[ip] = v
		a.mu.Unlock()
		return nil, ErrUnauthorized
	}
	a.mu.Lock()
	delete(a.attempts, ip)
	a.mu.Unlock()
	return &ClientIdentity{ID: "default"}, nil
}
