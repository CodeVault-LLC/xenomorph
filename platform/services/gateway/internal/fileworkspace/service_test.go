package fileworkspace

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"os"
	"path/filepath"
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
