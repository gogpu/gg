package cache

import (
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg/text"
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

// ShapingKey identifies shaped text in the shaping cache.
// All shaping parameters that affect the result must be included.
type ShapingKey struct {
	// TextHash is FNV-1a hash of the text string.
	TextHash uint64

	// FontID is the font source identifier.
	FontID uint64

	// SizeBits is the IEEE 754 bit pattern of the font size (float32).
	// Using bit pattern ensures exact matching without floating-point issues.
	SizeBits uint32

	// Direction is the text direction (LTR, RTL, TTB, BTT).
	Direction uint8

	// Features is a hash of OpenType feature settings.
	Features uint64
}

// NewShapingKey creates a ShapingKey from shaping parameters.
// This is the canonical way to create cache keys.
func NewShapingKey(textStr string, fontID uint64, size float32, direction text.Direction, features uint64) ShapingKey {
	return ShapingKey{
		TextHash:  hashString(textStr),
		FontID:    fontID,
		SizeBits:  math.Float32bits(size),
		Direction: uint8(direction & 0xFF), //nolint:gosec // Direction enum is < 4
		Features:  features,
	}
}

// hashString computes FNV-1a hash of a string.
// FNV-1a is fast and has good distribution for text keys.
func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s)) // fnv.Write never returns an error
	return h.Sum64()
}

// keyHash computes a hash of the ShapingKey for shard selection.
// Uses FNV-1a with all key fields.
func (k *ShapingKey) keyHash() uint64 {
	h := fnv.New64a()
	// Write all fields as bytes
	buf := make([]byte, 8+8+4+1+8) // 29 bytes total
	// TextHash (8 bytes)
	buf[0] = byte(k.TextHash)
	buf[1] = byte(k.TextHash >> 8)
	buf[2] = byte(k.TextHash >> 16)
	buf[3] = byte(k.TextHash >> 24)
	buf[4] = byte(k.TextHash >> 32)
	buf[5] = byte(k.TextHash >> 40)
	buf[6] = byte(k.TextHash >> 48)
	buf[7] = byte(k.TextHash >> 56)
	// FontID (8 bytes)
	buf[8] = byte(k.FontID)
	buf[9] = byte(k.FontID >> 8)
	buf[10] = byte(k.FontID >> 16)
	buf[11] = byte(k.FontID >> 24)
	buf[12] = byte(k.FontID >> 32)
	buf[13] = byte(k.FontID >> 40)
	buf[14] = byte(k.FontID >> 48)
	buf[15] = byte(k.FontID >> 56)
	// SizeBits (4 bytes)
	buf[16] = byte(k.SizeBits)
	buf[17] = byte(k.SizeBits >> 8)
	buf[18] = byte(k.SizeBits >> 16)
	buf[19] = byte(k.SizeBits >> 24)
	// Direction (1 byte)
	buf[20] = k.Direction
	// Features (8 bytes)
	buf[21] = byte(k.Features)
	buf[22] = byte(k.Features >> 8)
	buf[23] = byte(k.Features >> 16)
	buf[24] = byte(k.Features >> 24)
	buf[25] = byte(k.Features >> 32)
	buf[26] = byte(k.Features >> 40)
	buf[27] = byte(k.Features >> 48)
	buf[28] = byte(k.Features >> 56)

	_, _ = h.Write(buf)
	return h.Sum64()
}

// ShapingCache is a thread-safe, sharded LRU cache for shaped text runs.
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
type ShapingCache struct {
	shards   [DefaultShardCount]*cacheShard
	capacity int // Per-shard capacity

	// Statistics (atomic for zero-allocation reads)
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// cacheShard is a single shard of the cache.
// Each shard has its own mutex for reduced contention.
type cacheShard struct {
	mu      sync.RWMutex
	entries map[ShapingKey]*cacheEntry
	lru     *lruList[ShapingKey]
}

// cacheEntry holds a cached ShapedRun with its LRU node.
type cacheEntry struct {
	value *text.ShapedRun
	node  *lruNode[ShapingKey]
}

// NewShapingCache creates a new shaping cache with the specified capacity per shard.
// Total capacity is approximately capacity * DefaultShardCount (16).
//
// If capacity <= 0, DefaultCapacity (256) is used.
func NewShapingCache(capacity int) *ShapingCache {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	c := &ShapingCache{
		capacity: capacity,
	}

	for i := range c.shards {
		c.shards[i] = &cacheShard{
			entries: make(map[ShapingKey]*cacheEntry),
			lru:     newLRUList[ShapingKey](),
		}
	}

	return c
}

// DefaultShapingCache creates a shaping cache with default configuration.
// Total capacity: 16 shards * 256 entries = 4096 entries.
func DefaultShapingCache() *ShapingCache {
	return NewShapingCache(DefaultCapacity)
}

// getShard returns the shard for a given key.
// Uses bitwise AND for fast modulo (only works with power-of-2 shard count).
func (c *ShapingCache) getShard(key *ShapingKey) *cacheShard {
	hash := key.keyHash()
	return c.shards[hash&shardMask]
}

// Get retrieves a cached ShapedRun by key.
// Returns (value, true) if found, (nil, false) otherwise.
//
// On cache hit, the entry is moved to the front of the LRU list.
// This operation is thread-safe and optimized for minimal lock contention.
func (c *ShapingCache) Get(key ShapingKey) (*text.ShapedRun, bool) {
	shard := c.getShard(&key)

	// Fast path: read lock to check existence
	shard.mu.RLock()
	_, exists := shard.entries[key]
	shard.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		return nil, false
	}

	// Slow path: write lock for LRU update
	shard.mu.Lock()
	// Re-check after acquiring write lock (entry may have been evicted)
	entry, ok := shard.entries[key]
	if !ok {
		shard.mu.Unlock()
		c.misses.Add(1)
		return nil, false
	}
	shard.lru.MoveToFront(entry.node)
	value := entry.value
	shard.mu.Unlock()

	c.hits.Add(1)
	return value, true
}

