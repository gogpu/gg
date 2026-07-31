package svg

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// sceneRenderState holds the rendering state during SVG-to-scene traversal.
type sceneRenderState struct {
	overrideColor *gg.RGBA // non-nil → replace all non-"none" colors
	parentFill    string   // inherited fill from parent <g>
	parentStroke  string   // inherited stroke from parent <g>
	strokeHinting bool     // true → snap thin stroke coords to pixel centers
	scaleX        float64  // viewBox → canvas X scale (for hinting)
	scaleY        float64  // viewBox → canvas Y scale (for hinting)
}

// renderToSceneInternal is the shared implementation for scene rendering.
func (d *Document) renderToSceneInternal(
	s *scene.Scene,
	x, y, w, h float32,
	overrideColor *gg.RGBA,
) {
	if d.ViewBox.Width <= 0 || d.ViewBox.Height <= 0 {
		return
	}

	// Compute viewBox → target scaling.
	sx := w / float32(d.ViewBox.Width)
	sy := h / float32(d.ViewBox.Height)

	// Build the viewBox-to-target transform.
	viewBoxTransform := scene.IdentityAffine()

	// Position at (x, y).
	if x != 0 || y != 0 {
		viewBoxTransform = viewBoxTransform.Multiply(scene.TranslateAffine(x, y))
	}

	// Scale from viewBox to target size.
	viewBoxTransform = viewBoxTransform.Multiply(scene.ScaleAffine(sx, sy))

	// Apply viewBox minX/minY offset.
	if d.ViewBox.MinX != 0 || d.ViewBox.MinY != 0 {
		viewBoxTransform = viewBoxTransform.Multiply(
			scene.TranslateAffine(-float32(d.ViewBox.MinX), -float32(d.ViewBox.MinY)),
		)
	}

	s.PushTransform(viewBoxTransform)

	// Enable stroke hinting for small icons (same criteria as bitmap renderer).
	maxDim := math.Max(float64(w), float64(h))
	hinting := maxDim <= strokeHintMaxCanvasSize && !strokeHintingDisabled()

	state := &sceneRenderState{
		overrideColor: overrideColor,
		parentFill:    d.RootFill,
		strokeHinting: hinting,
		scaleX:        float64(sx),
		scaleY:        float64(sy),
	}

	sceneRenderElements(s, d.Elements, state)

	s.PopTransform()
}

// sceneRenderElements renders a list of SVG elements into the scene.
func sceneRenderElements(s *scene.Scene, elements []Element, state *sceneRenderState) {
	for _, elem := range elements {
		sceneRenderElement(s, elem, state)
	}
}

// sceneRenderElement dispatches rendering to the appropriate element handler.
func sceneRenderElement(s *scene.Scene, elem Element, state *sceneRenderState) {
	switch e := elem.(type) {
	case *PathElement:
		sceneRenderPath(s, e, state)
	case *CircleElement:
		sceneRenderCircle(s, e, state)
	case *RectElement:
		sceneRenderRect(s, e, state)
	case *EllipseElement:
		sceneRenderEllipse(s, e, state)
	case *LineElement:
		sceneRenderLine(s, e, state)
	case *PolygonElement:
		sceneRenderPolygon(s, e, state)
	case *PolylineElement:
		sceneRenderPolyline(s, e, state)
	case *GroupElement:
		sceneRenderGroup(s, e, state)
	}
}

// sceneRenderPath renders an SVG <path> element to the scene.
func sceneRenderPath(s *scene.Scene, e *PathElement, state *sceneRenderState) {
	if e.D == "" {
		return
	}
	ggPath, err := gg.ParseSVGPath(e.D)
	if err != nil {
		return // skip invalid paths silently
	}

	fillShape := scene.NewGGPathShape(ggPath)
	strokeShape := fillShape

	// Apply stroke hinting: snap thin stroke endpoints to pixel centers.
	// Reuses hintSVGPath from renderer.go. Same criteria as bitmap path.
	if state.strokeHinting && e.Attrs.Stroke != "" && e.Attrs.Stroke != colorNone {
		avgScale := (state.scaleX + state.scaleY) / 2.0
		deviceWidth := e.Attrs.StrokeWidth * avgScale
		if deviceWidth <= strokeHintMaxWidth {
			hintedPath := hintSVGPath(ggPath, state.scaleX, state.scaleY)
			strokeShape = scene.NewGGPathShape(hintedPath)
		}
	}

	pushSceneTransform(s, &e.Attrs)
	sceneApplyFillAndStroke(s, &e.Attrs, state, fillShape, strokeShape)
	popSceneTransform(s, &e.Attrs)
}

