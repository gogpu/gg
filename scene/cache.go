package scene

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogpu/gg"
)

// Default cache configuration constants.
const (
	// DefaultMaxSizeMB is the default maximum cache size in megabytes.
	DefaultMaxSizeMB = 64
	// bytesPerMB is the number of bytes in a megabyte.
	bytesPerMB = 1024 * 1024
	// bytesPerPixel is the number of bytes per RGBA pixel.
	bytesPerPixel = 4
)

// LayerCache provides an LRU cache for rendered layer pixmaps.
// It is thread-safe and uses atomic counters for statistics.
//
// The cache evicts least recently used entries when the memory limit is exceeded.
// Cache entries are keyed by a 64-bit hash computed from the encoding content.
type LayerCache struct {
	mu      sync.RWMutex
	entries map[uint64]*CacheEntry // hash -> entry
	lru     *list.List             // LRU order (front = most recent)
	size    int64                  // Current memory usage in bytes
	maxSize int64                  // Memory budget in bytes

	// Statistics (atomic for zero-allocation reads)
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// CacheEntry represents a single cached pixmap with metadata.
type CacheEntry struct {
	hash     uint64
	pixmap   *gg.Pixmap
	size     int64 // Memory size in bytes
	element  *list.Element
	version  uint64    // Scene version when cached
	lastUsed time.Time // Time of last access
}

// CacheStats contains cache statistics for monitoring.
type CacheStats struct {
	// Size is the current memory usage in bytes.
	Size int64
	// MaxSize is the memory budget in bytes.
	MaxSize int64
	// Entries is the number of cached entries.
	Entries int
	// Hits is the number of cache hits.
	Hits uint64
	// Misses is the number of cache misses.
	Misses uint64
	// HitRate is the cache hit rate (0.0 to 1.0).
	HitRate float64
	// Evictions is the number of entries evicted.
	Evictions uint64
}

// NewLayerCache creates a new layer cache with the specified maximum size.
// The maxSizeMB parameter sets the memory budget in megabytes.
func NewLayerCache(maxSizeMB int) *LayerCache {
	if maxSizeMB <= 0 {
		maxSizeMB = DefaultMaxSizeMB
	}
	return &LayerCache{
		entries: make(map[uint64]*CacheEntry),
		lru:     list.New(),
		maxSize: int64(maxSizeMB) * bytesPerMB,
	}
}

// DefaultLayerCache creates a new layer cache with the default 64MB limit.
func DefaultLayerCache() *LayerCache {
	return NewLayerCache(DefaultMaxSizeMB)
}

// Get retrieves a cached pixmap by its hash.
// Returns the pixmap and true if found, nil and false otherwise.
// On cache hit, the entry is moved to the front of the LRU list.
func (c *LayerCache) Get(hash uint64) (*gg.Pixmap, bool) {
	c.mu.RLock()
	_, ok := c.entries[hash]
	c.mu.RUnlock()

	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	// Move to front (requires write lock)
	c.mu.Lock()
	// Re-check after acquiring write lock (entry may have been evicted)
	entry, ok := c.entries[hash]
	if !ok {
		c.mu.Unlock()
		c.misses.Add(1)
		return nil, false
	}
	c.lru.MoveToFront(entry.element)
	entry.lastUsed = time.Now()
	pixmap := entry.pixmap
	c.mu.Unlock()

	c.hits.Add(1)
	return pixmap, true
}

// Put stores a pixmap in the cache with the given hash and version.
// If the cache exceeds its memory budget, least recently used entries are evicted.
// If an entry with the same hash exists, it is replaced.
func (c *LayerCache) Put(hash uint64, pixmap *gg.Pixmap, version uint64) {
	if pixmap == nil {
		return
	}

	entrySize := pixmapSize(pixmap)
	if entrySize <= 0 {
		return
	}

	// Don't cache if single entry exceeds budget
	if entrySize > c.maxSize {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists
	if existing, ok := c.entries[hash]; ok {
		// Update existing entry
		c.size -= existing.size
		c.lru.Remove(existing.element)
	}

	// Evict entries until we have space
	c.evictUntilSize(c.maxSize - entrySize)

	// Create new entry
	entry := &CacheEntry{
		hash:     hash,
		pixmap:   pixmap,
		size:     entrySize,
		version:  version,
		lastUsed: time.Now(),
	}
	entry.element = c.lru.PushFront(entry)
	c.entries[hash] = entry
	c.size += entrySize
}

// Invalidate removes a specific entry from the cache by hash.
func (c *LayerCache) Invalidate(hash uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[hash]; ok {
		c.lru.Remove(entry.element)
		c.size -= entry.size
		delete(c.entries, hash)
		c.evictions.Add(1)
	}
}

// InvalidateAll clears the entire cache.
func (c *LayerCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	evicted := uint64(len(c.entries))
	c.entries = make(map[uint64]*CacheEntry)
	c.lru.Init()
	c.size = 0

	if evicted > 0 {
		c.evictions.Add(evicted)
	}
}

// Trim evicts entries until the cache size is at or below the target size.
// The targetSize parameter is in bytes.
func (c *LayerCache) Trim(targetSize int64) {
	if targetSize < 0 {
		targetSize = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictUntilSize(targetSize)
}

// evictUntilSize evicts LRU entries until size is at or below target.
// Must be called with c.mu held.
func (c *LayerCache) evictUntilSize(targetSize int64) {
	for c.size > targetSize && c.lru.Len() > 0 {
		// Get least recently used entry (back of list)
		elem := c.lru.Back()
		if elem == nil {
			break
		}

		entry := elem.Value.(*CacheEntry)
		c.lru.Remove(elem)
		c.size -= entry.size
		delete(c.entries, entry.hash)
		c.evictions.Add(1)
	}
}

// Stats returns current cache statistics.
// This operation is lock-free for the atomic counters.
func (c *LayerCache) Stats() CacheStats {
	c.mu.RLock()
	size := c.size
	maxSize := c.maxSize
	entries := len(c.entries)
	c.mu.RUnlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	evictions := c.evictions.Load()

	var hitRate float64
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Size:      size,
		MaxSize:   maxSize,
		Entries:   entries,
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
		Evictions: evictions,
	}
}

// Size returns the current memory usage in bytes.
func (c *LayerCache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.size
}

// MaxSize returns the memory budget in bytes.
func (c *LayerCache) MaxSize() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxSize
}

// SetMaxSize updates the memory budget.
// If the new budget is smaller than current usage, entries are evicted.
// The mb parameter is the new budget in megabytes.
func (c *LayerCache) SetMaxSize(mb int) {
	if mb <= 0 {
		mb = DefaultMaxSizeMB
	}
	newMaxSize := int64(mb) * bytesPerMB

	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxSize = newMaxSize
	c.evictUntilSize(newMaxSize)
}

// EntryCount returns the number of entries in the cache.
func (c *LayerCache) EntryCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Contains checks if an entry with the given hash exists in the cache.
// This does not update the LRU order.
func (c *LayerCache) Contains(hash uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.entries[hash]
	return ok
}

// GetVersion returns the version of a cached entry if it exists.
// Returns 0 and false if the entry is not found.
func (c *LayerCache) GetVersion(hash uint64) (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if entry, ok := c.entries[hash]; ok {
		return entry.version, true
	}
	return 0, false
}

// ResetStats resets the hit, miss, and eviction counters to zero.
func (c *LayerCache) ResetStats() {
	c.hits.Store(0)
	c.misses.Store(0)
	c.evictions.Store(0)
}

// pixmapSize calculates the memory size of a pixmap in bytes.
func pixmapSize(p *gg.Pixmap) int64 {
	if p == nil {
		return 0
	}
	return int64(p.Width()) * int64(p.Height()) * bytesPerPixel
}
