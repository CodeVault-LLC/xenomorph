//go:build linux

package agent

import (
	"errors"
	"fmt"
	"os/exec"
)

func init() {
	defaultApprover = ZenityApprover{}
}

// ZenityApprover uses zenity dialogs for user consent prompts.
type ZenityApprover struct{}

// Approve displays a zenity dialog and returns the user's decision.
//
// #nosec G204 - command arguments are server-provided metadata, not user input.
func (z ZenityApprover) Approve(cmd CommandEnvelope) (bool, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return false, fmt.Errorf("zenity not found: %w", err)
	}

	question := fmt.Sprintf(
		"Remote support command request\n\nType: %s\nRequested By: %s\nReason: %s\nCommand ID: %s\n\nAccept this command?",
		firstNonEmpty(cmd.Type),
		firstNonEmpty(cmd.RequestedBy),
		firstNonEmpty(cmd.Reason),
		firstNonEmpty(cmd.CommandID),
	)

	c := exec.Command(
		"zenity",
		"--question",
		"--title", "Remote Support Request",
		"--width", "480",
		"--ok-label", "Allow",
		"--cancel-label", "Deny",
		"--text", question,
	)

	err := c.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}
