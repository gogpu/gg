package gg

import (
	"testing"
)

// TestCustomPatternFill reproduces the original bug: custom Pattern
// implementations should produce non-black pixels when used as fill.
func TestCustomPatternFill(t *testing.T) {
	dc := NewContext(100, 100)

	// Create a custom pattern that returns Green everywhere
	pattern := &testPattern{colorFn: func(_, _ float64) RGBA { return Green }}
	dc.SetFillPattern(pattern)

	// Draw and fill a rectangle
	dc.DrawRectangle(10, 10, 80, 80)
	dc.Fill()

	// Check center pixel — should be Green, not Black
	center := dc.pixmap.GetPixel(50, 50)
	if center.R < 0.01 && center.G < 0.01 && center.B < 0.01 {
		t.Errorf("center pixel is black (%v), custom pattern was not rendered", center)
	}
	if center.G < 0.9 {
		t.Errorf("center pixel G = %v, want ~1.0 (Green)", center.G)
	}
}

// TestCustomBrushFill verifies that CustomBrush (gradient, checkerboard, etc.)
// produces non-black pixels when used as fill.
func TestCustomBrushFill(t *testing.T) {
	dc := NewContext(100, 100)

	// Use a horizontal gradient from Red to Blue
	dc.SetFillBrush(HorizontalGradient(Red, Blue, 0, 100))

	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	// Left side should be reddish
	left := dc.pixmap.GetPixel(5, 50)
	if left.R < 0.5 {
		t.Errorf("left pixel R = %v, want > 0.5 (reddish)", left.R)
	}

	// Right side should be bluish
	right := dc.pixmap.GetPixel(95, 50)
	if right.B < 0.5 {
		t.Errorf("right pixel B = %v, want > 0.5 (bluish)", right.B)
	}

	// There should be color variation across the gradient
	if colorDistance(left, right) < 0.3 {
		t.Error("gradient fill shows no color variation between left and right")
	}
}

// TestCheckerboardFill verifies checkerboard brush produces alternating colors.
func TestCheckerboardFill(t *testing.T) {
	dc := NewContext(100, 100)

	checker := Checkerboard(White, Black, 10)
	dc.SetFillBrush(checker)

	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	// Pixel at (5, 5) — first square (should be White or Black)
	p1 := dc.pixmap.GetPixel(5, 5)
	// Pixel at (15, 5) — second square (should be opposite)
	p2 := dc.pixmap.GetPixel(15, 5)

	// The two pixels should be different (one White, one Black)
	dist := colorDistance(p1, p2)
	if dist < 0.5 {
		t.Errorf("checkerboard squares are too similar: p1=%v, p2=%v, dist=%v", p1, p2, dist)
	}
}

// TestSolidFillStillWorks ensures solid colors still use the fast path.
func TestSolidFillStillWorks(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetRGB(1, 0, 0) // Red

	dc.DrawRectangle(10, 10, 80, 80)
	dc.Fill()

	center := dc.pixmap.GetPixel(50, 50)
	if center.R < 0.9 || center.G > 0.1 || center.B > 0.1 {
		t.Errorf("solid fill center = %v, want approximately Red", center)
	}
}

// TestSetFillPatternUpdatesBrush verifies SetFillPattern keeps Brush in sync.
func TestSetFillPatternUpdatesBrush(t *testing.T) {
	dc := NewContext(100, 100)

	pattern := &testPattern{colorFn: func(_, _ float64) RGBA { return Cyan }}
	dc.SetFillPattern(pattern)

	// Brush should now reflect the pattern
	brush := dc.paint.Brush
	if brush == nil {
		t.Fatal("Brush is nil after SetFillPattern")
	}

	c := brush.ColorAt(0, 0)
	if c != Cyan {
		t.Errorf("Brush.ColorAt = %v, want Cyan", c)
	}
}

// TestSetStrokePatternUpdatesBrush verifies SetStrokePattern keeps Brush in sync.
func TestSetStrokePatternUpdatesBrush(t *testing.T) {
	dc := NewContext(100, 100)

	pattern := &testPattern{colorFn: func(_, _ float64) RGBA { return Magenta }}
	dc.SetStrokePattern(pattern)

	brush := dc.paint.Brush
	if brush == nil {
		t.Fatal("Brush is nil after SetStrokePattern")
	}

	c := brush.ColorAt(0, 0)
	if c != Magenta {
		t.Errorf("Brush.ColorAt = %v, want Magenta", c)
	}
}

// TestGradientFillVariation verifies gradient fills have actual color variation.
func TestGradientFillVariation(t *testing.T) {
	dc := NewContext(100, 100)

	dc.SetFillBrush(VerticalGradient(White, Black, 0, 100))
	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	// Top should be bright, bottom should be dark
	top := dc.pixmap.GetPixel(50, 5)
	bottom := dc.pixmap.GetPixel(50, 95)

	if top.R < 0.5 {
		t.Errorf("top pixel R = %v, want > 0.5 (bright)", top.R)
	}
	if bottom.R > 0.5 {
		t.Errorf("bottom pixel R = %v, want < 0.5 (dark)", bottom.R)
	}
}

// TestSolidColorFromPaint tests the internal solidColorFromPaint function.
func TestSolidColorFromPaint(t *testing.T) {
	t.Run("solid brush", func(t *testing.T) {
		paint := NewPaint()
		paint.SetBrush(Solid(Red))
		color, ok := solidColorFromPaint(paint)
		if !ok {
			t.Fatal("expected ok=true for solid brush")
		}
		if color != Red {
			t.Errorf("color = %v, want Red", color)
		}
	})

	t.Run("solid pattern", func(t *testing.T) {
		paint := &Paint{
			Pattern: NewSolidPattern(Blue),
		}
		color, ok := solidColorFromPaint(paint)
		if !ok {
			t.Fatal("expected ok=true for solid pattern")
		}
		if color != Blue {
			t.Errorf("color = %v, want Blue", color)
		}
	})

	t.Run("custom brush", func(t *testing.T) {
		paint := NewPaint()
		paint.SetBrush(NewCustomBrush(func(_, _ float64) RGBA { return Green }))
		_, ok := solidColorFromPaint(paint)
		if ok {
			t.Error("expected ok=false for custom brush")
		}
	})

	t.Run("custom pattern", func(t *testing.T) {
		paint := &Paint{
			Pattern: &testPattern{colorFn: func(_, _ float64) RGBA { return Green }},
		}
		_, ok := solidColorFromPaint(paint)
		if ok {
			t.Error("expected ok=false for custom pattern")
		}
	})
}

// colorDistance returns the squared Euclidean distance between two colors (RGB only).
func colorDistance(a, b RGBA) float64 {
	dr := a.R - b.R
	dg := a.G - b.G
	db := a.B - b.B
	return dr*dr + dg*dg + db*db
}
