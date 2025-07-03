package system

import (
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/shirou/gopsutil/disk"
)

func GetDisks() ([]types.Disks, error) {
	partitions, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}

	var disks []types.Disks

	for _, p := range partitions {
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue 
		}

		disks = append(disks, types.Disks{
			Name:       p.Device,
			TotalSize:  int64(usage.Total),
			FreeSize:   int64(usage.Free),
			UsedSize:   int64(usage.Used),
			MountPoint: p.Mountpoint,
			FileSystem: usage.Fstype,
		})
	}

	return disks, nil
}