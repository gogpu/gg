package svg

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

const sceneSemanticSVG = `<svg viewBox="1 2 10 20" fill="none">
<g transform="translate(2 3)" fill="#102030" stroke="#405060">
  <path d="M0 0 L1 0 Q2 1 3 0 C4 0 5 1 6 0 A2 2 0 0 1 8 2 Z" fill-rule="evenodd" fill-opacity=".5" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="bevel" stroke-opacity=".25"/>
  <circle cx="2" cy="2" r="1"/><rect x="0" y="0" width="2" height="3" rx=".5"/>
  <ellipse cx="3" cy="4" rx="2" ry="1"/><line x1="0" y1="1" x2="2" y2="1"/>
  <polygon points="0,0 2,0 1,2"/><polyline points="0,0 1,1 2,0"/>
</g></svg>`

func TestRenderToSceneContract(t *testing.T) {
	doc, err := Parse([]byte(sceneSemanticSVG))
	if err != nil {
		t.Fatal(err)
	}

	doc.RenderToScene(nil, 0, 0, 16, 16)
	var nilDoc *Document
	nilDoc.RenderToScene(scene.NewScene(), 0, 0, 16, 16)
	for _, target := range []struct{ width, height float32 }{
		{width: 0, height: 16}, {width: 16, height: 0}, {width: -1, height: 16},
	} {
		s := scene.NewScene()
		doc.RenderToScene(s, 0, 0, target.width, target.height)
		if len(s.Encoding().Tags()) != 0 {
			t.Fatalf("invalid target %+v emitted commands", target)
		}
	}

	s := scene.NewScene()
	doc.RenderToScene(s, 4, 5, 20, 40)
	tags := s.Encoding().Tags()
	if !containsTag(tags, scene.TagFill) || !containsTag(tags, scene.TagStroke) {
		t.Fatalf("tags %v do not contain fill and stroke", tags)
	}
	if containsTag(tags, scene.TagImage) {
		t.Fatalf("retained SVG emitted image tag: %v", tags)
	}

	invalidDoc := *doc
	invalidDoc.ViewBox.Width = 0
	invalidScene := scene.NewScene()
	invalidDoc.RenderToScene(invalidScene, 0, 0, 16, 16)
	if len(invalidScene.Encoding().Tags()) != 0 {
		t.Fatal("non-positive viewBox emitted commands")
	}
}

