package gg

import (
	"math"
	"unsafe"

	"github.com/gogpu/gg/internal/raster"
	"github.com/gogpu/gg/internal/stroke"
)

// SoftwareRenderer is a CPU-based scanline rasterizer using analytic anti-aliasing.
//
// Analytic AA computes the exact area of the shape within each pixel using
// trapezoidal integration. This provides higher quality anti-aliasing than
// supersampling approaches, with no extra memory overhead.
type SoftwareRenderer struct {
	// Analytic AA components
	edgeBuilder    *raster.EdgeBuilder
	analyticFiller *raster.AnalyticFiller

	// Dimensions (physical pixels)
	width, height int

	// HiDPI device scale factor (1.0 = no scaling).
	// Used to adjust curve flattening tolerance for sharper rendering on Retina.
	deviceScale float32

	// rasterizerMode is set by Context before calling Fill/Stroke
	// to support forced algorithm selection (RasterizerSparseStrips, etc.).
	// Reset to RasterizerAuto after each call.
	rasterizerMode RasterizerMode

	// antiAlias is set by Context before calling Fill/Stroke.
	// When false, the NoAAFiller (integer scanline, binary coverage) is used
	// instead of AnalyticFiller/CoverageFiller. Reset to true after each call.
	antiAlias bool

	// noAAFiller is the non-anti-aliased filler (lazy-initialized).
	noAAFiller *raster.NoAAFiller

	// noAAEdgeBuilder is a separate EdgeBuilder with aaShift=0 for non-AA.
	// Non-AA does not need sub-pixel edge coordinate shifting.
	noAAEdgeBuilder *raster.EdgeBuilder

	// scratchStrokePath reuses path allocation across Stroke calls.
	// Matches Skia fOuter.reset() pattern — zero per-stroke allocation.
	scratchStrokePath *Path

	// strokeMode is set by the Stroke() entry point to indicate that shared
	// blend functions should read from the stroke brush state instead of fill.
	// The Fill() entry clears this to false. This avoids threading a parameter
	// through every blend function — the renderer already carries state
	// (rasterizerMode, antiAlias) that is set contextually per-call.
	strokeMode bool

	// Clip bounds for scanline/tile skipping (ADR-052 Layer A).
	// When hasClip is true, both the AnalyticFiller and CoverageFiller
	// skip scanlines/tiles outside the clip rectangle at zero per-pixel cost.
	clipLeft, clipTop, clipRight, clipBottom int
	hasClip                                  bool
}

// NewSoftwareRenderer creates a new software renderer with analytic anti-aliasing.
func NewSoftwareRenderer(width, height int) *SoftwareRenderer {
	eb := raster.NewEdgeBuilder(2) // 4x AA (Skia default), max coord 8191px
	return &SoftwareRenderer{
		edgeBuilder:    eb,
		analyticFiller: raster.NewAnalyticFiller(width, height),
		width:          width,
		height:         height,
		deviceScale:    1.0,
		antiAlias:      true,
	}
}

// Resize updates the renderer dimensions (physical pixels).
// This should be called when the context is resized.
func (r *SoftwareRenderer) Resize(width, height int) {
	r.width = width
	r.height = height
	eb := raster.NewEdgeBuilder(2) // 4x AA (Skia default), max coord 8191px
	if r.deviceScale > 1.0 {
		eb.SetFlattenTolerance(0.1 / r.deviceScale)
	}
	r.edgeBuilder = eb
	r.analyticFiller = raster.NewAnalyticFiller(width, height)
	// Reset lazy no-AA resources so they pick up new dimensions.
	r.noAAFiller = nil
	r.noAAEdgeBuilder = nil
}

// SetAntiAlias enables or disables anti-aliasing for subsequent Fill/Stroke calls.
// When disabled, the NoAAFiller (integer scanline, binary coverage) is used
// instead of the AnalyticFiller or CoverageFiller.
//
// This method is intended for use by the scene renderer which needs to
// propagate per-draw AA state decoded from TagSetAntiAlias commands.
func (r *SoftwareRenderer) SetAntiAlias(enabled bool) {
	r.antiAlias = enabled
}

// SetRasterizerMode sets the rasterization algorithm for the next Fill call.
// RasterizerAuto (default) uses intelligent auto-selection based on path
// complexity. RasterizerAnalytic forces the scanline AnalyticFiller, which is
// required for stroke-expanded multi-contour paths where SparseStripsFiller
// cannot correctly track per-scanline winding.
//
// The mode is NOT auto-reset after each call — the caller is responsible for
// restoring it (matching the Context.doStroke pattern).
func (r *SoftwareRenderer) SetRasterizerMode(mode RasterizerMode) {
	r.rasterizerMode = mode
}

// SetDeviceScale sets the HiDPI device scale factor for the renderer.
// When scale > 1.0, curve flattening tolerance is reduced for finer
// subdivision on HiDPI displays (femtovg pattern: tol = baseTol / scale).
// This produces smoother curves at physical pixel resolution.
func (r *SoftwareRenderer) SetDeviceScale(scale float32) {
	if scale <= 0 {
		scale = 1.0
	}
	r.deviceScale = scale
	if scale > 1.0 {
		r.edgeBuilder.SetFlattenTolerance(0.1 / scale)
	}
}

// SetClipBounds sets the clip rectangle for scanline and tile skipping.
// Scanlines outside [top, bottom) and tiles outside [left, right) are skipped
// entirely at zero per-pixel cost. This is Layer A of the three-tier clip
// architecture (ADR-052, Skia SkRectClipBlitter pattern).
func (r *SoftwareRenderer) SetClipBounds(left, top, right, bottom int) {
	r.clipLeft = left
	r.clipTop = top
	r.clipRight = right
	r.clipBottom = bottom
	r.hasClip = true
	r.analyticFiller.SetClipBounds(left, top, right, bottom)
}

// ClearClipBounds removes the clip rectangle, restoring full rendering.
func (r *SoftwareRenderer) ClearClipBounds() {
	r.hasClip = false
	r.analyticFiller.ClearClipBounds()
}

// convertGGPathToCorePath converts a gg.Path to raster.PathLike.
func convertGGPathToCorePath(p *Path) raster.PathLike {
	verbs := make([]raster.PathVerb, 0, p.NumVerbs())
	points := make([]float32, 0, len(p.Coords())*2) //nolint:mnd // preallocate for float64→float32

	p.Iterate(func(verb PathVerb, coords []float64) {
		switch verb {
		case MoveTo:
			verbs = append(verbs, raster.MoveTo)
			points = append(points, float32(coords[0]), float32(coords[1]))
		case LineTo:
			verbs = append(verbs, raster.LineTo)
			points = append(points, float32(coords[0]), float32(coords[1]))
		case QuadTo:
			verbs = append(verbs, raster.QuadTo)
			points = append(points,
				float32(coords[0]), float32(coords[1]),
				float32(coords[2]), float32(coords[3]),
			)
		case CubicTo:
			verbs = append(verbs, raster.CubicTo)
			points = append(points,
				float32(coords[0]), float32(coords[1]),
				float32(coords[2]), float32(coords[3]),
				float32(coords[4]), float32(coords[5]),
			)
		case Close:
			verbs = append(verbs, raster.Close)
		}
	})

	return raster.NewScenePathAdapter(len(verbs) == 0, verbs, points)
}

const (
	// minTileArea is the minimum bounding box area (px²) for tile-based
	// rasterization. Below this, tile setup overhead exceeds scanline cost.
	// 512 = 32×16 — allows wide-but-short paths (e.g. text at 16px height)
	// while rejecting paths that are too small in both dimensions.
	minTileArea = 512

	// minSingleDimension prevents degenerate nearly-linear paths from
	// triggering tile rasterization. A 1000px × 1px line should not use tiles.
	minSingleDimension = 8

	// minElementThreshold is the absolute minimum element count for CoverageFiller.
	// Paths with fewer elements are always cheaper with scanline rasterization
	// since the per-pixel work scales linearly with edge crossings.
	minElementThreshold = 32

	// maxElementThreshold caps the adaptive threshold for tiny bounding boxes.
	// Even extremely complex paths in a small area are better handled by scanline
	// because the total pixel count is low.
	maxElementThreshold = 256
)

