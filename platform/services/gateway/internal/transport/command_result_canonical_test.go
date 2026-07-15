package transport

import (
	"bytes"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

func TestCanonicalCommandResultIsTransportIndependent(t *testing.T) {
	t.Parallel()

	commandID := "5f9ee36a-80c2-4f32-9257-975b00236f98"
	structuredRequest := commandResultRequest{
		CommandID: commandID, Type: "terminal.run", Status: commandResultStatusExecuted, Reason: "completed",
		ClientHostname: "agent-host", OutputData: []byte("result"), TerminalCommand: "untrusted echo",
		TerminalSessionID: "session", TerminalShell: "bash", TerminalWorkingDirectory: "/tmp", TerminalExitCode: 0,
	}
	quicRequest := commandResultFromWire(commandID, wire.CommandResult{
		CommandType: "terminal.run", State: uint64(wire.CommandResultStateExecuted), ReasonText: "completed",
		Hostname: "agent-host", Result: []byte("result"), TerminalSessionID: "session",
		TerminalShell: "bash", TerminalWorkingDirectory: "/tmp", TerminalExitCode: 0,
	})

	structuredCanonical, err := canonicalizeCommandResult(structuredRequest)
	if err != nil {
		t.Fatalf("canonicalize structured result: %v", err)
	}

	quicCanonical, err := canonicalizeCommandResult(quicRequest)
	if err != nil {
		t.Fatalf("canonicalize QUIC result: %v", err)
	}

	if !bytes.Equal(structuredCanonical, quicCanonical) {
		t.Fatalf("canonical results differ:\nstructured %s\nQUIC %s", structuredCanonical, quicCanonical)
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
