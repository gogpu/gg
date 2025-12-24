package cache

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

// Default configuration constants.
const (
	// DefaultShardCount is the number of shards for reduced lock contention.
	// Must be a power of 2 for fast modulo via bitwise AND.
	DefaultShardCount = 16

	// DefaultCapacity is the default maximum entries per shard.
	DefaultCapacity = 256

	// shardMask is used for fast shard selection (DefaultShardCount - 1).
	shardMask = DefaultShardCount - 1
)

// Hasher is a function that computes a hash for a key.
// Used by ShardedCache for shard selection.
type Hasher[K any] func(K) uint64

// StringHasher computes FNV-1a hash of a string key.
func StringHasher(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s)) // fnv.Write never returns an error
	return h.Sum64()
}

// IntHasher computes a hash of an int key using FNV-1a.
func IntHasher(i int) uint64 {
	h := fnv.New64a()
	buf := make([]byte, 8)
	buf[0] = byte(i)
	buf[1] = byte(i >> 8)
	buf[2] = byte(i >> 16)
	buf[3] = byte(i >> 24)
	buf[4] = byte(i >> 32)
	buf[5] = byte(i >> 40)
	buf[6] = byte(i >> 48)
	buf[7] = byte(i >> 56)
	_, _ = h.Write(buf)
	return h.Sum64()
}

// Uint64Hasher returns the key itself as the hash (identity hash).
func Uint64Hasher(u uint64) uint64 {
	return u
}

// ShardedCache is a thread-safe, sharded LRU cache for high-concurrency scenarios.
//
// Features:
//   - 16 shards for reduced lock contention
//   - LRU eviction with configurable capacity per shard
//   - Thread-safe for concurrent access
//   - Atomic statistics for monitoring
//   - Zero allocations on cache hit
//
// Performance (Intel i7-1255U):
//   - Cache hit: ~75ns
//   - Cache miss: ~35ns
//   - Parallel: ~170ns/op
type ShardedCache[K comparable, V any] struct {
	shards   [DefaultShardCount]*shardedCacheShard[K, V]
	hasher   Hasher[K]
	capacity int // Per-shard capacity

	// Statistics (atomic for zero-allocation reads)
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// shardedCacheShard is a single shard of the cache.
// Each shard has its own mutex for reduced contention.
type shardedCacheShard[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]*shardedCacheEntry[K, V]
	lru     *lruList[K]
}

// shardedCacheEntry holds a cached value with its LRU node.
type shardedCacheEntry[K comparable, V any] struct {
	value V
	node  *lruNode[K]
}

// NewSharded creates a new sharded cache with the specified capacity per shard.
// Total capacity is approximately capacity * DefaultShardCount (16).
//
// The hasher function is used to compute hash values for shard selection.
// Use StringHasher, IntHasher, or Uint64Hasher for common key types.
//
// If capacity <= 0, DefaultCapacity (256) is used.
func NewSharded[K comparable, V any](capacity int, hasher Hasher[K]) *ShardedCache[K, V] {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	c := &ShardedCache[K, V]{
		hasher:   hasher,
		capacity: capacity,
	}

	for i := range c.shards {
		c.shards[i] = &shardedCacheShard[K, V]{
			entries: make(map[K]*shardedCacheEntry[K, V]),
			lru:     newLRUList[K](),
		}
	}

	return c
}

// getShard returns the shard for a given key.
// Uses bitwise AND for fast modulo (only works with power-of-2 shard count).
func (c *ShardedCache[K, V]) getShard(key K) *shardedCacheShard[K, V] {
	hash := c.hasher(key)
	return c.shards[hash&shardMask]
}

// Get retrieves a cached value by key.
// Returns (value, true) if found, (zero, false) otherwise.
//
// On cache hit, the entry is moved to the front of the LRU list.
// This operation is thread-safe and optimized for minimal lock contention.
func (c *ShardedCache[K, V]) Get(key K) (V, bool) {
	shard := c.getShard(key)

	// Fast path: read lock to check existence
	shard.mu.RLock()
	_, exists := shard.entries[key]
	shard.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		var zero V
		return zero, false
	}

	// Slow path: write lock for LRU update
	shard.mu.Lock()
	// Re-check after acquiring write lock (entry may have been evicted)
	entry, ok := shard.entries[key]
	if !ok {
		shard.mu.Unlock()
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	shard.lru.MoveToFront(entry.node)
	value := entry.value
	shard.mu.Unlock()

	c.hits.Add(1)
	return value, true
}

