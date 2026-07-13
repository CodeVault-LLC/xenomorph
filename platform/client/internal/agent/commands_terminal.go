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

const (
	terminalCommandTimeout time.Duration = 30 * time.Second
	terminalOutputLimit    int           = 128 * 1024
	cdCommandArgCount      int           = 2
)

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
	switch ShellName(strings.ToLower(shellName)) {
	case ShellPowerShell, ShellPowerShellCore:
		binary := "powershell.exe"
		if ShellName(shellName) == ShellPowerShellCore {
			binary = "pwsh"
		}
		return exec.CommandContext(ctx, binary, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command)
	case ShellCmd:
		return exec.CommandContext(ctx, "cmd.exe", "/C", command)
	case ShellZsh:
		return exec.CommandContext(ctx, "zsh", "-lc", command)
	case ShellBash:
		return exec.CommandContext(ctx, "bash", "-lc", command)
	default:
		return exec.CommandContext(ctx, "sh", "-lc", command)
	}
}

func defaultShellName() string {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("pwsh"); err == nil {
			return string(ShellPowerShellCore)
		}
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return string(ShellPowerShell)
		}
		return string(ShellCmd)
	}
	if shell := filepath.Base(os.Getenv("SHELL")); shell != "" {
		switch ShellName(shell) {
		case ShellBash, ShellZsh, ShellSh:
			return shell
		}
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return string(ShellBash)
	}
	return string(ShellSh)
}

func normalizeShellName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "powershell", "powershell.exe", "windows powershell":
		return string(ShellPowerShell)
	case "pwsh", "powershell core":
		return string(ShellPowerShellCore)
	case "cmd", "cmd.exe":
		return string(ShellCmd)
	case "zsh":
		return string(ShellZsh)
	case "bash":
		return string(ShellBash)
	default:
		return string(ShellSh)
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
	if len(fields) > cdCommandArgCount {
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
