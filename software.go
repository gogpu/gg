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
	clipFn := paint.ClipCoverage
	maskFn := paint.MaskCoverage
	if color, ok := solidColorFromPaint(paint); ok {
		r.fillCoverageSolidPath(pixmap, p, filler, fillRule, color, clipFn, maskFn)
	} else {
		r.fillCoveragePaintPath(pixmap, p, filler, fillRule, paint, clipFn, maskFn)
	}
}

// fillCoverageSolidPath fills using the CoverageFiller with a solid color,
// applying optional clip and mask coverage.
func (r *SoftwareRenderer) fillCoverageSolidPath(
	pixmap *Pixmap, p *Path, filler CoverageFiller,
	fillRule FillRule, color RGBA, clipFn func(x, y float64) byte, maskFn func(x, y int) uint8,
) {
	filler.FillCoverage(p, r.width, r.height, fillRule,
		func(x, y int, coverage uint8) {
			coverage = applyClipCoverage(clipFn, x, y, coverage)
			coverage = applyMaskCoverage(maskFn, x, y, coverage)
			if coverage == 0 {
				return
			}
			r.blendCoverageSolid(pixmap, x, y, coverage, color)
		})
}

// fillCoveragePaintPath fills using the CoverageFiller with a paint pattern,
// applying optional clip and mask coverage.
func (r *SoftwareRenderer) fillCoveragePaintPath(
	pixmap *Pixmap, p *Path, filler CoverageFiller,
	fillRule FillRule, paint *Paint, clipFn func(x, y float64) byte, maskFn func(x, y int) uint8,
) {
	filler.FillCoverage(p, r.width, r.height, fillRule,
		func(x, y int, coverage uint8) {
			coverage = applyClipCoverage(clipFn, x, y, coverage)
			coverage = applyMaskCoverage(maskFn, x, y, coverage)
			if coverage == 0 {
				return
			}
			r.blendCoveragePaint(pixmap, x, y, coverage, paint)
		})
}

