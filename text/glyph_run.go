// Package text provides GPU text rendering infrastructure.
package text

import (
	"math"
	"sync"
)

// GlyphInstance represents a single glyph to be rendered at a specific position.
// It contains all information needed to look up the glyph outline and position it.
type GlyphInstance struct {
	// FontID uniquely identifies the font.
	FontID uint64

	// GlyphID is the glyph index within the font.
	GlyphID GlyphID

	// Position is where the glyph should be drawn.
	Position Point

	// Size is the font size in pixels (ppem).
	Size float32
}

// Point represents a 2D point for glyph positioning.
type Point struct {
	X, Y float32
}

// DrawCommand represents a single glyph draw command.
// Contains the outline path and the transformation to apply.
type DrawCommand struct {
	// Outline is the glyph outline path.
	Outline *GlyphOutline

	// Transform is the transformation to apply to the outline.
	Transform *AffineTransform

	// Instance is the original glyph instance.
	Instance GlyphInstance
}

// GlyphRunBuilder batches glyphs for efficient rendering.
// It accumulates glyphs from shaped text and creates draw commands
// by looking up glyph outlines from the cache.
//
// GlyphRunBuilder is NOT safe for concurrent use.
// Each goroutine should have its own builder.
type GlyphRunBuilder struct {
	cache     *GlyphCache
	instances []GlyphInstance
}

// NewGlyphRunBuilder creates a builder that uses the given cache.
// If cache is nil, the global glyph cache is used.
func NewGlyphRunBuilder(cache *GlyphCache) *GlyphRunBuilder {
	if cache == nil {
		cache = GetGlobalGlyphCache()
	}
	return &GlyphRunBuilder{
		cache:     cache,
		instances: make([]GlyphInstance, 0, 64),
	}
}

// AddGlyph adds a glyph to the batch.
func (b *GlyphRunBuilder) AddGlyph(fontID uint64, glyphID GlyphID, pos Point, size float32) {
	b.instances = append(b.instances, GlyphInstance{
		FontID:   fontID,
		GlyphID:  glyphID,
		Position: pos,
		Size:     size,
	})
}

// AddShapedGlyph adds a ShapedGlyph to the batch.
func (b *GlyphRunBuilder) AddShapedGlyph(fontID uint64, glyph *ShapedGlyph, size float32) {
	if glyph == nil {
		return
	}
	b.instances = append(b.instances, GlyphInstance{
		FontID:   fontID,
		GlyphID:  glyph.GID,
		Position: Point{X: float32(glyph.X), Y: float32(glyph.Y)},
		Size:     size,
	})
}

// AddShapedRun adds all glyphs from a ShapedRun.
// The origin parameter specifies the starting position for the run.
func (b *GlyphRunBuilder) AddShapedRun(run *ShapedRun, origin Point) {
	if run == nil || len(run.Glyphs) == 0 || run.Face == nil {
		return
	}

	font := run.Face.Source().Parsed()
	if font == nil {
		return
	}

	fontID := computeFontID(font)
	size := float32(run.Size)

	for i := range run.Glyphs {
		glyph := &run.Glyphs[i]
		pos := Point{
			X: origin.X + float32(glyph.X),
			Y: origin.Y + float32(glyph.Y),
		}
		b.instances = append(b.instances, GlyphInstance{
			FontID:   fontID,
			GlyphID:  glyph.GID,
			Position: pos,
			Size:     size,
		})
	}
}

// AddShapedGlyphs adds multiple shaped glyphs from a slice.
// All glyphs are assumed to be from the same font.
func (b *GlyphRunBuilder) AddShapedGlyphs(fontID uint64, glyphs []ShapedGlyph, origin Point, size float32) {
	for i := range glyphs {
		glyph := &glyphs[i]
		pos := Point{
			X: origin.X + float32(glyph.X),
			Y: origin.Y + float32(glyph.Y),
		}
		b.instances = append(b.instances, GlyphInstance{
			FontID:   fontID,
			GlyphID:  glyph.GID,
			Position: pos,
			Size:     size,
		})
	}
}

// Build retrieves all glyph outlines from cache and returns draw commands.
// The createGlyph function is called to create outlines for cache misses.
// If createGlyph is nil, cache misses result in nil outlines being skipped.
//
// Returns a slice of DrawCommands ready for rendering.
func (b *GlyphRunBuilder) Build(createGlyph func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline) []DrawCommand {
	if len(b.instances) == 0 {
		return nil
	}

	commands := make([]DrawCommand, 0, len(b.instances))

	for i := range b.instances {
		inst := &b.instances[i]

		// Build cache key
		key := OutlineCacheKey{
			FontID:  inst.FontID,
			GID:     inst.GlyphID,
			Size:    sizeToInt16(float64(inst.Size)),
			Hinting: HintingNone,
		}

		// Get or create outline
		var outline *GlyphOutline
		if createGlyph != nil {
			outline = b.cache.GetOrCreate(key, func() *GlyphOutline {
				return createGlyph(inst.FontID, inst.GlyphID, inst.Size)
			})
		} else {
			outline = b.cache.Get(key)
		}

		// Skip glyphs without outlines (e.g., spaces)
		if outline == nil || outline.IsEmpty() {
			continue
		}

		// Create transform for glyph positioning
		// Y-flip is applied because fonts have Y-up, screen has Y-down
		transform := &AffineTransform{
			A:  1,
			B:  0,
			C:  0,
			D:  -1, // Y-flip
			Tx: inst.Position.X,
			Ty: inst.Position.Y,
		}

		commands = append(commands, DrawCommand{
			Outline:   outline,
			Transform: transform,
			Instance:  *inst,
		})
	}

	return commands
}

