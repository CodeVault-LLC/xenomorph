//go:build !linux

package agent

import "runtime"

func collectSystemTelemetry() (osVersion string, cpuLoad float64, ramUsage float64) {
	return runtime.GOOS + "/" + runtime.GOARCH, 0, 0
}
