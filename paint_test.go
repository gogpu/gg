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
	if p.Brush == nil {
		t.Error("Brush = nil, want non-nil")
	}
	if p.Pattern == nil {
		t.Error("Pattern = nil, want non-nil")
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
	if clone.Brush == nil {
		t.Error("clone.Brush = nil")
	}

	// Verify it's a separate object
	clone.LineWidth = 10.0
	if p.LineWidth == clone.LineWidth {
		t.Error("Clone is not independent")
	}
}

// TestPaintSetBrush tests the SetBrush method.
func TestPaintSetBrush(t *testing.T) {
	p := NewPaint()
	brush := Solid(Blue)
	p.SetBrush(brush)

	if sb, ok := p.Brush.(SolidBrush); !ok || sb.Color != Blue {
		t.Error("SetBrush did not set brush correctly")
	}
	if p.Pattern == nil {
		t.Error("SetBrush did not update Pattern for compatibility")
	}
}

// TestPaintGetBrush tests the GetBrush method.
func TestPaintGetBrush(t *testing.T) {
	t.Run("with brush set", func(t *testing.T) {
		p := NewPaint()
		p.Brush = Solid(Green)
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
	t.Run("with brush set", func(t *testing.T) {
		p := NewPaint()
		p.Brush = Solid(Red)
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
}

// TestContextSetFillBrush tests the SetFillBrush method.
func TestContextSetFillBrush(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetFillBrush(Solid(Magenta))

	brush := ctx.FillBrush()
	c := brush.ColorAt(0, 0)
	if c != Magenta {
		t.Errorf("FillBrush color = %v, want Magenta", c)
	}
}

// TestContextSetStrokeBrush tests the SetStrokeBrush method.
func TestContextSetStrokeBrush(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetStrokeBrush(Solid(Cyan))

	brush := ctx.StrokeBrush()
	c := brush.ColorAt(0, 0)
	if c != Cyan {
		t.Errorf("StrokeBrush color = %v, want Cyan", c)
	}
}

// TestContextFillBrush tests the FillBrush getter.
func TestContextFillBrush(t *testing.T) {
	ctx := NewContext(100, 100)
	// Default should be black
	brush := ctx.FillBrush()
	c := brush.ColorAt(0, 0)
	if c != Black {
		t.Errorf("default FillBrush color = %v, want Black", c)
	}
}

// TestContextStrokeBrush tests the StrokeBrush getter.
func TestContextStrokeBrush(t *testing.T) {
	ctx := NewContext(100, 100)
	// Default should be black
	brush := ctx.StrokeBrush()
	c := brush.ColorAt(0, 0)
	if c != Black {
		t.Errorf("default StrokeBrush color = %v, want Black", c)
	}
}

// TestContextSetColorUpdatesPatternAndBrush tests that SetColor updates both.
func TestContextSetColorUpdatesPatternAndBrush(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetRGB(1, 0, 0) // Red

	// Check brush
	brush := ctx.FillBrush()
	c := brush.ColorAt(0, 0)
	if c != Red {
		t.Errorf("brush color = %v, want Red", c)
	}

	// Check pattern (for backward compatibility)
	if ctx.paint.Pattern == nil {
		t.Error("Pattern is nil after SetRGB")
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
