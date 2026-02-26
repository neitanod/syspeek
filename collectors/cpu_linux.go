//go:build linux

package collectors

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CPUCore struct {
	ID           int     `json:"id"`
	UsagePercent float64 `json:"usagePercent"`
	Temperature  float64 `json:"temperature,omitempty"`
	Frequency    float64 `json:"frequency,omitempty"`
}

type PhysicalCore struct {
	ID          int     `json:"id"`          // Intel's core ID (0, 4, 8, etc)
	Temperature float64 `json:"temperature"`
	Type        string  `json:"type"`        // "P" for Performance, "E" for Efficiency
}

type CPUInfo struct {
	Model         string         `json:"model"`
	Cores         int            `json:"cores"`
	Threads       int            `json:"threads"`
	PhysicalCores int            `json:"physicalCores"`
	UsagePercent  float64        `json:"usagePercent"`
	LoadAvg       []float64      `json:"loadAvg"`
	CoreStats     []CPUCore      `json:"coreStats"`
	CoreTemps     []PhysicalCore `json:"coreTemps,omitempty"` // Physical core temperatures
	PackageTemp   float64        `json:"packageTemp,omitempty"`
	Uptime        string         `json:"uptime"`
}

type cpuTimes struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	steal   uint64
}

var previousCPUTimes map[int]cpuTimes
var previousTotalTimes map[int]cpuTimes
var cpuMutex sync.Mutex

func init() {
	previousCPUTimes = make(map[int]cpuTimes)
	previousTotalTimes = make(map[int]cpuTimes)
}

func GetCPUInfo() (*CPUInfo, error) {
	info := &CPUInfo{}

	// Get CPU model
	cpuinfo, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		lines := strings.Split(string(cpuinfo), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.Model = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// Get load average
	loadavg, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(loadavg))
		if len(fields) >= 3 {
			info.LoadAvg = make([]float64, 3)
			for i := 0; i < 3; i++ {
				info.LoadAvg[i], _ = strconv.ParseFloat(fields[i], 64)
			}
		}
	}

	// Get uptime
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err == nil {
		fields := strings.Fields(string(uptimeData))
		if len(fields) >= 1 {
			seconds, _ := strconv.ParseFloat(fields[0], 64)
			info.Uptime = formatUptime(seconds)
		}
	}

	// Get CPU usage per core
	stat, err := os.Open("/proc/stat")
	if err != nil {
		return info, err
	}
	defer stat.Close()

	scanner := bufio.NewScanner(stat)
	coreID := -1 // -1 for total CPU

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		var times cpuTimes
		times.user, _ = strconv.ParseUint(fields[1], 10, 64)
		times.nice, _ = strconv.ParseUint(fields[2], 10, 64)
		times.system, _ = strconv.ParseUint(fields[3], 10, 64)
		times.idle, _ = strconv.ParseUint(fields[4], 10, 64)
		times.iowait, _ = strconv.ParseUint(fields[5], 10, 64)
		times.irq, _ = strconv.ParseUint(fields[6], 10, 64)
		times.softirq, _ = strconv.ParseUint(fields[7], 10, 64)
		if len(fields) > 8 {
			times.steal, _ = strconv.ParseUint(fields[8], 10, 64)
		}

		if fields[0] == "cpu" {
			// Total CPU
			coreID = -1
			usage := calculateCPUUsage(coreID, times)
			info.UsagePercent = usage
		} else {
			// Individual core
			coreNum, _ := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu"))
			coreID = coreNum
			usage := calculateCPUUsage(coreID, times)

			core := CPUCore{
				ID:           coreNum,
				UsagePercent: usage,
			}

			// Get temperature for this core
			core.Temperature = getCoreTemperature(coreNum)

			// Get frequency for this core
			core.Frequency = getCoreFrequency(coreNum)

			info.CoreStats = append(info.CoreStats, core)
		}
	}

	info.Cores = len(info.CoreStats)
	info.Threads = len(info.CoreStats)

	// Get physical core temperatures
	info.CoreTemps, info.PackageTemp = getPhysicalCoreTemperatures()
	info.PhysicalCores = len(info.CoreTemps)

	return info, nil
}

func calculateCPUUsage(coreID int, current cpuTimes) float64 {
	cpuMutex.Lock()
	prev, exists := previousCPUTimes[coreID]
	previousCPUTimes[coreID] = current
	cpuMutex.Unlock()

	if !exists {
		return 0
	}

	prevIdle := prev.idle + prev.iowait
	currIdle := current.idle + current.iowait

	prevNonIdle := prev.user + prev.nice + prev.system + prev.irq + prev.softirq + prev.steal
	currNonIdle := current.user + current.nice + current.system + current.irq + current.softirq + current.steal

	prevTotal := prevIdle + prevNonIdle
	currTotal := currIdle + currNonIdle

	totalDiff := currTotal - prevTotal
	idleDiff := currIdle - prevIdle

	if totalDiff == 0 {
		return 0
	}

	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100
}

