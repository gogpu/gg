package svg

import (
	"image/color"
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// renderToSceneInternal is the shared implementation for retained rendering.
// It preserves the public positional RenderToScene API while lowering through
// the same geometry and style model as the immediate renderer.
func (d *Document) renderToSceneInternal(
	target *scene.Scene,
	x, y, width, height float32,
	overrideColor *gg.RGBA,
) {
	if d == nil || target == nil || d.ViewBox.Width <= 0 || d.ViewBox.Height <= 0 ||
		width <= 0 || height <= 0 {
		return
	}

	root := gg.Translate(float64(x), float64(y)).
		Multiply(gg.Scale(float64(width)/d.ViewBox.Width, float64(height)/d.ViewBox.Height)).
		Multiply(gg.Translate(-d.ViewBox.MinX, -d.ViewBox.MinY))
	outer := matrixFromSceneAffine(target.Transform())
	outerScale := uniformAxisScale(outer)
	var override color.Color
	if overrideColor == nil {
		override = nil
	} else {
		override = *overrideColor
	}
	state := &renderState{
		overrideColor: override,
		parentFill:    d.RootFill,
		opacity:       1,
		matrix:        root,
		targetWidth:   float64(width) * outerScale,
		targetHeight:  float64(height) * outerScale,
		deviceScale:   1,
		outerMatrix:   outer,
	}
	renderSceneElements(target, d.Elements, state)
}

func renderSceneElements(target *scene.Scene, elements []Element, state *renderState) {
	for _, element := range elements {
		renderSceneElement(target, element, state)
	}
}

func renderSceneElement(target *scene.Scene, element Element, state *renderState) {
	switch e := element.(type) {
	case *PathElement:
		if e.D == "" {
			return
		}
		path, err := gg.ParseSVGPath(e.D)
		if err == nil {
			renderSceneGeometry(target, path, nil, &e.Attrs, state, true)
		}
	case *CircleElement:
		path := gg.NewPath()
		path.Circle(e.CX, e.CY, e.R)
		native := scene.NewCircleShape(float32(e.CX), float32(e.CY), float32(e.R))
		renderSceneGeometry(target, path, native, &e.Attrs, state, true)
	case *RectElement:
		path := gg.NewPath()
		var native scene.Shape
		if e.RX > 0 || e.RY > 0 {
			radius := max(e.RX, e.RY)
			path.RoundedRectangle(e.X, e.Y, e.W, e.H, radius)
			native = scene.NewRoundRectShapeUniform(scene.Rect{
				MinX: float32(e.X),
				MinY: float32(e.Y),
				MaxX: float32(e.X + e.W),
				MaxY: float32(e.Y + e.H),
			}, float32(radius))
		} else {
			path.Rectangle(e.X, e.Y, e.W, e.H)
			native = scene.NewRectShape(float32(e.X), float32(e.Y), float32(e.W), float32(e.H))
		}
		renderSceneGeometry(target, path, native, &e.Attrs, state, true)
	case *EllipseElement:
		path := gg.NewPath()
		path.Ellipse(e.CX, e.CY, e.RX, e.RY)
		native := scene.NewEllipseShape(float32(e.CX), float32(e.CY), float32(e.RX), float32(e.RY))
		renderSceneGeometry(target, path, native, &e.Attrs, state, true)
	case *LineElement:
		path := gg.NewPath()
		path.MoveTo(e.X1, e.Y1)
		path.LineTo(e.X2, e.Y2)
		native := scene.NewLineShape(float32(e.X1), float32(e.Y1), float32(e.X2), float32(e.Y2))
		renderSceneGeometry(target, path, native, &e.Attrs, state, false)
	case *PolygonElement:
		if path := pointsPath(e.Points, true); path != nil {
			points := make([]float32, len(e.Points))
			for i, point := range e.Points {
				points[i] = float32(point)
			}
			renderSceneGeometry(target, path, scene.NewPolygonShape(points...), &e.Attrs, state, true)
		}
	case *PolylineElement:
		if path := pointsPath(e.Points, false); path != nil {
			renderSceneGeometry(target, path, nil, &e.Attrs, state, true)
		}
	case *GroupElement:
		renderSceneGroup(target, e, state)
	}
}

func renderSceneGroup(target *scene.Scene, group *GroupElement, state *renderState) {
	child := *state
	child.matrix = state.matrix.Multiply(elementTransformMatrix(&group.Attrs))
	if group.Attrs.Fill != "" {
		child.parentFill = group.Attrs.Fill
	}
	if group.Attrs.Stroke != "" {
		child.parentStroke = group.Attrs.Stroke
	}

	layered := group.Attrs.Opacity < 1
	if layered {
		target.PushLayer(scene.BlendNormal, float32(group.Attrs.Opacity), nil)
	}
	renderSceneElements(target, group.Children, &child)
	if layered {
		target.PopLayer()
	}
}

func renderSceneGeometry(
	target *scene.Scene,
	path *gg.Path,
	native scene.Shape,
	attrs *Attrs,
	state *renderState,
	allowFill bool,
) {
	complete := state.matrix.Multiply(elementTransformMatrix(attrs))
	hintComplete := state.outerMatrix.Multiply(complete)
	style := resolveStyle(attrs, state)
	if !allowFill {
		style.fill.present = false
	}
	transform := scene.AffineFromMatrix(complete)
	if style.fill.present {
		fillRule := scene.FillNonZero
		if style.fill.rule == gg.FillRuleEvenOdd {
			fillRule = scene.FillEvenOdd
		}
		fillShape := native
		if fillShape == nil {
			fillShape = scene.NewGGPathShape(path)
		}
		target.Fill(fillRule, transform, scene.SolidBrush(style.fill.color), fillShape)
	}
	if style.stroke.present {
		policy := newStrokeHintPolicy(state.targetWidth, state.targetHeight, state.deviceScale, hintComplete)
		strokePath := hintStrokePath(path, policy, style.stroke.width)
		strokeShape := native
		if strokeShape == nil || strokePath != path {
			strokeShape = scene.NewGGPathShape(strokePath)
		}
		stroke := &scene.StrokeStyle{
			Width:      float32(style.stroke.width),
			MiterLimit: 10,
			Cap:        sceneLineCap(style.stroke.cap),
			Join:       sceneLineJoin(style.stroke.join),
		}
		target.Stroke(stroke, transform, scene.SolidBrush(style.stroke.color), strokeShape)
	}
}

func sceneLineCap(lineCap gg.LineCap) scene.LineCap {
	switch lineCap {
	case gg.LineCapRound:
		return scene.LineCapRound
	case gg.LineCapSquare:
		return scene.LineCapSquare
	default:
		return scene.LineCapButt
	}
}

func sceneLineJoin(join gg.LineJoin) scene.LineJoin {
	switch join {
	case gg.LineJoinRound:
		return scene.LineJoinRound
	case gg.LineJoinBevel:
		return scene.LineJoinBevel
	default:
		return scene.LineJoinMiter
	}
}

// parseSVGTransformAffine preserves the retained-renderer helper while sharing
// the immediate renderer's transform parser and composition semantics.
func parseSVGTransformAffine(value string) (scene.Affine, error) {
	matrix, err := transformMatrix(value)
	return scene.AffineFromMatrix(matrix), err
}

func matrixFromSceneAffine(a scene.Affine) gg.Matrix {
	return gg.Matrix{
		A: float64(a.A), B: float64(a.B), C: float64(a.C),
		D: float64(a.D), E: float64(a.E), F: float64(a.F),
	}
}

func uniformAxisScale(matrix gg.Matrix) float64 {
	sx, sy := math.Abs(matrix.A), math.Abs(matrix.E)
	if matrix.B == 0 && matrix.D == 0 && sx > 0 && sx == sy {
		return sx
	}
	return 1
}
