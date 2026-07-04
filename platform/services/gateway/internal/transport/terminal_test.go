package transport

import (
	"testing"
	"time"
)

func TestTerminalStoreCreatesAndCompletesEntry(t *testing.T) {
	store := NewTerminalStore()
	session := store.CreateSession("agent-1", "Ops", "bash", "/tmp")
	if session.SessionID == "" {
		t.Fatal("expected generated session id")
	}

	store.AppendQueued(TerminalEntry{
		AgentID:          "agent-1",
		SessionID:        session.SessionID,
		CommandID:        "cmd-1",
		Command:          "pwd",
		Shell:            "bash",
		WorkingDirectory: "/tmp",
		Status:           "queued",
		SubmittedAt:      time.Now().UTC(),
	})

	ok := store.Complete("agent-1", "cmd-1", TerminalEntry{
		Status:           "executed",
		ExitCode:         0,
		OutputLog:        "/tmp\n",
		Reason:           "terminal command completed",
		Shell:            "bash",
		WorkingDirectory: "/tmp",
	})
	if !ok {
		t.Fatal("expected terminal entry completion")
	}

	entries := store.ListEntries("agent-1", session.SessionID, 10)
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if entries[0].Status != "executed" || entries[0].OutputLog != "/tmp\n" {
		t.Fatalf("expected completed entry, got %#v", entries[0])
	}
	if entries[0].CompletedAt == nil {
		t.Fatal("expected completed_at timestamp")
	}
}

func TestTerminalStoreBoundsSessionsPerAgent(t *testing.T) {
	store := NewTerminalStore()
	for i := 0; i < maxTerminalSessionsPerAgent+1; i++ {
		store.CreateSession("agent-1", "session", "bash", "")
	}

	sessions := store.ListSessions("agent-1")
	if len(sessions) != maxTerminalSessionsPerAgent {
		t.Fatalf("expected bounded sessions, got %d", len(sessions))
	}
}

func TestTerminalStoreDeletesSessionAndEntries(t *testing.T) {
	store := NewTerminalStore()
	session := store.CreateSession("agent-1", "Ops", "bash", "")
	store.AppendQueued(TerminalEntry{
		AgentID:   "agent-1",
		SessionID: session.SessionID,
		CommandID: "cmd-1",
		Command:   "whoami",
		Status:    "queued",
	})

	if !store.DeleteSession("agent-1", session.SessionID) {
		t.Fatal("expected session delete")
	}
	if _, ok := store.Session("agent-1", session.SessionID); ok {
		t.Fatal("expected deleted session to be absent")
	}
	if entries := store.ListEntries("agent-1", session.SessionID, 10); len(entries) != 0 {
		t.Fatalf("expected deleted entries to be absent, got %#v", entries)
	}
}
