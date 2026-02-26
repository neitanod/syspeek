//go:build windows

package collectors

import (
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

	// Get disk info using PowerShell
	script := `
Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | ForEach-Object {
    "$($_.DeviceID)|$($_.FileSystem)|$($_.Size)|$($_.FreeSpace)"
}
`
	out, err := runPowerShell(script)
	if err != nil {
		return info, err
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		total, _ := strconv.ParseUint(parts[2], 10, 64)
		free, _ := strconv.ParseUint(parts[3], 10, 64)

		if total == 0 {
			continue
		}

		part := Partition{
			Device:     parts[0],
			MountPoint: parts[0] + "\\",
			FSType:     parts[1],
			Total:      total,
			Free:       free,
			Used:       total - free,
		}
		part.UsedPercent = float64(part.Used) / float64(part.Total) * 100

		info.Partitions = append(info.Partitions, part)
	}

	// Get disk I/O stats
	ioScript := `
Get-CimInstance Win32_PerfFormattedData_PerfDisk_LogicalDisk | Where-Object { $_.Name -ne '_Total' -and $_.Name -match '^[A-Z]:$' } | ForEach-Object {
    "$($_.Name)|$($_.DiskReadBytesPerSec)|$($_.DiskWriteBytesPerSec)"
}
`
	ioOut, err := runPowerShell(ioScript)
	if err == nil {
		lines := strings.Split(ioOut, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				readSpeed, _ := strconv.ParseUint(parts[1], 10, 64)
				writeSpeed, _ := strconv.ParseUint(parts[2], 10, 64)
				info.IO = append(info.IO, DiskIO{
					Device:     parts[0],
					ReadSpeed:  readSpeed,
					WriteSpeed: writeSpeed,
				})
			}
		}
	}

	return info, nil
}
