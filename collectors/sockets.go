package collectors

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Socket struct {
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"localAddr"`
	LocalPort  int    `json:"localPort"`
	RemoteAddr string `json:"remoteAddr"`
	RemotePort int    `json:"remotePort"`
	State      string `json:"state"`
	PID        int    `json:"pid"`
	ProcessName string `json:"processName"`
	Inode      string `json:"inode"`
}

type SocketInfo struct {
	TCP    []Socket `json:"tcp"`
	UDP    []Socket `json:"udp"`
	Unix   []Socket `json:"unix"`
	Total  int      `json:"total"`
	Listen int      `json:"listen"`
	Established int `json:"established"`
}

func GetSocketInfo() (*SocketInfo, error) {
	info := &SocketInfo{
		TCP:  []Socket{},
		UDP:  []Socket{},
		Unix: []Socket{},
	}

	// Build inode to PID/name mapping
	inodeToPID := buildInodeMap()

	// Parse TCP sockets
	tcpSockets := parseNetSockets("/proc/net/tcp", "tcp", inodeToPID)
	tcp6Sockets := parseNetSockets("/proc/net/tcp6", "tcp6", inodeToPID)
	info.TCP = append(info.TCP, tcpSockets...)
	info.TCP = append(info.TCP, tcp6Sockets...)

	// Parse UDP sockets
	udpSockets := parseNetSockets("/proc/net/udp", "udp", inodeToPID)
	udp6Sockets := parseNetSockets("/proc/net/udp6", "udp6", inodeToPID)
	info.UDP = append(info.UDP, udpSockets...)
	info.UDP = append(info.UDP, udp6Sockets...)

	// Parse Unix sockets
	info.Unix = parseUnixSockets(inodeToPID)

	// Calculate totals
	info.Total = len(info.TCP) + len(info.UDP) + len(info.Unix)

	for _, s := range info.TCP {
		if s.State == "LISTEN" {
			info.Listen++
		} else if s.State == "ESTABLISHED" {
			info.Established++
		}
	}

	return info, nil
}

func buildInodeMap() map[string]struct{ pid int; name string } {
	inodeMap := make(map[string]struct{ pid int; name string })

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return inodeMap
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Get process name
		commPath := filepath.Join("/proc", entry.Name(), "comm")
		commData, err := os.ReadFile(commPath)
		procName := ""
		if err == nil {
			procName = strings.TrimSpace(string(commData))
		}

		// Get socket inodes
		fdPath := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if err != nil {
				continue
			}

			if strings.HasPrefix(target, "socket:[") {
				inode := strings.TrimPrefix(strings.TrimSuffix(target, "]"), "socket:[")
				inodeMap[inode] = struct{ pid int; name string }{pid, procName}
			}
		}
	}

	return inodeMap
}

func parseNetSockets(path, protocol string, inodeMap map[string]struct{ pid int; name string }) []Socket {
	sockets := []Socket{}

	file, err := os.Open(path)
	if err != nil {
		return sockets
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

		localAddr, localPort := parseAddr(fields[1])
		remoteAddr, remotePort := parseAddr(fields[2])
		state := parseState(fields[3])
		inode := fields[9]

		socket := Socket{
			Protocol:   protocol,
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      state,
			Inode:      inode,
		}

		if proc, exists := inodeMap[inode]; exists {
			socket.PID = proc.pid
			socket.ProcessName = proc.name
		}

		sockets = append(sockets, socket)
	}

	return sockets
}

func parseUnixSockets(inodeMap map[string]struct{ pid int; name string }) []Socket {
	sockets := []Socket{}

	file, err := os.Open("/proc/net/unix")
	if err != nil {
		return sockets
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
		if len(fields) < 7 {
			continue
		}

		inode := fields[6]
		path := ""
		if len(fields) > 7 {
			path = fields[7]
		}

		state := parseUnixState(fields[5])

		socket := Socket{
			Protocol:   "unix",
			LocalAddr:  path,
			State:      state,
			Inode:      inode,
		}

		if proc, exists := inodeMap[inode]; exists {
			socket.PID = proc.pid
			socket.ProcessName = proc.name
		}

		sockets = append(sockets, socket)
	}

	return sockets
}

func parseUnixState(state string) string {
	states := map[string]string{
		"01": "FREE",
		"02": "UNCONNECTED",
		"03": "CONNECTING",
		"04": "CONNECTED",
		"05": "DISCONNECTING",
	}

	stateInt, err := strconv.ParseInt(state, 16, 32)
	if err != nil {
		return state
	}

	if name, exists := states[fmt.Sprintf("%02d", stateInt)]; exists {
		return name
	}
	return state
}
