package api

import (
	"net/http"
	"strings"

	"syspeek/auth"
)

func (a *API) SetupRoutes(mux *http.ServeMux, authMgr *auth.AuthManager) {
	// API endpoints - read-only, but may require login depending on mode
	mux.HandleFunc("/api/cpu", authMgr.Middleware(a.HandleCPU, false))
	mux.HandleFunc("/api/memory", authMgr.Middleware(a.HandleMemory, false))
	mux.HandleFunc("/api/disk", authMgr.Middleware(a.HandleDisk, false))
	mux.HandleFunc("/api/network", authMgr.Middleware(a.HandleNetwork, false))
	mux.HandleFunc("/api/gpu", authMgr.Middleware(a.HandleGPU, false))
	mux.HandleFunc("/api/processes", authMgr.Middleware(a.HandleProcesses, false))
	mux.HandleFunc("/api/sockets", authMgr.Middleware(a.HandleSockets, false))
	mux.HandleFunc("/api/firewall", authMgr.Middleware(a.HandleFirewall, false))
	mux.HandleFunc("/api/config", authMgr.Middleware(a.HandleConfig, false))

	// SSE stream - read-only but may require login
	mux.HandleFunc("/api/stream", authMgr.Middleware(a.HandleSSE, false))

	// Auth endpoints - always accessible (for login flow)
	mux.HandleFunc("/api/auth/login", a.HandleLogin)
	mux.HandleFunc("/api/auth/logout", a.HandleLogout)
	mux.HandleFunc("/api/auth/status", a.HandleAuthStatus)

	// Close endpoint - for desktop mode (ignored in serve mode)
	mux.HandleFunc("/api/close", a.HandleClose)

	// Process endpoints with dynamic PID
	mux.HandleFunc("/api/process/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path pattern
		if strings.HasSuffix(path, "/kill") {
			// Requires read-write access
			authMgr.MiddlewareReadWrite(a.HandleProcessKill)(w, r)
		} else if strings.HasSuffix(path, "/renice") {
			// Requires read-write access
			authMgr.MiddlewareReadWrite(a.HandleProcessRenice)(w, r)
		} else {
			// Process detail - read-only
			authMgr.Middleware(a.HandleProcessDetail, false)(w, r)
		}
	})

	// IP lookup endpoint - read-only
	mux.HandleFunc("/api/ip/", authMgr.Middleware(a.HandleIPLookup, false))

	// User endpoints - lookup and modify
	mux.HandleFunc("/api/user/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/modify") {
			// Requires read-write access
			authMgr.MiddlewareReadWrite(a.HandleUserModify)(w, r)
		} else {
			// User lookup - read-only
			authMgr.Middleware(a.HandleUserLookup, false)(w, r)
		}
	})

	// Group endpoints - lookup and remove user
	mux.HandleFunc("/api/group/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/remove") {
			// Requires read-write access
			authMgr.MiddlewareReadWrite(a.HandleGroupRemoveUser)(w, r)
		} else {
			// Group lookup - read-only
			authMgr.Middleware(a.HandleGroupLookup, false)(w, r)
		}
	})

	// Service PID endpoint - read-only
	mux.HandleFunc("/api/pid", authMgr.Middleware(a.HandleServicePID, false))

	// Docker endpoints
	mux.HandleFunc("/api/docker", authMgr.Middleware(a.HandleDocker, false))
	mux.HandleFunc("/api/docker/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check if it's an action (start, stop, restart, kill, pause, unpause)
		if strings.HasSuffix(path, "/start") ||
			strings.HasSuffix(path, "/stop") ||
			strings.HasSuffix(path, "/restart") ||
			strings.HasSuffix(path, "/kill") ||
			strings.HasSuffix(path, "/pause") ||
			strings.HasSuffix(path, "/unpause") {
			// Requires read-write access
			authMgr.MiddlewareReadWrite(a.HandleDockerAction)(w, r)
		} else if strings.HasSuffix(path, "/logs") {
			// Logs - read-only
			authMgr.Middleware(a.HandleDockerLogs, false)(w, r)
		} else if strings.HasSuffix(path, "/top") {
			// Top - read-only
			authMgr.Middleware(a.HandleDockerTop, false)(w, r)
		} else if strings.HasSuffix(path, "/inspect") {
			// Inspect - read-only
			authMgr.Middleware(a.HandleDockerInspect, false)(w, r)
		} else {
			// Container detail - read-only
			authMgr.Middleware(a.HandleDockerContainer, false)(w, r)
		}
	})

	// Services endpoints
	mux.HandleFunc("/api/services", authMgr.Middleware(a.HandleServices, false))
	mux.HandleFunc("/api/service/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check if it's an action (start, stop, restart, enable, disable)
		if strings.HasSuffix(path, "/start") ||
			strings.HasSuffix(path, "/stop") ||
			strings.HasSuffix(path, "/restart") ||
			strings.HasSuffix(path, "/enable") ||
			strings.HasSuffix(path, "/disable") {
			// Requires read-write access
			authMgr.MiddlewareReadWrite(a.HandleServiceAction)(w, r)
		} else if strings.HasSuffix(path, "/logs") {
			// Logs - read-only
			authMgr.Middleware(a.HandleServiceLogs, false)(w, r)
		} else {
			// Service detail - read-only
			authMgr.Middleware(a.HandleServiceDetail, false)(w, r)
		}
	})

	// Sessions endpoint - read-only
	mux.HandleFunc("/api/sessions", authMgr.Middleware(a.HandleSessions, false))

	// Users list endpoint - read-only
	mux.HandleFunc("/api/users", authMgr.Middleware(a.HandleUsersList, false))
}
