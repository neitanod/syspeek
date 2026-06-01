//go:build windows

package collectors

import (
	"strings"
	"sync"
	"time"

	gpshost "github.com/shirou/gopsutil/v3/host"
)

var (
	usersListCache    UsersListInfo
	usersListCachedAt time.Time
	usersListMu       sync.Mutex
	usersListTTL      = 30 * time.Second
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
	info := SessionsInfo{Sessions: []Session{}}

	users, err := gpshost.Users()
	if err != nil {
		// gopsutil/host can fail on stripped systems; treat as no sessions
		return info, nil
	}

	for _, u := range users {
		session := Session{
			User:     u.User,
			Terminal: u.Terminal,
			Host:     u.Host,
		}
		if u.Started > 0 {
			session.Login = time.Unix(int64(u.Started), 0).Format("2006-01-02 15:04")
		}
		info.Sessions = append(info.Sessions, session)
	}

	info.Total = len(info.Sessions)
	return info, nil
}

func GetUsersList() (UsersListInfo, error) {
	usersListMu.Lock()
	if !usersListCachedAt.IsZero() && time.Since(usersListCachedAt) < usersListTTL {
		cached := usersListCache
		usersListMu.Unlock()
		return cached, nil
	}
	usersListMu.Unlock()

	info := UsersListInfo{Users: []SystemUser{}}

	// Single PowerShell invocation with Win32_UserAccount, much cheaper than
	// iterating Get-LocalGroupMember per user.
	script := `Get-CimInstance Win32_UserAccount -Filter "LocalAccount=True" | ForEach-Object {
		$desc = if ($_.Description) { $_.Description -replace '\|', '-' } else { "" }
		$disabled = $_.Disabled
		"$($_.Name)|$($_.SID)|$disabled|$desc|C:\Users\$($_.Name)"
	}`

	output, err := runPowerShell(script)
	if err != nil {
		// Fall back to an empty list rather than failing the endpoint
		return info, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 5 {
			continue
		}
		u := SystemUser{
			Username: fields[0],
			Gecos:    fields[3],
			HomeDir:  fields[4],
			Shell:    "cmd.exe",
			IsSystem: strings.EqualFold(fields[2], "True"),
		}
		info.Users = append(info.Users, u)
	}

	info.Total = len(info.Users)

	usersListMu.Lock()
	usersListCache = info
	usersListCachedAt = time.Now()
	usersListMu.Unlock()

	return info, nil
}

// getUserGroups returns groups for a Windows user. Used by user_windows.go.
func getUserGroups(username string) []string {
	script := `$user = Get-LocalUser -Name '` + username + `' -ErrorAction SilentlyContinue
		if ($user) {
			$sid = $user.SID.Value
			Get-LocalGroup | ForEach-Object {
				$members = Get-LocalGroupMember $_.Name -ErrorAction SilentlyContinue
				if ($members | Where-Object { $_.SID -eq $sid }) {
					$_.Name
				}
			}
		}`

	output, err := runPowerShell(script)
	if err != nil {
		return nil
	}

	var groups []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if g := strings.TrimSpace(line); g != "" {
			groups = append(groups, g)
		}
	}
	return groups
}
