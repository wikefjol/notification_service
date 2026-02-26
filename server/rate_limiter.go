// Package server implements the notify-server daemon.
package server

import (
	"sync"
	"time"
)

// RateLimiter implements per-key-id token bucket rate limiting.
type RateLimiter struct {
	mu sync.Mutex

	// ratePerMinute is how many tokens are added per minute.
	ratePerMinute float64

	// burst is the maximum tokens (bucket capacity).
	burst float64

	// buckets holds per-key-id token state.
	buckets map[string]*bucket
}

// bucket holds the token state for a single key-id.
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter creates a new RateLimiter with the given rate and burst parameters.
func NewRateLimiter(ratePerMinute, burst int) *RateLimiter {
	return &RateLimiter{
		ratePerMinute: float64(ratePerMinute),
		burst:         float64(burst),
		buckets:       make(map[string]*bucket),
	}
}

// Allow checks if a request from keyID should be allowed.
// Returns true if allowed (consumes a token), false if rate limited.
func (r *RateLimiter) Allow(keyID string) bool {
	return r.AllowAt(keyID, time.Now())
}

// AllowAt checks if a request from keyID should be allowed at the given time.
// This is useful for testing with controlled time.
func (r *RateLimiter) AllowAt(keyID string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, exists := r.buckets[keyID]
	if !exists {
		// New key-id: start with full bucket minus one token (for this request)
		r.buckets[keyID] = &bucket{
			tokens:     r.burst - 1,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill)
	tokensToAdd := (elapsed.Seconds() / 60.0) * r.ratePerMinute
	b.tokens += tokensToAdd
	if b.tokens > r.burst {
		b.tokens = r.burst
	}
	b.lastRefill = now

	// Check if we have a token to consume
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}
