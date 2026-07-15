package operation

import (
	"path/filepath"
	"testing"
)

func TestJournalDuplicateConflictAndRecovery(t *testing.T) { //nolint:cyclop // One test owns the ordered crash-state matrix.
	t.Parallel()
	path := filepath.Join(t.TempDir(), "operations.json")

	journal, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	operationID := [16]byte{1}
	if disposition, err := journal.Begin("agent", 0x101, operationID, []byte("first")); err != nil || disposition != Execute {
		t.Fatalf("Begin() = %v, %v", disposition, err)
	}

	recovered, err := Open(path)
	if err != nil {
		t.Fatalf("Open() recovery error = %v", err)
	}

	if _, err := recovered.Begin("agent", 0x101, operationID, []byte("first")); err == nil {
		t.Fatal("Begin() after ambiguous restart succeeded")
	}

	secondID := [16]byte{2}
	if _, err := recovered.Begin("agent", 0x101, secondID, []byte("second")); err != nil {
		t.Fatalf("Begin() second error = %v", err)
	}

	if err := recovered.Commit("agent", 0x101, secondID); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if disposition, err := recovered.Begin("agent", 0x101, secondID, []byte("second")); err != nil || disposition != Duplicate {
		t.Fatalf("Begin() duplicate = %v, %v", disposition, err)
	}

	if _, err := recovered.Begin("agent", 0x101, secondID, []byte("changed")); err == nil {
		t.Fatal("Begin() conflicting payload succeeded")
	}

	if err := recovered.Release("agent", 0x101, [16]byte{9}); err != nil {
		t.Fatalf("Release() missing error = %v", err)
	}
}
