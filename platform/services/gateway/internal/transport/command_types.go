package transport

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
