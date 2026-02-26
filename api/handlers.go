package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"syscall"

	"syspeek/auth"
	"syspeek/collectors"
	"syspeek/config"
)

type API struct {
	config *config.Config
	auth   *auth.AuthManager
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type StatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	AuthEnabled   bool   `json:"authEnabled"`
	Username      string `json:"username,omitempty"`
}

type ActionRequest struct {
	Signal   int `json:"signal,omitempty"`
	Priority int `json:"priority,omitempty"`
}

type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func NewAPI(cfg *config.Config, authMgr *auth.AuthManager) *API {
	return &API{
		config: cfg,
		auth:   authMgr,
	}
}

func (a *API) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LoginResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	token, ok := a.auth.Login(req.Username, req.Password)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Invalid credentials",
		})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400, // 24 hours
	})

	writeJSON(w, http.StatusOK, LoginResponse{
		Success: true,
	})
}

func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie("session"); err == nil {
		a.auth.Logout(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	writeJSON(w, http.StatusOK, LoginResponse{
		Success: true,
	})
}

func (a *API) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	status := StatusResponse{
		AuthEnabled: a.auth.IsEnabled(),
	}

	if cookie, err := r.Cookie("session"); err == nil {
		if session := a.auth.GetSession(cookie.Value); session != nil {
			status.Authenticated = true
			status.Username = session.Username
		}
	}

	writeJSON(w, http.StatusOK, status)
}

