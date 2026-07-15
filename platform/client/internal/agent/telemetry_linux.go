//go:build linux

package agent

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	telemetryValueFields = 2
	bytesPerKiB          = 1024
	megahertzPerGHz      = 1000
	pciIDLength          = 4
	pciScannerBuffer     = 1024
	pciScannerMaxToken   = 1024 * 1024
)

func collectSystemTelemetry() SystemTelemetry {
	ramUsage, totalRAMBytes := linuxRAMUsage()
	network := linuxNetwork()
	storage := collectDiskTelemetry()

	return SystemTelemetry{
		OSVersion:             linuxOSVersion(),
		CPULoad:               linuxLoadAverage(),
		RAMUsage:              ramUsage,
		UptimeSeconds:         linuxUptimeSeconds(),
		CPUModel:              linuxCPUModel(),
		CPUCores:              cpuCountInt32(linuxCPUCores()),
		CPUThreads:            cpuCountInt32(runtime.NumCPU()),
		TotalRAMBytes:         totalRAMBytes,
		GPUDevices:            linuxGPUDevices(),
		NetworkName:           network.name,
		NetworkAddresses:      network.addresses,
		KernelVersion:         linuxKernelVersion(),
		CPUFrequencyMHz:       linuxCPUFrequencyMHz(),
		NetworkOnline:         network.online,
		NetworkLinkSpeedMbps:  network.linkSpeedMbps,
		NetworkType:           network.networkType,
		TotalStorageBytes:     storage.totalBytes,
		AvailableStorageBytes: storage.availableBytes,
		UsedStorageBytes:      storage.usedBytes,
		StorageUsage:          storage.usage,
		StorageInodeUsage:     storage.inodeUsage,
		StorageDevice:         storage.device,
		StorageFilesystem:     storage.filesystem,
		StorageMountpoint:     storage.mountpoint,
		StorageModel:          storage.model,
		StorageType:           storage.driveType,
		StorageReadOnly:       storage.readOnly,
		ApplicationTypes:      storage.applicationTypes,
		NetworkSSID:           network.ssid,
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
		if len(fields) < telemetryValueFields {
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

	return clampRatio((total - available) / total), uint64(total * bytesPerKiB)
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

func linuxCPUFrequencyMHz() uint64 {
	if frequency := sysfsCPUFrequencyMHz(); frequency > 0 {
		return frequency
	}

	return procCPUFrequencyMHz()
}

func sysfsCPUFrequencyMHz() uint64 {
	paths, err := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq")
	if err != nil || len(paths) == 0 {
		return 0
	}

	var total uint64

	var count uint64

	for _, path := range paths {
		value, err := strconv.ParseUint(readTrimmed(path), 10, 64)
		if err != nil || value == 0 {
			continue
		}

		total += value / megahertzPerGHz
		count++
	}

	if count == 0 {
		return 0
	}

	return total / count
}

func procCPUFrequencyMHz() uint64 {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0
	}

	defer func() { _ = file.Close() }()

	var total float64

	var count uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok || strings.TrimSpace(key) != "cpu MHz" {
			continue
		}

		mhz, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil && mhz > 0 {
			total += mhz
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return uint64(total / float64(count))
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

func cpuCountInt32(count int) int32 {
	const maxInt32 = int(^uint32(0) >> 1)

	if count <= 0 {
		return 0
	}

	if count > maxInt32 {
		return int32(maxInt32)
	}

	return int32(count)
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

		label := linuxPCIDeviceName(vendor, device)
		if label == "" {
			label = strings.TrimSpace(strings.Join([]string{vendor, device}, " "))
		}

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

type linuxNetworkTelemetry struct {
	name          string
	addresses     []string
	online        bool
	linkSpeedMbps uint64
	networkType   string
	ssid          string
}

func linuxNetwork() linuxNetworkTelemetry {
	ifaceName := linuxDefaultInterface()
	if ifaceName == "" {
		return linuxNetworkTelemetry{}
	}

	telemetry := linuxNetworkTelemetry{
		name:          ifaceName,
		online:        readTrimmed(filepath.Join("/sys/class/net", ifaceName, "carrier")) == "1",
		linkSpeedMbps: linuxLinkSpeedMbps(ifaceName),
		networkType:   "ethernet",
	}
	if _, err := os.Stat(filepath.Join("/sys/class/net", ifaceName, "wireless")); err == nil {
		telemetry.networkType = "wireless"
		telemetry.name += " (wireless)"
		telemetry.ssid, telemetry.linkSpeedMbps = linuxWirelessLink(ifaceName)
	}

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return telemetry
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return telemetry
	}

	addresses := make([]string, 0, len(addrs))

	for _, addr := range addrs {
		address := addr.String()
		if strings.TrimSpace(address) != "" {
			addresses = append(addresses, address)
		}
	}

	telemetry.addresses = addresses

	return telemetry
}

func linuxWirelessLink(ifaceName string) (string, uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// #nosec G204 -- ifaceName originates from the local kernel interface enumeration.
	output, err := exec.CommandContext(ctx, "iw", "dev", ifaceName, "link").Output()
	if err != nil {
		return "", 0
	}

	ssid := ""

	var speedMbps uint64

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if value, ok := strings.CutPrefix(line, "SSID: "); ok {
			ssid = strings.TrimSpace(value)
			continue
		}

		if value, ok := strings.CutPrefix(line, "tx bitrate: "); ok {
			speedMbps = parseLinkSpeedMbps(value)
		}
	}

	return ssid, speedMbps
}

func parseLinkSpeedMbps(value string) uint64 {
	fields := strings.Fields(value)
	if len(fields) < telemetryValueFields {
		return 0
	}

	speed, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || speed <= 0 {
		return 0
	}

	switch strings.ToLower(fields[1]) {
	case "mbit/s", "mbps":
		return uint64(speed)
	case "gbit/s", "gbps":
		return uint64(speed * megahertzPerGHz)
	default:
		return 0
	}
}

func linuxPCIDeviceName(vendorID, deviceID string) string {
	vendorID = strings.TrimPrefix(strings.ToLower(vendorID), "0x")
	deviceID = strings.TrimPrefix(strings.ToLower(deviceID), "0x")

	if vendorID == "" || deviceID == "" {
		return ""
	}

	for _, path := range []string{"/usr/share/hwdata/pci.ids", "/usr/share/misc/pci.ids", "/usr/share/pci.ids"} {
		name := pciDeviceName(path, vendorID, deviceID)
		if name != "" {
			return name
		}
	}

	return ""
}

//nolint:cyclop // The PCI identifier format requires ordered classification of each input line.
func pciDeviceName(path, vendorID, deviceID string) string {
	// #nosec G304 -- callers use a fixed system PCI database path or a test fixture.
	file, err := os.Open(path)
	if err != nil {
		return ""
	}

	defer func() { _ = file.Close() }()

	vendorName := ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, pciScannerBuffer), pciScannerMaxToken)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		if strings.HasPrefix(line, "\t\t") {
			continue
		}

		if strings.HasPrefix(line, "\t") {
			if vendorName == "" || len(line) < pciIDLength+1 || strings.ToLower(line[1:pciIDLength+1]) != deviceID {
				continue
			}

			if name := strings.TrimSpace(line[5:]); name != "" {
				return vendorName + " " + name
			}

			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || strings.ToLower(fields[0]) != vendorID {
			vendorName = ""
			continue
		}

		vendorName = strings.TrimSpace(line[4:])
	}

	return ""
}

func linuxLinkSpeedMbps(ifaceName string) uint64 {
	speed, err := strconv.ParseUint(readTrimmed(filepath.Join("/sys/class/net", ifaceName, "speed")), 10, 64)
	if err != nil || speed == 0 || speed > 1_000_000 {
		return 0
	}

	return speed
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
	// #nosec G304 -- callers construct paths below fixed procfs and sysfs roots.
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