// adaptiveThreshold computes the element count threshold for switching from
// AnalyticFiller to CoverageFiller based on bounding box area.
// Larger bounding boxes lower the threshold because scanline cost grows with
// width (O(width * edges)) while tile-based cost grows with fill area.
// The formula 2048/sqrt(area) produces: 100x100 -> 20, 50x50 -> 29, 200x200 -> 10.
// Results are clamped to [minElementThreshold, maxElementThreshold].
func adaptiveThreshold(bboxArea float64) int {
	if bboxArea <= 0 {
		return maxElementThreshold
	}
	threshold := int(2048.0 / math.Sqrt(bboxArea))
	if threshold < minElementThreshold {
		return minElementThreshold
	}
	if threshold > maxElementThreshold {
		return maxElementThreshold
	}
	return threshold
}

// pathBounds computes the axis-aligned bounding box of a path by iterating
// over all path elements. Returns (minX, minY, maxX, maxY).
// For an empty path, returns (0, 0, 0, 0).
func pathBounds(p *Path) (minX, minY, maxX, maxY float64) {
	if p.isEmpty() {
		return 0, 0, 0, 0
	}

	minX = math.MaxFloat64
	minY = math.MaxFloat64
	maxX = -math.MaxFloat64
	maxY = -math.MaxFloat64

	expandPt := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	p.Iterate(func(verb PathVerb, coords []float64) {
		switch verb {
		case MoveTo, LineTo:
			expandPt(coords[0], coords[1])
		case QuadTo:
			expandPt(coords[0], coords[1])
			expandPt(coords[2], coords[3])
		case CubicTo:
			expandPt(coords[0], coords[1])
			expandPt(coords[2], coords[3])
			expandPt(coords[4], coords[5])
		}
	})

	return minX, minY, maxX, maxY
}

// shouldUseTileRasterizer returns true if the path is complex enough to
// benefit from tile-based rasterization. It uses a bounding box area check
// (not per-dimension) so that wide-but-short dense paths (e.g. text outlines
// at 16px with hundreds of path elements) can be routed to the tile rasterizer.
func shouldUseTileRasterizer(p *Path) bool {
	nElems := p.NumVerbs()
	if nElems <= 0 {
		return false
	}

	x1, y1, x2, y2 := pathBounds(p)
	bboxW := x2 - x1
	bboxH := y2 - y1

	// Area check: tile setup overhead is only worthwhile when there is
	// enough fill area. This replaces the old per-dimension check
	// (bboxMinDimension=32) which rejected wide-but-short text paths.
	bboxArea := bboxW * bboxH
	if bboxArea < minTileArea {
		return false
	}

	// Require at least half-tile in each dimension to avoid degenerate
	// nearly-linear paths (e.g. 1000px × 1px hairline).
	if bboxW < minSingleDimension || bboxH < minSingleDimension {
		return false
	}

	return nElems > adaptiveThreshold(bboxArea)
}

// fillWithCoverageFiller rasterizes the path using the tile-based CoverageFiller
// and composites the result onto the pixmap.
func (r *SoftwareRenderer) fillWithCoverageFiller(
	pixmap *Pixmap, p *Path, paint *Paint, filler CoverageFiller,
) {
	fillRule := FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = FillRuleEvenOdd
	}

	// Layer A (ADR-052): pass clip bounds to tile filler for tile skipping.
	var cb *ClipBounds
	if r.hasClip {
		cb = &ClipBounds{
			XMin: r.clipLeft, YMin: r.clipTop,
			XMax: r.clipRight, YMax: r.clipBottom,
		}
	}

	clipFn := paint.ClipCoverage
	maskFn := paint.MaskCoverage
	clipMask := paint.ClipMask
	clipMaskW := paint.ClipMaskW
	clipMaskX := paint.ClipMaskX
	clipMaskY := paint.ClipMaskY
	bfn := lookupBlendFuncForPaint(paint)
	if color, ok := solidColorForMode(paint, r.strokeMode); ok {
		r.fillCoverageSolidPath(pixmap, p, filler, fillRule, cb, color, clipFn, maskFn, clipMask, clipMaskW, clipMaskX, clipMaskY, bfn)
	} else {
		r.fillCoveragePaintPath(pixmap, p, filler, fillRule, cb, paint, clipFn, maskFn, clipMask, clipMaskW, clipMaskX, clipMaskY, bfn)
	}
}

// fillCoverageSolidPath fills using the CoverageFiller with a solid color,
// applying optional clip and mask coverage.
// When bfn is nil, uses the fast SrcOver path; otherwise dispatches per-pixel.
func (r *SoftwareRenderer) fillCoverageSolidPath(
	pixmap *Pixmap, p *Path, filler CoverageFiller,
	fillRule FillRule, cb *ClipBounds, color RGBA,
	clipFn func(x, y float64) byte, maskFn func(x, y int) uint8,
	clipMask []uint8, clipMaskW, clipMaskX, clipMaskY int,
	bfn blendFunc,
) {
	if bfn == nil {
		// Fast path: SrcOver — zero overhead, inline formula.
		filler.FillCoverage(p, r.width, r.height, fillRule, cb,
			func(x, y int, coverage uint8) {
				coverage = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, coverage)
				coverage = applyMaskCoverage(maskFn, x, y, coverage)
				if coverage == 0 {
					return
				}
				r.blendCoverageSolid(pixmap, x, y, coverage, color)
			})
		return
	}

	// Non-SrcOver: apply blend function per-pixel.
	filler.FillCoverage(p, r.width, r.height, fillRule, cb,
		func(x, y int, coverage uint8) {
			coverage = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, coverage)
			coverage = applyMaskCoverage(maskFn, x, y, coverage)
			if coverage == 0 {
				return
			}
			r.blendCoverageSolidMode(pixmap, x, y, coverage, color, bfn)
		})
}

// fillCoveragePaintPath fills using the CoverageFiller with a paint pattern,
// applying optional clip and mask coverage.
// When bfn is nil, uses the fast SrcOver path; otherwise dispatches per-pixel.
func (r *SoftwareRenderer) fillCoveragePaintPath(
	pixmap *Pixmap, p *Path, filler CoverageFiller,
	fillRule FillRule, cb *ClipBounds, paint *Paint,
	clipFn func(x, y float64) byte, maskFn func(x, y int) uint8,
	clipMask []uint8, clipMaskW, clipMaskX, clipMaskY int,
	bfn blendFunc,
) {
	colorAt := paint.colorAtForMode(r.strokeMode)
	if bfn == nil {
		filler.FillCoverage(p, r.width, r.height, fillRule, cb,
			func(x, y int, coverage uint8) {
				coverage = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, coverage)
				coverage = applyMaskCoverage(maskFn, x, y, coverage)
				if coverage == 0 {
					return
				}
				c := colorAt(float64(x)+0.5, float64(y)+0.5)
				r.blendCoverageSolid(pixmap, x, y, coverage, c)
			})
		return
	}

	filler.FillCoverage(p, r.width, r.height, fillRule, cb,
		func(x, y int, coverage uint8) {
			coverage = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, coverage)
			coverage = applyMaskCoverage(maskFn, x, y, coverage)
			if coverage == 0 {
				return
			}
			c := colorAt(float64(x)+0.5, float64(y)+0.5)
			r.blendCoverageSolidMode(pixmap, x, y, coverage, c, bfn)
		})
}

// applyClipCoverageFromMaskOrFn applies clip coverage using pre-rasterized mask
// (Layer B, ADR-052) with legacy closure fallback. The mask lookup (~0.5ns/pixel)
// replaces the per-pixel closure call (~8ns/pixel) for non-rect clips.
//
// Priority: clipMask > clipFn > no-op (both nil).
func applyClipCoverageFromMaskOrFn(
	clipMask []uint8, clipMaskW, clipMaskX, clipMaskY int,
	clipFn func(x, y float64) byte,
	px, py int, coverage uint8,
) uint8 {
	if len(clipMask) > 0 {
		return applyClipFromMask(clipMask, clipMaskW, clipMaskX, clipMaskY, px, py, coverage)
	}
	// Legacy fallback: per-pixel closure.
	return applyClipCoverage(clipFn, px, py, coverage)
}

