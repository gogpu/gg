package text

import (
	"image/color"
	"testing"
)

// mockParsedFont implements ParsedFont for testing.
type mockParsedFont struct {
	name       string
	unitsPerEm int
	numGlyphs  int
}

func (m *mockParsedFont) Name() string                                { return m.name }
func (m *mockParsedFont) FullName() string                            { return m.name + " Regular" }
func (m *mockParsedFont) UnitsPerEm() int                             { return m.unitsPerEm }
func (m *mockParsedFont) NumGlyphs() int                              { return m.numGlyphs }
func (m *mockParsedFont) GlyphIndex(_ rune) uint16                    { return 1 }
func (m *mockParsedFont) GlyphAdvance(_ uint16, _ float64) float64    { return 10.0 }
func (m *mockParsedFont) GlyphBounds(_ uint16, _ float64) Rect        { return Rect{0, 0, 10, 10} }
func (m *mockParsedFont) Metrics(_ float64) FontMetrics               { return FontMetrics{} }

func TestRenderParams_Defaults(t *testing.T) {
	params := DefaultRenderParams()

	if params.Color.A != 255 {
		t.Errorf("expected alpha 255, got %d", params.Color.A)
	}
	if params.Opacity != 1.0 {
		t.Errorf("expected opacity 1.0, got %f", params.Opacity)
	}
	if params.Transform != nil {
		t.Errorf("expected nil transform, got %v", params.Transform)
	}
}

func TestRenderParams_WithMethods(t *testing.T) {
	params := DefaultRenderParams()

	// Test WithColor
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	params2 := params.WithColor(red)
	if params2.Color != red {
		t.Errorf("expected red color, got %v", params2.Color)
	}
	// Original should be unchanged
	if params.Color.R != 0 {
		t.Errorf("original params modified")
	}

	// Test WithOpacity
	params3 := params.WithOpacity(0.5)
	if params3.Opacity != 0.5 {
		t.Errorf("expected opacity 0.5, got %f", params3.Opacity)
	}

	// Test opacity clamping
	params4 := params.WithOpacity(-0.5)
	if params4.Opacity != 0 {
		t.Errorf("expected opacity 0 (clamped), got %f", params4.Opacity)
	}
	params5 := params.WithOpacity(1.5)
	if params5.Opacity != 1.0 {
		t.Errorf("expected opacity 1.0 (clamped), got %f", params5.Opacity)
	}

	// Test WithTransform
	transform := TranslateTransform(10, 20)
	params6 := params.WithTransform(transform)
	if params6.Transform != transform {
		t.Errorf("expected transform to be set")
	}
}

func TestNewGlyphRenderer(t *testing.T) {
	r := NewGlyphRenderer()
	if r == nil {
		t.Fatal("NewGlyphRenderer returned nil")
	}
	if r.cache == nil {
		t.Error("cache is nil")
	}
	if r.extractor == nil {
		t.Error("extractor is nil")
	}
}

func TestNewGlyphRendererWithCache(t *testing.T) {
	// With nil cache, should use global
	r1 := NewGlyphRendererWithCache(nil)
	if r1.cache != GetGlobalGlyphCache() {
		t.Error("expected global cache when nil passed")
	}

	// With custom cache
	customCache := NewGlyphCache()
	r2 := NewGlyphRendererWithCache(customCache)
	if r2.cache != customCache {
		t.Error("expected custom cache")
	}
}

func TestGlyphRenderer_CacheAccess(t *testing.T) {
	r := NewGlyphRenderer()

	// Test Cache getter
	cache := r.Cache()
	if cache == nil {
		t.Error("Cache() returned nil")
	}

	// Test SetCache
	newCache := NewGlyphCache()
	r.SetCache(newCache)
	if r.Cache() != newCache {
		t.Error("SetCache did not set cache")
	}

	// Test SetCache with nil (should use global)
	r.SetCache(nil)
	if r.Cache() != GetGlobalGlyphCache() {
		t.Error("SetCache(nil) should use global cache")
	}
}

func TestGlyphRenderer_RenderGlyph_NilInputs(t *testing.T) {
	r := NewGlyphRenderer()
	params := DefaultRenderParams()
	font := &mockParsedFont{name: "Test", unitsPerEm: 2048, numGlyphs: 256}

	// Nil glyph
	result := r.RenderGlyph(nil, font, 16, params)
	if result != nil {
		t.Errorf("expected nil for nil glyph, got %v", result)
	}

	// Nil font
	glyph := &ShapedGlyph{GID: 1, X: 0, Y: 0}
	result = r.RenderGlyph(glyph, nil, 16, params)
	if result != nil {
		t.Errorf("expected nil for nil font, got %v", result)
	}
}

