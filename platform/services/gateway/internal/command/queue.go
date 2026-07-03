// Package command implements a per-agent FIFO command queue. Commands are
// enqueued by the gateway operator or Discord command handler and dequeued
// by agents on their poll cycle.
package command

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultExpiryDuration = 2 * time.Minute

// Envelope is a server-authored command destined for a remote agent. Every
// field except Payload is required and set by the gateway at enqueue time.
//
// The Signature field is set to "gateway" to indicate server authorship.
// Agents must validate the signature before executing the command.
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

// Queue is a per-agent FIFO queue of command envelopes. Thread-safe.
//
// The queue is held in memory and is not persisted across gateway restarts.
// Operators must re-enqueue commands after a gateway restart.
type Queue struct {
	mu      sync.Mutex
	notify  chan struct{}
	entries map[string][]*Envelope
}

// NewQueue creates an empty command queue.
func NewQueue() *Queue {
	return &Queue{
		notify:  make(chan struct{}),
		entries: make(map[string][]*Envelope),
	}
}

// Enqueue adds a command to the end of the agent's queue. The command is
// assigned a UUID-based CommandID when empty, a zero-value IssuedAt is set
// to the current UTC time, and an empty ExpiresAt defaults to 2 minutes
// after IssuedAt. The Signature is always overwritten to "gateway".
//
// The 2-minute expiry ensures stale commands are not executed. Agents that
// poll less frequently than every 2 minutes may miss commands.
func (q *Queue) Enqueue(agentID string, cmd *Envelope) {
	if cmd.CommandID == "" {
		cmd.CommandID = uuid.New().String()
	}
	if cmd.IssuedAt.IsZero() {
		cmd.IssuedAt = time.Now().UTC()
	}
	if cmd.ExpiresAt.IsZero() {
		cmd.ExpiresAt = cmd.IssuedAt.Add(defaultExpiryDuration)
	}
	cmd.Signature = "gateway"

	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries[agentID] = append(q.entries[agentID], cmd)
	close(q.notify)
	q.notify = make(chan struct{})
}

// Dequeue removes and returns the next command for the agent. Returns nil
// when the agent's queue is empty.
func (q *Queue) Dequeue(agentID string) *Envelope {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dequeueLocked(agentID)
}

// WaitDequeue removes and returns the next command for the agent. It blocks
// until a command is available or ctx is canceled.
func (q *Queue) WaitDequeue(ctx context.Context, agentID string) *Envelope {
	for {
		q.mu.Lock()
		if cmd := q.dequeueLocked(agentID); cmd != nil {
			q.mu.Unlock()
			return cmd
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-notify:
		}
	}
}

func (q *Queue) dequeueLocked(agentID string) *Envelope {
	queue := q.entries[agentID]
	if len(queue) == 0 {
		return nil
	}

	cmd := queue[0]
	q.entries[agentID] = queue[1:]
	return cmd
}
