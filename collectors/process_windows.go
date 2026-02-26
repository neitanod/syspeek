//go:build windows

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type ProcessInfo struct {
	PID           int      `json:"pid"`
	PPID          int      `json:"ppid"`
	Name          string   `json:"name"`
	Command       string   `json:"command"`
	CommandLine   []string `json:"commandLine,omitempty"`
	State         string   `json:"state"`
	User          string   `json:"user"`
	UID           int      `json:"uid"`
	GID           int      `json:"gid"`
	CPUPercent    float64  `json:"cpuPercent"`
	MemoryPercent float64  `json:"memoryPercent"`
	MemoryBytes   uint64   `json:"memoryBytes"`
	Threads       int      `json:"threads"`
	Nice          int      `json:"nice"`
	VmSize        uint64   `json:"vmSize,omitempty"`
	VmRss         uint64   `json:"vmRss,omitempty"`
	VmSwap        uint64   `json:"vmSwap,omitempty"`
	IoReadBytes   uint64   `json:"ioReadBytes,omitempty"`
	IoWriteBytes  uint64   `json:"ioWriteBytes,omitempty"`
	Exe           string   `json:"exe,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	Uptime        string   `json:"uptime,omitempty"`
	Children      []int    `json:"children,omitempty"`
	Connections   []Socket `json:"connections,omitempty"`
	FDs           []FD     `json:"fds,omitempty"`
}

type FD struct {
	FD     int    `json:"fd"`
	Type   string `json:"type"`
	Target string `json:"target"`
}

type ProcessList struct {
	Processes  []ProcessInfo `json:"processes"`
	TotalCount int           `json:"totalCount"`
}

func GetProcessList() (ProcessList, error) {
	list := ProcessList{}

	// Get total memory for calculating percentages
	var totalMemory uint64
	memScript := `(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory`
	memOut, _ := runPowerShell(memScript)
	totalMemory, _ = strconv.ParseUint(strings.TrimSpace(memOut), 10, 64)

	// Get process info using PowerShell - more reliable than wmic
	script := `
Get-Process | ForEach-Object {
    $owner = ""
    try {
        $owner = (Get-CimInstance Win32_Process -Filter "ProcessId=$($_.Id)" -ErrorAction SilentlyContinue).GetOwner().User
    } catch {}
    "$($_.Id)|$($_.ProcessName)|$($_.CPU)|$($_.WorkingSet64)|$($_.Threads.Count)|$($_.Path)|$owner"
}
`
	out, err := runPowerShell(script)
	if err != nil {
		return list, err
	}

	// Also get parent process IDs
	ppidScript := `Get-CimInstance Win32_Process | ForEach-Object { "$($_.ProcessId)|$($_.ParentProcessId)|$($_.CommandLine)" }`
	ppidOut, _ := runPowerShell(ppidScript)
	ppidMap := make(map[int]struct {
		ppid    int
		cmdLine string
	})
	for _, line := range strings.Split(ppidOut, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "|", 3)
		if len(parts) >= 2 {
			pid, _ := strconv.Atoi(parts[0])
			ppid, _ := strconv.Atoi(parts[1])
			cmdLine := ""
			if len(parts) >= 3 {
				cmdLine = parts[2]
			}
			ppidMap[pid] = struct {
				ppid    int
				cmdLine string
			}{ppid, cmdLine}
		}
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 7)
		if len(parts) < 5 {
			continue
		}

		pid, _ := strconv.Atoi(parts[0])
		cpu, _ := strconv.ParseFloat(parts[2], 64)
		memBytes, _ := strconv.ParseUint(parts[3], 10, 64)
		threads, _ := strconv.Atoi(parts[4])

		proc := ProcessInfo{
			PID:         pid,
			Name:        parts[1],
			CPUPercent:  cpu,
			MemoryBytes: memBytes,
			VmRss:       memBytes,
			Threads:     threads,
			State:       "running",
		}

		if len(parts) >= 6 && parts[5] != "" {
			proc.Exe = parts[5]
		}
		if len(parts) >= 7 && parts[6] != "" {
			proc.User = parts[6]
		}

		// Add PPID and command line from second query
		if ppidData, ok := ppidMap[pid]; ok {
			proc.PPID = ppidData.ppid
			if ppidData.cmdLine != "" {
				proc.Command = ppidData.cmdLine
				proc.CommandLine = strings.Fields(ppidData.cmdLine)
			}
		}

		// Calculate memory percentage
		if totalMemory > 0 {
			proc.MemoryPercent = float64(memBytes) / float64(totalMemory) * 100
		}

		list.Processes = append(list.Processes, proc)
	}

	list.TotalCount = len(list.Processes)
	return list, nil
}

func GetProcessDetail(pid int) (*ProcessInfo, error) {
	// Get detailed info for a single process
	script := `
$p = Get-Process -Id ` + strconv.Itoa(pid) + ` -ErrorAction SilentlyContinue
if ($p) {
    $wmi = Get-CimInstance Win32_Process -Filter "ProcessId=$pid" -ErrorAction SilentlyContinue
    $owner = ""
    try { $owner = $wmi.GetOwner().User } catch {}
    "$($p.Id)|$($p.ProcessName)|$($p.CPU)|$($p.WorkingSet64)|$($p.VirtualMemorySize64)|$($p.Threads.Count)|$($p.Path)|$owner|$($wmi.ParentProcessId)|$($wmi.CommandLine)|$($wmi.CreationDate)"
}
`
	out, err := runPowerShell(script)
	if err != nil || out == "" {
		return nil, err
	}

	parts := strings.SplitN(strings.TrimSpace(out), "|", 11)
	if len(parts) < 6 {
		return nil, nil
	}

	pidVal, _ := strconv.Atoi(parts[0])
	cpu, _ := strconv.ParseFloat(parts[2], 64)
	memBytes, _ := strconv.ParseUint(parts[3], 10, 64)
	vmSize, _ := strconv.ParseUint(parts[4], 10, 64)
	threads, _ := strconv.Atoi(parts[5])

	proc := &ProcessInfo{
		PID:         pidVal,
		Name:        parts[1],
		CPUPercent:  cpu,
		MemoryBytes: memBytes,
		VmRss:       memBytes,
		VmSize:      vmSize,
		Threads:     threads,
		State:       "running",
	}

	if len(parts) >= 7 && parts[6] != "" {
		proc.Exe = parts[6]
	}
	if len(parts) >= 8 && parts[7] != "" {
		proc.User = parts[7]
	}
	if len(parts) >= 9 {
		proc.PPID, _ = strconv.Atoi(parts[8])
	}
	if len(parts) >= 10 && parts[9] != "" {
		proc.Command = parts[9]
		proc.CommandLine = strings.Fields(parts[9])
	}

	// Get memory percentage
	memScript := `(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory`
	memOut, _ := runPowerShell(memScript)
	totalMem, _ := strconv.ParseUint(strings.TrimSpace(memOut), 10, 64)
	if totalMem > 0 {
		proc.MemoryPercent = float64(memBytes) / float64(totalMem) * 100
	}

	// Get children
	childScript := `Get-CimInstance Win32_Process | Where-Object { $_.ParentProcessId -eq ` + strconv.Itoa(pid) + ` } | ForEach-Object { $_.ProcessId }`
	childOut, _ := runPowerShell(childScript)
	for _, line := range strings.Split(childOut, "\n") {
		if childPid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			proc.Children = append(proc.Children, childPid)
		}
	}

	return proc, nil
}

func GetProcessesByUser(username string) ([]ProcessInfo, error) {
	list, err := GetProcessList()
	if err != nil {
		return nil, err
	}

	var result []ProcessInfo
	for _, p := range list.Processes {
		if strings.EqualFold(p.User, username) {
			result = append(result, p)
		}
	}
	return result, nil
}

// KillProcess terminates a process on Windows using taskkill
func KillProcess(pid int, signal syscall.Signal) error {
	cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}

// ReniceProcess changes process priority on Windows
func ReniceProcess(pid int, priority int) error {
	// Map nice-like priority to Windows priority class
	var priorityClass string
	switch {
	case priority >= 15:
		priorityClass = "64" // Idle
	case priority >= 5:
		priorityClass = "16384" // Below Normal
	case priority >= -5:
		priorityClass = "32" // Normal
	case priority >= -10:
		priorityClass = "32768" // Above Normal
	default:
		priorityClass = "128" // High
	}

	script := `(Get-Process -Id ` + strconv.Itoa(pid) + `).PriorityClass = ` + priorityClass
	_, err := runPowerShell(script)
	return err
}
