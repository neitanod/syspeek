package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

type PortMapping struct {
	HostIp        string `json:"hostIp"`
	HostPort      string `json:"hostPort"`
	ContainerPort string `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type Mount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

type Container struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Image        string        `json:"image"`
	Command      string        `json:"command"`
	Created      string        `json:"created"`
	State        string        `json:"state"`
	Status       string        `json:"status"`
	Ports        string        `json:"ports"` // For list view (simple string)
	PortMappings []PortMapping `json:"portMappings,omitempty"` // For detail view
	Mounts       []Mount       `json:"mounts,omitempty"`
	Env          []string      `json:"env,omitempty"`
	// Stats
	CPUPercent   float64 `json:"cpuPercent,omitempty"`
	MemoryUsage  uint64  `json:"memoryUsage,omitempty"`
	MemoryLimit  uint64  `json:"memoryLimit,omitempty"`
	NetworkRx    uint64  `json:"networkRx,omitempty"`
	NetworkTx    uint64  `json:"networkTx,omitempty"`
	PIDs         int     `json:"pids,omitempty"`
}

type DockerInfo struct {
	Available  bool        `json:"available"`
	Containers []Container `json:"containers"`
}

var dockerAvailable *bool

func checkDockerAvailable() bool {
	if dockerAvailable != nil {
		return *dockerAvailable
	}

	_, err := exec.LookPath("docker")
	if err != nil {
		result := false
		dockerAvailable = &result
		return false
	}

	// Try to run docker ps to verify it works
	ctx, cancel := contextWithTimeout(2 * time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "ps", "-q")
	err = cmd.Run()
	result := err == nil
	dockerAvailable = &result
	return result
}

func GetDockerInfo() DockerInfo {
	if !checkDockerAvailable() {
		return DockerInfo{Available: false}
	}

	containers := getContainerList()

	return DockerInfo{
		Available:  true,
		Containers: containers,
	}
}

func getContainerList() []Container {
	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()

	// Get all containers (including stopped) with JSON format
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var containers []Container
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		var raw struct {
			ID      string `json:"ID"`
			Names   string `json:"Names"`
			Image   string `json:"Image"`
			Command string `json:"Command"`
			Created string `json:"CreatedAt"`
			State   string `json:"State"`
			Status  string `json:"Status"`
			Ports   string `json:"Ports"`
		}

		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		containers = append(containers, Container{
			ID:      raw.ID,
			Name:    strings.TrimPrefix(raw.Names, "/"),
			Image:   raw.Image,
			Command: raw.Command,
			Created: raw.Created,
			State:   strings.ToLower(raw.State),
			Status:  raw.Status,
			Ports:   raw.Ports,
		})
	}

	return containers
}

func GetContainerDetail(containerID string) (*Container, error) {
	if !checkDockerAvailable() {
		return nil, fmt.Errorf("docker not available")
	}

	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()

	// Get detailed container info using docker inspect
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("container not found: %s", containerID)
	}

	var inspectData []struct {
		ID      string `json:"Id"`
		Name    string `json:"Name"`
		Created string `json:"Created"`
		State   struct {
			Status string `json:"Status"`
			Pid    int    `json:"Pid"`
		} `json:"State"`
		Config struct {
			Image string   `json:"Image"`
			Cmd   []string `json:"Cmd"`
			Env   []string `json:"Env"`
		} `json:"Config"`
		HostConfig struct {
			PortBindings map[string][]struct {
				HostIp   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			Mode        string `json:"Mode"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIp   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}

	if err := json.Unmarshal(output, &inspectData); err != nil {
		return nil, err
	}

	if len(inspectData) == 0 {
		return nil, fmt.Errorf("container not found")
	}

	data := inspectData[0]

	// Parse port mappings
	var ports []PortMapping
	for portSpec, bindings := range data.NetworkSettings.Ports {
		parts := strings.Split(portSpec, "/")
		containerPort := parts[0]
		protocol := "tcp"
		if len(parts) > 1 {
			protocol = parts[1]
		}

		if len(bindings) > 0 {
			for _, binding := range bindings {
				ports = append(ports, PortMapping{
					HostIp:        binding.HostIp,
					HostPort:      binding.HostPort,
					ContainerPort: containerPort,
					Protocol:      protocol,
				})
			}
		} else {
			ports = append(ports, PortMapping{
				ContainerPort: containerPort,
				Protocol:      protocol,
			})
		}
	}

	// Parse mounts
	var mounts []Mount
	for _, m := range data.Mounts {
		mounts = append(mounts, Mount{
			Type:        m.Type,
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			RW:          m.RW,
		})
	}

	// Get stats if container is running
	var cpuPercent float64
	var memUsage, memLimit, netRx, netTx uint64
	var pids int

	if data.State.Status == "running" {
		stats := getContainerStats(containerID)
		if stats != nil {
			cpuPercent = stats.CPUPercent
			memUsage = stats.MemoryUsage
			memLimit = stats.MemoryLimit
			netRx = stats.NetworkRx
			netTx = stats.NetworkTx
			pids = stats.PIDs
		}
	}

	container := &Container{
		ID:           data.ID[:12],
		Name:         strings.TrimPrefix(data.Name, "/"),
		Image:        data.Config.Image,
		Command:      strings.Join(data.Config.Cmd, " "),
		Created:      data.Created,
		State:        data.State.Status,
		Status:       data.State.Status,
		PortMappings: ports,
		Mounts:       mounts,
		Env:          data.Config.Env,
		CPUPercent:   cpuPercent,
		MemoryUsage:  memUsage,
		MemoryLimit:  memLimit,
		NetworkRx:    netRx,
		NetworkTx:    netTx,
		PIDs:         pids,
	}

	return container, nil
}

