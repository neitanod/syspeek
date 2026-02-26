//go:build darwin

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
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

func GetDiskInfo() (DiskInfo, error) {
	info := DiskInfo{}

	// Get disk usage using df
	out, err := exec.Command("df", "-k").Output()
	if err != nil {
		return info, err
	}

	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 { // Skip header
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		// Skip pseudo filesystems
		if !strings.HasPrefix(fields[0], "/dev") {
			continue
		}

		total, _ := strconv.ParseUint(fields[1], 10, 64)
		used, _ := strconv.ParseUint(fields[2], 10, 64)
		free, _ := strconv.ParseUint(fields[3], 10, 64)

		// df -k gives values in 1K blocks
		total *= 1024
		used *= 1024
		free *= 1024

		var usedPercent float64
		if total > 0 {
			usedPercent = float64(used) / float64(total) * 100
		}

		info.Partitions = append(info.Partitions, Partition{
			Device:      fields[0],
			MountPoint:  fields[len(fields)-1],
			FSType:      "apfs", // Most modern macOS uses APFS
			Total:       total,
			Used:        used,
			Free:        free,
			UsedPercent: usedPercent,
		})
	}

	return info, nil
}
