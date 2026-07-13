package agent

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var allowedCommandTypes = map[CommandType]struct{}{
	CommandTypeNotice:            {},
	CommandTypeRequestScreenshot: {},
	CommandTypeStartScreenStream: {},
	CommandTypeStopScreenStream:  {},
	CommandTypeTerminalRun:       {},
}

// CommandDecision contains the result of processing a command.
type CommandDecision struct {
	Result CommandResultPayload
}

type commandOutcome struct {
	reason           string
	outputData       []byte
	terminalMetadata terminalResultMetadata
}

// HandleCommand validates and executes a command.
func HandleCommand(cmd CommandEnvelope) (CommandDecision, error) {
	hostname, _ := osHostname()
	decision := CommandDecision{
		Result: CommandResultPayload{
			CommandID:      cmd.CommandID,
			Type:           cmd.Type,
			RespondedAt:    time.Now().UTC(),
			ClientHostname: strings.TrimSpace(hostname),
		},
	}

	if reason := validateCommand(cmd); reason != "" {
		decision.Result.Status = CommandStatusRejected
		decision.Result.Reason = reason
		return decision, nil
	}

	outcome := executeAllowedCommand(cmd)
	decision.Result.Status = CommandStatusExecuted
	decision.Result.Reason = outcome.reason
	decision.Result.OutputData = outcome.outputData
	decision.Result.TerminalSessionID = outcome.terminalMetadata.SessionID
	decision.Result.TerminalShell = outcome.terminalMetadata.Shell
	decision.Result.TerminalCommand = outcome.terminalMetadata.Command
	decision.Result.TerminalWorkingDirectory = outcome.terminalMetadata.WorkingDirectory
	decision.Result.TerminalExitCode = outcome.terminalMetadata.ExitCode
	return decision, nil
}

func validateCommand(cmd CommandEnvelope) string {
	if strings.TrimSpace(cmd.CommandID) == "" {
		return "missing command_id"
	}
	if strings.TrimSpace(string(cmd.Type)) == "" {
		return "missing command type"
	}
	if _, ok := allowedCommandTypes[cmd.Type]; !ok {
		return fmt.Sprintf("command type %q is not allowed", cmd.Type)
	}

	now := time.Now().UTC()
	if !cmd.ExpiresAt.IsZero() && now.After(cmd.ExpiresAt) {
		return "command expired"
	}
	if !cmd.IssuedAt.IsZero() && cmd.IssuedAt.After(now.Add(commandExpiry)) {
		return "command issued_at is in the future"
	}
	if strings.TrimSpace(cmd.Signature) == "" {
		return "missing command signature"
	}

	return ""
}

func executeAllowedCommand(cmd CommandEnvelope) commandOutcome {
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
	default:
		return commandOutcome{reason: "no-op"}
	}
}

var osHostname = os.Hostname