func TestRenderToScenePreservesNativeRoundRect(t *testing.T) {
	doc, err := Parse([]byte(`<svg viewBox="0 0 20 20"><rect x="2" y="3" width="16" height="14" rx="4" fill="#3979c3"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}

	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 20, 20)
	if tags := s.Encoding().Tags(); !containsTag(tags, scene.TagFillRoundRect) {
		t.Fatalf("rounded rectangle lost native retained encoding: tags=%v", tags)
	}
}

func TestRenderToSceneTransformStyleAndOverride(t *testing.T) {
	doc, err := Parse([]byte(sceneSemanticSVG))
	if err != nil {
		t.Fatal(err)
	}
	s := scene.NewScene()
	doc.RenderToSceneWithColor(s, 4, 5, 20, 40,
		gg.FromColor(color.NRGBA{R: 200, G: 100, B: 50, A: 128}))

	dec := scene.NewDecoder(s.Encoding())
	var gotTransform scene.Affine
	var gotFill bool
	var gotStroke bool
	var sawQuad, sawCubic, sawClose bool
	for dec.Next() {
		switch dec.Tag() {
		case scene.TagTransform:
			if gotTransform == (scene.Affine{}) {
				gotTransform = dec.Transform()
			} else {
				_ = dec.Transform()
			}
		case scene.TagMoveTo:
			_, _ = dec.MoveTo()
		case scene.TagLineTo:
			_, _ = dec.LineTo()
		case scene.TagQuadTo:
			sawQuad = true
			_, _, _, _ = dec.QuadTo()
		case scene.TagCubicTo:
			sawCubic = true
			_, _, _, _, _, _ = dec.CubicTo()
		case scene.TagClosePath:
			sawClose = true
		case scene.TagFill:
			brush, style := dec.Fill()
			if !gotFill {
				gotFill = true
				if style != scene.FillEvenOdd {
					t.Errorf("fill rule=%v, want evenodd", style)
				}
				if brush.Color.A < .24 || brush.Color.A > .26 {
					t.Errorf("override fill alpha=%g, want about .25", brush.Color.A)
				}
			}
		case scene.TagStroke:
			brush, style := dec.Stroke()
			if !gotStroke {
				gotStroke = true
				if style.Width != 1.5 || style.Cap != scene.LineCapRound || style.Join != scene.LineJoinBevel {
					t.Errorf("stroke style=%+v", style)
				}
				if brush.Color.A < .12 || brush.Color.A > .13 {
					t.Errorf("override stroke alpha=%g, want about .125", brush.Color.A)
				}
			}
		}
	}
	// Root: translate(4,5)*scale(2,2)*translate(-1,-2), group translate(2,3).
	want := scene.NewAffine(2, 0, 6, 0, 2, 7)
	if gotTransform != want {
		t.Errorf("first transform=%+v, want %+v", gotTransform, want)
	}
	if !gotFill || !gotStroke || !sawQuad || !sawCubic || !sawClose {
		t.Errorf("semantic commands fill=%v stroke=%v quad=%v cubic=%v close=%v", gotFill, gotStroke, sawQuad, sawCubic, sawClose)
	}
}

func TestRenderToSceneDeterministic(t *testing.T) {
	doc, err := Parse([]byte(sceneSemanticSVG))
	if err != nil {
		t.Fatal(err)
	}
	a, b := scene.NewScene(), scene.NewScene()
	doc.RenderToScene(a, .25, .75, 24, 24)
	doc.RenderToScene(b, .25, .75, 24, 24)
	ea, eb := a.Encoding(), b.Encoding()
	if !reflect.DeepEqual(ea.Tags(), eb.Tags()) || !reflect.DeepEqual(ea.PathData(), eb.PathData()) ||
		!reflect.DeepEqual(ea.DrawData(), eb.DrawData()) || !reflect.DeepEqual(ea.Transforms(), eb.Transforms()) ||
		!reflect.DeepEqual(ea.Brushes(), eb.Brushes()) {
		t.Fatal("repeated lowering was not deterministic")
	}
}

func TestRenderToSceneImmediateParity(t *testing.T) {
	doc, err := Parse([]byte(`<svg viewBox="0 0 16 16"><path fill="#3979c3" fill-opacity=".7" d="M2 2h12v5H9v7H2z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{16, 32, 64} {
		t.Run(fmt.Sprintf("%d", size), func(t *testing.T) {
			immediate := gg.NewContext(size+4, size+4)
			doc.RenderTo(immediate, 1.25, .75, float64(size), float64(size))

			retained := gg.NewContext(size+4, size+4)
			s := scene.NewScene()
			doc.RenderToScene(s, 1.25, .75, float32(size), float32(size))
			if err := scene.NewGPUSceneRenderer(retained).RenderScene(s); err != nil {
				t.Fatal(err)
			}
			assertImagesNear(t, immediate.Image(), retained.Image(), 2)
		})
	}
}

func containsTag(tags []scene.Tag, wanted scene.Tag) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

func assertImagesNear(t *testing.T, a, b interface {
	Bounds() image.Rectangle
	At(int, int) color.Color
}, tolerance uint32) {
	t.Helper()
	if a.Bounds() != b.Bounds() {
		t.Fatalf("bounds differ: %v vs %v", a.Bounds(), b.Bounds())
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			for _, pair := range [][2]uint32{{ar, br}, {ag, bg}, {ab, bb}, {aa, ba}} {
				delta := pair[0]
				if pair[1] > delta {
					delta = pair[1] - delta
				} else {
					delta -= pair[1]
				}
				if delta > tolerance*257 {
					t.Fatalf("pixel (%d,%d) differs beyond %d: %v vs %v", x, y, tolerance, a.At(x, y), b.At(x, y))
				}
			}
		}
	}
}

func BenchmarkSVGScene(b *testing.B) {
	doc, err := Parse([]byte(sceneSemanticSVG))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		s := scene.NewScene()
		doc.RenderToScene(s, 0, 0, 24, 24)
	}
}
