# ADR-007: Server timeouts and graceful shutdown

## Status
Accepted

## Context
The server will be exposed to public internet via tunnel. We need:
- Protection against slow clients / slowloris-style resource exhaustion
- Clean shutdown for launchd integration (avoid restart flakiness)
- Bounded resource usage under adversarial conditions

## Decision
**Configure explicit http.Server timeouts and handle SIGTERM/SIGINT gracefully**

## Server timeouts

All timeouts configured on `http.Server`:

| Timeout | Value | Rationale |
|---------|-------|-----------|
| `ReadHeaderTimeout` | 5s | Time to read request headers; protects against slowloris |
| `ReadTimeout` | 10s | Total time to read entire request (headers + body) |
| `WriteTimeout` | 10s | Time to write response; generous for our small responses |
| `IdleTimeout` | 60s | Keep-alive connection idle time; matches HTTP/1.1 defaults |

### Why these values?
- Our requests are tiny (≤4KB body) — 10s read is extremely generous
- Responses are minimal (204 No Content typically) — 10s write is generous
- Timeouts are defense-in-depth; rate limiting and body size limits are primary controls
- Values are not configurable in MVP — these are safe defaults

## Graceful shutdown

On SIGTERM or SIGINT:
1. Stop accepting new connections
2. Wait for in-flight requests to complete (with timeout)
3. Cancel any pending sound playback goroutines
4. Exit cleanly

Implementation:
```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
defer stop()

// ... start server ...

<-ctx.Done()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
server.Shutdown(shutdownCtx)
```

### Shutdown timeout
- 5 seconds to drain in-flight requests
- If exceeded, force close remaining connections
- Sound playback goroutines use context cancellation

## Consequences

### Benefits
- Resilient to slow/malicious clients
- Clean launchd integration (no orphaned connections on restart)
- Bounded resource usage

### Tradeoffs
- Slightly more complex server setup (~15 lines)
- Shutdown delay up to 5s (acceptable for daemon)

## Implementation notes

- Use `http.Server{}` struct, not `http.ListenAndServe` helper
- Pass server's base context to handlers for cancellation propagation
- Log shutdown events for debugging launchd issues
