//go:build windows

package collectors

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	firewallCache    FirewallInfo
	firewallCachedAt time.Time
	firewallMu       sync.Mutex
	firewallTTL      = 20 * time.Second
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
	firewallMu.Lock()
	if !firewallCachedAt.IsZero() && time.Since(firewallCachedAt) < firewallTTL {
		cached := firewallCache
		firewallMu.Unlock()
		return cached, nil
	}
	firewallMu.Unlock()

	info := FirewallInfo{
		Available: true,
		Backend:   "Windows Firewall",
	}

	// Check firewall state
	out, err := exec.Command("netsh", "advfirewall", "show", "allprofiles", "state").Output()
	if err != nil {
		info.Available = false
		return info, nil
	}

	if strings.Contains(string(out), "ON") {
		info.Active = true
	}

	// Get some rules (simplified - full rule parsing is complex)
	rulesOut, err := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name=all", "dir=in").Output()
	if err == nil {
		lines := strings.Split(string(rulesOut), "\n")
		var currentRule *FirewallRule

		for _, line := range lines {
			line = strings.TrimSpace(line)

			if strings.HasPrefix(line, "Rule Name:") {
				if currentRule != nil && currentRule.Chain != "" {
					info.Rules = append(info.Rules, *currentRule)
				}
				currentRule = &FirewallRule{
					Chain: "IN",
				}
			} else if currentRule != nil {
				if strings.HasPrefix(line, "Protocol:") {
					currentRule.Protocol = strings.TrimSpace(strings.TrimPrefix(line, "Protocol:"))
				} else if strings.HasPrefix(line, "LocalPort:") {
					currentRule.Port = strings.TrimSpace(strings.TrimPrefix(line, "LocalPort:"))
				} else if strings.HasPrefix(line, "Action:") {
					action := strings.TrimSpace(strings.TrimPrefix(line, "Action:"))
					if action == "Allow" {
						currentRule.Action = "ACCEPT"
					} else if action == "Block" {
						currentRule.Action = "DROP"
					} else {
						currentRule.Action = action
					}
				}
			}

			// Limit to first 50 rules for performance
			if len(info.Rules) >= 50 {
				break
			}
		}

		if currentRule != nil && currentRule.Chain != "" {
			info.Rules = append(info.Rules, *currentRule)
		}
	}

	firewallMu.Lock()
	firewallCache = info
	firewallCachedAt = time.Now()
	firewallMu.Unlock()

	return info, nil
}
