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
func (p *pixmapAdapter) BlendPixelAlpha(x, y int, c raster.RGBA, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Bounds check
	if x < 0 || x >= p.pixmap.Width() || y < 0 || y >= p.pixmap.Height() {
		return
	}

	if alpha == 255 {
		p.pixmap.SetPixel(x, y, RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
		return
	}

	// Get existing pixel
	existing := p.pixmap.GetPixel(x, y)

	// Calculate blend factor
	srcAlpha := c.A * float64(alpha) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	// Source-over compositing
	outA := srcAlpha + existing.A*invSrcAlpha
	if outA > 0 {
		outR := (c.R*srcAlpha + existing.R*existing.A*invSrcAlpha) / outA
		outG := (c.G*srcAlpha + existing.G*existing.A*invSrcAlpha) / outA
		outB := (c.B*srcAlpha + existing.B*existing.A*invSrcAlpha) / outA
		p.pixmap.SetPixel(x, y, RGBA{R: outR, G: outG, B: outB, A: outA})
	}
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
	// Get color from paint
	color := r.getColorFromPaint(paint)

	// Reset the filler for new path
	r.analyticFiller.Reset()

	// Fill using the analytic filler interface
	r.analyticFiller.Fill(p, paint.FillRule, func(y int, iter func(yield func(x int, alpha uint8) bool)) {
		// Blend alpha values to pixmap
		r.blendAlphaRunsFromIter(pixmap, y, iter, color)
	})

	return nil
}

// fillSupersampled renders the path using 4x supersampling (legacy method).
func (r *SoftwareRenderer) fillSupersampled(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format and flatten
	elements := convertPath(p)
	flattenedPath := path.Flatten(elements)
	rasterPoints := convertPoints(flattenedPath)

	// Get color from paint
	color := r.getColorFromPaint(paint)

	// Convert fill rule
	fillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = raster.FillRuleEvenOdd
	}

	// Rasterize with anti-aliasing (4x supersampling)
	adapter := &pixmapAdapter{pixmap: pixmap}
	r.rasterizer.FillAA(adapter, rasterPoints, fillRule, raster.RGBA{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})

	return nil
}

// getColorFromPaint extracts the solid color from the paint.
// Returns Black if no solid pattern is found.
func (r *SoftwareRenderer) getColorFromPaint(paint *Paint) RGBA {
	solidPattern, ok := paint.Pattern.(*SolidPattern)
	if !ok {
		return Black
	}
	return solidPattern.Color
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

		// Partial coverage - blend with existing pixel
		existing := pixmap.GetPixel(x, y)

		// Calculate effective source alpha (color alpha * coverage)
		srcAlpha := color.A * float64(alpha) / 255.0
		invSrcAlpha := 1.0 - srcAlpha

		// Source-over compositing
		outA := srcAlpha + existing.A*invSrcAlpha
		if outA > 0 {
			outR := (color.R*srcAlpha + existing.R*existing.A*invSrcAlpha) / outA
			outG := (color.G*srcAlpha + existing.G*existing.A*invSrcAlpha) / outA
			outB := (color.B*srcAlpha + existing.B*existing.A*invSrcAlpha) / outA
			pixmap.SetPixel(x, y, RGBA{R: outR, G: outG, B: outB, A: outA})
		}
		return true // continue
	})
}

// FillNoAA fills without anti-aliasing (faster but aliased).
func (r *SoftwareRenderer) FillNoAA(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format and flatten
	elements := convertPath(p)
	flattenedPath := path.Flatten(elements)
	rasterPoints := convertPoints(flattenedPath)

	// Get color from paint
	solidPattern, ok := paint.Pattern.(*SolidPattern)
	if !ok {
		return nil // Only solid patterns supported in v0.1
	}
	color := solidPattern.Color

	// Convert fill rule
	fillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = raster.FillRuleEvenOdd
	}

	// Rasterize without AA
	adapter := &pixmapAdapter{pixmap: pixmap}
	r.rasterizer.Fill(adapter, rasterPoints, fillRule, raster.RGBA{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})

	return nil
}

// Stroke implements Renderer.Stroke with anti-aliasing support.
// Strokes are expanded to fill paths and rendered with the Fill method
// to get smooth anti-aliased edges.
func (r *SoftwareRenderer) Stroke(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Apply dash pattern if set
	pathToDraw := p
	if paint.IsDashed() {
		pathToDraw = dashPath(p, paint.EffectiveDash())
	}

	// Convert gg.Path to stroke.PathElement
	strokeElements := convertPathToStrokeElements(pathToDraw)

	// Create stroke style from paint (use effective methods for unified Stroke struct)
	strokeStyle := stroke.Stroke{
		Width:      paint.EffectiveLineWidth(),
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
