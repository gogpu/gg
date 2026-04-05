package text

import (
	"math"

	"github.com/gogpu/gg/internal/raster"
)

// GlyphMaskRasterizer renders glyph outlines into R8 alpha masks using the
// AnalyticFiller (256-level analytic anti-aliasing). This is the CPU side of
// the Tier 6 glyph mask cache pipeline.
//
// The rasterizer extracts glyph outlines from the font at the exact device
// pixel size, builds edges via EdgeBuilder, and fills to an alpha buffer.
// The result is a tight-bbox alpha mask suitable for packing into the
// GlyphMaskAtlas R8 texture.
//
// This follows the Skia/Chrome pattern:
//   - CPU rasterizes at exact pixel size (no scaling artifacts)
//   - 256-level coverage (vs MSDF's distance-based approximation)
//   - Hinting-ready (future TEXT-012)
//   - Subpixel positioning via fractional offset in outline coordinates
//
// GlyphMaskRasterizer is NOT safe for concurrent use. Each goroutine should
// use its own instance, or protect access with a mutex.
type GlyphMaskRasterizer struct {
	extractor *OutlineExtractor

	// Reusable path buffer to avoid allocations per glyph.
	pathVerbs  []raster.PathVerb
	pathPoints []float32
}

// NewGlyphMaskRasterizer creates a new glyph mask rasterizer.
func NewGlyphMaskRasterizer() *GlyphMaskRasterizer {
	return &GlyphMaskRasterizer{
		extractor:  NewOutlineExtractor(),
		pathVerbs:  make([]raster.PathVerb, 0, 64),
		pathPoints: make([]float32, 0, 256),
	}
}

// GlyphMaskResult holds the output of rasterizing a single glyph.
type GlyphMaskResult struct {
	// Mask is the R8 alpha buffer (1 byte per pixel, row-major).
	Mask []byte

	// Width and Height of the mask in pixels.
	Width, Height int

	// BearingX is the horizontal offset from the glyph origin to the left
	// edge of the mask bounding box, in pixels.
	BearingX float32

	// BearingY is the vertical offset from the baseline to the top edge
	// of the mask bounding box, in pixels. Positive = above baseline.
	BearingY float32
}

// Rasterize renders a single glyph into an R8 alpha mask.
//
// Parameters:
//   - font: parsed font to extract outlines from
//   - gid: glyph index in the font
//   - size: font size in pixels (ppem)
//   - subpixelX: fractional X offset in pixels [0, 1) for subpixel positioning
//   - subpixelY: fractional Y offset in pixels [0, 1) for subpixel positioning
//
// Returns nil result for empty glyphs (e.g., space character).
func (r *GlyphMaskRasterizer) Rasterize(
	font ParsedFont,
	gid GlyphID,
	size float64,
	subpixelX, subpixelY float64,
) (*GlyphMaskResult, error) {
	return r.RasterizeHinted(font, gid, size, subpixelX, subpixelY, HintingNone)
}

// RasterizeHinted renders a single glyph into an R8 alpha mask with hinting.
//
// When hinting is HintingVertical or HintingFull, the outline is grid-fitted
// before rasterization, producing crisper horizontal stems and consistent
// stem widths at small sizes (12-16px).
//
// Parameters:
//   - font: parsed font to extract outlines from
//   - gid: glyph index in the font
//   - size: font size in pixels (ppem)
//   - subpixelX: fractional X offset in pixels [0, 1) for subpixel positioning
//   - subpixelY: fractional Y offset in pixels [0, 1) for subpixel positioning
//   - hinting: hinting mode (HintingNone, HintingVertical, HintingFull)
//
// Returns nil result for empty glyphs (e.g., space character).
func (r *GlyphMaskRasterizer) RasterizeHinted(
	font ParsedFont,
	gid GlyphID,
	size float64,
	subpixelX, subpixelY float64,
	hinting Hinting,
) (*GlyphMaskResult, error) {
	// Extract outline at the target size with hinting.
	outline, err := r.extractor.ExtractOutlineHinted(font, gid, size, hinting)
	if err != nil {
		return nil, err
	}
	if outline == nil || outline.IsEmpty() {
		return nil, nil //nolint:nilnil // nil result = empty glyph, not an error
	}

	return r.rasterizeOutline(outline, subpixelX, subpixelY)
}

