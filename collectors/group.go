package collectors

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type GroupInfo struct {
	Name    string   `json:"name"`
	GID     int      `json:"gid"`
	Members []string `json:"members"`
}

// GetGroupInfo returns information about a group
func GetGroupInfo(groupname string) (*GroupInfo, error) {
	file, err := os.Open("/etc/group")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}

		name := parts[0]
		if name != groupname {
			continue
		}

		gid, _ := strconv.Atoi(parts[2])
		members := []string{}
		if parts[3] != "" {
			members = strings.Split(parts[3], ",")
		}

		// Also find users who have this group as primary group
		primaryMembers := getUsersWithPrimaryGroup(gid)
		for _, pm := range primaryMembers {
			found := false
			for _, m := range members {
				if m == pm {
					found = true
					break
				}
			}
			if !found {
				members = append(members, pm)
			}
		}

		return &GroupInfo{
			Name:    name,
			GID:     gid,
			Members: members,
		}, nil
	}

	return nil, fmt.Errorf("group not found: %s", groupname)
}

// getUsersWithPrimaryGroup finds users who have the given GID as their primary group
func getUsersWithPrimaryGroup(gid int) []string {
	var users []string

	file, err := os.Open("/etc/passwd")
	if err != nil {
		return users
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}

		userGid, _ := strconv.Atoi(parts[3])
		if userGid == gid {
			users = append(users, parts[0])
		}
	}

	return users
}

// RemoveUserFromGroup removes a user from a group using gpasswd
func RemoveUserFromGroup(groupname, username string) error {
	cmd := exec.Command("gpasswd", "-d", username, groupname)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove user from group: %s - %s", err.Error(), string(output))
	}
	return nil
}

// ModifyUserShell changes a user's shell using chsh
func ModifyUserShell(username, shell string) error {
	cmd := exec.Command("chsh", "-s", shell, username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to change shell: %s - %s", err.Error(), string(output))
	}
	return nil
}

// ModifyUserHome changes a user's home directory using usermod
func ModifyUserHome(username, home string) error {
	cmd := exec.Command("usermod", "-d", home, username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to change home directory: %s - %s", err.Error(), string(output))
	}
	return nil
}
