// Package session provides bounded, in-memory smart-routing affinity.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	MaxIDLength = 256
	MaxSessions = 10000
)

type Identity struct {
	ID        string
	Signature string
	Explicit  bool
}

// Identify resolves a stable, non-secret identity. Explicit headers take
// precedence over metadata and user; otherwise a bounded conversation
// fingerprint is used. Raw identifiers and request content are never stored.
func Identify(headers map[string]string, metadata map[string]any, user string, endpoint string, system string, firstUser string) Identity {
	for _, key := range []string{"X-Session-ID", "X-Conversation-ID"} {
		if value := safeID(headers[key]); value != "" {
			return Identity{ID: value, Signature: fingerprint(endpoint, system, firstUser), Explicit: true}
		}
	}
	if value, ok := metadata["session_id"].(string); ok {
		if value = safeID(value); value != "" {
			return Identity{ID: value, Signature: fingerprint(endpoint, system, firstUser), Explicit: true}
		}
	}
	if value := safeID(user); value != "" {
		return Identity{ID: "user:" + value, Signature: fingerprint(endpoint, system, firstUser)}
	}
	if system == "" && firstUser == "" {
		return Identity{}
	}
	return Identity{ID: "conversation:" + fingerprint(endpoint, system, firstUser), Signature: fingerprint(endpoint, system, firstUser)}
}

func safeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > MaxIDLength || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func fingerprint(endpoint, system, firstUser string) string {
	h := sha256.New()
	for _, value := range []string{endpoint, system, firstUser} {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(value))
	}
	return hex.EncodeToString(h.Sum(nil))
}

type Record struct {
	ID               string
	Model            string
	Classification   any
	CreatedAt        time.Time
	LastSeen         time.Time
	ContextSignature string
	ToolLoopActive   bool
}

type Manager struct {
	mu      sync.Mutex
	entries map[string]Record
	max     int
	timeout time.Duration
	now     func() time.Time
}

func NewManager(timeout time.Duration, max int) *Manager {
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	if max <= 0 || max > MaxSessions {
		max = MaxSessions
	}
	return &Manager{entries: make(map[string]Record), max: max, timeout: timeout, now: time.Now}
}

func (m *Manager) Get(id string) (Record, bool) {
	return m.GetWithSignature(id, "")
}

func (m *Manager) GetWithSignature(id, signature string) (Record, bool) {
	if m == nil || id == "" {
		return Record{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.entries[id]
	if !ok {
		return Record{}, false
	}
	if signature != "" && record.ContextSignature != "" && signature != record.ContextSignature {
		delete(m.entries, id)
		return Record{}, false
	}
	if m.now().Sub(record.LastSeen) >= m.timeout {
		delete(m.entries, id)
		return Record{}, false
	}
	record.LastSeen = m.now()
	m.entries[id] = record
	return record, true
}

func (m *Manager) Put(record Record) error {
	if m == nil || record.ID == "" {
		return errors.New("session ID is required")
	}
	if len(record.ID) > MaxIDLength {
		return errors.New("session ID is too long")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.entries[record.ID]; !exists && len(m.entries) >= m.max {
		m.evictOldest()
	}
	now := m.now()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.LastSeen.IsZero() {
		record.LastSeen = now
	}
	m.entries[record.ID] = record
	return nil
}

func (m *Manager) Delete(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.entries, id)
	m.mu.Unlock()
}

func (m *Manager) Cleanup() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	now := m.now()
	for id, record := range m.entries {
		if now.Sub(record.LastSeen) >= m.timeout {
			delete(m.entries, id)
			removed++
		}
	}
	return removed
}

func (m *Manager) evictOldest() {
	var oldest string
	var at time.Time
	for id, record := range m.entries {
		if oldest == "" || record.LastSeen.Before(at) {
			oldest, at = id, record.LastSeen
		}
	}
	if oldest != "" {
		delete(m.entries, oldest)
	}
}

// ContextSignature computes a stable signature from request context without
// retaining prompt text. It is intentionally exported for API adapters.
func ContextSignature(endpoint string, messages any) string {
	data, _ := json.Marshal(messages)
	return fingerprint(endpoint, "", string(data))
}
