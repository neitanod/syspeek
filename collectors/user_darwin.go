//go:build darwin

package collectors

import (
	"bufio"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
)

type UserInfo struct {
	Username        string        `json:"username"`
	UID             int           `json:"uid"`
	GID             int           `json:"gid"`
	Gecos           string        `json:"gecos,omitempty"`
	HomeDir         string        `json:"homeDir"`
	Shell           string        `json:"shell"`
	Groups          []string      `json:"groups,omitempty"`
	LastLogin       string        `json:"lastLogin,omitempty"`
	CurrentSessions int           `json:"currentSessions"`
	ProcessCount    int           `json:"processCount"`
	RunningProcs    []ProcessInfo `json:"runningProcs,omitempty"`
	Crontab         string        `json:"crontab,omitempty"`
	CrontabError    string        `json:"crontabError,omitempty"`
}

func GetUserInfo(usernameOrUID string) (*UserInfo, error) {
	var u *user.User
	var err error

	// Try as UID first
	if _, parseErr := strconv.Atoi(usernameOrUID); parseErr == nil {
		u, err = user.LookupId(usernameOrUID)
	}

	if u == nil {
		u, err = user.Lookup(usernameOrUID)
	}

	if err != nil {
		return nil, err
	}

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	info := &UserInfo{
		Username: u.Username,
		UID:      uid,
		GID:      gid,
		Gecos:    u.Name,
		HomeDir:  u.HomeDir,
	}

	// Get shell from /etc/passwd
	if file, err := os.Open("/etc/passwd"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, u.Username+":") {
				parts := strings.Split(line, ":")
				if len(parts) >= 7 {
					info.Shell = parts[6]
				}
				break
			}
		}
	}

	// Get groups
	if gids, err := u.GroupIds(); err == nil {
		for _, gid := range gids {
			if g, err := user.LookupGroupId(gid); err == nil {
				info.Groups = append(info.Groups, g.Name)
			}
		}
	}

	// Get last login
	if out, err := exec.Command("last", "-1", u.Username).Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], u.Username) {
			fields := strings.Fields(lines[0])
			if len(fields) >= 5 {
				info.LastLogin = strings.Join(fields[3:7], " ")
			}
		}
	}

	// Count sessions
	if out, err := exec.Command("who").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, u.Username+" ") {
				info.CurrentSessions++
			}
		}
	}

	// Get processes
	procs, _ := GetProcessesByUser(u.Username)
	info.ProcessCount = len(procs)
	if len(procs) > 20 {
		info.RunningProcs = procs[:20]
	} else {
		info.RunningProcs = procs
	}

	// Get crontab
	info.Crontab, info.CrontabError = getUserCrontab(u.Username)

	return info, nil
}

func getUserCrontab(username string) (string, string) {
	cmd := exec.Command("crontab", "-l", "-u", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		if strings.Contains(outputStr, "no crontab") {
			return "", ""
		}
		return "", outputStr
	}
	return strings.TrimSpace(string(output)), ""
}
