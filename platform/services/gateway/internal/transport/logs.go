package transport

import (
	"sort"
	"sync"
	"time"
)

const maxLogEntriesPerAgent int = 200

// AgentLogEntry is the dashboard read model for an authenticated agent log.
// Agent ID, client IP, event ID, and observed time are gateway-authored.
// Level, component, and message are event payload fields.
type AgentLogEntry struct {
	EventID    string    `json:"event_id"`
	AgentID    string    `json:"agent_id"`
	ClientIP   string    `json:"client_ip"`
	ObservedAt time.Time `json:"observed_at"`
	Level      string    `json:"level"`
	Component  string    `json:"component"`
	Message    string    `json:"message"`
}

// AgentLogStore keeps a bounded in-memory log history for dashboard reads.
type AgentLogStore struct {
	mu      sync.Mutex
	entries map[string][]AgentLogEntry
	limit   int
}

// NewAgentLogStore creates a bounded per-agent log store.
func NewAgentLogStore(limit int) *AgentLogStore {
	if limit <= 0 {
		limit = maxLogEntriesPerAgent
	}
	return &AgentLogStore{
		entries: make(map[string][]AgentLogEntry),
		limit:   limit,
	}
}

// Append stores a log entry. Empty agent IDs are ignored.
func (s *AgentLogStore) Append(entry AgentLogEntry) {
	if s == nil || entry.AgentID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entries[entry.AgentID]
	entries = append(entries, entry)
	if overflow := len(entries) - s.limit; overflow > 0 {
		entries = append([]AgentLogEntry(nil), entries[overflow:]...)
	}
	s.entries[entry.AgentID] = entries
}

// List returns the newest log entries for an agent in descending time order.
func (s *AgentLogStore) List(agentID string, limit int) []AgentLogEntry {
	if s == nil || agentID == "" {
		return nil
	}
	if limit <= 0 || limit > s.limit {
		limit = s.limit
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := append([]AgentLogEntry(nil), s.entries[agentID]...)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].ObservedAt.After(entries[j].ObservedAt)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}
