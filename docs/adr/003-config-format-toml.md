# ADR-003: Use TOML for configuration

## Status
Accepted

## Context
We need a configuration format for server settings and per-source secrets/sounds. The config includes:
- Server settings (listen address, limits, timeouts)
- Nested sender definitions (key-id → secrets + sound path)

Options considered: JSON, YAML, TOML, environment variables

## Decision
**TOML**

Dependency: `github.com/BurntSushi/toml`

## Rationale

### Why not JSON?
- No comments — config files benefit from inline documentation
- Verbose syntax for the moderate nesting we have
- Strict (trailing commas break parsing)

### Why not YAML?
- Whitespace-sensitive (subtle errors)
- Type coercion footguns (`no` → false, Norway problem)
- Also requires external dependency — no advantage over TOML

### Why TOML?
- Designed for config files specifically
- Comments allowed
- Clear section syntax: `[senders.claude]`
- Stable spec (frozen, won't change)
- Mature library (10+ years, widely used, essentially "done")

### Dependency tradeoff
Stdlib-only has value for libraries others depend on. For a personal daemon:
- One stable, single-purpose dependency is trivial to audit
- Config parsing isn't attack surface (local file, written by user)
- UX benefit (comments) outweighs theoretical purity

## Consequences

### Config file location

**Path:** `~/.config/notify-server/config.toml`

No fallback paths. Single location keeps behavior predictable and debuggable.

If the directory doesn't exist, server logs a clear error and exits (fail closed).

### Example config
```toml
listen_addr = "127.0.0.1:8666"
max_body_bytes = 4096
max_skew_seconds = 60
rate_limit_per_minute = 10
rate_limit_burst = 3
replay_cache_max_entries = 10000
default_sound = "/System/Library/Sounds/Ping.aiff"

# Claude Code notifications
[senders.claude]
secrets = ["secret1", "secret2_for_rotation"]
sound = "/Users/me/Library/Sounds/claude.wav"

# CI pipeline
[senders.ci]
secrets = ["ci_secret"]
sound = "/Users/me/Library/Sounds/ci.wav"
```

### Dependencies
- Add `github.com/BurntSushi/toml` to go.mod
