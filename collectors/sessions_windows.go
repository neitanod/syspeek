//go:build windows

package collectors

import (
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
	// Use 'query user' command to get active sessions
	script := `query user 2>$null | ForEach-Object {
		$line = $_.Trim()
		if ($line -and -not $line.StartsWith("USERNAME")) {
			$parts = $line -split '\s+'
			if ($parts.Count -ge 4) {
				$user = $parts[0].TrimStart('>')
				$sessionName = $parts[1]
				$id = $parts[2]
				$state = $parts[3]
				$idle = if ($parts.Count -ge 5) { $parts[4] } else { "" }
				$logon = if ($parts.Count -ge 6) { $parts[5..($parts.Count-1)] -join " " } else { "" }
				"$user|$sessionName|$id|$state|$idle|$logon"
			}
		}
	}`

	output, err := runPowerShell(script)
	if err != nil {
		return SessionsInfo{}, err
	}

	var sessions []Session
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, "|")
		if len(fields) < 4 {
			continue
		}

		session := Session{
			User:     fields[0],
			Terminal: fields[1], // Session name (console, rdp-tcp, etc.)
		}

		if len(fields) >= 5 && fields[4] != "" && fields[4] != "." {
			session.Idle = fields[4]
		}

		if len(fields) >= 6 {
			session.Login = fields[5]
		}

		// State is in fields[3] - Active, Disc, etc.
		if fields[3] != "Active" {
			session.Terminal += " (" + fields[3] + ")"
		}

		sessions = append(sessions, session)
	}

	return SessionsInfo{
		Sessions: sessions,
		Total:    len(sessions),
	}, nil
}

func GetUsersList() (UsersListInfo, error) {
	// Use PowerShell to get local users
	script := `Get-LocalUser | ForEach-Object {
		$sid = $_.SID.Value
		$groups = (Get-LocalGroup | Where-Object { (Get-LocalGroupMember $_.Name -ErrorAction SilentlyContinue | Where-Object { $_.SID -eq $sid }) }) | ForEach-Object { $_.Name }
		$groupList = $groups -join ","
		$enabled = $_.Enabled
		$desc = $_.Description -replace '\|', '-'
		$home = if ($_.HomeDirectory) { $_.HomeDirectory } else { "C:\Users\$($_.Name)" }
		"$($_.Name)|$sid|$enabled|$desc|$home|$groupList"
	}`

	output, err := runPowerShell(script)
	if err != nil {
		return UsersListInfo{}, err
	}

	var users []SystemUser
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, "|")
		if len(fields) < 5 {
			continue
		}

		user := SystemUser{
			Username: fields[0],
			Gecos:    fields[3],
			HomeDir:  fields[4],
			Shell:    "cmd.exe",
		}

		// Windows doesn't have numeric UIDs in the same sense
		// We'll use a hash or just set to 0
		user.UID = 0
		user.GID = 0

		// Check if system user (disabled or built-in)
		user.IsSystem = fields[2] == "False" || strings.HasPrefix(fields[0], "Default")

		if len(fields) >= 6 && fields[5] != "" {
			user.Groups = strings.Split(fields[5], ",")
		}

		users = append(users, user)
	}

	// Also try to get domain users if available
	domainScript := `try {
		$domain = [System.DirectoryServices.ActiveDirectory.Domain]::GetCurrentDomain()
		# Domain users would be retrieved here
	} catch {
		# Not domain joined
	}`
	runPowerShell(domainScript)

	return UsersListInfo{
		Users: users,
		Total: len(users),
	}, nil
}

// getUserGroups returns groups for a Windows user
func getUserGroups(username string) []string {
	script := `$groups = @()
	$user = Get-LocalUser -Name '` + username + `' -ErrorAction SilentlyContinue
	if ($user) {
		$sid = $user.SID.Value
		Get-LocalGroup | ForEach-Object {
			$members = Get-LocalGroupMember $_.Name -ErrorAction SilentlyContinue
			if ($members | Where-Object { $_.SID -eq $sid }) {
				$groups += $_.Name
			}
		}
	}
	$groups -join ","`

	output, err := runPowerShell(script)
	if err != nil {
		return nil
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}

	return strings.Split(output, ",")
}
