package agent

import "time"

const (
	httpTimeout           = 10 * time.Second
	pollInterval          = 5 * time.Second
	heartbeatResponseSize = 4096
	commandResponseSize   = 8192
	commandExpiry         = 2 * time.Minute
	stateDirPermission    = 0700
	stateFilePermission   = 0600
	maxInstalledApps      = 200
	maxReportedBrowsers   = 32
	hostnameMaxLength     = 120
	osVersionMaxLength    = 120
	browserNameMaxLength  = 80
	binaryPathMaxLength   = 260
	profileDirMaxLength   = 260
	appNameMaxLength      = 120
)

const unknownHostname = "unknown"
