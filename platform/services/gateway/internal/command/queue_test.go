package command

import (
	"testing"
)

func TestEnqueueDequeue(t *testing.T) {
	q := NewQueue()

	cmd := &Envelope{Type: "support.notice", Reason: "test"}
	q.Enqueue("agent-1", cmd)

	got := q.Dequeue("agent-1")
	if got == nil {
		t.Fatal("expected command, got nil")
	}
	if got.Type != "support.notice" {
		t.Fatalf("expected type support.notice, got %q", got.Type)
	}
	if got.CommandID == "" {
		t.Fatal("expected command_id to be auto-generated")
	}

	empty := q.Dequeue("agent-1")
	if empty != nil {
		t.Fatal("expected nil for empty queue")
	}
}

func TestEnqueueMultiple(t *testing.T) {
	q := NewQueue()

	q.Enqueue("agent-1", &Envelope{Type: "support.notice", Reason: "first"})
	q.Enqueue("agent-1", &Envelope{Type: "support.request_screenshot", Reason: "second"})

	first := q.Dequeue("agent-1")
	if first == nil || first.Reason != "first" {
		t.Fatalf("expected first command, got %v", first)
	}

	second := q.Dequeue("agent-1")
	if second == nil || second.Reason != "second" {
		t.Fatalf("expected second command, got %v", second)
	}
}

func TestDequeueDifferentAgents(t *testing.T) {
	q := NewQueue()

	q.Enqueue("agent-a", &Envelope{Type: "support.notice"})
	q.Enqueue("agent-b", &Envelope{Type: "support.request_screenshot"})

	if cmd := q.Dequeue("agent-a"); cmd == nil || cmd.Type != "support.notice" {
		t.Fatal("expected agent-a command")
	}
	if cmd := q.Dequeue("agent-b"); cmd == nil || cmd.Type != "support.request_screenshot" {
		t.Fatal("expected agent-b command")
	}
	if cmd := q.Dequeue("agent-a"); cmd != nil {
		t.Fatal("expected nil for agent-a after dequeue")
	}
}
