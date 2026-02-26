//go:build linux

package collectors

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ProcessBasic struct {
	PID         int     `json:"pid"`
	PPID        int     `json:"ppid"`
	Name        string  `json:"name"`
	Command     string  `json:"command"`
	User        string  `json:"user"`
	State       string  `json:"state"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryBytes uint64  `json:"memoryBytes"`
	MemoryPercent float64 `json:"memoryPercent"`
	Threads     int     `json:"threads"`
	Nice        int     `json:"nice"`
	StartTime   int64   `json:"startTime"`
}

type ProcessConnection struct {
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"localAddr"`
	LocalPort  int    `json:"localPort"`
	RemoteAddr string `json:"remoteAddr"`
	RemotePort int    `json:"remotePort"`
	State      string `json:"state"`
}

type ProcessFD struct {
	FD     int    `json:"fd"`
	Type   string `json:"type"`
	Target string `json:"target"`
}

type ProcessEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ProcessDetail struct {
	ProcessBasic
	CommandLine   []string            `json:"commandLine"`
	Cwd           string              `json:"cwd"`
	Exe           string              `json:"exe"`
	Environ       []ProcessEnvVar     `json:"environ"`
	FDs           []ProcessFD         `json:"fds"`
	Connections   []ProcessConnection `json:"connections"`
	Children      []int               `json:"children"`
	UID           int                 `json:"uid"`
	GID           int                 `json:"gid"`
	Groups        []int               `json:"groups"`
	Uptime        string              `json:"uptime"`
	VmSize        uint64              `json:"vmSize"`
	VmRSS         uint64              `json:"vmRss"`
	VmData        uint64              `json:"vmData"`
	VmStack       uint64              `json:"vmStack"`
	VmSwap        uint64              `json:"vmSwap"`
	IOReadBytes   uint64              `json:"ioReadBytes"`
	IOWriteBytes  uint64              `json:"ioWriteBytes"`
	VoluntaryCtxSwitches   uint64     `json:"voluntaryCtxSwitches"`
	InvoluntaryCtxSwitches uint64     `json:"involuntaryCtxSwitches"`
}

type ProcessList struct {
	Processes  []ProcessBasic `json:"processes"`
	TotalCount int            `json:"totalCount"`
}

var (
	previousCPUTicks map[int]uint64
	previousTime     time.Time
	systemBootTime   int64
	totalMemory      uint64
	processMutex     sync.Mutex
)

func init() {
	previousCPUTicks = make(map[int]uint64)
	previousTime = time.Now()

	// Get system boot time
	uptime, _ := os.ReadFile("/proc/uptime")
	if len(uptime) > 0 {
		fields := strings.Fields(string(uptime))
		if len(fields) > 0 {
			uptimeSec, _ := strconv.ParseFloat(fields[0], 64)
			systemBootTime = time.Now().Unix() - int64(uptimeSec)
		}
	}

	// Get total memory
	meminfo, _ := os.ReadFile("/proc/meminfo")
	if len(meminfo) > 0 {
		lines := strings.Split(string(meminfo), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					val, _ := strconv.ParseUint(fields[1], 10, 64)
					totalMemory = val * 1024 // kB to bytes
				}
				break
			}
		}
	}
}

func GetProcessList() (*ProcessList, error) {
	list := &ProcessList{
		Processes: []ProcessBasic{},
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	processMutex.Lock()
	elapsed := now.Sub(previousTime).Seconds()
	processMutex.Unlock()
	if elapsed < 0.1 {
		elapsed = 0.1
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		proc, err := getProcessBasic(pid, elapsed)
		if err != nil {
			continue
		}

		list.Processes = append(list.Processes, *proc)
	}

	processMutex.Lock()
	previousTime = now
	processMutex.Unlock()
	list.TotalCount = len(list.Processes)

	return list, nil
}

func getProcessBasic(pid int, elapsed float64) (*ProcessBasic, error) {
	proc := &ProcessBasic{PID: pid}
	procPath := fmt.Sprintf("/proc/%d", pid)

	// Read stat file
	statPath := filepath.Join(procPath, "stat")
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return nil, err
	}

	// Parse stat - careful with comm field which can contain spaces and parens
	statStr := string(statData)
	openParen := strings.Index(statStr, "(")
	closeParen := strings.LastIndex(statStr, ")")

	if openParen == -1 || closeParen == -1 {
		return nil, fmt.Errorf("invalid stat format")
	}

	proc.Name = statStr[openParen+1 : closeParen]
	fields := strings.Fields(statStr[closeParen+2:])

	if len(fields) < 22 {
		return nil, fmt.Errorf("stat fields too short")
	}

	proc.State = fields[0]
	proc.PPID, _ = strconv.Atoi(fields[1])
	proc.Nice, _ = strconv.Atoi(fields[16])
	proc.Threads, _ = strconv.Atoi(fields[17])

	// Calculate CPU usage
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	totalTicks := utime + stime

	processMutex.Lock()
	if prevTicks, exists := previousCPUTicks[pid]; exists {
		ticksDelta := totalTicks - prevTicks
		// Convert ticks to percentage (assuming 100 ticks/sec)
		proc.CPUPercent = float64(ticksDelta) / elapsed
	}
	previousCPUTicks[pid] = totalTicks
	processMutex.Unlock()

	// Get start time
	starttime, _ := strconv.ParseUint(fields[19], 10, 64)
	proc.StartTime = systemBootTime + int64(starttime/100)

	// Read memory info from statm
	statmPath := filepath.Join(procPath, "statm")
	statmData, err := os.ReadFile(statmPath)
	if err == nil {
		statmFields := strings.Fields(string(statmData))
		if len(statmFields) >= 2 {
			rssPages, _ := strconv.ParseUint(statmFields[1], 10, 64)
			pageSize := uint64(os.Getpagesize())
			proc.MemoryBytes = rssPages * pageSize
			if totalMemory > 0 {
				proc.MemoryPercent = float64(proc.MemoryBytes) / float64(totalMemory) * 100
			}
		}
	}

	// Read cmdline
	cmdlinePath := filepath.Join(procPath, "cmdline")
	cmdlineData, err := os.ReadFile(cmdlinePath)
	if err == nil && len(cmdlineData) > 0 {
		proc.Command = strings.ReplaceAll(string(cmdlineData), "\x00", " ")
		proc.Command = strings.TrimSpace(proc.Command)
	}

	if proc.Command == "" {
		proc.Command = "[" + proc.Name + "]"
	}

	// Get user name from UID
	statusPath := filepath.Join(procPath, "status")
	statusData, err := os.ReadFile(statusPath)
	if err == nil {
		lines := strings.Split(string(statusData), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Uid:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					uid, _ := strconv.Atoi(fields[1])
					proc.User = getUsername(uid)
				}
				break
			}
		}
	}

	return proc, nil
}