// applyClipFromMask looks up clip coverage from a pre-rasterized alpha mask.
// Returns 0 for pixels outside mask bounds (fully clipped).
func applyClipFromMask(clipMask []uint8, clipMaskW, clipMaskX, clipMaskY, px, py int, coverage uint8) uint8 {
	mx := px - clipMaskX
	my := py - clipMaskY
	if mx < 0 || my < 0 || mx >= clipMaskW {
		return 0
	}
	idx := my*clipMaskW + mx
	if idx < 0 || idx >= len(clipMask) {
		return 0
	}
	cc := clipMask[idx]
	if cc == 0 {
		return 0
	}
	if cc == 255 {
		return coverage
	}
	return uint8(uint16(coverage) * uint16(cc) / 255)
}

// applyClipCoverage multiplies pixel coverage by the clip mask coverage.
// Returns 0 if the pixel is fully clipped. When clipFn is nil, returns the
// original coverage unchanged.
//
// Deprecated: Use applyClipCoverageFromMaskOrFn for new code paths.
// Kept for backward compatibility in existing AnalyticFiller blend paths.
func applyClipCoverage(clipFn func(x, y float64) byte, px, py int, coverage uint8) uint8 {
	if clipFn == nil {
		return coverage
	}
	cc := clipFn(float64(px)+0.5, float64(py)+0.5)
	if cc == 0 {
		return 0
	}
	return uint8(uint16(coverage) * uint16(cc) / 255)
}

// applyMaskCoverage multiplies pixel coverage by the alpha mask coverage.
// Returns 0 if the pixel is fully masked out. When maskFn is nil, returns the
// original coverage unchanged. Uses int coords because masks are pixel-aligned.
func applyMaskCoverage(maskFn func(x, y int) uint8, px, py int, coverage uint8) uint8 {
	if maskFn == nil {
		return coverage
	}
	mc := maskFn(px, py)
	if mc == 0 {
		return 0
	}
	if mc == 255 {
		return coverage
	}
	return uint8(uint16(coverage) * uint16(mc) / 255)
}

// Fill implements Renderer.Fill using analytic anti-aliasing.
// For complex paths, it auto-selects the registered CoverageFiller (tile-based
// rasterizer) when available, using an adaptive threshold based on both path
// element count and bounding box area.
//
// When rasterizerMode is set (via Context.SetRasterizerMode), the forced
// algorithm is used instead of auto-selection.
func (r *SoftwareRenderer) Fill(pixmap *Pixmap, p *Path, paint *Paint) error {
	// strokeMode is set by the caller (Context.doFill → false, Context.doStroke → true)
	// or by SoftwareRenderer.Stroke → true. Fill does NOT override it because
	// Stroke expands the stroke to a fill path and calls Fill internally —
	// overriding strokeMode here would break the stroke color routing.

	// Non-AA path: completely separate code path (Skia/tiny-skia pattern).
	// Integer scanline, binary coverage, no CoverageFiller/AnalyticFiller.
	if !r.antiAlias {
		return r.fillNoAA(pixmap, p, paint)
	}

	// Force mode: specific algorithm without auto-selection.
	switch r.rasterizerMode {
	case RasterizerAnalytic:
		// Skip CoverageFiller entirely → always use AnalyticFiller below.

	case RasterizerSparseStrips:
		if filler := r.forcedFiller(RasterizerSparseStrips); filler != nil {
			r.fillWithCoverageFiller(pixmap, p, paint, filler)
			return nil
		}

	case RasterizerTileCompute:
		if filler := r.forcedFiller(RasterizerTileCompute); filler != nil {
			r.fillWithCoverageFiller(pixmap, p, paint, filler)
			return nil
		}

	default: // RasterizerAuto
		// Auto-selection: use CoverageFiller for complex paths.
		if filler := GetCoverageFiller(); filler != nil && shouldUseTileRasterizer(p) {
			r.fillWithCoverageFiller(pixmap, p, paint, filler)
			return nil
		}
	}

	// AnalyticFiller path (scanline) — simple paths, forced analytic, or no filler
	r.edgeBuilder.Reset()
	r.analyticFiller.Reset()

	// Clip paths to canvas bounds to prevent FDot6→FDot16 integer overflow.
	// At aaShift=4, coordinates > 2048px overflow int32 in FDot16, causing
	// silent wrap-around that places edges at wrong positions (RAST-010).
	// Small margin for AA bleed — coordinates at canvas+2px are still well
	// within safe range.
	clipMargin := float32(2)
	clipRect := raster.Rect{
		MinX: -clipMargin,
		MinY: -clipMargin,
		MaxX: float32(pixmap.Width()) + clipMargin,
		MaxY: float32(pixmap.Height()) + clipMargin,
	}
	r.edgeBuilder.SetClipRect(&clipRect)

	// Native forward-diff curve edges (Skia AAA pattern, ADR-063).
	// 30% faster than geometric flattening, max diff 7 vs Skia golden.
	// VelloLines populated separately for CoverageFiller compatibility.
	r.edgeBuilder.SetFlattenCurves(false)
	defer r.edgeBuilder.SetFlattenCurves(true)

	// Build edges from the path directly from float64 coords (zero-alloc).
	// PathVerb values match between gg and raster packages (both 0-4 iota).
	// gg.PathVerb is byte, so []PathVerb has identical memory layout to []byte.
	verbs := p.Verbs()
	if len(verbs) > 0 {
		verbBytes := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(verbs))), len(verbs))
		r.edgeBuilder.BuildFromPathF64(verbBytes, p.Coords())
	}

	// If no edges, nothing to fill
	if r.edgeBuilder.IsEmpty() {
		return nil
	}

	// Convert fill rule
	coreFillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		coreFillRule = raster.FillRuleEvenOdd
	}

	bfn := lookupBlendFuncForPaint(paint)
	if color, ok := solidColorForMode(paint, r.strokeMode); ok {
		// Fast path: solid color
		clipFn := paint.ClipCoverage
		maskFn := paint.MaskCoverage
		cm, cmW, cmX, cmY := paint.ClipMask, paint.ClipMaskW, paint.ClipMaskX, paint.ClipMaskY
		r.analyticFiller.Fill(r.edgeBuilder, coreFillRule, func(y int, runs *raster.AlphaRuns) {
			r.blendAlphaRunsFromCoreRuns(pixmap, y, runs, color, clipFn, maskFn, cm, cmW, cmX, cmY, bfn)
		})
	} else {
		// Pattern/gradient path: per-pixel color sampling
		clipFn := paint.ClipCoverage
		maskFn := paint.MaskCoverage
		cm, cmW, cmX, cmY := paint.ClipMask, paint.ClipMaskW, paint.ClipMaskX, paint.ClipMaskY
		r.analyticFiller.Fill(r.edgeBuilder, coreFillRule, func(y int, runs *raster.AlphaRuns) {
			r.blendAlphaRunsFromCoreRunsPaint(pixmap, y, runs, paint, clipFn, maskFn, cm, cmW, cmX, cmY, bfn)
		})
	}

	return nil
}

// fillNoAA renders a filled path without anti-aliasing.
// Uses a dedicated NoAAFiller that produces solid horizontal spans with
// binary coverage (0 or 255). This is a completely separate code path
// from the AA rasterizer (Skia SkScan::FillPath / tiny-skia scan::path pattern).
func (r *SoftwareRenderer) fillNoAA(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Lazy-init the no-AA edge builder and filler.
	if r.noAAEdgeBuilder == nil {
		r.noAAEdgeBuilder = raster.NewEdgeBuilder(0) // aaShift=0: no sub-pixel
		if r.deviceScale > 1.0 {
			r.noAAEdgeBuilder.SetFlattenTolerance(0.1 / r.deviceScale)
		}
	}
	if r.noAAFiller == nil {
		r.noAAFiller = raster.NewNoAAFiller(r.width, r.height)
	}

	r.noAAEdgeBuilder.Reset()

	clipMargin := float32(2)
	clipRect := raster.Rect{
		MinX: -clipMargin,
		MinY: -clipMargin,
		MaxX: float32(pixmap.Width()) + clipMargin,
		MaxY: float32(pixmap.Height()) + clipMargin,
	}
	r.noAAEdgeBuilder.SetClipRect(&clipRect)

	r.noAAEdgeBuilder.SetFlattenCurves(false)
	defer r.noAAEdgeBuilder.SetFlattenCurves(true)

	verbs := p.Verbs()
	if len(verbs) > 0 {
		verbBytes := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(verbs))), len(verbs))
		r.noAAEdgeBuilder.BuildFromPathF64(verbBytes, p.Coords())
	}

	if r.noAAEdgeBuilder.IsEmpty() {
		return nil
	}

	coreFillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		coreFillRule = raster.FillRuleEvenOdd
	}

	clipFn := paint.ClipCoverage
	maskFn := paint.MaskCoverage

	bfn := lookupBlendFuncForPaint(paint)
	if color, ok := solidColorForMode(paint, r.strokeMode); ok {
		r.noAAFiller.Fill(r.noAAEdgeBuilder, coreFillRule, func(y, left, spanWidth int) {
			r.blitNoAASolidSpan(pixmap, y, left, spanWidth, color, clipFn, maskFn, bfn)
		})
	} else {
		r.noAAFiller.Fill(r.noAAEdgeBuilder, coreFillRule, func(y, left, spanWidth int) {
			r.blitNoAAPaintSpan(pixmap, y, left, spanWidth, paint, clipFn, maskFn, bfn)
		})
	}

	return nil
}

