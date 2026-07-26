package gg

import (
	"testing"
)

// TestNewPaint tests the NewPaint constructor.
func TestNewPaint(t *testing.T) {
	p := NewPaint()

	if p.LineWidth != 1.0 {
		t.Errorf("LineWidth = %v, want 1.0", p.LineWidth)
	}
	if p.LineCap != LineCapButt {
		t.Errorf("LineCap = %v, want LineCapButt", p.LineCap)
	}
	if p.LineJoin != LineJoinMiter {
		t.Errorf("LineJoin = %v, want LineJoinMiter", p.LineJoin)
	}
	if p.MiterLimit != 10.0 {
		t.Errorf("MiterLimit = %v, want 10.0", p.MiterLimit)
	}
	if p.FillRule != FillRuleNonZero {
		t.Errorf("FillRule = %v, want FillRuleNonZero", p.FillRule)
	}
	if !p.Antialias {
		t.Error("Antialias = false, want true")
	}

	// Default paint is solid black (stored inline, no Brush/Pattern allocation).
	if !p.IsSolid() {
		t.Error("IsSolid = false, want true")
	}
	c, ok := p.SolidColor()
	if !ok || c != Black {
		t.Errorf("SolidColor = %v, %v, want Black, true", c, ok)
	}

	// GetBrush returns correct value even though Brush field is nil.
	brush := p.GetBrush()
	if brush.ColorAt(0, 0) != Black {
		t.Errorf("GetBrush color = %v, want Black", brush.ColorAt(0, 0))
	}
}

// TestPaintClone tests the Clone method.
func TestPaintClone(t *testing.T) {
	p := NewPaint()
	p.LineWidth = 5.0
	p.LineCap = LineCapRound
	p.SetBrush(Solid(Red))

	clone := p.Clone()

	if clone.LineWidth != p.LineWidth {
		t.Errorf("clone.LineWidth = %v, want %v", clone.LineWidth, p.LineWidth)
	}
	if clone.LineCap != p.LineCap {
		t.Errorf("clone.LineCap = %v, want %v", clone.LineCap, p.LineCap)
	}

	// Solid colors are stored inline — verify via SolidColor accessor.
	c, ok := clone.SolidColor()
	if !ok || c != Red {
		t.Errorf("clone SolidColor = %v, %v, want Red, true", c, ok)
	}

	// Verify it's a separate object
	clone.LineWidth = 10.0
	if p.LineWidth == clone.LineWidth {
		t.Error("Clone is not independent")
	}
}

// TestPaintSetBrush tests the SetBrush method.
func TestPaintSetBrush(t *testing.T) {
	t.Run("solid brush stores inline", func(t *testing.T) {
		p := NewPaint()
		p.SetBrush(Solid(Blue))

		// Solid brushes are stored inline — brush and pattern are nil.
		if !p.IsSolid() {
			t.Error("IsSolid = false after SetBrush(Solid)")
		}
		c, ok := p.SolidColor()
		if !ok || c != Blue {
			t.Errorf("SolidColor = %v, %v, want Blue, true", c, ok)
		}
		if p.fill.brush != nil {
			t.Error("fill.brush should be nil for solid color")
		}
		if p.fill.pattern != nil {
			t.Error("fill.pattern should be nil for solid color")
		}
	})

	t.Run("non-solid brush sets fields", func(t *testing.T) {
		p := NewPaint()
		custom := CustomBrush{Func: func(x, y float64) RGBA { return Red }, Name: "test"}
		p.SetBrush(custom)

		if p.IsSolid() {
			t.Error("IsSolid = true after SetBrush(CustomBrush)")
		}
		if p.fill.brush == nil {
			t.Error("fill.brush = nil after SetBrush(CustomBrush)")
		}
		if p.fill.pattern == nil {
			t.Error("fill.pattern = nil after SetBrush(CustomBrush)")
		}
	})
}

