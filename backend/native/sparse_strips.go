package native

import (
	"sync"

	"github.com/gogpu/gg/scene"
)

// SparseStripsRasterizer is the main entry point for vello-style sparse strips rendering.
// It orchestrates the complete pipeline: flatten -> coarse -> fine -> render.
//
// The sparse strips algorithm works by:
// 1. Flattening Bezier curves to monotonic line segments
// 2. Coarse rasterization: binning segments into tiles they intersect
// 3. Fine rasterization: calculating analytic anti-aliased coverage per pixel
// 4. Rendering: outputting coverage as strips for efficient GPU rendering
type SparseStripsRasterizer struct {
	// Pipeline components
	flattenCtx *FlattenContext
	coarse     *CoarseRasterizer
	fine       *FineRasterizer
	strips     *StripRenderer

	// Configuration
	width    uint16
	height   uint16
	fillRule scene.FillStyle

	// Computed data
	backdrop []int32

	// Pool for reuse
	pool *SparseStripsPool
}

// SparseStripsConfig configures the sparse strips rasterizer.
type SparseStripsConfig struct {
	Width     uint16
	Height    uint16
	FillRule  scene.FillStyle
	Tolerance float32 // Flattening tolerance (0 uses default)
}

// DefaultConfig returns a default configuration.
func DefaultConfig(width, height uint16) SparseStripsConfig {
	return SparseStripsConfig{
		Width:     width,
		Height:    height,
		FillRule:  scene.FillNonZero,
		Tolerance: FlattenTolerance,
	}
}

// NewSparseStripsRasterizer creates a new sparse strips rasterizer.
func NewSparseStripsRasterizer(config SparseStripsConfig) *SparseStripsRasterizer {
	return &SparseStripsRasterizer{
		flattenCtx: NewFlattenContext(),
		coarse:     NewCoarseRasterizer(config.Width, config.Height),
		fine:       NewFineRasterizer(config.Width, config.Height),
		strips:     NewStripRenderer(),
		width:      config.Width,
		height:     config.Height,
		fillRule:   config.FillRule,
		pool:       globalSparseStripsPool,
	}
}

// Reset clears the rasterizer state for reuse with new geometry.
func (ssr *SparseStripsRasterizer) Reset() {
	ssr.flattenCtx.Reset()
	ssr.coarse.Reset()
	ssr.fine.Reset()
	ssr.strips.Reset()
	ssr.backdrop = nil
}

// SetFillRule sets the fill rule for rendering.
func (ssr *SparseStripsRasterizer) SetFillRule(rule scene.FillStyle) {
	ssr.fillRule = rule
	ssr.fine.SetFillRule(rule)
	ssr.strips.SetFillRule(rule)
}

// raster.FillRule returns the current fill rule.
func (ssr *SparseStripsRasterizer) FillRule() scene.FillStyle {
	return ssr.fillRule
}

// SetSize changes the rasterizer viewport dimensions.
func (ssr *SparseStripsRasterizer) SetSize(width, height uint16) {
	if ssr.width != width || ssr.height != height {
		ssr.width = width
		ssr.height = height
		ssr.coarse = NewCoarseRasterizer(width, height)
		ssr.fine = NewFineRasterizer(width, height)
	}
}

// Width returns the viewport width.
func (ssr *SparseStripsRasterizer) Width() uint16 {
	return ssr.width
}

// Height returns the viewport height.
func (ssr *SparseStripsRasterizer) Height() uint16 {
	return ssr.height
}

// RasterizePath rasterizes a single path to the tile grid.
func (ssr *SparseStripsRasterizer) RasterizePath(
	path *scene.Path,
	transform scene.Affine,
	tolerance float32,
) {
	if path == nil || path.IsEmpty() {
		return
	}

	if tolerance <= 0 {
		tolerance = FlattenTolerance
	}

	// Phase 1: Flatten path to monotonic line segments
	ssr.flattenCtx.Reset()
	ssr.flattenCtx.FlattenPathTo(path, transform, tolerance)
	segments := ssr.flattenCtx.Segments()

	if segments.Len() == 0 {
		return
	}

	// Phase 2: Coarse rasterization - bin segments into tiles
	ssr.coarse.Rasterize(segments)
	ssr.coarse.SortEntries()

	// Phase 3: Calculate backdrop winding
	ssr.backdrop = ssr.coarse.CalculateBackdrop()

	// Phase 4: Fine rasterization - analytic anti-aliasing
	ssr.fine.SetFillRule(ssr.fillRule)
	ssr.fine.Rasterize(ssr.coarse, segments, ssr.backdrop)
}

