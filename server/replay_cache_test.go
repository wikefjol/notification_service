package server

import (
	"sync"
	"testing"
	"time"
)

func TestReplayCache_AddAndContains(t *testing.T) {
	cache := NewReplayCache(100, time.Minute)

	// Initially empty
	if cache.Contains("sig1") {
		t.Error("expected empty cache to not contain sig1")
	}

	// Add and check
	cache.Add("sig1")
	if !cache.Contains("sig1") {
		t.Error("expected cache to contain sig1 after adding")
	}

	// Different signature not present
	if cache.Contains("sig2") {
		t.Error("expected cache to not contain sig2")
	}

	// Add another
	cache.Add("sig2")
	if !cache.Contains("sig1") {
		t.Error("expected cache to still contain sig1")
	}
	if !cache.Contains("sig2") {
		t.Error("expected cache to contain sig2")
	}
}

func TestReplayCache_Expiry(t *testing.T) {
	// Short TTL for testing
	ttl := 50 * time.Millisecond
	cache := NewReplayCache(100, ttl)

	cache.Add("sig1")
	if !cache.Contains("sig1") {
		t.Error("expected cache to contain sig1 immediately after adding")
	}

	// Wait for expiry
	time.Sleep(ttl + 10*time.Millisecond)

	// Should be expired now
	if cache.Contains("sig1") {
		t.Error("expected sig1 to be expired")
	}
}

func TestReplayCache_LRUEviction(t *testing.T) {
	cache := NewReplayCache(3, time.Minute)

	cache.Add("sig1")
	cache.Add("sig2")
	cache.Add("sig3")

	// All three should be present
	if !cache.Contains("sig1") || !cache.Contains("sig2") || !cache.Contains("sig3") {
		t.Error("expected all three signatures to be present")
	}

	// Add a fourth - sig1 (oldest) should be evicted
	cache.Add("sig4")

	if cache.Contains("sig1") {
		t.Error("expected sig1 to be evicted (LRU)")
	}
	if !cache.Contains("sig2") || !cache.Contains("sig3") || !cache.Contains("sig4") {
		t.Error("expected sig2, sig3, sig4 to be present")
	}
}

func TestReplayCache_MaxEntries(t *testing.T) {
	maxEntries := 5
	cache := NewReplayCache(maxEntries, time.Minute)

	// Add more entries than max
	for i := 0; i < 10; i++ {
		cache.Add("sig" + string(rune('0'+i)))
	}

	// Cache should not exceed max entries
	if cache.Len() > maxEntries {
		t.Errorf("cache size %d exceeds max %d", cache.Len(), maxEntries)
	}
}

func TestReplayCache_DuplicateAdd(t *testing.T) {
	cache := NewReplayCache(3, time.Minute)

	cache.Add("sig1")
	cache.Add("sig2")
	cache.Add("sig1") // Duplicate - should update, not add new

	if cache.Len() != 2 {
		t.Errorf("expected 2 entries after duplicate add, got %d", cache.Len())
	}

	// sig1 should be moved to end (most recent)
	cache.Add("sig3")
	cache.Add("sig4") // Should evict sig2 (now oldest), not sig1

	if cache.Contains("sig2") {
		t.Error("expected sig2 to be evicted")
	}
	if !cache.Contains("sig1") {
		t.Error("expected sig1 to still be present (was refreshed)")
	}
}

func TestReplayCache_ConcurrentAccess(t *testing.T) {
	cache := NewReplayCache(1000, time.Minute)

	var wg sync.WaitGroup
	numGoroutines := 10
	numOps := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				sig := "sig" + string(rune('A'+id)) + string(rune('0'+j%10))
				cache.Add(sig)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				sig := "sig" + string(rune('A'+id)) + string(rune('0'+j%10))
				cache.Contains(sig)
			}
		}(i)
	}

	wg.Wait()

	// If we got here without deadlock or panic, the test passes
	// Just verify cache is in valid state
	if cache.Len() > 1000 {
		t.Errorf("cache exceeded max size: %d", cache.Len())
	}
}

func TestReplayCache_ExpiredEvictedOnAdd(t *testing.T) {
	ttl := 50 * time.Millisecond
	cache := NewReplayCache(5, ttl)

	// Fill cache
	cache.Add("sig1")
	cache.Add("sig2")
	cache.Add("sig3")

	// Wait for expiry
	time.Sleep(ttl + 10*time.Millisecond)

	// Add new entries - expired ones should be cleaned up first
	cache.Add("sig4")
	cache.Add("sig5")

	// Only the new non-expired entries should remain
	if cache.Len() != 2 {
		t.Errorf("expected 2 entries after expired cleanup, got %d", cache.Len())
	}

	if !cache.Contains("sig4") || !cache.Contains("sig5") {
		t.Error("expected new entries to be present")
	}
}
