package wire

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const (
	minimumHeartbeatMilliseconds uint64 = 10_000
	maximumHeartbeatMilliseconds uint64 = 30_000
)

// Platform identifies an allowlisted client operating-system family.
type Platform uint64

const (
	// PlatformLinux identifies Linux clients.
	PlatformLinux Platform = iota + 1
	// PlatformMacOS identifies macOS clients.
	PlatformMacOS
	// PlatformWindows identifies Windows clients.
	PlatformWindows
)

// Architecture identifies an allowlisted client processor architecture.
type Architecture uint64

const (
	// ArchitectureAMD64 identifies the amd64 architecture.
	ArchitectureAMD64 Architecture = iota + 1
	// ArchitectureARM64 identifies the arm64 architecture.
	ArchitectureARM64
)

// AcknowledgementStatus reports application handling at a defined commit point.
type AcknowledgementStatus uint64

const (
	// AcknowledgementAccepted reports successful handling.
	AcknowledgementAccepted AcknowledgementStatus = iota + 1
	// AcknowledgementDuplicate reports an identical previously committed message.
	AcknowledgementDuplicate
	// AcknowledgementRejected reports a nonretryable message rejection.
	AcknowledgementRejected
	// AcknowledgementBusy reports bounded temporary overload.
	AcknowledgementBusy
	// AcknowledgementFailed reports an internal failure before the commit point.
	AcknowledgementFailed
)

// AcknowledgementCommit identifies how far application work progressed.
type AcknowledgementCommit uint64

const (
	// CommitDecoded means canonical decoding completed.
	CommitDecoded AcknowledgementCommit = iota + 1
	// CommitValidated means schema and authorization validation completed.
	CommitValidated
	// CommitPersisted means the owned durable state was synchronized.
	CommitPersisted
	// CommitPublished means the broker acknowledged publication.
	CommitPublished
	// CommitOperationTerminal means the operation journal reached a terminal state.
	CommitOperationTerminal
)

// RetryClassification tells a sender whether application retry is permitted.
type RetryClassification uint64

const (
	// RetryNever prohibits application retry.
	RetryNever RetryClassification = iota + 1
	// RetrySameOperation permits retry with the same stable operation identifier.
	RetrySameOperation
	// RetryNewOperation requires a new operation identifier and policy decision.
	RetryNewOperation
)

// CommandResultState is the registered terminal result-state vocabulary.
type CommandResultState uint64

const (
	// CommandResultStateExecuted reports completed client execution.
	CommandResultStateExecuted CommandResultState = iota + 1
	// CommandResultStateRejected reports client-side validation rejection.
	CommandResultStateRejected
	// CommandResultStateOutcomeUnknown reports execution that cannot be safely inferred.
	CommandResultStateOutcomeUnknown
)

// TransferDirection identifies the authenticated byte sender for a transfer lane.
type TransferDirection uint64

const (
	// TransferAgentToGateway carries agent-authored download bytes into staging.
	TransferAgentToGateway TransferDirection = iota + 1
	// TransferGatewayToAgent carries browser-staged upload bytes to the agent.
	TransferGatewayToAgent
)

// LogLevel is the fixed client diagnostic severity registry.
type LogLevel uint64

const (
	// LogLevelDebug identifies low-priority diagnostic metadata.
	LogLevelDebug LogLevel = iota + 1
	// LogLevelInfo identifies normal client lifecycle metadata.
	LogLevelInfo
	// LogLevelWarn identifies degraded client behavior.
	LogLevelWarn
	// LogLevelError identifies failed client lifecycle behavior.
	LogLevelError
)

// LogComponent is the fixed client diagnostic component registry.
type LogComponent uint64

const (
	// LogComponentRuntime identifies the client runtime supervisor.
	LogComponentRuntime LogComponent = iota + 1
	// LogComponentAuthentication identifies the client authentication flow.
	LogComponentAuthentication
	// LogComponentAttestation identifies endpoint inventory submission.
	LogComponentAttestation
	// LogComponentHeartbeat identifies periodic heartbeat submission.
	LogComponentHeartbeat
	// LogComponentCommand identifies command receipt, validation, and result work.
	LogComponentCommand
)

// LogEvent is the fixed client diagnostic event registry.
type LogEvent uint64

const (
	// LogEventRuntimeStarted reports completed client setup.
	LogEventRuntimeStarted LogEvent = iota + 1
	// LogEventAuthenticationSucceeded reports an accepted authenticated session.
	LogEventAuthenticationSucceeded
	// LogEventAuthenticationFailed reports failed client authentication.
	LogEventAuthenticationFailed
	// LogEventAttestationSubmitted reports accepted endpoint inventory.
	LogEventAttestationSubmitted
	// LogEventAttestationFailed reports rejected endpoint inventory.
	LogEventAttestationFailed
	// LogEventHeartbeatFailed reports failed periodic heartbeat delivery.
	LogEventHeartbeatFailed
	// LogEventCommandReceived reports a received signed command.
	LogEventCommandReceived
	// LogEventCommandCompleted reports an accepted terminal command result.
	LogEventCommandCompleted
	// LogEventCommandTransportFailed reports command-lane failure.
	LogEventCommandTransportFailed
	// LogEventCommandProcessingFailed reports local processing failure.
	LogEventCommandProcessingFailed
	// LogEventCommandResultFailed reports failed result submission.
	LogEventCommandResultFailed
	// LogEventRuntimeLoopFailed reports termination of a runtime lane.
	LogEventRuntimeLoopFailed
	// LogEventQUICNetworkFallback reports an authorized, expiring network-only fallback.
	LogEventQUICNetworkFallback
)