// sceneRenderCircle renders an SVG <circle> element to the scene.
// Uses native CircleShape for SDF-friendly rendering.
func sceneRenderCircle(s *scene.Scene, e *CircleElement, state *sceneRenderState) {
	shape := scene.NewCircleShape(float32(e.CX), float32(e.CY), float32(e.R))

	pushSceneTransform(s, &e.Attrs)
	sceneApplyFillAndStroke(s, &e.Attrs, state, shape, shape)
	popSceneTransform(s, &e.Attrs)
}

// sceneRenderRect renders an SVG <rect> element to the scene.
// Uses RoundRectShape for rounded rects (SDF-based) or RectShape for plain rects.
func sceneRenderRect(s *scene.Scene, e *RectElement, state *sceneRenderState) {
	var fillShape, strokeShape scene.Shape

	if e.RX > 0 || e.RY > 0 {
		// Use the larger of rx/ry for the rounded rectangle radius (SVG spec).
		rx := float32(e.RX)
		ry := float32(e.RY)
		if ry > rx {
			rx = ry
		}
		rect := scene.Rect{
			MinX: float32(e.X),
			MinY: float32(e.Y),
			MaxX: float32(e.X + e.W),
			MaxY: float32(e.Y + e.H),
		}
		rr := scene.NewRoundRectShapeUniform(rect, rx)
		fillShape = rr
		strokeShape = rr
	} else {
		rs := scene.NewRectShape(float32(e.X), float32(e.Y), float32(e.W), float32(e.H))
		fillShape = rs
		strokeShape = rs
	}

	pushSceneTransform(s, &e.Attrs)
	sceneApplyFillAndStroke(s, &e.Attrs, state, fillShape, strokeShape)
	popSceneTransform(s, &e.Attrs)
}

// sceneRenderEllipse renders an SVG <ellipse> element to the scene.
func sceneRenderEllipse(s *scene.Scene, e *EllipseElement, state *sceneRenderState) {
	shape := scene.NewEllipseShape(float32(e.CX), float32(e.CY), float32(e.RX), float32(e.RY))

	pushSceneTransform(s, &e.Attrs)
	sceneApplyFillAndStroke(s, &e.Attrs, state, shape, shape)
	popSceneTransform(s, &e.Attrs)
}

// sceneRenderLine renders an SVG <line> element to the scene.
// Lines are stroke-only by default.
func sceneRenderLine(s *scene.Scene, e *LineElement, state *sceneRenderState) {
	shape := scene.NewLineShape(float32(e.X1), float32(e.Y1), float32(e.X2), float32(e.Y2))

	pushSceneTransform(s, &e.Attrs)

	// Lines are stroke-only in SVG.
	strokeStr := resolveStroke(&e.Attrs, toRenderState(state))
	if strokeStr == "" || strokeStr == colorNone {
		// Default stroke for lines is currentColor, which we treat as black.
		strokeStr = "black"
	}
	brush := resolveSceneBrush(strokeStr, state, e.Attrs.StrokeOpacity*e.Attrs.Opacity)
	style := buildSceneStrokeStyle(&e.Attrs)
	s.Stroke(style, scene.IdentityAffine(), brush, shape)

	popSceneTransform(s, &e.Attrs)
}

// sceneRenderPolygon renders an SVG <polygon> element to the scene.
func sceneRenderPolygon(s *scene.Scene, e *PolygonElement, state *sceneRenderState) {
	if len(e.Points) < 4 {
		return // need at least 2 points
	}

	pts := make([]float32, len(e.Points))
	for i, v := range e.Points {
		pts[i] = float32(v)
	}
	shape := scene.NewPolygonShape(pts...)

	pushSceneTransform(s, &e.Attrs)
	sceneApplyFillAndStroke(s, &e.Attrs, state, shape, shape)
	popSceneTransform(s, &e.Attrs)
}

