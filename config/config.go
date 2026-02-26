package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type SSLConfig struct {
	Enabled bool   `json:"enabled"`
	Cert    string `json:"cert"`
	Key     string `json:"key"`
}

type ServerConfig struct {
	Host string    `json:"host"`
	Port int       `json:"port"`
	SSL  SSLConfig `json:"ssl"`
}

type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UIConfig struct {
	Title       string `json:"title"`
	Hostname    string `json:"hostname"`
	HeaderColor string `json:"headerColor"`
	Favicon     string `json:"favicon"`
	Theme       string `json:"theme"`
	CompactMode bool   `json:"compactMode"`
}

type RefreshConfig struct {
	CPU       int `json:"cpu"`
	Memory    int `json:"memory"`
	Disk      int `json:"disk"`
	Network   int `json:"network"`
	GPU       int `json:"gpu"`
	Processes int `json:"processes"`
	Sockets   int `json:"sockets"`
	Firewall  int `json:"firewall"`
}

type Config struct {
	Server  ServerConfig  `json:"server"`
	Auth    AuthConfig    `json:"auth"`
	UI      UIConfig      `json:"ui"`
	Refresh RefreshConfig `json:"refresh"`
}

func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	return &Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 9876,
			SSL: SSLConfig{
				Enabled: false,
				Cert:    "",
				Key:     "",
			},
		},
		Auth: AuthConfig{
			Username: "",
			Password: "",
		},
		UI: UIConfig{
			Title:       hostname,
			Hostname:    hostname,
			HeaderColor: "#1a1a2e",
			Favicon:     "",
			Theme:       "dark",
			CompactMode: false,
		},
		Refresh: RefreshConfig{
			CPU:       5000,
			Memory:    5000,
			Disk:      5000,
			Network:   5000,
			GPU:       5000,
			Processes: 5000,
			Sockets:   5000,
			Firewall:  10000,
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) ToJSON() (string, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Config) HasAuth() bool {
	return c.Auth.Username != "" && c.Auth.Password != ""
}

func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
