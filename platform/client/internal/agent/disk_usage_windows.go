//go:build windows

package agent

import (
	"golang.org/x/sys/windows"
)

const fileReadOnlyVolume = uint32(0x00080000)

func sampleDiskUsage(path string) (diskUsage, bool) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return diskUsage{}, false
	}
	var availableBytes uint64
	var totalBytes uint64
	var freeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(pathPointer, &availableBytes, &totalBytes, &freeBytes); err != nil {
		return diskUsage{}, false
	}
	usedBytes := totalBytes - freeBytes
	return diskUsage{
		totalBytes:     totalBytes,
		availableBytes: availableBytes,
		usedBytes:      usedBytes,
		usage:          windowsRatio(usedBytes, totalBytes),
	}, true
}

func collectStaticDiskTelemetry(path string) staticDiskTelemetry {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return staticDiskTelemetry{mountpoint: path, driveType: "unknown"}
	}
	filesystem := make([]uint16, windows.MAX_PATH+1)
	var flags uint32
	if err := windows.GetVolumeInformation(pathPointer, nil, 0, nil, nil, &flags, &filesystem[0], uint32(len(filesystem))); err != nil {
		return staticDiskTelemetry{device: path, mountpoint: path, driveType: windowsDriveType(pathPointer)}
	}
	return staticDiskTelemetry{
		device:     path,
		filesystem: windows.UTF16ToString(filesystem),
		mountpoint: path,
		driveType:  windowsDriveType(pathPointer),
		readOnly:   flags&fileReadOnlyVolume != 0,
	}
}

func windowsDriveType(path *uint16) string {
	switch windows.GetDriveType(path) {
	case windows.DRIVE_FIXED:
		return "fixed"
	case windows.DRIVE_REMOVABLE:
		return "removable"
	case windows.DRIVE_REMOTE:
		return "network"
	case windows.DRIVE_CDROM:
		return "optical"
	default:
		return "unknown"
	}
}

func windowsRatio(numerator, denominator uint64) float64 {
	if denominator == 0 || numerator > denominator {
		return 0
	}
	return clampRatio(float64(numerator) / float64(denominator))
}
