package text

import (
	"math"

	"github.com/gogpu/gg/internal/cache"
)

// cpuGlyphMaskCacheCapacity is per shard. The cache holds up to 512 masks
// per font source, enough for several sizes and subpixel variants while
// keeping memory bounded for applications that load many fonts.
const cpuGlyphMaskCacheCapacity = 32

type cpuGlyphMaskCacheKey struct {
	glyphID       GlyphID
	sizeBits      uint64
	subpixelXBits uint64
	subpixelYBits uint64
	hinting       Hinting
	mode          glyphRasterMode
}

type cpuGlyphMaskCacheValue struct {
	result *GlyphMaskResult
	err    error
}

func makeCPUGlyphMaskCacheKey(
	glyphID GlyphID,
	size, subpixelX, subpixelY float64,
	hinting Hinting,
	mode glyphRasterMode,
) cpuGlyphMaskCacheKey {
	return cpuGlyphMaskCacheKey{
		glyphID:       glyphID,
		sizeBits:      math.Float64bits(size),
		subpixelXBits: math.Float64bits(subpixelX),
		subpixelYBits: math.Float64bits(subpixelY),
		hinting:       hinting,
		mode:          mode,
	}
}

func hashCPUGlyphMaskCacheKey(key cpuGlyphMaskCacheKey) uint64 {
	h := key.sizeBits ^ mixCPUGlyphMaskHash(key.subpixelXBits)
	h ^= mixCPUGlyphMaskHash(key.subpixelYBits)
	h ^= uint64(key.glyphID) * 0x9e3779b97f4a7c15
	h ^= uint64(key.hinting+1) * 0xbf58476d1ce4e5b9
	h ^= uint64(key.mode+1) * 0x94d049bb133111eb
	return mixCPUGlyphMaskHash(h)
}

func mixCPUGlyphMaskHash(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func (s *FontSource) cpuGlyphMaskCache() *cache.ShardedCache[cpuGlyphMaskCacheKey, cpuGlyphMaskCacheValue] {
	s.cpuGlyphMasksOnce.Do(func() {
		s.cpuGlyphMasks = cache.NewSharded[cpuGlyphMaskCacheKey, cpuGlyphMaskCacheValue](
			cpuGlyphMaskCacheCapacity,
			hashCPUGlyphMaskCacheKey,
		)
	})
	return s.cpuGlyphMasks
}

func (s *FontSource) clearCPUGlyphMaskCache() {
	// Synchronize with the first lazy initialization without allocating a cache
	// solely to close a source that was never used by the software text path.
	s.cpuGlyphMasksOnce.Do(func() {})
	if s.cpuGlyphMasks != nil {
		s.cpuGlyphMasks.Clear()
	}
}
