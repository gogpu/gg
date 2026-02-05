package gg

import (
	"math"

	"github.com/gogpu/gg/internal/path"
	"github.com/gogpu/gg/internal/raster"
	"github.com/gogpu/gg/internal/stroke"
)

// RenderMode specifies which anti-aliasing algorithm to use.
type RenderMode int

const (
	// RenderModeSupersampled uses 4x supersampling for anti-aliasing (default).
	// This is the current stable implementation.
	RenderModeSupersampled RenderMode = iota

	// RenderModeAnalytic uses exact geometric coverage calculation.
	// This provides higher quality anti-aliasing without supersampling overhead.
	// Note: Analytic mode requires importing backend/native and calling
	// SetAnalyticFiller to configure the analytic rendering components.
	RenderModeAnalytic
)

// AnalyticFillerInterface defines the interface for analytic coverage calculation.
// This allows the analytic filler from backend/native to be injected without
// creating an import cycle.
type AnalyticFillerInterface interface {
	// Fill renders the path using analytic coverage calculation.
	// Parameters:
	//   - path: the gg.Path to render
	//   - fillRule: FillRuleNonZero or FillRuleEvenOdd
	//   - callback: called for each scanline with (y, x, alpha) values
	Fill(path *Path, fillRule FillRule, callback func(y int, iter func(yield func(x int, alpha uint8) bool)))
	// Reset clears the filler state for reuse.
	Reset()
}

// SoftwareRenderer is a CPU-based scanline rasterizer.
type SoftwareRenderer struct {
	rasterizer *raster.Rasterizer

	// Render mode selection
	mode RenderMode

	// Analytic AA components (optional, injected via SetAnalyticFiller)
	analyticFiller AnalyticFillerInterface

	// Dimensions for analytic filler
	width, height int
}

// NewSoftwareRenderer creates a new software renderer.
// The default render mode is RenderModeSupersampled (4x supersampling).
// For higher quality, call SetAnalyticFiller with an analytic filler instance.
func NewSoftwareRenderer(width, height int) *SoftwareRenderer {
	return &SoftwareRenderer{
		rasterizer: raster.NewRasterizer(width, height),
		mode:       RenderModeSupersampled,
		width:      width,
		height:     height,
	}
}

// SetRenderMode sets the anti-aliasing mode.
// RenderModeSupersampled (default) uses 4x supersampling.
// RenderModeAnalytic uses exact geometric coverage calculation (requires SetAnalyticFiller).
func (r *SoftwareRenderer) SetRenderMode(mode RenderMode) {
	r.mode = mode
}

// RenderMode returns the current anti-aliasing mode.
func (r *SoftwareRenderer) RenderMode() RenderMode {
	return r.mode
}

// SetAnalyticFiller configures the analytic filler for RenderModeAnalytic.
// This must be called before using RenderModeAnalytic.
// The filler is typically created from backend/native.NewAnalyticFillerAdapter.
func (r *SoftwareRenderer) SetAnalyticFiller(filler AnalyticFillerInterface) {
	r.analyticFiller = filler
	if filler != nil {
		r.mode = RenderModeAnalytic
	}
}

// Resize updates the renderer dimensions.
// This should be called when the context is resized.
func (r *SoftwareRenderer) Resize(width, height int) {
	r.width = width
	r.height = height
	r.rasterizer = raster.NewRasterizer(width, height)
}

// pixmapAdapter adapts gg.Pixmap to raster.Pixmap interface.
type pixmapAdapter struct {
	pixmap *Pixmap
}

func (p *pixmapAdapter) Width() int {
	return p.pixmap.Width()
}

func (p *pixmapAdapter) Height() int {
	return p.pixmap.Height()
}