// sceneRenderPolyline renders an SVG <polyline> element to the scene.
func sceneRenderPolyline(s *scene.Scene, e *PolylineElement, state *sceneRenderState) {
	if len(e.Points) < 4 {
		return
	}

	// Build a path from points (polyline = open path, not closed).
	p := scene.NewPath()
	for i := 0; i+1 < len(e.Points); i += 2 {
		if i == 0 {
			p.MoveTo(float32(e.Points[i]), float32(e.Points[i+1]))
		} else {
			p.LineTo(float32(e.Points[i]), float32(e.Points[i+1]))
		}
	}
	shape := scene.NewPathShape(p)

	pushSceneTransform(s, &e.Attrs)
	sceneApplyFillAndStroke(s, &e.Attrs, state, shape, shape)
	popSceneTransform(s, &e.Attrs)
}

// sceneRenderGroup renders an SVG <g> element and its children.
func sceneRenderGroup(s *scene.Scene, e *GroupElement, state *sceneRenderState) {
	pushSceneTransform(s, &e.Attrs)

	// Create child state with inherited attrs.
	childState := &sceneRenderState{
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

	// Group-level opacity uses PushLayer for correct compositing.
	hasGroupOpacity := e.Attrs.Opacity < 1.0
	if hasGroupOpacity {
		s.PushLayer(scene.BlendNormal, float32(e.Attrs.Opacity), nil)
	}

	sceneRenderElements(s, e.Children, childState)

	if hasGroupOpacity {
		s.PopLayer()
	}

	popSceneTransform(s, &e.Attrs)
}

// sceneApplyFillAndStroke applies fill and/or stroke to the scene for a shape.
// Mirrors the logic of fillAndStroke in renderer.go.
func sceneApplyFillAndStroke(
	s *scene.Scene,
	a *Attrs,
	state *sceneRenderState,
	fillShape, strokeShape scene.Shape,
) {
	// Use the shared state helpers via a temporary renderState adapter.
	rs := toRenderState(state)
	hasFill := shouldFill(a, rs)
	hasStroke := shouldStroke(a, rs)

	if !hasFill && !hasStroke {
		// Default SVG behavior: fill with black if no fill/stroke specified.
		if a.Fill == "" && a.Stroke == "" {
			hasFill = true
		}
	}

	if hasFill {
		fillStyle := sceneFillStyle(a)
		brush := resolveSceneFillBrush(a, state)
		s.Fill(fillStyle, scene.IdentityAffine(), brush, fillShape)
	}

	if hasStroke {
		brush := resolveSceneStrokeBrush(a, state)
		style := buildSceneStrokeStyle(a)
		s.Stroke(style, scene.IdentityAffine(), brush, strokeShape)
	}
}

// --- Transform helpers ---

// pushSceneTransform pushes the element's transform onto the scene's transform
// stack, if the element has a transform attribute.
func pushSceneTransform(s *scene.Scene, a *Attrs) {
	if a.Transform == "" {
		return
	}
	affine, err := parseSVGTransformAffine(a.Transform)
	if err != nil {
		return // silently ignore bad transforms (matching renderer.go behavior)
	}
	s.PushTransform(affine)
}

// popSceneTransform pops the transform if one was pushed.
func popSceneTransform(s *scene.Scene, a *Attrs) {
	if a.Transform == "" {
		return
	}
	s.PopTransform()
}

// parseSVGTransformAffine parses an SVG transform attribute string into a
// scene.Affine. This parallels applyTransform in transform.go but builds
// a scene.Affine instead of modifying a gg.Context.
func parseSVGTransformAffine(transform string) (scene.Affine, error) {
	transform = strings.TrimSpace(transform)
	if transform == "" {
		return scene.IdentityAffine(), nil
	}

	result := scene.IdentityAffine()
	pos := 0

	for pos < len(transform) {
		// Skip whitespace.
		for pos < len(transform) && isSpace(transform[pos]) {
			pos++
		}
		if pos >= len(transform) {
			break
		}

		// Read function name.
		nameStart := pos
		for pos < len(transform) && transform[pos] != '(' && !isSpace(transform[pos]) {
			pos++
		}
		name := transform[nameStart:pos]

		// Skip whitespace before '('.
		for pos < len(transform) && isSpace(transform[pos]) {
			pos++
		}
		if pos >= len(transform) || transform[pos] != '(' {
			return scene.IdentityAffine(), fmt.Errorf("svg: expected '(' after transform function %q", name)
		}
		pos++ // skip '('

		// Find closing ')'.
		parenStart := pos
		for pos < len(transform) && transform[pos] != ')' {
			pos++
		}
		if pos >= len(transform) {
			return scene.IdentityAffine(), fmt.Errorf("svg: missing ')' in transform %q", name)
		}
		argsStr := transform[parenStart:pos]
		pos++ // skip ')'

		args, err := parseTransformArgsFloat32(argsStr)
		if err != nil {
			return scene.IdentityAffine(), fmt.Errorf("svg: transform %s: %w", name, err)
		}

		t, err := applyTransformAffine(name, args)
		if err != nil {
			return scene.IdentityAffine(), err
		}
		result = result.Multiply(t)
	}

	return result, nil
}

// applyTransformAffine builds a scene.Affine from a single SVG transform function.
func applyTransformAffine(name string, args []float32) (scene.Affine, error) {
	switch name {
	case "translate":
		if len(args) < 1 {
			return scene.IdentityAffine(), fmt.Errorf("svg: translate requires at least 1 arg, got %d", len(args))
		}
		tx := args[0]
		var ty float32
		if len(args) >= 2 {
			ty = args[1]
		}
		return scene.TranslateAffine(tx, ty), nil

	case "rotate":
		switch {
		case len(args) == 1:
			angle := args[0] * math.Pi / 180.0
			return scene.RotateAffine(angle), nil
		case len(args) >= 3:
			angle := args[0] * math.Pi / 180.0
			cx, cy := args[1], args[2]
			// rotate(angle, cx, cy) = translate(cx,cy) * rotate(angle) * translate(-cx,-cy)
			t := scene.TranslateAffine(cx, cy)
			r := scene.RotateAffine(angle)
			tInv := scene.TranslateAffine(-cx, -cy)
			return t.Multiply(r).Multiply(tInv), nil
		default:
			return scene.IdentityAffine(), fmt.Errorf("svg: rotate requires 1 or 3 args, got %d", len(args))
		}

	case "scale":
		if len(args) < 1 {
			return scene.IdentityAffine(), fmt.Errorf("svg: scale requires at least 1 arg, got %d", len(args))
		}
		sx := args[0]
		sy := sx
		if len(args) >= 2 {
			sy = args[1]
		}
		return scene.ScaleAffine(sx, sy), nil

	case "matrix":
		if len(args) != 6 {
			return scene.IdentityAffine(), fmt.Errorf("svg: matrix requires 6 args, got %d", len(args))
		}
		// SVG matrix(a,b,c,d,e,f): x' = a*x + c*y + e, y' = b*x + d*y + f
		// scene.Affine{A,B,C,D,E,F}: x' = A*x + B*y + C, y' = D*x + E*y + F
		return scene.NewAffine(args[0], args[2], args[4], args[1], args[3], args[5]), nil

	case "skewX":
		if len(args) != 1 {
			return scene.IdentityAffine(), fmt.Errorf("svg: skewX requires 1 arg, got %d", len(args))
		}
		angle := float32(math.Tan(float64(args[0]) * math.Pi / 180.0))
		return scene.NewAffine(1, angle, 0, 0, 1, 0), nil

	case "skewY":
		if len(args) != 1 {
			return scene.IdentityAffine(), fmt.Errorf("svg: skewY requires 1 arg, got %d", len(args))
		}
		angle := float32(math.Tan(float64(args[0]) * math.Pi / 180.0))
		return scene.NewAffine(1, 0, 0, angle, 1, 0), nil

	default:
		return scene.IdentityAffine(), fmt.Errorf("svg: unsupported transform function %q", name)
	}
}

// parseTransformArgsFloat32 parses comma/space-separated transform args as float32.
func parseTransformArgsFloat32(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	s = strings.ReplaceAll(s, ",", " ")
	parts := strings.Fields(s)

	args := make([]float32, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", p, err)
		}
		args = append(args, float32(v))
	}
	return args, nil
}

