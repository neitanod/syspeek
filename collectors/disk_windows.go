//go:build windows

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

	// Get disk info using WMIC
	out, err := exec.Command("wmic", "logicaldisk", "get", "DeviceID,FileSystem,FreeSpace,Size", "/value").Output()
	if err != nil {
		return info, err
	}

	// Parse output - it comes in blocks separated by blank lines
	blocks := strings.Split(string(out), "\r\n\r\n")
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		part := Partition{}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "DeviceID=") {
				part.Device = strings.TrimPrefix(line, "DeviceID=")
				part.MountPoint = part.Device
			} else if strings.HasPrefix(line, "FileSystem=") {
				part.FSType = strings.TrimPrefix(line, "FileSystem=")
			} else if strings.HasPrefix(line, "FreeSpace=") {
				part.Free, _ = strconv.ParseUint(strings.TrimPrefix(line, "FreeSpace="), 10, 64)
			} else if strings.HasPrefix(line, "Size=") {
				part.Total, _ = strconv.ParseUint(strings.TrimPrefix(line, "Size="), 10, 64)
			}
		}

		if part.Device != "" && part.Total > 0 {
			part.Used = part.Total - part.Free
			part.UsedPercent = float64(part.Used) / float64(part.Total) * 100
			info.Partitions = append(info.Partitions, part)
		}
	}

	return info, nil
}
