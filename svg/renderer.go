package svg

import (
	"image/color"
	"math"
	"os"

	"github.com/gogpu/gg"
)

const (
	// strokeHintMaxCanvasSize limits automatic hinting to icon-sized targets.
	strokeHintMaxCanvasSize = 48
	// strokeHintMaxWidth limits hinting to thin strokes in device pixels.
	strokeHintMaxWidth = 1.5
)

// renderState is shared by immediate and retained traversal. Matrix is the
// complete logical transform at the current nesting level.
type renderState struct {
	overrideColor color.Color
	parentFill    string
	parentStroke  string
	opacity       float64
	matrix        gg.Matrix
	strokeHinting bool
	scaleX        float64
	scaleY        float64
}

type resolvedFill struct {
	present bool
	rule    gg.FillRule
	color   gg.RGBA
}

type resolvedStroke struct {
	present bool
	width   float64
	cap     gg.LineCap
	join    gg.LineJoin
	color   gg.RGBA
}

type resolvedStyle struct {
	fill   resolvedFill
	stroke resolvedStroke
}

func renderElements(dc *gg.Context, elements []Element, state *renderState) {
	for _, elem := range elements {
		renderElement(dc, elem, state)
	}
}

func renderElement(dc *gg.Context, elem Element, state *renderState) {
	switch e := elem.(type) {
	case *PathElement:
		if e.D == "" {
			return
		}
		path, err := gg.ParseSVGPath(e.D)
		if err != nil {
			return
		}
		renderGeometry(dc, path, &e.Attrs, state, true)
	case *CircleElement:
		path := gg.NewPath()
		path.Circle(e.CX, e.CY, e.R)
		renderGeometry(dc, path, &e.Attrs, state, true)
	case *RectElement:
		path := gg.NewPath()
		if e.RX > 0 || e.RY > 0 {
			r := e.RX
			if e.RY > r {
				r = e.RY
			}
			path.RoundedRectangle(e.X, e.Y, e.W, e.H, r)
		} else {
			path.Rectangle(e.X, e.Y, e.W, e.H)
		}
		renderGeometry(dc, path, &e.Attrs, state, true)
	case *EllipseElement:
		path := gg.NewPath()
		path.Ellipse(e.CX, e.CY, e.RX, e.RY)
		renderGeometry(dc, path, &e.Attrs, state, true)
	case *LineElement:
		path := gg.NewPath()
		path.MoveTo(e.X1, e.Y1)
		path.LineTo(e.X2, e.Y2)
		renderGeometry(dc, path, &e.Attrs, state, false)
	case *PolygonElement:
		if path := pointsPath(e.Points, true); path != nil {
			renderGeometry(dc, path, &e.Attrs, state, true)
		}
	case *PolylineElement:
		if path := pointsPath(e.Points, false); path != nil {
			renderGeometry(dc, path, &e.Attrs, state, true)
		}
	case *GroupElement:
		renderGroup(dc, e, state)
	}
}