func (p *pixmapAdapter) SetPixel(x, y int, c raster.RGBA) {
	p.pixmap.SetPixel(x, y, RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
}

// BlendPixelAlpha blends a color with the existing pixel using given alpha.
// This implements the raster.AAPixmap interface for anti-aliased rendering.
// Uses premultiplied source-over compositing: Result = Src + Dst * (1 - SrcA).
func (p *pixmapAdapter) BlendPixelAlpha(x, y int, c raster.RGBA, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Bounds check
	if x < 0 || x >= p.pixmap.Width() || y < 0 || y >= p.pixmap.Height() {
		return
	}

	if alpha == 255 && c.A >= 1.0 {
		p.pixmap.SetPixel(x, y, RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
		return
	}

	// Premultiplied source-over compositing
	srcAlpha := c.A * float64(alpha) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	// Premultiply source color
	srcR := c.R * srcAlpha
	srcG := c.G * srcAlpha
	srcB := c.B * srcAlpha

	// Read existing pixel (already premultiplied in buffer)
	dstR, dstG, dstB, dstA := p.pixmap.getPremul(x, y)

	// Source-over in premultiplied space: Result = Src + Dst * (1 - SrcA)
	p.pixmap.setPremul(x, y,
		srcR+dstR*invSrcAlpha,
		srcG+dstG*invSrcAlpha,
		srcB+dstB*invSrcAlpha,
		srcAlpha+dstA*invSrcAlpha,
	)
}

// convertPath converts gg.Path elements to path.PathElement for flattening.
//
//nolint:dupl // Similar to convertPathToStrokeElements but different types
func convertPath(p *Path) []path.PathElement {
	var elements []path.PathElement
	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case MoveTo:
			elements = append(elements, path.MoveTo{Point: path.Point{X: e.Point.X, Y: e.Point.Y}})
		case LineTo:
			elements = append(elements, path.LineTo{Point: path.Point{X: e.Point.X, Y: e.Point.Y}})
		case QuadTo:
			elements = append(elements, path.QuadTo{
				Control: path.Point{X: e.Control.X, Y: e.Control.Y},
				Point:   path.Point{X: e.Point.X, Y: e.Point.Y},
			})
		case CubicTo:
			elements = append(elements, path.CubicTo{
				Control1: path.Point{X: e.Control1.X, Y: e.Control1.Y},
				Control2: path.Point{X: e.Control2.X, Y: e.Control2.Y},
				Point:    path.Point{X: e.Point.X, Y: e.Point.Y},
			})
		case Close:
			elements = append(elements, path.Close{})
		}
	}
	return elements
}

// convertPoints converts path.Point to raster.Point.
func convertPoints(points []path.Point) []raster.Point {
	result := make([]raster.Point, len(points))
	for i, p := range points {
		result[i] = raster.Point{X: p.X, Y: p.Y}
	}
	return result
}

// convertEdges converts path.Edge slice to raster.PathEdge slice.
func convertEdges(edges []path.Edge) []raster.PathEdge {
	result := make([]raster.PathEdge, len(edges))
	for i, e := range edges {
		result[i] = raster.PathEdge{
			P0: raster.Point{X: e.P0.X, Y: e.P0.Y},
			P1: raster.Point{X: e.P1.X, Y: e.P1.Y},
		}
	}
	return result
}

// Fill implements Renderer.Fill with anti-aliasing enabled by default.
// The rendering method is determined by the current RenderMode.
func (r *SoftwareRenderer) Fill(pixmap *Pixmap, p *Path, paint *Paint) error {
	switch r.mode {
	case RenderModeAnalytic:
		if r.analyticFiller != nil {
			return r.fillAnalytic(pixmap, p, paint)
		}
		// Fallback to supersampled if no analytic filler configured
		return r.fillSupersampled(pixmap, p, paint)
	case RenderModeSupersampled:
		return r.fillSupersampled(pixmap, p, paint)
	default:
		return r.fillSupersampled(pixmap, p, paint)
	}
}

// fillAnalytic renders the path using analytic coverage calculation.
// This provides high quality anti-aliasing without supersampling overhead.
func (r *SoftwareRenderer) fillAnalytic(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Reset the filler for new path
	r.analyticFiller.Reset()

	if color, ok := solidColorFromPaint(paint); ok {
		// Fast path: solid color
		r.analyticFiller.Fill(p, paint.FillRule, func(y int, iter func(yield func(x int, alpha uint8) bool)) {
			r.blendAlphaRunsFromIter(pixmap, y, iter, color)
		})
	} else {
		// Pattern/gradient path: per-pixel color sampling
		r.analyticFiller.Fill(p, paint.FillRule, func(y int, iter func(yield func(x int, alpha uint8) bool)) {
			r.blendAlphaRunsFromIterPaint(pixmap, y, iter, paint)
		})
	}

	return nil
}

