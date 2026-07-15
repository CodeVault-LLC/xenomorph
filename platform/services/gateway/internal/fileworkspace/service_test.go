package fileworkspace

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func newTestService(t *testing.T) (*Service, *command.Queue) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}

	queue, err := command.NewQueue(privateKey, "test-key")
	if err != nil {
		t.Fatalf("command.NewQueue() error = %v", err)
	}

	store, err := NewStore(filepath.Join(t.TempDir(), "operations.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	service, err := NewService(queue, store)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	return service, queue
}

func TestAuditChainSurvivesReloadAndDetectsTampering(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "operations.json")

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	operation, err := store.Create(Operation{CommandID: "command-1", AgentID: "agent-1", OperatorID: "operator-1", RootID: "root-1", Type: fileprotocol.CommandDirectoryList, ExpiresAt: time.Now().UTC().Add(time.Minute)})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := store.Complete("agent-1", operation.CommandID, "executed", json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if _, err := NewStore(path); err != nil {
		t.Fatalf("NewStore() reload error = %v", err)
	}

	auditPath := filepath.Join(directory, "file-audit.jsonl")
	// #nosec G304 -- auditPath is derived exclusively from t.TempDir().
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	data[len(data)/2] ^= 1
	// #nosec G703 -- the isolated test intentionally tampers with its own audit fixture.
	if err := os.WriteFile(auditPath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if _, err := NewStore(path); err == nil {
		t.Fatal("NewStore() tampered audit error = nil, want error")
	}
}

func TestDispatchPersistsBeforeSignedCommand(t *testing.T) {
	t.Parallel()
	service, queue := newTestService(t)
	request := &fileprotocol.DirectoryListRequest{RelativePath: "documents", PageSize: 25}

	operation, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandDirectoryList, "trace-1", request)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	if operation.CommandID == "" || operation.State != StateQueued {
		t.Fatalf("Dispatch() operation = %+v, want durable queued command", operation)
	}

	envelope := queue.Dequeue("agent-1")
	if envelope == nil || envelope.CommandID != operation.CommandID || envelope.AudienceAgentID != "agent-1" {
		t.Fatalf("Dequeue() = %+v, want bound signed command", envelope)
	}

	var dispatched fileprotocol.DirectoryListRequest
	if err := json.Unmarshal(envelope.Payload, &dispatched); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if dispatched.RootID != "root-1" || dispatched.ProtocolVersion != fileprotocol.Version {
		t.Fatalf("dispatched request = %+v, want selected root and protocol version", dispatched)
	}
}

func TestDispatchBoundsDirectorySearch(t *testing.T) {
	t.Parallel()
	service, queue := newTestService(t)
	request := &fileprotocol.DirectorySearchRequest{
		RelativePath: "documents", Query: "report", MaxResults: 25, MaxEntries: 1_000, MaxDepth: 8,
	}

	operation, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandDirectorySearch, "trace-1", request)
	if err != nil {
		t.Fatalf("Dispatch() search error = %v", err)
	}

	if operation.Type != fileprotocol.CommandDirectorySearch || queue.Dequeue("agent-1") == nil {
		t.Fatalf("Dispatch() operation = %+v, want queued search", operation)
	}

	invalid := []fileprotocol.DirectorySearchRequest{
		{Query: "x", MaxResults: 25, MaxEntries: 1_000, MaxDepth: 8},
		{Query: "report", MaxResults: 251, MaxEntries: 1_000, MaxDepth: 8},
		{Query: "report", MaxResults: 25, MaxEntries: 10_001, MaxDepth: 8},
		{RelativePath: "../escape", Query: "report", MaxResults: 25, MaxEntries: 1_000, MaxDepth: 8},
	}
	for _, candidate := range invalid {
		if _, dispatchErr := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandDirectorySearch, "trace-1", &candidate); dispatchErr == nil {
			t.Fatalf("Dispatch() accepted invalid search %+v", candidate)
		}
	}
}

