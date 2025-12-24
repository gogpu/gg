package cache

import "sync"

// Cache is a generic thread-safe LRU cache with soft limit.
// When the cache exceeds softLimit, oldest entries are evicted.
//
// Cache is safe for concurrent use.
// Cache must not be copied after creation (has mutex).
type Cache[K comparable, V any] struct {
	mu        sync.Mutex
	entries   map[K]*cacheEntry[V]
	softLimit int
	tick      int64 // Monotonic access counter
}

// cacheEntry holds a cached value with its access time.
type cacheEntry[V any] struct {
	value V
	atime int64 // Access time (tick value)
}

// New creates a new cache with the given soft limit.
// A softLimit of 0 means unlimited.
func New[K comparable, V any](softLimit int) *Cache[K, V] {
	return &Cache[K, V]{
		entries:   make(map[K]*cacheEntry[V]),
		softLimit: softLimit,
		tick:      0,
	}
}

// Get retrieves a value from the cache.
// Returns (value, true) if found, (zero, false) otherwise.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}

	// Update access time
	c.tick++
	entry.atime = c.tick

	return entry.value, true
}

// Set stores a value in the cache.
// If the cache exceeds softLimit after insertion, oldest entries are evicted.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++
	c.entries[key] = &cacheEntry[V]{
		value: value,
		atime: c.tick,
	}

	// Evict if over soft limit
	if c.softLimit > 0 && len(c.entries) > c.softLimit {
		c.evictOldest()
	}
}

// GetOrCreate returns cached value or creates it.
// Thread-safe: create is called under lock to prevent duplicate creation.
func (c *Cache[K, V]) GetOrCreate(key K, create func() V) V {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already in cache
	if entry, ok := c.entries[key]; ok {
		// Update access time
		c.tick++
		entry.atime = c.tick
		return entry.value
	}

	// Create new value (under lock)
	value := create()

	// Store in cache
	c.tick++
	c.entries[key] = &cacheEntry[V]{
		value: value,
		atime: c.tick,
	}

	// Evict if over soft limit
	if c.softLimit > 0 && len(c.entries) > c.softLimit {
		c.evictOldest()
	}

	return value
}

// Delete removes an entry from the cache.
// Returns true if the entry was found and removed.
func (c *Cache[K, V]) Delete(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[key]; ok {
		delete(c.entries, key)
		return true
	}
	return false
}

// Clear removes all entries from the cache.
func (c *Cache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[K]*cacheEntry[V])
	c.tick = 0
}

// Len returns the number of entries in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

// Capacity returns the soft limit of the cache.
func (c *Cache[K, V]) Capacity() int {
	return c.softLimit
}

// Stats returns cache statistics.
func (c *Cache[K, V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return Stats{
		Len:      len(c.entries),
		Capacity: c.softLimit,
	}
}

// evictOldest removes entries until under softLimit.
// Called internally when adding new entries.
// Caller must hold c.mu.
func (c *Cache[K, V]) evictOldest() {
	// Remove 25% of entries (or until under soft limit)
	targetSize := c.softLimit * 3 / 4
	if targetSize < 1 {
		targetSize = 1
	}

	// Find entries to evict
	toEvict := len(c.entries) - targetSize
	if toEvict <= 0 {
		return
	}

	// Collect entries with their access times
	type entry struct {
		key   K
		atime int64
	}
	entries := make([]entry, 0, len(c.entries))
	for key, e := range c.entries {
		entries = append(entries, entry{key: key, atime: e.atime})
	}

	// Sort by access time (oldest first)
	// Simple selection sort for eviction - good enough for small batches
	for i := 0; i < toEvict && i < len(entries); i++ {
		minIdx := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].atime < entries[minIdx].atime {
				minIdx = j
			}
		}
		if minIdx != i {
			entries[i], entries[minIdx] = entries[minIdx], entries[i]
		}
		delete(c.entries, entries[i].key)
	}
}

// Stats contains cache statistics.
type Stats struct {
	// Len is the current number of entries.
	Len int
	// Capacity is the cache capacity (soft limit, or per-shard for ShardedCache).
	Capacity int
	// TotalCapacity is the total capacity across all shards (ShardedCache only).
	TotalCapacity int
	// Hits is the number of cache hits (ShardedCache only).
	Hits uint64
	// Misses is the number of cache misses (ShardedCache only).
	Misses uint64
	// HitRate is the cache hit rate 0.0 to 1.0 (ShardedCache only).
	HitRate float64
	// Evictions is the number of evicted entries (ShardedCache only).
	Evictions uint64
}
