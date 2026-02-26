//go:build windows

package collectors

import (
	"net"
	"os/exec"
	"strconv"
	"strings"
)

type NetworkInterface struct {
	Name        string   `json:"name"`
	IPAddresses []string `json:"ipAddresses"`
	IsUp        bool     `json:"isUp"`
	IsLoopback  bool     `json:"isLoopback"`
	RxBytes     uint64   `json:"rxBytes"`
	TxBytes     uint64   `json:"txBytes"`
	RxSpeed     uint64   `json:"rxSpeed"`
	TxSpeed     uint64   `json:"txSpeed"`
}

type NetworkInfo struct {
	Interfaces   []NetworkInterface `json:"interfaces"`
	TotalRxBytes uint64             `json:"totalRxBytes"`
	TotalTxBytes uint64             `json:"totalTxBytes"`
	TotalRxSpeed uint64             `json:"totalRxSpeed"`
	TotalTxSpeed uint64             `json:"totalTxSpeed"`
}

func GetNetworkInfo() (NetworkInfo, error) {
	info := NetworkInfo{}

	// Get interfaces from Go
	interfaces, err := net.Interfaces()
	if err != nil {
		return info, err
	}

	// Get network stats from netstat
	statsMap := make(map[string]struct{ rx, tx uint64 })

	// Use wmic for network stats
	out, _ := exec.Command("wmic", "path", "Win32_PerfFormattedData_Tcpip_NetworkInterface", "get", "Name,BytesReceivedPerSec,BytesSentPerSec", "/value").Output()
	blocks := strings.Split(string(out), "\r\n\r\n")
	for _, block := range blocks {
		var name string
		var rx, tx uint64
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Name=") {
				name = strings.TrimPrefix(line, "Name=")
			} else if strings.HasPrefix(line, "BytesReceivedPerSec=") {
				rx, _ = strconv.ParseUint(strings.TrimPrefix(line, "BytesReceivedPerSec="), 10, 64)
			} else if strings.HasPrefix(line, "BytesSentPerSec=") {
				tx, _ = strconv.ParseUint(strings.TrimPrefix(line, "BytesSentPerSec="), 10, 64)
			}
		}
		if name != "" {
			statsMap[name] = struct{ rx, tx uint64 }{rx, tx}
		}
	}

	for _, iface := range interfaces {
		ni := NetworkInterface{
			Name:       iface.Name,
			IsUp:       iface.Flags&net.FlagUp != 0,
			IsLoopback: iface.Flags&net.FlagLoopback != 0,
		}

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ni.IPAddresses = append(ni.IPAddresses, addr.String())
		}

		// Try to match interface name to WMIC output
		for wmicName, stats := range statsMap {
			if strings.Contains(wmicName, iface.Name) || strings.Contains(iface.Name, wmicName) {
				ni.RxSpeed = stats.rx
				ni.TxSpeed = stats.tx
				break
			}
		}

		if !ni.IsLoopback && ni.IsUp {
			info.TotalRxSpeed += ni.RxSpeed
			info.TotalTxSpeed += ni.TxSpeed
		}

		info.Interfaces = append(info.Interfaces, ni)
	}

	return info, nil
}
