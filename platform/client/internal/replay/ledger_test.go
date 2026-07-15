package replay

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
)

func TestLedgerReservesCompletesAndRejectsReplay(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()

	ledger, err := Open(filepath.Join(directory, "ledger.json"), filepath.Join(directory, "ledger.key"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}

	entry := replayEntry()
	if err := ledger.Reserve(entry); err != nil {
		t.Fatalf("reserve command: %v", err)
	}

	if err := ledger.Reserve(entry); !errors.Is(err, agent.ErrCommandReplay) {
		t.Fatalf("duplicate reserve error = %v, want replay", err)
	}

	resultDigest := sha256.Sum256([]byte("terminal"))
	if err := ledger.Complete(entry.CommandID, resultDigest); err != nil {
		t.Fatalf("complete command: %v", err)
	}

	if err := ledger.Complete(entry.CommandID, resultDigest); err != nil {
		t.Fatalf("idempotent completion failed: %v", err)
	}
}

func TestLedgerRecoversAcceptedAsOutcomeUnknown(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	keyPath := filepath.Join(directory, "ledger.key")

	ledger, err := Open(path, keyPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}

	entry := replayEntry()
	if err := ledger.Reserve(entry); err != nil {
		t.Fatalf("reserve command: %v", err)
	}

	recovered, err := Open(path, keyPath)
	if err != nil {
		t.Fatalf("recover ledger: %v", err)
	}

	if err := recovered.Reserve(entry); !errors.Is(err, agent.ErrCommandOutcomeUnknown) {
		t.Fatalf("recovered reserve error = %v, want outcome unknown", err)
	}
}

func TestLedgerRejectsTamperingAndLoosePermissions(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	keyPath := filepath.Join(directory, "ledger.key")

	ledger, err := Open(path, keyPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}

	if err := ledger.Reserve(replayEntry()); err != nil {
		t.Fatalf("reserve command: %v", err)
	}

	data, err := os.ReadFile(path) // #nosec G304 -- test path is created under t.TempDir.
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}

	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, stateFileMode); err != nil { // #nosec G703 -- test path is created under t.TempDir.
		t.Fatalf("tamper ledger: %v", err)
	}

	if _, err := Open(path, keyPath); err == nil {
		t.Fatal("tampered ledger was accepted")
	}

	if err := os.Chmod(keyPath, 0o644); err != nil { //nolint:gosec // The test deliberately creates insecure permissions.
		t.Fatalf("loosen key permissions: %v", err)
	}

	if _, err := Open(filepath.Join(directory, "other.json"), keyPath); err == nil {
		t.Fatal("loosely protected key was accepted")
	}
}

func replayEntry() agent.CommandReplayEntry {
	now := time.Now().UTC()

	return agent.CommandReplayEntry{
		CommandID: uuid.New().String(), NonceDigest: sha256.Sum256([]byte("nonce")),
		KeyID: "key-1", Audience: uuid.New().String(), IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	}
}