// Set stores a ShapedRun in the cache.
// If the shard exceeds capacity after insertion, oldest entries are evicted.
//
// The value is stored as-is (not copied). Callers should not modify it
// after caching.
func (c *ShapingCache) Set(key ShapingKey, value *text.ShapedRun) {
	if value == nil {
		return
	}

	shard := c.getShard(&key)

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
	shard.entries[key] = &cacheEntry{
		value: value,
		node:  node,
	}
}

// GetOrCreate returns a cached ShapedRun or creates it using the provided function.
// This is the preferred method for cache access as it prevents duplicate computation.
//
// The create function is called with the shard lock held to prevent thundering herd.
// Keep the create function fast to minimize lock contention.
func (c *ShapingCache) GetOrCreate(key ShapingKey, create func() *text.ShapedRun) *text.ShapedRun {
	shard := c.getShard(&key)

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
	if value == nil {
		return nil
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
	shard.entries[key] = &cacheEntry{
		value: value,
		node:  node,
	}

	return value
}

// Delete removes an entry from the cache.
// Returns true if the entry was found and removed.
func (c *ShapingCache) Delete(key ShapingKey) bool {
	shard := c.getShard(&key)

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
func (c *ShapingCache) Clear() {
	for _, shard := range c.shards {
		shard.mu.Lock()
		shard.entries = make(map[ShapingKey]*cacheEntry)
		shard.lru.Clear()
		shard.mu.Unlock()
	}
}

// Len returns the total number of entries across all shards.
func (c *ShapingCache) Len() int {
	total := 0
	for _, shard := range c.shards {
		shard.mu.RLock()
		total += len(shard.entries)
		shard.mu.RUnlock()
	}
	return total
}

// Capacity returns the per-shard capacity.
func (c *ShapingCache) Capacity() int {
	return c.capacity
}

// TotalCapacity returns the total capacity across all shards.
func (c *ShapingCache) TotalCapacity() int {
	return c.capacity * DefaultShardCount
}

// ShardLen returns the number of entries in each shard.
// Useful for debugging load distribution.
func (c *ShapingCache) ShardLen() [DefaultShardCount]int {
	var lens [DefaultShardCount]int
	for i, shard := range c.shards {
		shard.mu.RLock()
		lens[i] = len(shard.entries)
		shard.mu.RUnlock()
	}
	return lens
}

// CacheStats contains cache statistics for monitoring.
type CacheStats struct {
	// Len is the current number of entries.
	Len int
	// Capacity is the per-shard capacity.
	Capacity int
	// TotalCapacity is the total capacity across all shards.
	TotalCapacity int
	// Hits is the number of cache hits.
	Hits uint64
	// Misses is the number of cache misses.
	Misses uint64
	// HitRate is the cache hit rate (0.0 to 1.0).
	HitRate float64
	// Evictions is the number of entries evicted.
	Evictions uint64
}

// Stats returns current cache statistics.
// This operation is mostly lock-free (atomic counters).
func (c *ShapingCache) Stats() CacheStats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	evictions := c.evictions.Load()

	var hitRate float64
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
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
func (c *ShapingCache) ResetStats() {
	c.hits.Store(0)
	c.misses.Store(0)
	c.evictions.Store(0)
}

// HashFeatures computes a hash of OpenType feature settings.
// This is a helper function for creating ShapingKey.
//
// Features should be passed as tag/value pairs, e.g.:
//
//	HashFeatures(map[string]int{"liga": 1, "kern": 1})
func HashFeatures(features map[string]int) uint64 {
	if len(features) == 0 {
		return 0
	}

	h := fnv.New64a()
	// Sort-independent hashing: XOR individual feature hashes
	var result uint64
	for tag, val := range features {
		h.Reset()
		_, _ = h.Write([]byte(tag))
		tagHash := h.Sum64()
		// Combine tag hash with value (val is always small for feature values)
		result ^= tagHash ^ uint64(val) //nolint:gosec // feature values are small integers
	}
	return result
}