// fillSupersampled renders the path using 4x supersampling.
// Uses EdgeIter to correctly handle subpath boundaries (BUG-002 fix).
func (r *SoftwareRenderer) fillSupersampled(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format
	elements := convertPath(p)

	// Use EdgeIter to collect edges - this correctly handles subpath boundaries
	// by not creating edges between separate subpaths (unlike the old Flatten approach)
	pathEdges := path.CollectEdges(elements)
	rasterEdges := convertEdges(pathEdges)

	// Convert fill rule
	fillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = raster.FillRuleEvenOdd
	}

	if color, ok := solidColorFromPaint(paint); ok {
		// Fast path: single color (existing behavior)
		adapter := &pixmapAdapter{pixmap: pixmap}
		r.rasterizer.FillAAFromEdges(adapter, rasterEdges, fillRule, raster.RGBA{
			R: color.R,
			G: color.G,
			B: color.B,
			A: color.A,
		})
	} else {
		// Pattern/gradient path: per-pixel color sampling
		adapter := &painterPixmapAdapter{pixmap: pixmap, paint: paint}
		r.rasterizer.FillAAFromEdges(adapter, rasterEdges, fillRule, raster.RGBA{})
	}

	return nil
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

// painterPixmapAdapter samples paint per-pixel during rasterization.
// It implements raster.AAPixmap but NOT AAPixmapBatch, so SuperBlitter
// falls back to the scalar path which calls BlendPixelAlpha per-pixel.
// This is where we sample the pattern color at each pixel.
type painterPixmapAdapter struct {
	pixmap *Pixmap
	paint  *Paint
}

func (p *painterPixmapAdapter) Width() int {
	return p.pixmap.Width()
}

func (p *painterPixmapAdapter) Height() int {
	return p.pixmap.Height()
}

func (p *painterPixmapAdapter) SetPixel(x, y int, _ raster.RGBA) {
	// Sample color from paint at pixel center
	color := p.paint.ColorAt(float64(x)+0.5, float64(y)+0.5)
	p.pixmap.SetPixel(x, y, color)
}

// BlendPixelAlpha ignores the passed color and samples from paint instead.
// Uses premultiplied source-over compositing.
func (p *painterPixmapAdapter) BlendPixelAlpha(x, y int, _ raster.RGBA, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Bounds check
	if x < 0 || x >= p.pixmap.Width() || y < 0 || y >= p.pixmap.Height() {
		return
	}

	// Sample color from paint at pixel center
	col := p.paint.ColorAt(float64(x)+0.5, float64(y)+0.5)

	if alpha == 255 && col.A >= 1.0 {
		p.pixmap.SetPixel(x, y, col)
		return
	}

	// Premultiplied source-over compositing
	srcAlpha := col.A * float64(alpha) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	srcR := col.R * srcAlpha
	srcG := col.G * srcAlpha
	srcB := col.B * srcAlpha

	dstR, dstG, dstB, dstA := p.pixmap.getPremul(x, y)

	p.pixmap.setPremul(x, y,
		srcR+dstR*invSrcAlpha,
		srcG+dstG*invSrcAlpha,
		srcB+dstB*invSrcAlpha,
		srcAlpha+dstA*invSrcAlpha,
	)
}

