// Package server implements the notify-server daemon.
package server

import (
	"sync"
	"time"
)

// ReplayCache is a bounded LRU cache for detecting replayed signatures.
// It is safe for concurrent access.
type ReplayCache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	entries    map[string]time.Time // signature -> expiry time
	order      []string             // LRU order (oldest first)
}

// NewReplayCache creates a new replay cache with the given capacity and TTL.
func NewReplayCache(maxEntries int, ttl time.Duration) *ReplayCache {
	return &ReplayCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		entries:    make(map[string]time.Time),
		order:      make([]string, 0, maxEntries),
	}
}

// Contains checks if the signature is in the cache and not expired.
func (c *ReplayCache) Contains(signature string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry, exists := c.entries[signature]
	if !exists {
		return false
	}

	// Check if expired
	if time.Now().After(expiry) {
		// Lazily remove expired entry
		delete(c.entries, signature)
		c.removeFromOrder(signature)
		return false
	}

	return true
}

// Add adds a signature to the cache. If the cache is at capacity,
// the oldest entry is evicted first.
func (c *ReplayCache) Add(signature string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already exists, update expiry and move to end of order
	if _, exists := c.entries[signature]; exists {
		c.entries[signature] = time.Now().Add(c.ttl)
		c.removeFromOrder(signature)
		c.order = append(c.order, signature)
		return
	}

	// Evict expired entries first
	c.evictExpired()

	// If still at capacity, evict oldest
	for len(c.entries) >= c.maxEntries && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}

	// Add new entry
	c.entries[signature] = time.Now().Add(c.ttl)
	c.order = append(c.order, signature)
}

// evictExpired removes all expired entries. Must be called with lock held.
func (c *ReplayCache) evictExpired() {
	now := time.Now()
	newOrder := make([]string, 0, len(c.order))

	for _, sig := range c.order {
		if expiry, exists := c.entries[sig]; exists && now.Before(expiry) {
			newOrder = append(newOrder, sig)
		} else {
			delete(c.entries, sig)
		}
	}

	c.order = newOrder
}

// removeFromOrder removes a signature from the order slice.
// Must be called with lock held.
func (c *ReplayCache) removeFromOrder(signature string) {
	for i, sig := range c.order {
		if sig == signature {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

// Len returns the current number of entries in the cache.
// Useful for testing.
func (c *ReplayCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
