package transport

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

type commandResultTestSigner struct{}

func (commandResultTestSigner) KeyID() string { return "test-key" }

func (commandResultTestSigner) SignCommand(envelope *command.Envelope) error {
	envelope.Signature = "test-signature"
	return nil
}

func TestCanonicalCommandResultIsTransportIndependent(t *testing.T) {
	t.Parallel()

	commandID := "5f9ee36a-80c2-4f32-9257-975b00236f98"
	httpRequest := commandResultRequest{
		CommandID: commandID, Type: "terminal.run", Status: commandResultStatusExecuted, Reason: "completed",
		ClientHostname: "agent-host", OutputData: []byte("result"), TerminalCommand: "untrusted echo",
		TerminalSessionID: "session", TerminalShell: "bash", TerminalWorkingDirectory: "/tmp", TerminalExitCode: 0,
	}
	quicRequest := commandResultFromWire(commandID, wire.CommandResult{
		CommandType: "terminal.run", State: uint64(wire.CommandResultStateExecuted), ReasonText: "completed",
		Hostname: "agent-host", Result: []byte("result"), TerminalSessionID: "session",
		TerminalShell: "bash", TerminalWorkingDirectory: "/tmp", TerminalExitCode: 0,
	})

	httpCanonical, err := canonicalizeCommandResult(httpRequest)
	if err != nil {
		t.Fatalf("canonicalize HTTP result: %v", err)
	}

	quicCanonical, err := canonicalizeCommandResult(quicRequest)
	if err != nil {
		t.Fatalf("canonicalize QUIC result: %v", err)
	}

	if !bytes.Equal(httpCanonical, quicCanonical) {
		t.Fatalf("transport canonical results differ:\nHTTP %s\nQUIC %s", httpCanonical, quicCanonical)
	}
}

func TestHTTPCommandResultCommitsDurableState(t *testing.T) { //nolint:cyclop // One test owns the ordered commit, duplicate, conflict, and audience matrix.
	t.Parallel()

	queue, err := command.NewDurableQueueWithSigner(
		commandResultTestSigner{}, filepath.Join(t.TempDir(), "commands.json"),
	)
	if err != nil {
		t.Fatalf("create durable command queue: %v", err)
	}

	envelope := &command.Envelope{Type: "terminal.run"}
	if err := queue.Enqueue("agent-1", envelope); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}

	if dispatched := queue.Dequeue("agent-1"); dispatched == nil {
		t.Fatal("dispatch command returned nil")
	}

	server := &Server{commandQueue: queue}
	request := commandResultRequest{
		CommandID: envelope.CommandID, Type: "terminal.run", Status: commandResultStatusExecuted, OutputData: []byte("result"),
	}

	disposition, eventID, err := server.commitHTTPCommandResult("agent-1", request)
	if err != nil || disposition != command.ResultCommitted || eventID == "" {
		t.Fatalf("first result = (%d, %q, %v), want committed event", disposition, eventID, err)
	}

	duplicate, duplicateEventID, err := server.commitHTTPCommandResult("agent-1", request)
	if err != nil || duplicate != command.ResultDuplicate || duplicateEventID != eventID {
		t.Fatalf("duplicate result = (%d, %q, %v), want duplicate %q", duplicate, duplicateEventID, err, eventID)
	}

	conflict := request
	conflict.Status = commandResultStatusRejected

	if _, _, err := server.commitHTTPCommandResult("agent-1", conflict); err == nil {
		t.Fatal("conflicting terminal result accepted")
	}

	if _, _, err := server.commitHTTPCommandResult("agent-2", request); err == nil {
		t.Fatal("cross-agent terminal result accepted")
	}
}

func TestCanonicalCommandResultRejectsUnregisteredStatus(t *testing.T) {
	t.Parallel()

	_, err := canonicalizeCommandResult(commandResultRequest{
		CommandID: "5f9ee36a-80c2-4f32-9257-975b00236f98", Type: "support.notice", Status: "complete",
	})
	if err == nil {
		t.Fatal("unregistered command result status accepted")
	}
}
