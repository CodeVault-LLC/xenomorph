package transport

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maximumCanonicalCommandResultBytes = 10 << 20
	commandResultStatusExecuted        = "executed"
	commandResultStatusRejected        = "rejected"
	commandResultStatusOutcomeUnknown  = "outcome_unknown"
)

type canonicalCommandResult struct {
	CommandID                string `json:"command_id"`
	Type                     string `json:"type"`
	Status                   string `json:"status"`
	Reason                   string `json:"reason"`
	ClientHostname           string `json:"client_hostname"`
	Result                   []byte `json:"result"`
	TerminalSessionID        string `json:"terminal_session_id"`
	TerminalShell            string `json:"terminal_shell"`
	TerminalWorkingDirectory string `json:"terminal_working_directory"`
	TerminalExitCode         int    `json:"terminal_exit_code"`
}

func canonicalizeCommandResult(request commandResultRequest) ([]byte, error) {
	if err := validateCanonicalCommandResult(request); err != nil {
		return nil, err
	}

	result := append([]byte(nil), request.OutputData...)
	if len(result) == 0 {
		result = append(result, request.Result...)
	}

	encoded, err := json.Marshal(canonicalCommandResult{
		CommandID: request.CommandID, Type: request.Type, Status: request.Status,
		Reason: request.Reason, ClientHostname: request.ClientHostname, Result: result,
		TerminalSessionID: request.TerminalSessionID, TerminalShell: request.TerminalShell,
		TerminalWorkingDirectory: request.TerminalWorkingDirectory, TerminalExitCode: request.TerminalExitCode,
	})
	if err != nil {
		return nil, fmt.Errorf("encode canonical command result: %w", err)
	}

	return encoded, nil
}

func validateCanonicalCommandResult(request commandResultRequest) error {
	if strings.TrimSpace(request.CommandID) == "" || strings.TrimSpace(request.Type) == "" {
		return fmt.Errorf("validate canonical command result: command ID and type are required")
	}

	if !isRegisteredCommandResultStatus(request.Status) {
		return fmt.Errorf("validate canonical command result: unregistered status")
	}

	if err := validateCanonicalCommandResultLengths(request); err != nil {
		return err
	}

	if int64(request.TerminalExitCode) < -2147483648 || int64(request.TerminalExitCode) > 2147483647 {
		return fmt.Errorf("validate canonical command result: exit code exceeds protocol bound")
	}

	return nil
}

func isRegisteredCommandResultStatus(status string) bool {
	switch status {
	case commandResultStatusExecuted, commandResultStatusRejected, commandResultStatusOutcomeUnknown:
		return true
	default:
		return false
	}
}

func validateCanonicalCommandResultLengths(request commandResultRequest) error {
	if len(request.Type) > maxTypeLen || len(request.Reason) > maxReasonLen ||
		len(request.ClientHostname) > maxHostnameLen || len(request.TerminalSessionID) > maxCommandIDLen ||
		len(request.TerminalShell) > 32 || len(request.TerminalWorkingDirectory) > 4096 ||
		len(request.OutputData) > maximumCanonicalCommandResultBytes || len(request.Result) > maximumCanonicalCommandResultBytes {
		return fmt.Errorf("validate canonical command result: field exceeds protocol bound")
	}

	return nil
}
