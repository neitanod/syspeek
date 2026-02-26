//go:build darwin

package collectors

import (
	"os/exec"
	"strings"
)

type FirewallRule struct {
	Chain    string `json:"chain"`
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Action   string `json:"action"`
}

type FirewallInfo struct {
	Available bool           `json:"available"`
	Backend   string         `json:"backend,omitempty"`
	Active    bool           `json:"active"`
	Rules     []FirewallRule `json:"rules,omitempty"`
}

func GetFirewallInfo() (FirewallInfo, error) {
	info := FirewallInfo{
		Available: true,
		Backend:   "pf",
	}

	// Check if pf is enabled
	out, err := exec.Command("pfctl", "-s", "info").Output()
	if err != nil {
		info.Available = false
		return info, nil
	}

	// Check if enabled
	if strings.Contains(string(out), "Status: Enabled") {
		info.Active = true
	}

	// Get rules (simplified)
	rulesOut, err := exec.Command("pfctl", "-s", "rules").Output()
	if err == nil {
		lines := strings.Split(string(rulesOut), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			rule := FirewallRule{
				Chain: "filter",
			}

			// Parse simplified rule info
			if strings.Contains(line, "pass") {
				rule.Action = "ACCEPT"
			} else if strings.Contains(line, "block") {
				rule.Action = "DROP"
			} else {
				continue
			}

			if strings.Contains(line, "tcp") {
				rule.Protocol = "tcp"
			} else if strings.Contains(line, "udp") {
				rule.Protocol = "udp"
			} else {
				rule.Protocol = "all"
			}

			// Try to extract port
			if idx := strings.Index(line, "port"); idx != -1 {
				parts := strings.Fields(line[idx:])
				if len(parts) > 1 {
					rule.Port = parts[1]
				}
			}

			info.Rules = append(info.Rules, rule)
		}
	}

	return info, nil
}
