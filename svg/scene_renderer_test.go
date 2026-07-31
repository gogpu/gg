package svg

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// --- Scene Renderer Tests (TDD: tests first, implementation follows) ---

func TestSceneRender_PathCircleRect(t *testing.T) {
	// SVG with path, circle, and rect — all should produce scene commands.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
  <rect x="4" y="4" width="40" height="40" fill="#FF0000"/>
  <circle cx="24" cy="24" r="10" fill="#00FF00"/>
  <path d="M 10 10 L 38 10 L 24 38 Z" fill="#0000FF"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 48, 48)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty after RenderToScene")
	}
	if enc.ShapeCount() < 3 {
		t.Errorf("ShapeCount = %d, want >= 3 (rect + circle + path)", enc.ShapeCount())
	}
}

func TestSceneRender_OverrideColor(t *testing.T) {
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <circle cx="8" cy="8" r="6" fill="#FF0000"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	override := gg.RGBA{R: 1, G: 1, B: 1, A: 1} // white
	s := scene.NewScene()
	doc.RenderToSceneWithColor(s, 0, 0, 16, 16, override)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty after RenderToSceneWithColor")
	}
	// Verify the brush in the encoding is white, not the original red.
	brushes := enc.Brushes()
	if len(brushes) == 0 {
		t.Fatal("no brushes in encoding")
	}
	// The first brush should be the override color (white).
	b := brushes[0]
	if b.Color.R != 1.0 || b.Color.G != 1.0 || b.Color.B != 1.0 {
		t.Errorf("brush color = (%.2f, %.2f, %.2f), want (1.0, 1.0, 1.0) white override",
			b.Color.R, b.Color.G, b.Color.B)
	}
}

func TestSceneRender_Transform(t *testing.T) {
	// SVG with translate, rotate, scale transforms.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect x="0" y="0" width="10" height="10" fill="red" transform="translate(5 5)"/>
  <circle cx="16" cy="16" r="4" fill="blue" transform="scale(2)"/>
  <rect x="0" y="0" width="8" height="8" fill="green" transform="rotate(45 4 4)"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 32, 32)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty")
	}
	// 3 shapes with transforms.
	if enc.ShapeCount() < 3 {
		t.Errorf("ShapeCount = %d, want >= 3", enc.ShapeCount())
	}
	// Transforms stream should have entries for each transformed element
	// plus the viewBox scaling transform.
	if len(enc.Transforms()) == 0 {
		t.Error("transforms stream is empty, expected transform entries")
	}
}

func TestSceneRender_FillRule(t *testing.T) {
	// EvenOdd fill rule on a path with two concentric circles.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <path fill-rule="evenodd" d="M8 2 A6 6 0 0 1 8 14 A6 6 0 0 1 8 2 Z M8 5 A3 3 0 0 0 8 11 A3 3 0 0 0 8 5 Z" fill="#333333"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 16, 16)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty")
	}
	// Check that TagFill draw data contains EvenOdd fill style.
	// The fill style is the second uint32 in fill draw data.
	tags := enc.Tags()
	drawData := enc.DrawData()
	drawIdx := 0
	foundEvenOdd := false
	for _, tag := range tags {
		switch tag {
		case scene.TagFill:
			if drawIdx+1 < len(drawData) {
				fillStyle := drawData[drawIdx+1]
				if scene.FillStyle(fillStyle) == scene.FillEvenOdd {
					foundEvenOdd = true
				}
			}
			drawIdx += 2
		case scene.TagStroke:
			drawIdx += 5
		case scene.TagPushLayer:
			drawIdx += 2
		case scene.TagSetAntiAlias:
			drawIdx++
		case scene.TagImage:
			drawIdx++
		case scene.TagFillRoundRect:
			drawIdx += 2
		}
	}
	if !foundEvenOdd {
		t.Error("expected EvenOdd fill style in encoding, not found")
	}
}

func TestSceneRender_StrokeProperties(t *testing.T) {
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <line x1="2" y1="8" x2="14" y2="8" stroke="#000000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 16, 16)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty")
	}
	// A line is stroke-only, so we expect TagStroke in the encoding.
	tags := enc.Tags()
	foundStroke := false
	for _, tag := range tags {
		if tag == scene.TagStroke {
			foundStroke = true
			break
		}
	}
	if !foundStroke {
		t.Error("expected TagStroke in encoding for stroked line")
	}
}

func TestSceneRender_GroupOpacity(t *testing.T) {
	// Group with opacity < 1 should use PushLayer.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <g opacity="0.5">
    <rect x="2" y="2" width="12" height="12" fill="red"/>
  </g>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 16, 16)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty")
	}
	tags := enc.Tags()
	foundPushLayer := false
	foundPopLayer := false
	for _, tag := range tags {
		if tag == scene.TagPushLayer {
			foundPushLayer = true
		}
		if tag == scene.TagPopLayer {
			foundPopLayer = true
		}
	}
	if !foundPushLayer || !foundPopLayer {
		t.Errorf("expected PushLayer/PopLayer for group opacity, push=%v pop=%v",
			foundPushLayer, foundPopLayer)
	}
}

