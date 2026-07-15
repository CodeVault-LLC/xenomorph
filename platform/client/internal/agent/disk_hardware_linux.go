//go:build linux

package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func collectStaticDiskTelemetry(path string) staticDiskTelemetry {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return staticDiskTelemetry{mountpoint: path, driveType: "unknown"}
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := mountInfoSeparator(fields)

		if len(fields) < 6 || separator < 0 || len(fields) <= separator+2 || !sameMountpoint(decodeMountInfoPath(fields[4]), path) {
			continue
		}

		device := decodeMountInfoPath(fields[separator+2])
		model, driveType := diskHardware(device)

		return staticDiskTelemetry{
			device:     device,
			filesystem: fields[separator+1],
			mountpoint: path,
			model:      model,
			driveType:  driveType,
			readOnly:   hasMountOption(strings.Split(fields[5], ","), "ro"),
		}
	}

	return staticDiskTelemetry{mountpoint: path, driveType: "unknown"}
}

func mountInfoSeparator(fields []string) int {
	for index, field := range fields {
		if field == "-" {
			return index
		}
	}

	return -1
}

func decodeMountInfoPath(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func diskHardware(device string) (string, string) {
	deviceName := linuxParentBlockDevice(filepath.Base(device))
	if deviceName == "" {
		return "", "unknown"
	}

	vendor := readTrimmed(filepath.Join("/sys/class/block", deviceName, "device", "vendor"))
	model := readTrimmed(filepath.Join("/sys/class/block", deviceName, "device", "model"))
	label := strings.TrimSpace(strings.Join([]string{vendor, model}, " "))
	rotational := readTrimmed(filepath.Join("/sys/class/block", deviceName, "queue", "rotational"))

	switch rotational {
	case "0":
		return label, "solid-state"
	case "1":
		return label, "rotational"
	default:
		return label, "unknown"
	}
}

func linuxParentBlockDevice(deviceName string) string {
	resolved, err := filepath.EvalSymlinks(filepath.Join("/sys/class/block", deviceName))
	if err != nil {
		return deviceName
	}

	parent := filepath.Base(filepath.Dir(resolved))
	if parent == "block" {
		return deviceName
	}

	return parent
}
