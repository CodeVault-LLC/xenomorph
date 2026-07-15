package agentquic

import (
	"context"

	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

// IngressReceipt is gateway-authored routing metadata for one authenticated frame.
// RemoteAddress is untrusted operational metadata and must not be used as identity.
type IngressReceipt struct {
	AgentID         string
	SessionID       [16]byte
	TraceID         [16]byte
	RemoteAddress   string
	MessageType     wire.MessageType
	MessageSequence uint64
	OperationID     [16]byte
}

// IngressMessage is a closed transport-neutral union of agent-authored bodies.
// Exactly one body matching Type is populated by the canonical XBP decoder.
type IngressMessage struct {
	Type          wire.MessageType
	Heartbeat     *wire.Heartbeat
	Attestation   *wire.Attestation
	LogEntry      *wire.LogEntry
	CommandResult *wire.CommandResult
	CommandState  *wire.CommandState
	TransferOpen  *wire.TransferOpen
	TransferChunk *wire.TransferChunk
	TransferFinal *wire.TransferFinalize
	TransferAbort *wire.TransferAbort
	MediaOpen     *wire.MediaOpen
	MediaFrame    *wire.MediaFrame
	MediaClose    *wire.MediaClose
}

// IngressResult describes the application commit point returned to the agent.
type IngressResult struct {
	Status          wire.AcknowledgementStatus
	Commit          wire.AcknowledgementCommit
	ReceiptID       [16]byte
	Retry           wire.RetryClassification
	PublicErrorCode string
}

// IngressSink commits validated agent-authored messages under a current session lease.
type IngressSink interface {
	CommitAgentMessage(context.Context, IngressReceipt, IngressMessage) (IngressResult, error)
}

// TransferChunkSource streams gateway-staged chunks after a validated
// TransferOpen without exposing workspace credentials to the QUIC package.
type TransferChunkSource interface {
	StreamTransferChunks(context.Context, IngressReceipt, wire.TransferOpen, func(wire.TransferChunk) error) error
}
