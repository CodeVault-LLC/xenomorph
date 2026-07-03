package transport

import (
	"context"
	"sync"
	"time"
)

// ScreenFrame stores the latest screenshot returned by an authenticated agent.
// Frame bytes originate from the agent and are treated as opaque image data.
type ScreenFrame struct {
	AgentID     string    `json:"agent_id"`
	CommandID   string    `json:"command_id"`
	CapturedAt  time.Time `json:"captured_at"`
	ContentType string    `json:"content_type"`
	Content     []byte    `json:"-"`
}

// ScreenStore keeps the most recent screenshot per agent in memory.
type ScreenStore struct {
	mu     sync.Mutex
	notify chan struct{}
	frames map[string]ScreenFrame
}

// ScreenSessions tracks active browser viewers per agent. It is intentionally
// separate from frame storage so viewer demand controls media streaming
// without making frame bytes trust-bearing.
type ScreenSessions struct {
	mu      sync.Mutex
	total   int
	viewers map[string]int
}

func NewScreenSessions() *ScreenSessions {
	return &ScreenSessions{
		viewers: make(map[string]int),
	}
}

func (s *ScreenSessions) BeginViewer(agentID string) (agentViewers int, totalViewers int) {
	if s == nil {
		return 0, 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewers[agentID]++
	s.total++
	return s.viewers[agentID], s.total
}

func (s *ScreenSessions) EndViewer(agentID string) (agentViewers int, totalViewers int) {
	if s == nil {
		return 0, 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	count := s.viewers[agentID]
	if count <= 0 {
		return 0, s.total
	}
	if s.total > 0 {
		s.total--
	}
	if count <= 1 {
		delete(s.viewers, agentID)
		return 0, s.total
	}
	s.viewers[agentID] = count - 1
	return s.viewers[agentID], s.total
}

func NewScreenStore() *ScreenStore {
	return &ScreenStore{
		notify: make(chan struct{}),
		frames: make(map[string]ScreenFrame),
	}
}

func (s *ScreenStore) Save(agentID string, frame ScreenFrame) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames[agentID] = frame
	close(s.notify)
	s.notify = make(chan struct{})
}

func (s *ScreenStore) Latest(agentID string) (ScreenFrame, bool) {
	if s == nil {
		return ScreenFrame{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	frame, ok := s.frames[agentID]
	return frame, ok
}

func (s *ScreenStore) WaitLatestAfter(ctx context.Context, agentID string, after time.Time) (ScreenFrame, bool) {
	if s == nil {
		return ScreenFrame{}, false
	}

	for {
		s.mu.Lock()
		if frame, ok := s.frames[agentID]; ok && frame.CapturedAt.After(after) {
			s.mu.Unlock()
			return frame, true
		}
		notify := s.notify
		s.mu.Unlock()

		if ctx.Err() != nil {
			return ScreenFrame{}, false
		}

		select {
		case <-ctx.Done():
			return ScreenFrame{}, false
		case <-notify:
		}
	}
}
