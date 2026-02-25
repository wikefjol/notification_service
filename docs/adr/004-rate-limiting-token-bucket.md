# ADR-004: Token bucket rate limiting

## Status
Accepted

## Context
We need rate limiting for a webhook receiver exposed to public internet via tunnel.

Options considered: fixed window, sliding window log, sliding window counter, token bucket

### Threat model clarification
Rate limiting is **not** the primary security control here:
- Auth (HMAC + replay cache) gates all requests
- Brute forcing HMAC-SHA256 is infeasible regardless of rate limits
- DDoS would hit the tunnel infrastructure before localhost

Rate limiting serves as **defense-in-depth** against:
- Compromised keys (limits sound/log spam if secret leaks)
- Buggy clients (misconfigured hook in a loop)

## Decision
**Token bucket** with per-key-id buckets

Default parameters:
- Rate: 10 requests/minute (1 token per 6 seconds)
- Burst: 3 (bucket capacity)

## Rationale

### Why not fixed window?
Boundary burst problem: 10 requests at 0:59 + 10 at 1:00 = 20 in 2 seconds. Not catastrophic for this use case, but token bucket is cleaner for similar complexity.

### Why token bucket?
- Explicit burst parameter matches real usage (agent might retry 2-3 times quickly)
- Smooth limiting without boundary artifacts
- O(keys) memory — trivial for ~10 sources
- Well-documented algorithm, easy to review
- ~30 lines of Go

### Why per-key-id?
One misbehaving or compromised source shouldn't block others.

## Consequences

### Behavior
- Fresh/idle source: 3 requests allowed immediately
- Sustained: 1 request per 6 seconds
- Idle time refills bucket up to burst capacity

### Implementation
- In-memory map: `buckets[keyId] = {tokens, lastRefill}`
- Check/refill on each request
- No persistence needed (restart = full buckets, acceptable)

### Rejected requests
- Return 429 Too Many Requests
- Do not play sound
- Log key-id and rejection (no secrets)
