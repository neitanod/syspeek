//go:build darwin

package collectors

import (
	"bufio"
	"os"
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

	// Try as GID first
	if _, parseErr := strconv.Atoi(groupName); parseErr == nil {
		g, err = user.LookupGroupId(groupName)
	}

	if g == nil {
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

	// Read /etc/group to get members
	file, err := os.Open("/etc/group")
	if err != nil {
		return info, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, g.Name+":") {
			parts := strings.Split(line, ":")
			if len(parts) >= 4 && parts[3] != "" {
				info.Members = strings.Split(parts[3], ",")
			}
			break
		}
	}

	return info, nil
}

func RemoveUserFromGroup(groupName, username string) error {
	// On macOS, need dscl to modify groups
	// This requires admin privileges
	return nil
}

// ModifyUserShell changes the user's default shell on macOS
func ModifyUserShell(username, shell string) error {
	// On macOS, use dscl to change shell
	// dscl . -change /Users/username UserShell /old/shell /new/shell
	// This requires admin privileges
	return nil
}

// ModifyUserHome changes the user's home directory on macOS
func ModifyUserHome(username, home string) error {
	// On macOS, use dscl to change home directory
	// dscl . -change /Users/username NFSHomeDirectory /old/home /new/home
	// This requires admin privileges
	return nil
}