// blitNoAASolidSpan blits a solid-color span with optional clip and mask.
func (r *SoftwareRenderer) blitNoAASolidSpan(
	pixmap *Pixmap, y, left, spanWidth int, color RGBA,
	clipFn func(float64, float64) byte, maskFn func(int, int) uint8,
	bfn blendFunc,
) {
	if bfn != nil {
		for x := left; x < left+spanWidth; x++ {
			cov := noaaPixelCoverage(x, y, clipFn, maskFn)
			if cov == 0 {
				continue
			}
			r.blendCoverageSolidMode(pixmap, x, y, cov, color, bfn)
		}
		return
	}
	for x := left; x < left+spanWidth; x++ {
		cov := noaaPixelCoverage(x, y, clipFn, maskFn)
		if cov == 0 {
			continue
		}
		r.blendCoverageSolid(pixmap, x, y, cov, color)
	}
}

// blitNoAAPaintSpan blits a paint-sampled span with optional clip and mask.
// Reads from fill or stroke brush state based on r.strokeMode.
func (r *SoftwareRenderer) blitNoAAPaintSpan(
	pixmap *Pixmap, y, left, spanWidth int, paint *Paint,
	clipFn func(float64, float64) byte, maskFn func(int, int) uint8,
	bfn blendFunc,
) {
	colorAt := paint.colorAtForMode(r.strokeMode)
	if bfn != nil {
		for x := left; x < left+spanWidth; x++ {
			cov := noaaPixelCoverage(x, y, clipFn, maskFn)
			if cov == 0 {
				continue
			}
			c := colorAt(float64(x)+0.5, float64(y)+0.5)
			r.blendCoverageSolidMode(pixmap, x, y, cov, c, bfn)
		}
		return
	}
	for x := left; x < left+spanWidth; x++ {
		cov := noaaPixelCoverage(x, y, clipFn, maskFn)
		if cov == 0 {
			continue
		}
		c := colorAt(float64(x)+0.5, float64(y)+0.5)
		r.blendCoverageSolid(pixmap, x, y, cov, c)
	}
}

// noaaPixelCoverage computes per-pixel coverage from clip and mask functions.
// Returns 0 if the pixel is fully clipped/masked, 255 if no clip/mask is active.
func noaaPixelCoverage(x, y int, clipFn func(float64, float64) byte, maskFn func(int, int) uint8) byte {
	cov := byte(255)
	if clipFn != nil {
		clipCov := clipFn(float64(x)+0.5, float64(y)+0.5)
		if clipCov == 0 {
			return 0
		}
		cov = clipCov
	}
	if maskFn != nil {
		mc := maskFn(x, y)
		if mc == 0 {
			return 0
		}
		if cov != 255 {
			cov = uint8(uint16(cov) * uint16(mc) / 255)
		} else {
			cov = mc
		}
	}
	return cov
}

// clipMaskCoverage looks up clip mask coverage for a pixel from a pre-rasterized
// mask (Layer B, ADR-052). Returns (coverage, true) if the mask is non-nil and
// covers the pixel, or (0, false) if no mask is active.
func clipMaskCoverage(clipMask []uint8, clipMaskW, clipMaskX, clipMaskY, px, py int) (uint8, bool) {
	if len(clipMask) == 0 {
		return 0, false
	}
	mx := px - clipMaskX
	my := py - clipMaskY
	if mx < 0 || my < 0 || mx >= clipMaskW {
		return 0, true // Outside mask = fully clipped.
	}
	idx := my*clipMaskW + mx
	if idx < 0 || idx >= len(clipMask) {
		return 0, true
	}
	return clipMask[idx], true
}

// blendCoverageSolid blends a single pixel with solid color and coverage.
// Uses premultiplied source-over compositing.
func (r *SoftwareRenderer) blendCoverageSolid(pixmap *Pixmap, x, y int, coverage uint8, color RGBA) {
	if x < 0 || x >= pixmap.Width() || y < 0 || y >= pixmap.Height() {
		return
	}

	if coverage == 255 && color.A == 1.0 {
		pixmap.SetPixel(x, y, color)
		return
	}

	srcAlpha := color.A * float64(coverage) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	srcR := color.R * srcAlpha
	srcG := color.G * srcAlpha
	srcB := color.B * srcAlpha

	dstR, dstG, dstB, dstA := pixmap.getPremul(x, y)

	pixmap.setPremul(x, y,
		srcR+dstR*invSrcAlpha,
		srcG+dstG*invSrcAlpha,
		srcB+dstB*invSrcAlpha,
		srcAlpha+dstA*invSrcAlpha,
	)
}

// blendCoverageSolidMode blends a single pixel with solid color, coverage, and
// an arbitrary blend mode. Coverage is applied AFTER blending via a lerp with
// the original destination (the correct approach for non-Porter-Duff modes where
// coverage cannot be pre-scaled into source alpha).
//
// Reference: tiny-skia blend_mode.rs:68 (should_pre_scale_coverage).
func (r *SoftwareRenderer) blendCoverageSolidMode(
	pixmap *Pixmap, x, y int, coverage uint8, color RGBA, fn blendFunc,
) {
	if x < 0 || x >= pixmap.Width() || y < 0 || y >= pixmap.Height() {
		return
	}

	// Convert source color to premultiplied byte.
	sr := floatToByte(color.R * color.A)
	sg := floatToByte(color.G * color.A)
	sb := floatToByte(color.B * color.A)
	sa := floatToByte(color.A)

	// Read destination as premultiplied bytes.
	dstR, dstG, dstB, dstA := pixmap.getPremul(x, y)
	dr := floatToByte(dstR)
	dg := floatToByte(dstG)
	db := floatToByte(dstB)
	da := floatToByte(dstA)

	// Apply blend function (premultiplied bytes).
	br, bg, bb, ba := fn(sr, sg, sb, sa, dr, dg, db, da)

	if coverage == 255 {
		pixmap.setPremul(x, y,
			byteToFloat(br), byteToFloat(bg),
			byteToFloat(bb), byteToFloat(ba))
		return
	}

	// lerp(dst, blended, coverage/255)
	outR := lerpByte(dr, br, coverage)
	outG := lerpByte(dg, bg, coverage)
	outB := lerpByte(db, bb, coverage)
	outA := lerpByte(da, ba, coverage)

	pixmap.setPremul(x, y,
		byteToFloat(outR), byteToFloat(outG),
		byteToFloat(outB), byteToFloat(outA))
}

// floatToByte converts a premultiplied float64 [0,1] to byte [0,255].
func floatToByte(v float64) byte {
	f := v*255.0 + 0.5
	if f < 0 {
		return 0
	}
	if f > 255 {
		return 255
	}
	return byte(f)
}

// byteToFloat converts a byte [0,255] to float64 [0,1].
func byteToFloat(b byte) float64 {
	return float64(b) / 255.0
}

// lerpByte linearly interpolates between a and b by t/255.
func lerpByte(a, b, t byte) byte {
	diff := int16(b) - int16(a)
	result := int16(a) + int16((int32(diff)*int32(t)+127)/255)
	if result < 0 {
		return 0
	}
	if result > 255 {
		return 255
	}
	return byte(result)
}

