//go:build windows

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type CPUCore struct {
	ID           int     `json:"id"`
	UsagePercent float64 `json:"usagePercent"`
	Temperature  float64 `json:"temperature,omitempty"`
	Frequency    float64 `json:"frequency,omitempty"`
}

type PhysicalCore struct {
	ID          int     `json:"id"`
	Temperature float64 `json:"temperature"`
	Type        string  `json:"type"`
}

type CPUInfo struct {
	Model         string         `json:"model"`
	Cores         int            `json:"cores"`
	Threads       int            `json:"threads"`
	PhysicalCores int            `json:"physicalCores"`
	UsagePercent  float64        `json:"usagePercent"`
	LoadAvg       []float64      `json:"loadAvg"`
	CoreStats     []CPUCore      `json:"coreStats"`
	CoreTemps     []PhysicalCore `json:"coreTemps,omitempty"`
	PackageTemp   float64        `json:"packageTemp,omitempty"`
	Uptime        string         `json:"uptime"`
}

func GetCPUInfo() (CPUInfo, error) {
	info := CPUInfo{}

	// Get CPU info using WMIC
	if out, err := exec.Command("wmic", "cpu", "get", "Name", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Name=") {
				info.Model = strings.TrimSpace(strings.TrimPrefix(line, "Name="))
				break
			}
		}
	}

	// Get core count
	if out, err := exec.Command("wmic", "cpu", "get", "NumberOfCores", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "NumberOfCores=") {
				info.Cores, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "NumberOfCores=")))
				info.PhysicalCores = info.Cores
				break
			}
		}
	}

	// Get thread count
	if out, err := exec.Command("wmic", "cpu", "get", "NumberOfLogicalProcessors", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "NumberOfLogicalProcessors=") {
				info.Threads, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "NumberOfLogicalProcessors=")))
				break
			}
		}
	}

	// Get CPU usage
	if out, err := exec.Command("wmic", "cpu", "get", "LoadPercentage", "/value").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "LoadPercentage=") {
				info.UsagePercent, _ = strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "LoadPercentage=")), 64)
				break
			}
		}
	}

	// Create core stats (simplified)
	for i := 0; i < info.Threads; i++ {
		info.CoreStats = append(info.CoreStats, CPUCore{
			ID:           i,
			UsagePercent: info.UsagePercent,
		})
	}

	// Get uptime
	if out, err := exec.Command("net", "statistics", "workstation").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Statistics since") {
				info.Uptime = strings.TrimSpace(strings.TrimPrefix(line, "Statistics since"))
				break
			}
		}
	}

	return info, nil
}
