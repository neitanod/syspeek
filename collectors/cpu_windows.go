//go:build windows

package collectors

import (
	"fmt"
	"os/exec"
	"strings"

	gpscpu "github.com/shirou/gopsutil/v3/cpu"
	gpshost "github.com/shirou/gopsutil/v3/host"
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

// runPowerShell runs a PowerShell snippet and returns its trimmed stdout. The
// script is passed via -EncodedCommand (UTF-16LE base64) so multi-line scripts
// and embedded quotes are not mangled by cmd.exe's argument parsing. A UTF-8
// prelude is prepended so accented characters survive the round-trip back to
// the HTTP response (PowerShell otherwise emits the current OEM code page).
func runPowerShell(script string) (string, error) {
	const utf8Prelude = "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); $OutputEncoding = [System.Text.UTF8Encoding]::new(); "
	encoded := encodePowerShellCommand(utf8Prelude + script)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// encodePowerShellCommand returns the base64 UTF-16LE encoding that
// powershell.exe expects for the -EncodedCommand flag.
func encodePowerShellCommand(script string) string {
	utf16 := make([]byte, 0, len(script)*2)
	for _, r := range script {
		if r < 0x10000 {
			utf16 = append(utf16, byte(r), byte(r>>8))
		} else {
			// Surrogate pair encoding for code points beyond the BMP.
			r -= 0x10000
			hi := 0xD800 + (r >> 10)
			lo := 0xDC00 + (r & 0x3FF)
			utf16 = append(utf16, byte(hi), byte(hi>>8), byte(lo), byte(lo>>8))
		}
	}
	return b64Encode(utf16)
}

func b64Encode(b []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	n := len(b)
	out := make([]byte, 0, ((n+2)/3)*4)
	i := 0
	for ; i+3 <= n; i += 3 {
		v := uint32(b[i])<<16 | uint32(b[i+1])<<8 | uint32(b[i+2])
		out = append(out, tbl[(v>>18)&0x3F], tbl[(v>>12)&0x3F], tbl[(v>>6)&0x3F], tbl[v&0x3F])
	}
	switch n - i {
	case 1:
		v := uint32(b[i]) << 16
		out = append(out, tbl[(v>>18)&0x3F], tbl[(v>>12)&0x3F], '=', '=')
	case 2:
		v := uint32(b[i])<<16 | uint32(b[i+1])<<8
		out = append(out, tbl[(v>>18)&0x3F], tbl[(v>>12)&0x3F], tbl[(v>>6)&0x3F], '=')
	}
	return string(out)
}

func GetCPUInfo() (CPUInfo, error) {
	info := CPUInfo{}

	if cpus, err := gpscpu.Info(); err == nil && len(cpus) > 0 {
		info.Model = strings.TrimSpace(cpus[0].ModelName)
	}

	if logical, err := gpscpu.Counts(true); err == nil {
		info.Threads = logical
	}
	if physical, err := gpscpu.Counts(false); err == nil {
		info.Cores = physical
		info.PhysicalCores = physical
	}

	if perCore, err := gpscpu.Percent(0, true); err == nil {
		for i, usage := range perCore {
			info.CoreStats = append(info.CoreStats, CPUCore{
				ID:           i,
				UsagePercent: usage,
			})
		}
	}

	if total, err := gpscpu.Percent(0, false); err == nil && len(total) > 0 {
		info.UsagePercent = total[0]
	} else if len(info.CoreStats) > 0 {
		var sum float64
		for _, c := range info.CoreStats {
			sum += c.UsagePercent
		}
		info.UsagePercent = sum / float64(len(info.CoreStats))
	}

	if uptime, err := gpshost.Uptime(); err == nil {
		days := uptime / 86400
		hours := (uptime % 86400) / 3600
		minutes := (uptime % 3600) / 60
		info.Uptime = fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}

	// Windows has no load average. Surface a synthetic equivalent so the UI
	// has something to plot.
	info.LoadAvg = []float64{info.UsagePercent / 100 * float64(info.Threads), 0, 0}

	return info, nil
}
