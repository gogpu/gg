package scene

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func TestDefaultTextRendererConfig(t *testing.T) {
	config := DefaultTextRendererConfig()

	if config.FlipY {
		t.Errorf("FlipY should be false by default (sfnt returns Y-down)")
	}
	if !config.SubpixelPositioning {
		t.Errorf("SubpixelPositioning should be true by default")
	}
	if config.HintingEnabled {
		t.Errorf("HintingEnabled should be false by default")
	}
}

func TestNewTextRenderer(t *testing.T) {
	r := NewTextRenderer()
	if r == nil {
		t.Fatal("NewTextRenderer should not return nil")
	}
	if r.extractor == nil {
		t.Errorf("extractor should not be nil")
	}
	if r.pathPool == nil {
		t.Errorf("pathPool should not be nil")
	}
}

func TestNewTextRendererWithConfig(t *testing.T) {
	config := TextRendererConfig{
		FlipY:               true,
		SubpixelPositioning: false,
		HintingEnabled:      true,
	}
	r := NewTextRendererWithConfig(config)

	got := r.Config()
	if got.FlipY != config.FlipY {
		t.Errorf("FlipY = %v, want %v", got.FlipY, config.FlipY)
	}
	if got.SubpixelPositioning != config.SubpixelPositioning {
		t.Errorf("SubpixelPositioning = %v, want %v", got.SubpixelPositioning, config.SubpixelPositioning)
	}
	if got.HintingEnabled != config.HintingEnabled {
		t.Errorf("HintingEnabled = %v, want %v", got.HintingEnabled, config.HintingEnabled)
	}
}

func TestTextRenderer_SetConfig(t *testing.T) {
	r := NewTextRenderer()

	newConfig := TextRendererConfig{
		FlipY:               true,
		SubpixelPositioning: false,
		HintingEnabled:      true,
	}
	r.SetConfig(newConfig)

	got := r.Config()
	if !got.FlipY {
		t.Errorf("FlipY should be true after SetConfig")
	}
}

func TestTextRendererPool(t *testing.T) {
	pool := NewTextRendererPool()
	if pool == nil {
		t.Errorf("NewTextRendererPool should not return nil")
	}

	// Get a renderer
	r1 := pool.Get()
	if r1 == nil {
		t.Errorf("Get should not return nil")
	}

	// Put it back
	pool.Put(r1)

	// Get again - should reuse
	r2 := pool.Get()
	if r2 == nil {
		t.Errorf("Get should not return nil after Put")
	}

	// Put nil should not panic
	pool.Put(nil)
}

func TestTextBounds_Empty(t *testing.T) {
	bounds := TextBounds(nil)
	if !bounds.IsEmpty() {
		t.Errorf("TextBounds of nil should be empty")
	}

	bounds = TextBounds([]*RenderedGlyph{})
	if !bounds.IsEmpty() {
		t.Errorf("TextBounds of empty slice should be empty")
	}
}

func TestTextAdvance_Empty(t *testing.T) {
	advance := TextAdvance(nil)
	if advance != 0 {
		t.Errorf("TextAdvance of nil should be 0, got %v", advance)
	}

	advance = TextAdvance([]*RenderedGlyph{})
	if advance != 0 {
		t.Errorf("TextAdvance of empty slice should be 0, got %v", advance)
	}
}

func TestTextAdvance_WithGlyphs(t *testing.T) {
	glyphs := []*RenderedGlyph{
		{X: 0, Advance: 10},
		{X: 10, Advance: 15},
		{X: 25, Advance: 20},
	}

	advance := TextAdvance(glyphs)
	expected := float32(45) // 25 + 20
	if advance != expected {
		t.Errorf("TextAdvance = %v, want %v", advance, expected)
	}
}

func TestRenderedGlyph_Fields(t *testing.T) {
	rg := &RenderedGlyph{
		Path:    NewPath().MoveTo(0, 0).LineTo(10, 10),
		Bounds:  Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		Advance: 15,
		X:       5,
		Y:       10,
		GID:     42,
		Type:    text.GlyphTypeOutline,
		Cluster: 3,
	}

	if rg.Advance != 15 {
		t.Errorf("Advance = %v, want 15", rg.Advance)
	}
	if rg.GID != 42 {
		t.Errorf("GID = %v, want 42", rg.GID)
	}
	if rg.Type != text.GlyphTypeOutline {
		t.Errorf("Type = %v, want Outline", rg.Type)
	}
	if rg.Cluster != 3 {
		t.Errorf("Cluster = %v, want 3", rg.Cluster)
	}
}

func TestAppendPath(t *testing.T) {
	t.Run("nil src", func(t *testing.T) {
		dst := NewPath().MoveTo(0, 0)
		appendPath(dst, nil)
		if dst.VerbCount() != 1 {
			t.Errorf("Appending nil should not change dst")
		}
	})

	t.Run("empty src", func(t *testing.T) {
		dst := NewPath().MoveTo(0, 0)
		appendPath(dst, NewPath())
		if dst.VerbCount() != 1 {
			t.Errorf("Appending empty should not change dst")
		}
	})

	t.Run("append paths", func(t *testing.T) {
		dst := NewPath().MoveTo(0, 0).LineTo(10, 10)
		src := NewPath().MoveTo(20, 20).LineTo(30, 30).Close()

		appendPath(dst, src)

		// dst should now have: MoveTo, LineTo, MoveTo, LineTo, Close
		if dst.VerbCount() != 5 {
			t.Errorf("VerbCount = %v, want 5", dst.VerbCount())
		}
	})
}

