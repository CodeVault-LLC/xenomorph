package keyservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateCommandSignerPersistsDedicatedKey(t *testing.T) {
	directory := t.TempDir()
	privatePath := filepath.Join(directory, "command-private.pem")
	publicPath := filepath.Join(directory, "command-public.pem")

	firstService := readyTestService()

	first, err := firstService.LoadOrCreateCommandSigner(context.Background(), privatePath, publicPath, 1)
	if err != nil {
		t.Fatalf("first LoadOrCreateCommandSigner() error = %v", err)
	}

	privateInfo, err := os.Stat(privatePath)
	if err != nil {
		t.Fatalf("Stat(private key) error = %v", err)
	}

	if privateInfo.Mode().Perm() != privateKeyPermission {
		t.Fatalf("private key permissions = %o, want %o", privateInfo.Mode().Perm(), privateKeyPermission)
	}

	secondService := readyTestService()

	second, err := secondService.LoadOrCreateCommandSigner(context.Background(), privatePath, publicPath, 1)
	if err != nil {
		t.Fatalf("second LoadOrCreateCommandSigner() error = %v", err)
	}

	if second.KeyID() != first.KeyID() {
		t.Fatalf("reloaded key ID = %q, want %q", second.KeyID(), first.KeyID())
	}
}

func TestLoadCommandPrivateKeyRejectsBroadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "command-private.pem")
	broadPermission := os.FileMode(0o600 | 0o044)

	if err := os.WriteFile(path, []byte("not a key"), broadPermission); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, _, err := loadCommandPrivateKey(path); err == nil {
		t.Fatal("loadCommandPrivateKey() error = nil for broad permissions")
	}
}

func TestValidateCommandKeyPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		privatePath string
		publicPath  string
		version     uint64
	}{
		{name: "missing private path", publicPath: "public.pem", version: 1},
		{name: "same path", privatePath: "key.pem", publicPath: "key.pem", version: 1},
		{name: "missing version", privatePath: "private.pem", publicPath: "public.pem"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := validateCommandKeyPaths(test.privatePath, test.publicPath, test.version); err == nil {
				t.Fatal("validateCommandKeyPaths() error = nil, want error")
			}
		})
	}
}