// applyClipCoverage multiplies pixel coverage by the clip mask coverage.
// Returns 0 if the pixel is fully clipped. When clipFn is nil, returns the
// original coverage unchanged.
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

	// Flatten curves to line segments for the AnalyticFiller.
	// Forward differencing (QuadraticEdge/CubicEdge) can produce zero-height
	// segments after FDot6 rounding, silently losing winding contribution.
	// Pre-flattening with adaptive subdivision (0.1px tolerance) eliminates
	// this class of errors. This is the standard approach in tiny-skia and
	// Skia's analytic AA scanline rasterizer.
	r.edgeBuilder.SetFlattenCurves(true)
	defer r.edgeBuilder.SetFlattenCurves(false)

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

	if color, ok := solidColorFromPaint(paint); ok {
		// Fast path: solid color
		clipFn := paint.ClipCoverage
		maskFn := paint.MaskCoverage
		r.analyticFiller.Fill(r.edgeBuilder, coreFillRule, func(y int, runs *raster.AlphaRuns) {
			r.blendAlphaRunsFromCoreRuns(pixmap, y, runs, color, clipFn, maskFn)
		})
	} else {
		// Pattern/gradient path: per-pixel color sampling
		clipFn := paint.ClipCoverage
		maskFn := paint.MaskCoverage
		r.analyticFiller.Fill(r.edgeBuilder, coreFillRule, func(y int, runs *raster.AlphaRuns) {
			r.blendAlphaRunsFromCoreRunsPaint(pixmap, y, runs, paint, clipFn, maskFn)
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

	r.noAAEdgeBuilder.SetFlattenCurves(true)
	defer r.noAAEdgeBuilder.SetFlattenCurves(false)

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

	if color, ok := solidColorFromPaint(paint); ok {
		r.noAAFiller.Fill(r.noAAEdgeBuilder, coreFillRule, func(y, left, spanWidth int) {
			r.blitNoAASolidSpan(pixmap, y, left, spanWidth, color, clipFn, maskFn)
		})
	} else {
		r.noAAFiller.Fill(r.noAAEdgeBuilder, coreFillRule, func(y, left, spanWidth int) {
			r.blitNoAAPaintSpan(pixmap, y, left, spanWidth, paint, clipFn, maskFn)
		})
	}

	return nil
}

// blitNoAASolidSpan blits a solid-color span with optional clip and mask.
func (r *SoftwareRenderer) blitNoAASolidSpan(
	pixmap *Pixmap, y, left, spanWidth int, color RGBA,
	clipFn func(float64, float64) byte, maskFn func(int, int) uint8,
) {
	for x := left; x < left+spanWidth; x++ {
		cov := noaaPixelCoverage(x, y, clipFn, maskFn)
		if cov == 0 {
			continue
		}
		r.blendCoverageSolid(pixmap, x, y, cov, color)
	}
}

// blitNoAAPaintSpan blits a paint-sampled span with optional clip and mask.
func (r *SoftwareRenderer) blitNoAAPaintSpan(
	pixmap *Pixmap, y, left, spanWidth int, paint *Paint,
	clipFn func(float64, float64) byte, maskFn func(int, int) uint8,
) {
	for x := left; x < left+spanWidth; x++ {
		cov := noaaPixelCoverage(x, y, clipFn, maskFn)
		if cov == 0 {
			continue
		}
		c := paint.ColorAt(float64(x)+0.5, float64(y)+0.5)
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

// blendCoveragePaint blends a single pixel with paint-sampled color and coverage.
// Uses premultiplied source-over compositing.
func (r *SoftwareRenderer) blendCoveragePaint(pixmap *Pixmap, x, y int, coverage uint8, paint *Paint) {
	if x < 0 || x >= pixmap.Width() || y < 0 || y >= pixmap.Height() {
		return
	}

	color := paint.ColorAt(float64(x)+0.5, float64(y)+0.5)

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

// solidColorFromPaint returns the solid color if paint is solid.
// Returns (color, true) for solid paints, (zero, false) for patterns/gradients.
func solidColorFromPaint(paint *Paint) (RGBA, bool) {
	// Check Brush first (takes precedence)
	if paint.Brush != nil {
		if sb, ok := paint.Brush.(SolidBrush); ok {
			return sb.Color, true
		}
		return RGBA{}, false
	}
	// Fall back to Pattern
	if sp, ok := paint.Pattern.(*SolidPattern); ok {
		return sp.Color, true
	}
	return RGBA{}, false
}

// blendAlphaRunsFromCoreRuns blends alpha values from raster.AlphaRuns to the pixmap.
// Uses source-over compositing for proper alpha blending.
// When clipFn is non-nil, each pixel's alpha is multiplied by the clip coverage.
// When maskFn is non-nil, each pixel's alpha is multiplied by the mask coverage.
func (r *SoftwareRenderer) blendAlphaRunsFromCoreRuns(pixmap *Pixmap, y int, runs *raster.AlphaRuns, color RGBA, clipFn func(x, y float64) byte, maskFn func(x, y int) uint8) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	fy := float64(y) + 0.5

	for x, alpha := range runs.Iter() {
		if alpha == 0 {
			continue
		}
		if x < 0 || x >= pixmap.Width() {
			continue
		}

		// Apply clip coverage if active.
		if clipFn != nil {
			cc := clipFn(float64(x)+0.5, fy)
			if cc == 0 {
				continue
			}
			alpha = uint8(uint16(alpha) * uint16(cc) / 255)
			if alpha == 0 {
				continue
			}
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
// When clipFn is non-nil, each pixel's alpha is multiplied by the clip coverage.
// When maskFn is non-nil, each pixel's alpha is multiplied by the mask coverage.
func (r *SoftwareRenderer) blendAlphaRunsFromCoreRunsPaint(pixmap *Pixmap, y int, runs *raster.AlphaRuns, paint *Paint, clipFn func(x, y float64) byte, maskFn func(x, y int) uint8) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	fy := float64(y) + 0.5

	for x, alpha := range runs.Iter() {
		if alpha == 0 {
			continue
		}
		if x < 0 || x >= pixmap.Width() {
			continue
		}

		fx := float64(x) + 0.5

		// Apply clip coverage if active.
		if clipFn != nil {
			cc := clipFn(fx, fy)
			if cc == 0 {
				continue
			}
			alpha = uint8(uint16(alpha) * uint16(cc) / 255)
			if alpha == 0 {
				continue
			}
		}

		// Apply mask coverage if active.
		alpha = applyMaskCoverage(maskFn, x, y, alpha)
		if alpha == 0 {
			continue
		}

		// Sample color from paint at pixel center
		color := paint.ColorAt(fx, fy)

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

// Stroke implements Renderer.Stroke with anti-aliasing support.
// Strokes are expanded to fill paths and rendered with the Fill method,
// which provides analytic anti-aliased results.
func (r *SoftwareRenderer) Stroke(pixmap *Pixmap, p *Path, paint *Paint) error {
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

	// Convert gg.PathVerb to stroke.PathVerb (same layout, just cast)
	strokeVerbs := convertVerbsToStroke(pathToDraw.Verbs())

	// Create stroke style from paint
	// Scale line width by transform scale (path coordinates are already transformed)
	effectiveWidth := width * transformScale
	if effectiveWidth < 1.0 {
		effectiveWidth = 1.0 // Minimum 1px stroke for visibility
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

	// Convert back to gg.Path
	strokePath := strokeResultToPath(outVerbs, outCoords)

	// Fill the stroke path - this gives us anti-aliased strokes
	return r.Fill(pixmap, strokePath, paint)
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

// strokeResultToPath converts stroke output (verbs+coords) back to gg.Path.
func strokeResultToPath(verbs []stroke.PathVerb, coords []float64) *Path {
	p := NewPath()
	ci := 0
	for _, v := range verbs {
		switch v {
		case stroke.VerbMoveTo:
			p.MoveTo(coords[ci], coords[ci+1])
			ci += 2
		case stroke.VerbLineTo:
			p.LineTo(coords[ci], coords[ci+1])
			ci += 2
		case stroke.VerbQuadTo:
			p.QuadraticTo(coords[ci], coords[ci+1], coords[ci+2], coords[ci+3])
			ci += 4
		case stroke.VerbCubicTo:
			p.CubicTo(coords[ci], coords[ci+1], coords[ci+2], coords[ci+3], coords[ci+4], coords[ci+5])
			ci += 6
		case stroke.VerbClose:
			p.Close()
		}
	}
	return p
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
	flattenQuadRecForDash(x0, y0, cx, cy, x1, y1, tolerance, &points)
	return points
}

func flattenQuadRecForDash(x0, y0, cx, cy, x1, y1, tolerance float64, points *[]float64) {
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

	flattenQuadRecForDash(x0, y0, x01, y01, x012, y012, tolerance, points)
	flattenQuadRecForDash(x012, y012, x12, y12, x1, y1, tolerance, points)
}

// flattenCubicForDash flattens a cubic bezier to line points.
func flattenCubicForDash(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float64) []float64 {
	points := []float64{x0, y0}
	flattenCubicRecForDash(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance, &points)
	return points
}

func flattenCubicRecForDash(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float64, points *[]float64) {
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

	flattenCubicRecForDash(x0, y0, x01, y01, x012, y012, x0123, y0123, tolerance, points)
	flattenCubicRecForDash(x0123, y0123, x123, y123, x23, y23, x1, y1, tolerance, points)
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
