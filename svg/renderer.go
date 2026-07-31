package svg

import (
	"image/color"
	"math"
	"os"

	"github.com/gogpu/gg"
)

// strokeHintMaxCanvasSize is the maximum canvas dimension (in pixels) at which
// stroke hinting is applied. Above this size, strokes are thick enough that
// sub-pixel positioning produces acceptable results without hinting.
const strokeHintMaxCanvasSize = 48

// strokeHintMaxWidth is the maximum stroke width (in device pixels, after
// viewBox scaling) that qualifies for hinting. Thin strokes suffer most from
// sub-pixel positioning; thicker strokes already span multiple pixels.
const strokeHintMaxWidth = 1.5

// renderState holds the rendering state during SVG traversal.
type renderState struct {
	overrideColor color.Color // non-nil → replace all non-colorNone colors
	parentFill    string      // inherited fill from parent <g>
	parentStroke  string      // inherited stroke from parent <g>
	strokeHinting bool        // true → snap thin stroke coords to pixel centers
	scaleX        float64     // viewBox → device X scale (for device-px stroke width)
	scaleY        float64     // viewBox → device Y scale
}

// renderElements renders a list of elements into the given gg.Context.
func renderElements(dc *gg.Context, elements []Element, state *renderState) {
	for _, elem := range elements {
		renderElement(dc, elem, state)
	}
}

// renderElement dispatches rendering to the appropriate element-specific function.
func renderElement(dc *gg.Context, elem Element, state *renderState) {
	switch e := elem.(type) {
	case *PathElement:
		renderPath(dc, e, state)
	case *CircleElement:
		renderCircle(dc, e, state)
	case *RectElement:
		renderRect(dc, e, state)
	case *EllipseElement:
		renderEllipse(dc, e, state)
	case *LineElement:
		renderLine(dc, e, state)
	case *PolygonElement:
		renderPolygon(dc, e, state)
	case *PolylineElement:
		renderPolyline(dc, e, state)
	case *GroupElement:
		renderGroup(dc, e, state)
	}
}

// renderPath renders an SVG <path> element.
func renderPath(dc *gg.Context, e *PathElement, state *renderState) {
	if e.D == "" {
		return
	}
	path, err := gg.ParseSVGPath(e.D)
	if err != nil {
		return // skip invalid paths silently
	}

	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	// Apply stroke hinting: snap path line endpoints to pixel centers.
	// The original path is used for fill (hinting would displace filled shapes).
	// A separate hinted path is used for stroke when conditions are met.
	strokePath := path
	if shouldStroke(&e.Attrs, state) && shouldHintStroke(&e.Attrs, state) {
		strokePath = hintSVGPath(path, state.scaleX, state.scaleY)
	}

	fillAndStroke(dc, &e.Attrs, state, func() {
		dc.DrawPath(path)
	}, func() {
		dc.DrawPath(strokePath)
	})

	dc.Pop()
}

// renderCircle renders an SVG <circle> element.
func renderCircle(dc *gg.Context, e *CircleElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	draw := func() { dc.DrawCircle(e.CX, e.CY, e.R) }
	fillAndStroke(dc, &e.Attrs, state, draw, draw)

	dc.Pop()
}

// renderRect renders an SVG <rect> element.
func renderRect(dc *gg.Context, e *RectElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	draw := func() {
		if e.RX > 0 || e.RY > 0 {
			// Use the larger of rx/ry for the rounded rectangle radius.
			r := e.RX
			if e.RY > r {
				r = e.RY
			}
			dc.DrawRoundedRectangle(e.X, e.Y, e.W, e.H, r)
		} else {
			dc.DrawRectangle(e.X, e.Y, e.W, e.H)
		}
	}
	fillAndStroke(dc, &e.Attrs, state, draw, draw)

	dc.Pop()
}

// renderEllipse renders an SVG <ellipse> element.
func renderEllipse(dc *gg.Context, e *EllipseElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	draw := func() { dc.DrawEllipse(e.CX, e.CY, e.RX, e.RY) }
	fillAndStroke(dc, &e.Attrs, state, draw, draw)

	dc.Pop()
}