func GetProcessDetail(pid int) (*ProcessDetail, error) {
	basic, err := getProcessBasic(pid, 1.0)
	if err != nil {
		return nil, err
	}

	detail := &ProcessDetail{
		ProcessBasic: *basic,
		Environ:      []ProcessEnvVar{},
		FDs:          []ProcessFD{},
		Connections:  []ProcessConnection{},
		Children:     []int{},
		Groups:       []int{},
	}

	procPath := fmt.Sprintf("/proc/%d", pid)

	// Get command line as array
	cmdlineData, err := os.ReadFile(filepath.Join(procPath, "cmdline"))
	if err == nil && len(cmdlineData) > 0 {
		parts := strings.Split(string(cmdlineData), "\x00")
		for _, part := range parts {
			if part != "" {
				detail.CommandLine = append(detail.CommandLine, part)
			}
		}
	}

	// Get cwd
	cwd, err := os.Readlink(filepath.Join(procPath, "cwd"))
	if err == nil {
		detail.Cwd = cwd
	}

	// Get exe
	exe, err := os.Readlink(filepath.Join(procPath, "exe"))
	if err == nil {
		detail.Exe = exe
	}

	// Get environment variables
	environData, err := os.ReadFile(filepath.Join(procPath, "environ"))
	if err == nil && len(environData) > 0 {
		vars := strings.Split(string(environData), "\x00")
		for _, v := range vars {
			if v == "" {
				continue
			}
			parts := strings.SplitN(v, "=", 2)
			if len(parts) == 2 {
				detail.Environ = append(detail.Environ, ProcessEnvVar{
					Name:  parts[0],
					Value: parts[1],
				})
			}
		}
	}

	// Get file descriptors
	fdPath := filepath.Join(procPath, "fd")
	fds, err := os.ReadDir(fdPath)
	if err == nil {
		for _, fd := range fds {
			fdNum, _ := strconv.Atoi(fd.Name())
			target, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if err != nil {
				continue
			}

			fdType := "unknown"
			if strings.HasPrefix(target, "socket:") {
				fdType = "socket"
			} else if strings.HasPrefix(target, "pipe:") {
				fdType = "pipe"
			} else if strings.HasPrefix(target, "/") {
				fdType = "file"
			} else if strings.HasPrefix(target, "anon_inode:") {
				fdType = "anon_inode"
			}

			detail.FDs = append(detail.FDs, ProcessFD{
				FD:     fdNum,
				Type:   fdType,
				Target: target,
			})
		}
	}

	// Get network connections for this process
	detail.Connections = getProcessConnections(pid)

	// Get children
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childPID, err := strconv.Atoi(entry.Name())
		if err != nil || childPID == pid {
			continue
		}

		statPath := filepath.Join("/proc", entry.Name(), "stat")
		statData, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		statStr := string(statData)
		closeParen := strings.LastIndex(statStr, ")")
		if closeParen == -1 {
			continue
		}

		fields := strings.Fields(statStr[closeParen+2:])
		if len(fields) >= 2 {
			ppid, _ := strconv.Atoi(fields[1])
			if ppid == pid {
				detail.Children = append(detail.Children, childPID)
			}
		}
	}

	// Get detailed status info
	statusData, err := os.ReadFile(filepath.Join(procPath, "status"))
	if err == nil {
		lines := strings.Split(string(statusData), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := strings.TrimSuffix(fields[0], ":")
			switch key {
			case "Uid":
				detail.UID, _ = strconv.Atoi(fields[1])
			case "Gid":
				detail.GID, _ = strconv.Atoi(fields[1])
			case "Groups":
				for _, g := range fields[1:] {
					gid, _ := strconv.Atoi(g)
					detail.Groups = append(detail.Groups, gid)
				}
			case "VmSize":
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				detail.VmSize = val * 1024
			case "VmRSS":
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				detail.VmRSS = val * 1024
			case "VmData":
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				detail.VmData = val * 1024
			case "VmStk":
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				detail.VmStack = val * 1024
			case "VmSwap":
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				detail.VmSwap = val * 1024
			case "voluntary_ctxt_switches":
				detail.VoluntaryCtxSwitches, _ = strconv.ParseUint(fields[1], 10, 64)
			case "nonvoluntary_ctxt_switches":
				detail.InvoluntaryCtxSwitches, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}

	// Get I/O stats
	ioData, err := os.ReadFile(filepath.Join(procPath, "io"))
	if err == nil {
		lines := strings.Split(string(ioData), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := strings.TrimSuffix(fields[0], ":")
			switch key {
			case "read_bytes":
				detail.IOReadBytes, _ = strconv.ParseUint(fields[1], 10, 64)
			case "write_bytes":
				detail.IOWriteBytes, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}

	// Calculate uptime
	if detail.StartTime > 0 {
		uptime := time.Now().Unix() - detail.StartTime
		detail.Uptime = formatUptime(float64(uptime))
	}

	return detail, nil
}

func getProcessConnections(pid int) []ProcessConnection {
	connections := []ProcessConnection{}

	// Get socket inodes for this process
	socketInodes := make(map[string]bool)
	fdPath := fmt.Sprintf("/proc/%d/fd", pid)
	fds, err := os.ReadDir(fdPath)
	if err != nil {
		return connections
	}

	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "socket:[") {
			inode := strings.TrimPrefix(strings.TrimSuffix(target, "]"), "socket:[")
			socketInodes[inode] = true
		}
	}

	// Parse TCP connections
	parseProcNet("/proc/net/tcp", "tcp", socketInodes, &connections)
	parseProcNet("/proc/net/tcp6", "tcp6", socketInodes, &connections)
	parseProcNet("/proc/net/udp", "udp", socketInodes, &connections)
	parseProcNet("/proc/net/udp6", "udp6", socketInodes, &connections)

	return connections
}

func parseProcNet(path, protocol string, socketInodes map[string]bool, connections *[]ProcessConnection) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		inode := fields[9]
		if !socketInodes[inode] {
			continue
		}

		localAddr, localPort := parseAddr(fields[1])
		remoteAddr, remotePort := parseAddr(fields[2])
		state := parseState(fields[3])

		*connections = append(*connections, ProcessConnection{
			Protocol:   protocol,
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      state,
		})
	}
}

