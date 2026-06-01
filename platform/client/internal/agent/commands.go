package agent

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var allowedCommandTypes = map[string]struct{}{
	"support.notice":             {},
	"support.request_screenshot": {},
}

// CommandDecision contains the result of processing a command.
type CommandDecision struct {
	Result CommandResultPayload
}

type commandOutcome struct {
	reason     string
	outputData []byte
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
		decision.Result.Status = "rejected"
		decision.Result.Reason = reason
		return decision, nil
	}

	outcome := executeAllowedCommand(cmd)
	decision.Result.Status = "executed"
	decision.Result.Reason = outcome.reason
	decision.Result.OutputData = outcome.outputData
	return decision, nil
}

func validateCommand(cmd CommandEnvelope) string {
	if strings.TrimSpace(cmd.CommandID) == "" {
		return "missing command_id"
	}
	if strings.TrimSpace(cmd.Type) == "" {
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
	case "support.notice":
		return commandOutcome{reason: "support notice acknowledged"}
	case "support.request_screenshot":
		data, err := captureScreenshot()
		if err != nil {
			return commandOutcome{reason: fmt.Sprintf("screenshot failed: %v", err)}
		}
		return commandOutcome{reason: "screenshot captured", outputData: data}
	default:
		return commandOutcome{reason: "no-op"}
	}
}

var osHostname = os.Hostname
