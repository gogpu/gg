package msdf

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg/text"
)

// AtlasConfig holds atlas configuration.
type AtlasConfig struct {
	// Size is the atlas texture size (width = height).
	// Must be power of 2. Default: 1024
	Size int

	// GlyphSize is the size of each glyph cell.
	// Default: 32
	GlyphSize int

	// Padding between glyphs to prevent bleeding.
	// Default: 2
	Padding int

	// MaxAtlases limits the number of atlases.
	// Default: 8
	MaxAtlases int
}

// DefaultAtlasConfig returns default configuration.
func DefaultAtlasConfig() AtlasConfig {
	return AtlasConfig{
		Size:       1024,
		GlyphSize:  32,
		Padding:    2,
		MaxAtlases: 8,
	}
}

// Validate checks if the configuration is valid.
func (c *AtlasConfig) Validate() error {
	if c.Size < 64 {
		return &AtlasConfigError{Field: "Size", Reason: "must be at least 64"}
	}
	if c.Size > 8192 {
		return &AtlasConfigError{Field: "Size", Reason: "must be at most 8192"}
	}
	// Check power of 2
	if c.Size&(c.Size-1) != 0 {
		return &AtlasConfigError{Field: "Size", Reason: "must be power of 2"}
	}
	if c.GlyphSize < 8 {
		return &AtlasConfigError{Field: "GlyphSize", Reason: "must be at least 8"}
	}
	if c.GlyphSize > c.Size {
		return &AtlasConfigError{Field: "GlyphSize", Reason: "must be at most Size"}
	}
	if c.Padding < 0 {
		return &AtlasConfigError{Field: "Padding", Reason: "must be non-negative"}
	}
	if c.Padding >= c.GlyphSize/2 {
		return &AtlasConfigError{Field: "Padding", Reason: "must be less than half GlyphSize"}
	}
	if c.MaxAtlases < 1 {
		return &AtlasConfigError{Field: "MaxAtlases", Reason: "must be at least 1"}
	}
	if c.MaxAtlases > 256 {
		return &AtlasConfigError{Field: "MaxAtlases", Reason: "must be at most 256"}
	}
	return nil
}

// AtlasConfigError represents a configuration validation error.
type AtlasConfigError struct {
	Field  string
	Reason string
}

func (e *AtlasConfigError) Error() string {
	return "msdf: invalid atlas config." + e.Field + ": " + e.Reason
}

// Atlas represents a single MSDF texture atlas.
type Atlas struct {
	// Data is the RGB pixel data.
	Data []byte

	// Size is width = height of the atlas.
	Size int

	// Regions tracks allocated glyph regions.
	regions map[GlyphKey]Region

	// allocator packs glyphs using grid algorithm.
	allocator *GridAllocator

	// dirty marks if atlas needs GPU upload.
	dirty bool

	// index is the atlas index in the manager.
	index int

	// glyphSize is the size of each glyph cell.
	glyphSize int
}

// newAtlas creates a new atlas with the given configuration.
func newAtlas(index, size, glyphSize, padding int) *Atlas {
	return &Atlas{
		Data:      make([]byte, size*size*3),
		Size:      size,
		regions:   make(map[GlyphKey]Region),
		allocator: NewGridAllocator(size, size, glyphSize, padding),
		dirty:     false,
		index:     index,
		glyphSize: glyphSize,
	}
}

// copyMSDF copies MSDF data into the atlas at the given position.
func (a *Atlas) copyMSDF(msdf *MSDF, x, y int) {
	if msdf == nil {
		return
	}

	// Calculate scaling if MSDF size differs from glyph cell size
	srcW := msdf.Width
	srcH := msdf.Height
	dstW := a.glyphSize
	dstH := a.glyphSize

	// Simple nearest-neighbor scaling
	for dy := 0; dy < dstH; dy++ {
		srcY := dy * srcH / dstH
		if srcY >= srcH {
			srcY = srcH - 1
		}

		for dx := 0; dx < dstW; dx++ {
			srcX := dx * srcW / dstW
			if srcX >= srcW {
				srcX = srcW - 1
			}

			r, g, b := msdf.GetPixel(srcX, srcY)

			// Calculate destination position
			dstX := x + dx
			dstY := y + dy
			if dstX >= 0 && dstX < a.Size && dstY >= 0 && dstY < a.Size {
				offset := (dstY*a.Size + dstX) * 3
				a.Data[offset] = r
				a.Data[offset+1] = g
				a.Data[offset+2] = b
			}
		}
	}

	a.dirty = true
}

// IsFull returns true if no more glyphs can be added.
func (a *Atlas) IsFull() bool {
	return a.allocator.IsFull()
}