// blendAlphaRunsFromIter blends alpha values to the pixmap for a given scanline.
// Uses source-over compositing for proper alpha blending.
func (r *SoftwareRenderer) blendAlphaRunsFromIter(pixmap *Pixmap, y int, iter func(yield func(x int, alpha uint8) bool), color RGBA) {
	// Skip if y is out of bounds
	if y < 0 || y >= pixmap.Height() {
		return
	}

	// Iterate over the alpha values
	iter(func(x int, alpha uint8) bool {
		if alpha == 0 {
			return true // continue
		}
		if x < 0 || x >= pixmap.Width() {
			return true // continue
		}

		// Full coverage - just set the pixel
		if alpha == 255 && color.A == 1.0 {
			pixmap.SetPixel(x, y, color)
			return true // continue
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
		return true // continue
	})
}

// blendAlphaRunsFromIterPaint is like blendAlphaRunsFromIter but samples
// the paint color at each pixel instead of using a single constant color.
func (r *SoftwareRenderer) blendAlphaRunsFromIterPaint(pixmap *Pixmap, y int, iter func(yield func(x int, alpha uint8) bool), paint *Paint) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	fy := float64(y) + 0.5

	iter(func(x int, alpha uint8) bool {
		if alpha == 0 {
			return true
		}
		if x < 0 || x >= pixmap.Width() {
			return true
		}

		// Sample color from paint at pixel center
		color := paint.ColorAt(float64(x)+0.5, fy)

		if alpha == 255 && color.A == 1.0 {
			pixmap.SetPixel(x, y, color)
			return true
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
		return true
	})
}

// FillNoAA fills without anti-aliasing (faster but aliased).
func (r *SoftwareRenderer) FillNoAA(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format and flatten
	elements := convertPath(p)
	flattenedPath := path.Flatten(elements)
	rasterPoints := convertPoints(flattenedPath)

	// Convert fill rule
	fillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = raster.FillRuleEvenOdd
	}

	if color, ok := solidColorFromPaint(paint); ok {
		// Fast path: solid color
		adapter := &pixmapAdapter{pixmap: pixmap}
		r.rasterizer.Fill(adapter, rasterPoints, fillRule, raster.RGBA{
			R: color.R,
			G: color.G,
			B: color.B,
			A: color.A,
		})
	} else {
		// Pattern/gradient path: fall back to AA fill for correct per-pixel sampling
		return r.fillSupersampled(pixmap, p, paint)
	}

	return nil
}

// Stroke implements Renderer.Stroke with anti-aliasing support.
// For thin strokes (<=1px after transform), uses optimized hairline rendering.
// For thicker strokes, expands to fill paths and renders with the Fill method.
func (r *SoftwareRenderer) Stroke(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Get effective line width
	width := paint.EffectiveLineWidth()

	// Get transform scale for dash pattern scaling
	transformScale := paint.TransformScale
	if transformScale <= 0 {
		transformScale = 1.0
	}

	// Check for hairline rendering based on TRANSFORMED line width.
	// Per tiny-skia: hairline only when transformed width <= 1.
	// At Scale(2,2) with lineWidth=1, transformed width = 2, so NOT hairline.
	effectiveWidth := width * transformScale
	if useHairline, coverage := treatAsHairline(effectiveWidth); useHairline {
		return r.strokeHairline(pixmap, p, paint, coverage)
	}

	// Apply dash pattern if set (for thick strokes)
	// Scale dash pattern by transform scale (Cairo/Skia convention)
	pathToDraw := p
	if paint.IsDashed() {
		dash := paint.EffectiveDash()
		if transformScale > 1.0 {
			dash = dash.Scale(transformScale)
		}
		pathToDraw = dashPath(p, dash)
	}

	// Convert gg.Path to stroke.PathElement
	strokeElements := convertPathToStrokeElements(pathToDraw)

	// Create stroke style from paint (use effective methods for unified Stroke struct)
	// Scale line width by transform scale (path coordinates are already transformed)
	strokeStyle := stroke.Stroke{
		Width:      paint.EffectiveLineWidth() * transformScale,
		Cap:        convertLineCap(paint.EffectiveLineCap()),
		Join:       convertLineJoin(paint.EffectiveLineJoin()),
		MiterLimit: paint.EffectiveMiterLimit(),
	}
	if strokeStyle.MiterLimit <= 0 {
		strokeStyle.MiterLimit = 4.0 // Default
	}

	// Create stroke expander with sub-pixel tolerance for smooth curves
	expander := stroke.NewStrokeExpander(strokeStyle)
	expander.SetTolerance(0.1) // Balance between smoothness and performance

	// Expand stroke to fill path
	expandedElements := expander.Expand(strokeElements)

	// Convert back to gg.Path
	strokePath := convertStrokeElementsToPath(expandedElements)

	// Fill the stroke path - this gives us anti-aliased strokes
	return r.Fill(pixmap, strokePath, paint)
}

