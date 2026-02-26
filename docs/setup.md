# Setup Guide

## Prerequisites

- macOS (tested on 12+)
- Go 1.22+ (for building from source)

## Building

```bash
# Clone the repository
git clone https://github.com/wikefjol/notification_service.git
cd notification_service

# Build both binaries
go build -o notify-server ./cmd/notify-server
go build -o notify-send ./client

# Install to /usr/local/bin (optional)
sudo cp notify-server notify-send /usr/local/bin/
```

## Configuration

Create the config directory and file:

```bash
mkdir -p ~/.config/notify-server
```

Create `~/.config/notify-server/config.toml`:

```toml
# Server settings
listen_addr = "127.0.0.1:8666"
max_body_bytes = 4096
max_skew_seconds = 60

# Rate limiting
rate_limit_per_minute = 60
rate_limit_burst = 10

# Replay protection
replay_cache_max_entries = 10000

# Default sound (required)
default_sound = "/System/Library/Sounds/Glass.aiff"

# Senders (at least one required)
[senders.github-actions]
secrets = ["your-secret-key-here"]
sound = "/System/Library/Sounds/Ping.aiff"

[senders.my-agent]
secrets = ["another-secret-key"]
# Uses default_sound if not specified
```

### Available System Sounds

macOS includes sounds at `/System/Library/Sounds/`:
- Basso.aiff, Blow.aiff, Bottle.aiff, Frog.aiff
- Funk.aiff, Glass.aiff, Hero.aiff, Morse.aiff
- Ping.aiff, Pop.aiff, Purr.aiff, Sosumi.aiff
- Submarine.aiff, Tink.aiff

## Running Manually

```bash
# Run the server
notify-server

# In another terminal, send a test notification
export NOTIFY_SECRET="your-secret-key-here"
notify-send --source github-actions --message "Test notification"
```

## Installing as LaunchAgent (Auto-start)

The LaunchAgent ensures the server starts on login and restarts if it crashes.

### Install

1. Edit the plist to use your username:

```bash
# Copy the plist
cp launchd/com.user.notify-server.plist ~/Library/LaunchAgents/

# Edit to replace USER with your username
sed -i '' "s/USER/$(whoami)/g" ~/Library/LaunchAgents/com.user.notify-server.plist
```

2. Ensure the binary is installed:

```bash
# The plist expects the binary at /usr/local/bin/notify-server
sudo cp notify-server /usr/local/bin/
```

3. Load the agent:

```bash
launchctl load -w ~/Library/LaunchAgents/com.user.notify-server.plist
```

### Verify

```bash
# Check if the service is running
launchctl list | grep notify

# Should show something like:
# 12345  0  com.user.notify-server
# (PID, exit code, label)
```

### View Logs

```bash
# Follow the log file
tail -f ~/Library/Logs/notify-server.log
```

### Uninstall

```bash
# Stop and remove the agent
launchctl unload ~/Library/LaunchAgents/com.user.notify-server.plist
rm ~/Library/LaunchAgents/com.user.notify-server.plist
```

### Reload After Changes

If you update the binary or plist:

```bash
# Method 1: Unload and reload
launchctl unload ~/Library/LaunchAgents/com.user.notify-server.plist
launchctl load -w ~/Library/LaunchAgents/com.user.notify-server.plist

# Method 2: Kickstart (restarts the service)
launchctl kickstart -k gui/$(id -u)/com.user.notify-server
```

## Client Usage

The `notify-send` CLI sends authenticated notifications to the server.

### Environment Variables

- `NOTIFY_KEY_ID` - Source/key identifier (alternative to `--source`)
- `NOTIFY_SECRET` - Shared secret for HMAC signing (required)
- `NOTIFY_URL` - Server URL (default: `http://127.0.0.1:8666/notify`)

### Examples

```bash
# Basic usage
export NOTIFY_SECRET="your-secret"
notify-send --source my-agent --message "Build completed"

# With optional fields
notify-send --source github-actions \
  --message "Workflow failed" \
  --event "workflow_run" \
  --repo "user/repo" \
  --severity "error" \
  --url "https://github.com/user/repo/actions/runs/123"

# Using environment variable for source
export NOTIFY_KEY_ID="my-agent"
notify-send --message "Task done"
```

## Exposing via Tunnel (Advanced)

To receive notifications from remote systems (e.g., GitHub Actions), expose the server via a tunnel:

```bash
# Example with Tailscale Funnel
tailscale funnel 8666
```

Note: The server is hardened against untrusted traffic but should only be exposed via authenticated tunnels.