// GlyphCount returns the number of glyphs in this atlas.
func (a *Atlas) GlyphCount() int {
	return len(a.regions)
}

// Utilization returns the percentage of atlas space used.
func (a *Atlas) Utilization() float64 {
	return a.allocator.Utilization()
}

// IsDirty returns true if the atlas has been modified since last upload.
func (a *Atlas) IsDirty() bool {
	return a.dirty
}

// Region describes a glyph's location in the atlas.
type Region struct {
	// AtlasIndex indicates which atlas this glyph is in.
	AtlasIndex int

	// UV coordinates [0, 1] for texture sampling.
	U0, V0, U1, V1 float32

	// Pixel coordinates in atlas.
	X, Y, Width, Height int
}

// GlyphKey uniquely identifies a glyph in the atlas.
type GlyphKey struct {
	// FontID identifies the font (hash of font data or path).
	FontID uint64

	// GlyphID is the glyph index within the font.
	GlyphID uint16

	// Size is the MSDF texture size (not font size).
	// Different MSDF sizes produce different textures.
	Size int16
}

// AtlasManager manages multiple MSDF atlases.
type AtlasManager struct {
	mu        sync.RWMutex
	config    AtlasConfig
	atlases   []*Atlas
	lookup    map[GlyphKey]Region
	generator *Generator

	// Statistics (atomic for lock-free reads)
	hits   atomic.Uint64
	misses atomic.Uint64
}

// NewAtlasManager creates a new atlas manager.
func NewAtlasManager(config AtlasConfig) (*AtlasManager, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Create generator with config matching glyph size
	genConfig := DefaultConfig()
	genConfig.Size = config.GlyphSize

	return &AtlasManager{
		config:    config,
		atlases:   make([]*Atlas, 0, config.MaxAtlases),
		lookup:    make(map[GlyphKey]Region),
		generator: NewGenerator(genConfig),
	}, nil
}

// NewAtlasManagerDefault creates a new atlas manager with default configuration.
func NewAtlasManagerDefault() *AtlasManager {
	m, _ := NewAtlasManager(DefaultAtlasConfig())
	return m
}

// Get retrieves a glyph region, generating MSDF if needed.
// Returns the region for the glyph, creating it if necessary.
func (m *AtlasManager) Get(key GlyphKey, outline *text.GlyphOutline) (Region, error) {
	// Fast path: check if already cached (read lock)
	m.mu.RLock()
	if region, ok := m.lookup[key]; ok {
		m.mu.RUnlock()
		m.hits.Add(1)
		return region, nil
	}
	m.mu.RUnlock()

	m.misses.Add(1)

	// Slow path: need to generate and add (write lock)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if region, ok := m.lookup[key]; ok {
		return region, nil
	}

	// Generate MSDF
	msdf, err := m.generator.Generate(outline)
	if err != nil {
		return Region{}, fmt.Errorf("failed to generate MSDF: %w", err)
	}

	// Find or create atlas with space
	atlas, err := m.findOrCreateAtlas()
	if err != nil {
		return Region{}, err
	}

	// Allocate cell in atlas
	x, y, ok := atlas.allocator.Allocate()
	if !ok {
		// This shouldn't happen if findOrCreateAtlas works correctly
		return Region{}, ErrAllocationFailed
	}

	// Copy MSDF data to atlas
	atlas.copyMSDF(msdf, x, y)

	// Create region
	glyphSize := m.config.GlyphSize
	atlasSize := float32(m.config.Size)

	region := Region{
		AtlasIndex: atlas.index,
		X:          x,
		Y:          y,
		Width:      glyphSize,
		Height:     glyphSize,
		U0:         float32(x) / atlasSize,
		V0:         float32(y) / atlasSize,
		U1:         float32(x+glyphSize) / atlasSize,
		V1:         float32(y+glyphSize) / atlasSize,
	}

	// Store in lookup
	m.lookup[key] = region
	atlas.regions[key] = region

	return region, nil
}