// strokeExpanded renders a stroke by expanding to a fill path.
// This is the fallback for non-solid hairline strokes where the hairline
// blitter cannot sample per-pixel colors.
func (r *SoftwareRenderer) strokeExpanded(pixmap *Pixmap, p *Path, paint *Paint) error {
	transformScale := paint.TransformScale
	if transformScale <= 0 {
		transformScale = 1.0
	}

	strokeElements := convertPathToStrokeElements(p)
	strokeStyle := stroke.Stroke{
		Width:      math.Max(paint.EffectiveLineWidth()*transformScale, 1.0),
		Cap:        convertLineCap(paint.EffectiveLineCap()),
		Join:       convertLineJoin(paint.EffectiveLineJoin()),
		MiterLimit: paint.EffectiveMiterLimit(),
	}
	if strokeStyle.MiterLimit <= 0 {
		strokeStyle.MiterLimit = 4.0
	}

	expander := stroke.NewStrokeExpander(strokeStyle)
	expander.SetTolerance(0.1)
	expandedElements := expander.Expand(strokeElements)
	strokePath := convertStrokeElementsToPath(expandedElements)

	return r.Fill(pixmap, strokePath, paint)
}

// treatAsHairline determines if a stroke should use hairline rendering.
// Returns (useHairline, coverageFactor) where coverageFactor is the
// opacity multiplier for very thin lines.
//
// Hairline rendering produces smoother results for strokes <= 1px because
// it calculates per-pixel coverage directly, rather than expanding to a
// fill path which can produce jagged edges for thin lines.
//
// NOTE: We use the nominal line width, not considering transform.
// Path coordinates are already transformed by Context, so checking
// the user-specified width is correct. A user setting lineWidth=1
// wants a 1px line regardless of transform.
func treatAsHairline(width float64) (bool, float64) {
	// Zero width is always a hairline
	if width == 0 {
		return true, 1.0
	}

	// Threshold for hairline rendering
	// Strokes <= 1px are rendered as hairlines for better quality
	const hairlineThreshold = 1.0

	if width <= hairlineThreshold {
		// For widths between 0.5 and 1.0, use full coverage
		// For widths < 0.5, scale the coverage to maintain visibility
		coverage := width
		if coverage < 0.5 {
			coverage = 0.5 // Minimum visibility
		}
		return true, coverage
	}

	return false, 0
}

// strokeHairline renders a path as a hairline (1px or thinner stroke).
// This provides smoother results than stroke expansion for thin lines.
func (r *SoftwareRenderer) strokeHairline(pixmap *Pixmap, p *Path, paint *Paint, coverage float64) error {
	// Apply dash pattern if set
	// IMPORTANT: Scale the dash pattern by transform scale because path coordinates
	// are already transformed. Per Cairo/Skia convention, dash lengths are in
	// user-space units evaluated at stroke time.
	pathToDraw := p
	if paint.IsDashed() {
		dash := paint.EffectiveDash()
		// Scale dash pattern if transform is applied
		if paint.TransformScale > 1.0 {
			dash = dash.Scale(paint.TransformScale)
		}
		pathToDraw = dashPath(p, dash)
	}

	// For non-solid paint, fall back to stroke expansion (which goes through Fill)
	if color, ok := solidColorFromPaint(paint); ok {
		// Fast path: solid color hairline
		adapter := &pixmapAdapter{pixmap: pixmap}
		blitter := raster.NewRGBAHairlineBlitter(adapter, raster.RGBA{
			R: color.R,
			G: color.G,
			B: color.B,
			A: color.A,
		})

		lineCap := convertToHairlineCap(paint.EffectiveLineCap())
		subpaths := flattenPathToHairlineSubpaths(pathToDraw)
		for _, subpath := range subpaths {
			if len(subpath) >= 2 {
				raster.StrokeHairlineAA(blitter, subpath, lineCap, coverage)
			}
		}
		return nil
	}

	// Non-solid paint: fall back to stroke expansion → Fill (which handles patterns)
	return r.strokeExpanded(pixmap, pathToDraw, paint)
}

// convertToHairlineCap converts gg.LineCap to raster.HairlineLineCap.
func convertToHairlineCap(c LineCap) raster.HairlineLineCap {
	switch c {
	case LineCapButt:
		return raster.HairlineCapButt
	case LineCapRound:
		return raster.HairlineCapRound
	case LineCapSquare:
		return raster.HairlineCapSquare
	default:
		return raster.HairlineCapButt
	}
}

