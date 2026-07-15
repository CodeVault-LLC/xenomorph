//go:build linux

package agent

import "golang.org/x/sys/unix"

func sampleDiskUsage(path string) (diskUsage, bool) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil || stat.Bsize <= 0 {
		return diskUsage{}, false
	}

	blockSize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * blockSize
	availableBytes := stat.Bavail * blockSize
	usedBytes := (stat.Blocks - stat.Bfree) * blockSize
	usage := ratio(usedBytes, totalBytes)
	inodeUsage := ratio(stat.Files-stat.Ffree, stat.Files)

	return diskUsage{
		totalBytes:     totalBytes,
		availableBytes: availableBytes,
		usedBytes:      usedBytes,
		usage:          usage,
		inodeUsage:     inodeUsage,
	}, true
}

func ratio(numerator, denominator uint64) float64 {
	if denominator == 0 || numerator > denominator {
		return 0
	}

	return clampRatio(float64(numerator) / float64(denominator))
}
