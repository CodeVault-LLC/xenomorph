package agent

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// CommandApprover prompts the user for command execution consent.
type CommandApprover interface {
	Approve(cmd CommandEnvelope) (bool, error)
}

var defaultApprover CommandApprover

var allowedCommandTypes = map[string]struct{}{
	"support.notice":             {},
	"support.request_screenshot": {},
}

// CommandDecision contains the result of processing a command with user consent.
type CommandDecision struct {
	Result        CommandResultPayload
	DisconnectNow bool
}

type commandOutcome struct {
	reason     string
	outputData []byte
}

// HandleCommandWithConsent validates, approves, and executes a command.
func HandleCommandWithConsent(cmd CommandEnvelope, approver CommandApprover, disconnectOnDeny bool) (CommandDecision, error) {
	if approver == nil {
		approver = defaultApprover
	}

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
		decision.Result.UserApproved = false
		decision.Result.DisconnectNow = false
		return decision, nil
	}

	approved, err := approver.Approve(cmd)
	if err != nil {
		decision.Result.Status = "failed"
		decision.Result.Reason = fmt.Sprintf("approval prompt failed: %v", err)
		decision.Result.UserApproved = false
		decision.Result.DisconnectNow = disconnectOnDeny
		decision.DisconnectNow = disconnectOnDeny
		return decision, nil
	}

	if !approved {
		decision.Result.Status = "denied"
		decision.Result.Reason = "user denied command"
		decision.Result.UserApproved = false
		decision.Result.DisconnectNow = disconnectOnDeny
		decision.DisconnectNow = disconnectOnDeny
		return decision, nil
	}

	outcome := executeAllowedCommand(cmd)
	decision.Result.Status = "executed"
	decision.Result.Reason = outcome.reason
	decision.Result.OutputData = outcome.outputData
	decision.Result.UserApproved = true
	decision.Result.DisconnectNow = false
	decision.DisconnectNow = false
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

// LoadDisconnectOnDenyFromEnv reads the disconnect-on-deny policy from the environment.
func LoadDisconnectOnDenyFromEnv() bool {
	raw := strings.TrimSpace(strings.ToLower(osGetenv("XENOMORPH_DISCONNECT_ON_DENY")))
	if raw == "" {
		return true
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return value
}

func firstNonEmpty(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "n/a"
	}
	return trimmed
}

var osHostname = os.Hostname
var osGetenv = os.Getenv