// RasterizeOutline renders a pre-extracted glyph outline into an R8 alpha mask.
// This is useful when the outline has already been extracted (e.g., from cache).
func (r *GlyphMaskRasterizer) RasterizeOutline(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
) (*GlyphMaskResult, error) {
	if outline == nil || outline.IsEmpty() {
		return nil, nil //nolint:nilnil // nil result = empty glyph, not an error
	}
	return r.rasterizeOutline(outline, subpixelX, subpixelY)
}

// rasterizeOutline is the internal implementation.
func (r *GlyphMaskRasterizer) rasterizeOutline(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
) (*GlyphMaskResult, error) {
	// Compute tight bounding box with subpixel offset.
	// The outline bounds are in pixel coordinates at the target size.
	// We add a 1-pixel margin for anti-aliasing coverage.
	const aaMargin = 1

	// Outline Y coordinates from sfnt are already in Y-down (screen) convention:
	// Y=0 at baseline, Y<0 above baseline, Y>0 below baseline.
	// No Y-flip needed — OutlineExtractor preserves sfnt's Y-down convention.
	boundsMinX := float64(outline.Bounds.MinX) + subpixelX
	boundsMaxX := float64(outline.Bounds.MaxX) + subpixelX
	boundsMinY := outline.Bounds.MinY + subpixelY
	boundsMaxY := outline.Bounds.MaxY + subpixelY

	// Compute pixel-aligned bounding box
	pixMinX := int(math.Floor(boundsMinX)) - aaMargin
	pixMinY := int(math.Floor(boundsMinY)) - aaMargin
	pixMaxX := int(math.Ceil(boundsMaxX)) + aaMargin
	pixMaxY := int(math.Ceil(boundsMaxY)) + aaMargin

	maskW := pixMaxX - pixMinX
	maskH := pixMaxY - pixMinY

	if maskW <= 0 || maskH <= 0 {
		return nil, nil //nolint:nilnil // degenerate bbox = no renderable content
	}

	// Safety cap: prevent absurdly large masks from bad outline data
	const maxMaskDim = 512
	if maskW > maxMaskDim || maskH > maxMaskDim {
		return nil, nil //nolint:nilnil // oversized glyph = skip rendering
	}

	// Build raster path from outline segments.
	// Translate so that the glyph bbox starts at (aaMargin, aaMargin) in the mask.
	// No Y-flip: sfnt coordinates are already Y-down (screen convention).
	offsetX := float32(-pixMinX) + float32(subpixelX)
	offsetY := float32(-pixMinY) + float32(subpixelY)

	r.pathVerbs = r.pathVerbs[:0]
	r.pathPoints = r.pathPoints[:0]

	for _, seg := range outline.Segments {
		switch seg.Op {
		case OutlineOpMoveTo:
			r.pathVerbs = append(r.pathVerbs, raster.MoveTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X+offsetX,
				seg.Points[0].Y+offsetY,
			)
		case OutlineOpLineTo:
			r.pathVerbs = append(r.pathVerbs, raster.LineTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X+offsetX,
				seg.Points[0].Y+offsetY,
			)
		case OutlineOpQuadTo:
			r.pathVerbs = append(r.pathVerbs, raster.QuadTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X+offsetX,
				seg.Points[0].Y+offsetY,
				seg.Points[1].X+offsetX,
				seg.Points[1].Y+offsetY,
			)
		case OutlineOpCubicTo:
			r.pathVerbs = append(r.pathVerbs, raster.CubicTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X+offsetX,
				seg.Points[0].Y+offsetY,
				seg.Points[1].X+offsetX,
				seg.Points[1].Y+offsetY,
				seg.Points[2].X+offsetX,
				seg.Points[2].Y+offsetY,
			)
		}
	}

	// Close the path (fonts always have closed contours)
	if len(r.pathVerbs) > 0 {
		r.pathVerbs = append(r.pathVerbs, raster.Close)
	}

	if len(r.pathVerbs) == 0 {
		return nil, nil //nolint:nilnil // no path segments = nothing to rasterize
	}

	// Build edges and fill to alpha buffer
	eb := raster.NewEdgeBuilder(2) // 4x AA (Skia default)
	eb.SetFlattenCurves(true)      // Flatten curves to lines for AnalyticFiller
	eb.BuildFromPath(&glyphPath{verbs: r.pathVerbs, points: r.pathPoints}, raster.IdentityTransform{})

	if eb.IsEmpty() {
		return nil, nil //nolint:nilnil // no edges produced = nothing to rasterize
	}

	// Rasterize to alpha buffer
	mask := make([]byte, maskW*maskH)
	raster.FillToBuffer(eb, maskW, maskH, raster.FillRuleNonZero, mask)

	// Compute bearings: offset from glyph origin to mask top-left.
	// BearingX: horizontal offset in pixels (negative = left of origin).
	// BearingY: vertical offset in pixels (positive = above baseline).
	// In Y-down coords, pixMinY is negative for above-baseline content,
	// so -pixMinY gives positive distance above baseline.
	bearingX := float32(pixMinX) - float32(subpixelX)
	bearingY := float32(-pixMinY) + float32(subpixelY)

	return &GlyphMaskResult{
		Mask:     mask,
		Width:    maskW,
		Height:   maskH,
		BearingX: bearingX,
		BearingY: bearingY,
	}, nil
}