// ValidateClientHello applies semantic negotiation bounds after canonical decoding.
func ValidateClientHello(hello ClientHello) error {
	if hello.MinimumMinor > uint64(ProtocolMinor) || hello.MaximumMinor < uint64(ProtocolMinor) ||
		hello.MinimumMinor > hello.MaximumMinor {
		return fmt.Errorf("validate client hello: %w: incompatible minor range", ErrUnexpectedMessage)
	}
	if strings.TrimSpace(hello.ImplementationVersion) == "" || isZero16(hello.ClientInstanceNonce) {
		return fmt.Errorf("validate client hello: %w: missing build label or instance nonce", ErrEncoding)
	}
	if Platform(hello.Platform) < PlatformLinux || Platform(hello.Platform) > PlatformWindows {
		return fmt.Errorf("validate client hello: %w: unknown platform", ErrEncoding)
	}
	if Architecture(hello.Architecture) < ArchitectureAMD64 || Architecture(hello.Architecture) > ArchitectureARM64 {
		return fmt.Errorf("validate client hello: %w: unknown architecture", ErrEncoding)
	}
	return nil
}

// ValidateServerHello applies semantic session bounds after canonical decoding.
func ValidateServerHello(hello ServerHello, offered ClientHello) error {
	if hello.SelectedMinor < offered.MinimumMinor || hello.SelectedMinor > offered.MaximumMinor {
		return fmt.Errorf("validate server hello: %w: selected minor outside offer", ErrUnexpectedMessage)
	}
	if hello.NegotiatedFeatures&^offered.Features != 0 || isZero16(hello.SessionID) {
		return fmt.Errorf("validate server hello: %w: invalid features or session ID", ErrEncoding)
	}
	if hello.HeartbeatIntervalMilliseconds < minimumHeartbeatMilliseconds || hello.HeartbeatIntervalMilliseconds > maximumHeartbeatMilliseconds {
		return fmt.Errorf("validate server hello: %w: heartbeat interval outside policy", ErrLimit)
	}
	if hello.MaximumIdleMilliseconds <= hello.HeartbeatIntervalMilliseconds || hello.EventFrameMaximum == 0 {
		return fmt.Errorf("validate server hello: %w: invalid liveness or frame limit", ErrLimit)
	}
	if strings.TrimSpace(hello.CommandVerificationKeyID) == "" {
		return fmt.Errorf("validate server hello: %w: missing command key ID", ErrEncoding)
	}
	return nil
}

// ValidateLogEntry rejects zero or unassigned registry values.
func ValidateLogEntry(entry LogEntry) error {
	if LogLevel(entry.Level) < LogLevelDebug || LogLevel(entry.Level) > LogLevelError {
		return fmt.Errorf("validate log entry: %w: unknown level", ErrEncoding)
	}
	if LogComponent(entry.Component) < LogComponentRuntime || LogComponent(entry.Component) > LogComponentCommand {
		return fmt.Errorf("validate log entry: %w: unknown component", ErrEncoding)
	}
	if LogEvent(entry.EventCode) < LogEventRuntimeStarted || LogEvent(entry.EventCode) > LogEventQUICNetworkFallback {
		return fmt.Errorf("validate log entry: %w: unknown event", ErrEncoding)
	}
	return nil
}

// ValidateCommandResult applies the registered state and payload revision
// contract after canonical structural decoding.
func ValidateCommandResult(result CommandResult) error {
	if strings.TrimSpace(result.CommandType) == "" {
		return fmt.Errorf("validate command result: %w: missing command type", ErrEncoding)
	}
	if CommandResultState(result.State) < CommandResultStateExecuted ||
		CommandResultState(result.State) > CommandResultStateOutcomeUnknown {
		return fmt.Errorf("validate command result: %w: unknown result state", ErrEncoding)
	}
	if result.RespondedAtMilliseconds == 0 {
		return fmt.Errorf("validate command result: %w: missing response time", ErrEncoding)
	}
	if result.ResultRevision != 1 {
		return fmt.Errorf("validate command result: %w: unsupported payload revision", ErrUnexpectedMessage)
	}
	return nil
}

// ValidateTransferChunk verifies declared size and SHA-256 content binding.
func ValidateTransferChunk(chunk TransferChunk) error {
	if chunk.DigestAlgorithm != 1 {
		return fmt.Errorf("validate transfer chunk: %w: unsupported digest algorithm", ErrEncoding)
	}
	if chunk.ChunkLength != uint64(len(chunk.Data)) {
		return fmt.Errorf("validate transfer chunk: %w: declared length mismatch", ErrEncoding)
	}
	digest := sha256.Sum256(chunk.Data)
	if digest != chunk.Digest {
		return fmt.Errorf("validate transfer chunk: %w: digest mismatch", ErrEncoding)
	}
	return nil
}

// ValidateMediaFrame verifies required media identifiers and nonempty content.
func ValidateMediaFrame(frame MediaFrame) error {
	if isZero16(frame.GenerationID) || frame.FrameNumber == 0 || len(frame.Data) == 0 {
		return fmt.Errorf("validate media frame: %w: missing generation, sequence, or content", ErrEncoding)
	}
	return nil
}
