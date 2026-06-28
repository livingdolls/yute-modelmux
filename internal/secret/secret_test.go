package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV1ToV2Migration(t *testing.T) {
	masterKey := "test-migration-key-12345"
	t.Setenv("MODELMUX_MASTER_KEY", masterKey)

	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")

	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := store.Set("key1", "value1"); err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("failed to read store: %v", err)
	}
	content := string(data)
	if strings.HasPrefix(content, v2Prefix) {
		t.Log("new store uses v2 format as expected")
	} else {
		t.Fatal("expected v2 format prefix on new store")
	}

	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	v, err := store2.Get("key1")
	if err != nil {
		t.Fatalf("failed to get after reopen: %v", err)
	}
	if v != "value1" {
		t.Fatalf("got %q, want value1", v)
	}

	if store2.salt == nil || len(store2.salt) != saltLen {
		t.Fatal("reopened store should have salt set")
	}
}

func TestLegacyV1Decrypt(t *testing.T) {
	masterKey := "legacy-test-key-abcde"
	t.Setenv("MODELMUX_MASTER_KEY", masterKey)

	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")

	legacyStore := &Store{
		path: storePath,
		key:  deriveKey(masterKey, nil),
		data: map[string]string{"old-key": "old-value"},
	}
	encrypted, err := legacyStore.encryptLegacy([]byte(`{"old-key":"old-value"}`))
	if err != nil {
		t.Fatalf("legacy encrypt failed: %v", err)
	}
	if err := os.WriteFile(storePath, encrypted, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("open legacy store failed: %v", err)
	}
	v, err := store.Get("old-key")
	if err != nil {
		t.Fatalf("get from legacy store failed: %v", err)
	}
	if v != "old-value" {
		t.Fatalf("got %q, want old-value", v)
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read after opening legacy store failed: %v", err)
	}
	if !strings.HasPrefix(string(data), v2Prefix) {
		t.Fatal("legacy store should be migrated to v2 format when opened")
	}

	if err := store.Set("new-key", "new-value"); err != nil {
		t.Fatalf("set after migration failed: %v", err)
	}
	data, err = os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read after migration failed: %v", err)
	}
	if !strings.HasPrefix(string(data), v2Prefix) {
		t.Fatal("migrated store should use v2 format")
	}

	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("reopen migrated store failed: %v", err)
	}
	v1, _ := store2.Get("old-key")
	v2, _ := store2.Get("new-key")
	if v1 != "old-value" || v2 != "new-value" {
		t.Fatalf("migrated store data mismatch: old=%q new=%q", v1, v2)
	}
}

func (s *Store) encryptLegacy(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return []byte(encoded), nil
}

func TestRotateMasterKey(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")

	t.Setenv("MODELMUX_MASTER_KEY", "old-master-key-abcdef")
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	if err := store.Set("k1", "v1"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if err := store.Set("k2", "v2"); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if err := store.RotateKey("new-master-key-123456"); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}

	t.Setenv("MODELMUX_MASTER_KEY", "new-master-key-123456")
	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("reopen with new key failed: %v", err)
	}
	if v, _ := store2.Get("k1"); v != "v1" {
		t.Fatalf("expected v1, got %q", v)
	}
	if v, _ := store2.Get("k2"); v != "v2" {
		t.Fatalf("expected v2, got %q", v)
	}

	t.Setenv("MODELMUX_MASTER_KEY", "old-master-key-abcdef")
	_, err = NewStore(storePath)
	if err == nil {
		t.Fatal("old master key should not work after rotation")
	}
}

func TestExportImport(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")

	t.Setenv("MODELMUX_MASTER_KEY", "export-test-key-xyz")
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	store.Set("original", "data")

	data, err := store.ExportData()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	importPath := filepath.Join(dir, "imported.enc")
	if err := ImportData(importPath, data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	store2, err := NewStore(importPath)
	if err != nil {
		t.Fatalf("open imported failed: %v", err)
	}
	if v, _ := store2.Get("original"); v != "data" {
		t.Fatalf("imported data mismatch: got %q", v)
	}
}

func TestVerifyFile(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")

	t.Setenv("MODELMUX_MASTER_KEY", "verify-test-key")
	store, _ := NewStore(storePath)
	store.Set("x", "y")

	if err := VerifyFile(storePath); err != nil {
		t.Fatalf("verify should pass: %v", err)
	}

	if err := VerifyFile("/nonexistent/path/secrets.enc"); err == nil {
		t.Fatal("verify should fail for missing file")
	}
}
