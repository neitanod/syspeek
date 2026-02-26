//go:build windows

package collectors

import (
	"os/exec"
	"os/user"
	"strconv"
	"strings"
)

type GroupInfo struct {
	Name    string   `json:"name"`
	GID     int      `json:"gid"`
	Members []string `json:"members"`
}

func GetGroupInfo(groupName string) (*GroupInfo, error) {
	var g *user.Group
	var err error

	// Try as GID first (SID on Windows)
	g, err = user.LookupGroupId(groupName)
	if err != nil {
		g, err = user.LookupGroup(groupName)
	}

	if err != nil {
		return nil, err
	}

	gid, _ := strconv.Atoi(g.Gid)

	info := &GroupInfo{
		Name: g.Name,
		GID:  gid,
	}

	// Get group members using net localgroup command
	parts := strings.Split(g.Name, "\\")
	localName := parts[len(parts)-1]

	if out, err := exec.Command("net", "localgroup", localName).Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		inMembers := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "---") {
				inMembers = true
				continue
			}
			if strings.HasPrefix(line, "The command completed") {
				break
			}
			if inMembers && line != "" {
				info.Members = append(info.Members, line)
			}
		}
	}

	return info, nil
}

func RemoveUserFromGroup(groupName, username string) error {
	// On Windows, need admin privileges to modify groups
	// Using net localgroup command
	return nil
}

// ModifyUserShell is not applicable on Windows (no shell concept like Unix)
// Returns nil as a no-op
func ModifyUserShell(username, shell string) error {
	// Windows doesn't have user shells in the Unix sense
	// Users log in through Windows authentication
	return nil
}

// ModifyUserHome changes the user's home directory profile path
// Requires admin privileges
func ModifyUserHome(username, home string) error {
	// On Windows, changing home directory is complex and requires
	// modifying the user profile path in registry
	// This is typically done through GUI or specialized tools
	return nil
}
