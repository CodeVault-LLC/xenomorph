// Package command implements a per-agent FIFO command queue. Commands are
// enqueued by the dashboard operator and dequeued by agents on their poll cycle.
package command

import (
	"context"
	"crypto/rsa"
	"fmt"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
	"github.com/google/uuid"
)

const (
	defaultExpiryDuration = 2 * time.Minute
	maxQueueDepth         = 256
)

// Envelope is the shared integrity-protected command wire contract.
type Envelope = commandauth.Envelope

// Queue is a per-agent FIFO queue of command envelopes. Thread-safe.
//
// The queue is held in memory and is not persisted across gateway restarts.
// Operators must re-enqueue commands after a gateway restart.
type Queue struct {
	mu      sync.Mutex
	notify  chan struct{}
	entries map[string][]*Envelope
	signer  Signer
}

// Signer is the opaque command-signing capability consumed by Queue. The
// implementation owns private-key storage and lifecycle; Queue receives only
// signatures and a server-authored key identifier.
type Signer interface {
	SignCommand(envelope *Envelope) error
	KeyID() string
}

type rsaSigner struct {
	privateKey *rsa.PrivateKey
	keyID      string
}

// NewQueue creates an empty bounded command queue using the supplied key.
func NewQueue(signingKey *rsa.PrivateKey, keyID string) (*Queue, error) {
	if signingKey == nil {
		return nil, fmt.Errorf("command signing key is required")
	}
	if keyID == "" {
		return nil, fmt.Errorf("command signing key ID is required")
	}
	return NewQueueWithSigner(&rsaSigner{privateKey: signingKey, keyID: keyID})
}

// NewQueueWithSigner creates an empty bounded command queue using an opaque
// gateway-owned signing capability.
func NewQueueWithSigner(signer Signer) (*Queue, error) {
	if signer == nil {
		return nil, fmt.Errorf("command signer is required")
	}
	if signer.KeyID() == "" {
		return nil, fmt.Errorf("command signing key ID is required")
	}
	return &Queue{
		notify:  make(chan struct{}),
		entries: make(map[string][]*Envelope),
		signer:  signer,
	}, nil
}

// Enqueue adds a command to the end of the agent's queue. The command is
// assigned a UUID-based CommandID when empty, a zero-value IssuedAt is set
// to the current UTC time, and an empty ExpiresAt defaults to 2 minutes
// after IssuedAt. The queue binds and signs the command for agentID. When the
// bounded queue is full, Enqueue rejects the new command without dropping an
// already approved operation.
//
// The 2-minute expiry ensures stale commands are not executed. Agents that
// poll less frequently than every 2 minutes may miss commands.
func (q *Queue) Enqueue(agentID string, cmd *Envelope) error {
	if q == nil || cmd == nil {
		return fmt.Errorf("command queue and envelope are required")
	}
	if agentID == "" {
		return fmt.Errorf("command audience agent ID is required")
	}
	if cmd.CommandID == "" {
		cmd.CommandID = uuid.New().String()
	}
	if cmd.IssuedAt.IsZero() {
		cmd.IssuedAt = time.Now().UTC()
	}
	if cmd.ExpiresAt.IsZero() {
		cmd.ExpiresAt = cmd.IssuedAt.Add(defaultExpiryDuration)
	}
	cmd.ProtocolVersion = commandauth.ProtocolVersion
	cmd.AudienceAgentID = agentID
	cmd.Nonce = uuid.New().String()
	cmd.KeyID = q.signer.KeyID()
	cmd.Signature = ""
	if err := q.signer.SignCommand(cmd); err != nil {
		return fmt.Errorf("sign command: %w", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.entries[agentID]) >= maxQueueDepth {
		return fmt.Errorf("command queue for agent is full")
	}
	q.entries[agentID] = append(q.entries[agentID], cmd)
	close(q.notify)
	q.notify = make(chan struct{})
	return nil
}

func (signer *rsaSigner) SignCommand(envelope *Envelope) error {
	return commandauth.Sign(envelope, signer.privateKey)
}

func (signer *rsaSigner) KeyID() string {
	return signer.keyID
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
