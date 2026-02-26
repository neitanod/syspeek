package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"syspeek/api"
	"syspeek/auth"
	"syspeek/config"
)

const (
	maxPortRetries = 50
	Version        = "1.1.0"
)

//go:embed static templates
var embeddedFS embed.FS

func main() {
	// Parse flags
	serve := flag.Bool("serve", false, "Run in server mode (don't open browser)")
	configFile := flag.String("config-file", "", "Path to config file")
	printConfig := flag.Bool("print-config-file", false, "Print default config and exit")
	port := flag.Int("port", 0, "Override port from config")
	host := flag.String("host", "", "Override host from config")
	https := flag.Bool("https", false, "Enable HTTPS with auto-generated self-signed certificate")
	certFile := flag.String("cert", "", "Path to TLS certificate file (requires --key)")
	keyFile := flag.String("key", "", "Path to TLS key file (requires --cert)")
	public := flag.Bool("public", false, "Allow public read-only access without authentication")
	flag.Bool("p", false, "Alias for --public")
	admin := flag.Bool("admin", false, "Allow full admin access without authentication")
	flag.Bool("a", false, "Alias for --admin")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Bool("v", false, "Alias for --version")
	flag.Parse()

	// Handle version flag
	showVersion := *version
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "v" {
			showVersion = true
		}
	})
	if showVersion {
		fmt.Printf("syspeek version %s\n", Version)
		os.Exit(0)
	}

	// Handle aliases
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "p" {
			*public = true
		}
		if f.Name == "a" {
			*admin = true
		}
	})

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

	// Handle HTTPS flags
	useHTTPS := *https || (*certFile != "" && *keyFile != "")
	if *certFile != "" && *keyFile != "" {
		cfg.Server.SSL.Enabled = true
		cfg.Server.SSL.Cert = *certFile
		cfg.Server.SSL.Key = *keyFile
	} else if *https {
		cfg.Server.SSL.Enabled = true
		// Will generate self-signed certificate
	}

	// Setup auth manager
	authMgr := auth.NewAuthManager(
		cfg.Auth.Username, cfg.Auth.Password,
		cfg.Auth.ReadOnlyUsername, cfg.Auth.ReadOnlyPassword,
		*public, *admin,
	)

	// Validate: if no auth configured and no public/admin mode, abort
	if !authMgr.IsEnabled() && !*public && !*admin {
		log.Fatalf("No users configured. Run with -p for public read-only mode or -a for public admin mode.")
	}

	authMgr.StartCleanupRoutine()

	// Setup API
	apiHandler := api.NewAPI(cfg, authMgr, *serve)

	// Store service PID and try to set higher priority
	pid := os.Getpid()
	api.SetServicePID(pid)

	// Try to set higher priority (nice -5) for the service
	// This requires appropriate permissions
	if err := SetProcessPriority(-5); err != nil {
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
	if useHTTPS {
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

	// Print auth status
	if authMgr.IsAdminMode() {
		fmt.Printf("Mode: admin (no authentication required)\n")
	} else if authMgr.IsPublic() {
		if authMgr.HasReadWriteAuth() {
			fmt.Printf("Mode: public read-only (login for read-write)\n")
		} else {
			fmt.Printf("Mode: public read-only (no admin configured)\n")
		}
	} else if authMgr.IsEnabled() {
		fmt.Printf("Mode: login required\n")
	} else {
		fmt.Printf("Mode: no authentication configured\n")
	}

	// Open browser if not in serve mode
	if !*serve {
		fmt.Printf("Opening browser...\n")
		openBrowser(url)
	}

	// Start server using the listener we already have
	if useHTTPS {
		fmt.Printf("Starting HTTPS server on %s:%d\n", cfg.Server.Host, cfg.Server.Port)

		var tlsConfig *tls.Config
		if cfg.Server.SSL.Cert != "" && cfg.Server.SSL.Key != "" {
			// Use provided certificate
			cert, err := tls.LoadX509KeyPair(cfg.Server.SSL.Cert, cfg.Server.SSL.Key)
			if err != nil {
				log.Fatalf("Error loading TLS certificate: %v", err)
			}
			tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		} else {
			// Generate self-signed certificate
			fmt.Println("Generating self-signed certificate...")
			cert, err := generateSelfSignedCert(cfg.Server.Host, displayHost)
			if err != nil {
				log.Fatalf("Error generating certificate: %v", err)
			}
			tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
			fmt.Println("Warning: Using self-signed certificate. Browser will show security warning.")
		}

		tlsListener := tls.NewListener(listener, tlsConfig)
		err = http.Serve(tlsListener, mux)
	} else {
		fmt.Printf("Starting HTTP server on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
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

// generateSelfSignedCert creates a self-signed TLS certificate
func generateSelfSignedCert(host, displayHost string) (tls.Certificate, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %v", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Syspeek"},
			CommonName:   displayHost,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // Valid for 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add hosts to certificate
	hosts := []string{host, displayHost, "localhost", "127.0.0.1"}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %v", err)
	}

	// Encode certificate and key to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to marshal private key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// Create TLS certificate
	return tls.X509KeyPair(certPEM, keyPEM)
}
