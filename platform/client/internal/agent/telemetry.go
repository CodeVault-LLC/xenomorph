package agent

import (
	"os"
	"runtime"
	"strings"
)

// HostnameProvider returns the system hostname.
type HostnameProvider func() (string, error)

// BuildHeartbeatPayload constructs the heartbeat payload from system telemetry.
func BuildHeartbeatPayload(provider HostnameProvider) HeartbeatPayload {
	if provider == nil {
		provider = os.Hostname
	}

	hostname := resolveHostname(provider)

	return HeartbeatPayload{
		Hostname:  hostname,
		OsVersion: runtime.GOOS + "/" + runtime.GOARCH,
	}
}

func resolveHostname(provider HostnameProvider) string {
	hostname, err := provider()
	if err != nil {
		return unknownHostname
	}

	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return unknownHostname
	}

	return hostname
}
