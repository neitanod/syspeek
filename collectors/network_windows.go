//go:build windows

package collectors

import (
	"net"
	"sync"
	"time"

	gpsnet "github.com/shirou/gopsutil/v3/net"
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

type netIOSnapshot struct {
	rxBytes uint64
	txBytes uint64
	at      time.Time
}

var (
	prevNetIO   = map[string]netIOSnapshot{}
	prevNetIOMu sync.Mutex
)

func GetNetworkInfo() (NetworkInfo, error) {
	info := NetworkInfo{}

	ioStats, err := gpsnet.IOCounters(true)
	if err != nil {
		return info, err
	}

	now := time.Now()
	prevNetIOMu.Lock()
	defer prevNetIOMu.Unlock()

	ifMap := map[string]net.Interface{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, ifc := range ifs {
			ifMap[ifc.Name] = ifc
		}
	}

	for _, s := range ioStats {
		ni := NetworkInterface{
			Name:    s.Name,
			RxBytes: s.BytesRecv,
			TxBytes: s.BytesSent,
		}

		if ifc, ok := ifMap[s.Name]; ok {
			ni.IsUp = ifc.Flags&net.FlagUp != 0
			ni.IsLoopback = ifc.Flags&net.FlagLoopback != 0
			if addrs, err := ifc.Addrs(); err == nil {
				for _, addr := range addrs {
					ni.IPAddresses = append(ni.IPAddresses, addr.String())
				}
			}
		}

		cur := netIOSnapshot{rxBytes: s.BytesRecv, txBytes: s.BytesSent, at: now}
		if prev, ok := prevNetIO[s.Name]; ok {
			elapsed := now.Sub(prev.at).Seconds()
			if elapsed > 0 {
				if s.BytesRecv >= prev.rxBytes {
					ni.RxSpeed = uint64(float64(s.BytesRecv-prev.rxBytes) / elapsed)
				}
				if s.BytesSent >= prev.txBytes {
					ni.TxSpeed = uint64(float64(s.BytesSent-prev.txBytes) / elapsed)
				}
			}
		}
		prevNetIO[s.Name] = cur

		if !ni.IsLoopback {
			info.TotalRxBytes += ni.RxBytes
			info.TotalTxBytes += ni.TxBytes
			info.TotalRxSpeed += ni.RxSpeed
			info.TotalTxSpeed += ni.TxSpeed
		}

		info.Interfaces = append(info.Interfaces, ni)
	}

	return info, nil
}
