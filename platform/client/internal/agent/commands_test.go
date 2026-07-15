package agent

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

func signedTestCommand(t *testing.T, commandType CommandType, payload json.RawMessage) (CommandEnvelope, *CommandValidator) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	keyID, err := commandauth.KeyID(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("KeyID() error = %v", err)
	}
	now := time.Now().UTC()
	cmd := CommandEnvelope{
		ProtocolVersion: commandauth.ProtocolVersion,
		CommandID:       "cmd-1",
		AudienceAgentID: "agent-1",
		Type:            commandType,
		Payload:         payload,
		RequestedBy:     "ops-user",
		IssuedAt:        now,
		ExpiresAt:       now.Add(time.Minute),
		Nonce:           "nonce-1",
		Reason:          "authorized support operation",
		KeyID:           keyID,
	}
	authEnvelope := toAuthEnvelope(cmd)
	if err := commandauth.Sign(&authEnvelope, privateKey); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	cmd.Signature = authEnvelope.Signature
	validator, err := NewCommandValidator(&privateKey.PublicKey, keyID, "agent-1")
	if err != nil {
		t.Fatalf("NewCommandValidator() error = %v", err)
	}
	return cmd, validator
}

func TestHandleCommandExecutesAuthenticCommand(t *testing.T) {
	cmd, validator := signedTestCommand(t, CommandTypeNotice, nil)
	decision, err := HandleCommand(cmd, validator)
	if err != nil {
		t.Fatalf("HandleCommand() error = %v", err)
	}
	if decision.Result.Status != CommandStatusExecuted {
		t.Fatalf("status = %q, want executed", decision.Result.Status)
	}
}

func TestHandleCommandRejectsInvalidCommands(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CommandEnvelope)
	}{
		{name: "missing signature", mutate: func(cmd *CommandEnvelope) { cmd.Signature = "" }},
		{name: "unknown type", mutate: func(cmd *CommandEnvelope) { cmd.Type = "unknown.type" }},
		{name: "expired", mutate: func(cmd *CommandEnvelope) { cmd.ExpiresAt = time.Now().UTC().Add(-time.Hour) }},
		{name: "cross agent", mutate: func(cmd *CommandEnvelope) { cmd.AudienceAgentID = "agent-2" }},
		{name: "forged payload", mutate: func(cmd *CommandEnvelope) { cmd.Payload = json.RawMessage(`{"forged":true}`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd, validator := signedTestCommand(t, CommandTypeNotice, nil)
			test.mutate(&cmd)
			decision, err := HandleCommand(cmd, validator)
			if err != nil {
				t.Fatalf("HandleCommand() error = %v", err)
			}
			if decision.Result.Status != CommandStatusRejected {
				t.Fatalf("status = %q, want rejected", decision.Result.Status)
			}
		})
	}
}

func TestHandleCommandRejectsReplay(t *testing.T) {
	cmd, validator := signedTestCommand(t, CommandTypeNotice, nil)
	if first, err := HandleCommand(cmd, validator); err != nil || first.Result.Status != CommandStatusExecuted {
		t.Fatalf("first HandleCommand() = (%q, %v), want executed", first.Result.Status, err)
	}
	second, err := HandleCommand(cmd, validator)
	if err != nil {
		t.Fatalf("second HandleCommand() error = %v", err)
	}
	if second.Result.Status != CommandStatusRejected || second.Result.Reason != ErrCommandReplay.Error() {
		t.Fatalf("second result = (%q, %q), want replay rejection", second.Result.Status, second.Result.Reason)
	}
}

func TestHandleCommandAllowsScreenStreamControls(t *testing.T) {
	tests := []CommandType{CommandTypeStartScreenStream, CommandTypeStopScreenStream}
	for _, commandType := range tests {
		t.Run(string(commandType), func(t *testing.T) {
			cmd, validator := signedTestCommand(t, commandType, nil)
			decision, err := HandleCommand(cmd, validator)
			if err != nil || decision.Result.Status != CommandStatusExecuted {
				t.Fatalf("HandleCommand() = (%q, %v), want executed", decision.Result.Status, err)
			}
		})
	}
}

func TestHandleCommandRunsTerminalCommand(t *testing.T) {
	payload := terminalPayload(t, terminalCommandPayload{SessionID: "session-1", Command: "printf terminal-ok", Shell: "sh"})
	cmd, validator := signedTestCommand(t, CommandTypeTerminalRun, payload)
	decision, err := HandleCommand(cmd, validator)
	if err != nil {
		t.Fatalf("HandleCommand() error = %v", err)
	}
	if decision.Result.TerminalExitCode != 0 || string(decision.Result.OutputData) != "terminal-ok" {
		t.Fatalf("terminal result = (%d, %q), want (0, terminal-ok)", decision.Result.TerminalExitCode, decision.Result.OutputData)
	}
}

func TestHandleCommandTracksTerminalCD(t *testing.T) {
	dir := t.TempDir()
	firstPayload := terminalPayload(t, terminalCommandPayload{SessionID: "session-cd", Command: "cd " + dir, Shell: "sh"})
	first, firstValidator := signedTestCommand(t, CommandTypeTerminalRun, firstPayload)
	decision, err := HandleCommand(first, firstValidator)
	if err != nil || decision.Result.TerminalWorkingDirectory != dir {
		t.Fatalf("first HandleCommand() = (%q, %v), want working directory %q", decision.Result.TerminalWorkingDirectory, err, dir)
	}

	secondPayload := terminalPayload(t, terminalCommandPayload{SessionID: "session-cd", Command: "pwd", Shell: "sh"})
	second, secondValidator := signedTestCommand(t, CommandTypeTerminalRun, secondPayload)
	decision, err = HandleCommand(second, secondValidator)
	if err != nil {
		t.Fatalf("second HandleCommand() error = %v", err)
	}
	if got := strings.TrimSpace(string(decision.Result.OutputData)); got != dir {
		t.Fatalf("pwd output = %q, want %q", got, dir)
	}
}

func terminalPayload(t *testing.T, payload terminalCommandPayload) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
