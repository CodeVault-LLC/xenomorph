//go:build darwin

package agent

import (
	"strings"

	"golang.org/x/sys/unix"
)

func sampleDiskUsage(path string) (diskUsage, bool) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil || stat.Bsize == 0 {
		return diskUsage{}, false
	}
	blockSize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * blockSize
	availableBytes := stat.Bavail * blockSize
	usedBytes := (stat.Blocks - stat.Bfree) * blockSize
	return diskUsage{
		totalBytes:     totalBytes,
		availableBytes: availableBytes,
		usedBytes:      usedBytes,
		usage:          portableRatio(usedBytes, totalBytes),
		inodeUsage:     portableRatio(stat.Files-stat.Ffree, stat.Files),
	}, true
}

func collectStaticDiskTelemetry(path string) staticDiskTelemetry {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return staticDiskTelemetry{mountpoint: path, driveType: "unknown"}
	}
	return staticDiskTelemetry{
		device:     byteArrayString(stat.Mntfromname[:]),
		filesystem: byteArrayString(stat.Fstypename[:]),
		mountpoint: byteArrayString(stat.Mntonname[:]),
		driveType:  "unknown",
		readOnly:   stat.Flags&unix.MNT_RDONLY != 0,
	}
}

func byteArrayString(value []byte) string {
	if index := strings.IndexByte(string(value), 0); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(string(value))
}

func portableRatio(numerator, denominator uint64) float64 {
	if denominator == 0 || numerator > denominator {
		return 0
	}
	return clampRatio(float64(numerator) / float64(denominator))
}
