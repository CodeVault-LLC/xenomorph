package transport

import (
	"testing"
	"time"
)

func TestAgentLogStoreBoundsAndSortsPerAgent(t *testing.T) {
	store := NewAgentLogStore(2)
	base := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	store.Append(AgentLogEntry{AgentID: "agent-1", EventID: "old", ObservedAt: base})
	store.Append(AgentLogEntry{AgentID: "agent-1", EventID: "new", ObservedAt: base.Add(time.Second)})
	store.Append(AgentLogEntry{AgentID: "agent-2", EventID: "other", ObservedAt: base.Add(2 * time.Second)})
	store.Append(AgentLogEntry{AgentID: "agent-1", EventID: "newest", ObservedAt: base.Add(3 * time.Second)})

	entries := store.List("agent-1", 10)
	if len(entries) != 2 {
		t.Fatalf("expected bounded entries, got %d", len(entries))
	}

	if entries[0].EventID != "newest" || entries[1].EventID != "new" {
		t.Fatalf("expected newest entries first, got %#v", entries)
	}

	other := store.List("agent-2", 10)
	if len(other) != 1 || other[0].EventID != "other" {
		t.Fatalf("expected separate per-agent logs, got %#v", other)
	}
}
