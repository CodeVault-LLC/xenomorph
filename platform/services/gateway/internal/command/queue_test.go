package command

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

func newTestQueue(t *testing.T) (*Queue, *rsa.PublicKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	queue, err := NewQueue(privateKey, "test-key")
	if err != nil {
		t.Fatalf("NewQueue() error = %v", err)
	}
	return queue, &privateKey.PublicKey
}

func enqueueTestCommand(t *testing.T, queue *Queue, agentID string, envelope *Envelope) {
	t.Helper()
	if err := queue.Enqueue(agentID, envelope); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
}

func TestEnqueueSignsAndBindsCommand(t *testing.T) {
	t.Parallel()
	queue, publicKey := newTestQueue(t)
	envelope := &Envelope{Type: "support.notice", Reason: "test"}
	enqueueTestCommand(t, queue, "agent-1", envelope)

	got := queue.Dequeue("agent-1")
	if got == nil {
		t.Fatal("Dequeue() = nil, want command")
	}
	if got.CommandID == "" || got.Nonce == "" {
		t.Fatal("Dequeue() command is missing server-authored identifiers")
	}
	if got.AudienceAgentID != "agent-1" {
		t.Fatalf("AudienceAgentID = %q, want agent-1", got.AudienceAgentID)
	}
	if err := commandauth.Verify(*got, publicKey); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestEnqueuePreservesFIFOAndAgentScope(t *testing.T) {
	t.Parallel()
	queue, _ := newTestQueue(t)
	enqueueTestCommand(t, queue, "agent-a", &Envelope{Type: "support.notice", Reason: "first"})
	enqueueTestCommand(t, queue, "agent-a", &Envelope{Type: "support.notice", Reason: "second"})
	enqueueTestCommand(t, queue, "agent-b", &Envelope{Type: "support.notice", Reason: "other"})

	if got := queue.Dequeue("agent-a"); got == nil || got.Reason != "first" {
		t.Fatalf("first Dequeue() = %v, want first command", got)
	}
	if got := queue.Dequeue("agent-a"); got == nil || got.Reason != "second" {
		t.Fatalf("second Dequeue() = %v, want second command", got)
	}
	if got := queue.Dequeue("agent-b"); got == nil || got.Reason != "other" {
		t.Fatalf("agent-b Dequeue() = %v, want scoped command", got)
	}
}

func TestWaitDequeueReturnsEnqueuedCommand(t *testing.T) {
	t.Parallel()
	queue, _ := newTestQueue(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got := make(chan *Envelope, 1)
	go func() { got <- queue.WaitDequeue(ctx, "agent-1") }()
	enqueueTestCommand(t, queue, "agent-1", &Envelope{Type: "support.notice", Reason: "wake"})

	select {
	case envelope := <-got:
		if envelope == nil || envelope.Reason != "wake" {
			t.Fatalf("WaitDequeue() = %v, want wake command", envelope)
		}
	case <-ctx.Done():
		t.Fatal("WaitDequeue() timed out")
	}
}

func TestWaitDequeueReturnsNilOnContextCancel(t *testing.T) {
	t.Parallel()
	queue, _ := newTestQueue(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := queue.WaitDequeue(ctx, "agent-1"); got != nil {
		t.Fatalf("WaitDequeue() = %v, want nil", got)
	}
}