// renderLine renders an SVG <line> element.
func renderLine(dc *gg.Context, e *LineElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	// Lines are stroke-only by default.
	applyStrokeAttrs(dc, &e.Attrs, state)
	x1, y1, x2, y2 := e.X1, e.Y1, e.X2, e.Y2
	if shouldHintStroke(&e.Attrs, state) {
		x1, y1 = hintLineCoords(x1, y1, state.scaleX, state.scaleY)
		x2, y2 = hintLineCoords(x2, y2, state.scaleX, state.scaleY)
	}
	dc.DrawLine(x1, y1, x2, y2)
	_ = dc.Stroke()

	dc.Pop()
}

// renderPolygon renders an SVG <polygon> element.
func renderPolygon(dc *gg.Context, e *PolygonElement, state *renderState) {
	if len(e.Points) < 4 {
		return // need at least 2 points
	}

	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	drawFill := func() {
		dc.ClearPath()
		drawPointsPath(dc, e.Points, true)
	}
	drawStroke := drawFill
	if shouldStroke(&e.Attrs, state) && shouldHintStroke(&e.Attrs, state) {
		hintedPts := hintPoints(e.Points, state.scaleX, state.scaleY)
		drawStroke = func() {
			dc.ClearPath()
			drawPointsPath(dc, hintedPts, true)
		}
	}
	fillAndStroke(dc, &e.Attrs, state, drawFill, drawStroke)

	dc.Pop()
}

// renderPolyline renders an SVG <polyline> element.
func renderPolyline(dc *gg.Context, e *PolylineElement, state *renderState) {
	if len(e.Points) < 4 {
		return
	}

	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	drawFill := func() {
		dc.ClearPath()
		drawPointsPath(dc, e.Points, false)
	}
	drawStroke := drawFill
	if shouldStroke(&e.Attrs, state) && shouldHintStroke(&e.Attrs, state) {
		hintedPts := hintPoints(e.Points, state.scaleX, state.scaleY)
		drawStroke = func() {
			dc.ClearPath()
			drawPointsPath(dc, hintedPts, false)
		}
	}
	fillAndStroke(dc, &e.Attrs, state, drawFill, drawStroke)

	dc.Pop()
}

// renderGroup renders an SVG <g> element and its children.
func renderGroup(dc *gg.Context, e *GroupElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	// Create child state with inherited attrs.
	childState := &renderState{
		overrideColor: state.overrideColor,
		parentFill:    state.parentFill,
		parentStroke:  state.parentStroke,
		strokeHinting: state.strokeHinting,
		scaleX:        state.scaleX,
		scaleY:        state.scaleY,
	}
	if e.Attrs.Fill != "" {
		childState.parentFill = e.Attrs.Fill
	}
	if e.Attrs.Stroke != "" {
		childState.parentStroke = e.Attrs.Stroke
	}

	renderElements(dc, e.Children, childState)

	dc.Pop()
}

// fillAndStroke applies fill and/or stroke to the current path.
//
// drawFill sets up the path for filling (original coordinates).
// drawStroke sets up the path for stroking (may have hinted coordinates
// when stroke hinting is active for crisp thin lines in small icons).
func fillAndStroke(dc *gg.Context, a *Attrs, state *renderState, drawFill, drawStroke func()) {
	hasFill := shouldFill(a, state)
	hasStroke := shouldStroke(a, state)

	if !hasFill && !hasStroke {
		// Default SVG behavior: fill with black if no fill/stroke specified.
		if a.Fill == "" && a.Stroke == "" {
			hasFill = true
		}
	}

	if hasFill {
		applyFillAttrs(dc, a, state)
		drawFill()
		_ = dc.Fill()
	}

	if hasStroke {
		applyStrokeAttrs(dc, a, state)
		drawStroke()
		_ = dc.Stroke()
	}
}

// shouldHintStroke reports whether stroke hinting should be applied to this
// element's stroke. Checks stroke width in device pixels against the threshold.
func shouldHintStroke(a *Attrs, state *renderState) bool {
	if !state.strokeHinting {
		return false
	}
	// Compute device-pixel stroke width. Use the average scale when sx != sy
	// (non-uniform scaling). For typical icon rendering sx == sy.
	avgScale := (state.scaleX + state.scaleY) / 2.0
	deviceWidth := a.StrokeWidth * avgScale
	return deviceWidth <= strokeHintMaxWidth
}

