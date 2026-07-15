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
// Pending commands remain in memory for dispatch. Production construction also
// installs a journal that persists intent and lifecycle state before each
// dispatch or result boundary and restores only commands still in queued state.
type Queue struct {
	mu      sync.Mutex
	notify  chan struct{}
	entries map[string][]*Envelope
	signer  Signer
	journal *Journal
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
	return newQueueWithSigner(signer, nil)
}

// NewDurableQueueWithSigner restores queued commands from a bounded journal.
func NewDurableQueueWithSigner(signer Signer, journalPath string) (*Queue, error) {
	journal, err := NewJournal(journalPath)
	if err != nil {
		return nil, err
	}
	return newQueueWithSigner(signer, journal)
}

func newQueueWithSigner(signer Signer, journal *Journal) (*Queue, error) {
	if signer == nil {
		return nil, fmt.Errorf("command signer is required")
	}
	if signer.KeyID() == "" {
		return nil, fmt.Errorf("command signing key ID is required")
	}
	entries := make(map[string][]*Envelope)
	if journal != nil {
		entries = journal.Queued()
	}
	return &Queue{
		notify:  make(chan struct{}),
		entries: entries,
		signer:  signer,
		journal: journal,
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
	if err := q.prepareEnvelope(agentID, cmd); err != nil {
		return fmt.Errorf("sign command: %w", err)
	}

	stored := cloneEnvelope(*cmd)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.entries[agentID]) >= maxQueueDepth {
		return fmt.Errorf("command queue for agent is full")
	}
	if q.journal != nil {
		if err := q.journal.RecordQueued(agentID, stored); err != nil {
			return fmt.Errorf("persist queued command: %w", err)
		}
	}
	q.entries[agentID] = append(q.entries[agentID], &stored)
	close(q.notify)
	q.notify = make(chan struct{})
	return nil
}

func (q *Queue) prepareEnvelope(agentID string, envelope *Envelope) error {
	if envelope.CommandID == "" {
		envelope.CommandID = uuid.New().String()
	}
	if envelope.IssuedAt.IsZero() {
		envelope.IssuedAt = time.Now().UTC()
	}
	if envelope.ExpiresAt.IsZero() {
		envelope.ExpiresAt = envelope.IssuedAt.Add(defaultExpiryDuration)
	}
	envelope.ProtocolVersion = commandauth.ProtocolVersion
	envelope.AudienceAgentID = agentID
	envelope.Nonce = uuid.New().String()
	envelope.KeyID = q.signer.KeyID()
	envelope.Signature = ""
	return q.signer.SignCommand(envelope)
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
	command, _ := q.dispatchLocked(agentID)
	return command
}

// WaitDequeue removes and returns the next command for the agent. It blocks
// until a command is available or ctx is canceled.
func (q *Queue) WaitDequeue(ctx context.Context, agentID string) *Envelope {
	command, _ := q.WaitDispatch(ctx, agentID)
	return command
}

// WaitDispatch durably marks the next command dispatched before returning it.
func (q *Queue) WaitDispatch(ctx context.Context, agentID string) (*Envelope, error) {
	for {
		q.mu.Lock()
		if len(q.entries[agentID]) > 0 {
			cmd, err := q.dispatchLocked(agentID)
			q.mu.Unlock()
			return cmd, err
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, nil
		case <-notify:
		}
	}
}

func (q *Queue) dispatchLocked(agentID string) (*Envelope, error) {
	queue := q.entries[agentID]
	if len(queue) == 0 {
		return nil, nil
	}

	cmd := queue[0]
	if q.journal != nil {
		if err := q.journal.MarkDispatched(agentID, cmd.CommandID); err != nil {
			return nil, fmt.Errorf("persist command dispatch: %w", err)
		}
	}
	q.entries[agentID] = queue[1:]
	return cmd, nil
}

// CommitResult applies a terminal authenticated-agent result to durable state.
func (q *Queue) CommitResult(agentID, commandID string, canonicalResult []byte) (ResultDisposition, error) {
	if q == nil || q.journal == nil {
		return 0, fmt.Errorf("commit command result: durable journal is unavailable")
	}
	return q.journal.CommitResult(agentID, commandID, canonicalResult)
}

// MarkOutcomeUnknown records an ambiguous command delivery without retrying it.
func (q *Queue) MarkOutcomeUnknown(agentID, commandID string) error {
	if q == nil || q.journal == nil {
		return fmt.Errorf("mark command outcome unknown: durable journal is unavailable")
	}
	return q.journal.MarkOutcomeUnknown(agentID, commandID)
}

// MarkAccepted records the authenticated client's durable replay reservation.
func (q *Queue) MarkAccepted(agentID, commandID string) error {
	if q == nil || q.journal == nil {
		return fmt.Errorf("mark command accepted: durable journal is unavailable")
	}
	return q.journal.MarkAccepted(agentID, commandID)
}

// Command returns the durable gateway-authored command contract for result validation.
func (q *Queue) Command(agentID, commandID string) (Envelope, bool) {
	if q == nil || q.journal == nil {
		return Envelope{}, false
	}
	return q.journal.Envelope(agentID, commandID)
}
