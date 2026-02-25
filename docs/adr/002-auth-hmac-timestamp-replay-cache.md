# ADR-002: HMAC-SHA256 + Timestamp + Replay Cache for authentication

## Status
Accepted (updated with bounded cache and signature versioning)

## Context
We need an authentication mechanism for a webhook receiver that:
- Will be exposed to public internet via tunnel
- Must resist replay attacks
- Should not require refactoring as the project grows
- Should remain simple and stateless where possible

Options considered:
- Simple API key (bearer token)
- HMAC-SHA256 + timestamp only
- HMAC-SHA256 + timestamp + nonce/replay cache
- JWT
- mTLS

## Decision
**HMAC-SHA256 + Timestamp + Replay Cache**

Signature scheme (v1):
```
X-Signature = base64(HMAC-SHA256(secret, "<timestamp>.POST./notify.<raw_body>"))
```

Required headers:
- `X-Key-Id`: source identifier
- `X-Timestamp`: unix seconds
- `X-Signature`: computed signature

### Signature base string format

The signed string includes method and path to prevent cross-endpoint replay if we add endpoints later:
```
<timestamp>.POST./notify.<body>
```

This is "v1" of the signature format. No version header is needed now — if we ever need v2, we can detect format by inspecting signature structure or add a header then. Overengineering versioning now adds complexity without benefit; the current format is extensible enough.

### Replay cache (bounded)

- Use `X-Signature` as cache key (already unique per request)
- Global cache with explicit bounds:
  - `replay_cache_max_entries`: 10,000 (configurable)
  - `replay_cache_ttl`: 2 × `max_skew_seconds` (default 120s)
- Eviction: LRU when at capacity
- On overflow: evict oldest entry, then insert new one (never reject valid requests due to cache size)

### Idempotency semantics

Using X-Signature as cache key means identical retries within TTL are rejected. This is acceptable and even desirable for "play sound" — we don't want duplicate pings for the same logical event. Clients should not retry the exact same request; if they need to retry, the new timestamp produces a new signature.

### Body.source validation

**Decision:** `body.source` is required and must match `X-Key-Id`.

Rationale:
- Keeps payload self-describing (useful for logging, future storage/UI)
- Catches client misconfiguration early (wrong key-id in env vs code)
- Minimal burden on clients (they already know their source)

If `body.source` is missing or doesn't match `X-Key-Id` → 401 Unauthorized.

## Rationale

### Why not timestamp-only?
Timestamp window (±60s) allows replay within that window. Adding a replay cache closes this gap with minimal complexity.

### Why not JWT?
JWT solves identity, claims, and delegation — problems we don't have. Webhook auth needs "prove you know the secret," not "prove who you are."

### Why X-Signature as cache key?
- Already unique per request (includes timestamp + body in signature)
- No additional header for clients to generate
- Simpler client contract

### Why include method+path in signature?
- Prevents cross-endpoint replay if we add endpoints (e.g., POST /notify vs future POST /ack)
- Minimal complexity increase (client already knows method+path)
- No version header needed — format is self-describing and extensible

### Why in-memory cache with explicit bounds?
- No external dependencies
- Explicit max_entries (10,000) prevents unbounded growth even under attack
- TTL of 2× skew provides margin for clock drift edge cases
- LRU eviction is simple and correct — oldest entries are least likely to be replayed
- Restart clears cache — acceptable; attacker would need captured request + server restart + still within timestamp window

## Consequences

### Auth flow
1. Check timestamp within allowed skew
2. Verify HMAC signature (constant-time compare)
3. Check signature not in replay cache
4. Add signature to cache with TTL
5. Process request

### Failure modes
- Stale timestamp → 401
- Invalid signature → 401
- Replayed signature → 401
- Cache full → LRU eviction (not a security failure given rate limiting)

### Client requirements
- Must generate unique requests (trivial — timestamp + body differ)
- No additional `X-Request-Id` header needed
