package state

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestAdminCreationHashingRotationAndPersistence(t *testing.T) {
	d, path, key := testDB(t)
	required, err := d.AdminRequired()
	if err != nil || !required {
		t.Fatal(required, err)
	}
	if _, err = d.CreateAdmin("bad:name", "long-enough-password"); err == nil {
		t.Fatal("invalid username accepted")
	}
	if _, err = d.CreateAdmin("my-admin", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	apiKey, err := d.CreateAdmin("my-admin", "long-enough-password")
	if err != nil || !strings.HasPrefix(apiKey, "torii_") {
		t.Fatal(err)
	}
	if !d.VerifyAdmin("my-admin", "long-enough-password") || d.VerifyAdmin("other", "long-enough-password") || d.VerifyAdmin("my-admin", "wrong-password") {
		t.Fatal("password verification")
	}
	admin, err := d.Admin()
	if err != nil || len(admin.Salt) != 32 || bytes.Contains(admin.Hash, []byte("long-enough-password")) {
		t.Fatal("unsafe password record")
	}
	if _, err = d.CreateAdmin("second", "another-long-password"); err == nil {
		t.Fatal("second administrator allowed")
	}
	list, err := d.List(Owner)
	if err != nil || len(list) != 1 || list[0].ID != GatewayAPIKeyID {
		t.Fatal("admin hash exposed as credential metadata")
	}
	next, err := d.RotateAPIKey()
	if err != nil || next == apiKey {
		t.Fatal("rotation", err)
	}
	for _, p := range []string{path, path + "-wal"} {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{apiKey, next, "long-enough-password", "my-admin"} {
			if bytes.Contains(raw, []byte(s)) {
				t.Fatal("plaintext written to SQLite")
			}
		}
	}
	d.Close()
	restarted, err := OpenDatabase(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	stored, err := restarted.GatewayAPIKey()
	if err != nil || stored != next || !restarted.VerifyAdmin("my-admin", "long-enough-password") {
		t.Fatal("restart lost authentication")
	}
}
func TestImportedAPIKeyIsNeverRevealedOrOverwrittenBySetup(t *testing.T) {
	d, _, _ := testDB(t)
	if err := d.AdoptGatewayAPIKey("old-env-key"); err != nil {
		t.Fatal(err)
	}
	revealed, err := d.CreateAdmin("chosen-user", "long-enough-password")
	if err != nil || revealed != "" {
		t.Fatal("imported key revealed", err)
	}
	stored, _ := d.GatewayAPIKey()
	if stored != "old-env-key" {
		t.Fatal("imported key overwritten")
	}
	next, err := d.RotateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if err = d.AdoptGatewayAPIKey("changed-env-key"); err != nil {
		t.Fatal(err)
	}
	stored, _ = d.GatewayAPIKey()
	if stored != next {
		t.Fatal("legacy input overwrote rotation")
	}
}
func TestConcurrentAdminCreationHasSingleWinner(t *testing.T) {
	d, _, _ := testDB(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"first", "second"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, err := d.CreateAdmin(name, "long-enough-password")
			results <- err
		}(name)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("multiple administrators created")
	}
}
