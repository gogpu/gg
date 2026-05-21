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

		// Solid brushes are stored inline — Brush and Pattern are nil.
		if !p.IsSolid() {
			t.Error("IsSolid = false after SetBrush(Solid)")
		}
		c, ok := p.SolidColor()
		if !ok || c != Blue {
			t.Errorf("SolidColor = %v, %v, want Blue, true", c, ok)
		}
		if p.Brush != nil {
			t.Error("Brush should be nil for solid color")
		}
		if p.Pattern != nil {
			t.Error("Pattern should be nil for solid color")
		}
	})

	t.Run("non-solid brush sets fields", func(t *testing.T) {
		p := NewPaint()
		custom := CustomBrush{Func: func(x, y float64) RGBA { return Red }, Name: "test"}
		p.SetBrush(custom)

		if p.IsSolid() {
			t.Error("IsSolid = true after SetBrush(CustomBrush)")
		}
		if p.Brush == nil {
			t.Error("Brush = nil after SetBrush(CustomBrush)")
		}
		if p.Pattern == nil {
			t.Error("Pattern = nil after SetBrush(CustomBrush)")
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
		p.Brush = Solid(Green) // Direct field write bypasses SetBrush
		p.isSolid = false      // Explicitly not inline
		brush := p.GetBrush()
		if sb, ok := brush.(SolidBrush); !ok || sb.Color != Green {
			t.Error("GetBrush did not return set brush")
		}
	})

	t.Run("with only pattern set", func(t *testing.T) {
		p := &Paint{
			Pattern: NewSolidPattern(Yellow),
		}
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
		p := &Paint{Brush: Solid(Red)}
		c := p.ColorAt(0, 0)
		if c != Red {
			t.Errorf("ColorAt = %v, want Red", c)
		}
	})

	t.Run("with only pattern set", func(t *testing.T) {
		p := &Paint{
			Pattern: NewSolidPattern(Blue),
		}
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
		p := &Paint{
			Pattern: NewSolidPattern(Blue),
			Brush:   Solid(Red),
		}
		c := p.ColorAt(0, 0)
		if c != Red {
			t.Errorf("ColorAt = %v, want Red (brush should take precedence)", c)
		}
	})

	t.Run("isSolid takes precedence over both", func(t *testing.T) {
		p := &Paint{
			solidColor: Green,
			isSolid:    true,
			Pattern:    NewSolidPattern(Blue),
			Brush:      Solid(Red),
		}
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

	// Verify inline solid storage (Brush and Pattern are nil for zero alloc)
	if !dc.paint.IsSolid() {
		t.Error("IsSolid = false after SetRGB")
	}
	sc, ok := dc.paint.SolidColor()
	if !ok || sc != Red {
		t.Errorf("SolidColor = %v, %v, want Red, true", sc, ok)
	}
	if dc.paint.Brush != nil {
		t.Error("Brush should be nil after SetRGB (stored inline)")
	}
	if dc.paint.Pattern != nil {
		t.Error("Pattern should be nil after SetRGB (stored inline)")
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