// TestPaintGetBrush tests the GetBrush method.
func TestPaintGetBrush(t *testing.T) {
	t.Run("with solid brush (inline)", func(t *testing.T) {
		p := NewPaint()
		p.SetBrush(Solid(Green))
		brush := p.GetBrush()
		if sb, ok := brush.(SolidBrush); !ok || sb.Color != Green {
			t.Error("GetBrush did not return correct solid brush")
		}
	})

	t.Run("with brush field set directly", func(t *testing.T) {
		p := &Paint{}
		p.fill.brush = Solid(Green) // Direct field write bypasses SetBrush
		p.fill.isSolid = false      // Explicitly not inline
		brush := p.GetBrush()
		if sb, ok := brush.(SolidBrush); !ok || sb.Color != Green {
			t.Error("GetBrush did not return set brush")
		}
	})

	t.Run("with only pattern set", func(t *testing.T) {
		p := &Paint{}
		p.fill.pattern = NewSolidPattern(Yellow)
		brush := p.GetBrush()
		if brush == nil {
			t.Error("GetBrush returned nil for Pattern-only paint")
		}
		c := brush.ColorAt(0, 0)
		if c != Yellow {
			t.Errorf("GetBrush returned wrong color: %v, want Yellow", c)
		}
	})

	t.Run("with nothing set", func(t *testing.T) {
		p := &Paint{}
		brush := p.GetBrush()
		if brush == nil {
			t.Error("GetBrush returned nil for empty paint")
		}
		// Should return default black
		c := brush.ColorAt(0, 0)
		if c != Black {
			t.Errorf("GetBrush returned wrong default color: %v, want Black", c)
		}
	})
}

// TestPaintColorAt tests the ColorAt method.
func TestPaintColorAt(t *testing.T) {
	t.Run("with solid brush via SetBrush", func(t *testing.T) {
		p := NewPaint()
		p.SetBrush(Solid(Red))
		c := p.ColorAt(0, 0)
		if c != Red {
			t.Errorf("ColorAt = %v, want Red", c)
		}
	})

	t.Run("with brush field directly", func(t *testing.T) {
		p := &Paint{}
		p.fill.brush = Solid(Red)
		c := p.ColorAt(0, 0)
		if c != Red {
			t.Errorf("ColorAt = %v, want Red", c)
		}
	})

	t.Run("with only pattern set", func(t *testing.T) {
		p := &Paint{}
		p.fill.pattern = NewSolidPattern(Blue)
		c := p.ColorAt(0, 0)
		if c != Blue {
			t.Errorf("ColorAt = %v, want Blue", c)
		}
	})

	t.Run("with nothing set", func(t *testing.T) {
		p := &Paint{}
		c := p.ColorAt(0, 0)
		if c != Black {
			t.Errorf("ColorAt = %v, want Black (default)", c)
		}
	})

	t.Run("brush takes precedence over pattern", func(t *testing.T) {
		p := &Paint{}
		p.fill.pattern = NewSolidPattern(Blue)
		p.fill.brush = Solid(Red)
		c := p.ColorAt(0, 0)
		if c != Red {
			t.Errorf("ColorAt = %v, want Red (brush should take precedence)", c)
		}
	})

	t.Run("isSolid takes precedence over both", func(t *testing.T) {
		p := &Paint{}
		p.fill.solidColor = Green
		p.fill.isSolid = true
		p.fill.pattern = NewSolidPattern(Blue)
		p.fill.brush = Solid(Red)
		c := p.ColorAt(0, 0)
		if c != Green {
			t.Errorf("ColorAt = %v, want Green (isSolid takes precedence)", c)
		}
	})
}

// TestContextSetFillBrush tests the SetFillBrush method.
func TestContextSetFillBrush(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetFillBrush(Solid(Magenta))

	brush := dc.FillBrush()
	c := brush.ColorAt(0, 0)
	if c != Magenta {
		t.Errorf("FillBrush color = %v, want Magenta", c)
	}
}

// TestContextSetStrokeBrush tests the SetStrokeBrush method.
func TestContextSetStrokeBrush(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetStrokeBrush(Solid(Cyan))

	brush := dc.StrokeBrush()
	c := brush.ColorAt(0, 0)
	if c != Cyan {
		t.Errorf("StrokeBrush color = %v, want Cyan", c)
	}
}

