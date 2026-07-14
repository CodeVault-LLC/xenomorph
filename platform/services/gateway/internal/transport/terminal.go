package transport

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxTerminalSessionsPerAgent = 12
	maxTerminalEntriesPerAgent  = 300
	maxTerminalLabelBytes       = 80
	terminalShellPowerShell     = "powershell"
	terminalShellZsh            = "zsh"
	terminalShellBash           = "bash"
)

// TerminalSession is the dashboard read model for a browser-created terminal
// session. AgentID, SessionID, timestamps, and command IDs are gateway-authored.
// Shell and working directory are operator requests until confirmed by a
// command result from the authenticated agent.
type TerminalSession struct {
	AgentID          string    `json:"agent_id"`
	SessionID        string    `json:"session_id"`
	Label            string    `json:"label"`
	Shell            string    `json:"shell"`
	WorkingDirectory string    `json:"working_directory"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	LastCommandID    string    `json:"last_command_id"`
}

// TerminalEntry is one command request and, when available, its authenticated
// agent response. Command text is user-authored. Output and exit code are
// client-authored and must not be used as identity evidence.
type TerminalEntry struct {
	AgentID          string     `json:"agent_id"`
	SessionID        string     `json:"session_id"`
	CommandID        string     `json:"command_id"`
	Command          string     `json:"command"`
	Shell            string     `json:"shell"`
	WorkingDirectory string     `json:"working_directory"`
	Status           string     `json:"status"`
	ExitCode         int        `json:"exit_code"`
	OutputLog        string     `json:"output_log"`
	Reason           string     `json:"reason"`
	SubmittedAt      time.Time  `json:"submitted_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// TerminalStore keeps a bounded in-memory dashboard view of terminal sessions
// and command results. It does not dispatch commands and is not an authority
// for agent identity.
type TerminalStore struct {
	mu       sync.Mutex
	sessions map[string][]TerminalSession
	entries  map[string][]TerminalEntry
}

// NewTerminalStore creates an empty bounded terminal read model.
func NewTerminalStore() *TerminalStore {
	return &TerminalStore{
		sessions: make(map[string][]TerminalSession),
		entries:  make(map[string][]TerminalEntry),
	}
}

// CreateSession creates gateway-authored session identity for operator-authored terminal preferences.
func (s *TerminalStore) CreateSession(agentID, label, shell, workingDirectory string) TerminalSession {
	now := time.Now().UTC()
	session := TerminalSession{
		AgentID:          agentID,
		SessionID:        uuid.New().String(),
		Label:            clampText(strings.TrimSpace(label), maxTerminalLabelBytes),
		Shell:            normalizeTerminalShell(shell),
		WorkingDirectory: clampText(strings.TrimSpace(workingDirectory), maxPathLen),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if session.Label == "" {
		session.Label = session.Shell
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := s.sessions[agentID]
	sessions = append(sessions, session)
	if overflow := len(sessions) - maxTerminalSessionsPerAgent; overflow > 0 {
		sessions = append([]TerminalSession(nil), sessions[overflow:]...)
	}
	s.sessions[agentID] = sessions
	return session
}

// ListSessions returns bounded sessions for one gateway-authenticated agent.
func (s *TerminalStore) ListSessions(agentID string) []TerminalSession {
	if s == nil || agentID == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := append([]TerminalSession(nil), s.sessions[agentID]...)
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions
}

// Session returns one session only when it belongs to the requested agent.
func (s *TerminalStore) Session(agentID, sessionID string) (TerminalSession, bool) {
	if s == nil || agentID == "" || sessionID == "" {
		return TerminalSession{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, session := range s.sessions[agentID] {
		if session.SessionID == sessionID {
			return session, true
		}
	}
	return TerminalSession{}, false
}

// DeleteSession removes one agent-scoped session and its in-memory entries.
func (s *TerminalStore) DeleteSession(agentID, sessionID string) bool {
	if s == nil || agentID == "" || sessionID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := s.sessions[agentID]
	nextSessions := sessions[:0]
	deleted := false
	for _, session := range sessions {
		if session.SessionID == sessionID {
			deleted = true
			continue
		}
		nextSessions = append(nextSessions, session)
	}
	if !deleted {
		return false
	}
	s.sessions[agentID] = append([]TerminalSession(nil), nextSessions...)

	entries := s.entries[agentID]
	nextEntries := entries[:0]
	for _, entry := range entries {
		if entry.SessionID != sessionID {
			nextEntries = append(nextEntries, entry)
		}
	}
	s.entries[agentID] = append([]TerminalEntry(nil), nextEntries...)
	return true
}

// AppendQueued records a bounded gateway-authored command submission.
func (s *TerminalStore) AppendQueued(entry TerminalEntry) {
	if s == nil || entry.AgentID == "" || entry.SessionID == "" || entry.CommandID == "" {
		return
	}
	if entry.SubmittedAt.IsZero() {
		entry.SubmittedAt = time.Now().UTC()
	}
	if entry.Status == "" {
		entry.Status = "queued"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entries[entry.AgentID]
	entries = append(entries, entry)
	if overflow := len(entries) - maxTerminalEntriesPerAgent; overflow > 0 {
		entries = append([]TerminalEntry(nil), entries[overflow:]...)
	}
	s.entries[entry.AgentID] = entries
	s.touchSessionLocked(entry.AgentID, entry.SessionID, entry.CommandID, entry.Shell, entry.WorkingDirectory, entry.SubmittedAt)
}

// Complete binds an authenticated agent result to an existing gateway command ID.
func (s *TerminalStore) Complete(agentID, commandID string, result TerminalEntry) bool {
	if s == nil || agentID == "" || commandID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entries[agentID]
	for i := range entries {
		if entries[i].CommandID != commandID {
			continue
		}
		now := time.Now().UTC()
		entries[i].Status = result.Status
		entries[i].ExitCode = result.ExitCode
		entries[i].OutputLog = result.OutputLog
		entries[i].Reason = result.Reason
		entries[i].CompletedAt = &now
		if result.Shell != "" {
			entries[i].Shell = result.Shell
		}
		if result.WorkingDirectory != "" {
			entries[i].WorkingDirectory = result.WorkingDirectory
		}
		s.entries[agentID] = entries
		s.touchSessionLocked(agentID, entries[i].SessionID, commandID, entries[i].Shell, entries[i].WorkingDirectory, now)
		return true
	}
	return false
}

// ListEntries returns bounded entries for one agent-scoped terminal session.
func (s *TerminalStore) ListEntries(agentID, sessionID string, limit int) []TerminalEntry {
	if s == nil || agentID == "" || sessionID == "" {
		return nil
	}
	if limit <= 0 || limit > maxTerminalEntriesPerAgent {
		limit = maxTerminalEntriesPerAgent
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]TerminalEntry, 0, len(s.entries[agentID]))
	for _, entry := range s.entries[agentID] {
		if entry.SessionID == sessionID {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].SubmittedAt.Before(entries[j].SubmittedAt)
	})
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries
}

func (s *TerminalStore) touchSessionLocked(agentID, sessionID, commandID, shell, workingDirectory string, at time.Time) {
	sessions := s.sessions[agentID]
	for i := range sessions {
		if sessions[i].SessionID != sessionID {
			continue
		}
		sessions[i].LastCommandID = commandID
		sessions[i].UpdatedAt = at
		if shell != "" {
			sessions[i].Shell = shell
		}
		if workingDirectory != "" {
			sessions[i].WorkingDirectory = workingDirectory
		}
		s.sessions[agentID] = sessions
		return
	}
}

func normalizeTerminalShell(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case terminalShellPowerShell, "powershell.exe", "windows powershell":
		return terminalShellPowerShell
	case "pwsh", "powershell core":
		return "pwsh"
	case "cmd", "cmd.exe":
		return "cmd"
	case terminalShellZsh:
		return terminalShellZsh
	case terminalShellBash:
		return terminalShellBash
	case "sh":
		return "sh"
	default:
		return terminalShellBash
	}
}

func defaultTerminalShell(osVersion string) string {
	normalized := strings.ToLower(osVersion)
	switch {
	case strings.Contains(normalized, "windows"):
		return terminalShellPowerShell
	case strings.Contains(normalized, "darwin"), strings.Contains(normalized, "mac"):
		return terminalShellZsh
	default:
		return terminalShellBash
	}
}

func clampTerminalOutput(value []byte, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return string(value)
	}
	return string(value[:limit]) + "\n[output truncated]\n"
}
