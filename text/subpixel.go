// Package text provides GPU text rendering infrastructure.
package text

import (
	"sync"
)

// SubpixelMode controls subpixel text positioning.
// Subpixel positioning improves text quality by allowing glyphs to be rendered
// at fractional pixel positions. This is especially important for small text
// where the difference between whole pixel positions is noticeable.
type SubpixelMode int

const (
	// SubpixelNone disables subpixel positioning.
	// Glyphs snap to whole pixels. Fastest but lower quality.
	SubpixelNone SubpixelMode = 0

	// Subpixel4 uses 4 subpixel positions (0.0, 0.25, 0.5, 0.75).
	// Good balance of quality and cache size.
	Subpixel4 SubpixelMode = 4

	// Subpixel10 uses 10 subpixel positions (0.0, 0.1, ..., 0.9).
	// Highest quality but 10x cache entries per glyph.
	Subpixel10 SubpixelMode = 10
)

// String returns the string representation of the subpixel mode.
func (m SubpixelMode) String() string {
	switch m {
	case SubpixelNone:
		return noneStr
	case Subpixel4:
		return "Subpixel4"
	case Subpixel10:
		return "Subpixel10"
	default:
		return unknownStr
	}
}

// IsEnabled returns true if subpixel positioning is enabled.
func (m SubpixelMode) IsEnabled() bool {
	return m > 0
}

// Divisions returns the number of subpixel divisions.
// Returns 1 for SubpixelNone (no divisions).
func (m SubpixelMode) Divisions() int {
	if m <= 0 {
		return 1
	}
	return int(m)
}

// SubpixelConfig holds subpixel positioning configuration.
type SubpixelConfig struct {
	// Mode determines the number of subpixel positions.
	Mode SubpixelMode

	// Horizontal enables subpixel positioning on X axis.
	Horizontal bool

	// Vertical enables subpixel positioning on Y axis (rarely needed).
	Vertical bool
}

// DefaultSubpixelConfig returns default configuration.
// Uses 4 horizontal subpixel positions.
func DefaultSubpixelConfig() SubpixelConfig {
	return SubpixelConfig{
		Mode:       Subpixel4,
		Horizontal: true,
		Vertical:   false,
	}
}

// NoSubpixelConfig returns a configuration with subpixel positioning disabled.
func NoSubpixelConfig() SubpixelConfig {
	return SubpixelConfig{
		Mode:       SubpixelNone,
		Horizontal: false,
		Vertical:   false,
	}
}

// HighQualitySubpixelConfig returns a configuration with maximum subpixel quality.
// Uses 10 horizontal subpixel positions.
func HighQualitySubpixelConfig() SubpixelConfig {
	return SubpixelConfig{
		Mode:       Subpixel10,
		Horizontal: true,
		Vertical:   false,
	}
}

// IsEnabled returns true if any subpixel positioning is enabled.
func (c SubpixelConfig) IsEnabled() bool {
	return c.Mode.IsEnabled() && (c.Horizontal || c.Vertical)
}

// CacheMultiplier returns the factor by which cache size increases.
// For Subpixel4 with horizontal only: 4x
// For Subpixel10 with both: 100x
func (c SubpixelConfig) CacheMultiplier() int {
	if !c.Mode.IsEnabled() {
		return 1
	}
	mult := 1
	if c.Horizontal {
		mult *= c.Mode.Divisions()
	}
	if c.Vertical {
		mult *= c.Mode.Divisions()
	}
	return mult
}

// SubpixelKey extends OutlineCacheKey with subpixel offset.
// This allows caching separate rasterized glyphs for each subpixel position.
type SubpixelKey struct {
	OutlineCacheKey

	// SubX is the quantized horizontal subpixel position (0 to Mode-1).
	SubX uint8

	// SubY is the quantized vertical subpixel position (0 to Mode-1).
	SubY uint8
}