// TestContextFillBrush tests the FillBrush getter.
func TestContextFillBrush(t *testing.T) {
	dc := NewContext(100, 100)
	// Default should be black
	brush := dc.FillBrush()
	c := brush.ColorAt(0, 0)
	if c != Black {
		t.Errorf("default FillBrush color = %v, want Black", c)
	}
}

// TestContextStrokeBrush tests the StrokeBrush getter.
func TestContextStrokeBrush(t *testing.T) {
	dc := NewContext(100, 100)
	// Default should be black
	brush := dc.StrokeBrush()
	c := brush.ColorAt(0, 0)
	if c != Black {
		t.Errorf("default StrokeBrush color = %v, want Black", c)
	}
}

// TestContextSetColorUpdatesInlineSolid tests that SetColor stores inline.
func TestContextSetColorUpdatesInlineSolid(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetRGB(1, 0, 0) // Red

	// Check via GetBrush (returns SolidBrush from inline color)
	brush := dc.FillBrush()
	c := brush.ColorAt(0, 0)
	if c != Red {
		t.Errorf("brush color = %v, want Red", c)
	}

	// Verify inline solid storage (brush and pattern are nil for zero alloc)
	if !dc.paint.IsSolid() {
		t.Error("IsSolid = false after SetRGB")
	}
	sc, ok := dc.paint.SolidColor()
	if !ok || sc != Red {
		t.Errorf("SolidColor = %v, %v, want Red, true", sc, ok)
	}
	if dc.paint.fill.brush != nil {
		t.Error("fill.brush should be nil after SetRGB (stored inline)")
	}
	if dc.paint.fill.pattern != nil {
		t.Error("fill.pattern should be nil after SetRGB (stored inline)")
	}
}

// TestDualBrushIndependence verifies that fill and stroke brushes are independent.
// This is the core behavioral test for ADR-055.
func TestDualBrushIndependence(t *testing.T) {
	t.Run("SetFillBrush does not affect stroke", func(t *testing.T) {
		p := NewPaint()
		p.SetFillBrush(Solid(Red))

		// Fill should be red
		fc := p.FillColorAt(0, 0)
		if fc != Red {
			t.Errorf("FillColorAt = %v, want Red", fc)
		}

		// Stroke should still be black (default)
		sc := p.StrokeColorAt(0, 0)
		if sc != Black {
			t.Errorf("StrokeColorAt = %v, want Black", sc)
		}
	})

	t.Run("SetStrokeBrush does not affect fill", func(t *testing.T) {
		p := NewPaint()
		p.SetStrokeBrush(Solid(Blue))

		// Fill should still be black (default)
		fc := p.FillColorAt(0, 0)
		if fc != Black {
			t.Errorf("FillColorAt = %v, want Black", fc)
		}

		// Stroke should be blue
		sc := p.StrokeColorAt(0, 0)
		if sc != Blue {
			t.Errorf("StrokeColorAt = %v, want Blue", sc)
		}
	})

	t.Run("SetBrush sets both sides", func(t *testing.T) {
		p := NewPaint()
		p.SetBrush(Solid(Green))

		fc := p.FillColorAt(0, 0)
		sc := p.StrokeColorAt(0, 0)
		if fc != Green {
			t.Errorf("FillColorAt = %v, want Green", fc)
		}
		if sc != Green {
			t.Errorf("StrokeColorAt = %v, want Green", sc)
		}
	})

	t.Run("SetColor sets both sides", func(t *testing.T) {
		dc := NewContext(100, 100)
		dc.SetRGB(1, 0, 0) // Red

		fc := dc.paint.FillColorAt(0, 0)
		sc := dc.paint.StrokeColorAt(0, 0)
		if fc != Red {
			t.Errorf("FillColorAt = %v, want Red", fc)
		}
		if sc != Red {
			t.Errorf("StrokeColorAt = %v, want Red", sc)
		}
	})

	t.Run("independent fill and stroke colors", func(t *testing.T) {
		dc := NewContext(100, 100)
		dc.SetFillBrush(Solid(Red))
		dc.SetStrokeBrush(Solid(Blue))

		fb := dc.FillBrush()
		sb := dc.StrokeBrush()

		fc := fb.ColorAt(0, 0)
		sc := sb.ColorAt(0, 0)

		if fc != Red {
			t.Errorf("FillBrush color = %v, want Red", fc)
		}
		if sc != Blue {
			t.Errorf("StrokeBrush color = %v, want Blue", sc)
		}
	})
}