func TestGlyphRenderer_RenderGlyphs_NilInputs(t *testing.T) {
	r := NewGlyphRenderer()
	params := DefaultRenderParams()
	font := &mockParsedFont{name: "Test", unitsPerEm: 2048, numGlyphs: 256}

	// Empty glyphs
	result := r.RenderGlyphs(nil, font, 16, params)
	if result != nil {
		t.Errorf("expected nil for nil glyphs, got %v", result)
	}

	result = r.RenderGlyphs([]ShapedGlyph{}, font, 16, params)
	if result != nil {
		t.Errorf("expected nil for empty glyphs, got %v", result)
	}

	// Nil font
	glyphs := []ShapedGlyph{{GID: 1, X: 0, Y: 0}}
	result = r.RenderGlyphs(glyphs, nil, 16, params)
	if result != nil {
		t.Errorf("expected nil for nil font, got %v", result)
	}
}

func TestGlyphRenderer_RenderRun_NilInputs(t *testing.T) {
	r := NewGlyphRenderer()
	params := DefaultRenderParams()

	// Nil run
	result := r.RenderRun(nil, params)
	if result != nil {
		t.Errorf("expected nil for nil run, got %v", result)
	}

	// Empty run
	result = r.RenderRun(&ShapedRun{}, params)
	if result != nil {
		t.Errorf("expected nil for empty run, got %v", result)
	}

	// Run with glyphs but nil face
	result = r.RenderRun(&ShapedRun{Glyphs: []ShapedGlyph{{GID: 1}}}, params)
	if result != nil {
		t.Errorf("expected nil for run with nil face, got %v", result)
	}
}

func TestGlyphRenderer_RenderLayout_NilInputs(t *testing.T) {
	r := NewGlyphRenderer()
	params := DefaultRenderParams()

	// Nil layout
	result := r.RenderLayout(nil, params)
	if result != nil {
		t.Errorf("expected nil for nil layout, got %v", result)
	}

	// Empty layout
	result = r.RenderLayout(&Layout{}, params)
	if result != nil {
		t.Errorf("expected nil for empty layout, got %v", result)
	}
}

