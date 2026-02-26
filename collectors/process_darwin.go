//go:build darwin

package collectors

import (
	"os/exec"
	"os/user"
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

	// Use ps to get process list
	// Format: pid,ppid,user,state,%cpu,%mem,rss,vsz,command
	out, err := exec.Command("ps", "-axo", "pid,ppid,user,state,%cpu,%mem,rss,vsz,comm").Output()
	if err != nil {
		return list, err
	}

	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		cpuPercent, _ := strconv.ParseFloat(fields[4], 64)
		memPercent, _ := strconv.ParseFloat(fields[5], 64)
		rss, _ := strconv.ParseUint(fields[6], 10, 64)
		vsz, _ := strconv.ParseUint(fields[7], 10, 64)

		proc := ProcessInfo{
			PID:           pid,
			PPID:          ppid,
			User:          fields[2],
			State:         fields[3],
			CPUPercent:    cpuPercent,
			MemoryPercent: memPercent,
			MemoryBytes:   rss * 1024, // rss is in KB
			VmRss:         rss * 1024,
			VmSize:        vsz * 1024,
			Name:          fields[8],
			Command:       strings.Join(fields[8:], " "),
		}

		// Get UID
		if u, err := user.Lookup(proc.User); err == nil {
			proc.UID, _ = strconv.Atoi(u.Uid)
			proc.GID, _ = strconv.Atoi(u.Gid)
		}

		list.Processes = append(list.Processes, proc)
	}

	list.TotalCount = len(list.Processes)
	return list, nil
}

func GetProcessDetail(pid int) (*ProcessInfo, error) {
	list, err := GetProcessList()
	if err != nil {
		return nil, err
	}

	for _, p := range list.Processes {
		if p.PID == pid {
			// Get full command line
			if out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output(); err == nil {
				p.Command = strings.TrimSpace(string(out))
				p.CommandLine = strings.Fields(p.Command)
			}
			return &p, nil
		}
	}

	return nil, nil
}

func GetProcessesByUser(username string) ([]ProcessInfo, error) {
	list, err := GetProcessList()
	if err != nil {
		return nil, err
	}

	var result []ProcessInfo
	for _, p := range list.Processes {
		if p.User == username {
			result = append(result, p)
		}
	}
	return result, nil
}

// KillProcess sends a signal to a process on macOS
func KillProcess(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}

// ReniceProcess changes the nice value of a process on macOS
func ReniceProcess(pid int, priority int) error {
	cmd := exec.Command("renice", strconv.Itoa(priority), "-p", strconv.Itoa(pid))
	return cmd.Run()
}
