package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"syspeek/collectors"
	"syspeek/config"
)

type SSEData struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func (a *API) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create channels for each data type
	ctx := r.Context()

	// Timers for different refresh rates
	cpuTicker := time.NewTicker(time.Duration(a.config.Refresh.CPU) * time.Millisecond)
	memTicker := time.NewTicker(time.Duration(a.config.Refresh.Memory) * time.Millisecond)
	diskTicker := time.NewTicker(time.Duration(a.config.Refresh.Disk) * time.Millisecond)
	netTicker := time.NewTicker(time.Duration(a.config.Refresh.Network) * time.Millisecond)
	gpuTicker := time.NewTicker(time.Duration(a.config.Refresh.GPU) * time.Millisecond)
	procTicker := time.NewTicker(time.Duration(a.config.Refresh.Processes) * time.Millisecond)
	sockTicker := time.NewTicker(time.Duration(a.config.Refresh.Sockets) * time.Millisecond)
	fwTicker := time.NewTicker(time.Duration(a.config.Refresh.Firewall) * time.Millisecond)

	defer func() {
		cpuTicker.Stop()
		memTicker.Stop()
		diskTicker.Stop()
		netTicker.Stop()
		gpuTicker.Stop()
		procTicker.Stop()
		sockTicker.Stop()
		fwTicker.Stop()
	}()

	// Send initial data immediately
	if !sendInitialData(w, flusher, a.config) {
		return // Client disconnected during initial data
	}

	// Main loop
	for {
		select {
		case <-ctx.Done():
			return

		case <-cpuTicker.C:
			if data, err := collectors.GetCPUInfo(); err == nil {
				if sendSSEEvent(w, flusher, "cpu", data) != nil {
					return // Client disconnected
				}
			}

		case <-memTicker.C:
			if data, err := collectors.GetMemoryInfo(); err == nil {
				if sendSSEEvent(w, flusher, "memory", data) != nil {
					return // Client disconnected
				}
			}

		case <-diskTicker.C:
			if data, err := collectors.GetDiskInfo(); err == nil {
				if sendSSEEvent(w, flusher, "disk", data) != nil {
					return // Client disconnected
				}
			}

		case <-netTicker.C:
			if data, err := collectors.GetNetworkInfo(); err == nil {
				if sendSSEEvent(w, flusher, "network", data) != nil {
					return // Client disconnected
				}
			}

		case <-gpuTicker.C:
			if data, err := collectors.GetGPUInfo(); err == nil {
				if sendSSEEvent(w, flusher, "gpu", data) != nil {
					return // Client disconnected
				}
			}

		case <-procTicker.C:
			if data, err := collectors.GetProcessList(); err == nil {
				if sendSSEEvent(w, flusher, "processes", data) != nil {
					return // Client disconnected
				}
			}

		case <-sockTicker.C:
			if data, err := collectors.GetSocketInfo(); err == nil {
				if sendSSEEvent(w, flusher, "sockets", data) != nil {
					return // Client disconnected
				}
			}

		case <-fwTicker.C:
			if data, err := collectors.GetFirewallInfo(); err == nil {
				if sendSSEEvent(w, flusher, "firewall", data) != nil {
					return // Client disconnected
				}
			}
		}
	}
}

func sendInitialData(w http.ResponseWriter, flusher http.Flusher, cfg *config.Config) bool {
	// Send all data immediately on connection
	// Returns false if client disconnected
	if data, err := collectors.GetCPUInfo(); err == nil {
		if sendSSEEvent(w, flusher, "cpu", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetMemoryInfo(); err == nil {
		if sendSSEEvent(w, flusher, "memory", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetDiskInfo(); err == nil {
		if sendSSEEvent(w, flusher, "disk", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetNetworkInfo(); err == nil {
		if sendSSEEvent(w, flusher, "network", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetGPUInfo(); err == nil {
		if sendSSEEvent(w, flusher, "gpu", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetProcessList(); err == nil {
		if sendSSEEvent(w, flusher, "processes", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetSocketInfo(); err == nil {
		if sendSSEEvent(w, flusher, "sockets", data) != nil {
			return false
		}
	}
	if data, err := collectors.GetFirewallInfo(); err == nil {
		if sendSSEEvent(w, flusher, "firewall", data) != nil {
			return false
		}
	}
	return true
}

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) error {
	sseData := SSEData{
		Type: eventType,
		Data: data,
	}

	jsonData, err := json.Marshal(sseData)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: %s\n", eventType)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "data: %s\n\n", jsonData)
	if err != nil {
		return err
	}

	flusher.Flush()
	return nil
}