// BuildTransformed is like Build but applies an additional transformation to all commands.
func (b *GlyphRunBuilder) BuildTransformed(
	createGlyph func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline,
	userTransform *AffineTransform,
) []DrawCommand {
	if len(b.instances) == 0 {
		return nil
	}

	commands := make([]DrawCommand, 0, len(b.instances))

	for i := range b.instances {
		inst := &b.instances[i]

		// Build cache key
		key := OutlineCacheKey{
			FontID:  inst.FontID,
			GID:     inst.GlyphID,
			Size:    sizeToInt16(float64(inst.Size)),
			Hinting: HintingNone,
		}

		// Get or create outline
		var outline *GlyphOutline
		if createGlyph != nil {
			outline = b.cache.GetOrCreate(key, func() *GlyphOutline {
				return createGlyph(inst.FontID, inst.GlyphID, inst.Size)
			})
		} else {
			outline = b.cache.Get(key)
		}

		// Skip glyphs without outlines
		if outline == nil || outline.IsEmpty() {
			continue
		}

		// Create glyph positioning transform
		glyphTransform := &AffineTransform{
			A:  1,
			B:  0,
			C:  0,
			D:  -1, // Y-flip
			Tx: inst.Position.X,
			Ty: inst.Position.Y,
		}

		// Combine with user transform
		var finalTransform *AffineTransform
		if userTransform != nil {
			finalTransform = userTransform.Multiply(glyphTransform)
		} else {
			finalTransform = glyphTransform
		}

		commands = append(commands, DrawCommand{
			Outline:   outline,
			Transform: finalTransform,
			Instance:  *inst,
		})
	}

	return commands
}

// Clear resets the builder for reuse.
// The cache is not cleared.
func (b *GlyphRunBuilder) Clear() {
	b.instances = b.instances[:0]
}

// Len returns the number of glyph instances currently buffered.
func (b *GlyphRunBuilder) Len() int {
	return len(b.instances)
}

// Cache returns the glyph cache used by this builder.
func (b *GlyphRunBuilder) Cache() *GlyphCache {
	return b.cache
}

// SetCache sets the glyph cache used by this builder.
// If cache is nil, the global glyph cache is used.
func (b *GlyphRunBuilder) SetCache(cache *GlyphCache) {
	if cache == nil {
		cache = GetGlobalGlyphCache()
	}
	b.cache = cache
}

// Instances returns a copy of the current glyph instances.
func (b *GlyphRunBuilder) Instances() []GlyphInstance {
	if len(b.instances) == 0 {
		return nil
	}
	result := make([]GlyphInstance, len(b.instances))
	copy(result, b.instances)
	return result
}

// GlyphRunBuilderPool provides pooled GlyphRunBuilders for high-concurrency scenarios.
type GlyphRunBuilderPool struct {
	pool  sync.Pool
	cache *GlyphCache
}

// NewGlyphRunBuilderPool creates a new pool with the given cache.
// If cache is nil, the global glyph cache is used.
func NewGlyphRunBuilderPool(cache *GlyphCache) *GlyphRunBuilderPool {
	if cache == nil {
		cache = GetGlobalGlyphCache()
	}
	return &GlyphRunBuilderPool{
		pool: sync.Pool{
			New: func() any {
				return NewGlyphRunBuilder(nil)
			},
		},
		cache: cache,
	}
}

// Get retrieves a GlyphRunBuilder from the pool.
func (p *GlyphRunBuilderPool) Get() *GlyphRunBuilder {
	builder := p.pool.Get().(*GlyphRunBuilder)
	builder.cache = p.cache
	return builder
}

// Put returns a GlyphRunBuilder to the pool.
// The builder is cleared before being returned.
func (p *GlyphRunBuilderPool) Put(builder *GlyphRunBuilder) {
	if builder != nil {
		builder.Clear()
		p.pool.Put(builder)
	}
}

// sizeBitsToFloat32 converts float32 size bits back to float32.
// Used for exact key matching with float32 sizes.
func sizeBitsToFloat32(bits uint32) float32 {
	return math.Float32frombits(bits)
}

// float32ToSizeBits converts float32 to bits for exact key matching.
// This avoids float comparison issues in cache keys.
func float32ToSizeBits(size float32) uint32 {
	return math.Float32bits(size)
}
