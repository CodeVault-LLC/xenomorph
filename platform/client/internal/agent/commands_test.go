package agent

import (
	"errors"
	"testing"
	"time"
)

type stubApprover struct {
	approved bool
	err      error
}

func (s stubApprover) Approve(_ CommandEnvelope) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.approved, nil
}

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

func TestHandleCommandWithConsentDeniedDisconnect(t *testing.T) {
	decision, err := HandleCommandWithConsent(validCommand(), stubApprover{approved: false}, true)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != "denied" {
		t.Fatalf("expected denied status, got %q", decision.Result.Status)
	}
	if !decision.DisconnectNow {
		t.Fatal("expected disconnect on deny")
	}
}

func TestHandleCommandWithConsentApproved(t *testing.T) {
	decision, err := HandleCommandWithConsent(validCommand(), stubApprover{approved: true}, true)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != "executed" {
		t.Fatalf("expected executed status, got %q", decision.Result.Status)
	}
	if decision.DisconnectNow {
		t.Fatal("did not expect disconnect when approved")
	}
}

func TestHandleCommandWithConsentRejectsMissingSignature(t *testing.T) {
	cmd := validCommand()
	cmd.Signature = ""

	decision, err := HandleCommandWithConsent(cmd, stubApprover{approved: true}, true)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", decision.Result.Status)
	}
}

func TestHandleCommandWithConsentPromptFailure(t *testing.T) {
	decision, err := HandleCommandWithConsent(validCommand(), stubApprover{err: errors.New("prompt failed")}, true)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if decision.Result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", decision.Result.Status)
	}
	if !decision.DisconnectNow {
		t.Fatal("expected disconnect when prompt fails")
	}
}
