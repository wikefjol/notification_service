# ADR-006: Use afplay for sound playback (bounded)

## Status
Accepted (updated with bounded execution requirements)

## Context
We need to play notification sounds on macOS when authenticated webhooks arrive.

Options considered: afplay (CLI), NSSound/AVAudioPlayer (cgo), Go audio libraries (oto/beep), osascript

## Decision
**afplay** via `exec.CommandContext` with timeout and concurrency limits

## Rationale

### Why afplay?
- Ships with macOS — no install or dependencies
- Simple invocation: `afplay /path/to/sound.aiff`
- Supports common formats (AIFF, WAV, MP3, AAC)
- Satisfies security requirement: `exec.Command("afplay", path)` uses argv array, no shell

### Why not native APIs (cgo)?
- Complicates build process
- More control than we need for "play a ping"

### Why not Go audio libraries?
- Additional dependencies
- Cross-platform support we don't need (macOS only)

## Implementation

### Execution (bounded)
- Use `exec.CommandContext` with 30-second timeout
  - Prevents goroutine/process accumulation if afplay hangs
  - 30s is generous; typical sounds are <5s
- Concurrency limit via semaphore (capacity: 2)
  - Prevents process accumulation under webhook spam
  - Allows brief overlap for natural UX, but caps total concurrent playback
  - If semaphore full, skip playback (log warning, still return 204)
- Return 204 immediately; sound plays async in bounded goroutine

### Failure handling
- If configured sound file missing/unplayable:
  1. Try `default_sound` from config
  2. If default also fails, log error
  3. Always return 204 after successful auth (avoids retry storms)
- Timeout/cancel: log warning, process killed, goroutine exits cleanly

### Sound selection
- Source → sound mapping is strictly server-configured
- Never accept sound paths from request payload (security boundary)
- Lookup: `config.senders[keyId].sound` → fallback to `config.default_sound`

## Future considerations

Burst detection: if >N notifications in T seconds, play a distinct "burst" chime instead of overlapping pings. This avoids cacophony and provides clearer feedback ("something needs attention" vs "many things need attention").

Not implemented in MVP — note for future enhancement if overlap becomes annoying in practice.
