// Package image provides image buffer management for gogpu/gg.
package image

import "math"

// MipmapChain holds pre-computed downscaled versions of an image.
//
// A mipmap chain consists of multiple levels, where each level is half the
// size of the previous level (both width and height). Level 0 is the original
// full-resolution image. The chain continues until the smallest dimension
// reaches 1 pixel.
//
// Mipmaps are used for efficient texture filtering at different scales,
// reducing aliasing artifacts when rendering scaled images.
type MipmapChain struct {
	levels []*ImageBuf // Level 0 = original size
}

// GenerateMipmaps creates a mipmap chain from the source image.
//
// Uses a box filter (2x2 average) to downsample each level. The process
// continues until the smallest dimension reaches 1 pixel. The source image
// becomes level 0 and is not copied.
//
// Returns nil if src is nil or empty.
func GenerateMipmaps(src *ImageBuf) *MipmapChain {
	if src == nil || src.IsEmpty() {
		return nil
	}

	// Calculate number of levels
	maxDim := max(src.Width(), src.Height())
	numLevels := 1 + int(math.Floor(math.Log2(float64(maxDim))))

	chain := &MipmapChain{
		levels: make([]*ImageBuf, numLevels),
	}

	// Level 0 is the original image (no copy)
	chain.levels[0] = src

	// Generate downsampled levels
	for i := 1; i < numLevels; i++ {
		chain.levels[i] = downsample(chain.levels[i-1])
	}

	return chain
}

// downsample creates a half-size version of src using box filter.
// Returns a new ImageBuf from the pool.
func downsample(src *ImageBuf) *ImageBuf {
	srcW, srcH := src.Bounds()
	dstW := max(1, srcW/2)
	dstH := max(1, srcH/2)

	// Get buffer from pool for efficiency
	dst := GetFromDefault(dstW, dstH, src.Format())
	if dst == nil {
		return nil
	}

	// Box filter: average 2x2 pixels into 1
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			sx := dx * 2
			sy := dy * 2

			// Sample 2x2 region (handle odd dimensions)
			r0, g0, b0, a0 := src.GetRGBA(sx, sy)
			r1, g1, b1, a1 := src.GetRGBA(min(sx+1, srcW-1), sy)
			r2, g2, b2, a2 := src.GetRGBA(sx, min(sy+1, srcH-1))
			r3, g3, b3, a3 := src.GetRGBA(min(sx+1, srcW-1), min(sy+1, srcH-1))

			// Average the 4 pixels
			r := (uint16(r0) + uint16(r1) + uint16(r2) + uint16(r3)) / 4
			g := (uint16(g0) + uint16(g1) + uint16(g2) + uint16(g3)) / 4
			b := (uint16(b0) + uint16(b1) + uint16(b2) + uint16(b3)) / 4
			a := (uint16(a0) + uint16(a1) + uint16(a2) + uint16(a3)) / 4

			_ = dst.SetRGBA(dx, dy, byte(r), byte(g), byte(b), byte(a))
		}
	}

	return dst
}

// Level returns the mipmap at the specified level.
// Level 0 is the original image. Returns nil if level is out of range.
func (m *MipmapChain) Level(n int) *ImageBuf {
	if m == nil || n < 0 || n >= len(m.levels) {
		return nil
	}
	return m.levels[n]
}

// NumLevels returns the total number of mipmap levels in the chain.
// Returns 0 if the chain is nil.
func (m *MipmapChain) NumLevels() int {
	if m == nil {
		return 0
	}
	return len(m.levels)
}

// LevelForScale returns the appropriate mipmap level for a given scale factor.
//
// The scale parameter represents the ratio of displayed size to original size:
//   - scale = 1.0: original size (level 0)
//   - scale = 0.5: half size (level 1)
//   - scale = 0.25: quarter size (level 2)
//   - etc.
//
// The level is calculated as: level = floor(-log2(scale))
// The result is clamped to [0, NumLevels-1].
//
// Returns 0 if scale is >= 1.0 or if the chain is nil.
func (m *MipmapChain) LevelForScale(scale float64) *ImageBuf {
	if m == nil || len(m.levels) == 0 {
		return nil
	}

	if scale >= 1.0 {
		return m.levels[0]
	}

	// Calculate appropriate level: level = floor(-log2(scale))
	level := int(math.Floor(-math.Log2(scale)))

	// Clamp to valid range
	if level < 0 {
		level = 0
	}
	if level >= len(m.levels) {
		level = len(m.levels) - 1
	}

	return m.levels[level]
}

// Release returns all mipmap buffers to the pool except level 0.
//
// Level 0 is the original image and is not returned to the pool since
// it was provided by the caller. After calling Release, the chain should
// not be used.
func (m *MipmapChain) Release() {
	if m == nil {
		return
	}

	// Return all levels except 0 (which is the original image)
	for i := 1; i < len(m.levels); i++ {
		if m.levels[i] != nil {
			PutToDefault(m.levels[i])
			m.levels[i] = nil
		}
	}
}
