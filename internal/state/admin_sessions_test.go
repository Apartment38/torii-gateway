package state

import (
	"bytes"
	"testing"
	"time"
)

func TestAdminSessionsHashesExpiryRevocationAndPersistence(t *testing.T) {
	d, path, key := testDB(t)
	if _, err := d.CreateAdmin("admin", "administrator-password"); err != nil {
		t.Fatal(err)
	}
	token, s, err := d.NewAdminSession(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var hash []byte
	if err = d.db.QueryRow("SELECT token_hash FROM admin_sessions").Scan(&hash); err != nil || bytes.Contains(hash, []byte(token)) {
		t.Fatal("raw session stored")
	}
	d.Close()
	d, err = OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got, err := d.AdminSession(token); err != nil || got.CSRF != s.CSRF {
		t.Fatal("restart session lost", err)
	}
	if _, err = d.db.Exec("UPDATE admin_sessions SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	if _, err = d.AdminSession(token); err == nil {
		t.Fatal("expired session accepted")
	}
	token, _, err = d.NewAdminSession(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	admin, _ := d.Get(adminRecordID, Owner)
	admin = bytes.Replace(admin, []byte(`"username":"admin"`), []byte(`"username":"changed"`), 1)
	if err = d.Put(adminRecordID, Owner, "gateway", "administrator", admin); err != nil {
		t.Fatal(err)
	}
	if _, err = d.AdminSession(token); err == nil {
		t.Fatal("changed admin retained session")
	}
}