func TestDispatchBoundsMetadataDelta(t *testing.T) {
	t.Parallel()
	service, queue := newTestService(t)
	mode := uint32(0o640)
	modified := time.Now().UTC().Add(-time.Hour)
	request := &fileprotocol.MetadataSetRequest{RelativePath: "reports/result.txt", Delta: fileprotocol.MetadataDelta{POSIXMode: &mode, ModifiedAt: &modified}}

	operation, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandMetadataSet, "trace-1", request)
	if err != nil {
		t.Fatal(err)
	}

	if request.ProtocolVersion != fileprotocol.Version || request.OperationID != operation.OperationID || queue.Dequeue("agent-1") == nil {
		t.Fatalf("prepared metadata request = %+v, want bound queued command", request)
	}

	empty := &fileprotocol.MetadataSetRequest{RelativePath: "result.txt"}
	if _, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandMetadataSet, "trace-1", empty); err == nil {
		t.Fatal("Dispatch() error = nil, want empty delta rejection")
	}

	tooWide := uint32(0o10000)

	empty.Delta.POSIXMode = &tooWide
	if _, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandMetadataSet, "trace-1", empty); err == nil {
		t.Fatal("Dispatch() error = nil, want mode bound rejection")
	}
}

func TestDispatchAuthorsArchiveSafetyLimits(t *testing.T) {
	t.Parallel()
	service, queue := newTestService(t)
	request := &fileprotocol.ArchiveRequest{
		Action: fileprotocol.ArchiveCreate, Format: fileprotocol.ArchiveZIP,
		ArchivePath: "bundle.zip", SourcePaths: []string{"reports"}, Conflict: fileprotocol.ConflictFail,
	}

	operation, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandArchiveExecute, "trace-1", request)
	if err != nil {
		t.Fatal(err)
	}

	if request.OperationID != operation.OperationID || request.Limits.MaxEntries != maxArchiveEntries || request.Limits.MaxRuntime != maxArchiveRuntime || queue.Dequeue("agent-1") == nil {
		t.Fatalf("prepared archive request = %+v, want fixed signed limits", request)
	}

	request.Format = "tar"
	if _, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandArchiveExecute, "trace-1", request); err == nil {
		t.Fatal("Dispatch() error = nil, want archive format rejection")
	}
}

func TestCompleteEnforcesAuthenticatedAgentScope(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t)

	operation, err := service.Dispatch("agent-1", "operator-1", "root-1", fileprotocol.CommandDirectoryList, "trace-1", &fileprotocol.DirectoryListRequest{PageSize: 10})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	if err := service.Complete("agent-2", operation.CommandID, "executed", json.RawMessage(`{"result":true}`)); err == nil {
		t.Fatal("Complete() error = nil, want cross-agent rejection")
	}

	if err := service.Complete("agent-1", operation.CommandID, "executed", json.RawMessage(`{"result":true}`)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	completed, ok := service.Operation("agent-1", operation.OperationID)
	if !ok || completed.State != StateCompleted {
		t.Fatalf("Operation() = (%+v, %v), want completed", completed, ok)
	}
}

func TestDispatchRejectsInvalidRootAndCommand(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t)

	tests := []struct {
		name        string
		rootID      string
		commandType string
		request     any
	}{
		{name: "invalid root", rootID: "../root", commandType: fileprotocol.CommandDirectoryList, request: &fileprotocol.DirectoryListRequest{PageSize: 10}},
		{name: "unsupported command", rootID: "root-1", commandType: "files.delete", request: &fileprotocol.DirectoryListRequest{PageSize: 10}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Dispatch("agent-1", "operator-1", test.rootID, test.commandType, "trace-1", test.request); err == nil {
				t.Fatal("Dispatch() error = nil, want protocol rejection")
			}
		})
	}
}

func TestProbeRootsDoesNotRequireConfiguredGrants(t *testing.T) {
	t.Parallel()
	service, queue := newTestService(t)

	operation, err := service.ProbeRoots("agent-without-grants", "internal-website", "trace-1")
	if err != nil {
		t.Fatalf("ProbeRoots() error = %v", err)
	}

	envelope := queue.Dequeue("agent-without-grants")
	if envelope == nil || envelope.CommandID != operation.CommandID || envelope.Type != fileprotocol.CommandRootsList {
		t.Fatalf("Dequeue() = %+v, want automatic root discovery command", envelope)
	}

	var request fileprotocol.RootsListRequest
	if err := json.Unmarshal(envelope.Payload, &request); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if request.ProtocolVersion != fileprotocol.Version {
		t.Fatalf("protocol version = %d, want %d", request.ProtocolVersion, fileprotocol.Version)
	}
}

