//go:build linux

package collectors

import (
	"os/exec"
	"strconv"
	"strings"
)

type FirewallRule struct {
	Chain       string `json:"chain"`
	Protocol    string `json:"protocol"`
	Port        int    `json:"port"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Action      string `json:"action"`
	Interface   string `json:"interface"`
	Raw         string `json:"raw"`
}

type FirewallInfo struct {
	Available bool           `json:"available"`
	Backend   string         `json:"backend"`
	Active    bool           `json:"active"`
	Rules     []FirewallRule `json:"rules"`
}

func GetFirewallInfo() (*FirewallInfo, error) {
	info := &FirewallInfo{
		Available: false,
		Rules:     []FirewallRule{},
	}

	// Try UFW first (common on Ubuntu)
	if ufwInfo := tryUFW(); ufwInfo != nil {
		return ufwInfo, nil
	}

	// Try firewalld (common on RHEL/Fedora)
	if firewalldInfo := tryFirewalld(); firewalldInfo != nil {
		return firewalldInfo, nil
	}

	// Try nftables
	if nftInfo := tryNftables(); nftInfo != nil {
		return nftInfo, nil
	}

	// Try iptables (fallback)
	if iptInfo := tryIptables(); iptInfo != nil {
		return iptInfo, nil
	}

	return info, nil
}

func tryUFW() *FirewallInfo {
	// Check if ufw is available
	cmd := exec.Command("ufw", "status", "verbose")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	info := &FirewallInfo{
		Available: true,
		Backend:   "ufw",
		Rules:     []FirewallRule{},
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Status:") {
			info.Active = strings.Contains(line, "active")
			continue
		}

		// Parse rules - UFW format: "22/tcp ALLOW IN Anywhere"
		if strings.Contains(line, "ALLOW") || strings.Contains(line, "DENY") || strings.Contains(line, "REJECT") {
			rule := parseUFWRule(line)
			if rule != nil {
				info.Rules = append(info.Rules, *rule)
			}
		}
	}

	return info
}

func parseUFWRule(line string) *FirewallRule {
	rule := &FirewallRule{Raw: line}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil
	}

	// Parse port/protocol
	portProto := parts[0]
	if strings.Contains(portProto, "/") {
		pp := strings.Split(portProto, "/")
		rule.Port, _ = strconv.Atoi(pp[0])
		rule.Protocol = pp[1]
	} else {
		rule.Port, _ = strconv.Atoi(portProto)
	}

	// Parse action
	for _, p := range parts {
		switch p {
		case "ALLOW":
			rule.Action = "ALLOW"
		case "DENY":
			rule.Action = "DENY"
		case "REJECT":
			rule.Action = "REJECT"
		}
	}

	// Parse direction
	if strings.Contains(line, " IN ") {
		rule.Chain = "INPUT"
	} else if strings.Contains(line, " OUT ") {
		rule.Chain = "OUTPUT"
	}

	return rule
}

func tryFirewalld() *FirewallInfo {
	// Check if firewalld is running
	cmd := exec.Command("firewall-cmd", "--state")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	info := &FirewallInfo{
		Available: true,
		Backend:   "firewalld",
		Active:    strings.TrimSpace(string(output)) == "running",
		Rules:     []FirewallRule{},
	}

	// Get open ports
	cmd = exec.Command("firewall-cmd", "--list-ports")
	output, err = cmd.Output()
	if err == nil {
		ports := strings.Fields(string(output))
		for _, p := range ports {
			parts := strings.Split(p, "/")
			if len(parts) == 2 {
				port, _ := strconv.Atoi(parts[0])
				info.Rules = append(info.Rules, FirewallRule{
					Port:     port,
					Protocol: parts[1],
					Action:   "ALLOW",
					Raw:      p,
				})
			}
		}
	}

	// Get services
	cmd = exec.Command("firewall-cmd", "--list-services")
	output, err = cmd.Output()
	if err == nil {
		services := strings.Fields(string(output))
		for _, svc := range services {
			info.Rules = append(info.Rules, FirewallRule{
				Action: "ALLOW",
				Raw:    "service: " + svc,
			})
		}
	}

	return info
}

func tryNftables() *FirewallInfo {
	cmd := exec.Command("nft", "list", "ruleset")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	info := &FirewallInfo{
		Available: true,
		Backend:   "nftables",
		Active:    true,
		Rules:     []FirewallRule{},
	}

	lines := strings.Split(string(output), "\n")
	currentChain := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "chain ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentChain = parts[1]
			}
			continue
		}

		// Parse rules with port specifications
		if strings.Contains(line, "dport") || strings.Contains(line, "sport") {
			rule := FirewallRule{
				Chain: currentChain,
				Raw:   line,
			}

			// Extract protocol
			if strings.Contains(line, "tcp") {
				rule.Protocol = "tcp"
			} else if strings.Contains(line, "udp") {
				rule.Protocol = "udp"
			}

			// Extract port
			if idx := strings.Index(line, "dport "); idx != -1 {
				portStr := strings.Fields(line[idx+6:])[0]
				rule.Port, _ = strconv.Atoi(portStr)
			}

			// Extract action
			if strings.Contains(line, "accept") {
				rule.Action = "ACCEPT"
			} else if strings.Contains(line, "drop") {
				rule.Action = "DROP"
			} else if strings.Contains(line, "reject") {
				rule.Action = "REJECT"
			}

			info.Rules = append(info.Rules, rule)
		}
	}

	return info
}

func tryIptables() *FirewallInfo {
	cmd := exec.Command("iptables", "-L", "-n", "--line-numbers")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	info := &FirewallInfo{
		Available: true,
		Backend:   "iptables",
		Active:    true,
		Rules:     []FirewallRule{},
	}

	lines := strings.Split(string(output), "\n")
	currentChain := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Chain ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentChain = parts[1]
			}
			continue
		}

		// Skip header lines
		if strings.HasPrefix(line, "num") || line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		rule := FirewallRule{
			Chain: currentChain,
			Raw:   line,
		}

		// fields[1] = target (ACCEPT, DROP, REJECT, etc)
		rule.Action = fields[1]

		// fields[2] = protocol
		rule.Protocol = fields[2]

		// fields[4] = source
		rule.Source = fields[4]

		// fields[5] = destination
		if len(fields) > 5 {
			rule.Destination = fields[5]
		}

		// Look for dpt: (destination port)
		for _, f := range fields {
			if strings.HasPrefix(f, "dpt:") {
				portStr := strings.TrimPrefix(f, "dpt:")
				rule.Port, _ = strconv.Atoi(portStr)
			}
		}

		info.Rules = append(info.Rules, rule)
	}

	return info
}
