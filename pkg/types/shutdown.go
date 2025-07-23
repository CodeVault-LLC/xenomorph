package types

type ShutdownReason int

const (
	ShutdownUnknown ShutdownReason = iota
	ShutdownManual
	ShutdownSigInt
	ShutdownSigTerm
	ShutdownSystemSleep
	ShutdownNetworkLoss
	ShutdownServerClosed
	ShutdownError
)

func (r ShutdownReason) String() string {
	switch r {
	case ShutdownManual:
		return "manual"
	case ShutdownSigInt:
		return "sigint"
	case ShutdownSigTerm:
		return "sigterm"
	case ShutdownSystemSleep:
		return "system_sleep"
	case ShutdownNetworkLoss:
		return "network_loss"
	case ShutdownServerClosed:
		return "server_closed"
	case ShutdownError:
		return "error"
	default:
		return "unknown"
	}
}

type DisconnectData struct {
	Reason   string // e.g. "manual", "sigint", "error"
	Level    string // same as Reason or specific severity
	Uptime   string // e.g. "5m30s"
	TS       string // ISO8601 e.g. "2025-07-23T14:00:00Z"
	Hostname string
	Message  string // optional user note
	Error    string // optional error details
}