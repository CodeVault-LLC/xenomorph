//go:build !linux

package agent

import "runtime"

func collectSystemTelemetry() SystemTelemetry {
	return SystemTelemetry{
		OSVersion:  runtime.GOOS + "/" + runtime.GOARCH,
		CPUThreads: int32(runtime.NumCPU()),
	}
}
