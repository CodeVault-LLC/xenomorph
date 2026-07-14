package agent

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	clientfs "github.com/codevault-llc/xenomorph/platform/client/internal/filesystem"
	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

var allowedCommandTypes = map[CommandType]struct{}{
	CommandTypeNotice:                {},
	CommandTypeRequestScreenshot:     {},
	CommandTypeStartScreenStream:     {},
	CommandTypeStopScreenStream:      {},
	CommandTypeTerminalRun:           {},
	CommandTypeFilesRootsList:        {},
	CommandTypeFilesDirectoryList:    {},
	CommandTypeFilesDirectorySearch:  {},
	CommandTypeFilesMetadataGet:      {},
	CommandTypeFilesPreviewRead:      {},
	CommandTypeFilesOperationExecute: {},
	CommandTypeFilesTransferPrepare:  {},
	CommandTypeFilesTransferResume:   {},
	CommandTypeFilesTransferAbort:    {},
}

// CommandDecision contains the result of processing a command.
type CommandDecision struct {
	Result CommandResultPayload
}

type commandOutcome struct {
	reason           string
	outputData       []byte
	terminalMetadata terminalResultMetadata
	resultData       json.RawMessage
}

// CommandValidator verifies gateway command authenticity, audience binding,
// expiry, and process-lifetime replay state before local execution. The gateway remains the
// authority for agent identity and policy; the audience value derived locally
// is used only to reject commands signed for a different certificate.
type CommandValidator struct {
	mu         sync.Mutex
	publicKey  *rsa.PublicKey
	keyID      string
	audience   string
	seenNonces map[string]struct{}
	now        func() time.Time
}

