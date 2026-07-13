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
	OSVersion             string
	CPULoad               float64
	RAMUsage              float64
	UptimeSeconds         uint64
	CPUModel              string
	CPUCores              int32
	CPUThreads            int32
	TotalRAMBytes         uint64
	GPUDevices            []string
	NetworkName           string
	NetworkAddresses      []string
	KernelVersion         string
	CPUFrequencyMHz       uint64
	NetworkOnline         bool
	NetworkLinkSpeedMbps  uint64
	NetworkType           string
	TotalStorageBytes     uint64
	AvailableStorageBytes uint64
	UsedStorageBytes      uint64
	StorageUsage          float64
	StorageInodeUsage     float64
	StorageDevice         string
	StorageFilesystem     string
	StorageMountpoint     string
	StorageModel          string
	StorageType           string
	StorageReadOnly       bool
	ApplicationTypes      []ApplicationTypeUsage
	NetworkSSID           string
}

// BuildHeartbeatPayload constructs the heartbeat payload from system telemetry.
func BuildHeartbeatPayload(provider HostnameProvider) HeartbeatPayload {
	if provider == nil {
		provider = os.Hostname
	}

	hostname := resolveHostname(provider)
	telemetry := collectSystemTelemetry()

	return HeartbeatPayload{
		Hostname:              hostname,
		OsVersion:             telemetry.OSVersion,
		CPULoad:               telemetry.CPULoad,
		RAMUsage:              telemetry.RAMUsage,
		UptimeSeconds:         telemetry.UptimeSeconds,
		CPUModel:              telemetry.CPUModel,
		CPUCores:              telemetry.CPUCores,
		CPUThreads:            telemetry.CPUThreads,
		TotalRAMBytes:         telemetry.TotalRAMBytes,
		GPUDevices:            telemetry.GPUDevices,
		NetworkName:           telemetry.NetworkName,
		NetworkAddresses:      telemetry.NetworkAddresses,
		KernelVersion:         telemetry.KernelVersion,
		CPUFrequencyMHz:       telemetry.CPUFrequencyMHz,
		NetworkOnline:         telemetry.NetworkOnline,
		NetworkLinkSpeedMbps:  telemetry.NetworkLinkSpeedMbps,
		NetworkType:           telemetry.NetworkType,
		TotalStorageBytes:     telemetry.TotalStorageBytes,
		AvailableStorageBytes: telemetry.AvailableStorageBytes,
		UsedStorageBytes:      telemetry.UsedStorageBytes,
		StorageUsage:          telemetry.StorageUsage,
		StorageInodeUsage:     telemetry.StorageInodeUsage,
		StorageDevice:         telemetry.StorageDevice,
		StorageFilesystem:     telemetry.StorageFilesystem,
		StorageMountpoint:     telemetry.StorageMountpoint,
		StorageModel:          telemetry.StorageModel,
		StorageType:           telemetry.StorageType,
		StorageReadOnly:       telemetry.StorageReadOnly,
		ApplicationTypes:      telemetry.ApplicationTypes,
		NetworkSSID:           telemetry.NetworkSSID,
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