// RasterizeToStrips rasterizes a path and generates sparse strips.
func (ssr *SparseStripsRasterizer) RasterizeToStrips(
	path *scene.Path,
	transform scene.Affine,
	tolerance float32,
) {
	if path == nil || path.IsEmpty() {
		return
	}

	if tolerance <= 0 {
		tolerance = FlattenTolerance
	}

	// Phase 1: Flatten
	ssr.flattenCtx.Reset()
	ssr.flattenCtx.FlattenPathTo(path, transform, tolerance)
	segments := ssr.flattenCtx.Segments()

	if segments.Len() == 0 {
		return
	}

	// Phase 2: Coarse rasterization
	ssr.coarse.Rasterize(segments)
	ssr.coarse.SortEntries()

	// Phase 3: Calculate backdrop
	ssr.backdrop = ssr.coarse.CalculateBackdrop()

	// Phase 4: Generate strips
	ssr.strips.SetFillRule(ssr.fillRule)
	ssr.strips.RenderTiles(ssr.coarse, segments, ssr.backdrop)
}

// Grid returns the tile grid with computed coverage.
func (ssr *SparseStripsRasterizer) Grid() *TileGrid {
	return ssr.fine.Grid()
}

// Strips returns the strip renderer with generated strips.
func (ssr *SparseStripsRasterizer) Strips() *StripRenderer {
	return ssr.strips
}

// Segments returns the flattened segments.
func (ssr *SparseStripsRasterizer) Segments() *SegmentList {
	return ssr.flattenCtx.Segments()
}

// RenderToBuffer renders the rasterized coverage to a pixel buffer.
func (ssr *SparseStripsRasterizer) RenderToBuffer(
	buffer []uint8,
	stride int,
	color [4]uint8,
) {
	ssr.fine.RenderToBuffer(buffer, int(ssr.width), int(ssr.height), stride, color)
}

// RenderStripsToBuffer renders strips to a pixel buffer.
func (ssr *SparseStripsRasterizer) RenderStripsToBuffer(
	buffer []uint8,
	stride int,
	color [4]uint8,
) {
	strips := ssr.strips.Strips()
	alphas := ssr.strips.Alphas()

	if len(strips) == 0 {
		return
	}

	for i := 0; i < len(strips)-1; i++ {
		strip := strips[i]
		nextStrip := strips[i+1]

		if strip.X == 0xFFFF {
			continue // Sentinel, skip
		}

		// Calculate strip width in pixels
		endX := calculateStripEndX(strip, nextStrip)

		// Fill gap if needed
		if strip.FillGap && i > 0 {
			prevStrip := strips[i-1]
			if prevStrip.Y == strip.Y && prevStrip.X != 0xFFFF {
				prevEndX := strip.X
				//nolint:gosec // Integer overflow is bounded by viewport dimensions
				gapStartX := prevStrip.X + uint16((strip.AlphaIdx-prevStrip.AlphaIdx)/TileHeight)
				fillGap(buffer, stride, int(gapStartX), int(prevEndX), int(strip.Y), TileHeight, color)
			}
		}

		// Render strip alphas
		renderStripAlphas(buffer, stride, int(strip.X), int(strip.Y), int(endX-strip.X), alphas[strip.AlphaIdx:nextStrip.AlphaIdx], color)
	}
}

// calculateStripEndX calculates the end X coordinate of a strip.
//
//nolint:gosec // Integer overflow is bounded by viewport dimensions
func calculateStripEndX(strip, nextStrip SparseStrip) uint16 {
	switch {
	case nextStrip.X == 0xFFFF:
		// Use alpha buffer to determine width
		alphaCount := nextStrip.AlphaIdx - strip.AlphaIdx
		return strip.X + uint16(alphaCount/TileHeight)
	case nextStrip.Y == strip.Y:
		return nextStrip.X
	default:
		alphaCount := nextStrip.AlphaIdx - strip.AlphaIdx
		return strip.X + uint16(alphaCount/TileHeight)
	}
}

