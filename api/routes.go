package api

import (
	"net/http"
	"strings"

	"syspeek/auth"
)

func (a *API) SetupRoutes(mux *http.ServeMux, authMgr *auth.AuthManager) {
	// API endpoints
	mux.HandleFunc("/api/cpu", a.HandleCPU)
	mux.HandleFunc("/api/memory", a.HandleMemory)
	mux.HandleFunc("/api/disk", a.HandleDisk)
	mux.HandleFunc("/api/network", a.HandleNetwork)
	mux.HandleFunc("/api/gpu", a.HandleGPU)
	mux.HandleFunc("/api/processes", a.HandleProcesses)
	mux.HandleFunc("/api/sockets", a.HandleSockets)
	mux.HandleFunc("/api/firewall", a.HandleFirewall)
	mux.HandleFunc("/api/config", a.HandleConfig)

	// SSE stream
	mux.HandleFunc("/api/stream", a.HandleSSE)

	// Auth endpoints
	mux.HandleFunc("/api/auth/login", a.HandleLogin)
	mux.HandleFunc("/api/auth/logout", a.HandleLogout)
	mux.HandleFunc("/api/auth/status", a.HandleAuthStatus)

	// Process endpoints with dynamic PID - use a wrapper to handle routing
	mux.HandleFunc("/api/process/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path pattern
		if strings.HasSuffix(path, "/kill") {
			authMgr.Middleware(a.HandleProcessKill, true)(w, r)
		} else if strings.HasSuffix(path, "/renice") {
			authMgr.Middleware(a.HandleProcessRenice, true)(w, r)
		} else {
			// Process detail
			a.HandleProcessDetail(w, r)
		}
	})

	// IP lookup endpoint
	mux.HandleFunc("/api/ip/", a.HandleIPLookup)

	// User endpoints - lookup and modify
	mux.HandleFunc("/api/user/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/modify") {
			authMgr.Middleware(a.HandleUserModify, true)(w, r)
		} else {
			a.HandleUserLookup(w, r)
		}
	})

	// Group endpoints - lookup and remove user
	mux.HandleFunc("/api/group/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/remove") {
			authMgr.Middleware(a.HandleGroupRemoveUser, true)(w, r)
		} else {
			a.HandleGroupLookup(w, r)
		}
	})

	// Service PID endpoint
	mux.HandleFunc("/api/pid", a.HandleServicePID)

	// Docker endpoints
	mux.HandleFunc("/api/docker", a.HandleDocker)
	mux.HandleFunc("/api/docker/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check if it's an action (start, stop, restart, kill)
		if strings.HasSuffix(path, "/start") ||
			strings.HasSuffix(path, "/stop") ||
			strings.HasSuffix(path, "/restart") ||
			strings.HasSuffix(path, "/kill") {
			authMgr.Middleware(a.HandleDockerAction, true)(w, r)
		} else {
			// Container detail
			a.HandleDockerContainer(w, r)
		}
	})
}