// --- Color/Brush helpers ---

// colorToRGBA converts a color.Color to gg.RGBA with an opacity multiplier.
func colorToRGBA(c color.Color, opacity float64) gg.RGBA {
	if c == nil {
		return gg.RGBA{R: 0, G: 0, B: 0, A: opacity}
	}
	r, g, b, a := c.RGBA()
	// Un-premultiply from color.Color's premultiplied format.
	if a == 0 {
		return gg.RGBA{R: 0, G: 0, B: 0, A: 0}
	}
	fa := float64(a) / 65535.0
	return gg.RGBA{
		R: float64(r) / 65535.0 / fa,
		G: float64(g) / 65535.0 / fa,
		B: float64(b) / 65535.0 / fa,
		A: fa * opacity,
	}
}

// resolveSceneFillBrush creates a scene.Brush for filling, respecting override colors.
func resolveSceneFillBrush(a *Attrs, state *sceneRenderState) scene.Brush {
	fillStr := resolveFill(a, toRenderState(state))
	opacity := a.FillOpacity * a.Opacity

	if state.overrideColor != nil && fillStr != colorNone {
		c := *state.overrideColor
		c.A *= opacity
		return scene.SolidBrush(c)
	}

	return resolveSceneBrush(fillStr, state, opacity)
}