// fillGap fills a solid gap between strips.
func fillGap(buffer []uint8, stride, startX, endX, y, height int, color [4]uint8) {
	for py := 0; py < height; py++ {
		pixelY := y + py
		rowOffset := pixelY * stride
		for px := startX; px < endX; px++ {
			idx := rowOffset + px*4
			if idx+3 < len(buffer) {
				buffer[idx+0] = color[0]
				buffer[idx+1] = color[1]
				buffer[idx+2] = color[2]
				buffer[idx+3] = color[3]
			}
		}
	}
}

// renderStripAlphas renders alpha values for a strip.
func renderStripAlphas(buffer []uint8, stride, x, y, width int, alphas []uint8, color [4]uint8) {
	alphaIdx := 0
	for px := 0; px < width && alphaIdx < len(alphas); px++ {
		pixelX := x + px
		for py := 0; py < TileHeight && alphaIdx < len(alphas); py++ {
			pixelY := y + py
			alpha := alphas[alphaIdx]
			alphaIdx++

			if alpha == 0 {
				continue
			}

			idx := pixelY*stride + pixelX*4
			if idx+3 >= len(buffer) {
				continue
			}

			// Premultiplied alpha blending
			srcA := uint16(alpha) * uint16(color[3]) / 255
			invA := 255 - srcA

			//nolint:gosec // Result is bounded 0-255
			buffer[idx+0] = uint8((uint16(color[0])*srcA + uint16(buffer[idx+0])*invA) / 255)
			//nolint:gosec // Result is bounded 0-255
			buffer[idx+1] = uint8((uint16(color[1])*srcA + uint16(buffer[idx+1])*invA) / 255)
			//nolint:gosec // Result is bounded 0-255
			buffer[idx+2] = uint8((uint16(color[2])*srcA + uint16(buffer[idx+2])*invA) / 255)
			//nolint:gosec // Result is bounded 0-255
			buffer[idx+3] = uint8(srcA + uint16(buffer[idx+3])*invA/255)
		}
	}
}

// SparseStripsPool manages pooled rasterizers for reuse.
type SparseStripsPool struct {
	mu   sync.Mutex
	pool []*SparseStripsRasterizer
}

// globalSparseStripsPool is the default pool.
var globalSparseStripsPool = NewSparseStripsPool()

// NewSparseStripsPool creates a new pool.
func NewSparseStripsPool() *SparseStripsPool {
	return &SparseStripsPool{
		pool: make([]*SparseStripsRasterizer, 0, 4),
	}
}

// Get retrieves a rasterizer from the pool or creates a new one.
func (p *SparseStripsPool) Get(config SparseStripsConfig) *SparseStripsRasterizer {
	p.mu.Lock()
	if len(p.pool) > 0 {
		ssr := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		p.mu.Unlock()

		ssr.Reset()
		ssr.SetSize(config.Width, config.Height)
		ssr.SetFillRule(config.FillRule)
		return ssr
	}
	p.mu.Unlock()

	return NewSparseStripsRasterizer(config)
}

// Put returns a rasterizer to the pool.
func (p *SparseStripsPool) Put(ssr *SparseStripsRasterizer) {
	if ssr == nil {
		return
	}
	ssr.Reset()

	p.mu.Lock()
	if len(p.pool) < 16 {
		p.pool = append(p.pool, ssr)
	}
	p.mu.Unlock()
}

// RasterizePath is a convenience function that rasterizes a path using a pooled rasterizer.
func RasterizePath(
	path *scene.Path,
	transform scene.Affine,
	width, height uint16,
	fillRule scene.FillStyle,
) *TileGrid {
	config := DefaultConfig(width, height)
	config.FillRule = fillRule

	ssr := globalSparseStripsPool.Get(config)
	defer globalSparseStripsPool.Put(ssr)

	ssr.RasterizePath(path, transform, FlattenTolerance)
	return ssr.Grid()
}

// Stats contains statistics about the rasterization process.
type Stats struct {
	SegmentCount    int
	TileEntryCount  int
	ActiveTileCount int
	StripCount      int
	AlphaByteCount  int
}

// GetStats returns statistics about the current rasterization state.
func (ssr *SparseStripsRasterizer) GetStats() Stats {
	return Stats{
		SegmentCount:    ssr.flattenCtx.Segments().Len(),
		TileEntryCount:  len(ssr.coarse.Entries()),
		ActiveTileCount: ssr.fine.Grid().TileCount(),
		StripCount:      len(ssr.strips.Strips()),
		AlphaByteCount:  len(ssr.strips.Alphas()),
	}
}
