package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SSH holds SSH tunnel configuration.
type SSH struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	User       string `yaml:"user"`
	KeyPath    string `yaml:"key_path"`
	RemotePort int    `yaml:"remote_port"`
}

// Config is the top-level application configuration.
// Priority: CLI flags > environment variables > config file defaults.
type Config struct {
	GatewayURL string `yaml:"gateway_url"`
	Token      string `yaml:"token"`
	SessionKey string `yaml:"session_key"`
	SSH        *SSH   `yaml:"ssh,omitempty"`
	Backend    string `yaml:"backend"` // "openclaw" (default) or "zeroclaw"
}

// Load reads config from file, applies env overrides, then flag overrides.
func Load() (*Config, error) {
	cfg := defaults()

	// 1. Config file
	path := FilePath()
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", path, err)
		}
	}

	// 2. Environment variables
	if v := env("OPENCLAW_GATEWAY_URL", "CLAWCHAT_GATEWAY"); v != "" {
		cfg.GatewayURL = v
	}
	if v := os.Getenv("OPENCLAW_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("CLAWCHAT_SESSION"); v != "" {
		cfg.SessionKey = v
	}
	if v := os.Getenv("CLAWCHAT_BACKEND"); v != "" {
		cfg.Backend = v
	}

	// SSH env
	if v := os.Getenv("CLAWCHAT_SSH_HOST"); v != "" {
		if cfg.SSH == nil {
			cfg.SSH = &SSH{}
		}
		cfg.SSH.Host = v
	}

	// 3. CLI flags (defined here so help text is accurate)
	var (
		flagGateway   = flag.String("gateway", cfg.GatewayURL, "Gateway WebSocket URL (ws:// or wss://)")
		flagToken     = flag.String("token", cfg.Token, "Gateway auth token")
		flagSession   = flag.String("session", cfg.SessionKey, "Session key to connect to (default: first available)")
		flagBackend   = flag.String("backend", cfg.Backend, `Backend to use: "openclaw" (default) or "zeroclaw"`)
		flagSSHHost   = flag.String("ssh-host", "", "SSH tunnel host")
		flagSSHPort   = flag.Int("ssh-port", 22, "SSH tunnel port")
		flagSSHUser   = flag.String("ssh-user", "", "SSH tunnel user")
		flagSSHKey    = flag.String("ssh-key", "", "Path to SSH private key")
		flagSSHRemote = flag.Int("ssh-remote-port", 18789, "Remote gateway port to forward")
		flagVersion   = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Println("clawchat-cli dev")
		os.Exit(0)
	}

	if *flagGateway != "" {
		cfg.GatewayURL = *flagGateway
	}
	if *flagToken != "" {
		cfg.Token = *flagToken
	}
	if *flagSession != "" {
		cfg.SessionKey = *flagSession
	}
	if *flagBackend != "" {
		cfg.Backend = *flagBackend
	}
	if *flagSSHHost != "" {
		if cfg.SSH == nil {
			cfg.SSH = &SSH{}
		}
		cfg.SSH.Host = *flagSSHHost
		cfg.SSH.Port = *flagSSHPort
		cfg.SSH.User = *flagSSHUser
		cfg.SSH.KeyPath = *flagSSHKey
		cfg.SSH.RemotePort = *flagSSHRemote
	}

	// Apply backend-specific defaults when backend is zeroclaw.
	if cfg.Backend == "zeroclaw" {
		if cfg.GatewayURL == "" || cfg.GatewayURL == "ws://localhost:18789" {
			cfg.GatewayURL = "ws://localhost:42617"
		}
	}

	return cfg, nil
}

// Save writes the config to the default config file path.
func (c *Config) Save() error {
	path := FilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Validate returns an error if required fields are missing.
func (c *Config) Validate() error {
	if c.GatewayURL == "" {
		return fmt.Errorf("gateway URL is required (--gateway or OPENCLAW_GATEWAY_URL)")
	}
	if c.Token == "" {
		return fmt.Errorf("auth token is required (--token or OPENCLAW_TOKEN)")
	}
	switch c.Backend {
	case "", "openclaw", "zeroclaw":
		// valid
	default:
		return fmt.Errorf("unknown backend %q: must be \"openclaw\" or \"zeroclaw\"", c.Backend)
	}
	// SSH tunnel is only applicable to the openclaw backend.
	if c.SSH != nil && c.Backend != "zeroclaw" {
		if c.SSH.Host == "" {
			return fmt.Errorf("ssh-host is required when using SSH tunnel")
		}
		if c.SSH.User == "" {
			return fmt.Errorf("ssh-user is required when using SSH tunnel")
		}
	}
	return nil
}

// IsZeroClaw returns true when the zeroclaw backend is active.
func (c *Config) IsZeroClaw() bool {
	return c.Backend == "zeroclaw"
}

// SSHEnabled returns true if SSH tunnel is configured.
func (c *Config) SSHEnabled() bool {
	return c.SSH != nil && c.SSH.Host != ""
}

// FilePath returns the path to the config file.
// Always uses ~/.config (XDG convention) regardless of platform.
func FilePath() string {
	if v := os.Getenv("CLAWCHAT_CONFIG"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clawchat-cli", "config.yaml")
}

func defaults() *Config {
	return &Config{
		GatewayURL: "ws://localhost:18789",
		Backend:    "openclaw",
	}
}

// env returns the first non-empty value from the given env var names.
func env(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// ExpandTilde expands a leading ~ to the user's home directory.
func ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