// hintSVGPath creates a copy of an SVG path with line endpoints snapped to
// pixel centers in device space. The path coordinates are in viewBox space;
// we compute device positions via the scale factors, snap to pixel centers,
// and convert back.
//
// Curve verbs (QuadTo, CubicTo) are passed through unchanged — curves don't
// align to the pixel grid, and snapping control points would distort the shape.
//
// This is modeled after Java2D's VALUE_STROKE_NORMALIZE which snaps stroke
// edges to pixel boundaries for crisp 1px lines in small icons.
func hintSVGPath(src *gg.Path, sx, sy float64) *gg.Path {
	if src == nil || src.NumVerbs() == 0 {
		return src
	}

	// Don't hint paths that contain curves — snapping line endpoints while
	// leaving curve endpoints unsnapped creates misaligned joints and
	// garbage pixels at corners. Only hint pure line/polyline paths.
	hasCurves := false
	src.Iterate(func(verb gg.PathVerb, _ []float64) {
		if verb == gg.QuadTo || verb == gg.CubicTo {
			hasCurves = true
		}
	})
	if hasCurves {
		return src
	}

	result := gg.NewPath()
	src.Iterate(func(verb gg.PathVerb, coords []float64) {
		switch verb {
		case gg.MoveTo:
			result.MoveTo(
				snapViewBoxCoord(coords[0], sx),
				snapViewBoxCoord(coords[1], sy),
			)
		case gg.LineTo:
			result.LineTo(
				snapViewBoxCoord(coords[0], sx),
				snapViewBoxCoord(coords[1], sy),
			)
		case gg.QuadTo:
			// Don't snap curve control points — would distort curve shape.
			result.QuadraticTo(coords[0], coords[1], coords[2], coords[3])
		case gg.CubicTo:
			result.CubicTo(coords[0], coords[1], coords[2], coords[3], coords[4], coords[5])
		case gg.Close:
			result.Close()
		}
	})
	return result
}

// hintLineCoords snaps line endpoint coordinates (in viewBox space) to pixel
// centers in device space, returning the snapped viewBox coordinates.
func hintLineCoords(x, y, sx, sy float64) (float64, float64) {
	return snapViewBoxCoord(x, sx), snapViewBoxCoord(y, sy)
}

// snapViewBoxCoord converts a viewBox coordinate to device space, snaps it
// to the nearest pixel center, and converts back to viewBox space.
//
// For a viewBox coordinate v with scale s:
//
//	device = v * s
//	snapped_device = floor(device) + 0.5
//	snapped_viewbox = snapped_device / s
//
// This ensures a 1px stroke centered at the snapped position covers exactly
// one full pixel after stroke expansion by ±0.5.
func snapViewBoxCoord(v, scale float64) float64 {
	if scale == 0 {
		return v
	}
	device := v * scale
	snapped := math.Floor(device) + 0.5
	return snapped / scale
}

// hintPoints snaps alternating x,y coordinate pairs to pixel centers.
// Returns a new slice; the original is not modified.
func hintPoints(pts []float64, sx, sy float64) []float64 {
	out := make([]float64, len(pts))
	for i := 0; i+1 < len(pts); i += 2 {
		out[i] = snapViewBoxCoord(pts[i], sx)
		out[i+1] = snapViewBoxCoord(pts[i+1], sy)
	}
	return out
}

// strokeHintingDisabled reports whether the GOGPU_SVG_NO_HINT environment
// variable is set to disable stroke hinting.
func strokeHintingDisabled() bool {
	return os.Getenv("GOGPU_SVG_NO_HINT") != ""
}

// shouldFill returns true if the element should be filled.
func shouldFill(a *Attrs, state *renderState) bool {
	fill := resolveFill(a, state)
	return fill != colorNone
}

// shouldStroke returns true if the element should be stroked.
func shouldStroke(a *Attrs, state *renderState) bool {
	stroke := resolveStroke(a, state)
	return stroke != "" && stroke != colorNone
}