// blendCoveragePaint blends a single pixel with paint-sampled color and coverage.
// Uses premultiplied source-over compositing.
// Reads from fill or stroke brush state based on r.strokeMode.
func (r *SoftwareRenderer) blendCoveragePaint(pixmap *Pixmap, x, y int, coverage uint8, paint *Paint) {
	if x < 0 || x >= pixmap.Width() || y < 0 || y >= pixmap.Height() {
		return
	}

	var color RGBA
	if r.strokeMode {
		color = paint.StrokeColorAt(float64(x)+0.5, float64(y)+0.5)
	} else {
		color = paint.FillColorAt(float64(x)+0.5, float64(y)+0.5)
	}

	if coverage == 255 && color.A == 1.0 {
		pixmap.SetPixel(x, y, color)
		return
	}

	srcAlpha := color.A * float64(coverage) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	srcR := color.R * srcAlpha
	srcG := color.G * srcAlpha
	srcB := color.B * srcAlpha

	dstR, dstG, dstB, dstA := pixmap.getPremul(x, y)

	pixmap.setPremul(x, y,
		srcR+dstR*invSrcAlpha,
		srcG+dstG*invSrcAlpha,
		srcB+dstB*invSrcAlpha,
		srcAlpha+dstA*invSrcAlpha,
	)
}

// forcedFiller returns the CoverageFiller for a forced rasterizer mode.
// If the registered filler implements ForceableFiller, the specific sub-filler
// (SparseStrips or TileCompute) is returned. Otherwise, the filler is used as-is.
func (r *SoftwareRenderer) forcedFiller(mode RasterizerMode) CoverageFiller {
	filler := GetCoverageFiller()
	if filler == nil {
		return nil
	}
	ff, ok := filler.(ForceableFiller)
	if !ok {
		return filler
	}
	switch mode {
	case RasterizerSparseStrips:
		return ff.SparseFiller()
	case RasterizerTileCompute:
		return ff.ComputeFiller()
	default:
		return filler
	}
}

// solidColorFromPaint returns the solid fill color if paint fill is solid.
// Returns (color, true) for solid paints, (zero, false) for patterns/gradients.
//
// Deprecated: Use solidColorForMode for fill/stroke-aware color extraction.
func solidColorFromPaint(paint *Paint) (RGBA, bool) {
	return solidColorFromBrushState(&paint.fill)
}

// solidColorForMode returns the solid color from the fill or stroke side,
// depending on strokeMode.
func solidColorForMode(paint *Paint, strokeMode bool) (RGBA, bool) {
	if strokeMode {
		return solidColorFromBrushState(&paint.stroke)
	}
	return solidColorFromBrushState(&paint.fill)
}

// solidColorFromBrushState extracts a solid color from a brushState.
func solidColorFromBrushState(s *brushState) (RGBA, bool) {
	if s.isSolid {
		return s.solidColor, true
	}
	if s.brush != nil {
		if sb, ok := s.brush.(SolidBrush); ok {
			return sb.Color, true
		}
		return RGBA{}, false
	}
	if sp, ok := s.pattern.(*SolidPattern); ok {
		return sp.Color, true
	}
	return RGBA{}, false
}

// blendAlphaRunsFromCoreRuns blends alpha values from raster.AlphaRuns to the pixmap.
// Uses source-over compositing for proper alpha blending.
// When clipFn is non-nil, each pixel's alpha is multiplied by the clip coverage.
// When maskFn is non-nil, each pixel's alpha is multiplied by the mask coverage.
func (r *SoftwareRenderer) blendAlphaRunsFromCoreRuns(pixmap *Pixmap, y int, runs *raster.AlphaRuns, color RGBA,
	clipFn func(x, y float64) byte, maskFn func(x, y int) uint8,
	clipMask []uint8, clipMaskW, clipMaskX, clipMaskY int,
	bfn blendFunc,
) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	// Non-SourceOver: dispatch via blend function.
	if bfn != nil {
		for x, alpha := range runs.Iter() {
			if alpha == 0 || x < 0 || x >= pixmap.Width() {
				continue
			}
			alpha = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, alpha)
			if alpha == 0 {
				continue
			}
			alpha = applyMaskCoverage(maskFn, x, y, alpha)
			if alpha == 0 {
				continue
			}
			r.blendCoverageSolidMode(pixmap, x, y, alpha, color, bfn)
		}
		return
	}

	// Fast path: SourceOver — inline float64 formula (zero overhead).
	for x, alpha := range runs.Iter() {
		if alpha == 0 {
			continue
		}
		if x < 0 || x >= pixmap.Width() {
			continue
		}

		// Apply clip coverage: pre-rasterized mask (Layer B, ADR-052) has
		// priority over legacy closure. Unified with CoverageFiller path.
		alpha = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, alpha)
		if alpha == 0 {
			continue
		}

		// Apply mask coverage if active.
		alpha = applyMaskCoverage(maskFn, x, y, alpha)
		if alpha == 0 {
			continue
		}

		// Full coverage - just set the pixel
		if alpha == 255 && color.A == 1.0 {
			pixmap.SetPixel(x, y, color)
			continue
		}

		// Partial coverage - premultiplied source-over compositing
		srcAlpha := color.A * float64(alpha) / 255.0
		invSrcAlpha := 1.0 - srcAlpha

		srcR := color.R * srcAlpha
		srcG := color.G * srcAlpha
		srcB := color.B * srcAlpha

		dstR, dstG, dstB, dstA := pixmap.getPremul(x, y)

		pixmap.setPremul(x, y,
			srcR+dstR*invSrcAlpha,
			srcG+dstG*invSrcAlpha,
			srcB+dstB*invSrcAlpha,
			srcAlpha+dstA*invSrcAlpha,
		)
	}
}

// blendAlphaRunsFromCoreRunsPaint is like blendAlphaRunsFromCoreRuns but samples
// the paint color at each pixel instead of using a single constant color.
// Reads from fill or stroke brush state based on r.strokeMode.
// When clipFn is non-nil, each pixel's alpha is multiplied by the clip coverage.
// When maskFn is non-nil, each pixel's alpha is multiplied by the mask coverage.
func (r *SoftwareRenderer) blendAlphaRunsFromCoreRunsPaint(pixmap *Pixmap, y int, runs *raster.AlphaRuns, paint *Paint,
	clipFn func(x, y float64) byte, maskFn func(x, y int) uint8,
	clipMask []uint8, clipMaskW, clipMaskX, clipMaskY int,
	bfn blendFunc,
) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	colorAt := paint.colorAtForMode(r.strokeMode)

	// Non-SourceOver: dispatch via blend function.
	if bfn != nil {
		for x, alpha := range runs.Iter() {
			if alpha == 0 || x < 0 || x >= pixmap.Width() {
				continue
			}
			alpha = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, alpha)
			if alpha == 0 {
				continue
			}
			alpha = applyMaskCoverage(maskFn, x, y, alpha)
			if alpha == 0 {
				continue
			}
			c := colorAt(float64(x)+0.5, float64(y)+0.5)
			r.blendCoverageSolidMode(pixmap, x, y, alpha, c, bfn)
		}
		return
	}

	// Fast path: SourceOver — inline float64 formula (zero overhead).
	for x, alpha := range runs.Iter() {
		if alpha == 0 {
			continue
		}
		if x < 0 || x >= pixmap.Width() {
			continue
		}

		// Apply clip coverage: mask > closure > no-op (ADR-052).
		alpha = applyClipCoverageFromMaskOrFn(clipMask, clipMaskW, clipMaskX, clipMaskY, clipFn, x, y, alpha)
		if alpha == 0 {
			continue
		}

		// Apply mask coverage if active.
		alpha = applyMaskCoverage(maskFn, x, y, alpha)
		if alpha == 0 {
			continue
		}

		fx := float64(x) + 0.5
		fy := float64(y) + 0.5

		// Sample color from paint at pixel center
		color := colorAt(fx, fy)

		if alpha == 255 && color.A == 1.0 {
			pixmap.SetPixel(x, y, color)
			continue
		}

		srcAlpha := color.A * float64(alpha) / 255.0
		invSrcAlpha := 1.0 - srcAlpha

		srcR := color.R * srcAlpha
		srcG := color.G * srcAlpha
		srcB := color.B * srcAlpha

		dstR, dstG, dstB, dstA := pixmap.getPremul(x, y)

		pixmap.setPremul(x, y,
			srcR+dstR*invSrcAlpha,
			srcG+dstG*invSrcAlpha,
			srcB+dstB*invSrcAlpha,
			srcAlpha+dstA*invSrcAlpha,
		)
	}
}

