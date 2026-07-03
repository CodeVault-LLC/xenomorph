package agent

import "time"

const (
	heartbeatResponseSize = 4096
	commandResponseSize   = 8192
	commandExpiry         = 2 * time.Minute
	stateDirPermission    = 0700
	stateFilePermission   = 0600
	maxInstalledApps      = 200
)

const unknownHostname = "unknown"
