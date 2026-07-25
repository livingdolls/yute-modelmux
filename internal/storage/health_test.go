package storage

import (
	"path/filepath"
	"testing"
)

func TestGuardedStorageBecomesUnhealthyAfterPersistenceFailure(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "modelmux.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveKeyRuntime(KeyRuntimeRecord{KeyID: "key-1"}); err == nil {
		t.Fatal("expected write to closed database to fail")
	}
	if err := HealthError(store); err == nil {
		t.Fatal("expected storage health error")
	}
}