// Set stores a value in the cache.
// If the shard exceeds capacity after insertion, oldest entries are evicted.
//
// The value is stored as-is (not copied). Callers should not modify it
// after caching.
func (c *ShardedCache[K, V]) Set(key K, value V) {
	shard := c.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check if entry already exists
	if existing, ok := shard.entries[key]; ok {
		// Update existing entry
		existing.value = value
		shard.lru.MoveToFront(existing.node)
		return
	}

	// Evict if at capacity
	for shard.lru.Len() >= c.capacity {
		if oldest, ok := shard.lru.RemoveOldest(); ok {
			delete(shard.entries, oldest)
			c.evictions.Add(1)
		} else {
			break
		}
	}

	// Add new entry
	node := shard.lru.PushFront(key)
	shard.entries[key] = &shardedCacheEntry[K, V]{
		value: value,
		node:  node,
	}
}

// GetOrCreate returns a cached value or creates it using the provided function.
// This is the preferred method for cache access as it prevents duplicate computation.
//
// The create function is called with the shard lock held to prevent thundering herd.
// Keep the create function fast to minimize lock contention.
func (c *ShardedCache[K, V]) GetOrCreate(key K, create func() V) V {
	shard := c.getShard(key)

	// Fast path: read lock to check existence
	shard.mu.RLock()
	_, exists := shard.entries[key]
	shard.mu.RUnlock()

	if exists {
		// Update LRU (requires write lock)
		shard.mu.Lock()
		if entry, ok := shard.entries[key]; ok {
			shard.lru.MoveToFront(entry.node)
			value := entry.value
			shard.mu.Unlock()
			c.hits.Add(1)
			return value
		}
		shard.mu.Unlock()
	}

	// Slow path: create new entry
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Re-check after acquiring write lock
	if entry, ok := shard.entries[key]; ok {
		shard.lru.MoveToFront(entry.node)
		c.hits.Add(1)
		return entry.value
	}

	c.misses.Add(1)

	// Create new value (under lock)
	value := create()

	// Evict if at capacity
	for shard.lru.Len() >= c.capacity {
		if oldest, ok := shard.lru.RemoveOldest(); ok {
			delete(shard.entries, oldest)
			c.evictions.Add(1)
		} else {
			break
		}
	}

	// Add new entry
	node := shard.lru.PushFront(key)
	shard.entries[key] = &shardedCacheEntry[K, V]{
		value: value,
		node:  node,
	}

	return value
}

// Delete removes an entry from the cache.
// Returns true if the entry was found and removed.
func (c *ShardedCache[K, V]) Delete(key K) bool {
	shard := c.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	entry, ok := shard.entries[key]
	if !ok {
		return false
	}

	shard.lru.Remove(entry.node)
	delete(shard.entries, key)
	return true
}

// Clear removes all entries from the cache.
func (c *ShardedCache[K, V]) Clear() {
	for _, shard := range c.shards {
		shard.mu.Lock()
		shard.entries = make(map[K]*shardedCacheEntry[K, V])
		shard.lru.Clear()
		shard.mu.Unlock()
	}
}

// Len returns the total number of entries across all shards.
func (c *ShardedCache[K, V]) Len() int {
	total := 0
	for _, shard := range c.shards {
		shard.mu.RLock()
		total += len(shard.entries)
		shard.mu.RUnlock()
	}
	return total
}

// Capacity returns the per-shard capacity.
func (c *ShardedCache[K, V]) Capacity() int {
	return c.capacity
}

// TotalCapacity returns the total capacity across all shards.
func (c *ShardedCache[K, V]) TotalCapacity() int {
	return c.capacity * DefaultShardCount
}

// ShardLen returns the number of entries in each shard.
// Useful for debugging load distribution.
func (c *ShardedCache[K, V]) ShardLen() [DefaultShardCount]int {
	var lens [DefaultShardCount]int
	for i, shard := range c.shards {
		shard.mu.RLock()
		lens[i] = len(shard.entries)
		shard.mu.RUnlock()
	}
	return lens
}

// Stats returns current cache statistics.
// This operation is mostly lock-free (atomic counters).
func (c *ShardedCache[K, V]) Stats() Stats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	evictions := c.evictions.Load()

	var hitRate float64
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return Stats{
		Len:           c.Len(),
		Capacity:      c.capacity,
		TotalCapacity: c.capacity * DefaultShardCount,
		Hits:          hits,
		Misses:        misses,
		HitRate:       hitRate,
		Evictions:     evictions,
	}
}

// ResetStats resets all statistics counters to zero.
func (c *ShardedCache[K, V]) ResetStats() {
	c.hits.Store(0)
	c.misses.Store(0)
	c.evictions.Store(0)
}
