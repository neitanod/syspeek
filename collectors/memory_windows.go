//go:build windows

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type MemoryInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	Available   uint64  `json:"available"`
	Cached      uint64  `json:"cached"`
	Buffers     uint64  `json:"buffers"`
	UsedPercent float64 `json:"usedPercent"`
	SwapTotal   uint64  `json:"swapTotal"`
	SwapUsed    uint64  `json:"swapUsed"`
	SwapFree    uint64  `json:"swapFree"`
	SwapPercent float64 `json:"swapPercent"`
}

func GetMemoryInfo() (MemoryInfo, error) {
	info := MemoryInfo{}

	// Get total memory
	if out, err := exec.Command("wmic", "computersystem", "get", "TotalPhysicalMemory", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "TotalPhysicalMemory=") {
				info.Total, _ = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "TotalPhysicalMemory=")), 10, 64)
				break
			}
		}
	}

	// Get free memory
	if out, err := exec.Command("wmic", "os", "get", "FreePhysicalMemory", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "FreePhysicalMemory=") {
				freeKB, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "FreePhysicalMemory=")), 10, 64)
				info.Free = freeKB * 1024
				info.Available = info.Free
				break
			}
		}
	}

	info.Used = info.Total - info.Free
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}

	// Get swap/page file info
	if out, err := exec.Command("wmic", "pagefile", "get", "CurrentUsage,AllocatedBaseSize", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "AllocatedBaseSize=") {
				sizeMB, _ := strconv.ParseUint(strings.TrimPrefix(line, "AllocatedBaseSize="), 10, 64)
				info.SwapTotal = sizeMB * 1024 * 1024
			} else if strings.HasPrefix(line, "CurrentUsage=") {
				usageMB, _ := strconv.ParseUint(strings.TrimPrefix(line, "CurrentUsage="), 10, 64)
				info.SwapUsed = usageMB * 1024 * 1024
			}
		}
		info.SwapFree = info.SwapTotal - info.SwapUsed
		if info.SwapTotal > 0 {
			info.SwapPercent = float64(info.SwapUsed) / float64(info.SwapTotal) * 100
		}
	}

	return info, nil
}
