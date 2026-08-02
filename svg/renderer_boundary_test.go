package svg

import (
	"image/color"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

func TestImmediateRendererSkipsInvalidAndInvisibleGeometry(t *testing.T) {
	dc := gg.NewContext(16, 16)
	state := &renderState{opacity: 1, matrix: gg.Identity()}
	attrs := testPresentationAttrs()

	renderElements(dc, []Element{
		&PathElement{Attrs: attrs},
		&PathElement{Attrs: attrs, D: "M definitely-not-a-number"},
	}, state)

	path := gg.NewPath()
	path.Circle(8, 8, 4)
	invisible := attrs
	invisible.Fill = colorNone
	invisible.Stroke = colorNone
	renderGeometry(dc, path, &invisible, state, true)

	renderElement(dc, &PolygonElement{Attrs: attrs, Points: []float64{1, 2}}, state)
	renderElement(dc, &PolylineElement{Attrs: attrs, Points: []float64{1, 2}}, state)
	if got := pointsPath([]float64{1, 2}, true); got != nil {
		t.Fatalf("pointsPath accepted one point: %+v", got)
	}

	if imageHasAlpha(dc) {
		t.Fatal("invalid or explicitly invisible geometry produced pixels")
	}
}

func TestImmediateRendererBoundaryStylesAndGeometry(t *testing.T) {
	doc, err := Parse([]byte(`<svg viewBox="0 0 20 20">
  <rect x="1" y="1" width="7" height="6" rx="1" ry="3" fill="red"/>
  <g transform="translate(0 1)" stroke="blue" opacity=".5">
    <path d="M2 12 L18 12 L18 17" fill="none" stroke-width="2" stroke-linecap="square" stroke-linejoin="round"/>
  </g>
</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	img := doc.Render(20, 20)
	assertNonEmpty(t, img, "rounded rectangle and inherited stroke")
	if alphaOf(img.At(2, 2)) == 0 {
		t.Error("rounded rectangle with ry greater than rx was not filled")
	}
	if alphaOf(img.At(10, 13)) == 0 {
		t.Error("group-inherited stroke was not rendered")
	}
}

func TestResolvedPaintAndStrokeBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		opacity float64
		wantA   float64
	}{
		{name: "negative clamps transparent", opacity: -0.1, wantA: 0},
		{name: "fractional preserved", opacity: 0.4, wantA: 0.4},
		{name: "above one clamps opaque", opacity: 1.1, wantA: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := resolvePaintColor("red", nil, nil, test.opacity)
			if !ok || got.R != 1 || got.G != 0 || got.B != 0 || got.A != test.wantA {
				t.Fatalf("resolvePaintColor(red, %g)=(%+v,%v), want red alpha %g", test.opacity, got, ok, test.wantA)
			}
		})
	}

	if got, ok := resolvePaintColor("not-a-color", nil, nil, 1); ok || got != (gg.RGBA{}) {
		t.Fatalf("invalid stroke color=(%+v,%v), want unavailable", got, ok)
	}
	got, ok := resolvePaintColor("not-a-color", color.Black, color.NRGBA{R: 12, G: 34, B: 56, A: 128}, 1)
	if !ok || got.A == 0 || got.R == 0 {
		t.Fatalf("override color was not used for non-none paint: (%+v,%v)", got, ok)
	}

	capTests := map[string]gg.LineCap{
		"": gg.LineCapButt, "round": gg.LineCapRound, "square": gg.LineCapSquare,
	}
	for input, want := range capTests {
		if got := resolveLineCap(input); got != want {
			t.Errorf("resolveLineCap(%q)=%v, want %v", input, got, want)
		}
		if got := sceneLineCap(want); got != scene.LineCap(want) {
			t.Errorf("sceneLineCap(%v)=%v, want %v", want, got, scene.LineCap(want))
		}
	}
	joinTests := map[string]gg.LineJoin{
		"": gg.LineJoinMiter, "round": gg.LineJoinRound, "bevel": gg.LineJoinBevel,
	}
	for input, want := range joinTests {
		if got := resolveLineJoin(input); got != want {
			t.Errorf("resolveLineJoin(%q)=%v, want %v", input, got, want)
		}
		if got := sceneLineJoin(want); got != scene.LineJoin(want) {
			t.Errorf("sceneLineJoin(%v)=%v, want %v", want, got, scene.LineJoin(want))
		}
	}
}

func TestSceneRendererBoundaryElements(t *testing.T) {
	attrs := testPresentationAttrs()
	red := attrs
	red.Fill = "red"
	stroke := attrs
	stroke.Fill = colorNone
	stroke.Stroke = "blue"
	stroke.StrokeCap = "square"
	stroke.StrokeJoin = "round"
	stroke.StrokeWidth = 2

	doc := &Document{
		ViewBox: ViewBox{Width: 20, Height: 20},
		Elements: []Element{
			&PathElement{Attrs: attrs},
			&PathElement{Attrs: attrs, D: "M invalid"},
			&RectElement{Attrs: red, X: 1, Y: 1, W: 4, H: 4},
			&RectElement{Attrs: red, X: 7, Y: 1, W: 5, H: 6, RX: 1, RY: 2},
			&PathElement{Attrs: stroke, D: "M2 12 L18 12 L18 17"},
			&PolygonElement{Attrs: red, Points: []float64{1, 2}},
			&PolylineElement{Attrs: red, Points: []float64{1, 2}},
			&GroupElement{Attrs: attrs, Children: []Element{
				&CircleElement{Attrs: red, CX: 15, CY: 5, R: 2},
			}},
		},
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 20, 20)

	var fills int
	var gotStroke *scene.StrokeStyle
	dec := scene.NewDecoder(s.Encoding())
	for dec.Next() {
		switch dec.Tag() {
		case scene.TagFill:
			fills++
			_, _ = dec.Fill()
		case scene.TagFillRoundRect:
			fills++
			_, _, _, _, _ = dec.FillRoundRect()
		case scene.TagStroke:
			_, gotStroke = dec.Stroke()
		case scene.TagTransform:
			_ = dec.Transform()
		}
	}
	if fills != 3 {
		t.Errorf("fill command count=%d, want 3 valid filled shapes", fills)
	}
	if gotStroke == nil || gotStroke.Width != 2 || gotStroke.Cap != scene.LineCapSquare || gotStroke.Join != scene.LineJoinRound {
		t.Fatalf("stroke style=%+v, want width=2 square cap round join", gotStroke)
	}
}

func testPresentationAttrs() Attrs {
	return Attrs{FillOpacity: 1, StrokeOpacity: 1, StrokeWidth: 1, Opacity: 1}
}

func imageHasAlpha(dc *gg.Context) bool {
	bounds := dc.Image().Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := dc.Image().At(x, y).RGBA()
			if alpha != 0 {
				return true
			}
		}
	}
	return false
}
