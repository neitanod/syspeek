//go:build darwin

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type Socket struct {
	Protocol    string `json:"protocol"`
	LocalAddr   string `json:"localAddr"`
	LocalPort   int    `json:"localPort"`
	RemoteAddr  string `json:"remoteAddr"`
	RemotePort  int    `json:"remotePort"`
	State       string `json:"state"`
	PID         int    `json:"pid"`
	ProcessName string `json:"processName"`
}

type SocketInfo struct {
	TCP         []Socket `json:"tcp"`
	UDP         []Socket `json:"udp"`
	Total       int      `json:"total"`
	Listen      int      `json:"listen"`
	Established int      `json:"established"`
}

func GetSocketInfo() (SocketInfo, error) {
	info := SocketInfo{}

	// Use netstat to get socket info
	// Note: lsof gives better info but requires root for some connections
	out, err := exec.Command("netstat", "-an", "-p", "tcp").Output()
	if err == nil {
		info.TCP = parseNetstatOutput(string(out), "tcp")
	}

	out, err = exec.Command("netstat", "-an", "-p", "udp").Output()
	if err == nil {
		info.UDP = parseNetstatOutput(string(out), "udp")
	}

	// Count stats
	for _, s := range info.TCP {
		if s.State == "LISTEN" {
			info.Listen++
		} else if s.State == "ESTABLISHED" {
			info.Established++
		}
	}

	info.Total = len(info.TCP) + len(info.UDP)

	return info, nil
}

func parseNetstatOutput(output, protocol string) []Socket {
	var sockets []Socket

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "tcp") && !strings.HasPrefix(line, "udp") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Parse local address
		localAddr, localPort := parseAddress(fields[3])
		remoteAddr, remotePort := parseAddress(fields[4])

		state := ""
		if len(fields) > 5 {
			state = fields[5]
		}

		sockets = append(sockets, Socket{
			Protocol:   protocol,
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      state,
		})
	}

	return sockets
}

func parseAddress(addr string) (string, int) {
	// Format: 127.0.0.1.80 or *.80
	lastDot := strings.LastIndex(addr, ".")
	if lastDot == -1 {
		return addr, 0
	}

	ip := addr[:lastDot]
	port, _ := strconv.Atoi(addr[lastDot+1:])

	if ip == "*" {
		ip = "0.0.0.0"
	}

	return ip, port
}

func GetSocketsByPID(pid int) ([]Socket, error) {
	// Use lsof to get connections for a specific PID
	var sockets []Socket

	out, err := exec.Command("lsof", "-i", "-n", "-P", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return sockets, nil // May fail without root
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] == "COMMAND" {
			continue
		}

		// Parse the connection info
		protoField := strings.ToLower(fields[7])
		if !strings.HasPrefix(protoField, "tcp") && !strings.HasPrefix(protoField, "udp") {
			continue
		}

		connInfo := fields[8]
		parts := strings.Split(connInfo, "->")

		localAddr, localPort := parseLsofAddress(parts[0])
		var remoteAddr string
		var remotePort int
		if len(parts) > 1 {
			remoteAddr, remotePort = parseLsofAddress(parts[1])
		}

		state := ""
		if len(fields) > 9 {
			state = strings.Trim(fields[9], "()")
		}

		sockets = append(sockets, Socket{
			Protocol:    strings.Split(protoField, "4")[0], // Remove IPv4/6 suffix
			LocalAddr:   localAddr,
			LocalPort:   localPort,
			RemoteAddr:  remoteAddr,
			RemotePort:  remotePort,
			State:       state,
			PID:         pid,
			ProcessName: fields[0],
		})
	}

	return sockets, nil
}

func parseLsofAddress(addr string) (string, int) {
	// Format: host:port
	lastColon := strings.LastIndex(addr, ":")
	if lastColon == -1 {
		return addr, 0
	}

	ip := addr[:lastColon]
	port, _ := strconv.Atoi(addr[lastColon+1:])

	if ip == "*" {
		ip = "0.0.0.0"
	}

	return ip, port
}