// TestDualBrushClone verifies Clone copies both sides independently.
func TestDualBrushClone(t *testing.T) {
	p := NewPaint()
	p.SetFillBrush(Solid(Red))
	p.SetStrokeBrush(Solid(Blue))

	clone := p.Clone()

	// Verify clone has independent colors
	fc, _ := clone.FillSolidColor()
	sc, _ := clone.StrokeSolidColor()
	if fc != Red {
		t.Errorf("clone FillSolidColor = %v, want Red", fc)
	}
	if sc != Blue {
		t.Errorf("clone StrokeSolidColor = %v, want Blue", sc)
	}

	// Modify clone, verify original unchanged
	clone.SetFillBrush(Solid(Green))
	origFC, _ := p.FillSolidColor()
	if origFC != Red {
		t.Errorf("original FillSolidColor = %v after clone mutation, want Red", origFC)
	}
}

// TestDualBrushGradient verifies gradient fill with solid stroke.
func TestDualBrushGradient(t *testing.T) {
	p := NewPaint()
	grad := NewLinearGradientBrush(0, 0, 100, 0)
	grad.AddColorStop(0, Red)
	grad.AddColorStop(1, Blue)

	p.SetFillBrush(grad)
	p.SetStrokeBrush(Solid(Green))

	// Fill should not be solid (it's a gradient)
	if p.IsFillSolid() {
		t.Error("IsFillSolid = true for gradient")
	}

	// Stroke should be solid green
	if !p.IsStrokeSolid() {
		t.Error("IsStrokeSolid = false for solid brush")
	}
	sc, ok := p.StrokeSolidColor()
	if !ok || sc != Green {
		t.Errorf("StrokeSolidColor = %v, %v, want Green, true", sc, ok)
	}

	// Verify gradient samples
	fc := p.FillColorAt(0, 0) // at x=0, should be ~Red
	if fc.R < 0.9 {
		t.Errorf("FillColorAt(0,0) red component = %f, want ~1.0", fc.R)
	}
}

// TestDualBrushNewPaint verifies both sides initialized to Black.
func TestDualBrushNewPaint(t *testing.T) {
	p := NewPaint()

	fc, fOK := p.FillSolidColor()
	sc, sOK := p.StrokeSolidColor()

	if !fOK || fc != Black {
		t.Errorf("NewPaint fill = %v, %v, want Black, true", fc, fOK)
	}
	if !sOK || sc != Black {
		t.Errorf("NewPaint stroke = %v, %v, want Black, true", sc, sOK)
	}
}

// TestDualBrushPattern verifies SetFillPattern and SetStrokePattern independence.
func TestDualBrushPattern(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetFillPattern(NewSolidPattern(Red))
	dc.SetStrokePattern(NewSolidPattern(Blue))

	fc := dc.paint.FillColorAt(0, 0)
	sc := dc.paint.StrokeColorAt(0, 0)
	if fc != Red {
		t.Errorf("FillColorAt after SetFillPattern = %v, want Red", fc)
	}
	if sc != Blue {
		t.Errorf("StrokeColorAt after SetStrokePattern = %v, want Blue", sc)
	}
}

