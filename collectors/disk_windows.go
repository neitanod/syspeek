//go:build windows

package collectors

import (
	"sync"
	"time"

	gpsdisk "github.com/shirou/gopsutil/v3/disk"
)

type Partition struct {
	Device      string  `json:"device"`
	MountPoint  string  `json:"mountPoint"`
	FSType      string  `json:"fsType"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"usedPercent"`
}

type DiskIO struct {
	Device     string `json:"device"`
	ReadBytes  uint64 `json:"readBytes"`
	WriteBytes uint64 `json:"writeBytes"`
	ReadSpeed  uint64 `json:"readSpeed"`
	WriteSpeed uint64 `json:"writeSpeed"`
}

type DiskInfo struct {
	Partitions []Partition `json:"partitions"`
	IO         []DiskIO    `json:"io,omitempty"`
}

type diskIOSnapshot struct {
	readBytes  uint64
	writeBytes uint64
	at         time.Time
}

var (
	prevDiskIO   = map[string]diskIOSnapshot{}
	prevDiskIOMu sync.Mutex
)

func GetDiskInfo() (DiskInfo, error) {
	info := DiskInfo{}

	parts, err := gpsdisk.Partitions(false)
	if err != nil {
		return info, err
	}

	for _, p := range parts {
		usage, err := gpsdisk.Usage(p.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}
		info.Partitions = append(info.Partitions, Partition{
			Device:      p.Device,
			MountPoint:  p.Mountpoint,
			FSType:      p.Fstype,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
		})
	}

	// I/O counters per physical disk. Derive per-second speed by diffing
	// against the previous snapshot.
	io, err := gpsdisk.IOCounters()
	if err == nil {
		now := time.Now()
		prevDiskIOMu.Lock()
		for name, c := range io {
			cur := diskIOSnapshot{readBytes: c.ReadBytes, writeBytes: c.WriteBytes, at: now}
			row := DiskIO{
				Device:     name,
				ReadBytes:  c.ReadBytes,
				WriteBytes: c.WriteBytes,
			}
			if prev, ok := prevDiskIO[name]; ok {
				elapsed := now.Sub(prev.at).Seconds()
				if elapsed > 0 {
					if c.ReadBytes >= prev.readBytes {
						row.ReadSpeed = uint64(float64(c.ReadBytes-prev.readBytes) / elapsed)
					}
					if c.WriteBytes >= prev.writeBytes {
						row.WriteSpeed = uint64(float64(c.WriteBytes-prev.writeBytes) / elapsed)
					}
				}
			}
			prevDiskIO[name] = cur
			info.IO = append(info.IO, row)
		}
		prevDiskIOMu.Unlock()
	}

	return info, nil
}
