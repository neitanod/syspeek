//go:build darwin

package collectors

import (
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

var previousNetworkStats map[string]struct {
	rxBytes uint64
	txBytes uint64
	time    time.Time
}

func GetNetworkInfo() (NetworkInfo, error) {
	if previousNetworkStats == nil {
		previousNetworkStats = make(map[string]struct {
			rxBytes uint64
			txBytes uint64
			time    time.Time
		})
	}

	info := NetworkInfo{}
	now := time.Now()

	// Get interface list from Go's net package
	interfaces, err := net.Interfaces()
	if err != nil {
		return info, err
	}

	// Get stats from netstat
	statsMap := make(map[string]struct{ rx, tx uint64 })
	if out, err := exec.Command("netstat", "-ibn").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[0] == "Name" {
				continue
			}
			// Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes
			name := fields[0]
			ibytes, _ := strconv.ParseUint(fields[6], 10, 64)
			obytes, _ := strconv.ParseUint(fields[9], 10, 64)
			if existing, ok := statsMap[name]; ok {
				existing.rx += ibytes
				existing.tx += obytes
				statsMap[name] = existing
			} else {
				statsMap[name] = struct{ rx, tx uint64 }{ibytes, obytes}
			}
		}
	}

	for _, iface := range interfaces {
		ni := NetworkInterface{
			Name:       iface.Name,
			IsUp:       iface.Flags&net.FlagUp != 0,
			IsLoopback: iface.Flags&net.FlagLoopback != 0,
		}

		// Get IP addresses
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ni.IPAddresses = append(ni.IPAddresses, addr.String())
		}

		// Get stats
		if stats, ok := statsMap[iface.Name]; ok {
			ni.RxBytes = stats.rx
			ni.TxBytes = stats.tx

			// Calculate speed
			if prev, ok := previousNetworkStats[iface.Name]; ok {
				elapsed := now.Sub(prev.time).Seconds()
				if elapsed > 0 {
					ni.RxSpeed = uint64(float64(stats.rx-prev.rxBytes) / elapsed)
					ni.TxSpeed = uint64(float64(stats.tx-prev.txBytes) / elapsed)
				}
			}

			// Store current stats
			previousNetworkStats[iface.Name] = struct {
				rxBytes uint64
				txBytes uint64
				time    time.Time
			}{stats.rx, stats.tx, now}
		}

		// Skip loopback for totals
		if !ni.IsLoopback && ni.IsUp {
			info.TotalRxBytes += ni.RxBytes
			info.TotalTxBytes += ni.TxBytes
			info.TotalRxSpeed += ni.RxSpeed
			info.TotalTxSpeed += ni.TxSpeed
		}

		info.Interfaces = append(info.Interfaces, ni)
	}

	return info, nil
}