// TestDualBrushColorAtBackwardCompat verifies backward compatibility.
// ColorAt reads from fill side (backward compat for code that doesn't
// distinguish fill/stroke).
func TestDualBrushColorAtBackwardCompat(t *testing.T) {
	p := NewPaint()
	p.SetFillBrush(Solid(Red))
	p.SetStrokeBrush(Solid(Blue))

	// ColorAt (deprecated) reads from fill
	c := p.ColorAt(0, 0)
	if c != Red {
		t.Errorf("ColorAt = %v, want Red (backward compat reads fill)", c)
	}

	// SolidColor (deprecated) reads from fill
	sc, ok := p.SolidColor()
	if !ok || sc != Red {
		t.Errorf("SolidColor = %v, %v, want Red, true", sc, ok)
	}

	// IsSolid (deprecated) reads from fill
	if !p.IsSolid() {
		t.Error("IsSolid = false, want true (backward compat reads fill)")
	}
}

// TestDualBrushRendering verifies that FillPreserve + Stroke with different
// brushes produces correct pixel colors (ADR-055 integration test).
func TestDualBrushRendering(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetFillBrush(Solid(Red))
	dc.SetStrokeBrush(Solid(Blue))
	dc.SetLineWidth(4)

	// Draw a 60x60 rectangle centered in the canvas.
	dc.DrawRectangle(20, 20, 60, 60)
	if err := dc.FillPreserve(); err != nil {
		t.Fatalf("FillPreserve: %v", err)
	}
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	// Center of rectangle should be red (fill color).
	img := dc.Image()
	centerPixel := img.At(50, 50)
	cr, cg, cb, _ := centerPixel.RGBA()
	if cr>>8 < 200 || cg>>8 > 50 || cb>>8 > 50 {
		t.Errorf("center pixel = (%d,%d,%d), want red", cr>>8, cg>>8, cb>>8)
	}

	// Edge of rectangle should be blue (stroke color).
	// Check left edge at y=50 (inside the 4px stroke centered on x=20).
	edgePixel := img.At(20, 50)
	er, eg, eb, _ := edgePixel.RGBA()
	if eb>>8 < 150 || er>>8 > 100 {
		t.Errorf("edge pixel = (%d,%d,%d), want blue", er>>8, eg>>8, eb>>8)
	}
}

// TestDualBrushSetColorOverridesBoth verifies SetColor overrides both sides.
func TestDualBrushSetColorOverridesBoth(t *testing.T) {
	dc := NewContext(100, 100)

	// Set independent colors
	dc.SetFillBrush(Solid(Red))
	dc.SetStrokeBrush(Solid(Blue))

	// Now SetColor should reset both to green
	dc.SetRGB(0, 1, 0)

	fc := dc.paint.FillColorAt(0, 0)
	sc := dc.paint.StrokeColorAt(0, 0)
	if fc != Green {
		t.Errorf("FillColorAt after SetRGB = %v, want Green", fc)
	}
	if sc != Green {
		t.Errorf("StrokeColorAt after SetRGB = %v, want Green", sc)
	}
}

// TestDualBrushSetHexColorOverridesBoth verifies SetHexColor overrides both.
func TestDualBrushSetHexColorOverridesBoth(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetFillBrush(Solid(Red))
	dc.SetStrokeBrush(Solid(Blue))

	dc.SetHexColor("#00FF00")
	fc := dc.paint.FillColorAt(0, 0)
	sc := dc.paint.StrokeColorAt(0, 0)
	// Both should be green
	if fc.G < 0.99 || fc.R > 0.01 {
		t.Errorf("fill after SetHexColor = %v, want green", fc)
	}
	if sc.G < 0.99 || sc.R > 0.01 {
		t.Errorf("stroke after SetHexColor = %v, want green", sc)
	}
}

// BenchmarkPaintSetBrush benchmarks SetBrush.
func BenchmarkPaintSetBrush(b *testing.B) {
	p := NewPaint()
	brush := Solid(Red)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.SetBrush(brush)
	}
}

// BenchmarkPaintColorAt benchmarks ColorAt.
func BenchmarkPaintColorAt(b *testing.B) {
	p := NewPaint()
	p.SetBrush(Solid(Red))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.ColorAt(float64(i%100), float64(i%100))
	}
}
