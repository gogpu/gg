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

// rasterizeOutline is the internal AA implementation (256-level coverage).
func (r *GlyphMaskRasterizer) rasterizeOutline(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
) (*GlyphMaskResult, error) {
	// AA rasterization: 1px margin for AA fringe, aaShift=2 (4x subpixel grid),
	// 256-level coverage via FillToBuffer.
	return r.rasterizeOutlineCore(outline, subpixelX, subpixelY, 1, 2, raster.FillToBuffer)
}

// RasterizeAliased renders a single glyph into an R8 alpha mask with binary
// coverage (0 or 255 only). No anti-aliasing, no sub-pixel coverage.
//
// This matches Skia's SkFont::Edging::kAlias — the glyph outline is filled
// with integer scanline walking (NoAAFiller) instead of the AnalyticFiller.
// The result is a crisp, staircase-edged mask suitable for pixel-art
// aesthetics, terminal emulators, or bitmap font emulation.
//
// Unlike RasterizeHinted (which uses aaMargin=1 for AA fringe), aliased
// rasterization uses aaMargin=0 because there is no sub-pixel fringe.
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
func (r *GlyphMaskRasterizer) RasterizeAliased(
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

	return r.rasterizeOutlineAliased(outline, subpixelX, subpixelY)
}

// RasterizeOutlineAliased renders a pre-extracted glyph outline into an R8 alpha
// mask with binary (0 or 255) coverage. Same as RasterizeOutline but with no
// anti-aliasing — matches Skia's SkFont::Edging::kAlias applied to any outline
// source (own parser, with optional gvar variations).
func (r *GlyphMaskRasterizer) RasterizeOutlineAliased(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
) (*GlyphMaskResult, error) {
	if outline == nil || outline.IsEmpty() {
		return nil, nil //nolint:nilnil // nil result = empty glyph, not an error
	}
	return r.rasterizeOutlineAliased(outline, subpixelX, subpixelY)
}

// rasterizeOutlineAliased is the aliased (binary coverage) rasterization path.
func (r *GlyphMaskRasterizer) rasterizeOutlineAliased(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
) (*GlyphMaskResult, error) {
	// Aliased rasterization: no AA margin, aaShift=0 (integer scanline),
	// binary coverage (0/255) via FillToBufferNoAA.
	return r.rasterizeOutlineCore(outline, subpixelX, subpixelY, 0, 0, raster.FillToBufferNoAA)
}

// fillFunc is the signature for raster fill functions (FillToBuffer, FillToBufferNoAA).
type fillFunc = func(eb *raster.EdgeBuilder, width, height int, fillRule raster.FillRule, buffer []uint8)

// rasterizeOutlineCore is the shared implementation for AA and aliased glyph
// mask rasterization. The aaMargin controls the bounding box expansion (1 for
// AA fringe, 0 for aliased). The aaShift controls the EdgeBuilder sub-pixel
// grid (2 = 4x AA, 0 = integer scanline). The fill function determines
// coverage computation (FillToBuffer = 256-level, FillToBufferNoAA = binary).
func (r *GlyphMaskRasterizer) rasterizeOutlineCore(
	outline *GlyphOutline,
	subpixelX, subpixelY float64,
	aaMargin int,
	aaShift int,
	fill fillFunc,
) (*GlyphMaskResult, error) {
	// Compute tight bounding box with subpixel offset.
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

	r.buildOutlinePath(outline, offsetX, offsetY)

	if len(r.pathVerbs) == 0 {
		return nil, nil //nolint:nilnil // no path segments = nothing to rasterize
	}

	// Build edges and fill to alpha buffer.
	eb := raster.NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(&glyphPath{verbs: r.pathVerbs, points: r.pathPoints}, raster.IdentityTransform{})

	if eb.IsEmpty() {
		return nil, nil //nolint:nilnil // no edges produced = nothing to rasterize
	}

	mask := make([]byte, maskW*maskH)
	fill(eb, maskW, maskH, raster.FillRuleNonZero, mask)

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

// buildOutlinePath converts glyph outline segments into the reusable path
// buffers (pathVerbs/pathPoints). Extracted to eliminate duplication between
// rasterizeOutline, rasterizeOutlineAliased, and rasterizeLCDOutline.
func (r *GlyphMaskRasterizer) buildOutlinePath(outline *GlyphOutline, offsetX, offsetY float32) {
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

	// Close the path (fonts always have closed contours).
	if len(r.pathVerbs) > 0 {
		r.pathVerbs = append(r.pathVerbs, raster.Close)
	}
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