// treatAsHairline checks whether a stroke with the given device-pixel width
// should be rendered as a hairline (direct line rasterization) rather than
// through stroke expansion. This follows the Skia/tiny-skia pattern:
//
//   - Width == 0: always hairline, coverage = 1.0
//   - Width <= 1.0 (with anti-alias): hairline, coverage = width
//     (modulate paint alpha by coverage for sub-pixel strokes)
//   - Width > 1.0: normal stroke expansion
//
// Reference: tiny-skia painter.rs treat_as_hairline()
func treatAsHairline(effectiveWidth float64) (isHairline bool, coverage float64) {
	if effectiveWidth <= 0 {
		return true, 1.0
	}
	// Only truly sub-pixel strokes use hairline. Width >= 1.0 goes through
	// normal stroke expansion with Skia cubicStroke port for correct geometry.
	if effectiveWidth < 1.0 {
		return true, effectiveWidth
	}
	return false, 0
}

// Stroke implements Renderer.Stroke with anti-aliasing support.
// Strokes are expanded to fill paths and rendered with the Fill method,
// which provides analytic anti-aliased results.
//
// For hairline strokes (device-pixel width <= 1.0), the stroke is rendered
// with width=1.0 and the paint alpha is modulated by coverage (Skia/tiny-skia
// pattern). For circles, SDF coverage is used directly. This avoids the
// coverage loss from stroke expansion on thin strokes where the expanded
// shape straddles pixel boundaries.
func (r *SoftwareRenderer) Stroke(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Stroke reads from the stroke brush state.
	r.strokeMode = true
	defer func() { r.strokeMode = false }()

	// Get effective line width
	width := paint.EffectiveLineWidth()

	// Get transform scale for dash pattern scaling
	transformScale := paint.TransformScale
	if transformScale <= 0 {
		transformScale = 1.0
	}

	// Apply dash pattern if set
	// Scale dash pattern by transform scale (Cairo/Skia convention)
	pathToDraw := p
	if paint.IsDashed() {
		dash := paint.EffectiveDash()
		if transformScale > 1.0 {
			dash = dash.Scale(transformScale)
		}
		pathToDraw = dashPath(p, dash)
	}

	// Hairline detection (Skia/tiny-skia pattern): when device-pixel width
	// is <= 1.0, bypass stroke expansion and render directly.
	effectiveWidth := width * transformScale
	if isHairline, coverage := treatAsHairline(effectiveWidth); isHairline && r.antiAlias {
		return r.strokeHairline(pixmap, pathToDraw, paint, coverage)
	}

	// Convert gg.PathVerb to stroke.PathVerb (same layout, just cast)
	strokeVerbs := convertVerbsToStroke(pathToDraw.Verbs())

	// Create stroke style from paint
	// Scale line width by transform scale (path coordinates are already transformed).
	// Clamp to 1px minimum for non-hairline path (non-AA or effectiveWidth > 1.0).
	if effectiveWidth < 1.0 {
		effectiveWidth = 1.0
	}
	strokeStyle := stroke.Stroke{
		Width:      effectiveWidth,
		Cap:        convertLineCap(paint.EffectiveLineCap()),
		Join:       convertLineJoin(paint.EffectiveLineJoin()),
		MiterLimit: paint.EffectiveMiterLimit(),
	}
	if strokeStyle.MiterLimit <= 0 {
		strokeStyle.MiterLimit = 4.0 // Default
	}

	// Create stroke expander with tight tolerance for smooth curves.
	// 0.1 px base tolerance; on HiDPI, divide by deviceScale for finer curves.
	expander := stroke.NewStrokeExpander(strokeStyle)
	strokeTol := float64(0.1)
	if r.deviceScale > 1.0 {
		strokeTol = 0.1 / float64(r.deviceScale)
	}
	expander.SetTolerance(strokeTol)

	// Expand stroke to fill path (SOA: verb+coords in, verb+coords out)
	outVerbs, outCoords := expander.Expand(strokeVerbs, pathToDraw.Coords())

	// Convert back to gg.Path (reuse scratch to avoid per-stroke allocation).
	if r.scratchStrokePath == nil {
		r.scratchStrokePath = NewPath()
	}
	strokeResultToPath(r.scratchStrokePath, outVerbs, outCoords)

	// Route stroke fills through AnalyticFiller (Skia AAA scanline).
	// Stroke-expanded multi-contour outlines (e.g., closed path → 4 contours)
	// require per-scanline winding tracking that the tile-based SparseStripsFiller
	// does not support (Vello's strip pipeline uses per-strip fill_gap flags).
	// This matches Skia Ganesh which routes strokes through scanline renderers,
	// not tile rasterizers. Single-contour strokes work with either filler after
	// the expander.go fix (#347), but multi-contour needs scanline.
	prevMode := r.rasterizerMode
	r.rasterizerMode = RasterizerAnalytic
	err := r.Fill(pixmap, r.scratchStrokePath, paint)
	r.rasterizerMode = prevMode
	return err
}

// strokeHairline renders a stroke with device-pixel width <= 1.0 using the
// hairline rasterizer (Skia/tiny-skia pattern). Instead of stroke expansion
// (which creates a thin rect that straddles pixel boundaries, losing coverage),
// this renders lines snapped to pixel centers and circles via SDF, with
// paint alpha modulated by coverage for sub-pixel widths.
//
// Reference: tiny-skia painter.rs stroke_hairline(), scan/hairline_aa.rs
func (r *SoftwareRenderer) strokeHairline(pixmap *Pixmap, p *Path, paint *Paint, coverage float64) error {
	// Try SDF path for detected shapes (circles, rrects).
	// SDF produces exact coverage for curves, avoiding the coverage loss
	// from stroke expansion on thin annular rings.
	shape := DetectShape(p)
	if shape.Kind == ShapeCircle || shape.Kind == ShapeEllipse {
		return r.strokeHairlineSDF(pixmap, p, paint, shape, coverage)
	}

	// For general line paths: render as 1px stroke with alpha modulated
	// by coverage, snapping line centers to pixel centers.
	hairlinePaint := *paint // shallow copy to preserve original
	if coverage < 1.0 {
		// Modulate paint alpha by coverage (tiny-skia apply_opacity pattern).
		hairlinePaint.stroke.solidColor.A *= coverage
		hairlinePaint.stroke.isSolid = paint.stroke.isSolid
	}

	// Snap line segments to pixel centers for maximum coverage.
	// A horizontal line at y=50.3 straddles rows 50 and 51 after 1px expansion.
	// Snapping to y=50.5 aligns the stroke to exactly one pixel row.
	snappedPath := r.snapHairlineToPixelCenter(p)

	// Stroke expand with width=1.0 (the minimum visible stroke).
	strokeVerbs := convertVerbsToStroke(snappedPath.Verbs())
	strokeStyle := stroke.Stroke{
		Width:      1.0,
		Cap:        convertLineCap(hairlinePaint.EffectiveLineCap()),
		Join:       convertLineJoin(hairlinePaint.EffectiveLineJoin()),
		MiterLimit: hairlinePaint.EffectiveMiterLimit(),
	}
	if strokeStyle.MiterLimit <= 0 {
		strokeStyle.MiterLimit = 4.0
	}
	expander := stroke.NewStrokeExpander(strokeStyle)
	strokeTol := float64(0.1)
	if r.deviceScale > 1.0 {
		strokeTol = 0.1 / float64(r.deviceScale)
	}
	expander.SetTolerance(strokeTol)
	outVerbs, outCoords := expander.Expand(strokeVerbs, snappedPath.Coords())

	if r.scratchStrokePath == nil {
		r.scratchStrokePath = NewPath()
	}
	strokeResultToPath(r.scratchStrokePath, outVerbs, outCoords)

	prevMode := r.rasterizerMode
	r.rasterizerMode = RasterizerAnalytic
	err := r.Fill(pixmap, r.scratchStrokePath, &hairlinePaint)
	r.rasterizerMode = prevMode
	return err
}