func getCoreTemperature(coreNum int) float64 {
	// Try hwmon
	hwmonPath := "/sys/class/hwmon"
	entries, err := os.ReadDir(hwmonPath)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		namePath := filepath.Join(hwmonPath, entry.Name(), "name")
		name, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}

		nameStr := strings.TrimSpace(string(name))
		if nameStr == "coretemp" || nameStr == "k10temp" || nameStr == "zenpower" {
			// Try to find the temperature for this core
			for i := 1; i <= 32; i++ {
				tempPath := filepath.Join(hwmonPath, entry.Name(), fmt.Sprintf("temp%d_input", i))
				labelPath := filepath.Join(hwmonPath, entry.Name(), fmt.Sprintf("temp%d_label", i))

				label, err := os.ReadFile(labelPath)
				if err != nil {
					continue
				}

				labelStr := strings.TrimSpace(string(label))
				expectedLabel := fmt.Sprintf("Core %d", coreNum)

				if labelStr == expectedLabel || (coreNum == 0 && strings.Contains(labelStr, "Package")) {
					temp, err := os.ReadFile(tempPath)
					if err != nil {
						continue
					}
					tempVal, _ := strconv.ParseFloat(strings.TrimSpace(string(temp)), 64)
					return tempVal / 1000 // Convert from millidegrees
				}
			}

			// If no specific core found, try generic temp
			tempPath := filepath.Join(hwmonPath, entry.Name(), fmt.Sprintf("temp%d_input", coreNum+2))
			temp, err := os.ReadFile(tempPath)
			if err == nil {
				tempVal, _ := strconv.ParseFloat(strings.TrimSpace(string(temp)), 64)
				return tempVal / 1000
			}
		}
	}

	return 0
}

func getCoreFrequency(coreNum int) float64 {
	// Try scaling_cur_freq first
	freqPath := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/scaling_cur_freq", coreNum)
	freq, err := os.ReadFile(freqPath)
	if err == nil {
		freqVal, _ := strconv.ParseFloat(strings.TrimSpace(string(freq)), 64)
		return freqVal / 1000 // Convert from KHz to MHz
	}

	// Fallback to cpuinfo_cur_freq
	freqPath = fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/cpuinfo_cur_freq", coreNum)
	freq, err = os.ReadFile(freqPath)
	if err == nil {
		freqVal, _ := strconv.ParseFloat(strings.TrimSpace(string(freq)), 64)
		return freqVal / 1000
	}

	return 0
}

func formatUptime(seconds float64) string {
	duration := time.Duration(seconds) * time.Second
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// getPhysicalCoreTemperatures reads all physical core temperatures from coretemp
// Returns slice of PhysicalCore sorted by ID, and package temperature
func getPhysicalCoreTemperatures() ([]PhysicalCore, float64) {
	var cores []PhysicalCore
	var packageTemp float64

	hwmonPath := "/sys/class/hwmon"
	entries, err := os.ReadDir(hwmonPath)
	if err != nil {
		return cores, packageTemp
	}

	for _, entry := range entries {
		namePath := filepath.Join(hwmonPath, entry.Name(), "name")
		name, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}

		nameStr := strings.TrimSpace(string(name))
		if nameStr != "coretemp" && nameStr != "k10temp" && nameStr != "zenpower" {
			continue
		}

		// Read all temperature sensors
		for i := 1; i <= 64; i++ {
			labelPath := filepath.Join(hwmonPath, entry.Name(), fmt.Sprintf("temp%d_label", i))
			tempPath := filepath.Join(hwmonPath, entry.Name(), fmt.Sprintf("temp%d_input", i))

			label, err := os.ReadFile(labelPath)
			if err != nil {
				continue
			}

			temp, err := os.ReadFile(tempPath)
			if err != nil {
				continue
			}

			labelStr := strings.TrimSpace(string(label))
			tempVal, _ := strconv.ParseFloat(strings.TrimSpace(string(temp)), 64)
			tempVal = tempVal / 1000 // Convert from millidegrees

			if strings.HasPrefix(labelStr, "Package") {
				packageTemp = tempVal
			} else if strings.HasPrefix(labelStr, "Core ") {
				// Extract core ID
				coreIDStr := strings.TrimPrefix(labelStr, "Core ")
				coreID, err := strconv.Atoi(coreIDStr)
				if err != nil {
					continue
				}

				// Determine core type based on Intel 13th gen hybrid architecture
				// P-cores: IDs 0, 4, 8, 12, 16, 20 (multiples of 4, up to 20)
				// E-cores: IDs 24-31
				coreType := "P"
				if coreID >= 24 {
					coreType = "E"
				}

				cores = append(cores, PhysicalCore{
					ID:          coreID,
					Temperature: tempVal,
					Type:        coreType,
				})
			}
		}
		break // Found coretemp, no need to check other hwmon entries
	}

	// Sort cores by ID
	sortPhysicalCores(cores)

	return cores, packageTemp
}

// sortPhysicalCores sorts cores by ID (P-cores first, then E-cores)
func sortPhysicalCores(cores []PhysicalCore) {
	// Simple insertion sort (small slice)
	for i := 1; i < len(cores); i++ {
		key := cores[i]
		j := i - 1
		for j >= 0 && cores[j].ID > key.ID {
			cores[j+1] = cores[j]
			j--
		}
		cores[j+1] = key
	}
}
