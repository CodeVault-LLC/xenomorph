package agent

import (
	"os"
	"time"
)

const (
	heartbeatResponseSize int64         = 4096
	commandResponseSize   int64         = 8192
	commandExpiry         time.Duration = 2 * time.Minute
	stateDirPermission    os.FileMode   = 0700
	stateFilePermission   os.FileMode   = 0600
	maxInstalledApps      int           = 200
)

const unknownHostname string = "unknown"

// CommandType is a typed command identifier exchanged between the gateway and
// the agent. Typed identifiers prevent accidental use of arbitrary strings as
// command types and centralize the supported command vocabulary.
type CommandType string

const (
	// CommandTypeNotice is a no-op support notice from the gateway.
	CommandTypeNotice CommandType = "support.notice"
	// CommandTypeRequestScreenshot requests a single screenshot from the agent.
	CommandTypeRequestScreenshot CommandType = "support.request_screenshot"
	// CommandTypeStartScreenStream requests that the agent begin streaming
	// screen frames to the gateway media plane.
	CommandTypeStartScreenStream CommandType = "support.start_screen_stream"
	// CommandTypeStopScreenStream requests that the agent stop streaming screen
	// frames.
	CommandTypeStopScreenStream CommandType = "support.stop_screen_stream"
	// CommandTypeTerminalRun requests execution of a shell command in a
	// terminal session.
	CommandTypeTerminalRun CommandType = "support.terminal.run"
)

// CommandStatus is a typed result status for command execution.
type CommandStatus string

const (
	// CommandStatusExecuted indicates the command was accepted and executed.
	CommandStatusExecuted CommandStatus = "executed"
	// CommandStatusRejected indicates the command failed validation.
	CommandStatusRejected CommandStatus = "rejected"
)

// ShellName is a normalized shell identifier used by the terminal executor.
type ShellName string

const (
	// ShellPowerShell is the Windows PowerShell shell.
	ShellPowerShell ShellName = "powershell"
	// ShellPowerShellCore is the cross-platform PowerShell Core shell.
	ShellPowerShellCore ShellName = "pwsh"
	// ShellCmd is the Windows Command Prompt shell.
	ShellCmd ShellName = "cmd"
	// ShellZsh is the Zsh shell.
	ShellZsh ShellName = "zsh"
	// ShellBash is the Bash shell.
	ShellBash ShellName = "bash"
	// ShellSh is the POSIX sh shell.
	ShellSh ShellName = "sh"
)