// flattenPathToHairlineSubpaths converts a path to a slice of subpaths.
// Each subpath (started by MoveTo) becomes a separate point slice.
// Curves are flattened to line segments with fine tolerance for smooth hairlines.
func flattenPathToHairlineSubpaths(p *Path) [][]raster.HairlinePoint {
	var subpaths [][]raster.HairlinePoint
	var currentSubpath []raster.HairlinePoint
	var current raster.HairlinePoint
	var start raster.HairlinePoint
	inSubpath := false

	// Fine tolerance for hairline flattening
	const tolerance = 0.25

	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case MoveTo:
			// Start new subpath - save previous one first if exists
			if len(currentSubpath) >= 2 {
				subpaths = append(subpaths, currentSubpath)
			}
			currentSubpath = nil

			current = raster.HairlinePoint{X: e.Point.X, Y: e.Point.Y}
			start = current
			currentSubpath = append(currentSubpath, current)
			inSubpath = true

		case LineTo:
			if inSubpath {
				current = raster.HairlinePoint{X: e.Point.X, Y: e.Point.Y}
				currentSubpath = append(currentSubpath, current)
			}

		case QuadTo:
			if inSubpath {
				// Flatten quadratic curve
				flatPts := flattenQuadForHairline(
					current.X, current.Y,
					e.Control.X, e.Control.Y,
					e.Point.X, e.Point.Y,
					tolerance,
				)
				currentSubpath = append(currentSubpath, flatPts...)
				current = raster.HairlinePoint{X: e.Point.X, Y: e.Point.Y}
			}

		case CubicTo:
			if inSubpath {
				// Flatten cubic curve
				flatPts := flattenCubicForHairline(
					current.X, current.Y,
					e.Control1.X, e.Control1.Y,
					e.Control2.X, e.Control2.Y,
					e.Point.X, e.Point.Y,
					tolerance,
				)
				currentSubpath = append(currentSubpath, flatPts...)
				current = raster.HairlinePoint{X: e.Point.X, Y: e.Point.Y}
			}

		case Close:
			if inSubpath && (current.X != start.X || current.Y != start.Y) {
				// Close back to start
				currentSubpath = append(currentSubpath, start)
			}
			inSubpath = false
		}
	}

	// Add final subpath
	if len(currentSubpath) >= 2 {
		subpaths = append(subpaths, currentSubpath)
	}

	return subpaths
}

// flattenQuadForHairline flattens a quadratic bezier for hairline rendering.
func flattenQuadForHairline(x0, y0, cx, cy, x1, y1, tolerance float64) []raster.HairlinePoint {
	var points []raster.HairlinePoint
	flattenQuadRecForHairline(x0, y0, cx, cy, x1, y1, tolerance, &points)
	return points
}

func flattenQuadRecForHairline(x0, y0, cx, cy, x1, y1, tolerance float64, points *[]raster.HairlinePoint) {
	// Check if curve is flat enough
	mx := (x0 + x1) / 2
	my := (y0 + y1) / 2
	dx := cx - mx
	dy := cy - my
	dist := math.Sqrt(dx*dx + dy*dy)

	if dist < tolerance {
		*points = append(*points, raster.HairlinePoint{X: x1, Y: y1})
		return
	}

	// Subdivide using de Casteljau
	x01 := (x0 + cx) / 2
	y01 := (y0 + cy) / 2
	x12 := (cx + x1) / 2
	y12 := (cy + y1) / 2
	x012 := (x01 + x12) / 2
	y012 := (y01 + y12) / 2

	flattenQuadRecForHairline(x0, y0, x01, y01, x012, y012, tolerance, points)
	flattenQuadRecForHairline(x012, y012, x12, y12, x1, y1, tolerance, points)
}

// flattenCubicForHairline flattens a cubic bezier for hairline rendering.
func flattenCubicForHairline(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float64) []raster.HairlinePoint {
	var points []raster.HairlinePoint
	flattenCubicRecForHairline(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance, &points)
	return points
}

