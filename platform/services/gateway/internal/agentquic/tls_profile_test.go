package agentquic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSecretKeyPersistsOwnerOnlyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets", "transport.key")

	created, err := loadOrCreateSecretKey(path)
	if err != nil {
		t.Fatalf("loadOrCreateSecretKey(create): %v", err)
	}

	if created == [secretKeyBytes]byte{} {
		t.Fatal("created key is zero")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat created key: %v", err)
	}

	if got := info.Mode().Perm(); got != secretFileMode {
		t.Fatalf("created key permissions = %o, want 600", got)
	}

	loaded, err := loadOrCreateSecretKey(path)
	if err != nil {
		t.Fatalf("loadOrCreateSecretKey(load): %v", err)
	}

	if loaded != created {
		t.Fatal("existing transport key was replaced")
	}
}
