package main

import (
	"sync"
	"time"
)

// CacheEntry represents a cached value with expiration
type CacheEntry struct {
	Value      interface{}
	Expiration time.Time
}

// SimpleCache is a thread-safe in-memory cache
type SimpleCache struct {
	mu    sync.RWMutex
	items map[string]*CacheEntry
}

// NewSimpleCache creates a new cache
func NewSimpleCache() *SimpleCache {
	cache := &SimpleCache{
		items: make(map[string]*CacheEntry),
	}
	// Cleanup expired entries every minute
	go cache.cleanupLoop()
	return cache
}

// Get retrieves a value from cache
func (c *SimpleCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, found := c.items[key]
	if !found {
		return nil, false
	}
	
	if time.Now().After(entry.Expiration) {
		return nil, false
	}
	
	return entry.Value, true
}

// Set stores a value in cache with TTL
func (c *SimpleCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items[key] = &CacheEntry{
		Value:      value,
		Expiration: time.Now().Add(ttl),
	}
}

// Delete removes a value from cache
func (c *SimpleCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all items from cache
func (c *SimpleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*CacheEntry)
}

// cleanupLoop removes expired entries periodically
func (c *SimpleCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.items {
			if now.After(entry.Expiration) {
				delete(c.items, key)
			}
		}
		c.mu.Unlock()
	}
}
