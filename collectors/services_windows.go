//go:build windows

package collectors

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	servicesCache    ServicesInfo
	servicesCachedAt time.Time
	servicesMu       sync.Mutex
	servicesTTL      = 30 * time.Second
)

type Service struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	State       string `json:"state"`    // running, stopped
	SubState    string `json:"subState"` // Running, Stopped, Paused, etc.
	PID         int    `json:"pid,omitempty"`
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type,omitempty"` // Win32OwnProcess, Win32ShareProcess, etc.
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
	DisplayName    string   `json:"displayName,omitempty"`
	StartType      string   `json:"startType,omitempty"` // Automatic, Manual, Disabled
	ServiceType    string   `json:"serviceType,omitempty"`
	ErrorControl   string   `json:"errorControl,omitempty"`
	BinaryPath     string   `json:"binaryPath,omitempty"`
	Account        string   `json:"account,omitempty"`
}

type ServicesInfo struct {
	Available bool      `json:"available"`
	Manager   string    `json:"manager"` // systemd, launchd, windows
	Services  []Service `json:"services"`
}

func GetServicesInfo() (ServicesInfo, error) {
	servicesMu.Lock()
	if !servicesCachedAt.IsZero() && time.Since(servicesCachedAt) < servicesTTL {
		cached := servicesCache
		servicesMu.Unlock()
		return cached, nil
	}
	servicesMu.Unlock()

	services, err := getWindowsServices()
	if err != nil {
		return ServicesInfo{Available: true, Manager: "windows"}, err
	}

	info := ServicesInfo{
		Available: true,
		Manager:   "windows",
		Services:  services,
	}

	servicesMu.Lock()
	servicesCache = info
	servicesCachedAt = time.Now()
	servicesMu.Unlock()

	return info, nil
}

func getWindowsServices() ([]Service, error) {
	// Single Win32_Service query — avoids one PowerShell roundtrip per service.
	script := `Get-CimInstance Win32_Service | ForEach-Object {
		$desc = if ($_.Description) { $_.Description -replace '\|', '-' -replace "\r?\n", " " } else { "" }
		"$($_.Name)|$($_.DisplayName)|$($_.State)|$($_.ProcessId)|$($_.StartMode)|$desc|$($_.ServiceType)"
	}`

	output, err := runPowerShell(script)
	if err != nil {
		return nil, err
	}

	var services []Service
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, "|")
		if len(fields) < 7 {
			continue
		}

		name := fields[0]
		displayName := fields[1]
		status := fields[2]
		pid, _ := strconv.Atoi(fields[3])
		startType := fields[4]
		description := fields[5]
		serviceType := fields[6]

		state := "stopped"
		if status == "Running" {
			state = "running"
		} else if status == "Paused" {
			state = "paused"
		}

		enabled := startType == "Auto" || startType == "Automatic"

		// Use displayName as description if description is empty
		if description == "" && displayName != name {
			description = displayName
		}

		services = append(services, Service{
			Name:        name,
			Description: description,
			State:       state,
			SubState:    status,
			PID:         pid,
			Enabled:     enabled,
			Type:        serviceType,
		})
	}

	return services, nil
}

func GetServiceDetail(name string) (*ServiceDetail, error) {
	// Get detailed service info using PowerShell
	script := `$svc = Get-CimInstance Win32_Service -Filter "Name='` + name + `'"
if ($svc) {
	$deps = (Get-Service -Name '` + name + `' -ErrorAction SilentlyContinue).ServicesDependedOn | ForEach-Object { $_.Name }
	$depList = $deps -join ","
	"Name:" + $svc.Name
	"DisplayName:" + $svc.DisplayName
	"Description:" + $svc.Description
	"State:" + $svc.State
	"Status:" + $svc.Status
	"PID:" + $svc.ProcessId
	"StartMode:" + $svc.StartMode
	"ServiceType:" + $svc.ServiceType
	"PathName:" + $svc.PathName
	"StartName:" + $svc.StartName
	"ErrorControl:" + $svc.ErrorControl
	"Dependencies:" + $depList
}`

	output, err := runPowerShell(script)
	if err != nil {
		return nil, err
	}

	detail := &ServiceDetail{
		Service: Service{
			Name: name,
		},
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		key := line[:idx]
		value := strings.TrimSpace(line[idx+1:])

		switch key {
		case "Name":
			detail.Name = value
		case "DisplayName":
			detail.DisplayName = value
		case "Description":
			detail.Description = value
		case "State":
			if value == "Running" {
				detail.State = "running"
			} else if value == "Stopped" {
				detail.State = "stopped"
			} else {
				detail.State = strings.ToLower(value)
			}
			detail.SubState = value
		case "PID":
			detail.PID, _ = strconv.Atoi(value)
		case "StartMode":
			detail.StartType = value
			detail.Enabled = value == "Auto" || value == "Automatic"
		case "ServiceType":
			detail.ServiceType = value
			detail.Type = value
		case "PathName":
			detail.BinaryPath = value
			detail.ExecStart = value
		case "StartName":
			detail.Account = value
			detail.User = value
		case "ErrorControl":
			detail.ErrorControl = value
		case "Dependencies":
			if value != "" {
				detail.Dependencies = strings.Split(value, ",")
			}
		}
	}

	return detail, nil
}

func GetServiceLogs(name string, lines int) (string, error) {
	// Get Windows Event Log entries for the service
	script := `Get-WinEvent -FilterHashtable @{LogName='System'; ProviderName='Service Control Manager'} -MaxEvents ` + strconv.Itoa(lines*2) + ` -ErrorAction SilentlyContinue | Where-Object { $_.Message -like '*` + name + `*' } | Select-Object -First ` + strconv.Itoa(lines) + ` | ForEach-Object { "$($_.TimeCreated.ToString('yyyy-MM-dd HH:mm:ss')) $($_.LevelDisplayName): $($_.Message)" }`

	output, err := runPowerShell(script)
	if err != nil {
		return "", err
	}

	return output, nil
}

func ServiceAction(name string, action string) error {
	var script string

	switch action {
	case "start":
		script = `Start-Service -Name '` + name + `'`
	case "stop":
		script = `Stop-Service -Name '` + name + `' -Force`
	case "restart":
		script = `Restart-Service -Name '` + name + `' -Force`
	case "enable":
		script = `Set-Service -Name '` + name + `' -StartupType Automatic`
	case "disable":
		script = `Set-Service -Name '` + name + `' -StartupType Disabled`
	default:
		return nil
	}

	_, err := runPowerShell(script)
	return err
}