func TestTransformOutline(t *testing.T) {
	r := NewGlyphRenderer()

	tests := []struct {
		name    string
		outline *GlyphOutline
		glyph   *ShapedGlyph
		params  RenderParams
		wantNil bool
	}{
		{
			name:    "nil outline",
			outline: nil,
			glyph:   &ShapedGlyph{X: 10, Y: 20},
			params:  DefaultRenderParams(),
			wantNil: true,
		},
		{
			name: "basic outline",
			outline: &GlyphOutline{
				Segments: []OutlineSegment{
					{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
					{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
				},
			},
			glyph:   &ShapedGlyph{X: 100, Y: 50},
			params:  DefaultRenderParams(),
			wantNil: false,
		},
		{
			name: "with user transform",
			outline: &GlyphOutline{
				Segments: []OutlineSegment{
					{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
				},
			},
			glyph:   &ShapedGlyph{X: 0, Y: 0},
			params:  RenderParams{Transform: ScaleTransform(2, 2)},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.transformOutline(tt.outline, tt.glyph, tt.params)
			if tt.wantNil && result != nil {
				t.Error("expected nil result")
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func TestComputeFontID(t *testing.T) {
	font1 := &mockParsedFont{name: "Arial", unitsPerEm: 2048, numGlyphs: 256}
	font2 := &mockParsedFont{name: "Arial", unitsPerEm: 2048, numGlyphs: 256}
	font3 := &mockParsedFont{name: "Times", unitsPerEm: 2048, numGlyphs: 256}

	id1 := computeFontID(font1)
	id2 := computeFontID(font2)
	id3 := computeFontID(font3)

	// Same fonts should have same ID
	if id1 != id2 {
		t.Errorf("identical fonts should have same ID: %d != %d", id1, id2)
	}

	// Different fonts should have different IDs
	if id1 == id3 {
		t.Errorf("different fonts should have different IDs: %d == %d", id1, id3)
	}
}

func TestSizeToInt16(t *testing.T) {
	tests := []struct {
		name   string
		input  float64
		expect int16
	}{
		{"normal size", 16.0, 16},
		{"fractional size", 16.5, 16},
		{"zero", 0, 0},
		{"negative clamped", -5, 0},
		{"large clamped", 50000, 32767},
		{"max", 32767, 32767},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sizeToInt16(tt.input)
			if result != tt.expect {
				t.Errorf("sizeToInt16(%f) = %d, expected %d", tt.input, result, tt.expect)
			}
		})
	}
}

func TestClampOpacity(t *testing.T) {
	tests := []struct {
		input  float64
		expect float64
	}{
		{0.5, 0.5},
		{0, 0},
		{1, 1},
		{-0.5, 0},
		{1.5, 1},
	}

	for _, tt := range tests {
		result := clampOpacity(tt.input)
		if result != tt.expect {
			t.Errorf("clampOpacity(%f) = %f, expected %f", tt.input, result, tt.expect)
		}
	}
}

func TestRotateTransform(t *testing.T) {
	// Test rotation by 0
	r0 := RotateTransform(0)
	if r0.A != 1 || r0.D != 1 {
		t.Errorf("rotation by 0 should be identity-like, got A=%f D=%f", r0.A, r0.D)
	}

	// Test rotation by pi/2 (90 degrees)
	const halfPi = 1.5707963267948966
	r90 := RotateTransform(halfPi)
	// cos(pi/2) should be ~0, sin(pi/2) should be ~1
	if r90.C < 0.99 || r90.C > 1.01 {
		t.Errorf("rotation by pi/2: C should be ~1, got %f", r90.C)
	}
}

func TestScaleTransformXY(t *testing.T) {
	s := ScaleTransformXY(2, 3)
	if s.A != 2 {
		t.Errorf("expected A=2, got %f", s.A)
	}
	if s.D != 3 {
		t.Errorf("expected D=3, got %f", s.D)
	}
}

func TestSineApprox(t *testing.T) {
	tests := []struct {
		x      float64
		expect float64
		tol    float64
	}{
		{0, 0, 0.001},
		{0.5, 0.479, 0.01},
		{1.0, 0.841, 0.01},
		{-0.5, -0.479, 0.01},
	}

	for _, tt := range tests {
		result := sineApprox(tt.x)
		diff := result - tt.expect
		if diff < 0 {
			diff = -diff
		}
		if diff > tt.tol {
			t.Errorf("sineApprox(%f) = %f, expected ~%f", tt.x, result, tt.expect)
		}
	}
}

func TestCosineApprox(t *testing.T) {
	tests := []struct {
		x      float64
		expect float64
		tol    float64
	}{
		{0, 1, 0.001},
		{0.5, 0.877, 0.01},
		{1.0, 0.540, 0.01},
		{-0.5, 0.877, 0.01},
	}

	for _, tt := range tests {
		result := cosineApprox(tt.x)
		diff := result - tt.expect
		if diff < 0 {
			diff = -diff
		}
		if diff > tt.tol {
			t.Errorf("cosineApprox(%f) = %f, expected ~%f", tt.x, result, tt.expect)
		}
	}
}

func TestNewTextRenderer(t *testing.T) {
	tr := NewTextRenderer()
	if tr == nil {
		t.Fatal("NewTextRenderer returned nil")
	}
	if tr.glyphRenderer == nil {
		t.Error("glyphRenderer is nil")
	}
	if tr.defaultSize != 16.0 {
		t.Errorf("default size should be 16.0, got %f", tr.defaultSize)
	}
}

func TestTextRenderer_SetDefaults(t *testing.T) {
	tr := NewTextRenderer()

	// SetDefaultSize
	tr.SetDefaultSize(24)
	if tr.defaultSize != 24 {
		t.Errorf("expected size 24, got %f", tr.defaultSize)
	}
	tr.SetDefaultSize(0) // Should not change
	if tr.defaultSize != 24 {
		t.Errorf("size should remain 24, got %f", tr.defaultSize)
	}
	tr.SetDefaultSize(-5) // Should not change
	if tr.defaultSize != 24 {
		t.Errorf("size should remain 24, got %f", tr.defaultSize)
	}

	// SetDefaultColor
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	tr.SetDefaultColor(red)
	if tr.defaultColor != red {
		t.Errorf("expected red, got %v", tr.defaultColor)
	}
}

func TestTextRenderer_GlyphRenderer(t *testing.T) {
	tr := NewTextRenderer()
	gr := tr.GlyphRenderer()
	if gr == nil {
		t.Error("GlyphRenderer() returned nil")
	}
	if gr != tr.glyphRenderer {
		t.Error("GlyphRenderer() returned different instance")
	}
}

func TestGetGlobalTextRenderer(t *testing.T) {
	gr := GetGlobalTextRenderer()
	if gr == nil {
		t.Fatal("GetGlobalTextRenderer returned nil")
	}

	// Should return same instance
	gr2 := GetGlobalTextRenderer()
	if gr != gr2 {
		t.Error("GetGlobalTextRenderer should return same instance")
	}
}

func TestTextRenderer_ShapeAndRender_NoFace(t *testing.T) {
	tr := NewTextRenderer()

	// No face set, should return error
	_, err := tr.ShapeAndRender("hello")
	if err == nil {
		t.Error("expected error when no face is set")
	}
}

func TestTextRenderer_ShapeAndRenderAt_NoFace(t *testing.T) {
	tr := NewTextRenderer()

	// No face set, should return error
	_, err := tr.ShapeAndRenderAt("hello", 10, 20)
	if err == nil {
		t.Error("expected error when no face is set")
	}
}
