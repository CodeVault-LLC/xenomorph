package agent

import (
	"os"
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
	osVersion, cpuLoad, ramUsage := collectSystemTelemetry()

	return HeartbeatPayload{
		Hostname:  hostname,
		OsVersion: osVersion,
		CPULoad:   cpuLoad,
		RAMUsage:  ramUsage,
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

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