func (a *API) HandleCPU(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetCPUInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleMemory(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetMemoryInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleDisk(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetDiskInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleNetwork(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetNetworkInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleGPU(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetGPUInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleProcesses(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetProcessList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleProcessDetail(w http.ResponseWriter, r *http.Request) {
	pidStr := r.URL.Query().Get("pid")
	if pidStr == "" {
		// Try to get from path
		// Expected path: /api/process/123
		pidStr = extractPID(r.URL.Path)
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		http.Error(w, "Invalid PID", http.StatusBadRequest)
		return
	}

	info, err := collectors.GetProcessDetail(pid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleProcessKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if r.Header.Get("X-Authenticated") != "true" {
		writeJSON(w, http.StatusUnauthorized, ActionResponse{
			Success: false,
			Message: "Authentication required",
		})
		return
	}

	pidStr := extractPID(r.URL.Path)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Invalid PID",
		})
		return
	}

	// Prevent killing the service itself
	if pid == servicePID {
		writeJSON(w, http.StatusForbidden, ActionResponse{
			Success: false,
			Message: "Cannot send signals to the Syspeek service itself",
		})
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Signal = int(syscall.SIGTERM) // Default to SIGTERM
	}

	signal := syscall.Signal(req.Signal)
	if req.Signal == 0 {
		signal = syscall.SIGTERM
	}

	if err := collectors.KillProcess(pid, signal); err != nil {
		writeJSON(w, http.StatusInternalServerError, ActionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: "Signal sent",
	})
}

func (a *API) HandleProcessRenice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if r.Header.Get("X-Authenticated") != "true" {
		writeJSON(w, http.StatusUnauthorized, ActionResponse{
			Success: false,
			Message: "Authentication required",
		})
		return
	}

	pidStr := extractPID(r.URL.Path)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Invalid PID",
		})
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	if err := collectors.ReniceProcess(pid, req.Priority); err != nil {
		writeJSON(w, http.StatusInternalServerError, ActionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: "Priority changed",
	})
}

func (a *API) HandleSockets(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetSocketInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleFirewall(w http.ResponseWriter, r *http.Request) {
	info, err := collectors.GetFirewallInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleConfig(w http.ResponseWriter, r *http.Request) {
	// Return UI-relevant config (without sensitive data)
	uiConfig := struct {
		UI          config.UIConfig      `json:"ui"`
		Refresh     config.RefreshConfig `json:"refresh"`
		AuthEnabled bool                 `json:"authEnabled"`
	}{
		UI:          a.config.UI,
		Refresh:     a.config.Refresh,
		AuthEnabled: a.auth.IsEnabled(),
	}
	writeJSON(w, http.StatusOK, uiConfig)
}

func (a *API) HandleIPLookup(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		// Try to get from path: /api/ip/1.2.3.4 or /api/ip/2001:db8::1
		// For IPv6, the address contains colons, so we take everything after /api/ip/
		pathPart := strings.TrimPrefix(r.URL.Path, "/api/ip/")
		// Remove trailing slashes
		pathPart = strings.TrimSuffix(pathPart, "/")
		if pathPart != "" {
			ip = pathPart
		}
	}

	if ip == "" {
		http.Error(w, "IP address required", http.StatusBadRequest)
		return
	}

	// Clean up IPv6 addresses (remove brackets if present)
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")

	info, err := collectors.GetIPInfo(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleUserLookup(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	if username == "" {
		// Try to get from path: /api/user/sebas or /api/user/1000
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/user/"), "/")
		if len(parts) > 0 {
			username = parts[0]
		}
	}

	if username == "" {
		http.Error(w, "Username or UID required", http.StatusBadRequest)
		return
	}

	info, err := collectors.GetUserInfo(username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// extractPID extracts PID from paths like /api/process/123 or /api/process/123/kill
func extractPID(path string) string {
	// Remove trailing slash
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")
	// Find "process" and get the next part
	for i, part := range parts {
		if part == "process" && i+1 < len(parts) {
			// Return the PID part (which might be followed by /kill or /renice)
			pidPart := parts[i+1]
			// If it's a number, return it
			if _, err := strconv.Atoi(pidPart); err == nil {
				return pidPart
			}
		}
	}
	return ""
}

// Group handlers
func (a *API) HandleGroupLookup(w http.ResponseWriter, r *http.Request) {
	groupname := r.URL.Query().Get("name")
	if groupname == "" {
		// Try to get from path: /api/group/groupname
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/group/"), "/")
		if len(parts) > 0 {
			groupname = parts[0]
		}
	}

	if groupname == "" {
		http.Error(w, "Group name required", http.StatusBadRequest)
		return
	}

	info, err := collectors.GetGroupInfo(groupname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type RemoveFromGroupRequest struct {
	Username string `json:"username"`
}

func (a *API) HandleGroupRemoveUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if r.Header.Get("X-Authenticated") != "true" {
		writeJSON(w, http.StatusUnauthorized, ActionResponse{
			Success: false,
			Message: "Authentication required",
		})
		return
	}

	// Extract group name from path: /api/group/groupname/remove
	path := strings.TrimPrefix(r.URL.Path, "/api/group/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Group name required",
		})
		return
	}
	groupname := parts[0]

	var req RemoveFromGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Username required",
		})
		return
	}

	if err := collectors.RemoveUserFromGroup(groupname, req.Username); err != nil {
		writeJSON(w, http.StatusInternalServerError, ActionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: "User removed from group",
	})
}

type ModifyUserRequest struct {
	Shell string `json:"shell,omitempty"`
	Home  string `json:"home,omitempty"`
}

func (a *API) HandleUserModify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if r.Header.Get("X-Authenticated") != "true" {
		writeJSON(w, http.StatusUnauthorized, ActionResponse{
			Success: false,
			Message: "Authentication required",
		})
		return
	}

	// Extract username from path: /api/user/username/modify
	path := strings.TrimPrefix(r.URL.Path, "/api/user/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Username required",
		})
		return
	}
	username := parts[0]

	var req ModifyUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	if req.Shell != "" {
		if err := collectors.ModifyUserShell(username, req.Shell); err != nil {
			writeJSON(w, http.StatusInternalServerError, ActionResponse{
				Success: false,
				Message: err.Error(),
			})
			return
		}
	}

	if req.Home != "" {
		if err := collectors.ModifyUserHome(username, req.Home); err != nil {
			writeJSON(w, http.StatusInternalServerError, ActionResponse{
				Success: false,
				Message: err.Error(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: "User modified",
	})
}

// GetServicePID returns the current process PID
func (a *API) HandleServicePID(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int{"pid": servicePID})
}

// servicePID is stored at package level
var servicePID = 0

func SetServicePID(pid int) {
	servicePID = pid
}

func GetServicePID() int {
	return servicePID
}

// Docker handlers
func (a *API) HandleDocker(w http.ResponseWriter, r *http.Request) {
	info := collectors.GetDockerInfo()
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleDockerContainer(w http.ResponseWriter, r *http.Request) {
	// Extract container ID from path: /api/docker/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/docker/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Container ID required",
		})
		return
	}

	containerID := parts[0]

	container, err := collectors.GetContainerDetail(containerID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ActionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, container)
}

func (a *API) HandleDockerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authentication is handled by middleware in routes.go

	// Extract container ID and action from path: /api/docker/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/docker/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Container ID and action required",
		})
		return
	}

	containerID := parts[0]
	action := parts[1]

	// Validate action
	validActions := map[string]bool{"start": true, "stop": true, "restart": true, "kill": true}
	if !validActions[action] {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "Invalid action. Valid actions: start, stop, restart, kill",
		})
		return
	}

	err := collectors.DockerAction(containerID, action)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ActionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: "Container " + action + " successful",
	})
}
