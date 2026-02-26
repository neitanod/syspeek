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

	// Use tasklist for basic process info
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return list, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse CSV: "Image Name","PID","Session Name","Session#","Mem Usage"
		parts := parseCSVLine(line)
		if len(parts) < 5 {
			continue
		}

		pid, _ := strconv.Atoi(parts[1])
		memStr := strings.ReplaceAll(parts[4], ",", "")
		memStr = strings.ReplaceAll(memStr, " K", "")
		memStr = strings.TrimSpace(memStr)
		memKB, _ := strconv.ParseUint(memStr, 10, 64)

		proc := ProcessInfo{
			PID:         pid,
			Name:        parts[0],
			MemoryBytes: memKB * 1024,
			State:       "running",
		}

		list.Processes = append(list.Processes, proc)
	}

	// Get more details using WMIC
	wmicOut, err := exec.Command("wmic", "process", "get", "ProcessId,ParentProcessId,ThreadCount,WorkingSetSize,CommandLine", "/format:csv").Output()
	if err == nil {
		wmicMap := make(map[int]struct {
			ppid       int
			threads    int
			workingSet uint64
			cmdLine    string
		})

		lines := strings.Split(string(wmicOut), "\n")
		for _, line := range lines {
			parts := strings.Split(line, ",")
			if len(parts) < 5 {
				continue
			}

			pid, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-4]))
			ppid, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-3]))
			threads, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-2]))
			ws, _ := strconv.ParseUint(strings.TrimSpace(parts[len(parts)-1]), 10, 64)
			cmdLine := strings.TrimSpace(parts[1])

			wmicMap[pid] = struct {
				ppid       int
				threads    int
				workingSet uint64
				cmdLine    string
			}{ppid, threads, ws, cmdLine}
		}

		// Merge data
		for i := range list.Processes {
			if data, ok := wmicMap[list.Processes[i].PID]; ok {
				list.Processes[i].PPID = data.ppid
				list.Processes[i].Threads = data.threads
				list.Processes[i].VmRss = data.workingSet
				list.Processes[i].Command = data.cmdLine
				if data.cmdLine != "" {
					list.Processes[i].CommandLine = strings.Fields(data.cmdLine)
				}
			}
		}
	}

	list.TotalCount = len(list.Processes)
	return list, nil
}

func parseCSVLine(line string) []string {
	var result []string
	var current strings.Builder
	inQuote := false

	for _, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case ',':
			if inQuote {
				current.WriteRune(r)
			} else {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	result = append(result, current.String())
	return result
}

func GetProcessDetail(pid int) (*ProcessInfo, error) {
	list, err := GetProcessList()
	if err != nil {
		return nil, err
	}

	for _, p := range list.Processes {
		if p.PID == pid {
			return &p, nil
		}
	}

	return nil, nil
}

func GetProcessesByUser(username string) ([]ProcessInfo, error) {
	// Get all processes with owner info using WMIC
	var result []ProcessInfo

	out, err := exec.Command("wmic", "process", "get", "ProcessId,Caption", "/format:csv").Output()
	if err != nil {
		return result, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}

		pid, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))

		result = append(result, ProcessInfo{
			PID:  pid,
			Name: strings.TrimSpace(parts[1]),
			User: username, // Simplified
		})
	}

	return result, nil
}

// KillProcess terminates a process on Windows using taskkill
// The signal parameter is ignored on Windows
func KillProcess(pid int, signal syscall.Signal) error {
	// On Windows, use taskkill
	// /F = force, /PID = process ID
	cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}

// ReniceProcess changes process priority on Windows
// Windows doesn't have nice values, so we map to priority classes
func ReniceProcess(pid int, priority int) error {
	// Map nice-like priority to Windows priority class
	// Windows priority classes: Idle, Below Normal, Normal, Above Normal, High, Realtime
	var priorityClass string
	switch {
	case priority >= 15:
		priorityClass = "idle"
	case priority >= 5:
		priorityClass = "below normal"
	case priority >= -5:
		priorityClass = "normal"
	case priority >= -10:
		priorityClass = "above normal"
	default:
		priorityClass = "high"
	}

	// Use wmic to set priority
	cmd := exec.Command("wmic", "process", "where", "ProcessId="+strconv.Itoa(pid), "call", "SetPriority", priorityClass)
	return cmd.Run()
}
