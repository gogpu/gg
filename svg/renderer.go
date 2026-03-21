package svg

import (
	"image/color"

	"github.com/gogpu/gg"
)

// renderState holds the rendering state during SVG traversal.
type renderState struct {
	overrideColor color.Color // non-nil → replace all non-colorNone colors
	parentFill    string      // inherited fill from parent <g>
	parentStroke  string      // inherited stroke from parent <g>
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

	fillAndStroke(dc, &e.Attrs, state, func() {
		dc.DrawPath(path)
	})

	dc.Pop()
}

// renderCircle renders an SVG <circle> element.
func renderCircle(dc *gg.Context, e *CircleElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	fillAndStroke(dc, &e.Attrs, state, func() {
		dc.DrawCircle(e.CX, e.CY, e.R)
	})

	dc.Pop()
}

// renderRect renders an SVG <rect> element.
func renderRect(dc *gg.Context, e *RectElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	fillAndStroke(dc, &e.Attrs, state, func() {
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
	})

	dc.Pop()
}

// renderEllipse renders an SVG <ellipse> element.
func renderEllipse(dc *gg.Context, e *EllipseElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	fillAndStroke(dc, &e.Attrs, state, func() {
		dc.DrawEllipse(e.CX, e.CY, e.RX, e.RY)
	})

	dc.Pop()
}

// renderLine renders an SVG <line> element.
func renderLine(dc *gg.Context, e *LineElement, state *renderState) {
	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	// Lines are stroke-only by default.
	applyStrokeAttrs(dc, &e.Attrs, state)
	dc.DrawLine(e.X1, e.Y1, e.X2, e.Y2)
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

	fillAndStroke(dc, &e.Attrs, state, func() {
		dc.ClearPath()
		drawPointsPath(dc, e.Points, true)
	})

	dc.Pop()
}

// renderPolyline renders an SVG <polyline> element.
func renderPolyline(dc *gg.Context, e *PolylineElement, state *renderState) {
	if len(e.Points) < 4 {
		return
	}

	dc.Push()
	applyElementTransform(dc, &e.Attrs)

	fillAndStroke(dc, &e.Attrs, state, func() {
		dc.ClearPath()
		drawPointsPath(dc, e.Points, false)
	})

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
// The drawShape function should set up the path on the context.
func fillAndStroke(dc *gg.Context, a *Attrs, state *renderState, drawShape func()) {
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
		drawShape()
		if hasStroke {
			_ = dc.FillPreserve()
		} else {
			_ = dc.Fill()
		}
	}

	if hasStroke {
		applyStrokeAttrs(dc, a, state)
		if !hasFill {
			drawShape()
		}
		_ = dc.Stroke()
	}
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
