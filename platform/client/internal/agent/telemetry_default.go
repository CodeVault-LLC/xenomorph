//go:build !linux

package agent

import "runtime"

func collectSystemTelemetry() SystemTelemetry {
	storage := collectDiskTelemetry()
	return SystemTelemetry{
		OSVersion:             runtime.GOOS + "/" + runtime.GOARCH,
		CPUThreads:            int32(runtime.NumCPU()),
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
	}
}
