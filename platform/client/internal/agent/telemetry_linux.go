//go:build linux

package agent

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func collectSystemTelemetry() (osVersion string, cpuLoad float64, ramUsage float64) {
	osVersion = linuxOSVersion()
	cpuLoad = linuxLoadAverage()
	ramUsage = linuxRAMUsage()
	return osVersion, cpuLoad, ramUsage
}

func linuxOSVersion() string {
	name := ""
	version := ""

	file, err := os.Open("/etc/os-release")
	if err == nil {
		defer func() { _ = file.Close() }()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			key, value, ok := strings.Cut(scanner.Text(), "=")
			if !ok {
				continue
			}
			value = strings.Trim(value, `"`)
			switch key {
			case "PRETTY_NAME":
				name = value
			case "VERSION_ID":
				version = value
			}
		}
	}

	if name == "" {
		name = "Linux"
	}
	if version != "" && !strings.Contains(name, version) {
		name = fmt.Sprintf("%s %s", name, version)
	}

	return fmt.Sprintf("%s/%s", name, runtime.GOARCH)
}

func linuxLoadAverage() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}

	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	cpus := runtime.NumCPU()
	if cpus <= 0 {
		return 0
	}

	return clampRatio(load / float64(cpus))
}

func linuxRAMUsage() float64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()

	var total, available float64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}

		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			total = value
		case "MemAvailable":
			available = value
		}
	}

	if total <= 0 || available < 0 {
		return 0
	}

	return clampRatio((total - available) / total)
}