// snapHairlineToPixelCenter snaps axis-aligned line segments to pixel centers.
// For a horizontal line, the Y coordinate is snapped to floor(y) + 0.5.
// For a vertical line, the X coordinate is snapped to floor(x) + 0.5.
// Diagonal lines and curves are left unchanged.
//
// This ensures a 1px stroke expansion covers exactly one pixel row/column
// instead of straddling two, maximizing peak coverage from ~75% to ~100%.
func (r *SoftwareRenderer) snapHairlineToPixelCenter(p *Path) *Path {
	verbs := p.Verbs()
	coords := p.Coords()
	out := NewPath()

	ci := 0
	var prevX, prevY float64
	for _, v := range verbs {
		switch v {
		case MoveTo:
			prevX, prevY = coords[ci], coords[ci+1]
			out.MoveTo(prevX, prevY)
			ci += 2
		case LineTo:
			x, y := coords[ci], coords[ci+1]

			switch {
			case math.Abs(y-prevY) < 0.01:
				// Horizontal line: snap Y to pixel center.
				snappedY := math.Floor(prevY) + 0.5
				snapHairlineSegment(out, prevX, snappedY, x, snappedY)
				prevX, prevY = x, snappedY
			case math.Abs(x-prevX) < 0.01:
				// Vertical line: snap X to pixel center.
				snappedX := math.Floor(prevX) + 0.5
				snapHairlineSegment(out, snappedX, prevY, snappedX, y)
				prevX, prevY = snappedX, y
			default:
				// Diagonal: no snapping.
				out.LineTo(x, y)
				prevX, prevY = x, y
			}
			ci += 2
		case QuadTo:
			out.QuadraticTo(coords[ci], coords[ci+1], coords[ci+2], coords[ci+3])
			prevX, prevY = coords[ci+2], coords[ci+3]
			ci += 4
		case CubicTo:
			out.CubicTo(coords[ci], coords[ci+1], coords[ci+2], coords[ci+3], coords[ci+4], coords[ci+5])
			prevX, prevY = coords[ci+4], coords[ci+5]
			ci += 6
		case Close:
			out.Close()
		}
	}
	return out
}

// snapHairlineSegment adds a line segment to the output path, resetting the
// current point to the snapped start if needed. This handles the case where
// the MoveTo was at a non-snapped position and the LineTo needs snapping.
func snapHairlineSegment(out *Path, x0, y0, x1, y1 float64) {
	// Check if we need to adjust the start point.
	cp := out.CurrentPoint()
	if cp.X != x0 || cp.Y != y0 {
		// The current point doesn't match our snapped start.
		// Replace it with a new MoveTo at the snapped position.
		out.MoveTo(x0, y0)
	}
	out.LineTo(x1, y1)
}

// strokeHairlineSDF renders a thin circle/ellipse stroke using SDF coverage.
// For hairline strokes (effectiveWidth <= 1.0), the annular ring is thinner
// than the smoothstep AA zone, causing coverage loss in normal SDF rendering.
// To fix this, we render with a boosted halfStrokeWidth (0.7px = AA zone width)
// and scale the resulting coverage by the actual stroke width fraction.
// This produces crisp circles at any stroke width <= 1.0px.
func (r *SoftwareRenderer) strokeHairlineSDF(
	pixmap *Pixmap, _ *Path, paint *Paint,
	shape DetectedShape, coverage float64,
) error {
	color := getStrokeColorFromPaint(paint)
	color.A *= coverage // Modulate alpha for sub-pixel strokes.

	cx, cy := shape.CenterX, shape.CenterY
	radius := shape.RadiusX
	// Geometric half-width for the stroke. For a 1px hairline, halfW=0.5
	// produces an effective width of ~1.0px (matching Skia's hairline rasterizer).
	// Previous value sdfAntialiasWidth=0.7 produced 1.4px — 40% too wide.
	halfW := 0.5

	pad := halfW + 1
	minX := int(math.Max(0, math.Floor(cx-radius-pad)))
	maxX := int(math.Min(float64(pixmap.Width()-1), math.Ceil(cx+radius+pad)))
	minY := int(math.Max(0, math.Floor(cy-radius-pad)))
	maxY := int(math.Min(float64(pixmap.Height()-1), math.Ceil(cy+radius+pad)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			cov := SDFCircleCoverage(float64(px)+0.5, float64(py)+0.5, cx, cy, radius, halfW)
			if cov > 0 {
				alpha := color.A * cov
				if alpha < 1.0/255.0 {
					continue
				}
				r.blendCoverageSolid(pixmap, px, py, uint8(alpha*255+0.5), RGBA{
					R: color.R, G: color.G, B: color.B, A: 1.0,
				})
			}
		}
	}
	return nil
}

// convertVerbsToStroke converts gg.PathVerb slice to stroke.PathVerb slice.
// Both types have identical byte values, so this is a simple cast.
func convertVerbsToStroke(verbs []PathVerb) []stroke.PathVerb {
	result := make([]stroke.PathVerb, len(verbs))
	for i, v := range verbs {
		result[i] = stroke.PathVerb(v)
	}
	return result
}

// strokeResultToPath converts stroke output (verbs+coords) into dst Path.
// Reuses dst to avoid per-stroke allocation (Skia fOuter.reset() pattern).
func strokeResultToPath(dst *Path, verbs []stroke.PathVerb, coords []float64) {
	dst.Reset()
	ci := 0
	for _, v := range verbs {
		switch v {
		case stroke.VerbMoveTo:
			dst.MoveTo(coords[ci], coords[ci+1])
			ci += 2
		case stroke.VerbLineTo:
			dst.LineTo(coords[ci], coords[ci+1])
			ci += 2
		case stroke.VerbQuadTo:
			dst.QuadraticTo(coords[ci], coords[ci+1], coords[ci+2], coords[ci+3])
			ci += 4
		case stroke.VerbCubicTo:
			dst.CubicTo(coords[ci], coords[ci+1], coords[ci+2], coords[ci+3], coords[ci+4], coords[ci+5])
			ci += 6
		case stroke.VerbClose:
			dst.Close()
		}
	}
}

// convertLineCap converts gg.LineCap to stroke.LineCap.
func convertLineCap(c LineCap) stroke.LineCap {
	switch c {
	case LineCapButt:
		return stroke.LineCapButt
	case LineCapRound:
		return stroke.LineCapRound
	case LineCapSquare:
		return stroke.LineCapSquare
	default:
		return stroke.LineCapButt
	}
}

// convertLineJoin converts gg.LineJoin to stroke.LineJoin.
func convertLineJoin(join LineJoin) stroke.LineJoin {
	switch join {
	case LineJoinMiter:
		return stroke.LineJoinMiter
	case LineJoinRound:
		return stroke.LineJoinRound
	case LineJoinBevel:
		return stroke.LineJoinBevel
	default:
		return stroke.LineJoinMiter
	}
}

// dashPath converts a path to a dashed path using the given dash pattern.
// This walks along the path and outputs only the "dash" portions, skipping gaps.
func dashPath(p *Path, dash *Dash) *Path {
	if dash == nil || !dash.IsDashed() {
		return p
	}

	pattern := dash.effectiveArray()
	if len(pattern) == 0 {
		return p
	}

	result := NewPath()

	// State for walking along the path
	var (
		currentX, currentY float64 // current position
		startX, startY     float64 // subpath start
		patternIdx         int     // current index in pattern
		patternPos         float64 // position within current pattern element
		inDash             bool    // true if currently drawing (vs gap)
	)

	// Initialize with offset
	offset := dash.NormalizedOffset()
	patternIdx, patternPos, inDash = dashStateAtOffset(pattern, offset)

	p.Iterate(func(verb PathVerb, coords []float64) {
		switch verb {
		case MoveTo:
			currentX, currentY = coords[0], coords[1]
			startX, startY = currentX, currentY
			// Reset pattern state for new subpath
			patternIdx, patternPos, inDash = dashStateAtOffset(pattern, offset)
			if inDash {
				result.MoveTo(currentX, currentY)
			}

		case LineTo:
			dashLine(result, &currentX, &currentY, coords[0], coords[1],
				pattern, &patternIdx, &patternPos, &inDash)

		case QuadTo:
			// Flatten quadratic to lines for dashing
			ctrl := Pt(coords[0], coords[1])
			pt := Pt(coords[2], coords[3])
			dashQuad(result, &currentX, &currentY, ctrl, pt,
				pattern, &patternIdx, &patternPos, &inDash)

		case CubicTo:
			// Flatten cubic to lines for dashing
			ctrl1 := Pt(coords[0], coords[1])
			ctrl2 := Pt(coords[2], coords[3])
			pt := Pt(coords[4], coords[5])
			dashCubic(result, &currentX, &currentY, ctrl1, ctrl2, pt,
				pattern, &patternIdx, &patternPos, &inDash)

		case Close:
			// Close by dashing line back to start
			if currentX != startX || currentY != startY {
				dashLine(result, &currentX, &currentY, startX, startY,
					pattern, &patternIdx, &patternPos, &inDash)
			}
		}
	})

	return result
}

