//go:build linux

package collectors

import (
	"bufio"
	"net"
	"os"
	"strconv"
	"strings"
)

type NetworkInterface struct {
	Name        string   `json:"name"`
	IPAddresses []string `json:"ipAddresses"`
	MAC         string   `json:"mac"`
	RxBytes     uint64   `json:"rxBytes"`
	TxBytes     uint64   `json:"txBytes"`
	RxSpeed     uint64   `json:"rxSpeed"`
	TxSpeed     uint64   `json:"txSpeed"`
	RxPackets   uint64   `json:"rxPackets"`
	TxPackets   uint64   `json:"txPackets"`
	IsUp        bool     `json:"isUp"`
}

type NetworkInfo struct {
	Interfaces   []NetworkInterface `json:"interfaces"`
	TotalRxBytes uint64             `json:"totalRxBytes"`
	TotalTxBytes uint64             `json:"totalTxBytes"`
	TotalRxSpeed uint64             `json:"totalRxSpeed"`
	TotalTxSpeed uint64             `json:"totalTxSpeed"`
}

var previousNetStats map[string]NetworkInterface

func init() {
	previousNetStats = make(map[string]NetworkInterface)
}

func GetNetworkInfo() (*NetworkInfo, error) {
	info := &NetworkInfo{
		Interfaces: []NetworkInterface{},
	}

	// Get network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Get network stats from /proc/net/dev
	netStats := make(map[string]struct {
		rxBytes   uint64
		txBytes   uint64
		rxPackets uint64
		txPackets uint64
	})

	netdev, err := os.Open("/proc/net/dev")
	if err == nil {
		defer netdev.Close()
		scanner := bufio.NewScanner(netdev)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			if lineNum <= 2 {
				continue // Skip header lines
			}

			line := scanner.Text()
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			name := strings.TrimSpace(parts[0])
			fields := strings.Fields(parts[1])
			if len(fields) < 10 {
				continue
			}

			rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
			rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
			txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
			txPackets, _ := strconv.ParseUint(fields[9], 10, 64)

			netStats[name] = struct {
				rxBytes   uint64
				txBytes   uint64
				rxPackets uint64
				txPackets uint64
			}{rxBytes, txBytes, rxPackets, txPackets}
		}
	}

	for _, iface := range ifaces {
		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		ni := NetworkInterface{
			Name:        iface.Name,
			MAC:         iface.HardwareAddr.String(),
			IPAddresses: []string{},
			IsUp:        iface.Flags&net.FlagUp != 0,
		}

		// Get IP addresses
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				ni.IPAddresses = append(ni.IPAddresses, addr.String())
			}
		}

		// Get stats
		if stats, exists := netStats[iface.Name]; exists {
			ni.RxBytes = stats.rxBytes
			ni.TxBytes = stats.txBytes
			ni.RxPackets = stats.rxPackets
			ni.TxPackets = stats.txPackets

			// Calculate speed
			if prev, exists := previousNetStats[iface.Name]; exists {
				ni.RxSpeed = ni.RxBytes - prev.RxBytes
				ni.TxSpeed = ni.TxBytes - prev.TxBytes
			}

			previousNetStats[iface.Name] = NetworkInterface{
				Name:    iface.Name,
				RxBytes: ni.RxBytes,
				TxBytes: ni.TxBytes,
			}

			info.TotalRxBytes += ni.RxBytes
			info.TotalTxBytes += ni.TxBytes
			info.TotalRxSpeed += ni.RxSpeed
			info.TotalTxSpeed += ni.TxSpeed
		}

		info.Interfaces = append(info.Interfaces, ni)
	}

	return info, nil
}
