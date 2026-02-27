//go:build windows

package collectors

import (
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
	Crontab         string        `json:"crontab,omitempty"`      // Scheduled tasks for user
	CrontabError    string        `json:"crontabError,omitempty"`
}

func GetUserInfo(usernameOrUID string) (*UserInfo, error) {
	var u *user.User
	var err error

	// Try as SID first
	u, err = user.LookupId(usernameOrUID)
	if err != nil {
		u, err = user.Lookup(usernameOrUID)
	}

	if err != nil {
		return nil, err
	}

	info := &UserInfo{
		Username: u.Username,
		Gecos:    u.Name,
		HomeDir:  u.HomeDir,
		Shell:    "cmd.exe",
	}

	// Try to get numeric UID (Windows uses SIDs)
	info.UID, _ = strconv.Atoi(u.Uid)
	info.GID, _ = strconv.Atoi(u.Gid)

	// Get groups
	if gids, err := u.GroupIds(); err == nil {
		for _, gid := range gids {
			if g, err := user.LookupGroupId(gid); err == nil {
				info.Groups = append(info.Groups, g.Name)
			}
		}
	}

	// Get last login using net user command
	parts := strings.Split(u.Username, "\\")
	username := parts[len(parts)-1]

	if out, err := exec.Command("net", "user", username).Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Last logon") {
				info.LastLogin = strings.TrimSpace(strings.TrimPrefix(line, "Last logon"))
				break
			}
		}
	}

	// Count current sessions
	if out, err := exec.Command("query", "user").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, username) {
				info.CurrentSessions++
			}
		}
	}

	// Get processes (simplified)
	procs, _ := GetProcessesByUser(username)
	info.ProcessCount = len(procs)
	if len(procs) > 20 {
		info.RunningProcs = procs[:20]
	} else {
		info.RunningProcs = procs
	}

	// Get scheduled tasks for user
	info.Crontab, info.CrontabError = getUserScheduledTasks(username)

	return info, nil
}

func getUserScheduledTasks(username string) (string, string) {
	// Get scheduled tasks for the user using schtasks
	script := `Get-ScheduledTask | Where-Object { $_.Principal.UserId -like '*` + username + `*' } | ForEach-Object {
		$info = Get-ScheduledTaskInfo $_.TaskName -ErrorAction SilentlyContinue
		$triggers = ($_.Triggers | ForEach-Object { $_.CimClass.CimClassName }) -join ", "
		"$($_.TaskName) | $($_.State) | $triggers | $($_.Actions.Execute)"
	}`

	output, err := runPowerShell(script)
	if err != nil {
		return "", "Could not retrieve scheduled tasks"
	}

	if strings.TrimSpace(output) == "" {
		return "", "" // No scheduled tasks
	}

	return output, ""
}