func TestDownloadPreparationFreezesManifestThenDispatchesResume(t *testing.T) {
	t.Parallel()
	service, queue := newTestService(t)

	transfer, err := service.CreateTransfer("agent-1", "operator-1", "trace-1", fileprotocol.TransferManifest{
		Direction: fileprotocol.TransferDownload, RootID: "root-1", RelativePath: "report.bin",
		ChunkSize: defaultChunkSize, Conflict: fileprotocol.ConflictFail,
	})
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}

	prepare := queue.Dequeue("agent-1")
	if prepare == nil || prepare.Type != fileprotocol.CommandTransferPrepare {
		t.Fatalf("prepare command = %+v", prepare)
	}

	data := []byte("report")
	manifest := testManifest(fileprotocol.TransferDownload, data)
	manifest.TransferID, manifest.RootID, manifest.RelativePath = transfer.TransferID, "root-1", "report.bin"

	resultData, err := json.Marshal(fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: transfer.TransferID, State: "prepared", Manifest: &manifest, Scanning: "not_scanned"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	envelope, err := json.Marshal(fileprotocol.CommandResult{ProtocolVersion: fileprotocol.Version, Data: resultData})
	if err != nil {
		t.Fatalf("Marshal() envelope error = %v", err)
	}

	if err := service.Complete("agent-1", prepare.CommandID, "executed", envelope); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	resume := queue.Dequeue("agent-1")
	if resume == nil || resume.Type != fileprotocol.CommandTransferResume {
		t.Fatalf("resume command = %+v", resume)
	}

	stored, ok := service.Transfer("agent-1", transfer.TransferID)
	assertStoredManifest(t, stored, ok, manifest.SHA256)
}

func TestRemoveTransferWritesAuditBeforeCleanup(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t)
	data := []byte("audited cleanup")

	transfer := createCompletedDownloadTransfer(t, service.transfers, "agent-1", data)
	if err := service.RemoveTransfer("agent-1", "operator-2", "trace-remove", transfer.TransferID); err != nil {
		t.Fatalf("RemoveTransfer() error = %v", err)
	}

	audit, err := os.ReadFile(service.store.auditPath) // #nosec G304 -- the path is an isolated service fixture.
	if err != nil {
		t.Fatalf("os.ReadFile() audit error = %v", err)
	}

	if !strings.Contains(string(audit), "transfer_removal_requested") || !strings.Contains(string(audit), "trace-remove") {
		t.Fatalf("audit = %q, want transfer removal event and trace", audit)
	}
}

func TestValidateOperatorRelativePathRejectsTraversalAndInvalidEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "absolute", value: "/etc/passwd"},
		{name: "parent traversal", value: "documents/../secret"},
		{name: "current directory", value: "documents/./report"},
		{name: "windows separator", value: `documents\report`},
		{name: "empty component", value: "documents//report"},
		{name: "line break", value: "documents/report\nname"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := validateOperatorRelativePath(test.value); err == nil {
				t.Fatalf("validateOperatorRelativePath(%q) error = nil, want rejection", test.value)
			}
		})
	}
}

func TestCreateTransferRejectsOperatorPathTraversal(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t)

	_, err := service.CreateTransfer("agent-1", "operator-1", "trace-1", fileprotocol.TransferManifest{
		Direction: fileprotocol.TransferDownload, RootID: "root-1", RelativePath: "../secret",
		ChunkSize: defaultChunkSize, Conflict: fileprotocol.ConflictFail,
	})
	if err == nil {
		t.Fatal("CreateTransfer() error = nil, want path traversal rejection")
	}
}

func assertStoredManifest(t *testing.T, transfer Transfer, ok bool, expectedDigest string) {
	t.Helper()

	if !ok || transfer.Manifest.SHA256 != expectedDigest {
		t.Fatalf("stored transfer = %+v", transfer)
	}
}
