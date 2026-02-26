//go:build windows

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

	// Use netstat to get connections
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return info, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		proto := strings.ToLower(fields[0])
		if proto != "tcp" && proto != "udp" {
			continue
		}

		localAddr, localPort := parseWindowsAddress(fields[1])
		remoteAddr, remotePort := parseWindowsAddress(fields[2])

		var state string
		var pid int
		if proto == "tcp" && len(fields) >= 5 {
			state = fields[3]
			pid, _ = strconv.Atoi(fields[4])
		} else if proto == "udp" && len(fields) >= 4 {
			pid, _ = strconv.Atoi(fields[3])
		}

		sock := Socket{
			Protocol:   proto,
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      state,
			PID:        pid,
		}

		if proto == "tcp" {
			info.TCP = append(info.TCP, sock)
			if state == "LISTENING" {
				info.Listen++
			} else if state == "ESTABLISHED" {
				info.Established++
			}
		} else {
			info.UDP = append(info.UDP, sock)
		}
	}

	info.Total = len(info.TCP) + len(info.UDP)
	return info, nil
}

func parseWindowsAddress(addr string) (string, int) {
	// Format: 0.0.0.0:80 or [::]:80
	if strings.HasPrefix(addr, "[") {
		// IPv6
		end := strings.Index(addr, "]:")
		if end == -1 {
			return addr, 0
		}
		ip := addr[1:end]
		port, _ := strconv.Atoi(addr[end+2:])
		return ip, port
	}

	// IPv4
	lastColon := strings.LastIndex(addr, ":")
	if lastColon == -1 {
		return addr, 0
	}

	ip := addr[:lastColon]
	port, _ := strconv.Atoi(addr[lastColon+1:])
	return ip, port
}

func GetSocketsByPID(pid int) ([]Socket, error) {
	info, err := GetSocketInfo()
	if err != nil {
		return nil, err
	}

	var result []Socket
	for _, s := range info.TCP {
		if s.PID == pid {
			result = append(result, s)
		}
	}
	for _, s := range info.UDP {
		if s.PID == pid {
			result = append(result, s)
		}
	}

	return result, nil
}
