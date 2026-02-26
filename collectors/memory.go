package collectors

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type MemoryInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	Available   uint64  `json:"available"`
	Buffers     uint64  `json:"buffers"`
	Cached      uint64  `json:"cached"`
	SwapTotal   uint64  `json:"swapTotal"`
	SwapUsed    uint64  `json:"swapUsed"`
	SwapFree    uint64  `json:"swapFree"`
	UsedPercent float64 `json:"usedPercent"`
	SwapPercent float64 `json:"swapPercent"`
}

func GetMemoryInfo() (*MemoryInfo, error) {
	info := &MemoryInfo{}

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	memInfo := make(map[string]uint64)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, _ := strconv.ParseUint(fields[1], 10, 64)

		// Values in /proc/meminfo are in kB
		memInfo[key] = value * 1024 // Convert to bytes
	}

	info.Total = memInfo["MemTotal"]
	info.Free = memInfo["MemFree"]
	info.Available = memInfo["MemAvailable"]
	info.Buffers = memInfo["Buffers"]
	info.Cached = memInfo["Cached"] + memInfo["SReclaimable"]
	info.SwapTotal = memInfo["SwapTotal"]
	info.SwapFree = memInfo["SwapFree"]
	info.SwapUsed = info.SwapTotal - info.SwapFree

	// Calculate used memory (Total - Available is most accurate)
	if info.Available > 0 {
		info.Used = info.Total - info.Available
	} else {
		info.Used = info.Total - info.Free - info.Buffers - info.Cached
	}

	// Calculate percentages
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}
	if info.SwapTotal > 0 {
		info.SwapPercent = float64(info.SwapUsed) / float64(info.SwapTotal) * 100
	}

	return info, nil
}