// resolveSceneStrokeBrush creates a scene.Brush for stroking, respecting override colors.
func resolveSceneStrokeBrush(a *Attrs, state *sceneRenderState) scene.Brush {
	strokeStr := resolveStroke(a, toRenderState(state))
	opacity := a.StrokeOpacity * a.Opacity

	if state.overrideColor != nil && strokeStr != colorNone {
		c := *state.overrideColor
		c.A *= opacity
		return scene.SolidBrush(c)
	}

	return resolveSceneBrush(strokeStr, state, opacity)
}

// resolveSceneBrush creates a scene.Brush from an SVG color string.
func resolveSceneBrush(colorStr string, _ *sceneRenderState, opacity float64) scene.Brush {
	c, err := parseColor(colorStr)
	if err != nil || c == nil {
		// Default fill is black per SVG spec.
		return scene.SolidBrush(gg.RGBA{R: 0, G: 0, B: 0, A: opacity})
	}
	return scene.SolidBrush(colorToRGBA(c, opacity))
}

// sceneFillStyle converts SVG fill-rule to scene.FillStyle.
func sceneFillStyle(a *Attrs) scene.FillStyle {
	fillRule := a.FillRule
	if fillRule == "" {
		fillRule = a.ClipRule
	}
	if fillRule == "evenodd" {
		return scene.FillEvenOdd
	}
	return scene.FillNonZero
}

// buildSceneStrokeStyle creates a scene.StrokeStyle from SVG stroke attributes.
func buildSceneStrokeStyle(a *Attrs) *scene.StrokeStyle {
	style := scene.DefaultStrokeStyle()
	style.Width = float32(a.StrokeWidth)

	switch a.StrokeCap {
	case "round":
		style.Cap = scene.LineCapRound
	case "square":
		style.Cap = scene.LineCapSquare
	default:
		style.Cap = scene.LineCapButt
	}

	switch a.StrokeJoin {
	case "round":
		style.Join = scene.LineJoinRound
	case "bevel":
		style.Join = scene.LineJoinBevel
	default:
		style.Join = scene.LineJoinMiter
	}

	return style
}

// toRenderState creates a temporary renderState adapter for reusing
// shouldFill, shouldStroke, resolveFill, resolveStroke helpers.
func toRenderState(state *sceneRenderState) *renderState {
	var override color.Color
	if state.overrideColor != nil {
		override = state.overrideColor
	}
	return &renderState{
		overrideColor: override,
		parentFill:    state.parentFill,
		parentStroke:  state.parentStroke,
	}
}
