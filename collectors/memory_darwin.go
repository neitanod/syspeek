//go:build darwin

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
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		info.Total, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	}

	// Get memory pressure from vm_stat
	if out, err := exec.Command("vm_stat").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		var pageSize uint64 = 4096
		var free, active, inactive, wired, compressed uint64

		for _, line := range lines {
			if strings.HasPrefix(line, "Mach Virtual Memory Statistics") {
				continue
			}

			parts := strings.Split(line, ":")
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
			val, _ := strconv.ParseUint(value, 10, 64)

			switch key {
			case "Pages free":
				free = val * pageSize
			case "Pages active":
				active = val * pageSize
			case "Pages inactive":
				inactive = val * pageSize
			case "Pages wired down":
				wired = val * pageSize
			case "Pages occupied by compressor":
				compressed = val * pageSize
			}
		}

		info.Free = free
		info.Used = active + wired + compressed
		info.Available = free + inactive
		info.Cached = inactive

		if info.Total > 0 {
			info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
		}
	}

	// Get swap info
	if out, err := exec.Command("sysctl", "-n", "vm.swapusage").Output(); err == nil {
		// Format: total = 2048.00M  used = 1024.00M  free = 1024.00M
		str := string(out)
		for _, part := range strings.Split(str, "  ") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "total") {
				info.SwapTotal = parseMemSize(strings.TrimPrefix(part, "total = "))
			} else if strings.HasPrefix(part, "used") {
				info.SwapUsed = parseMemSize(strings.TrimPrefix(part, "used = "))
			} else if strings.HasPrefix(part, "free") {
				info.SwapFree = parseMemSize(strings.TrimPrefix(part, "free = "))
			}
		}
		if info.SwapTotal > 0 {
			info.SwapPercent = float64(info.SwapUsed) / float64(info.SwapTotal) * 100
		}
	}

	return info, nil
}

func parseMemSize(s string) uint64 {
	s = strings.TrimSpace(s)
	multiplier := uint64(1)

	if strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "G")
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "K")
	}

	val, _ := strconv.ParseFloat(s, 64)
	return uint64(val * float64(multiplier))
}
