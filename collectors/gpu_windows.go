//go:build windows

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type GPUInfo struct {
	Available    bool    `json:"available"`
	Name         string  `json:"name,omitempty"`
	UsagePercent float64 `json:"usagePercent,omitempty"`
	MemoryUsed   uint64  `json:"memoryUsed,omitempty"`
	MemoryTotal  uint64  `json:"memoryTotal,omitempty"`
	Temperature  float64 `json:"temperature,omitempty"`
	PowerDraw    float64 `json:"powerDraw,omitempty"`
	FanSpeed     int     `json:"fanSpeed,omitempty"`
}

func GetGPUInfo() (GPUInfo, error) {
	info := GPUInfo{}

	// Try nvidia-smi first
	if out, err := exec.Command("nvidia-smi", "--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,fan.speed", "--format=csv,noheader,nounits").Output(); err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(parts) >= 7 {
			info.Available = true
			info.Name = strings.TrimSpace(parts[0])
			info.UsagePercent, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			memUsed, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			info.MemoryUsed = uint64(memUsed * 1024 * 1024)
			memTotal, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
			info.MemoryTotal = uint64(memTotal * 1024 * 1024)
			info.Temperature, _ = strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
			info.PowerDraw, _ = strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)
			fanSpeed, _ := strconv.ParseFloat(strings.TrimSpace(parts[6]), 64)
			info.FanSpeed = int(fanSpeed)
			return info, nil
		}
	}

	// Try getting basic GPU info from WMIC
	if out, err := exec.Command("wmic", "path", "win32_VideoController", "get", "Name", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Name=") {
				info.Name = strings.TrimSpace(strings.TrimPrefix(line, "Name="))
				if info.Name != "" {
					info.Available = true
				}
				break
			}
		}
	}

	return info, nil
}
