//go:build windows

package collectors

import (
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

	// Get memory info using PowerShell
	script := `
$os = Get-CimInstance Win32_OperatingSystem
$cs = Get-CimInstance Win32_ComputerSystem
$cs.TotalPhysicalMemory
$os.FreePhysicalMemory * 1024
$os.TotalVirtualMemorySize * 1024
$os.FreeVirtualMemory * 1024
`
	out, err := runPowerShell(script)
	if err == nil {
		lines := strings.Split(out, "\n")
		if len(lines) >= 1 {
			info.Total, _ = strconv.ParseUint(strings.TrimSpace(lines[0]), 10, 64)
		}
		if len(lines) >= 2 {
			info.Free, _ = strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
			info.Available = info.Free
		}
		if len(lines) >= 3 {
			info.SwapTotal, _ = strconv.ParseUint(strings.TrimSpace(lines[2]), 10, 64)
		}
		if len(lines) >= 4 {
			info.SwapFree, _ = strconv.ParseUint(strings.TrimSpace(lines[3]), 10, 64)
		}
	}

	info.Used = info.Total - info.Free
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}

	info.SwapUsed = info.SwapTotal - info.SwapFree
	if info.SwapTotal > 0 {
		info.SwapPercent = float64(info.SwapUsed) / float64(info.SwapTotal) * 100
	}

	return info, nil
}
