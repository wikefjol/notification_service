// Package server implements the notify-server daemon.
package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all server configuration.
type Config struct {
	// ListenAddr is the address to bind to (default "127.0.0.1:8666").
	ListenAddr string `toml:"listen_addr"`

	// MaxBodyBytes is the maximum allowed request body size (default 4096).
	MaxBodyBytes int `toml:"max_body_bytes"`

	// MaxSkewSeconds is the maximum allowed timestamp skew for replay resistance (default 60).
	MaxSkewSeconds int `toml:"max_skew_seconds"`

	// RateLimitPerMinute is the token bucket refill rate per key-id (default 10).
	RateLimitPerMinute int `toml:"rate_limit_per_minute"`

	// RateLimitBurst is the token bucket burst size per key-id (default 3).
	RateLimitBurst int `toml:"rate_limit_burst"`

	// ReplayCacheMaxEntries is the maximum entries in the replay cache (default 10000).
	ReplayCacheMaxEntries int `toml:"replay_cache_max_entries"`

	// DefaultSound is the fallback sound file path when a sender has no configured sound.
	DefaultSound string `toml:"default_sound"`

	// Senders maps key-id to sender configuration.
	Senders map[string]SenderConfig `toml:"senders"`

	// AllowNonLocalhost permits binding to non-localhost addresses.
	// This is a security risk and should only be used if you understand the implications.
	AllowNonLocalhost bool `toml:"allow_non_localhost"`
}

// SenderConfig holds per-sender configuration.
type SenderConfig struct {
	// Secrets is a list of valid HMAC secrets for this sender.
	// Multiple secrets allow for key rotation.
	Secrets []string `toml:"secrets"`

	// Sound is the path to the sound file to play for this sender.
	// Falls back to Config.DefaultSound if empty or missing.
	Sound string `toml:"sound"`
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "notify-server", "config.toml")
}

// LoadConfig reads and validates the configuration file at the given path.
// Returns an error if the file is missing, unparseable, or invalid.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets default values for unspecified fields.
func (c *Config) applyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:8666"
	}
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = 4096
	}
	if c.MaxSkewSeconds == 0 {
		c.MaxSkewSeconds = 60
	}
	if c.RateLimitPerMinute == 0 {
		c.RateLimitPerMinute = 10
	}
	if c.RateLimitBurst == 0 {
		c.RateLimitBurst = 3
	}
	if c.ReplayCacheMaxEntries == 0 {
		c.ReplayCacheMaxEntries = 10000
	}
}

// validate checks that all required fields are present and valid.
func (c *Config) validate() error {
	if c.DefaultSound == "" {
		return errors.New("default_sound is required")
	}

	if len(c.Senders) == 0 {
		return errors.New("at least one sender must be configured")
	}

	for keyID, sender := range c.Senders {
		if len(sender.Secrets) == 0 {
			return fmt.Errorf("sender %q has no secrets configured", keyID)
		}
		for i, secret := range sender.Secrets {
			if secret == "" {
				return fmt.Errorf("sender %q has empty secret at index %d", keyID, i)
			}
		}
	}

	// Validate listen_addr is localhost-only unless explicitly overridden
	if !c.AllowNonLocalhost && !isLocalhostAddr(c.ListenAddr) {
		return fmt.Errorf("listen_addr %q is not a localhost address; "+
			"binding to non-localhost exposes the server to the network which is a security risk; "+
			"set allow_non_localhost = true in config to override", c.ListenAddr)
	}

	return nil
}

// isLocalhostAddr checks if the given address binds to localhost only.
func isLocalhostAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If we can't parse it, assume it's not safe
		return false
	}

	// Check for common localhost representations
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}
