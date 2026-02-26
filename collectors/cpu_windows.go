//go:build windows

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

func runPowerShell(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func GetCPUInfo() (CPUInfo, error) {
	info := CPUInfo{}

	// Get CPU info using PowerShell
	script := `
$cpu = Get-CimInstance Win32_Processor
$cpu.Name
$cpu.NumberOfCores
$cpu.NumberOfLogicalProcessors
$cpu.LoadPercentage
`
	out, err := runPowerShell(script)
	if err == nil {
		lines := strings.Split(out, "\n")
		if len(lines) >= 1 {
			info.Model = strings.TrimSpace(lines[0])
		}
		if len(lines) >= 2 {
			info.Cores, _ = strconv.Atoi(strings.TrimSpace(lines[1]))
			info.PhysicalCores = info.Cores
		}
		if len(lines) >= 3 {
			info.Threads, _ = strconv.Atoi(strings.TrimSpace(lines[2]))
		}
		if len(lines) >= 4 {
			info.UsagePercent, _ = strconv.ParseFloat(strings.TrimSpace(lines[3]), 64)
		}
	}

	// Get per-core CPU usage
	coreScript := `
Get-CimInstance Win32_PerfFormattedData_PerfOS_Processor | Where-Object { $_.Name -ne '_Total' } | ForEach-Object { $_.PercentProcessorTime }
`
	coreOut, err := runPowerShell(coreScript)
	if err == nil && coreOut != "" {
		lines := strings.Split(coreOut, "\n")
		for i, line := range lines {
			usage, _ := strconv.ParseFloat(strings.TrimSpace(line), 64)
			info.CoreStats = append(info.CoreStats, CPUCore{
				ID:           i,
				UsagePercent: usage,
			})
		}
	}

	// If no per-core stats, create from total
	if len(info.CoreStats) == 0 && info.Threads > 0 {
		for i := 0; i < info.Threads; i++ {
			info.CoreStats = append(info.CoreStats, CPUCore{
				ID:           i,
				UsagePercent: info.UsagePercent,
			})
		}
	}

	// Get uptime
	uptimeScript := `(Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime | ForEach-Object { "{0}d {1}h {2}m" -f $_.Days, $_.Hours, $_.Minutes }`
	uptimeOut, err := runPowerShell(uptimeScript)
	if err == nil {
		info.Uptime = uptimeOut
	}

	// Windows doesn't have load average, simulate with current usage
	info.LoadAvg = []float64{info.UsagePercent / 100 * float64(info.Threads), 0, 0}

	return info, nil
}

// Store previous CPU times for calculating delta
var prevCPUTimes map[int]uint64
var prevCPUTime time.Time

func init() {
	prevCPUTimes = make(map[int]uint64)
}
