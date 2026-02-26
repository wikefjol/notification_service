package server

import (
	"testing"
	"time"
)

func TestRateLimiter_BurstAllowed(t *testing.T) {
	// rate: 10/min, burst: 3
	rl := NewRateLimiter(10, 3)
	now := time.Now()

	// First 3 requests should be allowed (burst)
	for i := 0; i < 3; i++ {
		if !rl.AllowAt("test-key", now) {
			t.Errorf("request %d should be allowed (within burst)", i+1)
		}
	}

	// 4th request should be denied
	if rl.AllowAt("test-key", now) {
		t.Error("request 4 should be denied (burst exhausted)")
	}
}

func TestRateLimiter_RefillOverTime(t *testing.T) {
	// rate: 10/min = 1 token per 6 seconds, burst: 3
	rl := NewRateLimiter(10, 3)
	now := time.Now()

	// Exhaust burst
	for i := 0; i < 3; i++ {
		rl.AllowAt("test-key", now)
	}

	// Immediately after, should be denied
	if rl.AllowAt("test-key", now) {
		t.Error("should be denied immediately after burst exhausted")
	}

	// After 6 seconds, should have 1 token
	later := now.Add(6 * time.Second)
	if !rl.AllowAt("test-key", later) {
		t.Error("should be allowed after 6 seconds (1 token refilled)")
	}

	// Immediately after consuming that token, denied again
	if rl.AllowAt("test-key", later) {
		t.Error("should be denied after consuming refilled token")
	}
}

func TestRateLimiter_BurstCap(t *testing.T) {
	// rate: 10/min, burst: 3
	rl := NewRateLimiter(10, 3)
	now := time.Now()

	// Use 1 token
	rl.AllowAt("test-key", now)

	// Wait a long time (should cap at burst, not accumulate beyond)
	later := now.Add(10 * time.Minute)

	// Should be able to use exactly burst tokens
	for i := 0; i < 3; i++ {
		if !rl.AllowAt("test-key", later) {
			t.Errorf("request %d should be allowed (burst refilled)", i+1)
		}
	}

	// 4th should fail
	if rl.AllowAt("test-key", later) {
		t.Error("should be denied (burst exhausted, tokens should not exceed burst)")
	}
}

func TestRateLimiter_PerKeyIsolation(t *testing.T) {
	rl := NewRateLimiter(10, 3)
	now := time.Now()

	// Exhaust key1's burst
	for i := 0; i < 3; i++ {
		rl.AllowAt("key1", now)
	}
	if rl.AllowAt("key1", now) {
		t.Error("key1 should be rate limited")
	}

	// key2 should still have full burst
	for i := 0; i < 3; i++ {
		if !rl.AllowAt("key2", now) {
			t.Errorf("key2 request %d should be allowed (independent bucket)", i+1)
		}
	}
}

func TestRateLimiter_SustainedRate(t *testing.T) {
	// rate: 60/min = 1 per second, burst: 1
	rl := NewRateLimiter(60, 1)
	now := time.Now()

	// First request allowed
	if !rl.AllowAt("test-key", now) {
		t.Error("first request should be allowed")
	}

	// Immediately denied
	if rl.AllowAt("test-key", now) {
		t.Error("immediate second request should be denied")
	}

	// 1 second later, allowed
	if !rl.AllowAt("test-key", now.Add(1*time.Second)) {
		t.Error("request after 1 second should be allowed")
	}

	// Sustained rate: 10 requests over 10 seconds should all succeed
	for i := 0; i < 10; i++ {
		requestTime := now.Add(time.Duration(i+2) * time.Second)
		if !rl.AllowAt("test-key", requestTime) {
			t.Errorf("sustained request %d at t+%ds should be allowed", i+1, i+2)
		}
	}
}