// RasterizeLCD renders a glyph with 3x horizontal oversampling for LCD
// subpixel (ClearType) rendering. The glyph outline is rasterized at 3x
// horizontal width, then the LCD filter is applied row-by-row to produce
// per-channel RGB coverage. The result is stored in the R8 atlas at 3x width
// (3 atlas texels per logical pixel: R, G, B coverage).
//
// For BGR layout, the R and B channels are swapped after filtering.
//
// Parameters:
//   - font: parsed font to extract outlines from
//   - gid: glyph index in the font
//   - size: font size in pixels (ppem)
//   - subpixelX: fractional X offset in pixels [0, 1) for subpixel positioning
//   - subpixelY: fractional Y offset in pixels [0, 1) for subpixel positioning
//   - hinting: hinting mode (HintingNone, HintingVertical, HintingFull)
//   - filter: LCD FIR filter for fringe reduction
//   - layout: physical subpixel arrangement (RGB or BGR)
//
// Returns nil result for empty glyphs (e.g., space character).
func (r *GlyphMaskRasterizer) RasterizeLCD(
	font ParsedFont,
	gid GlyphID,
	size float64,
	subpixelX, subpixelY float64,
	hinting Hinting,
	filter LCDFilter,
	layout LCDLayout,
) (*LCDMaskResult, error) {
	// Extract outline at the target size with hinting.
	outline, err := r.extractor.ExtractOutlineHinted(font, gid, size, hinting)
	if err != nil {
		return nil, err
	}
	if outline == nil || outline.IsEmpty() {
		return nil, nil //nolint:nilnil // nil result = empty glyph, not an error
	}

	return r.rasterizeLCDOutline(outline, subpixelX, subpixelY, filter, layout)
}

// RasterizeLCDOutline renders a pre-extracted glyph outline with 3x horizontal
// oversampling for LCD subpixel rendering. This is useful when the outline has
// already been extracted (e.g., from cache).
func (r *GlyphMaskRasterizer) RasterizeLCDOutline(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
	filter LCDFilter,
	layout LCDLayout,
) (*LCDMaskResult, error) {
	if outline == nil || outline.IsEmpty() {
		return nil, nil //nolint:nilnil // nil result = empty glyph, not an error
	}
	return r.rasterizeLCDOutline(outline, subpixelX, subpixelY, filter, layout)
}

