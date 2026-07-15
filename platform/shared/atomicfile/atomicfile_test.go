package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceAndCreateSynchronizedState(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	statePath := filepath.Join(directory, "state.json")
	if err := Replace(statePath, []byte("first"), 0o700, 0o600); err != nil {
		t.Fatalf("write first state: %v", err)
	}
	if err := Replace(statePath, []byte("second"), 0o700, 0o600); err != nil {
		t.Fatalf("replace state: %v", err)
	}
	data, err := os.ReadFile(statePath) // #nosec G304 -- the test path is rooted in t.TempDir.
	if err != nil || string(data) != "second" {
		t.Fatalf("retained state = %q, %v", data, err)
	}
	keyPath := filepath.Join(directory, "key")
	if err := Create(keyPath, []byte("key"), 0o700, 0o600); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := Create(keyPath, []byte("replacement"), 0o700, 0o600); err == nil {
		t.Fatal("exclusive key creation replaced existing key")
	}
}
