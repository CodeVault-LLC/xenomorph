package agent

import (
	"os"
	"strings"
)

// HostnameProvider returns the system hostname.
type HostnameProvider func() (string, error)

// SystemTelemetry contains client-authored host facts included in heartbeat
// payloads. These fields are operational labels, not identity evidence.
type SystemTelemetry struct {
	OSVersion        string
	CPULoad          float64
	RAMUsage         float64
	UptimeSeconds    uint64
	CPUModel         string
	CPUCores         int32
	CPUThreads       int32
	TotalRAMBytes    uint64
	GPUDevices       []string
	NetworkName      string
	NetworkAddresses []string
	KernelVersion    string
}

// BuildHeartbeatPayload constructs the heartbeat payload from system telemetry.
func BuildHeartbeatPayload(provider HostnameProvider) HeartbeatPayload {
	if provider == nil {
		provider = os.Hostname
	}

	hostname := resolveHostname(provider)
	telemetry := collectSystemTelemetry()

	return HeartbeatPayload{
		Hostname:         hostname,
		OsVersion:        telemetry.OSVersion,
		CPULoad:          telemetry.CPULoad,
		RAMUsage:         telemetry.RAMUsage,
		UptimeSeconds:    telemetry.UptimeSeconds,
		CPUModel:         telemetry.CPUModel,
		CPUCores:         telemetry.CPUCores,
		CPUThreads:       telemetry.CPUThreads,
		TotalRAMBytes:    telemetry.TotalRAMBytes,
		GPUDevices:       telemetry.GPUDevices,
		NetworkName:      telemetry.NetworkName,
		NetworkAddresses: telemetry.NetworkAddresses,
		KernelVersion:    telemetry.KernelVersion,
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
