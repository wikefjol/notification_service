# ADR-001: Use Go for implementation

## Status
Accepted

## Context
We need to choose a language for a hardened webhook receiver daemon (~500 lines) that:
- Runs as a launchd service on macOS
- Must be security-focused (auth, rate limiting, input validation)
- Will be implemented by coding agents, reviewed by a human
- Will be exposed to public internet via tunnel

Options considered: Go, Python, Rust

## Decision
**Go**

## Rationale

### Deployment simplicity
- Single static binary, no runtime dependencies
- Trivial launchd integration — just run the executable
- No venv, pip, or Python version management

### Security review
- Explicit error handling (`if err != nil`) makes failure paths visible
- No framework magic (decorators, metaclasses) hiding behavior
- Stdlib covers HTTP, HMAC, JSON — minimal third-party dependencies
- Strongly typed; compiler catches errors before review

### LLM code generation
- Claude produces "the most idiomatic, maintainable Go code" per benchmarks
- Go's boilerplate-heavy style suits LLMs well
- Compiler acts as first-pass reviewer, catching type errors before human review
- Simpler language surface than Rust, fewer ways to write incorrect code

### Tradeoffs accepted
- More verbose than Python
- Slower to prototype than Python
- Less training data than Python (but sufficient for this scope)

## Consequences
- Server and client will be Go binaries
- Minimal external dependencies (see ADR-003 for TOML parser)
- Build with `go build`, test with `go test`
