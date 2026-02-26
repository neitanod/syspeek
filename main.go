package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"syspeek/api"
	"syspeek/auth"
	"syspeek/config"
)

const maxPortRetries = 50

//go:embed static templates
var embeddedFS embed.FS

func main() {
	// Parse flags
	serve := flag.Bool("serve", false, "Run in server mode (don't open browser)")
	configFile := flag.String("config-file", "", "Path to config file")
	printConfig := flag.Bool("print-config-file", false, "Print default config and exit")
	port := flag.Int("port", 0, "Override port from config")
	host := flag.String("host", "", "Override host from config")
	flag.Parse()

	// Handle --print-config-file
	if *printConfig {
		cfg := config.DefaultConfig()
		jsonStr, err := cfg.ToJSON()
		if err != nil {
			log.Fatalf("Error generating config: %v", err)
		}
		fmt.Println(jsonStr)
		os.Exit(0)
	}

	// Determine config file path
	cfgPath := *configFile
	if cfgPath == "" {
		// Try default location: ~/.config/syspeek/config.json
		homeDir, err := os.UserHomeDir()
		if err == nil {
			defaultPath := homeDir + "/.config/syspeek/config.json"
			if _, err := os.Stat(defaultPath); err == nil {
				cfgPath = defaultPath
			}
		}
	}

	// Load config
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Override with flags
	if *port != 0 {
		cfg.Server.Port = *port
	}
	if *host != "" {
		cfg.Server.Host = *host
	}

	// In non-serve mode, bind to localhost only
	if !*serve && cfg.Server.Host == "0.0.0.0" {
		cfg.Server.Host = "127.0.0.1"
	}

	// Setup auth manager
	authMgr := auth.NewAuthManager(cfg.Auth.Username, cfg.Auth.Password)
	authMgr.StartCleanupRoutine()

	// Setup API
	apiHandler := api.NewAPI(cfg, authMgr)

	// Store service PID and try to set higher priority
	pid := os.Getpid()
	api.SetServicePID(pid)

	// Try to set higher priority (nice -5) for the service
	// This requires appropriate permissions
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, -5); err != nil {
		// Not critical if it fails - just log it
		log.Printf("Note: Could not set service priority (requires elevated permissions)")
	}

	// Setup routes
	mux := http.NewServeMux()
	apiHandler.SetupRoutes(mux, authMgr)

	// Serve static files
	staticFS, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		log.Fatalf("Error getting static fs: %v", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Serve index.html for root and SPA routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only serve index for root path or non-API paths
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/static/") {
			// For SPA routing, serve index.html
			serveIndex(w, r, cfg)
			return
		}
		if r.URL.Path == "/" {
			serveIndex(w, r, cfg)
		}
	})

	// Build URL helper
	scheme := "http"
	if cfg.Server.SSL.Enabled {
		scheme = "https"
	}

	displayHost := cfg.Server.Host
	if displayHost == "0.0.0.0" {
		displayHost = "localhost"
	}

	// Find available port (try up to maxPortRetries times)
	portSpecified := *port != 0
	startPort := cfg.Server.Port
	var listener net.Listener

	for i := 0; i < maxPortRetries; i++ {
		tryPort := startPort + i
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, tryPort)

		listener, err = net.Listen("tcp", addr)
		if err == nil {
			cfg.Server.Port = tryPort
			if i > 0 && !portSpecified {
				fmt.Printf("Port %d busy, using %d\n", startPort, tryPort)
			}
			break
		}

		// If user specified a port explicitly, don't try others
		if portSpecified {
			log.Fatalf("Port %d is already in use", startPort)
		}
	}

	if listener == nil {
		log.Fatalf("Could not find available port after trying %d ports (from %d to %d)",
			maxPortRetries, startPort, startPort+maxPortRetries-1)
	}

	url := fmt.Sprintf("%s://%s:%d", scheme, displayHost, cfg.Server.Port)

	// Print startup info
	fmt.Printf("Syspeek starting...\n")
	fmt.Printf("URL: %s\n", url)

	if authMgr.IsEnabled() {
		fmt.Printf("Authentication: enabled\n")
	} else {
		fmt.Printf("Authentication: disabled (read-only mode)\n")
	}

	// Open browser if not in serve mode
	if !*serve {
		fmt.Printf("Opening browser...\n")
		openBrowser(url)
	}

	// Start server using the listener we already have
	fmt.Printf("Starting HTTP server on %s:%d\n", cfg.Server.Host, cfg.Server.Port)

	if cfg.Server.SSL.Enabled {
		// For TLS, we need to wrap the listener
		listener.Close() // Close the TCP listener
		addr := cfg.GetAddress()
		err = http.ListenAndServeTLS(addr, cfg.Server.SSL.Cert, cfg.Server.SSL.Key, mux)
	} else {
		err = http.Serve(listener, mux)
	}

	if err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	// Read the template
	tmpl, err := embeddedFS.ReadFile("templates/index.html")
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(tmpl)
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		fmt.Printf("Could not open browser: %v\n", err)
		fmt.Printf("Please open %s manually\n", url)
	}
}