// Quantize converts a fractional position to quantized subpixel offset.
// Returns the integer position and subpixel key component.
//
// For example, with Subpixel4 mode:
//   - pos=10.0 returns (10, 0)
//   - pos=10.25 returns (10, 1)
//   - pos=10.5 returns (10, 2)
//   - pos=10.75 returns (10, 3)
//   - pos=10.99 returns (10, 3) // quantized to nearest
func Quantize(pos float64, mode SubpixelMode) (intPos int, subPos uint8) {
	if !mode.IsEnabled() {
		// No subpixel positioning - round to nearest integer
		return int(pos + 0.5), 0
	}

	// Compute floor (integer part that is <= pos)
	intPart := int(pos)
	if pos < 0 && pos != float64(intPart) {
		intPart--
	}

	// Get the fractional part [0, 1)
	frac := pos - float64(intPart)

	// Quantize to subpixel position
	divisions := float64(mode.Divisions())
	subPosFloat := frac * divisions
	subPosInt := int(subPosFloat)

	// Clamp to valid range [0, mode-1]
	if subPosInt >= mode.Divisions() {
		subPosInt = mode.Divisions() - 1
	}
	if subPosInt < 0 {
		subPosInt = 0
	}

	// Safe conversion since subPosInt is in range [0, mode-1]
	// and mode is at most 10
	return intPart, uint8(subPosInt) //nolint:gosec // subPosInt is bounded [0, mode-1]
}

// QuantizePoint quantizes both X and Y positions.
// Returns integer positions and subpixel key components.
func QuantizePoint(x, y float64, config SubpixelConfig) (intX, intY int, subX, subY uint8) {
	if config.Horizontal {
		intX, subX = Quantize(x, config.Mode)
	} else {
		intX, subX = int(x+0.5), 0
	}

	if config.Vertical {
		intY, subY = Quantize(y, config.Mode)
	} else {
		intY, subY = int(y+0.5), 0
	}

	return intX, intY, subX, subY
}

// SubpixelOffset returns the rendering offset for a subpixel position.
// For Subpixel4 mode: 0 -> 0.0, 1 -> 0.25, 2 -> 0.5, 3 -> 0.75
// For Subpixel10 mode: 0 -> 0.0, 1 -> 0.1, ..., 9 -> 0.9
func SubpixelOffset(subPos uint8, mode SubpixelMode) float64 {
	if !mode.IsEnabled() {
		return 0
	}
	return float64(subPos) / float64(mode.Divisions())
}

// SubpixelOffsets returns both X and Y rendering offsets.
func SubpixelOffsets(subX, subY uint8, config SubpixelConfig) (offsetX, offsetY float64) {
	if config.Horizontal {
		offsetX = SubpixelOffset(subX, config.Mode)
	}
	if config.Vertical {
		offsetY = SubpixelOffset(subY, config.Mode)
	}
	return offsetX, offsetY
}

// SubpixelCache wraps GlyphCache with subpixel awareness.
// It provides the same interface as GlyphCache but uses SubpixelKey
// to differentiate between the same glyph at different subpixel offsets.
//
// SubpixelCache is safe for concurrent use.
type SubpixelCache struct {
	// cache is the underlying glyph cache
	cache *GlyphCache

	// config holds the subpixel configuration
	config SubpixelConfig

	// stats holds cache statistics
	stats SubpixelCacheStats

	mu sync.RWMutex
}

// SubpixelCacheStats holds subpixel cache statistics.
type SubpixelCacheStats struct {
	// Hits is the number of cache hits
	Hits uint64

	// Misses is the number of cache misses
	Misses uint64

	// SubpixelHits is hits where subpixel position matched
	SubpixelHits uint64

	// SubpixelCreates is glyphs created for subpixel positions
	SubpixelCreates uint64
}

// NewSubpixelCache creates a cache with subpixel support.
func NewSubpixelCache(config SubpixelConfig) *SubpixelCache {
	return &SubpixelCache{
		cache:  NewGlyphCache(),
		config: config,
	}
}

// NewSubpixelCacheWithConfig creates a cache with custom glyph cache config.
func NewSubpixelCacheWithConfig(config SubpixelConfig, glyphConfig GlyphCacheConfig) *SubpixelCache {
	// Adjust cache size to account for subpixel multiplier
	// This maintains similar memory usage per unique glyph
	adjustedConfig := glyphConfig
	mult := config.CacheMultiplier()
	if mult > 1 {
		adjustedConfig.MaxEntries = glyphConfig.MaxEntries * mult
	}

	return &SubpixelCache{
		cache:  NewGlyphCacheWithConfig(adjustedConfig),
		config: config,
	}
}

// Config returns the subpixel configuration.
func (c *SubpixelCache) Config() SubpixelConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// SetConfig updates the subpixel configuration.
// Note: This clears the cache as existing entries may be invalid.
func (c *SubpixelCache) SetConfig(config SubpixelConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = config
	c.cache.Clear()
}

