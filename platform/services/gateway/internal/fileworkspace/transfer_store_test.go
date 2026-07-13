package fileworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestTransferStoreSerializesEmptyAcknowledgementsAsArray(t *testing.T) {
	t.Parallel()
	store, err := newTransferStore(filepath.Join(t.TempDir(), "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() error = %v", err)
	}
	transfer, _, err := store.CreateTransfer("agent-1", "operator-1", testManifest(fileprotocol.TransferUpload, []byte("pending")))
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}
	data, err := json.Marshal(transfer)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"acknowledged_chunks":[]`) {
		t.Fatalf("transfer JSON = %s, want non-null acknowledgement array", data)
	}
}

func TestTransferStorePersistsEncryptedVerifiedChunks(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store, err := newTransferStore(filepath.Join(directory, "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() error = %v", err)
	}
	data := []byte("durable transfer")
	manifest := testManifest(fileprotocol.TransferUpload, data)
	transfer, _, err := store.CreateTransfer("agent-1", "operator-1", manifest)
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}
	if _, err := store.StageBrowserChunk("agent-1", transfer.TransferID, 0, data); err != nil {
		t.Fatalf("StageBrowserChunk() error = %v", err)
	}
	stagedPath := filepath.Join(directory, "file-spool", transfer.TransferID, "000000.chunk")
	// #nosec G304 -- stagedPath is under the isolated test directory.
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(staged) == string(data) {
		t.Fatal("staged chunk is plaintext")
	}
	reloaded, err := newTransferStore(filepath.Join(directory, "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() reload error = %v", err)
	}
	committed, err := reloaded.Finalize("agent-1", transfer.TransferID)
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if committed.State != TransferQueued {
		t.Fatalf("state = %q, want queued", committed.State)
	}
}

func TestTransferStoreRejectsChecksumAndCrossAgentAccess(t *testing.T) {
	t.Parallel()
	store, err := newTransferStore(filepath.Join(t.TempDir(), "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() error = %v", err)
	}
	data := []byte("expected")
	transfer, lease, err := store.CreateTransfer("agent-1", "operator-1", testManifest(fileprotocol.TransferDownload, data))
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}
	if _, err := store.PutChunk("agent-1", transfer.TransferID, lease.Token, 0, []byte("tampered")); err == nil {
		t.Fatal("PutChunk() checksum error = nil")
	}
	if _, err := store.PutChunk("agent-2", transfer.TransferID, lease.Token, 0, data); err == nil {
		t.Fatal("PutChunk() cross-agent error = nil")
	}
	if _, err := store.PutChunk("agent-1", transfer.TransferID, lease.Token, 0, data); err != nil {
		t.Fatalf("PutChunk() error = %v", err)
	}
	if _, err := store.Finalize("agent-1", transfer.TransferID); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
}

func TestTransferStoreRemoveDeletesTerminalTransferAndSpool(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store, err := newTransferStore(filepath.Join(directory, "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() error = %v", err)
	}
	data := []byte("remove me")
	transfer := createCompletedDownloadTransfer(t, store, "agent-1", data)
	if err := store.Remove("agent-2", transfer.TransferID); err == nil {
		t.Fatal("Remove() cross-agent error = nil")
	}
	if err := store.Remove("agent-1", transfer.TransferID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, ok := store.Transfer("agent-1", transfer.TransferID); ok {
		t.Fatal("Transfer() found removed transfer")
	}
	spoolPath := filepath.Join(directory, "file-spool", transfer.TransferID)
	if _, err := os.Stat(spoolPath); !os.IsNotExist(err) {
		t.Fatalf("os.Stat() error = %v, want not exist", err)
	}
	reloaded, err := newTransferStore(filepath.Join(directory, "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() reload error = %v", err)
	}
	if _, ok := reloaded.Transfer("agent-1", transfer.TransferID); ok {
		t.Fatal("reloaded Transfer() found removed transfer")
	}
}

func TestTransferStoreRemoveFinishedRetainsActiveAndOtherAgentTransfers(t *testing.T) {
	t.Parallel()
	store, err := newTransferStore(filepath.Join(t.TempDir(), "operations.json"))
	if err != nil {
		t.Fatalf("newTransferStore() error = %v", err)
	}
	data := []byte("finished")
	createCompletedDownloadTransfer(t, store, "agent-1", data)
	active, _, err := store.CreateTransfer("agent-1", "operator-1", testManifest(fileprotocol.TransferUpload, data))
	if err != nil {
		t.Fatalf("CreateTransfer() active error = %v", err)
	}
	otherAgent, _, err := store.CreateTransfer("agent-2", "operator-1", testManifest(fileprotocol.TransferDownload, data))
	if err != nil {
		t.Fatalf("CreateTransfer() other agent error = %v", err)
	}
	removed, err := store.RemoveFinished("agent-1")
	if err != nil {
		t.Fatalf("RemoveFinished() error = %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, ok := store.Transfer("agent-1", active.TransferID); !ok {
		t.Fatal("active transfer was removed")
	}
	if _, ok := store.Transfer("agent-2", otherAgent.TransferID); !ok {
		t.Fatal("other-agent transfer was removed")
	}
}

func createCompletedDownloadTransfer(t *testing.T, store *TransferStore, agentID string, data []byte) Transfer {
	t.Helper()
	transfer, lease, err := store.CreateTransfer(agentID, "operator-1", testManifest(fileprotocol.TransferDownload, data))
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}
	if _, err := store.PutChunk(agentID, transfer.TransferID, lease.Token, 0, data); err != nil {
		t.Fatalf("PutChunk() error = %v", err)
	}
	completed, err := store.Finalize(agentID, transfer.TransferID)
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	return completed
}

func testManifest(direction fileprotocol.TransferDirection, data []byte) fileprotocol.TransferManifest {
	digest := sha256.Sum256(data)
	encoded := hex.EncodeToString(digest[:])
	return fileprotocol.TransferManifest{
		Direction: direction, RootID: "root-1", RelativePath: "file.bin",
		Size: int64(len(data)), ChunkSize: defaultChunkSize, SHA256: encoded,
		Chunks:   []fileprotocol.ChunkManifest{{Index: 0, Size: int64(len(data)), SHA256: encoded}},
		Conflict: fileprotocol.ConflictFail,
	}
}