// GetBatch retrieves multiple glyph regions efficiently.
// This is more efficient than calling Get multiple times as it
// reduces lock contention and can batch MSDF generation.
func (m *AtlasManager) GetBatch(keys []GlyphKey, outlines []*text.GlyphOutline) ([]Region, error) {
	if len(keys) != len(outlines) {
		return nil, ErrLengthMismatch
	}

	results := make([]Region, len(keys))
	missing := make([]int, 0, len(keys))

	// First pass: find cached entries (read lock)
	m.mu.RLock()
	for i, key := range keys {
		if region, ok := m.lookup[key]; ok {
			results[i] = region
			m.hits.Add(1)
		} else {
			missing = append(missing, i)
		}
	}
	m.mu.RUnlock()

	// If all cached, we're done
	if len(missing) == 0 {
		return results, nil
	}

	// Second pass: generate missing entries (write lock)
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, idx := range missing {
		key := keys[idx]

		// Double-check after acquiring write lock
		if region, ok := m.lookup[key]; ok {
			results[idx] = region
			continue
		}

		m.misses.Add(1)

		// Generate MSDF
		msdf, err := m.generator.Generate(outlines[idx])
		if err != nil {
			return nil, fmt.Errorf("failed to generate MSDF for key %v: %w", key, err)
		}

		// Find or create atlas with space
		atlas, err := m.findOrCreateAtlas()
		if err != nil {
			return nil, err
		}

		// Allocate cell in atlas
		x, y, ok := atlas.allocator.Allocate()
		if !ok {
			return nil, ErrAllocationFailed
		}

		// Copy MSDF data to atlas
		atlas.copyMSDF(msdf, x, y)

		// Create region
		glyphSize := m.config.GlyphSize
		atlasSize := float32(m.config.Size)

		region := Region{
			AtlasIndex: atlas.index,
			X:          x,
			Y:          y,
			Width:      glyphSize,
			Height:     glyphSize,
			U0:         float32(x) / atlasSize,
			V0:         float32(y) / atlasSize,
			U1:         float32(x+glyphSize) / atlasSize,
			V1:         float32(y+glyphSize) / atlasSize,
		}

		// Store in lookup
		m.lookup[key] = region
		atlas.regions[key] = region
		results[idx] = region
	}

	return results, nil
}

// findOrCreateAtlas finds an atlas with space or creates a new one.
// Must be called with write lock held.
func (m *AtlasManager) findOrCreateAtlas() (*Atlas, error) {
	// Find existing atlas with space
	for _, atlas := range m.atlases {
		if !atlas.IsFull() {
			return atlas, nil
		}
	}

	// Need to create new atlas
	if len(m.atlases) >= m.config.MaxAtlases {
		return nil, &AtlasFullError{MaxAtlases: m.config.MaxAtlases}
	}

	atlas := newAtlas(
		len(m.atlases),
		m.config.Size,
		m.config.GlyphSize,
		m.config.Padding,
	)
	m.atlases = append(m.atlases, atlas)

	return atlas, nil
}

// Clear removes all cached glyphs and resets all atlases.
func (m *AtlasManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.atlases = m.atlases[:0]
	m.lookup = make(map[GlyphKey]Region)
	m.hits.Store(0)
	m.misses.Store(0)
}

// Stats returns cache statistics.
func (m *AtlasManager) Stats() (hits, misses uint64, atlasCount int) {
	m.mu.RLock()
	atlasCount = len(m.atlases)
	m.mu.RUnlock()

	hits = m.hits.Load()
	misses = m.misses.Load()
	return
}

// GlyphCount returns the total number of cached glyphs.
func (m *AtlasManager) GlyphCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.lookup)
}

// AtlasCount returns the number of atlases currently in use.
func (m *AtlasManager) AtlasCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.atlases)
}

// GetAtlas returns atlas data for GPU upload.
// Returns nil if index is out of range.
func (m *AtlasManager) GetAtlas(index int) *Atlas {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if index < 0 || index >= len(m.atlases) {
		return nil
	}
	return m.atlases[index]
}

// DirtyAtlases returns indices of atlases needing GPU upload.
func (m *AtlasManager) DirtyAtlases() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var dirty []int
	for i, atlas := range m.atlases {
		if atlas.dirty {
			dirty = append(dirty, i)
		}
	}
	return dirty
}

// MarkClean marks an atlas as uploaded to GPU.
func (m *AtlasManager) MarkClean(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index >= 0 && index < len(m.atlases) {
		m.atlases[index].dirty = false
	}
}

// MarkAllClean marks all atlases as uploaded to GPU.
func (m *AtlasManager) MarkAllClean() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, atlas := range m.atlases {
		atlas.dirty = false
	}
}

// Config returns the atlas configuration.
func (m *AtlasManager) Config() AtlasConfig {
	return m.config
}

// Generator returns the MSDF generator used by this manager.
func (m *AtlasManager) Generator() *Generator {
	return m.generator
}

// SetGenerator sets a custom MSDF generator.
func (m *AtlasManager) SetGenerator(g *Generator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generator = g
}

// HasGlyph returns true if the glyph is already cached.
func (m *AtlasManager) HasGlyph(key GlyphKey) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.lookup[key]
	return ok
}

// Remove removes a specific glyph from the cache.
// Note: This does not reclaim space in the atlas.
func (m *AtlasManager) Remove(key GlyphKey) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	region, ok := m.lookup[key]
	if !ok {
		return false
	}

	delete(m.lookup, key)
	if region.AtlasIndex >= 0 && region.AtlasIndex < len(m.atlases) {
		delete(m.atlases[region.AtlasIndex].regions, key)
	}

	return true
}