// dashStateAtOffset calculates the pattern state at a given offset.
func dashStateAtOffset(pattern []float64, offset float64) (idx int, pos float64, inDash bool) {
	patternLen := 0.0
	for _, l := range pattern {
		patternLen += l
	}
	if patternLen <= 0 {
		return 0, 0, true
	}

	// Normalize offset
	offset = math.Mod(offset, patternLen)
	if offset < 0 {
		offset += patternLen
	}

	// Walk through pattern to find position
	accumulated := 0.0
	for i, l := range pattern {
		if offset < accumulated+l {
			return i, offset - accumulated, i%2 == 0
		}
		accumulated += l
	}

	return 0, 0, true
}

// dashLine dashes a line segment from (currentX, currentY) to (x, y).
func dashLine(result *Path, currentX, currentY *float64, x, y float64,
	pattern []float64, patternIdx *int, patternPos *float64, inDash *bool) {
	dx := x - *currentX
	dy := y - *currentY
	segmentLen := math.Sqrt(dx*dx + dy*dy)

	if segmentLen < 1e-10 {
		return
	}

	// Unit direction
	ux, uy := dx/segmentLen, dy/segmentLen

	remaining := segmentLen
	startX, startY := *currentX, *currentY

	for remaining > 1e-10 {
		patternVal := pattern[*patternIdx]
		available := patternVal - *patternPos

		if available <= 0 {
			// Move to next pattern element
			*patternIdx = (*patternIdx + 1) % len(pattern)
			*patternPos = 0
			*inDash = (*patternIdx % 2) == 0
			continue
		}

		consume := math.Min(available, remaining)
		endX := startX + ux*consume
		endY := startY + uy*consume

		if *inDash {
			// We're in a dash - draw the line
			if result.isEmpty() || !pathEndAt(result, startX, startY) {
				result.MoveTo(startX, startY)
			}
			result.LineTo(endX, endY)
		}
		// If in gap, we just skip

		startX, startY = endX, endY
		remaining -= consume
		*patternPos += consume

		// Check if we've finished current pattern element
		if *patternPos >= patternVal-1e-10 {
			*patternIdx = (*patternIdx + 1) % len(pattern)
			*patternPos = 0
			*inDash = (*patternIdx % 2) == 0
		}
	}

	*currentX, *currentY = x, y
}

// dashQuad dashes a quadratic bezier curve by flattening it.
func dashQuad(result *Path, currentX, currentY *float64, control, end Point,
	pattern []float64, patternIdx *int, patternPos *float64, inDash *bool) {
	// Flatten quadratic to line segments
	tolerance := 0.5 // reasonable tolerance for dashing
	points := flattenQuadForDash(*currentX, *currentY, control.X, control.Y, end.X, end.Y, tolerance)

	for i := 2; i < len(points); i += 2 {
		dashLine(result, currentX, currentY, points[i], points[i+1],
			pattern, patternIdx, patternPos, inDash)
	}
}

// dashCubic dashes a cubic bezier curve by flattening it.
func dashCubic(result *Path, currentX, currentY *float64, c1, c2, end Point,
	pattern []float64, patternIdx *int, patternPos *float64, inDash *bool) {
	// Flatten cubic to line segments
	tolerance := 0.5 // reasonable tolerance for dashing
	points := flattenCubicForDash(*currentX, *currentY,
		c1.X, c1.Y, c2.X, c2.Y, end.X, end.Y, tolerance)

	for i := 2; i < len(points); i += 2 {
		dashLine(result, currentX, currentY, points[i], points[i+1],
			pattern, patternIdx, patternPos, inDash)
	}
}

// pathEndAt checks if the path ends at the given point.
func pathEndAt(p *Path, x, y float64) bool {
	if p.isEmpty() {
		return false
	}
	cp := p.CurrentPoint()
	return math.Abs(cp.X-x) < 1e-10 && math.Abs(cp.Y-y) < 1e-10
}

// flattenQuadForDash flattens a quadratic bezier to line points.
func flattenQuadForDash(x0, y0, cx, cy, x1, y1, tolerance float64) []float64 {
	points := []float64{x0, y0}
	flattenQuadRecForDash(x0, y0, cx, cy, x1, y1, tolerance, &points, 0)
	return points
}

func flattenQuadRecForDash(x0, y0, cx, cy, x1, y1, tolerance float64, points *[]float64, depth int) {
	// Max recursion depth to prevent stack overflow (e.g. NaN coordinates)
	if depth > 10 {
		*points = append(*points, x1, y1)
		return
	}

	// Check if curve is flat enough (distance from control to midpoint of line)
	mx := (x0 + x1) / 2
	my := (y0 + y1) / 2
	dx := cx - mx
	dy := cy - my
	dist := math.Sqrt(dx*dx + dy*dy)

	if dist < tolerance {
		*points = append(*points, x1, y1)
		return
	}

	// Subdivide using de Casteljau
	x01 := (x0 + cx) / 2
	y01 := (y0 + cy) / 2
	x12 := (cx + x1) / 2
	y12 := (cy + y1) / 2
	x012 := (x01 + x12) / 2
	y012 := (y01 + y12) / 2

	flattenQuadRecForDash(x0, y0, x01, y01, x012, y012, tolerance, points, depth+1)
	flattenQuadRecForDash(x012, y012, x12, y12, x1, y1, tolerance, points, depth+1)
}

// flattenCubicForDash flattens a cubic bezier to line points.
func flattenCubicForDash(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float64) []float64 {
	points := []float64{x0, y0}
	flattenCubicRecForDash(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance, &points, 0)
	return points
}

func flattenCubicRecForDash(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float64, points *[]float64, depth int) {
	// Max recursion depth to prevent stack overflow (e.g. NaN coordinates)
	if depth > 10 {
		*points = append(*points, x1, y1)
		return
	}

	// Check if curve is flat enough
	// Use distance of control points from the line
	d1 := pointLineDistance(c1x, c1y, x0, y0, x1, y1)
	d2 := pointLineDistance(c2x, c2y, x0, y0, x1, y1)
	dist := math.Max(d1, d2)

	if dist < tolerance {
		*points = append(*points, x1, y1)
		return
	}

	// Subdivide using de Casteljau
	x01 := (x0 + c1x) / 2
	y01 := (y0 + c1y) / 2
	x12 := (c1x + c2x) / 2
	y12 := (c1y + c2y) / 2
	x23 := (c2x + x1) / 2
	y23 := (c2y + y1) / 2
	x012 := (x01 + x12) / 2
	y012 := (y01 + y12) / 2
	x123 := (x12 + x23) / 2
	y123 := (y12 + y23) / 2
	x0123 := (x012 + x123) / 2
	y0123 := (y012 + y123) / 2

	flattenCubicRecForDash(x0, y0, x01, y01, x012, y012, x0123, y0123, tolerance, points, depth+1)
	flattenCubicRecForDash(x0123, y0123, x123, y123, x23, y23, x1, y1, tolerance, points, depth+1)
}

// pointLineDistance calculates perpendicular distance from point to line.
func pointLineDistance(px, py, x0, y0, x1, y1 float64) float64 {
	dx := x1 - x0
	dy := y1 - y0
	length := math.Sqrt(dx*dx + dy*dy)
	if length < 1e-10 {
		// Line is a point, return distance to that point
		return math.Sqrt((px-x0)*(px-x0) + (py-y0)*(py-y0))
	}
	// Cross product gives area of parallelogram, divide by base for height
	return math.Abs((py-y0)*dx-(px-x0)*dy) / length
}
