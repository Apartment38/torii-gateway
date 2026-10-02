package state

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
)

func TestGatewayClientKeysEncryptionPersistenceAndDeletion(t *testing.T) {
	d, path, master := testDB(t)
	if _, _, err := d.CreateGatewayAPIKey("before admin"); err == nil {
		t.Fatal("key created without administrator")
	}
	primary, err := d.CreateAdmin("admin", "long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "   ", "bad\nname", string(bytes.Repeat([]byte{'x'}, 129))} {
		if _, _, err := d.CreateGatewayAPIKey(name); err == nil {
			t.Fatal("invalid name accepted")
		}
	}
	metadata, first, err := d.CreateGatewayAPIKey(" Hermes agent ")
	if err != nil || metadata.Name != "Hermes agent" || metadata.CreatedAt == "" || first == primary {
		t.Fatal("creation", err)
	}
	secondMetadata, second, err := d.CreateGatewayAPIKey("Desktop")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{primary, first, second} {
		if !d.VerifyGatewayAPIKey(key) {
			t.Fatal("valid key rejected")
		}
	}
	if d.VerifyGatewayAPIKey(first+"x") || d.VerifyGatewayAPIKey("") {
		t.Fatal("invalid key accepted")
	}
	if _, err := d.Get(metadata.ID, "other-user"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatal("credential owner isolation", err)
	}
	var ciphertext []byte
	if err := d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id=?", metadata.ID).Scan(&ciphertext); err != nil || bytes.Contains(ciphertext, []byte(first)) {
		t.Fatal("plaintext storage", err)
	}
	keys, err := d.ListGatewayAPIKeys()
	if err != nil || len(keys) != 3 {
		t.Fatal("list", err)
	}
	// Rotation replaces only the default key, leaving named clients connected.
	rotated, err := d.RotateAPIKey()
	if err != nil || d.VerifyGatewayAPIKey(primary) || !d.VerifyGatewayAPIKey(rotated) || !d.VerifyGatewayAPIKey(first) {
		t.Fatal("rotation", err)
	}
	d.Close()
	d, err = OpenDatabase(path, master)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !d.VerifyGatewayAPIKey(first) || !d.VerifyGatewayAPIKey(second) || !d.VerifyGatewayAPIKey(rotated) {
		t.Fatal("restart lost keys")
	}
	if err := d.DeleteGatewayAPIKey(metadata.ID); err != nil {
		t.Fatal(err)
	}
	if d.VerifyGatewayAPIKey(first) || !d.VerifyGatewayAPIKey(second) {
		t.Fatal("key revocation isolation")
	}
	for _, table := range []string{"credentials", "gateway_api_keys"} {
		column := "id"
		if table == "gateway_api_keys" {
			column = "credential_id"
		}
		var count int
		if err := d.db.QueryRow("SELECT count(*) FROM "+table+" WHERE "+column+"=?", metadata.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("key not physically removed", err)
		}
	}
	if err := d.DeleteGatewayAPIKey("__admin"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatal("deleted admin", err)
	}
	if err := d.DeleteGatewayAPIKey(metadata.ID); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatal("deleted missing key", err)
	}
	if err := d.DeleteGatewayAPIKey(GatewayAPIKeyID); err != nil || d.VerifyGatewayAPIKey(rotated) {
		t.Fatal("default revocation", err)
	}
	if err := d.DeleteGatewayAPIKey(secondMetadata.ID); err != nil {
		t.Fatal(err)
	}
	if !d.VerifyAdmin("admin", "long-enough-password") {
		t.Fatal("deleting API keys locked out console")
	}
	keys, err = d.ListGatewayAPIKeys()
	if err != nil || len(keys) != 0 {
		t.Fatal("empty keys", err)
	}
}

func TestGatewayKeyHashTamperingDoesNotAuthenticate(t *testing.T) {
	d, _, _ := testDB(t)
	if _, err := d.CreateAdmin("admin", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	m, _, err := d.CreateGatewayAPIKey("client")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("forged-key"))
	if _, err := d.db.Exec("UPDATE gateway_api_keys SET key_hash=? WHERE credential_id=?", digest[:], m.ID); err != nil {
		t.Fatal(err)
	}
	if d.VerifyGatewayAPIKey("forged-key") {
		t.Fatal("lookup metadata bypassed encrypted key verification")
	}
}

func TestSchemaFourUpgradePreservesExistingCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "torii.db")
	master := bytes.Repeat([]byte{0x36}, 32)
	d, err := openDatabase(path, master, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := d.CreateAdmin("existing-admin", "existing-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Put("provider-secret", Owner, "provider", "api_key", []byte("preserved-secret")); err != nil {
		t.Fatal(err)
	}
	var original []byte
	d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id=?", GatewayAPIKeyID).Scan(&original)
	d.Close()
	if bad, err := OpenDatabase(path, bytes.Repeat([]byte{0x37}, 32)); err == nil {
		bad.Close()
		t.Fatal("wrong master key accepted")
	}
	d, err = openDatabase(path, master, 4, nil)
	if err != nil {
		t.Fatal("wrong key advanced the schema", err)
	}
	d.Close()
	for attempt := 0; attempt < 2; attempt++ {
		d, err = OpenDatabase(path, master)
		if err != nil {
			t.Fatal(err)
		}
		var version int
		d.db.QueryRow("PRAGMA user_version").Scan(&version)
		var encrypted []byte
		d.db.QueryRow("SELECT ciphertext FROM credentials WHERE id=?", GatewayAPIKeyID).Scan(&encrypted)
		secret, err := d.Get("provider-secret", Owner)
		if err != nil || string(secret) != "preserved-secret" || version != SchemaVersion || !bytes.Equal(original, encrypted) || !d.VerifyGatewayAPIKey(key) || !d.VerifyAdmin("existing-admin", "existing-long-password") {
			t.Fatal("upgrade lost existing state", err)
		}
		keys, err := d.ListGatewayAPIKeys()
		if err != nil || len(keys) != 1 || keys[0].ID != GatewayAPIKeyID {
			t.Fatal("existing key missing from list", err)
		}
		d.Close()
	}
}
