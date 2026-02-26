package collectors

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type IPInfo struct {
	IP            string   `json:"ip"`
	Hostname      string   `json:"hostname,omitempty"`
	IsPrivate     bool     `json:"isPrivate"`
	IsLoopback    bool     `json:"isLoopback"`
	Version       string   `json:"version"` // "IPv4" or "IPv6"
	Whois         string   `json:"whois,omitempty"`
	ReverseDNS    []string `json:"reverseDns,omitempty"`
	GeoIP         *GeoInfo `json:"geoip,omitempty"`
	RelatedProcs  []int    `json:"relatedProcs,omitempty"`  // PIDs using this IP
}

type GeoInfo struct {
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"countryCode,omitempty"`
	Region      string  `json:"region,omitempty"`
	City        string  `json:"city,omitempty"`
	Org         string  `json:"org,omitempty"`
	ASN         string  `json:"asn,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
}

func GetIPInfo(ipStr string) (*IPInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	info := &IPInfo{
		IP:         ipStr,
		IsPrivate:  ip.IsPrivate(),
		IsLoopback: ip.IsLoopback(),
	}

	// Determine version
	if ip.To4() != nil {
		info.Version = "IPv4"
	} else {
		info.Version = "IPv6"
	}

	// Reverse DNS lookup
	names, err := net.LookupAddr(ipStr)
	if err == nil && len(names) > 0 {
		info.ReverseDNS = names
		info.Hostname = strings.TrimSuffix(names[0], ".")
	}

	// For public IPs, get more info
	if !info.IsPrivate && !info.IsLoopback {
		// Get whois info (run in background, with timeout)
		info.Whois = getWhoisInfo(ipStr)

		// Get GeoIP info from ip-api.com (free, no API key needed)
		info.GeoIP = getGeoIPInfo(ipStr)
	}

	// Find processes using this IP
	info.RelatedProcs = findProcessesUsingIP(ipStr)

	return info, nil
}

func getWhoisInfo(ip string) string {
	cmd := exec.Command("timeout", "5", "whois", ip)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse and simplify whois output - extract key fields
	lines := strings.Split(string(output), "\n")
	var relevantLines []string
	keywords := []string{"OrgName", "Organization", "org-name", "NetName", "netname",
		"Country", "country", "descr", "abuse", "Address", "address", "inet6num", "route6"}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "%") {
			continue
		}
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(line), strings.ToLower(kw)) {
				relevantLines = append(relevantLines, line)
				break
			}
		}
	}

	if len(relevantLines) > 20 {
		relevantLines = relevantLines[:20]
	}

	return strings.Join(relevantLines, "\n")
}

func getGeoIPInfo(ip string) *GeoInfo {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,region,city,lat,lon,org,as", ip))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Status      string  `json:"status"`
		Country     string  `json:"country"`
		CountryCode string  `json:"countryCode"`
		Region      string  `json:"region"`
		City        string  `json:"city"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		Org         string  `json:"org"`
		AS          string  `json:"as"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.Status != "success" {
		return nil
	}

	return &GeoInfo{
		Country:     result.Country,
		CountryCode: result.CountryCode,
		Region:      result.Region,
		City:        result.City,
		Org:         result.Org,
		ASN:         result.AS,
		Latitude:    result.Lat,
		Longitude:   result.Lon,
	}
}

func findProcessesUsingIP(ip string) []int {
	sockets, err := GetSocketInfo()
	if err != nil {
		return nil
	}

	pidMap := make(map[int]bool)

	for _, sock := range sockets.TCP {
		if sock.LocalAddr == ip || sock.RemoteAddr == ip {
			if sock.PID > 0 {
				pidMap[sock.PID] = true
			}
		}
	}

	for _, sock := range sockets.UDP {
		if sock.LocalAddr == ip || sock.RemoteAddr == ip {
			if sock.PID > 0 {
				pidMap[sock.PID] = true
			}
		}
	}

	var pids []int
	for pid := range pidMap {
		pids = append(pids, pid)
	}

	return pids
}
