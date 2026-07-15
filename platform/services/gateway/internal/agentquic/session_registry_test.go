package agentquic

import "testing"

func TestSessionRegistryFencesReplacementBeforeCommit(t *testing.T) {
	t.Parallel()

	registry := newSessionRegistry(4, &Metrics{})
	first := &session{agentID: "agent-1"}

	if previous, err := registry.install(first); err != nil || previous != nil {
		t.Fatalf("install first = (%v, %v), want no previous", previous, err)
	}

	release, ok := registry.beginCommit(first)
	if !ok {
		t.Fatal("current first session could not begin commit")
	}

	release()

	second := &session{agentID: "agent-1"}

	previous, err := registry.install(second)
	if err != nil || previous != first {
		t.Fatalf("install replacement = (%v, %v), want first", previous, err)
	}

	if !first.fenced.Load() {
		t.Fatal("old session was not fenced")
	}

	if release, ok := registry.beginCommit(first); ok {
		release()
		t.Fatal("fenced session began a commit")
	}

	if release, ok := registry.beginCommit(second); !ok {
		t.Fatal("replacement session could not begin commit")
	} else {
		release()
	}
}

func TestSessionRegistryEvictsOnlyInactiveEntries(t *testing.T) {
	t.Parallel()

	registry := newSessionRegistry(1, &Metrics{})
	first := &session{agentID: "agent-1"}

	if _, err := registry.install(first); err != nil {
		t.Fatalf("install first: %v", err)
	}

	if _, err := registry.install(&session{agentID: "agent-2"}); err == nil {
		t.Fatal("active registry entry was evicted")
	}

	registry.remove(first)

	if _, err := registry.install(&session{agentID: "agent-2"}); err != nil {
		t.Fatalf("inactive registry entry was not evicted: %v", err)
	}
}
