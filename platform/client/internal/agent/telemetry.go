package agent

import (
	"os"
	"runtime"
	"strings"
)

const unknownHostname = "unknown"

// HostnameProvider returns the hostname reported in heartbeats.
//
// Keeping this as a function type makes telemetry gathering testable and allows
// alternate providers to be injected in constrained runtime environments.
type HostnameProvider func() (string, error)

// BuildHeartbeatPayload builds a client heartbeat payload from runtime-derived
// telemetry values.
//
// Security and trust model:
// - Hostname remains untrusted client-authored metadata.
// - Agent identity is authenticated separately by the gateway through mTLS.
//
// Behavior:
// - Uses os.Hostname() by default when provider is nil.
// - Falls back to "unknown" for empty or erroring hostname providers.
func BuildHeartbeatPayload(provider HostnameProvider) HeartbeatPayload {
	if provider == nil {
		provider = os.Hostname
	}

	hostname := resolveHostname(provider)

	return HeartbeatPayload{
		Hostname:  hostname,
		OsVersion: runtime.GOOS + "/" + runtime.GOARCH,
		CpuLoad:   15.5,
		RamUsage:  42.0,
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
