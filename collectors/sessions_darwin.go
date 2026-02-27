//go:build darwin

package collectors

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Session struct {
	User     string `json:"user"`
	Terminal string `json:"terminal"`
	Host     string `json:"host,omitempty"`
	Login    string `json:"login"`
	Idle     string `json:"idle,omitempty"`
	PID      int    `json:"pid,omitempty"`
}

type SessionsInfo struct {
	Sessions []Session `json:"sessions"`
	Total    int       `json:"total"`
}

type SystemUser struct {
	Username string   `json:"username"`
	UID      int      `json:"uid"`
	GID      int      `json:"gid"`
	Gecos    string   `json:"gecos,omitempty"`
	HomeDir  string   `json:"homeDir"`
	Shell    string   `json:"shell"`
	Groups   []string `json:"groups,omitempty"`
	IsSystem bool     `json:"isSystem"`
}

type UsersListInfo struct {
	Users []SystemUser `json:"users"`
	Total int          `json:"total"`
}

func GetSessions() (SessionsInfo, error) {
	// Use 'who' command to get active sessions
	cmd := exec.Command("who")
	output, err := cmd.Output()
	if err != nil {
		return SessionsInfo{}, err
	}

	var sessions []Session
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		session := Session{
			User:     fields[0],
			Terminal: fields[1],
		}

		// Parse date/time
		if len(fields) >= 4 {
			session.Login = fields[2] + " " + fields[3]
		} else if len(fields) >= 3 {
			session.Login = fields[2]
		}

		// Check for host in parentheses
		for _, field := range fields {
			if strings.HasPrefix(field, "(") && strings.HasSuffix(field, ")") {
				session.Host = strings.Trim(field, "()")
			}
		}

		sessions = append(sessions, session)
	}

	return SessionsInfo{
		Sessions: sessions,
		Total:    len(sessions),
	}, nil
}

func GetUsersList() (UsersListInfo, error) {
	// Read /etc/passwd on macOS
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return UsersListInfo{}, err
	}
	defer file.Close()

	var users []SystemUser
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}

		uid, _ := strconv.Atoi(fields[2])
		gid, _ := strconv.Atoi(fields[3])

		user := SystemUser{
			Username: fields[0],
			UID:      uid,
			GID:      gid,
			Gecos:    fields[4],
			HomeDir:  fields[5],
			Shell:    fields[6],
			IsSystem: uid < 500, // macOS uses 500 as threshold
		}

		// Get groups for this user
		if gids, err := exec.Command("groups", user.Username).Output(); err == nil {
			line := strings.TrimSpace(string(gids))
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				groupsStr := strings.TrimSpace(parts[1])
				if groupsStr != "" {
					user.Groups = strings.Fields(groupsStr)
				}
			}
		}

		users = append(users, user)
	}

	return UsersListInfo{
		Users: users,
		Total: len(users),
	}, nil
}
