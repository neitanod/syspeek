package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
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

type HealthCheck struct {
	Status        string `json:"status"` // healthy, unhealthy, starting, none
	FailingStreak int    `json:"failingStreak,omitempty"`
	Log           string `json:"log,omitempty"` // Last health check output
}

type ContainerNetwork struct {
	Name       string `json:"name"`
	IPAddress  string `json:"ipAddress,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	MacAddress string `json:"macAddress,omitempty"`
}

type RestartPolicy struct {
	Name              string `json:"name"` // no, always, on-failure, unless-stopped
	MaximumRetryCount int    `json:"maximumRetryCount,omitempty"`
}

type ResourceLimits struct {
	CPUShares  int64  `json:"cpuShares,omitempty"`
	CPUQuota   int64  `json:"cpuQuota,omitempty"`
	CPUPeriod  int64  `json:"cpuPeriod,omitempty"`
	Memory     int64  `json:"memory,omitempty"`
	MemorySwap int64  `json:"memorySwap,omitempty"`
	PidsLimit  int64  `json:"pidsLimit,omitempty"`
}

type Container struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Command      string            `json:"command"`
	Created      string            `json:"created"`
	State        string            `json:"state"`
	Status       string            `json:"status"`
	ExitCode     *int              `json:"exitCode,omitempty"` // nil if running, 0+ if exited
	Ports        string            `json:"ports"`                  // For list view (simple string)
	PortMappings []PortMapping     `json:"portMappings,omitempty"` // For detail view
	Mounts       []Mount           `json:"mounts,omitempty"`
	Env          []string          `json:"env,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"` // All labels including compose info
	// Stats
	CPUPercent  float64 `json:"cpuPercent,omitempty"`
	MemoryUsage uint64  `json:"memoryUsage,omitempty"`
	MemoryLimit uint64  `json:"memoryLimit,omitempty"`
	NetworkRx   uint64  `json:"networkRx,omitempty"`
	NetworkTx   uint64  `json:"networkTx,omitempty"`
	PIDs        int     `json:"pids,omitempty"`
	// Extended info
	HealthCheck    *HealthCheck       `json:"healthCheck,omitempty"`
	Networks       []ContainerNetwork `json:"networks,omitempty"`
	RestartPolicy  *RestartPolicy     `json:"restartPolicy,omitempty"`
	ResourceLimits *ResourceLimits    `json:"resourceLimits,omitempty"`
}

type DockerInfo struct {
	Available  bool        `json:"available"`
	Containers []Container `json:"containers"`
}

var dockerAvailable *bool
var exitCodeRegex = regexp.MustCompile(`Exited \((\d+)\)`)

// parseExitCode extracts the exit code from status like "Exited (1) 2 hours ago"
func parseExitCode(status string) *int {
	matches := exitCodeRegex.FindStringSubmatch(status)
	if len(matches) >= 2 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return &code
		}
	}
	return nil
}

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
			ID:       raw.ID,
			Name:     strings.TrimPrefix(raw.Names, "/"),
			Image:    raw.Image,
			Command:  raw.Command,
			Created:  raw.Created,
			State:    strings.ToLower(raw.State),
			Status:   raw.Status,
			ExitCode: parseExitCode(raw.Status),
			Ports:    raw.Ports,
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
			Status  string `json:"Status"`
			Pid     int    `json:"Pid"`
			Health  *struct {
				Status        string `json:"Status"`
				FailingStreak int    `json:"FailingStreak"`
				Log           []struct {
					Output string `json:"Output"`
				} `json:"Log"`
			} `json:"Health"`
		} `json:"State"`
		Config struct {
			Image  string            `json:"Image"`
			Cmd    []string          `json:"Cmd"`
			Env    []string          `json:"Env"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		HostConfig struct {
			PortBindings map[string][]struct {
				HostIp   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
			RestartPolicy struct {
				Name              string `json:"Name"`
				MaximumRetryCount int    `json:"MaximumRetryCount"`
			} `json:"RestartPolicy"`
			// Resource limits
			CpuShares  int64 `json:"CpuShares"`
			CpuQuota   int64 `json:"CpuQuota"`
			CpuPeriod  int64 `json:"CpuPeriod"`
			Memory     int64 `json:"Memory"`
			MemorySwap int64 `json:"MemorySwap"`
			PidsLimit  int64 `json:"PidsLimit"`
		} `json:"HostConfig"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			Mode        string `json:"Mode"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Ports    map[string][]struct {
				HostIp   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
			Networks map[string]struct {
				IPAddress  string `json:"IPAddress"`
				Gateway    string `json:"Gateway"`
				MacAddress string `json:"MacAddress"`
			} `json:"Networks"`
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

	// Parse networks
	var networks []ContainerNetwork
	for name, net := range data.NetworkSettings.Networks {
		networks = append(networks, ContainerNetwork{
			Name:       name,
			IPAddress:  net.IPAddress,
			Gateway:    net.Gateway,
			MacAddress: net.MacAddress,
		})
	}

	// Parse health check
	var healthCheck *HealthCheck
	if data.State.Health != nil {
		hc := &HealthCheck{
			Status:        data.State.Health.Status,
			FailingStreak: data.State.Health.FailingStreak,
		}
		if len(data.State.Health.Log) > 0 {
			hc.Log = data.State.Health.Log[len(data.State.Health.Log)-1].Output
		}
		healthCheck = hc
	}

	// Parse restart policy
	var restartPolicy *RestartPolicy
	if data.HostConfig.RestartPolicy.Name != "" {
		restartPolicy = &RestartPolicy{
			Name:              data.HostConfig.RestartPolicy.Name,
			MaximumRetryCount: data.HostConfig.RestartPolicy.MaximumRetryCount,
		}
	}

	// Parse resource limits
	var resourceLimits *ResourceLimits
	if data.HostConfig.Memory > 0 || data.HostConfig.CpuShares > 0 || data.HostConfig.CpuQuota > 0 {
		resourceLimits = &ResourceLimits{
			CPUShares:  data.HostConfig.CpuShares,
			CPUQuota:   data.HostConfig.CpuQuota,
			CPUPeriod:  data.HostConfig.CpuPeriod,
			Memory:     data.HostConfig.Memory,
			MemorySwap: data.HostConfig.MemorySwap,
			PidsLimit:  data.HostConfig.PidsLimit,
		}
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
		ID:             data.ID[:12],
		Name:           strings.TrimPrefix(data.Name, "/"),
		Image:          data.Config.Image,
		Command:        strings.Join(data.Config.Cmd, " "),
		Created:        data.Created,
		State:          data.State.Status,
		Status:         data.State.Status,
		PortMappings:   ports,
		Mounts:         mounts,
		Env:            data.Config.Env,
		Labels:         data.Config.Labels,
		CPUPercent:     cpuPercent,
		MemoryUsage:    memUsage,
		MemoryLimit:    memLimit,
		NetworkRx:      netRx,
		NetworkTx:      netTx,
		PIDs:           pids,
		HealthCheck:    healthCheck,
		Networks:       networks,
		RestartPolicy:  restartPolicy,
		ResourceLimits: resourceLimits,
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
	case "pause":
		cmd = exec.CommandContext(ctx, "docker", "pause", containerID)
	case "unpause":
		cmd = exec.CommandContext(ctx, "docker", "unpause", containerID)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	return cmd.Run()
}

// GetContainerLogs returns the last n lines of container logs
func GetContainerLogs(containerID string, tail int) (string, error) {
	if !checkDockerAvailable() {
		return "", fmt.Errorf("docker not available")
	}

	ctx, cancel := contextWithTimeout(10 * time.Second)
	defer cancel()

	tailStr := fmt.Sprintf("%d", tail)
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", tailStr, "--timestamps", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %v", err)
	}

	return string(output), nil
}

// ContainerProcess represents a process running inside a container
type ContainerProcess struct {
	UID     string `json:"uid"`
	PID     string `json:"pid"`
	PPID    string `json:"ppid"`
	CPU     string `json:"cpu"`
	STime   string `json:"stime"`
	TTY     string `json:"tty"`
	Time    string `json:"time"`
	Command string `json:"command"`
}

// GetContainerTop returns processes running inside a container
func GetContainerTop(containerID string) ([]ContainerProcess, error) {
	if !checkDockerAvailable() {
		return nil, fmt.Errorf("docker not available")
	}

	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "top", containerID, "-o", "uid,pid,ppid,%cpu,stime,tty,time,cmd")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get top: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return []ContainerProcess{}, nil
	}

	var processes []ContainerProcess
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) >= 8 {
			processes = append(processes, ContainerProcess{
				UID:     fields[0],
				PID:     fields[1],
				PPID:    fields[2],
				CPU:     fields[3],
				STime:   fields[4],
				TTY:     fields[5],
				Time:    fields[6],
				Command: strings.Join(fields[7:], " "),
			})
		}
	}

	return processes, nil
}

// GetContainerInspect returns raw docker inspect JSON
func GetContainerInspect(containerID string) (string, error) {
	if !checkDockerAvailable() {
		return "", fmt.Errorf("docker not available")
	}

	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("container not found: %s", containerID)
	}

	return string(output), nil
}
