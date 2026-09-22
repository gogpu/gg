package text

import (
	"container/list"
	"math"
	"sync"
)

const (
	glyphMaskCacheLimit = 8192
	// Bounds apply per font. Entry metadata is bounded separately by count.
	glyphMaskCacheBytes = 4 << 20
	glyphMaskMaxBytes   = 64 << 10
)

// glyphMaskKey uses exact offsets: caching must not change rasterization.
// Font identity is implicit because each parsed font owns its cache.
type glyphMaskKey struct {
	gid                  GlyphID
	ppem                 float64
	subpixelX, subpixelY float64
	hinting              Hinting
	mode                 glyphRasterMode
}

type cpuGlyphMaskEntry struct {
	key    glyphMaskKey
	result *GlyphMaskResult
	bytes  int
}

// glyphMaskCache stores immutable CPU masks in LRU order. The zero value is
// ready to use. It has no global references and dies with its owning font.
type glyphMaskCache struct {
	mu      sync.Mutex
	entries map[glyphMaskKey]*list.Element
	lru     list.List
	bytes   int
	closed  bool
}

// get rasterizes misses outside the lock. Nil outlines are cached; errors and
// large masks are not. Concurrent misses may rasterize twice, but only one
// result is retained and charged to the byte budget.
func (c *glyphMaskCache) get(key glyphMaskKey, rasterize func() (*GlyphMaskResult, error)) (*GlyphMaskResult, error) {
	// NaN keys cannot be found or deleted from a map, even with the same key.
	if math.IsNaN(key.ppem) || math.IsNaN(key.subpixelX) || math.IsNaN(key.subpixelY) {
		return rasterize()
	}
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		c.lru.MoveToFront(e)
		result := e.Value.(cpuGlyphMaskEntry).result
		c.mu.Unlock()
		return result, nil
	}
	closed := c.closed
	c.mu.Unlock()

	result, err := rasterize()
	if err != nil || closed {
		return result, err
	}
	size := 0
	if result != nil {
		size = cap(result.Mask)
	}
	if size > glyphMaskMaxBytes {
		return result, nil
	}
	return c.store(key, result, size), nil
}

func (c *glyphMaskCache) store(key glyphMaskKey, result *GlyphMaskResult, size int) *GlyphMaskResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Close may have run while this miss was being rasterized.
	if c.closed {
		return result
	}
	if e, ok := c.entries[key]; ok {
		c.lru.MoveToFront(e)
		return e.Value.(cpuGlyphMaskEntry).result
	}
	if c.entries == nil {
		c.entries = make(map[glyphMaskKey]*list.Element)
	}
	for len(c.entries) >= glyphMaskCacheLimit || c.bytes+size > glyphMaskCacheBytes {
		e := c.lru.Back()
		entry := e.Value.(cpuGlyphMaskEntry)
		delete(c.entries, entry.key)
		c.bytes -= entry.bytes
		c.lru.Remove(e)
	}
	c.entries[key] = c.lru.PushFront(cpuGlyphMaskEntry{key, result, size})
	c.bytes += size
	return result
}

// close releases masks immediately and prevents in-flight misses from
// repopulating a font that FontSource.Close has invalidated.
func (c *glyphMaskCache) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	c.lru.Init()
	c.bytes = 0
	c.closed = true
}
