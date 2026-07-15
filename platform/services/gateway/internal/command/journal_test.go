package command

import (
	"crypto/rand"
	"crypto/rsa"
	"path/filepath"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

func TestDurableQueueRecoversQueuedAndFencesDispatched(t *testing.T) { //nolint:cyclop // One test owns the ordered crash-state matrix.
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	keyID, err := commandauth.KeyID(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("derive key ID: %v", err)
	}
	path := filepath.Join(t.TempDir(), "commands.json")
	queue, err := NewDurableQueueWithSigner(&rsaSigner{privateKey: privateKey, keyID: keyID}, path)
	if err != nil {
		t.Fatalf("create durable queue: %v", err)
	}
	queued := &Envelope{Type: "support.notice", Reason: "queued"}
	dispatched := &Envelope{Type: "support.notice", Reason: "dispatched"}
	if err := queue.Enqueue("agent-1", queued); err != nil {
		t.Fatalf("enqueue queued command: %v", err)
	}
	if err := queue.Enqueue("agent-2", dispatched); err != nil {
		t.Fatalf("enqueue dispatched command: %v", err)
	}
	if command := queue.Dequeue("agent-2"); command == nil {
		t.Fatal("dispatch command returned nil")
	}

	recovered, err := NewDurableQueueWithSigner(&rsaSigner{privateKey: privateKey, keyID: keyID}, path)
	if err != nil {
		t.Fatalf("recover durable queue: %v", err)
	}
	if command := recovered.Dequeue("agent-1"); command == nil || command.CommandID != queued.CommandID {
		t.Fatalf("recovered command = %v, want queued command", command)
	}
	if command := recovered.Dequeue("agent-2"); command != nil {
		t.Fatalf("ambiguous command was redispatched: %v", command)
	}
	if state, ok := recovered.journal.State("agent-2", dispatched.CommandID); !ok || state != JournalOutcomeUnknown {
		t.Fatalf("recovered state = (%q, %t), want outcome_unknown", state, ok)
	}
}

func TestJournalCommitsIdempotentResultAndRejectsConflict(t *testing.T) {
	t.Parallel()

	queue, _ := newTestQueue(t)
	path := filepath.Join(t.TempDir(), "commands.json")
	durable, err := NewDurableQueueWithSigner(queue.signer, path)
	if err != nil {
		t.Fatalf("create durable queue: %v", err)
	}
	envelope := &Envelope{Type: "support.notice"}
	if err := durable.Enqueue("agent-1", envelope); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if command := durable.Dequeue("agent-1"); command == nil {
		t.Fatal("dispatch command returned nil")
	}
	result := []byte("canonical-result")
	if disposition, err := durable.CommitResult("agent-1", envelope.CommandID, result); err != nil || disposition != ResultCommitted {
		t.Fatalf("first result = (%d, %v), want committed", disposition, err)
	}
	if disposition, err := durable.CommitResult("agent-1", envelope.CommandID, result); err != nil || disposition != ResultDuplicate {
		t.Fatalf("duplicate result = (%d, %v), want duplicate", disposition, err)
	}
	if _, err := durable.CommitResult("agent-1", envelope.CommandID, []byte("different")); err == nil {
		t.Fatal("conflicting terminal result accepted")
	}
	if _, err := durable.CommitResult("agent-2", envelope.CommandID, result); err == nil {
		t.Fatal("cross-agent terminal result accepted")
	}
}
