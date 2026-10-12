package state

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAdminEmailUpdateReauthenticationAndRevocation(t *testing.T) {
	d, _, _ := testDB(t)
	if _, err := d.CreateAdminWithEmail("admin", "bad\r\nBcc: victim@example.com", "old-password-123"); err == nil {
		t.Fatal("header injection accepted")
	}
	key, err := d.CreateAdminWithEmail("admin", "Admin@Example.com", "old-password-123")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := d.Admin()
	if a.Email != "admin@example.com" {
		t.Fatal(a.Email)
	}
	token, _, err := d.NewAdminSession(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.UpdateAdmin("new-admin", "new@example.com", "", "wrong-password"); err == nil {
		t.Fatal("no reauthentication")
	}
	if err = d.UpdateAdmin("new-admin", "new@example.com", "", "old-password-123"); err != nil {
		t.Fatal(err)
	}
	if !d.VerifyAdmin("new-admin", "old-password-123") || d.VerifyAdmin("admin", "old-password-123") {
		t.Fatal("username update")
	}
	if _, err = d.AdminSession(token); err == nil {
		t.Fatal("session not revoked")
	}
	if got, _ := d.GatewayAPIKey(); got != key {
		t.Fatal("inference key changed")
	}
	if err = d.UpdateAdmin("new-admin", "new@example.com", "short", "old-password-123"); err == nil {
		t.Fatal("short password")
	}
	if err = d.UpdateAdmin("new-admin", "new@example.com", "new-password-123", "old-password-123"); err != nil {
		t.Fatal(err)
	}
	if !d.VerifyAdmin("new-admin", "new-password-123") || d.VerifyAdmin("new-admin", "old-password-123") {
		t.Fatal("password update")
	}
}
func TestPasswordResetPersistenceExpirySingleUseAndEncryption(t *testing.T) {
	d, path, key := testDB(t)
	if _, err := d.CreateAdminWithEmail("admin", "admin@example.com", "old-password-123"); err != nil {
		t.Fatal(err)
	}
	session, _, _ := d.NewAdminSession(time.Hour)
	if _, _, err := d.IssueAdminPasswordReset("other@example.com"); !errors.Is(err, ErrInvalidReset) {
		t.Fatal(err)
	}
	token, email, err := d.IssueAdminPasswordReset("ADMIN@example.com")
	if err != nil || email != "admin@example.com" {
		t.Fatal(email, err)
	}
	if _, _, err = d.IssueAdminPasswordReset(email); err == nil {
		t.Fatal("mail cooldown ignored")
	}
	for _, file := range []string{path, path + "-wal"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{token, email, "old-password-123"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatal("plaintext persisted")
			}
		}
	}
	d.Close()
	d, err = OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.ResetAdminPassword(strings.Repeat("0", 64), "new-password-123"); !errors.Is(err, ErrInvalidReset) {
		t.Fatal(err)
	}
	if err = d.ResetAdminPassword(token, "short"); err == nil {
		t.Fatal("short reset password")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- d.ResetAdminPassword(token, "new-password-123") }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("reset replay", successes)
	}
	if !d.VerifyAdmin("admin", "new-password-123") || d.VerifyAdmin("admin", "old-password-123") {
		t.Fatal("password not reset")
	}
	if _, err = d.AdminSession(session); err == nil {
		t.Fatal("reset retained browser session")
	}
	token, _, err = d.IssueAdminPasswordReset(email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.db.Exec("UPDATE admin_password_reset SET expires_at=?", time.Now().Unix()-1); err != nil {
		t.Fatal(err)
	}
	if err = d.ResetAdminPassword(token, "another-password-123"); !errors.Is(err, ErrInvalidReset) {
		t.Fatal("expired link accepted", err)
	}
}
func TestResetReplacementAndAccountChangesInvalidateLinks(t *testing.T) {
	d, _, _ := testDB(t)
	d.CreateAdminWithEmail("admin", "admin@example.com", "old-password-123")
	old, _, err := d.IssueAdminPasswordReset("admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	d.db.Exec("UPDATE admin_password_reset SET requested_at=0")
	next, _, err := d.IssueAdminPasswordReset("admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err = d.ResetAdminPassword(old, "new-password-123"); !errors.Is(err, ErrInvalidReset) {
		t.Fatal("replaced link accepted")
	}
	if err = d.UpdateAdmin("admin", "other@example.com", "", "old-password-123"); err != nil {
		t.Fatal(err)
	}
	if err = d.ResetAdminPassword(next, "new-password-123"); !errors.Is(err, ErrInvalidReset) {
		t.Fatal("link survived email change")
	}
}
