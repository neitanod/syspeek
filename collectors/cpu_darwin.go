//go:build darwin

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
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

	// Get CPU model using sysctl
	if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		info.Model = strings.TrimSpace(string(out))
	}

	// Get core count
	if out, err := exec.Command("sysctl", "-n", "hw.physicalcpu").Output(); err == nil {
		info.PhysicalCores, _ = strconv.Atoi(strings.TrimSpace(string(out)))
		info.Cores = info.PhysicalCores
	}

	// Get thread count
	if out, err := exec.Command("sysctl", "-n", "hw.logicalcpu").Output(); err == nil {
		info.Threads, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}

	// Get load average
	if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		parts := strings.Fields(strings.Trim(string(out), "{ }"))
		for _, p := range parts {
			if v, err := strconv.ParseFloat(p, 64); err == nil {
				info.LoadAvg = append(info.LoadAvg, v)
			}
		}
	}

	// Get CPU usage from top
	if out, err := exec.Command("top", "-l", "1", "-n", "0", "-stats", "cpu").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "CPU usage") {
				// Parse: CPU usage: 5.55% user, 11.11% sys, 83.33% idle
				parts := strings.Split(line, ",")
				var user, sys float64
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if strings.Contains(p, "user") {
						fields := strings.Fields(p)
						for _, f := range fields {
							if strings.HasSuffix(f, "%") {
								user, _ = strconv.ParseFloat(strings.TrimSuffix(f, "%"), 64)
							}
						}
					} else if strings.Contains(p, "sys") {
						fields := strings.Fields(p)
						for _, f := range fields {
							if strings.HasSuffix(f, "%") {
								sys, _ = strconv.ParseFloat(strings.TrimSuffix(f, "%"), 64)
							}
						}
					}
				}
				info.UsagePercent = user + sys
				break
			}
		}
	}

	// Get uptime
	if out, err := exec.Command("uptime").Output(); err == nil {
		upStr := string(out)
		if idx := strings.Index(upStr, "up "); idx != -1 {
			end := strings.Index(upStr[idx:], ",")
			if end > 0 {
				info.Uptime = strings.TrimSpace(upStr[idx+3 : idx+end])
			}
		}
	}

	// Create core stats (simplified for macOS)
	for i := 0; i < info.Threads; i++ {
		info.CoreStats = append(info.CoreStats, CPUCore{
			ID:           i,
			UsagePercent: info.UsagePercent,
		})
	}

	return info, nil
}

var startTime = time.Now()
