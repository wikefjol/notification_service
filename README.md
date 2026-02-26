# notify-server

A hardened webhook receiver for macOS that plays sounds on authenticated events from coding agents and automation.

## Features

- **Sound notifications** - Plays configurable sounds when webhooks arrive
- **Multi-source support** - Different sounds for different sources (Claude, CI, etc.)
- **Hardened security** - HMAC authentication, replay protection, rate limiting
- **Safe for public exposure** - Designed to be exposed via tunnel (Tailscale Funnel, ngrok)
- **Auto-start** - LaunchAgent for automatic startup on login

## Quick Start

### 1. Build

```bash
git clone https://github.com/wikefjol/notification_service.git
cd notification_service
go build -o notify-server ./cmd/notify-server
go build -o notify-send ./client
```

### 2. Configure

Create `~/.config/notify-server/config.toml`:

```toml
listen_addr = "127.0.0.1:8666"
max_body_bytes = 4096
max_skew_seconds = 60
rate_limit_per_minute = 60
rate_limit_burst = 10
default_sound = "/System/Library/Sounds/Glass.aiff"

[senders.claude]
secrets = ["your-secret-here"]
sound = "/System/Library/Sounds/Ping.aiff"

[senders.ci]
secrets = ["another-secret"]
sound = "/System/Library/Sounds/Submarine.aiff"
```

Generate secrets with: `openssl rand -hex 32`

### 3. Run

```bash
./notify-server
```

### 4. Test

```bash
export NOTIFY_SECRET="your-secret-here"
./notify-send --source claude --message "Hello from notify-send"
```

You should hear a sound.

## Installation

### Install Binaries

```bash
sudo cp notify-server notify-send /usr/local/bin/
```

### Auto-start with launchd

```bash
# Copy and configure the LaunchAgent
cp launchd/com.user.notify-server.plist ~/Library/LaunchAgents/
sed -i '' "s/USER/$(whoami)/g" ~/Library/LaunchAgents/com.user.notify-server.plist

# Load the agent
launchctl load -w ~/Library/LaunchAgents/com.user.notify-server.plist
```

If `load -w` doesn't work on newer macOS:

```bash
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.user.notify-server.plist
launchctl enable gui/$(id -u)/com.user.notify-server
```

### Verify

```bash
launchctl list | grep notify-server
# Should show: <PID>  <exit>  com.user.notify-server
```

## API

### Endpoint

`POST /notify`

### Headers (required)

| Header | Description |
|--------|-------------|
| `X-Key-Id` | Source identifier (e.g., `claude`, `ci`) |
| `X-Timestamp` | Unix timestamp (seconds) |
| `X-Signature` | `base64(HMAC-SHA256(secret, "<timestamp>.<body>"))` |

### Body (JSON)

```json
{
  "source": "claude",
  "message": "Build completed successfully"
}
```

Optional fields: `event`, `repo`, `context_id`, `severity`, `url`, `meta`

### Response Codes

| Code | Meaning |
|------|---------|
| 204 | Success (sound played) |
| 400 | Invalid JSON or missing fields |
| 401 | Auth failure (bad signature, timestamp, or source mismatch) |
| 413 | Payload too large |
| 415 | Unsupported media type (not JSON) |
| 429 | Rate limited |

## Client Usage

The `notify-send` CLI computes authentication headers and sends notifications.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `NOTIFY_SECRET` | Shared secret for HMAC signing (required) |
| `NOTIFY_KEY_ID` | Source identifier (alternative to `--source`) |
| `NOTIFY_URL` | Server URL (default: `http://127.0.0.1:8666/notify`) |

### Examples

```bash
# Basic
export NOTIFY_SECRET="your-secret"
notify-send --source claude --message "Task complete"

# With metadata
notify-send --source ci \
  --message "Build failed" \
  --event "build" \
  --repo "user/repo" \
  --severity "error" \
  --url "https://github.com/user/repo/actions/runs/123"
```

## Exposing Publicly (Tunnel)

To receive notifications from remote systems (GitHub Actions, remote agents):

```bash
# Tailscale Funnel
tailscale funnel 8666

# Or ngrok
ngrok http 8666
```

Then set `NOTIFY_URL` on remote systems to your tunnel URL.

**Note:** The server is hardened for public exposure but should only be used with authenticated tunnels when possible.

## Security Model

### Authentication

- HMAC-SHA256 signatures over `<timestamp>.<body>`
- Constant-time signature comparison
- Source field must match `X-Key-Id` header

### Replay Protection

- Requests rejected if timestamp differs from server time by more than 60 seconds
- Configurable via `max_skew_seconds`

### Rate Limiting

- Token bucket per `X-Key-Id`
- Default: 60/minute with burst of 10
- Configurable via `rate_limit_per_minute` and `rate_limit_burst`

### Hardening

- Binds only to `127.0.0.1` (localhost)
- Body size limit (default 4KB)
- Sound paths are server-configured only (never from request body)
- No shell expansion in command execution
- Secrets never logged

### Key Rotation

Multiple secrets per source are supported for zero-downtime rotation:

```toml
[senders.claude]
secrets = ["new-secret", "old-secret"]
```

## Configuration Reference

| Key | Default | Description |
|-----|---------|-------------|
| `listen_addr` | `127.0.0.1:8666` | Server bind address |
| `max_body_bytes` | `4096` | Maximum request body size |
| `max_skew_seconds` | `60` | Maximum timestamp drift |
| `rate_limit_per_minute` | `60` | Requests per minute per source |
| `rate_limit_burst` | `10` | Burst allowance |
| `replay_cache_max_entries` | `10000` | Replay cache size |
| `default_sound` | (required) | Fallback sound path |
| `senders.<name>.secrets` | (required) | List of valid secrets |
| `senders.<name>.sound` | (optional) | Sound path for this source |

### Available System Sounds

macOS includes sounds at `/System/Library/Sounds/`:

Basso, Blow, Bottle, Frog, Funk, Glass, Hero, Morse, Ping, Pop, Purr, Sosumi, Submarine, Tink

## Logs

```bash
tail -f ~/Library/Logs/notify-server.log
```

## Troubleshooting

### Server won't start

1. Check if port is in use: `lsof -i :8666`
2. Verify config syntax: server logs errors on startup
3. Ensure config file exists at `~/.config/notify-server/config.toml`

### 401 Unauthorized

1. Verify `NOTIFY_SECRET` matches a secret in config
2. Check system clocks are synchronized (timestamp validation)
3. Ensure `--source` matches a configured sender name

### No sound

1. Check sound file path exists
2. Test manually: `afplay /System/Library/Sounds/Ping.aiff`
3. Check macOS sound is not muted

### LaunchAgent not starting

```bash
# Check status
launchctl list | grep notify-server

# View logs
cat ~/Library/Logs/notify-server.log

# Reload
launchctl kickstart -k gui/$(id -u)/com.user.notify-server
```

## Development

```bash
# Run tests
go test ./...

# Run server in foreground
go run ./cmd/notify-server

# Test with curl
curl -X POST http://127.0.0.1:8666/notify \
  -H "Content-Type: application/json" \
  -H "X-Key-Id: claude" \
  -H "X-Timestamp: $(date +%s)" \
  -H "X-Signature: $(echo -n "$(date +%s).{\"source\":\"claude\",\"message\":\"test\"}" | openssl dgst -sha256 -hmac "your-secret" -binary | base64)" \
  -d '{"source":"claude","message":"test"}'
```

## License

MIT
