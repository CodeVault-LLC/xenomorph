package command

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Envelope struct {
	CommandID   string          `json:"command_id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	RequestedBy string          `json:"requested_by"`
	IssuedAt    time.Time       `json:"issued_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Reason      string          `json:"reason"`
	Signature   string          `json:"signature"`
}

type Queue struct {
	mu      sync.Mutex
	entries map[string][]*Envelope
}

func NewQueue() *Queue {
	return &Queue{
		entries: make(map[string][]*Envelope),
	}
}

func (q *Queue) Enqueue(agentID string, cmd *Envelope) {
	if cmd.CommandID == "" {
		cmd.CommandID = uuid.New().String()
	}
	if cmd.IssuedAt.IsZero() {
		cmd.IssuedAt = time.Now().UTC()
	}
	if cmd.ExpiresAt.IsZero() {
		cmd.ExpiresAt = cmd.IssuedAt.Add(2 * time.Minute)
	}
	cmd.Signature = "gateway" // server-authored

	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries[agentID] = append(q.entries[agentID], cmd)
}

func (q *Queue) Dequeue(agentID string) *Envelope {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue := q.entries[agentID]
	if len(queue) == 0 {
		return nil
	}

	cmd := queue[0]
	q.entries[agentID] = queue[1:]
	return cmd
}