// rasterizeLCDOutline is the internal LCD rasterization implementation.
func (r *GlyphMaskRasterizer) rasterizeLCDOutline(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
	filter LCDFilter,
	layout LCDLayout,
) (*LCDMaskResult, error) {
	// Compute tight bounding box at 1x width (logical pixels).
	const aaMargin = 1

	boundsMinX := float64(outline.Bounds.MinX) + subpixelX
	boundsMaxX := float64(outline.Bounds.MaxX) + subpixelX
	boundsMinY := outline.Bounds.MinY + subpixelY
	boundsMaxY := outline.Bounds.MaxY + subpixelY

	pixMinX := int(math.Floor(boundsMinX)) - aaMargin
	pixMinY := int(math.Floor(boundsMinY)) - aaMargin
	pixMaxX := int(math.Ceil(boundsMaxX)) + aaMargin
	pixMaxY := int(math.Ceil(boundsMaxY)) + aaMargin

	maskW := pixMaxX - pixMinX // logical pixel width
	maskH := pixMaxY - pixMinY

	if maskW <= 0 || maskH <= 0 {
		return nil, nil //nolint:nilnil // degenerate bbox = no renderable content
	}

	const maxMaskDim = 512
	if maskW > maxMaskDim || maskH > maxMaskDim {
		return nil, nil //nolint:nilnil // oversized glyph = skip rendering
	}

	// Rasterize at 3x horizontal width.
	// X-coordinates are scaled by 3 in the path data, and the buffer is 3x wider.
	tripleW := maskW * 3
	offsetX := float32(-pixMinX*3) + float32(subpixelX*3)
	offsetY := float32(-pixMinY) + float32(subpixelY)

	r.pathVerbs = r.pathVerbs[:0]
	r.pathPoints = r.pathPoints[:0]

	for _, seg := range outline.Segments {
		switch seg.Op {
		case OutlineOpMoveTo:
			r.pathVerbs = append(r.pathVerbs, raster.MoveTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X*3+offsetX,
				seg.Points[0].Y+offsetY,
			)
		case OutlineOpLineTo:
			r.pathVerbs = append(r.pathVerbs, raster.LineTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X*3+offsetX,
				seg.Points[0].Y+offsetY,
			)
		case OutlineOpQuadTo:
			r.pathVerbs = append(r.pathVerbs, raster.QuadTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X*3+offsetX,
				seg.Points[0].Y+offsetY,
				seg.Points[1].X*3+offsetX,
				seg.Points[1].Y+offsetY,
			)
		case OutlineOpCubicTo:
			r.pathVerbs = append(r.pathVerbs, raster.CubicTo)
			r.pathPoints = append(r.pathPoints,
				seg.Points[0].X*3+offsetX,
				seg.Points[0].Y+offsetY,
				seg.Points[1].X*3+offsetX,
				seg.Points[1].Y+offsetY,
				seg.Points[2].X*3+offsetX,
				seg.Points[2].Y+offsetY,
			)
		}
	}

	if len(r.pathVerbs) > 0 {
		r.pathVerbs = append(r.pathVerbs, raster.Close)
	}

	if len(r.pathVerbs) == 0 {
		return nil, nil //nolint:nilnil // no path segments = nothing to rasterize
	}

	// Build edges and fill to 3x-wide alpha buffer.
	eb := raster.NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(&glyphPath{verbs: r.pathVerbs, points: r.pathPoints}, raster.IdentityTransform{})

	if eb.IsEmpty() {
		return nil, nil //nolint:nilnil // no edges produced = nothing to rasterize
	}

	oversampled := make([]byte, tripleW*maskH)
	raster.FillToBuffer(eb, tripleW, maskH, raster.FillRuleNonZero, oversampled)

	// Apply LCD filter row-by-row: 3x-wide R8 → per-pixel RGB.
	// The output is stored as 3 bytes per pixel (R, G, B coverage) which
	// will be packed into the R8 atlas at 3x width (one R8 texel per channel).
	rgbMask := make([]byte, maskW*3*maskH)
	for row := range maskH {
		srcRow := oversampled[row*tripleW : row*tripleW+tripleW]
		dstRow := rgbMask[row*maskW*3 : row*maskW*3+maskW*3]
		filter.Apply(dstRow, srcRow, maskW)
	}

	// For BGR layout, swap R and B channels in each pixel.
	if layout == LCDLayoutBGR {
		for i := 0; i < len(rgbMask)-2; i += 3 {
			rgbMask[i], rgbMask[i+2] = rgbMask[i+2], rgbMask[i]
		}
	}

	bearingX := float32(pixMinX) - float32(subpixelX)
	bearingY := float32(-pixMinY) + float32(subpixelY)

	return &LCDMaskResult{
		Mask:     rgbMask,
		Width:    maskW,
		Height:   maskH,
		BearingX: bearingX,
		BearingY: bearingY,
	}, nil
}

// glyphPath implements raster.PathLike for glyph outline data.
type glyphPath struct {
	verbs  []raster.PathVerb
	points []float32
}

func (p *glyphPath) IsEmpty() bool            { return len(p.verbs) == 0 }
func (p *glyphPath) Verbs() []raster.PathVerb { return p.verbs }
func (p *glyphPath) Points() []float32        { return p.points }