func renderGeometry(dc *gg.Context, path *gg.Path, attrs *Attrs, state *renderState, allowFill bool) {
	local := elementTransformMatrix(attrs)
	style := resolveStyle(attrs, state)
	if !allowFill {
		style.fill.present = false
	}
	if !style.fill.present && !style.stroke.present {
		return
	}

	dc.Push()
	dc.Transform(local)
	if style.fill.present {
		dc.SetFillRule(style.fill.rule)
		setResolvedColor(dc, style.fill.color)
		dc.DrawPath(path)
		_ = dc.Fill()
	}
	if style.stroke.present {
		strokePath := path
		if shouldHintStroke(attrs, state) {
			strokePath = hintSVGPath(path, state.scaleX, state.scaleY)
		}
		dc.SetLineWidth(style.stroke.width)
		dc.SetLineCap(style.stroke.cap)
		dc.SetLineJoin(style.stroke.join)
		setResolvedColor(dc, style.stroke.color)
		dc.DrawPath(strokePath)
		_ = dc.Stroke()
	}
	dc.Pop()
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

func renderGroup(dc *gg.Context, group *GroupElement, state *renderState) {
	local := elementTransformMatrix(&group.Attrs)
	child := *state
	child.matrix = state.matrix.Multiply(local)
	child.opacity *= group.Attrs.Opacity
	if group.Attrs.Fill != "" {
		child.parentFill = group.Attrs.Fill
	}
	if group.Attrs.Stroke != "" {
		child.parentStroke = group.Attrs.Stroke
	}

	dc.Push()
	dc.Transform(local)
	renderElements(dc, group.Children, &child)
	dc.Pop()
}

func resolveStyle(attrs *Attrs, state *renderState) resolvedStyle {
	fillString := resolveFill(attrs, state)
	strokeString := resolveStroke(attrs, state)

	fillRule := gg.FillRuleNonZero
	rule := attrs.FillRule
	if rule == "" {
		rule = attrs.ClipRule
	}
	if rule == "evenodd" {
		fillRule = gg.FillRuleEvenOdd
	}

	style := resolvedStyle{
		fill: resolvedFill{
			present: fillString != colorNone,
			rule:    fillRule,
		},
		stroke: resolvedStroke{
			present: strokeString != "" && strokeString != colorNone,
			width:   attrs.StrokeWidth,
			cap:     resolveLineCap(attrs.StrokeCap),
			join:    resolveLineJoin(attrs.StrokeJoin),
		},
	}

	fillColor, fillOK := resolvePaintColor(fillString, color.Black, state.overrideColor,
		attrs.FillOpacity*attrs.Opacity*state.opacity)
	style.fill.color = fillColor
	style.fill.present = style.fill.present && fillOK

	strokeColor, strokeOK := resolvePaintColor(strokeString, nil, state.overrideColor,
		attrs.StrokeOpacity*attrs.Opacity*state.opacity)
	style.stroke.color = strokeColor
	style.stroke.present = style.stroke.present && strokeOK
	return style
}

func resolveFill(attrs *Attrs, state *renderState) string {
	if attrs.Fill != "" {
		return attrs.Fill
	}
	if state.parentFill != "" {
		return state.parentFill
	}
	return "" // SVG's default fill is black.
}

func resolveStroke(attrs *Attrs, state *renderState) string {
	if attrs.Stroke != "" {
		return attrs.Stroke
	}
	return state.parentStroke
}

func resolvePaintColor(value string, fallback color.Color, override color.Color, opacity float64) (gg.RGBA, bool) {
	var c color.Color
	if override != nil && value != colorNone {
		c = override
	} else {
		parsed, err := parseColor(value)
		if err != nil || parsed == nil {
			c = fallback
		} else {
			c = parsed
		}
	}
	if c == nil {
		return gg.RGBA{}, false
	}
	if opacity < 0 {
		opacity = 0
	} else if opacity > 1 {
		opacity = 1
	}
	result := gg.FromColor(c)
	result.A *= opacity
	return result, true
}

func resolveLineCap(value string) gg.LineCap {
	switch value {
	case "round":
		return gg.LineCapRound
	case "square":
		return gg.LineCapSquare
	default:
		return gg.LineCapButt
	}
}

func resolveLineJoin(value string) gg.LineJoin {
	switch value {
	case "round":
		return gg.LineJoinRound
	case "bevel":
		return gg.LineJoinBevel
	default:
		return gg.LineJoinMiter
	}
}

func setResolvedColor(dc *gg.Context, c gg.RGBA) {
	dc.SetRGBA(c.R, c.G, c.B, c.A)
}

func elementTransformMatrix(attrs *Attrs) gg.Matrix {
	if attrs == nil || attrs.Transform == "" {
		return gg.Identity()
	}
	matrix, _ := transformMatrix(attrs.Transform)
	return matrix
}

func pointsPath(points []float64, closed bool) *gg.Path {
	if len(points) < 4 {
		return nil
	}
	path := gg.NewPath()
	for i := 0; i+1 < len(points); i += 2 {
		if i == 0 {
			path.MoveTo(points[i], points[i+1])
		} else {
			path.LineTo(points[i], points[i+1])
		}
	}
	if closed {
		path.Close()
	}
	return path
}
