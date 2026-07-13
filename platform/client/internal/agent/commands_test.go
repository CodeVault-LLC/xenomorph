package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validCommand() CommandEnvelope {
	now := time.Now().UTC()
	return CommandEnvelope{
		CommandID:   "cmd-1",
		Type:        CommandTypeNotice,
		RequestedBy: "ops-user",
		IssuedAt:    now,
		ExpiresAt:   now.Add(10 * time.Minute),
		Reason:      "display message",
		Signature:   "sig",
	}
}

func TestHandleCommandExecutes(t *testing.T) {
	decision, err := HandleCommand(validCommand())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != CommandStatusExecuted {
		t.Fatalf("expected executed status, got %q", decision.Result.Status)
	}
}

func TestHandleCommandRejectsMissingSignature(t *testing.T) {
	cmd := validCommand()
	cmd.Signature = ""

	decision, err := HandleCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != CommandStatusRejected {
		t.Fatalf("expected rejected status, got %q", decision.Result.Status)
	}
}

func TestHandleCommandRejectsUnknownType(t *testing.T) {
	cmd := validCommand()
	cmd.Type = CommandType("unknown.type")

	decision, err := HandleCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != CommandStatusRejected {
		t.Fatalf("expected rejected status, got %q", decision.Result.Status)
	}
}

func TestHandleCommandRejectsExpiredCommand(t *testing.T) {
	cmd := validCommand()
	cmd.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour)

	decision, err := HandleCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", decision.Result.Status)
	}
}

func TestHandleCommandAllowsScreenStreamControls(t *testing.T) {
	tests := []struct {
		name string
		typ  CommandType
	}{
		{name: "start screen stream", typ: CommandTypeStartScreenStream},
		{name: "stop screen stream", typ: CommandTypeStopScreenStream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := validCommand()
			cmd.Type = tt.typ

			decision, err := HandleCommand(cmd)
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if decision.Result.Status != CommandStatusExecuted {
				t.Fatalf("expected executed status, got %q", decision.Result.Status)
			}
		})
	}
}

func TestHandleCommandRunsTerminalCommand(t *testing.T) {
	cmd := validCommand()
	cmd.Type = CommandTypeTerminalRun
	cmd.Payload = terminalPayload(t, terminalCommandPayload{
		SessionID: "session-1",
		Command:   "printf terminal-ok",
		Shell:     "sh",
	})

	decision, err := HandleCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != CommandStatusExecuted {
		t.Fatalf("expected executed status, got %q", decision.Result.Status)
	}
	if decision.Result.TerminalSessionID != "session-1" {
		t.Fatalf("expected terminal session id, got %q", decision.Result.TerminalSessionID)
	}
	if decision.Result.TerminalExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", decision.Result.TerminalExitCode)
	}
	if string(decision.Result.OutputData) != "terminal-ok" {
		t.Fatalf("expected command output, got %q", string(decision.Result.OutputData))
	}
}

func TestHandleCommandTracksTerminalCD(t *testing.T) {
	dir := t.TempDir()
	cmd := validCommand()
	cmd.Type = "support.terminal.run"
	cmd.Payload = terminalPayload(t, terminalCommandPayload{
		SessionID: "session-cd",
		Command:   "cd " + dir,
		Shell:     "sh",
	})

	decision, err := HandleCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.TerminalWorkingDirectory != dir {
		t.Fatalf("expected cwd %q, got %q", dir, decision.Result.TerminalWorkingDirectory)
	}

	second := validCommand()
	second.Type = CommandTypeTerminalRun
	second.Payload = terminalPayload(t, terminalCommandPayload{
		SessionID: "session-cd",
		Command:   "pwd",
		Shell:     "sh",
	})
	decision, err = HandleCommand(second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := strings.TrimSpace(string(decision.Result.OutputData)); got != dir {
		t.Fatalf("expected pwd %q, got %q", dir, got)
	}
}

func terminalPayload(t *testing.T, payload terminalCommandPayload) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
