//go:build linux

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type GPUInfo struct {
	Available   bool    `json:"available"`
	Name        string  `json:"name"`
	Driver      string  `json:"driver"`
	MemoryTotal uint64  `json:"memoryTotal"`
	MemoryUsed  uint64  `json:"memoryUsed"`
	MemoryFree  uint64  `json:"memoryFree"`
	UsagePercent float64 `json:"usagePercent"`
	Temperature float64 `json:"temperature"`
	PowerDraw   float64 `json:"powerDraw"`
	PowerLimit  float64 `json:"powerLimit"`
	FanSpeed    int     `json:"fanSpeed"`
}

func GetGPUInfo() (*GPUInfo, error) {
	info := &GPUInfo{
		Available: false,
	}

	// Try nvidia-smi first
	nvidiaInfo, err := getNvidiaGPU()
	if err == nil && nvidiaInfo != nil {
		return nvidiaInfo, nil
	}

	// Try AMD GPU
	amdInfo, err := getAMDGPU()
	if err == nil && amdInfo != nil {
		return amdInfo, nil
	}

	// No GPU found
	return info, nil
}

func getNvidiaGPU() (*GPUInfo, error) {
	// Check if nvidia-smi is available
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=name,driver_version,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,power.draw,power.limit,fan.speed",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return nil, nil
	}

	// Parse first GPU
	fields := strings.Split(lines[0], ", ")
	if len(fields) < 10 {
		return nil, nil
	}

	info := &GPUInfo{
		Available: true,
		Name:      strings.TrimSpace(fields[0]),
		Driver:    strings.TrimSpace(fields[1]),
	}

	// Memory in MiB, convert to bytes
	memTotal, _ := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 64)
	memUsed, _ := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 64)
	memFree, _ := strconv.ParseUint(strings.TrimSpace(fields[4]), 10, 64)
	info.MemoryTotal = memTotal * 1024 * 1024
	info.MemoryUsed = memUsed * 1024 * 1024
	info.MemoryFree = memFree * 1024 * 1024

	info.UsagePercent, _ = strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
	info.Temperature, _ = strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)
	info.PowerDraw, _ = strconv.ParseFloat(strings.TrimSpace(fields[7]), 64)
	info.PowerLimit, _ = strconv.ParseFloat(strings.TrimSpace(fields[8]), 64)
	info.FanSpeed, _ = strconv.Atoi(strings.TrimSpace(fields[9]))

	return info, nil
}

func getAMDGPU() (*GPUInfo, error) {
	// Try rocm-smi for AMD GPUs
	cmd := exec.Command("rocm-smi", "--showtemp", "--showuse", "--showmeminfo", "vram", "--json")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Basic parsing - AMD GPU detection
	if len(output) > 0 {
		return &GPUInfo{
			Available: true,
			Name:      "AMD GPU (detected)",
			Driver:    "amdgpu",
		}, nil
	}

	return nil, nil
}
