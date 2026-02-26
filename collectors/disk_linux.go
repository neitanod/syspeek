//go:build linux

package collectors

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type DiskPartition struct {
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
	Partitions []DiskPartition `json:"partitions"`
	IO         []DiskIO        `json:"io"`
}

var previousDiskIO map[string]DiskIO
var diskMutex sync.Mutex

func init() {
	previousDiskIO = make(map[string]DiskIO)
}

func GetDiskInfo() (*DiskInfo, error) {
	info := &DiskInfo{
		Partitions: []DiskPartition{},
		IO:         []DiskIO{},
	}

	// Get mounted filesystems
	mounts, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer mounts.Close()

	// Track seen devices to avoid duplicates
	seenDevices := make(map[string]bool)

	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// Skip non-physical filesystems
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}

		// Skip snap and loop devices
		if strings.Contains(device, "loop") || strings.Contains(mountPoint, "/snap/") {
			continue
		}

		// Skip duplicates
		if seenDevices[device] {
			continue
		}
		seenDevices[device] = true

		partition := DiskPartition{
			Device:     device,
			MountPoint: mountPoint,
			FSType:     fsType,
		}

		// Get disk usage using statfs
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stat); err == nil {
			partition.Total = stat.Blocks * uint64(stat.Bsize)
			partition.Free = stat.Bavail * uint64(stat.Bsize)
			partition.Used = partition.Total - (stat.Bfree * uint64(stat.Bsize))

			if partition.Total > 0 {
				partition.UsedPercent = float64(partition.Used) / float64(partition.Total) * 100
			}
		}

		info.Partitions = append(info.Partitions, partition)
	}

	// Get disk I/O stats
	diskstats, err := os.Open("/proc/diskstats")
	if err == nil {
		defer diskstats.Close()

		scanner := bufio.NewScanner(diskstats)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 14 {
				continue
			}

			device := fields[2]

			// Skip partitions, only show whole disks
			// Also skip loop devices
			if strings.HasPrefix(device, "loop") || strings.HasPrefix(device, "dm-") {
				continue
			}

			// Check if this is a whole disk (no number at end) or important partition
			lastChar := device[len(device)-1]
			isPartition := lastChar >= '0' && lastChar <= '9'

			// Include whole disks and common partitions like nvme0n1p1
			if !isPartition && !strings.Contains(device, "nvme") {
				// It's a whole disk like sda, sdb
			} else if strings.Contains(device, "nvme") && strings.Contains(device, "n1") && !strings.Contains(device, "p") {
				// It's an NVMe disk like nvme0n1
			} else {
				continue
			}

			readSectors, _ := strconv.ParseUint(fields[5], 10, 64)
			writeSectors, _ := strconv.ParseUint(fields[9], 10, 64)

			// Sector size is typically 512 bytes
			readBytes := readSectors * 512
			writeBytes := writeSectors * 512

			io := DiskIO{
				Device:     device,
				ReadBytes:  readBytes,
				WriteBytes: writeBytes,
			}

			// Calculate speed based on previous reading
			diskMutex.Lock()
			if prev, exists := previousDiskIO[device]; exists {
				io.ReadSpeed = readBytes - prev.ReadBytes
				io.WriteSpeed = writeBytes - prev.WriteBytes
			}

			previousDiskIO[device] = DiskIO{
				Device:     device,
				ReadBytes:  readBytes,
				WriteBytes: writeBytes,
			}
			diskMutex.Unlock()

			info.IO = append(info.IO, io)
		}
	}

	return info, nil
}
