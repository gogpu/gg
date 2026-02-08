package gg

import (
	"math"

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

	// Dimensions
	width, height int
}

// NewSoftwareRenderer creates a new software renderer with analytic anti-aliasing.
func NewSoftwareRenderer(width, height int) *SoftwareRenderer {
	eb := raster.NewEdgeBuilder(4) // 16x AA quality
	return &SoftwareRenderer{
		edgeBuilder:    eb,
		analyticFiller: raster.NewAnalyticFiller(width, height),
		width:          width,
		height:         height,
	}
}

// Resize updates the renderer dimensions.
// This should be called when the context is resized.
func (r *SoftwareRenderer) Resize(width, height int) {
	r.width = width
	r.height = height
	eb := raster.NewEdgeBuilder(4)
	r.edgeBuilder = eb
	r.analyticFiller = raster.NewAnalyticFiller(width, height)
}

// convertGGPathToCorePath converts a gg.Path to raster.PathLike.
func convertGGPathToCorePath(p *Path) raster.PathLike {
	var verbs []raster.PathVerb
	var points []float32

	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case MoveTo:
			verbs = append(verbs, raster.VerbMoveTo)
			points = append(points, float32(e.Point.X), float32(e.Point.Y))
		case LineTo:
			verbs = append(verbs, raster.VerbLineTo)
			points = append(points, float32(e.Point.X), float32(e.Point.Y))
		case QuadTo:
			verbs = append(verbs, raster.VerbQuadTo)
			points = append(points,
				float32(e.Control.X), float32(e.Control.Y),
				float32(e.Point.X), float32(e.Point.Y),
			)
		case CubicTo:
			verbs = append(verbs, raster.VerbCubicTo)
			points = append(points,
				float32(e.Control1.X), float32(e.Control1.Y),
				float32(e.Control2.X), float32(e.Control2.Y),
				float32(e.Point.X), float32(e.Point.Y),
			)
		case Close:
			verbs = append(verbs, raster.VerbClose)
		}
	}

	return raster.NewScenePathAdapter(len(verbs) == 0, verbs, points)
}

// Fill implements Renderer.Fill using analytic anti-aliasing.
func (r *SoftwareRenderer) Fill(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Reset the edge builder and filler
	r.edgeBuilder.Reset()
	r.analyticFiller.Reset()

	// Build edges from the path
	pathAdapter := convertGGPathToCorePath(p)
	r.edgeBuilder.BuildFromPath(pathAdapter, raster.IdentityTransform{})

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
		r.analyticFiller.Fill(r.edgeBuilder, coreFillRule, func(y int, runs *raster.AlphaRuns) {
			r.blendAlphaRunsFromCoreRuns(pixmap, y, runs, color)
		})
	} else {
		// Pattern/gradient path: per-pixel color sampling
		r.analyticFiller.Fill(r.edgeBuilder, coreFillRule, func(y int, runs *raster.AlphaRuns) {
			r.blendAlphaRunsFromCoreRunsPaint(pixmap, y, runs, paint)
		})
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

// blendAlphaRunsFromCoreRuns blends alpha values from raster.AlphaRuns to the pixmap.
// Uses source-over compositing for proper alpha blending.
func (r *SoftwareRenderer) blendAlphaRunsFromCoreRuns(pixmap *Pixmap, y int, runs *raster.AlphaRuns, color RGBA) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	for x, alpha := range runs.Iter() {
		if alpha == 0 {
			continue
		}
		if x < 0 || x >= pixmap.Width() {
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
func (r *SoftwareRenderer) blendAlphaRunsFromCoreRunsPaint(pixmap *Pixmap, y int, runs *raster.AlphaRuns, paint *Paint) {
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

		// Sample color from paint at pixel center
		color := paint.ColorAt(float64(x)+0.5, fy)

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

	// Convert gg.Path to stroke.PathElement
	strokeElements := convertPathToStrokeElements(pathToDraw)

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
	// 0.025 px produces ~128 segments per circle — eliminates visible
	// polygon faceting on small UI circles (radio buttons, checkboxes).
	expander := stroke.NewStrokeExpander(strokeStyle)
	expander.SetTolerance(0.1)

	// Expand stroke to fill path
	expandedElements := expander.Expand(strokeElements)

	// Convert back to gg.Path
	strokePath := convertStrokeElementsToPath(expandedElements)

	// Fill the stroke path - this gives us anti-aliased strokes
	return r.Fill(pixmap, strokePath, paint)
}

// convertPathToStrokeElements converts gg.Path elements to stroke.PathElement.
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