type containerStats struct {
	CPUPercent  float64
	MemoryUsage uint64
	MemoryLimit uint64
	NetworkRx   uint64
	NetworkTx   uint64
	PIDs        int
}

func getContainerStats(containerID string) *containerStats {
	ctx, cancel := contextWithTimeout(3 * time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "stats", containerID, "--no-stream", "--format", "{{json .}}")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var raw struct {
		CPUPerc  string `json:"CPUPerc"`
		MemUsage string `json:"MemUsage"`
		NetIO    string `json:"NetIO"`
		PIDs     string `json:"PIDs"`
	}

	if err := json.Unmarshal(output, &raw); err != nil {
		return nil
	}

	stats := &containerStats{}

	// Parse CPU percentage (e.g., "0.50%")
	cpuStr := strings.TrimSuffix(raw.CPUPerc, "%")
	fmt.Sscanf(cpuStr, "%f", &stats.CPUPercent)

	// Parse memory (e.g., "54.3MiB / 7.764GiB")
	memParts := strings.Split(raw.MemUsage, " / ")
	if len(memParts) == 2 {
		stats.MemoryUsage = parseSize(strings.TrimSpace(memParts[0]))
		stats.MemoryLimit = parseSize(strings.TrimSpace(memParts[1]))
	}

	// Parse network I/O (e.g., "1.45kB / 0B")
	netParts := strings.Split(raw.NetIO, " / ")
	if len(netParts) == 2 {
		stats.NetworkRx = parseSize(strings.TrimSpace(netParts[0]))
		stats.NetworkTx = parseSize(strings.TrimSpace(netParts[1]))
	}

	// Parse PIDs
	fmt.Sscanf(raw.PIDs, "%d", &stats.PIDs)

	return stats
}

func parseSize(s string) uint64 {
	s = strings.TrimSpace(s)

	var value float64
	var unit string

	fmt.Sscanf(s, "%f%s", &value, &unit)

	unit = strings.ToLower(unit)

	switch {
	case strings.HasPrefix(unit, "k"):
		return uint64(value * 1024)
	case strings.HasPrefix(unit, "m"):
		return uint64(value * 1024 * 1024)
	case strings.HasPrefix(unit, "g"):
		return uint64(value * 1024 * 1024 * 1024)
	case strings.HasPrefix(unit, "t"):
		return uint64(value * 1024 * 1024 * 1024 * 1024)
	default:
		return uint64(value)
	}
}

func DockerAction(containerID, action string) error {
	if !checkDockerAvailable() {
		return fmt.Errorf("docker not available")
	}

	ctx, cancel := contextWithTimeout(30 * time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch action {
	case "start":
		cmd = exec.CommandContext(ctx, "docker", "start", containerID)
	case "stop":
		cmd = exec.CommandContext(ctx, "docker", "stop", containerID)
	case "restart":
		cmd = exec.CommandContext(ctx, "docker", "restart", containerID)
	case "kill":
		cmd = exec.CommandContext(ctx, "docker", "kill", containerID)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	return cmd.Run()
}
