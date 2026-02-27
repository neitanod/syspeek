//go:build darwin

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type Service struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	State       string `json:"state"`    // running, stopped
	SubState    string `json:"subState"` // running, waiting, etc.
	PID         int    `json:"pid,omitempty"`
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type,omitempty"` // system, user, global
}

type ServiceDetail struct {
	Service
	UnitFile       string   `json:"unitFile,omitempty"`
	UnitContent    string   `json:"unitContent,omitempty"`
	ExecStart      string   `json:"execStart,omitempty"`
	ExecStop       string   `json:"execStop,omitempty"`
	User           string   `json:"user,omitempty"`
	Group          string   `json:"group,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	Environment    []string `json:"environment,omitempty"`
	Restart        string   `json:"restart,omitempty"`
	RestartSec     string   `json:"restartSec,omitempty"`
	StartedAt      string   `json:"startedAt,omitempty"`
	MemoryCurrent  uint64   `json:"memoryCurrent,omitempty"`
	CPUUsage       string   `json:"cpuUsage,omitempty"`
	Tasks          int      `json:"tasks,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	WantedBy       []string `json:"wantedBy,omitempty"`
	Label          string   `json:"label,omitempty"`
	LastExitStatus int      `json:"lastExitStatus,omitempty"`
}

type ServicesInfo struct {
	Available bool      `json:"available"`
	Manager   string    `json:"manager"` // systemd, launchd, windows
	Services  []Service `json:"services"`
}

func GetServicesInfo() (ServicesInfo, error) {
	// Check if launchctl is available
	if _, err := exec.LookPath("launchctl"); err != nil {
		return ServicesInfo{Available: false, Manager: "launchd"}, nil
	}

	services, err := getLaunchdServices()
	if err != nil {
		return ServicesInfo{Available: true, Manager: "launchd"}, err
	}

	return ServicesInfo{
		Available: true,
		Manager:   "launchd",
		Services:  services,
	}, nil
}

func getLaunchdServices() ([]Service, error) {
	// Get system services
	cmd := exec.Command("launchctl", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var services []Service
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for i, line := range lines {
		if i == 0 {
			// Skip header
			continue
		}
		if line == "" {
			continue
		}

		// Format: PID	Status	Label
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pidStr := fields[0]
		status := fields[1]
		label := fields[2]

		pid := 0
		state := "stopped"
		if pidStr != "-" {
			pid, _ = strconv.Atoi(pidStr)
			state = "running"
		}

		// Determine service type from label prefix
		serviceType := "user"
		if strings.HasPrefix(label, "com.apple.") {
			serviceType = "system"
		} else if strings.Contains(label, ".") {
			serviceType = "global"
		}

		services = append(services, Service{
			Name:     label,
			State:    state,
			SubState: status,
			PID:      pid,
			Enabled:  true, // launchd services are typically enabled if they appear
			Type:     serviceType,
		})
	}

	return services, nil
}

func GetServiceDetail(name string) (*ServiceDetail, error) {
	// Try to get service info
	cmd := exec.Command("launchctl", "print", "system/"+name)
	output, err := cmd.Output()
	if err != nil {
		// Try user domain
		cmd = exec.Command("launchctl", "print", "user/"+strconv.Itoa(getUID())+"/"+name)
		output, err = cmd.Output()
		if err != nil {
			// Basic fallback
			return getBasicServiceDetail(name)
		}
	}

	return parseLaunchctlPrint(name, string(output))
}

func getUID() int {
	cmd := exec.Command("id", "-u")
	output, _ := cmd.Output()
	uid, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return uid
}

func getBasicServiceDetail(name string) (*ServiceDetail, error) {
	// Get basic info from launchctl list
	cmd := exec.Command("launchctl", "list", name)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	detail := &ServiceDetail{
		Service: Service{
			Name:  name,
			State: "unknown",
		},
		Label: name,
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "\"PID\"") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				pidStr := strings.TrimSpace(strings.Trim(parts[1], ";"))
				detail.PID, _ = strconv.Atoi(pidStr)
				if detail.PID > 0 {
					detail.State = "running"
				}
			}
		} else if strings.HasPrefix(line, "\"LastExitStatus\"") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				statusStr := strings.TrimSpace(strings.Trim(parts[1], ";"))
				detail.LastExitStatus, _ = strconv.Atoi(statusStr)
			}
		}
	}

	// Try to find plist file
	plistPaths := []string{
		"/Library/LaunchDaemons/" + name + ".plist",
		"/Library/LaunchAgents/" + name + ".plist",
		"/System/Library/LaunchDaemons/" + name + ".plist",
		"/System/Library/LaunchAgents/" + name + ".plist",
	}

	for _, path := range plistPaths {
		if content, err := readFile(path); err == nil {
			detail.UnitFile = path
			detail.UnitContent = content
			break
		}
	}

	return detail, nil
}

func parseLaunchctlPrint(name string, output string) (*ServiceDetail, error) {
	detail := &ServiceDetail{
		Service: Service{
			Name: name,
		},
		Label: name,
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "pid = ") {
			pidStr := strings.TrimPrefix(line, "pid = ")
			detail.PID, _ = strconv.Atoi(pidStr)
			if detail.PID > 0 {
				detail.State = "running"
			} else {
				detail.State = "stopped"
			}
		} else if strings.HasPrefix(line, "state = ") {
			detail.SubState = strings.TrimPrefix(line, "state = ")
		} else if strings.HasPrefix(line, "program = ") {
			detail.ExecStart = strings.TrimPrefix(line, "program = ")
		} else if strings.HasPrefix(line, "working directory = ") {
			detail.WorkingDir = strings.TrimPrefix(line, "working directory = ")
		} else if strings.HasPrefix(line, "path = ") {
			detail.UnitFile = strings.TrimPrefix(line, "path = ")
		}
	}

	// Read plist content if we have the path
	if detail.UnitFile != "" {
		if content, err := readFile(detail.UnitFile); err == nil {
			detail.UnitContent = content
		}
	}

	return detail, nil
}

func readFile(path string) (string, error) {
	cmd := exec.Command("cat", path)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func GetServiceLogs(name string, lines int) (string, error) {
	// macOS uses unified logging
	cmd := exec.Command("log", "show", "--predicate", "subsystem == '"+name+"'", "--last", strconv.Itoa(lines)+"m", "--style", "compact")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try to find log files
		return "", err
	}

	return string(output), nil
}

func ServiceAction(name string, action string) error {
	var cmd *exec.Cmd

	switch action {
	case "start":
		cmd = exec.Command("launchctl", "start", name)
	case "stop":
		cmd = exec.Command("launchctl", "stop", name)
	case "restart":
		// launchd doesn't have restart, so stop then start
		exec.Command("launchctl", "stop", name).Run()
		cmd = exec.Command("launchctl", "start", name)
	case "enable":
		cmd = exec.Command("launchctl", "load", "-w", name)
	case "disable":
		cmd = exec.Command("launchctl", "unload", "-w", name)
	default:
		return nil
	}

	return cmd.Run()
}
