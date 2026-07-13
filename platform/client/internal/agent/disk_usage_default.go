//go:build !linux && !windows && !darwin

package agent

func sampleDiskUsage(_ string) (diskUsage, bool) {
	return diskUsage{}, false
}

func collectStaticDiskTelemetry(path string) staticDiskTelemetry {
	return staticDiskTelemetry{mountpoint: path, driveType: "unknown"}
}
