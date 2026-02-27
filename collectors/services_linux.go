//go:build linux

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type Service struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	State       string `json:"state"`       // running, stopped, failed, etc.
	SubState    string `json:"subState"`    // dead, running, exited, etc.
	PID         int    `json:"pid,omitempty"`
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type,omitempty"` // simple, forking, oneshot, etc.
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
	Restart        string   `json:"restart,omitempty"` // always, on-failure, no
	RestartSec     string   `json:"restartSec,omitempty"`
	StartedAt      string   `json:"startedAt,omitempty"`
	MemoryCurrent  uint64   `json:"memoryCurrent,omitempty"`
	CPUUsage       string   `json:"cpuUsage,omitempty"`
	Tasks          int      `json:"tasks,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	WantedBy       []string `json:"wantedBy,omitempty"`
}

type ServicesInfo struct {
	Available bool      `json:"available"`
	Manager   string    `json:"manager"` // systemd, launchd, windows
	Services  []Service `json:"services"`
}

func GetServicesInfo() (ServicesInfo, error) {
	// Check if systemctl is available
	if _, err := exec.LookPath("systemctl"); err != nil {
		return ServicesInfo{Available: false, Manager: "systemd"}, nil
	}

	services, err := getSystemdServices()
	if err != nil {
		return ServicesInfo{Available: true, Manager: "systemd"}, err
	}

	return ServicesInfo{
		Available: true,
		Manager:   "systemd",
		Services:  services,
	}, nil
}

func getSystemdServices() ([]Service, error) {
	// Get all services with their status
	// Format: UNIT|LOAD|ACTIVE|SUB|DESCRIPTION|MAINPID
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend",
		"--plain", "--output=json")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to text parsing if JSON not available
		return getSystemdServicesText()
	}

	// Parse JSON output
	return parseSystemdJSON(output)
}

func getSystemdServicesText() ([]Service, error) {
	// Fallback: use text output
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend", "--plain")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var services []Service
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Format: UNIT LOAD ACTIVE SUB DESCRIPTION...
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		name := strings.TrimSuffix(fields[0], ".service")
		state := fields[2]  // active, inactive, failed
		subState := fields[3] // running, dead, exited, failed
		description := ""
		if len(fields) > 4 {
			description = strings.Join(fields[4:], " ")
		}

		// Get PID for running services
		pid := 0
		if state == "active" && subState == "running" {
			pid = getServicePID(fields[0])
		}

		// Check if enabled
		enabled := isServiceEnabled(fields[0])

		services = append(services, Service{
			Name:        name,
			Description: description,
			State:       state,
			SubState:    subState,
			PID:         pid,
			Enabled:     enabled,
		})
	}

	return services, nil
}

func parseSystemdJSON(output []byte) ([]Service, error) {
	// systemctl --output=json returns JSON array
	// Try text fallback since JSON format varies by systemd version
	return getSystemdServicesText()
}

func getServicePID(unit string) int {
	cmd := exec.Command("systemctl", "show", "-p", "MainPID", "--value", unit)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	pid, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return pid
}

func isServiceEnabled(unit string) bool {
	cmd := exec.Command("systemctl", "is-enabled", unit)
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) == "enabled"
}

func GetServiceDetail(name string) (*ServiceDetail, error) {
	unit := name
	if !strings.HasSuffix(unit, ".service") {
		unit = name + ".service"
	}

	// Get all properties at once
	cmd := exec.Command("systemctl", "show", unit, "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	props := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		if idx := strings.Index(line, "="); idx > 0 {
			key := line[:idx]
			value := line[idx+1:]
			props[key] = value
		}
	}

	// Parse properties
	pid, _ := strconv.Atoi(props["MainPID"])
	memoryCurrent, _ := strconv.ParseUint(props["MemoryCurrent"], 10, 64)
	tasks, _ := strconv.Atoi(props["TasksCurrent"])

	detail := &ServiceDetail{
		Service: Service{
			Name:        name,
			Description: props["Description"],
			State:       strings.ToLower(props["ActiveState"]),
			SubState:    strings.ToLower(props["SubState"]),
			PID:         pid,
			Enabled:     props["UnitFileState"] == "enabled",
			Type:        props["Type"],
		},
		UnitFile:      props["FragmentPath"],
		ExecStart:     cleanExecPath(props["ExecStart"]),
		ExecStop:      cleanExecPath(props["ExecStop"]),
		User:          props["User"],
		Group:         props["Group"],
		WorkingDir:    props["WorkingDirectory"],
		Restart:       props["Restart"],
		RestartSec:    props["RestartUSec"],
		StartedAt:     props["ActiveEnterTimestamp"],
		MemoryCurrent: memoryCurrent,
		CPUUsage:      props["CPUUsageNSec"],
		Tasks:         tasks,
	}

	// Parse environment
	if env := props["Environment"]; env != "" {
		detail.Environment = strings.Fields(env)
	}

	// Parse dependencies (Requires + Wants)
	var deps []string
	if requires := props["Requires"]; requires != "" {
		deps = append(deps, strings.Fields(requires)...)
	}
	if wants := props["Wants"]; wants != "" {
		deps = append(deps, strings.Fields(wants)...)
	}
	detail.Dependencies = deps

	// Parse WantedBy
	if wantedBy := props["WantedBy"]; wantedBy != "" {
		detail.WantedBy = strings.Fields(wantedBy)
	}

	// Read unit file content
	if detail.UnitFile != "" {
		if content, err := readFile(detail.UnitFile); err == nil {
			detail.UnitContent = content
		}
	}

	return detail, nil
}

func cleanExecPath(s string) string {
	// ExecStart comes as "{ path=/usr/bin/foo ; argv[]=/usr/bin/foo -arg ; ... }"
	// Extract just the path
	if idx := strings.Index(s, "path="); idx >= 0 {
		s = s[idx+5:]
		if end := strings.Index(s, " "); end > 0 {
			s = s[:end]
		}
		if end := strings.Index(s, ";"); end > 0 {
			s = s[:end]
		}
	}
	return strings.TrimSpace(s)
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
	unit := name
	if !strings.HasSuffix(unit, ".service") {
		unit = name + ".service"
	}

	cmd := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func ServiceAction(name string, action string) error {
	unit := name
	if !strings.HasSuffix(unit, ".service") {
		unit = name + ".service"
	}

	var cmd *exec.Cmd
	switch action {
	case "start":
		cmd = exec.Command("systemctl", "start", unit)
	case "stop":
		cmd = exec.Command("systemctl", "stop", unit)
	case "restart":
		cmd = exec.Command("systemctl", "restart", unit)
	case "enable":
		cmd = exec.Command("systemctl", "enable", unit)
	case "disable":
		cmd = exec.Command("systemctl", "disable", unit)
	default:
		return nil
	}

	return cmd.Run()
}
