# ADR-005: Use net/http stdlib for HTTP server

## Status
Accepted

## Context
We need an HTTP server for a single endpoint: `POST /notify`.

Options considered: net/http (stdlib), chi, gin, echo, fiber

## Decision
**net/http (stdlib)**

## Rationale

- One endpoint, one method — frameworks add no value
- Zero dependencies
- Production-grade (powers much of the internet)
- Middleware is just function composition — no abstractions needed
- Full control, no magic

## Consequences

- Handler registered with `http.HandleFunc`
- Auth, rate limiting, body limits implemented as handler wrappers
- No routing library to learn or maintain

### Request validation

**Content-Type:** Require `Content-Type: application/json` for POST /notify.
- Missing or wrong Content-Type → 415 Unsupported Media Type
- Defense in depth; helps catch misconfigured clients early

### Error response format

All error responses return **status code only, empty body**:
- 400 Bad Request — invalid JSON or missing required fields
- 401 Unauthorized — auth failure (bad signature, stale timestamp, replay, source mismatch)
- 405 Method Not Allowed — wrong HTTP method
- 413 Payload Too Large — body exceeds max_body_bytes
- 415 Unsupported Media Type — missing/wrong Content-Type
- 429 Too Many Requests — rate limited

Rationale:
- Clients only need status codes to decide retry behavior
- No information leakage in error bodies
- Simpler implementation

Success response:
- 204 No Content — empty body (sound plays async)