func parseAddr(addr string) (string, int) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0
	}

	ip := parseHexIP(parts[0])
	port, _ := strconv.ParseInt(parts[1], 16, 32)

	return ip, int(port)
}

func parseHexIP(hex string) string {
	if len(hex) == 8 {
		// IPv4
		bytes := make([]byte, 4)
		for i := 0; i < 4; i++ {
			val, _ := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
			bytes[3-i] = byte(val)
		}
		return fmt.Sprintf("%d.%d.%d.%d", bytes[0], bytes[1], bytes[2], bytes[3])
	}
	if len(hex) == 32 {
		// IPv6 - stored as 4 groups of 4 bytes in little-endian order
		bytes := make([]byte, 16)
		for i := 0; i < 4; i++ {
			// Each group of 8 hex chars (4 bytes) is in little-endian
			group := hex[i*8 : i*8+8]
			for j := 0; j < 4; j++ {
				val, _ := strconv.ParseUint(group[j*2:j*2+2], 16, 8)
				bytes[i*4+(3-j)] = byte(val)
			}
		}
		// Format as IPv6 address
		ip := net.IP(bytes)
		return ip.String()
	}
	return hex
}

func parseState(state string) string {
	states := map[string]string{
		"01": "ESTABLISHED",
		"02": "SYN_SENT",
		"03": "SYN_RECV",
		"04": "FIN_WAIT1",
		"05": "FIN_WAIT2",
		"06": "TIME_WAIT",
		"07": "CLOSE",
		"08": "CLOSE_WAIT",
		"09": "LAST_ACK",
		"0A": "LISTEN",
		"0B": "CLOSING",
	}

	if name, exists := states[state]; exists {
		return name
	}
	return state
}

func getUsername(uid int) string {
	// Simple cache
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return strconv.Itoa(uid)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 3 {
			u, _ := strconv.Atoi(parts[2])
			if u == uid {
				return parts[0]
			}
		}
	}

	return strconv.Itoa(uid)
}

// KillProcess sends a signal to a process
func KillProcess(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}

// ReniceProcess changes the priority of a process
func ReniceProcess(pid int, priority int) error {
	return syscall.Setpriority(syscall.PRIO_PROCESS, pid, priority)
}