// resolveFill returns the effective fill color string, considering inheritance.
func resolveFill(a *Attrs, state *renderState) string {
	if a.Fill != "" {
		return a.Fill
	}
	if state.parentFill != "" {
		return state.parentFill
	}
	return "" // will be treated as "black" by SVG spec default
}

// resolveStroke returns the effective stroke color string, considering inheritance.
func resolveStroke(a *Attrs, state *renderState) string {
	if a.Stroke != "" {
		return a.Stroke
	}
	return state.parentStroke
}

// applyFillAttrs sets the fill color and fill rule on the context.
func applyFillAttrs(dc *gg.Context, a *Attrs, state *renderState) {
	fillStr := resolveFill(a, state)

	// Set fill rule.
	fillRule := a.FillRule
	if fillRule == "" {
		fillRule = a.ClipRule
	}
	switch fillRule {
	case "evenodd":
		dc.SetFillRule(gg.FillRuleEvenOdd)
	default:
		dc.SetFillRule(gg.FillRuleNonZero)
	}

	// Set fill color.
	if state.overrideColor != nil && fillStr != colorNone {
		setColorWithOpacity(dc, state.overrideColor, a.FillOpacity*a.Opacity)
		return
	}

	c, err := parseColor(fillStr)
	if err != nil || c == nil {
		// Default fill is black per SVG spec.
		setColorWithOpacity(dc, color.Black, a.FillOpacity*a.Opacity)
		return
	}
	setColorWithOpacity(dc, c, a.FillOpacity*a.Opacity)
}

// applyStrokeAttrs sets stroke color, width, cap, and join on the context.
func applyStrokeAttrs(dc *gg.Context, a *Attrs, state *renderState) {
	strokeStr := resolveStroke(a, state)

	dc.SetLineWidth(a.StrokeWidth)

	// Stroke cap
	switch a.StrokeCap {
	case "round":
		dc.SetLineCap(gg.LineCapRound)
	case "square":
		dc.SetLineCap(gg.LineCapSquare)
	default:
		dc.SetLineCap(gg.LineCapButt)
	}

	// Stroke join
	switch a.StrokeJoin {
	case "round":
		dc.SetLineJoin(gg.LineJoinRound)
	case "bevel":
		dc.SetLineJoin(gg.LineJoinBevel)
	default:
		dc.SetLineJoin(gg.LineJoinMiter)
	}

	// Stroke color
	if state.overrideColor != nil && strokeStr != colorNone {
		setColorWithOpacity(dc, state.overrideColor, a.StrokeOpacity*a.Opacity)
		return
	}

	c, err := parseColor(strokeStr)
	if err != nil || c == nil {
		return
	}
	setColorWithOpacity(dc, c, a.StrokeOpacity*a.Opacity)
}

// setColorWithOpacity sets the drawing color on the context, applying
// an additional opacity multiplier.
func setColorWithOpacity(dc *gg.Context, c color.Color, opacity float64) {
	if opacity >= 1.0 {
		dc.SetColor(c)
		return
	}
	r, g, b, a := c.RGBA()
	// Un-premultiply, apply opacity, set as straight alpha RGBA.
	if a == 0 {
		dc.SetRGBA(0, 0, 0, 0)
		return
	}
	fa := float64(a) / 65535.0
	dc.SetRGBA(
		float64(r)/65535.0/fa,
		float64(g)/65535.0/fa,
		float64(b)/65535.0/fa,
		fa*opacity,
	)
}

// applyElementTransform applies the element's transform attribute to the context.
func applyElementTransform(dc *gg.Context, a *Attrs) {
	if a.Transform != "" {
		// Errors in transforms are silently ignored (best effort).
		_ = applyTransform(dc, a.Transform)
	}
}

// drawPointsPath draws a path from alternating x,y point values.
// If closed is true, the path is closed (polygon). Otherwise it's open (polyline).
func drawPointsPath(dc *gg.Context, points []float64, closed bool) {
	for i := 0; i+1 < len(points); i += 2 {
		if i == 0 {
			dc.MoveTo(points[i], points[i+1])
		} else {
			dc.LineTo(points[i], points[i+1])
		}
	}
	if closed {
		dc.ClosePath()
	}
}
