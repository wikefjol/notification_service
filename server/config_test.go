package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_ValidConfig(t *testing.T) {
	content := `
listen_addr = "127.0.0.1:9999"
max_body_bytes = 8192
max_skew_seconds = 120
rate_limit_per_minute = 20
rate_limit_burst = 5
replay_cache_max_entries = 5000
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.claude]
secrets = ["secret1", "secret2"]
sound = "/path/to/claude.wav"

[senders.ci]
secrets = ["ci_secret"]
`
	path := writeTempConfig(t, content)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check explicitly set values
	if cfg.ListenAddr != "127.0.0.1:9999" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:9999")
	}
	if cfg.MaxBodyBytes != 8192 {
		t.Errorf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, 8192)
	}
	if cfg.MaxSkewSeconds != 120 {
		t.Errorf("MaxSkewSeconds = %d, want %d", cfg.MaxSkewSeconds, 120)
	}
	if cfg.RateLimitPerMinute != 20 {
		t.Errorf("RateLimitPerMinute = %d, want %d", cfg.RateLimitPerMinute, 20)
	}
	if cfg.RateLimitBurst != 5 {
		t.Errorf("RateLimitBurst = %d, want %d", cfg.RateLimitBurst, 5)
	}
	if cfg.ReplayCacheMaxEntries != 5000 {
		t.Errorf("ReplayCacheMaxEntries = %d, want %d", cfg.ReplayCacheMaxEntries, 5000)
	}
	if cfg.DefaultSound != "/System/Library/Sounds/Ping.aiff" {
		t.Errorf("DefaultSound = %q, want %q", cfg.DefaultSound, "/System/Library/Sounds/Ping.aiff")
	}

	// Check senders
	if len(cfg.Senders) != 2 {
		t.Fatalf("len(Senders) = %d, want 2", len(cfg.Senders))
	}

	claude, ok := cfg.Senders["claude"]
	if !ok {
		t.Fatal("sender 'claude' not found")
	}
	if len(claude.Secrets) != 2 {
		t.Errorf("claude.Secrets len = %d, want 2", len(claude.Secrets))
	}
	if claude.Sound != "/path/to/claude.wav" {
		t.Errorf("claude.Sound = %q, want %q", claude.Sound, "/path/to/claude.wav")
	}

	ci, ok := cfg.Senders["ci"]
	if !ok {
		t.Fatal("sender 'ci' not found")
	}
	if len(ci.Secrets) != 1 {
		t.Errorf("ci.Secrets len = %d, want 1", len(ci.Secrets))
	}
	// ci has no sound configured, should be empty (falls back to default at runtime)
	if ci.Sound != "" {
		t.Errorf("ci.Sound = %q, want empty", ci.Sound)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	content := `
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.test]
secrets = ["secret"]
`
	path := writeTempConfig(t, content)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check defaults are applied
	if cfg.ListenAddr != "127.0.0.1:8666" {
		t.Errorf("ListenAddr default = %q, want %q", cfg.ListenAddr, "127.0.0.1:8666")
	}
	if cfg.MaxBodyBytes != 4096 {
		t.Errorf("MaxBodyBytes default = %d, want %d", cfg.MaxBodyBytes, 4096)
	}
	if cfg.MaxSkewSeconds != 60 {
		t.Errorf("MaxSkewSeconds default = %d, want %d", cfg.MaxSkewSeconds, 60)
	}
	if cfg.RateLimitPerMinute != 10 {
		t.Errorf("RateLimitPerMinute default = %d, want %d", cfg.RateLimitPerMinute, 10)
	}
	if cfg.RateLimitBurst != 3 {
		t.Errorf("RateLimitBurst default = %d, want %d", cfg.RateLimitBurst, 3)
	}
	if cfg.ReplayCacheMaxEntries != 10000 {
		t.Errorf("ReplayCacheMaxEntries default = %d, want %d", cfg.ReplayCacheMaxEntries, 10000)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	content := `
this is not valid toml {{{
`
	path := writeTempConfig(t, content)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadConfig_MissingDefaultSound(t *testing.T) {
	content := `
[senders.test]
secrets = ["secret"]
`
	path := writeTempConfig(t, content)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing default_sound")
	}
}

func TestLoadConfig_NoSenders(t *testing.T) {
	content := `
default_sound = "/System/Library/Sounds/Ping.aiff"
`
	path := writeTempConfig(t, content)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for no senders")
	}
}

func TestLoadConfig_EmptySecretsArray(t *testing.T) {
	content := `
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.test]
secrets = []
`
	path := writeTempConfig(t, content)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty secrets array")
	}
}

func TestLoadConfig_EmptySecretString(t *testing.T) {
	content := `
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.test]
secrets = ["valid", ""]
`
	path := writeTempConfig(t, content)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty secret string")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Skip("could not determine home directory")
	}
	// Should end with expected suffix
	expected := filepath.Join(".config", "notify-server", "config.toml")
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultConfigPath() = %q, want absolute path", path)
	}
	if filepath.Base(path) != "config.toml" {
		t.Errorf("DefaultConfigPath() = %q, want to end with config.toml", path)
	}
	_ = expected // used for documentation
}

func TestLoadConfig_LocalhostAddresses(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"IPv4 localhost", "127.0.0.1:8666"},
		{"localhost hostname", "localhost:8666"},
		{"IPv6 localhost", "[::1]:8666"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `
listen_addr = "` + tt.addr + `"
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.test]
secrets = ["secret"]
`
			path := writeTempConfig(t, content)
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("expected %q to be valid localhost, got error: %v", tt.addr, err)
			}
			if cfg.ListenAddr != tt.addr {
				t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, tt.addr)
			}
		})
	}
}

func TestLoadConfig_NonLocalhostRejected(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"all interfaces IPv4", "0.0.0.0:8666"},
		{"all interfaces IPv6", "[::]:8666"},
		{"private IP", "192.168.1.100:8666"},
		{"public IP", "8.8.8.8:8666"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `
listen_addr = "` + tt.addr + `"
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.test]
secrets = ["secret"]
`
			path := writeTempConfig(t, content)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatalf("expected %q to be rejected as non-localhost", tt.addr)
			}
			// Verify error message mentions the security concern
			if !strings.Contains(err.Error(), "not a localhost address") {
				t.Errorf("error should mention 'not a localhost address', got: %v", err)
			}
		})
	}
}

func TestLoadConfig_NonLocalhostWithOverride(t *testing.T) {
	content := `
listen_addr = "0.0.0.0:8666"
allow_non_localhost = true
default_sound = "/System/Library/Sounds/Ping.aiff"

[senders.test]
secrets = ["secret"]
`
	path := writeTempConfig(t, content)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected non-localhost with override to be valid, got error: %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:8666" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:8666")
	}
	if !cfg.AllowNonLocalhost {
		t.Error("AllowNonLocalhost should be true")
	}
}

func TestIsLocalhostAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8666", true},
		{"localhost:8666", true},
		{"[::1]:8666", true},
		{"0.0.0.0:8666", false},
		{"[::]:8666", false},
		{"192.168.1.1:8666", false},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			got := isLocalhostAddr(tt.addr)
			if got != tt.want {
				t.Errorf("isLocalhostAddr(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// writeTempConfig writes content to a temporary file and returns the path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}