// Get retrieves a glyph at the specified subpixel position.
// Returns nil if not found.
func (c *SubpixelCache) Get(key SubpixelKey) *GlyphOutline {
	outline := c.cache.Get(c.toOutlineKey(key))
	if outline != nil {
		c.mu.Lock()
		c.stats.Hits++
		c.stats.SubpixelHits++
		c.mu.Unlock()
	} else {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
	}
	return outline
}

// Set stores a glyph outline at the specified subpixel position.
func (c *SubpixelCache) Set(key SubpixelKey, outline *GlyphOutline) {
	c.cache.Set(c.toOutlineKey(key), outline)
}

// GetOrCreate retrieves or creates a glyph at subpixel position.
// The create function receives the subpixel offsets to apply during rasterization.
func (c *SubpixelCache) GetOrCreate(
	key SubpixelKey,
	create func(offsetX, offsetY float64) *GlyphOutline,
) *GlyphOutline {
	// Try fast path first
	if outline := c.Get(key); outline != nil {
		return outline
	}

	if create == nil {
		return nil
	}

	// Calculate subpixel offsets for rendering
	c.mu.RLock()
	config := c.config
	c.mu.RUnlock()

	offsetX, offsetY := SubpixelOffsets(key.SubX, key.SubY, config)

	// Create the outline with subpixel offset applied
	outline := create(offsetX, offsetY)
	if outline != nil {
		c.Set(key, outline)
		c.mu.Lock()
		c.stats.SubpixelCreates++
		c.mu.Unlock()
	}

	return outline
}

// Delete removes an entry from the cache.
func (c *SubpixelCache) Delete(key SubpixelKey) {
	c.cache.Delete(c.toOutlineKey(key))
}

// Clear removes all entries from the cache.
func (c *SubpixelCache) Clear() {
	c.cache.Clear()
}

// Maintain performs periodic maintenance on the cache.
func (c *SubpixelCache) Maintain() {
	c.cache.Maintain()
}

// Len returns the total number of cached entries.
func (c *SubpixelCache) Len() int {
	return c.cache.Len()
}

// Stats returns subpixel cache statistics.
func (c *SubpixelCache) Stats() SubpixelCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// ResetStats resets the cache statistics.
func (c *SubpixelCache) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = SubpixelCacheStats{}
	c.cache.ResetStats()
}

// HitRate returns the cache hit rate as a percentage.
func (c *SubpixelCache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.stats.Hits + c.stats.Misses
	if total == 0 {
		return 0
	}
	return float64(c.stats.Hits) / float64(total) * 100
}

// toOutlineKey converts a SubpixelKey to an OutlineCacheKey.
// The subpixel position is encoded into the FontID using bit manipulation.
func (c *SubpixelCache) toOutlineKey(key SubpixelKey) OutlineCacheKey {
	// Encode subpixel position into the top bits of FontID.
	// This ensures that the same glyph at different subpixel positions
	// has different cache keys.
	//
	// Layout: [original FontID (48 bits)][SubX (8 bits)][SubY (8 bits)]
	encodedFontID := key.FontID
	encodedFontID = (encodedFontID << 8) | uint64(key.SubX)
	encodedFontID = (encodedFontID << 8) | uint64(key.SubY)

	return OutlineCacheKey{
		FontID:  encodedFontID,
		GID:     key.GID,
		Size:    key.Size,
		Hinting: key.Hinting,
	}
}

// MakeSubpixelKey creates a SubpixelKey from glyph position and base cache key.
func MakeSubpixelKey(baseKey OutlineCacheKey, x, y float64, config SubpixelConfig) SubpixelKey {
	_, _, subX, subY := QuantizePoint(x, y, config)
	return SubpixelKey{
		OutlineCacheKey: baseKey,
		SubX:            subX,
		SubY:            subY,
	}
}

// globalSubpixelCache is the default shared subpixel cache.
var globalSubpixelCache = NewSubpixelCache(DefaultSubpixelConfig())

// GetGlobalSubpixelCache returns the global shared subpixel cache.
func GetGlobalSubpixelCache() *SubpixelCache {
	return globalSubpixelCache
}

// SetGlobalSubpixelCache replaces the global subpixel cache.
// The old cache is returned for cleanup if needed.
func SetGlobalSubpixelCache(cache *SubpixelCache) *SubpixelCache {
	if cache == nil {
		cache = NewSubpixelCache(DefaultSubpixelConfig())
	}
	old := globalSubpixelCache
	globalSubpixelCache = cache
	return old
}