// NewCommandValidator creates a validator with in-memory replay protection.
// The nonce history is deliberately discarded when the client exits.
func NewCommandValidator(publicKey *rsa.PublicKey, keyID, audience string) (*CommandValidator, error) {
	if publicKey == nil || strings.TrimSpace(keyID) == "" || strings.TrimSpace(audience) == "" {
		return nil, fmt.Errorf("command verification key, key ID, and audience are required")
	}
	return &CommandValidator{
		publicKey:  publicKey,
		keyID:      keyID,
		audience:   audience,
		seenNonces: make(map[string]struct{}),
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

// HandleCommand validates and executes a command.
func HandleCommand(cmd CommandEnvelope, validator *CommandValidator) (CommandDecision, error) {
	return handleCommand(context.Background(), cmd, validator, nil)
}

// HandleCommandWithTransferPlane validates and executes a command with the
// authenticated gateway data plane available to transfer commands.
func HandleCommandWithTransferPlane(ctx context.Context, cmd CommandEnvelope, validator *CommandValidator, plane clientfs.TransferPlane) (CommandDecision, error) {
	return handleCommand(ctx, cmd, validator, plane)
}

func handleCommand(ctx context.Context, cmd CommandEnvelope, validator *CommandValidator, plane clientfs.TransferPlane) (CommandDecision, error) {
	hostname, _ := osHostname()
	decision := CommandDecision{
		Result: CommandResultPayload{
			CommandID:      cmd.CommandID,
			Type:           cmd.Type,
			RespondedAt:    time.Now().UTC(),
			ClientHostname: strings.TrimSpace(hostname),
		},
	}

	if reason := validateCommand(cmd, validator); reason != "" {
		decision.Result.Status = CommandStatusRejected
		decision.Result.Reason = reason
		return decision, nil
	}

	outcome := executeAllowedCommand(ctx, cmd, plane)
	decision.Result.Status = CommandStatusExecuted
	decision.Result.Reason = outcome.reason
	decision.Result.OutputData = outcome.outputData
	decision.Result.TerminalSessionID = outcome.terminalMetadata.SessionID
	decision.Result.TerminalShell = outcome.terminalMetadata.Shell
	decision.Result.TerminalCommand = outcome.terminalMetadata.Command
	decision.Result.TerminalWorkingDirectory = outcome.terminalMetadata.WorkingDirectory
	decision.Result.TerminalExitCode = outcome.terminalMetadata.ExitCode
	decision.Result.Result = outcome.resultData
	return decision, nil
}

func validateCommand(cmd CommandEnvelope, validator *CommandValidator) string {
	if validator == nil {
		return "command validator unavailable"
	}
	if reason := validateCommandShape(cmd, validator); reason != "" {
		return reason
	}
	if reason := validateCommandWindow(cmd, validator.now()); reason != "" {
		return reason
	}
	if !hasValidCommandSignature(cmd, validator.publicKey) {
		return "invalid command signature"
	}
	return validator.recordVerifiedNonce(cmd.Nonce)
}

func hasValidCommandSignature(cmd CommandEnvelope, publicKey *rsa.PublicKey) bool {
	return commandauth.Verify(toAuthEnvelope(cmd), publicKey) == nil
}

func validateCommandShape(cmd CommandEnvelope, validator *CommandValidator) string {
	if strings.TrimSpace(cmd.CommandID) == "" {
		return "missing command_id"
	}
	if strings.TrimSpace(string(cmd.Type)) == "" {
		return "missing command type"
	}
	if _, ok := allowedCommandTypes[cmd.Type]; !ok {
		return fmt.Sprintf("command type %q is not allowed", cmd.Type)
	}
	if cmd.ProtocolVersion != commandauth.ProtocolVersion {
		return "unsupported command protocol version"
	}
	if cmd.AudienceAgentID != validator.audience {
		return "command audience mismatch"
	}
	if cmd.KeyID != validator.keyID {
		return "command signing key mismatch"
	}
	if strings.TrimSpace(cmd.Nonce) == "" || strings.TrimSpace(cmd.Signature) == "" {
		return "missing command authenticity fields"
	}
	return ""
}

func validateCommandWindow(cmd CommandEnvelope, now time.Time) string {
	if cmd.IssuedAt.IsZero() || cmd.ExpiresAt.IsZero() || !cmd.ExpiresAt.After(cmd.IssuedAt) {
		return "invalid command validity window"
	}
	if now.After(cmd.ExpiresAt) {
		return "command expired"
	}
	if cmd.IssuedAt.After(now.Add(commandClockSkew)) {
		return "command issued_at is in the future"
	}
	if cmd.ExpiresAt.Sub(cmd.IssuedAt) > commandExpiry {
		return "command validity window exceeds limit"
	}
	return ""
}

func (validator *CommandValidator) recordVerifiedNonce(nonce string) string {
	validator.mu.Lock()
	defer validator.mu.Unlock()
	if _, exists := validator.seenNonces[nonce]; exists {
		return "command replay detected"
	}
	validator.seenNonces[nonce] = struct{}{}
	return ""
}

func toAuthEnvelope(cmd CommandEnvelope) commandauth.Envelope {
	return commandauth.Envelope{
		ProtocolVersion: cmd.ProtocolVersion,
		CommandID:       cmd.CommandID,
		AudienceAgentID: cmd.AudienceAgentID,
		Type:            string(cmd.Type),
		Payload:         cmd.Payload,
		RequestedBy:     cmd.RequestedBy,
		IssuedAt:        cmd.IssuedAt,
		ExpiresAt:       cmd.ExpiresAt,
		Nonce:           cmd.Nonce,
		Reason:          cmd.Reason,
		KeyID:           cmd.KeyID,
		Signature:       cmd.Signature,
	}
}

func executeAllowedCommand(ctx context.Context, cmd CommandEnvelope, plane clientfs.TransferPlane) commandOutcome {
	switch cmd.Type {
	case CommandTypeNotice:
		return commandOutcome{reason: "support notice acknowledged"}
	case CommandTypeRequestScreenshot:
		data, err := CaptureScreenshot()
		if err != nil {
			return commandOutcome{reason: fmt.Sprintf("screenshot failed: %v", err)}
		}
		return commandOutcome{reason: "screenshot captured", outputData: data}
	case CommandTypeStartScreenStream:
		return commandOutcome{reason: "screen stream start acknowledged"}
	case CommandTypeStopScreenStream:
		return commandOutcome{reason: "screen stream stop acknowledged"}
	case CommandTypeTerminalRun:
		return executeTerminalCommand(cmd.Payload)
	case CommandTypeFilesRootsList, CommandTypeFilesDirectoryList, CommandTypeFilesDirectorySearch, CommandTypeFilesMetadataGet,
		CommandTypeFilesPreviewRead, CommandTypeFilesOperationExecute,
		CommandTypeFilesTransferPrepare, CommandTypeFilesTransferResume, CommandTypeFilesTransferAbort:
		return executeFileCommand(ctx, cmd.Type, cmd.Payload, plane)
	default:
		return commandOutcome{reason: "no-op"}
	}
}

var osHostname = os.Hostname