func flattenCubicRecForHairline(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float64, points *[]raster.HairlinePoint) {
	// Check if curve is flat enough
	d1 := pointLineDistance(c1x, c1y, x0, y0, x1, y1)
	d2 := pointLineDistance(c2x, c2y, x0, y0, x1, y1)
	dist := math.Max(d1, d2)

	if dist < tolerance {
		*points = append(*points, raster.HairlinePoint{X: x1, Y: y1})
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

	flattenCubicRecForHairline(x0, y0, x01, y01, x012, y012, x0123, y0123, tolerance, points)
	flattenCubicRecForHairline(x0123, y0123, x123, y123, x23, y23, x1, y1, tolerance, points)
}

// convertPathToStrokeElements converts gg.Path elements to stroke.PathElement.
//
//nolint:dupl // Similar to convertPath but different types
func convertPathToStrokeElements(p *Path) []stroke.PathElement {
	var elements []stroke.PathElement
	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case MoveTo:
			elements = append(elements, stroke.MoveTo{Point: stroke.Point{X: e.Point.X, Y: e.Point.Y}})
		case LineTo:
			elements = append(elements, stroke.LineTo{Point: stroke.Point{X: e.Point.X, Y: e.Point.Y}})
		case QuadTo:
			elements = append(elements, stroke.QuadTo{
				Control: stroke.Point{X: e.Control.X, Y: e.Control.Y},
				Point:   stroke.Point{X: e.Point.X, Y: e.Point.Y},
			})
		case CubicTo:
			elements = append(elements, stroke.CubicTo{
				Control1: stroke.Point{X: e.Control1.X, Y: e.Control1.Y},
				Control2: stroke.Point{X: e.Control2.X, Y: e.Control2.Y},
				Point:    stroke.Point{X: e.Point.X, Y: e.Point.Y},
			})
		case Close:
			elements = append(elements, stroke.Close{})
		}
	}
	return elements
}

// convertStrokeElementsToPath converts stroke.PathElement back to gg.Path.
func convertStrokeElementsToPath(elements []stroke.PathElement) *Path {
	p := NewPath()
	for _, elem := range elements {
		switch e := elem.(type) {
		case stroke.MoveTo:
			p.MoveTo(e.Point.X, e.Point.Y)
		case stroke.LineTo:
			p.LineTo(e.Point.X, e.Point.Y)
		case stroke.QuadTo:
			p.QuadraticTo(e.Control.X, e.Control.Y, e.Point.X, e.Point.Y)
		case stroke.CubicTo:
			p.CubicTo(e.Control1.X, e.Control1.Y, e.Control2.X, e.Control2.Y, e.Point.X, e.Point.Y)
		case stroke.Close:
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

	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case MoveTo:
			currentX, currentY = e.Point.X, e.Point.Y
			startX, startY = currentX, currentY
			// Reset pattern state for new subpath
			patternIdx, patternPos, inDash = dashStateAtOffset(pattern, offset)
			if inDash {
				result.MoveTo(currentX, currentY)
			}

		case LineTo:
			dashLine(result, &currentX, &currentY, e.Point.X, e.Point.Y,
				pattern, &patternIdx, &patternPos, &inDash)

		case QuadTo:
			// Flatten quadratic to lines for dashing
			dashQuad(result, &currentX, &currentY, e.Control, e.Point,
				pattern, &patternIdx, &patternPos, &inDash)

		case CubicTo:
			// Flatten cubic to lines for dashing
			dashCubic(result, &currentX, &currentY, e.Control1, e.Control2, e.Point,
				pattern, &patternIdx, &patternPos, &inDash)

		case Close:
			// Close by dashing line back to start
			if currentX != startX || currentY != startY {
				dashLine(result, &currentX, &currentY, startX, startY,
					pattern, &patternIdx, &patternPos, &inDash)
			}
		}
	}

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

	for i := 1; i < len(points); i += 2 {
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

	for i := 1; i < len(points); i += 2 {
		dashLine(result, currentX, currentY, points[i], points[i+1],
			pattern, patternIdx, patternPos, inDash)
	}
}

// pathEndAt checks if the path ends at the given point.
func pathEndAt(p *Path, x, y float64) bool {
	elements := p.Elements()
	if len(elements) == 0 {
		return false
	}

	last := elements[len(elements)-1]
	switch e := last.(type) {
	case MoveTo:
		return math.Abs(e.Point.X-x) < 1e-10 && math.Abs(e.Point.Y-y) < 1e-10
	case LineTo:
		return math.Abs(e.Point.X-x) < 1e-10 && math.Abs(e.Point.Y-y) < 1e-10
	case QuadTo:
		return math.Abs(e.Point.X-x) < 1e-10 && math.Abs(e.Point.Y-y) < 1e-10
	case CubicTo:
		return math.Abs(e.Point.X-x) < 1e-10 && math.Abs(e.Point.Y-y) < 1e-10
	}
	return false
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
