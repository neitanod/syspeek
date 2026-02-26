//go:build windows

package collectors

import (
	"net"
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

	// Get network stats using PowerShell
	statsMap := make(map[string]struct {
		rxBytes, txBytes uint64
		rxSpeed, txSpeed uint64
	})

	// Get bytes per second (speed) from performance counters
	script := `
Get-CimInstance Win32_PerfFormattedData_Tcpip_NetworkInterface | ForEach-Object {
    "$($_.Name)|$($_.BytesReceivedPerSec)|$($_.BytesSentPerSec)|$($_.BytesTotalPerSec)"
}
`
	out, err := runPowerShell(script)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				rxSpeed, _ := strconv.ParseUint(parts[1], 10, 64)
				txSpeed, _ := strconv.ParseUint(parts[2], 10, 64)
				statsMap[parts[0]] = struct {
					rxBytes, txBytes uint64
					rxSpeed, txSpeed uint64
				}{0, 0, rxSpeed, txSpeed}
			}
		}
	}

	// Get total bytes from network adapter stats
	bytesScript := `
Get-CimInstance Win32_PerfRawData_Tcpip_NetworkInterface | ForEach-Object {
    "$($_.Name)|$($_.BytesReceivedPerSec)|$($_.BytesSentPerSec)"
}
`
	bytesOut, err := runPowerShell(bytesScript)
	if err == nil {
		for _, line := range strings.Split(bytesOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				rxBytes, _ := strconv.ParseUint(parts[1], 10, 64)
				txBytes, _ := strconv.ParseUint(parts[2], 10, 64)
				if existing, ok := statsMap[parts[0]]; ok {
					existing.rxBytes = rxBytes
					existing.txBytes = txBytes
					statsMap[parts[0]] = existing
				} else {
					statsMap[parts[0]] = struct {
						rxBytes, txBytes uint64
						rxSpeed, txSpeed uint64
					}{rxBytes, txBytes, 0, 0}
				}
			}
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

		// Try to match interface name to PowerShell output
		// Windows interface names from PowerShell are different from Go's
		for psName, stats := range statsMap {
			// Try various matching strategies
			if strings.Contains(strings.ToLower(psName), strings.ToLower(iface.Name)) ||
				strings.Contains(strings.ToLower(iface.Name), strings.ToLower(psName)) ||
				strings.ReplaceAll(psName, " ", "") == strings.ReplaceAll(iface.Name, " ", "") {
				ni.RxBytes = stats.rxBytes
				ni.TxBytes = stats.txBytes
				ni.RxSpeed = stats.rxSpeed
				ni.TxSpeed = stats.txSpeed
				break
			}
		}

		if !ni.IsLoopback && ni.IsUp {
			info.TotalRxBytes += ni.RxBytes
			info.TotalTxBytes += ni.TxBytes
			info.TotalRxSpeed += ni.RxSpeed
			info.TotalTxSpeed += ni.TxSpeed
		}

		info.Interfaces = append(info.Interfaces, ni)
	}

	// If no matches found for interfaces, just add stats directly from PS
	if info.TotalRxSpeed == 0 && info.TotalTxSpeed == 0 {
		for _, stats := range statsMap {
			info.TotalRxSpeed += stats.rxSpeed
			info.TotalTxSpeed += stats.txSpeed
			info.TotalRxBytes += stats.rxBytes
			info.TotalTxBytes += stats.txBytes
		}
	}

	return info, nil
}
