# notify-service

> Hardened webhook receiver for macOS that plays sounds on authenticated events from coding agents and automation.

## Commands

```bash
# Dev
go run ./server
# Test
go test ./...
# Build
go build -o notify-server ./server
go build -o notify-send ./client
# Install launchd agent
cp launchd/com.user.notify-server.plist ~/Library/LaunchAgents/
launchctl load -w ~/Library/LaunchAgents/com.user.notify-server.plist
```

## Architecture

Go single binary. Server + CLI client.

```
notification_service/
├── server/           # HTTP daemon (127.0.0.1:8666)
├── client/           # notify-send CLI wrapper
├── launchd/          # LaunchAgent plist
├── examples/         # Sample config, curl examples
└── tests/            # Auth, replay, size, rate limit tests
```

**Key flows**:
- `notify-send` → computes HMAC signature → POST to server
- Server validates auth → looks up source → plays configured sound via `afplay`

## Code Style

- No shell=True or command string expansion (security)
- Constant-time signature comparison
- Structured logging (no secrets/signatures logged)
- Explicit error handling; fail closed on config errors

## Workflow

1. **Before coding**: Read relevant `docs/adr/` entries. Propose plan and wait for approval.
2. Check `gh issue view <N>` for full context on issue-tagged work.
3. **Implementation**: Small, focused diffs. One concern per commit.
4. **Testing**: Run tests. Verify nothing breaks.
5. **Documentation**:
   - New pattern or architectural decision (after approval) → write ADR
   - New/changed user-facing behavior → update relevant doc in docs/
   - New config or env var → update docs/setup.md
6. **Commit**: Conventional commits (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`).

## Testing

Required coverage:
- Signature verification (valid/invalid)
- Timestamp window (replay resistance)
- Payload size limits
- Source/X-Key-Id mismatch rejection
- Rate limiting behavior

## Boundaries

- YOU MUST NOT execute arbitrary commands or accept sound paths from requests
- YOU MUST NOT log secrets, signatures, or full payloads
- YOU MUST bind server only to 127.0.0.1 (localhost)
- YOU MUST use argv array (not shell) for afplay invocation
- IMPORTANT: Sound selection is strictly server-configured per source

## Context Pointers

- Architecture decisions: `docs/adr/INDEX.md`
- API spec: `initial_spec_and_prompt` (sections 5-6)
- Security requirements: `initial_spec_and_prompt` (section 6)

## Current Focus

**Active**: Tailscale Funnel setup (remote notifications)
  - Blocked: DNS not resolving publicly (`mbp.tailf0fb1e.ts.net` returns NXDOMAIN)
  - Likely cause: DNS propagation delay or eduVPN conflict on remote machine
  - Next: Test from phone on mobile data, or wait for DNS propagation
**Last session**: 2026-02-26 - see `docs/session-2026-02-26.md`
**Done**: Server, client, launchd auto-start, local notifications working

## Gotchas

- Server exposed via tunnel (e.g., Tailscale Funnel) - expect random internet traffic
- Config file lives in user home, not in repo (secrets outside version control)
- Multiple secrets per key-id supported for rotation
