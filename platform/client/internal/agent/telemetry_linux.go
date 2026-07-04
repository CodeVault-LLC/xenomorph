//go:build linux

package agent

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func collectSystemTelemetry() SystemTelemetry {
	ramUsage, totalRAMBytes := linuxRAMUsage()
	networkName, networkAddresses := linuxNetwork()
	return SystemTelemetry{
		OSVersion:        linuxOSVersion(),
		CPULoad:          linuxLoadAverage(),
		RAMUsage:         ramUsage,
		UptimeSeconds:    linuxUptimeSeconds(),
		CPUModel:         linuxCPUModel(),
		CPUCores:         int32(linuxCPUCores()),
		CPUThreads:       int32(runtime.NumCPU()),
		TotalRAMBytes:    totalRAMBytes,
		GPUDevices:       linuxGPUDevices(),
		NetworkName:      networkName,
		NetworkAddresses: networkAddresses,
		KernelVersion:    linuxKernelVersion(),
	}
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

func linuxRAMUsage() (float64, uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
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
		return 0, 0
	}

	return clampRatio((total - available) / total), uint64(total * 1024)
}

func linuxUptimeSeconds() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || uptime < 0 {
		return 0
	}

	return uint64(uptime)
}

func linuxCPUModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "model name", "Hardware", "Processor":
			model := strings.TrimSpace(value)
			if model != "" {
				return model
			}
		}
	}

	return ""
}

func linuxCPUCores() int {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return runtime.NumCPU()
	}
	defer func() { _ = file.Close() }()

	coreIDs := make(map[string]struct{})
	physicalID := "0"
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			physicalID = "0"
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "physical id":
			physicalID = strings.TrimSpace(value)
		case "core id":
			coreID := strings.TrimSpace(value)
			if coreID != "" {
				coreIDs[physicalID+":"+coreID] = struct{}{}
			}
		}
	}

	if len(coreIDs) == 0 {
		return runtime.NumCPU()
	}
	return len(coreIDs)
}

func linuxGPUDevices() []string {
	paths, err := filepath.Glob("/sys/class/drm/card*/device")
	if err != nil {
		return nil
	}

	devices := make([]string, 0, len(paths))
	seen := make(map[string]struct{})
	for _, path := range paths {
		if strings.Contains(filepath.Base(filepath.Dir(path)), "-") {
			continue
		}

		vendor := readTrimmed(filepath.Join(path, "vendor"))
		device := readTrimmed(filepath.Join(path, "device"))
		if vendor == "" && device == "" {
			continue
		}

		label := strings.TrimSpace(strings.Join([]string{vendor, device}, " "))
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		devices = append(devices, label)
	}

	return devices
}

func linuxNetwork() (string, []string) {
	ifaceName := linuxDefaultInterface()
	if ifaceName == "" {
		return "", nil
	}

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return ifaceName, nil
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return ifaceName, nil
	}

	addresses := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		address := addr.String()
		if strings.TrimSpace(address) != "" {
			addresses = append(addresses, address)
		}
	}

	if _, err := os.Stat(filepath.Join("/sys/class/net", ifaceName, "wireless")); err == nil {
		ifaceName += " (wireless)"
	}

	return ifaceName, addresses
}

func linuxDefaultInterface() string {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[1] != "00000000" {
			continue
		}
		if fields[0] != "Iface" {
			return fields[0]
		}
	}

	return ""
}

func linuxKernelVersion() string {
	return readTrimmed("/proc/sys/kernel/osrelease")
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
