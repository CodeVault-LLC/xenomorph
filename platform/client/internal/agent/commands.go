package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var allowedCommandTypes = map[string]struct{}{
	"support.notice":              {},
	"support.request_screenshot":  {},
	"support.start_screen_stream": {},
	"support.stop_screen_stream":  {},
	"support.terminal.run":        {},
}

const (
	terminalCommandTimeout = 30 * time.Second
	terminalOutputLimit    = 128 * 1024
)

// CommandDecision contains the result of processing a command.
type CommandDecision struct {
	Result CommandResultPayload
}

type commandOutcome struct {
	reason           string
	outputData       []byte
	terminalMetadata terminalResultMetadata
}

type terminalCommandPayload struct {
	SessionID        string `json:"session_id"`
	Command          string `json:"command"`
	Shell            string `json:"shell"`
	WorkingDirectory string `json:"working_directory"`
}

type terminalResultMetadata struct {
	SessionID        string
	Shell            string
	Command          string
	WorkingDirectory string
	ExitCode         int
}

type terminalSessionState struct {
	Shell            string
	WorkingDirectory string
}

type terminalRuntimeState struct {
	mu       sync.Mutex
	sessions map[string]terminalSessionState
}

var terminalRuntime = terminalRuntimeState{
	sessions: make(map[string]terminalSessionState),
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
		data, err := CaptureScreenshot()
		if err != nil {
			return commandOutcome{reason: fmt.Sprintf("screenshot failed: %v", err)}
		}
		return commandOutcome{reason: "screenshot captured", outputData: data}
	case "support.start_screen_stream":
		return commandOutcome{reason: "screen stream start acknowledged"}
	case "support.stop_screen_stream":
		return commandOutcome{reason: "screen stream stop acknowledged"}
	case "support.terminal.run":
		return executeTerminalCommand(cmd.Payload)
	default:
		return commandOutcome{reason: "no-op"}
	}
}

var osHostname = os.Hostname

func executeTerminalCommand(raw json.RawMessage) commandOutcome {
	var payload terminalCommandPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return commandOutcome{reason: fmt.Sprintf("terminal payload rejected: %v", err)}
	}

	payload.SessionID = strings.TrimSpace(payload.SessionID)
	payload.Command = strings.TrimSpace(payload.Command)
	if payload.SessionID == "" {
		return commandOutcome{reason: "terminal payload rejected: missing session_id"}
	}
	if payload.Command == "" {
		return commandOutcome{reason: "terminal payload rejected: missing command"}
	}

	state := terminalRuntime.session(payload)
	if changed, output, err := terminalRuntime.applyBuiltinCD(payload.SessionID, payload.Command, state); changed {
		state = terminalRuntime.session(terminalCommandPayload{SessionID: payload.SessionID})
		return commandOutcome{
			reason:     reasonForTerminalExit(err),
			outputData: output,
			terminalMetadata: terminalResultMetadata{
				SessionID:        payload.SessionID,
				Shell:            state.Shell,
				Command:          payload.Command,
				WorkingDirectory: state.WorkingDirectory,
				ExitCode:         exitCodeForError(err),
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), terminalCommandTimeout)
	defer cancel()

	cmd := buildShellCommand(ctx, state.Shell, payload.Command)
	cmd.Dir = state.WorkingDirectory
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = ctx.Err()
	}

	return commandOutcome{
		reason:     reasonForTerminalExit(err),
		outputData: output.Bytes(),
		terminalMetadata: terminalResultMetadata{
			SessionID:        payload.SessionID,
			Shell:            state.Shell,
			Command:          payload.Command,
			WorkingDirectory: state.WorkingDirectory,
			ExitCode:         exitCodeForError(err),
		},
	}
}

func (s *terminalRuntimeState) session(payload terminalCommandPayload) terminalSessionState {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.sessions[payload.SessionID]
	if strings.TrimSpace(payload.Shell) != "" {
		state.Shell = normalizeShellName(payload.Shell)
	}
	if state.Shell == "" {
		state.Shell = defaultShellName()
	}
	if dir := strings.TrimSpace(payload.WorkingDirectory); dir != "" {
		state.WorkingDirectory = cleanWorkingDirectory(dir)
	}
	if state.WorkingDirectory == "" {
		state.WorkingDirectory = defaultWorkingDirectory()
	}
	s.sessions[payload.SessionID] = state
	return state
}

func (s *terminalRuntimeState) applyBuiltinCD(sessionID, command string, state terminalSessionState) (bool, []byte, error) {
	target, ok := parseCDCommand(command)
	if !ok {
		return false, nil, nil
	}

	if target == "" {
		target = defaultWorkingDirectory()
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(state.WorkingDirectory, target)
	}
	target = cleanWorkingDirectory(target)
	info, err := os.Stat(target)
	if err != nil {
		return true, []byte(err.Error() + "\n"), err
	}
	if !info.IsDir() {
		err := fmt.Errorf("%s is not a directory", target)
		return true, []byte(err.Error() + "\n"), err
	}

	s.mu.Lock()
	next := s.sessions[sessionID]
	next.Shell = state.Shell
	next.WorkingDirectory = target
	s.sessions[sessionID] = next
	s.mu.Unlock()
	return true, []byte(target + "\n"), nil
}

func buildShellCommand(ctx context.Context, shellName, command string) *exec.Cmd {
	switch strings.ToLower(shellName) {
	case "powershell", "pwsh":
		binary := "powershell.exe"
		if shellName == "pwsh" {
			binary = "pwsh"
		}
		return exec.CommandContext(ctx, binary, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command)
	case "cmd":
		return exec.CommandContext(ctx, "cmd.exe", "/C", command)
	case "zsh":
		return exec.CommandContext(ctx, "zsh", "-lc", command)
	case "bash":
		return exec.CommandContext(ctx, "bash", "-lc", command)
	default:
		return exec.CommandContext(ctx, "sh", "-lc", command)
	}
}

func defaultShellName() string {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh"
		}
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return "powershell"
		}
		return "cmd"
	}
	if shell := filepath.Base(os.Getenv("SHELL")); shell != "" {
		switch shell {
		case "bash", "zsh", "sh":
			return shell
		}
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "bash"
	}
	return "sh"
}

func normalizeShellName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "powershell", "powershell.exe", "windows powershell":
		return "powershell"
	case "pwsh", "powershell core":
		return "pwsh"
	case "cmd", "cmd.exe":
		return "cmd"
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	default:
		return "sh"
	}
}

func defaultWorkingDirectory() string {
	if dir, err := os.UserHomeDir(); err == nil && dir != "" {
		return cleanWorkingDirectory(dir)
	}
	if dir, err := os.Getwd(); err == nil && dir != "" {
		return cleanWorkingDirectory(dir)
	}
	return "."
}

func cleanWorkingDirectory(value string) string {
	if value == "" {
		return ""
	}
	if abs, err := filepath.Abs(value); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(value)
}

func parseCDCommand(command string) (string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 || strings.ToLower(fields[0]) != "cd" {
		return "", false
	}
	if len(fields) == 1 {
		return "", true
	}
	if len(fields) > 2 {
		return "", false
	}
	return strings.Trim(fields[1], `"'`), true
}

func reasonForTerminalExit(err error) string {
	if err == nil {
		return "terminal command completed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "terminal command timed out"
	}
	return fmt.Sprintf("terminal command failed: %v", err)
}

func exitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := terminalOutputLimit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buf.Write(p[:remaining])
			_, _ = b.buf.WriteString("\n[output truncated]\n")
		} else {
			_, _ = b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}
