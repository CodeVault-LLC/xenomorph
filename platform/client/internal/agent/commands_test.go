package agent

import (
	"testing"
	"time"
)

func validCommand() CommandEnvelope {
	now := time.Now().UTC()
	return CommandEnvelope{
		CommandID:   "cmd-1",
		Type:        "support.notice",
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
	if decision.Result.Status != "executed" {
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
	if decision.Result.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", decision.Result.Status)
	}
}

func TestHandleCommandRejectsUnknownType(t *testing.T) {
	cmd := validCommand()
	cmd.Type = "unknown.type"

	decision, err := HandleCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != "rejected" {
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