func TestTextRenderer_RenderGlyphs_EmptySlice(t *testing.T) {
	r := NewTextRenderer()

	rendered, err := r.RenderGlyphs(nil, nil)
	if err != nil {
		t.Errorf("RenderGlyphs nil should not error, got %v", err)
	}
	if rendered != nil {
		t.Errorf("RenderGlyphs nil should return nil")
	}

	rendered, err = r.RenderGlyphs([]text.ShapedGlyph{}, nil)
	if err != nil {
		t.Errorf("RenderGlyphs empty should not error, got %v", err)
	}
	if rendered != nil {
		t.Errorf("RenderGlyphs empty should return nil")
	}
}

func TestTextRenderer_RenderRun_Nil(t *testing.T) {
	r := NewTextRenderer()

	rendered, err := r.RenderRun(nil)
	if err != nil {
		t.Errorf("RenderRun nil should not error, got %v", err)
	}
	if rendered != nil {
		t.Errorf("RenderRun nil should return nil")
	}

	rendered, err = r.RenderRun(&text.ShapedRun{Glyphs: nil})
	if err != nil {
		t.Errorf("RenderRun empty should not error, got %v", err)
	}
	if rendered != nil {
		t.Errorf("RenderRun empty should return nil")
	}
}

func TestTextRenderer_RenderText_Empty(t *testing.T) {
	r := NewTextRenderer()

	rendered, err := r.RenderText("", nil)
	if err != nil {
		t.Errorf("RenderText empty should not error, got %v", err)
	}
	if rendered != nil {
		t.Errorf("RenderText empty should return nil")
	}
}

func TestTextRenderer_RenderTextToScene_Empty(t *testing.T) {
	r := NewTextRenderer()
	s := NewScene()

	err := r.RenderTextToScene(s, "", nil, 0, 0, SolidBrush(gg.RGBA{R: 255, G: 0, B: 0, A: 255}))
	if err != nil {
		t.Errorf("RenderTextToScene empty should not error, got %v", err)
	}

	if !s.IsEmpty() {
		t.Errorf("Scene should be empty after rendering empty text")
	}
}

func TestScene_DrawText_Empty(t *testing.T) {
	s := NewScene()

	err := s.DrawText("", nil, 0, 0, SolidBrush(gg.RGBA{R: 255, G: 0, B: 0, A: 255}))
	if err != nil {
		t.Errorf("DrawText empty should not error, got %v", err)
	}

	if !s.IsEmpty() {
		t.Errorf("Scene should be empty after drawing empty text")
	}
}

func TestTextRenderer_ToCompositePath_Empty(t *testing.T) {
	r := NewTextRenderer()

	composite := r.ToCompositePath(nil)
	if composite != nil {
		t.Errorf("ToCompositePath nil should return nil")
	}

	// Empty slice also returns nil (consistent behavior)
	composite = r.ToCompositePath([]*RenderedGlyph{})
	if composite != nil {
		t.Errorf("ToCompositePath empty slice should return nil")
	}
}

func TestTextRenderer_ToCompositePath_WithGlyphs(t *testing.T) {
	r := NewTextRenderer()

	glyphs := []*RenderedGlyph{
		{Path: NewPath().MoveTo(0, 0).LineTo(10, 10)},
		{Path: nil}, // nil path should be skipped
		{Path: NewPath().MoveTo(20, 20).LineTo(30, 30)},
	}

	composite := r.ToCompositePath(glyphs)
	if composite == nil {
		t.Errorf("ToCompositePath should not return nil")
	}
	if composite.VerbCount() != 4 { // MoveTo, LineTo, MoveTo, LineTo
		t.Errorf("VerbCount = %v, want 4", composite.VerbCount())
	}
}

func TestNewTextShape_Empty(t *testing.T) {
	ts, err := NewTextShape("", nil, 0, 0)
	if err != nil {
		t.Errorf("NewTextShape empty should not error, got %v", err)
	}
	if ts == nil {
		t.Errorf("NewTextShape should not return nil")
	}
	if ts.ToPath() == nil {
		t.Errorf("ToPath should not be nil even for empty text")
	}
	if !ts.ToPath().IsEmpty() {
		t.Errorf("Path should be empty for empty text")
	}
}

func TestTextShape_Bounds_Empty(t *testing.T) {
	ts := &TextShape{path: nil}
	bounds := ts.Bounds()
	if !bounds.IsEmpty() {
		t.Errorf("Bounds of nil path should be empty")
	}

	ts = &TextShape{path: NewPath()}
	bounds = ts.Bounds()
	if !bounds.IsEmpty() {
		t.Errorf("Bounds of empty path should be empty")
	}
}

// BenchmarkTextRenderer_New benchmarks renderer creation.
func BenchmarkTextRenderer_New(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewTextRenderer()
	}
}

// BenchmarkTextRendererPool_GetPut benchmarks pool operations.
func BenchmarkTextRendererPool_GetPut(b *testing.B) {
	pool := NewTextRendererPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := pool.Get()
		pool.Put(r)
	}
}

// BenchmarkAppendPath benchmarks path appending.
func BenchmarkAppendPath(b *testing.B) {
	src := NewPath()
	for i := 0; i < 20; i++ {
		src.MoveTo(float32(i), float32(i))
		src.LineTo(float32(i+1), float32(i+1))
		src.QuadTo(float32(i+2), float32(i+2), float32(i+3), float32(i+3))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := NewPath()
		appendPath(dst, src)
	}
}