// Compact removes all atlases that have no glyphs.
// This reclaims memory from cleared atlases.
func (m *AtlasManager) Compact() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	newAtlases := make([]*Atlas, 0, len(m.atlases))

	for _, atlas := range m.atlases {
		if len(atlas.regions) > 0 {
			atlas.index = len(newAtlases)
			// Update region atlas indices
			for key, region := range atlas.regions {
				region.AtlasIndex = atlas.index
				atlas.regions[key] = region
				m.lookup[key] = region
			}
			newAtlases = append(newAtlases, atlas)
		} else {
			removed++
		}
	}

	m.atlases = newAtlases
	return removed
}

// MemoryUsage returns the total memory used by all atlases in bytes.
func (m *AtlasManager) MemoryUsage() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total int64
	for _, atlas := range m.atlases {
		total += int64(len(atlas.Data))
	}
	return total
}

// AtlasInfo contains information about a single atlas.
type AtlasInfo struct {
	Index       int
	GlyphCount  int
	Utilization float64
	Dirty       bool
	MemoryBytes int
}

// AtlasInfos returns information about all atlases.
func (m *AtlasManager) AtlasInfos() []AtlasInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]AtlasInfo, len(m.atlases))
	for i, atlas := range m.atlases {
		infos[i] = AtlasInfo{
			Index:       i,
			GlyphCount:  len(atlas.regions),
			Utilization: atlas.Utilization(),
			Dirty:       atlas.dirty,
			MemoryBytes: len(atlas.Data),
		}
	}
	return infos
}

// AtlasFullError is returned when all atlases are full.
type AtlasFullError struct {
	MaxAtlases int
}

func (e *AtlasFullError) Error() string {
	return fmt.Sprintf("msdf: all %d atlases are full", e.MaxAtlases)
}

// ConcurrentAtlasManager wraps AtlasManager with optimized concurrent access patterns.
// It uses sharding to reduce lock contention for high-throughput scenarios.
type ConcurrentAtlasManager struct {
	shards    []*AtlasManager
	shardMask uint64
}

// NewConcurrentAtlasManager creates a sharded atlas manager.
// numShards must be a power of 2.
func NewConcurrentAtlasManager(config AtlasConfig, numShards int) (*ConcurrentAtlasManager, error) {
	// Ensure numShards is power of 2
	if numShards <= 0 || (numShards&(numShards-1)) != 0 {
		numShards = 4 // Default to 4 shards
	}

	shards := make([]*AtlasManager, numShards)
	for i := range shards {
		m, err := NewAtlasManager(config)
		if err != nil {
			return nil, err
		}
		shards[i] = m
	}

	return &ConcurrentAtlasManager{
		shards:    shards,
		shardMask: uint64(numShards - 1), //nolint:gosec // numShards is validated to be positive power of 2
	}, nil
}

// getShard returns the appropriate shard for a key.
func (c *ConcurrentAtlasManager) getShard(key GlyphKey) *AtlasManager {
	// Simple hash combining FontID and GlyphID
	hash := key.FontID ^ uint64(key.GlyphID)<<16
	return c.shards[hash&c.shardMask]
}

// Get retrieves a glyph region from the appropriate shard.
func (c *ConcurrentAtlasManager) Get(key GlyphKey, outline *text.GlyphOutline) (Region, error) {
	return c.getShard(key).Get(key, outline)
}

// HasGlyph checks if a glyph is cached in the appropriate shard.
func (c *ConcurrentAtlasManager) HasGlyph(key GlyphKey) bool {
	return c.getShard(key).HasGlyph(key)
}

// Clear clears all shards.
func (c *ConcurrentAtlasManager) Clear() {
	for _, shard := range c.shards {
		shard.Clear()
	}
}

// Stats returns combined statistics from all shards.
func (c *ConcurrentAtlasManager) Stats() (hits, misses uint64, atlasCount int) {
	for _, shard := range c.shards {
		h, m, a := shard.Stats()
		hits += h
		misses += m
		atlasCount += a
	}
	return
}

// GlyphCount returns the total glyph count across all shards.
func (c *ConcurrentAtlasManager) GlyphCount() int {
	total := 0
	for _, shard := range c.shards {
		total += shard.GlyphCount()
	}
	return total
}

// MemoryUsage returns total memory usage across all shards.
func (c *ConcurrentAtlasManager) MemoryUsage() int64 {
	var total int64
	for _, shard := range c.shards {
		total += shard.MemoryUsage()
	}
	return total
}