func TestSceneRender_FillNone(t *testing.T) {
	// fill="none" should not produce a fill command.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <circle cx="8" cy="8" r="6" fill="none" stroke="#000000"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 16, 16)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty (stroke-only circle should produce content)")
	}
	// Should have stroke but no fill.
	tags := enc.Tags()
	foundFill := false
	foundStroke := false
	for _, tag := range tags {
		if tag == scene.TagFill || tag == scene.TagFillRoundRect {
			foundFill = true
		}
		if tag == scene.TagStroke {
			foundStroke = true
		}
	}
	if foundFill {
		t.Error("expected NO fill for fill=none element, but found TagFill")
	}
	if !foundStroke {
		t.Error("expected TagStroke for stroked circle")
	}
}

func TestSceneRender_AllJetBrainsIcons(t *testing.T) {
	// Render all 7 JetBrains icons from svg_test.go — should not panic.
	icons := []struct {
		name string
		svg  string
	}{
		{"close", closeIconSVG},
		{"search", searchIconSVG},
		{"refresh", refreshIconSVG},
		{"back", backIconSVG},
		{"execute", executeIconSVG},
		{"commit", commitIconSVG},
		{"problems", problemsIconSVG},
	}
	for _, icon := range icons {
		t.Run(icon.name, func(t *testing.T) {
			doc, err := Parse([]byte(icon.svg))
			if err != nil {
				t.Fatalf("Parse %s: %v", icon.name, err)
			}
			s := scene.NewScene()
			doc.RenderToScene(s, 0, 0, 32, 32)
			enc := s.Encoding()
			if enc.IsEmpty() {
				t.Errorf("%s: scene encoding is empty", icon.name)
			}
			if enc.ShapeCount() == 0 {
				t.Errorf("%s: ShapeCount is 0", icon.name)
			}
		})
	}
}

func TestSceneRender_EmptyViewBox(t *testing.T) {
	// Document with zero-size viewbox should not panic.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 0">
  <rect x="0" y="0" width="10" height="10" fill="red"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 16, 16)
	// Should be empty — zero viewbox means nothing to render.
	enc := s.Encoding()
	if !enc.IsEmpty() {
		t.Error("expected empty scene for zero-size viewbox")
	}
}

func TestSceneRender_PositionOffset(t *testing.T) {
	// RenderToScene with non-zero x,y should apply translation.
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <rect x="0" y="0" width="16" height="16" fill="red"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 10, 20, 16, 16)
	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty")
	}
	// The transform stack should include a translation for the offset.
	if len(enc.Transforms()) == 0 {
		t.Error("expected transforms for offset rendering")
	}
}

// TestParseSVGTransformAffine tests the scene.Affine-based SVG transform parser.
func TestParseSVGTransformAffine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantC    float32 // translation X component
		wantF    float32 // translation Y component
		wantIsID bool    // expect identity
		wantErr  bool
	}{
		{
			name:    "translate",
			input:   "translate(10 20)",
			wantC:   10,
			wantF:   20,
			wantErr: false,
		},
		{
			name:    "translate_single",
			input:   "translate(5)",
			wantC:   5,
			wantF:   0,
			wantErr: false,
		},
		{
			name:     "scale_uniform",
			input:    "scale(2)",
			wantIsID: false,
			wantErr:  false,
		},
		{
			name:     "rotate",
			input:    "rotate(90)",
			wantIsID: false,
			wantErr:  false,
		},
		{
			name:    "matrix",
			input:   "matrix(1 0 0 1 10 20)",
			wantC:   10,
			wantF:   20,
			wantErr: false,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: false,
		},
		{
			name:    "bad_func",
			input:   "invalid(1 2)",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := parseSVGTransformAffine(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSVGTransformAffine(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.input == "" {
				if !a.IsIdentity() {
					t.Errorf("empty transform should be identity, got %+v", a)
				}
				return
			}
			if tt.wantC != 0 && a.C != tt.wantC {
				t.Errorf("C = %v, want %v", a.C, tt.wantC)
			}
			if tt.wantF != 0 && a.F != tt.wantF {
				t.Errorf("F = %v, want %v", a.F, tt.wantF)
			}
		})
	}
}

func TestSceneRender_RoundedRect(t *testing.T) {
	svgData := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect x="4" y="4" width="24" height="24" rx="6" fill="#336699"/>
</svg>`
	doc, err := Parse([]byte(svgData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := scene.NewScene()
	doc.RenderToScene(s, 0, 0, 32, 32)

	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Fatal("scene encoding is empty")
	}
	if enc.ShapeCount() == 0 {
		t.Error("ShapeCount is 0 for rounded rect")
	}
}
