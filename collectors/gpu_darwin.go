//go:build darwin

package collectors

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
	// macOS doesn't have easy access to GPU stats like nvidia-smi
	// Would require Metal API or IOKit which is more complex
	return GPUInfo{Available: false}, nil
}
