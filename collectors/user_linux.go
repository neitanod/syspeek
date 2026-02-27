//go:build linux

package collectors

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type UserProcess struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

type UserInfo struct {
	Username        string        `json:"username"`
	UID             int           `json:"uid"`
	GID             int           `json:"gid"`
	Gecos           string        `json:"gecos,omitempty"` // Full name, etc.
	HomeDir         string        `json:"homeDir"`
	Shell           string        `json:"shell"`
	Groups          []string      `json:"groups,omitempty"`
	LastLogin       string        `json:"lastLogin,omitempty"`
	CurrentSessions int           `json:"currentSessions"`
	ProcessCount    int           `json:"processCount"`
	RunningProcs    []UserProcess `json:"runningProcs,omitempty"` // PIDs with names
	Crontab         string        `json:"crontab,omitempty"`      // User's crontab content
	CrontabError    string        `json:"crontabError,omitempty"` // Error if couldn't read crontab
}

func GetUserInfo(username string) (*UserInfo, error) {
	// First try to parse username as UID
	if uid, err := strconv.Atoi(username); err == nil {
		return getUserInfoByUID(uid)
	}
	return getUserInfoByName(username)
}

func getUserInfoByName(username string) (*UserInfo, error) {
	// Read /etc/passwd
	file, err := os.Open("/etc/passwd")
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

		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}

		if fields[0] == username {
			return parsePasswdLine(fields)
		}
	}

	return nil, fmt.Errorf("user not found: %s", username)
}

func getUserInfoByUID(uid int) (*UserInfo, error) {
	// Read /etc/passwd
	file, err := os.Open("/etc/passwd")
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

		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}

		parsedUID, _ := strconv.Atoi(fields[2])
		if parsedUID == uid {
			return parsePasswdLine(fields)
		}
	}

	return nil, fmt.Errorf("user not found: UID %d", uid)
}

func parsePasswdLine(fields []string) (*UserInfo, error) {
	uid, _ := strconv.Atoi(fields[2])
	gid, _ := strconv.Atoi(fields[3])

	info := &UserInfo{
		Username: fields[0],
		UID:      uid,
		GID:      gid,
		Gecos:    fields[4],
		HomeDir:  fields[5],
		Shell:    fields[6],
	}

	// Get groups
	info.Groups = getUserGroups(info.Username)

	// Get last login
	info.LastLogin = getLastLogin(info.Username)

	// Get current sessions
	info.CurrentSessions = countCurrentSessions(info.Username)

	// Get process count and PIDs
	info.ProcessCount, info.RunningProcs = getUserProcesses(info.Username)

	// Get crontab
	info.Crontab, info.CrontabError = getUserCrontab(info.Username)

	return info, nil
}

func getUserGroups(username string) []string {
	cmd := exec.Command("groups", username)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Output format: "username : group1 group2 group3"
	line := strings.TrimSpace(string(output))
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return nil
	}

	groupsStr := strings.TrimSpace(parts[1])
	if groupsStr == "" {
		return nil
	}

	return strings.Fields(groupsStr)
}

func getLastLogin(username string) string {
	cmd := exec.Command("lastlog", "-u", username)
	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return "Unknown"
	}

	// Skip header, get the actual login info
	line := strings.TrimSpace(lines[1])
	if line == "" {
		return "Never"
	}

	// Parse the lastlog output
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "Unknown"
	}

	// Check if "Never logged in"
	if strings.Contains(line, "Never logged in") || strings.Contains(line, "**Never logged in**") {
		return "Never"
	}

	// Try to extract the date portion (skip username and terminal columns)
	if len(fields) >= 4 {
		// Format typically: username terminal host date...
		// Skip first 3 fields (username, terminal, host/localhost) and join the rest
		dateFields := fields[3:]
		return strings.Join(dateFields, " ")
	}

	return line
}

func countCurrentSessions(username string) int {
	cmd := exec.Command("who")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	count := 0
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == username {
			count++
		}
	}

	return count
}

func getUserProcesses(username string) (int, []UserProcess) {
	procs, err := GetProcessList()
	if err != nil {
		return 0, nil
	}

	var userProcs []UserProcess
	for _, p := range procs.Processes {
		if p.User == username {
			userProcs = append(userProcs, UserProcess{
				PID:  p.PID,
				Name: p.Name,
			})
		}
	}

	// Limit returned processes to first 50
	if len(userProcs) > 50 {
		return len(userProcs), userProcs[:50]
	}

	return len(userProcs), userProcs
}

func getUserCrontab(username string) (string, string) {
	// Try to read user's crontab using crontab -l -u username
	// This requires root privileges or being the user
	cmd := exec.Command("crontab", "-l", "-u", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		// Check if it's "no crontab for user"
		if strings.Contains(outputStr, "no crontab") {
			return "", "" // No crontab, not an error
		}
		// Permission denied or other error
		return "", outputStr
	}

	return strings.TrimSpace(string(output)), ""
}
