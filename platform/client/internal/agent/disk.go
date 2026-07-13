package agent

import (
	"strings"
	"sync"
)

type diskTelemetry struct {
	totalBytes       uint64
	availableBytes   uint64
	usedBytes        uint64
	usage            float64
	inodeUsage       float64
	device           string
	filesystem       string
	mountpoint       string
	model            string
	driveType        string
	readOnly         bool
	applicationTypes []ApplicationTypeUsage
}

type staticDiskTelemetry struct {
	device     string
	filesystem string
	mountpoint string
	model      string
	driveType  string
	readOnly   bool
}

type diskUsage struct {
	totalBytes     uint64
	availableBytes uint64
	usedBytes      uint64
	usage          float64
	inodeUsage     float64
}

// staticDiskCache holds bounded process-lifetime hardware and filesystem
// labels so heartbeats only sample volatile capacity counters.
var staticDiskCache = sync.OnceValue(func() staticDiskTelemetry {
	return collectStaticDiskTelemetry(systemDiskPath())
})

func collectDiskTelemetry() diskTelemetry {
	path := systemDiskPath()
	static := staticDiskCache()
	telemetry := diskTelemetry{
		device:           static.device,
		filesystem:       static.filesystem,
		mountpoint:       static.mountpoint,
		model:            static.model,
		driveType:        static.driveType,
		readOnly:         static.readOnly,
		applicationTypes: collectApplicationTypes(),
	}
	usage, ok := sampleDiskUsage(path)
	if !ok {
		return telemetry
	}
	telemetry.totalBytes = usage.totalBytes
	telemetry.availableBytes = usage.availableBytes
	telemetry.usedBytes = usage.usedBytes
	telemetry.usage = usage.usage
	telemetry.inodeUsage = usage.inodeUsage
	return telemetry
}

func sameMountpoint(left, right string) bool {
	return strings.EqualFold(strings.TrimRight(left, `/\`), strings.TrimRight(right, `/\`))
}

func hasMountOption(options []string, target string) bool {
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option), target) {
			return true
		}
	}
	return false
}
