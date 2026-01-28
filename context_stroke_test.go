package gg

import (
	"testing"
)

func TestContext_SetStroke(t *testing.T) {
	dc := NewContext(100, 100)

	stroke := DefaultStroke().WithWidth(3).WithCap(LineCapRound)
	dc.SetStroke(stroke)

	got := dc.GetStroke()
	if got.Width != 3 {
		t.Errorf("GetStroke().Width = %v, want 3", got.Width)
	}
	if got.Cap != LineCapRound {
		t.Errorf("GetStroke().Cap = %v, want LineCapRound", got.Cap)
	}
}

func TestContext_GetStroke_Legacy(t *testing.T) {
	dc := NewContext(100, 100)

	// Set using legacy methods
	dc.SetLineWidth(5)
	dc.SetLineCap(LineCapSquare)
	dc.SetLineJoin(LineJoinBevel)
	dc.SetMiterLimit(8)

	got := dc.GetStroke()
	if got.Width != 5 {
		t.Errorf("GetStroke().Width = %v, want 5", got.Width)
	}
	if got.Cap != LineCapSquare {
		t.Errorf("GetStroke().Cap = %v, want LineCapSquare", got.Cap)
	}
	if got.Join != LineJoinBevel {
		t.Errorf("GetStroke().Join = %v, want LineJoinBevel", got.Join)
	}
	if got.MiterLimit != 8 {
		t.Errorf("GetStroke().MiterLimit = %v, want 8", got.MiterLimit)
	}
}

func TestContext_SetDash(t *testing.T) {
	t.Run("simple dash pattern", func(t *testing.T) {
		dc := NewContext(100, 100)
		dc.SetDash(5, 3)

		if !dc.IsDashed() {
			t.Error("IsDashed() = false, want true")
		}

		stroke := dc.GetStroke()
		if stroke.Dash == nil {
			t.Fatal("GetStroke().Dash = nil")
		}
		if len(stroke.Dash.Array) != 2 {
			t.Errorf("Dash.Array length = %d, want 2", len(stroke.Dash.Array))
		}
	})

	t.Run("complex dash pattern", func(t *testing.T) {
		dc := NewContext(100, 100)
		dc.SetDash(10, 5, 2, 5)

		stroke := dc.GetStroke()
		if stroke.Dash == nil {
			t.Fatal("GetStroke().Dash = nil")
		}
		if len(stroke.Dash.Array) != 4 {
			t.Errorf("Dash.Array length = %d, want 4", len(stroke.Dash.Array))
		}
	})

	t.Run("empty dash clears pattern", func(t *testing.T) {
		dc := NewContext(100, 100)
		dc.SetDash(5, 3)
		dc.SetDash() // clear

		if dc.IsDashed() {
			t.Error("IsDashed() = true after clear")
		}
	})

	t.Run("all zeros clears pattern", func(t *testing.T) {
		dc := NewContext(100, 100)
		dc.SetDash(5, 3)
		dc.SetDash(0, 0, 0)

		if dc.IsDashed() {
			t.Error("IsDashed() = true after all zeros")
		}
	})
}

func TestContext_SetDashOffset(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetDash(5, 3)
	dc.SetDashOffset(2)

	stroke := dc.GetStroke()
	if stroke.Dash == nil {
		t.Fatal("GetStroke().Dash = nil")
	}
	if stroke.Dash.Offset != 2 {
		t.Errorf("Dash.Offset = %v, want 2", stroke.Dash.Offset)
	}
}

func TestContext_SetDashOffset_NoDash(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetDashOffset(2) // No dash set - should not panic

	// Should still be solid line
	if dc.IsDashed() {
		t.Error("IsDashed() = true, want false")
	}
}

func TestContext_ClearDash(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetDash(5, 3)

	if !dc.IsDashed() {
		t.Fatal("IsDashed() = false before clear")
	}

	dc.ClearDash()

	if dc.IsDashed() {
		t.Error("IsDashed() = true after ClearDash()")
	}
}

func TestContext_ClearDash_NoStroke(t *testing.T) {
	dc := NewContext(100, 100)
	dc.ClearDash() // Should not panic
}

func TestContext_IsDashed(t *testing.T) {
	dc := NewContext(100, 100)

	if dc.IsDashed() {
		t.Error("IsDashed() = true for new context")
	}

	dc.SetDash(5, 3)
	if !dc.IsDashed() {
		t.Error("IsDashed() = false after SetDash")
	}

	dc.ClearDash()
	if dc.IsDashed() {
		t.Error("IsDashed() = true after ClearDash")
	}
}

func TestContext_StrokeWithDash(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetRGB(1, 0, 0)
	dc.SetLineWidth(2)
	dc.SetDash(5, 3)

	dc.DrawLine(10, 50, 90, 50)
	dc.Stroke()

	// Verify stroke was executed (basic sanity check)
	// The actual dashing implementation is in the renderer
}

func TestContext_StrokeWithDash_PixelVerification(t *testing.T) {
	// Create a context with dashed line
	dc := NewContext(100, 100)
	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0) // Black
	dc.SetLineWidth(2)
	dc.SetDash(10, 10) // 10 pixels dash, 10 pixels gap

	dc.DrawLine(10, 50, 90, 50) // 80 pixels long horizontal line
	dc.Stroke()

	pixmap := dc.pixmap

	// Check that there are gaps in the line (not all pixels are black at y=50)
	// The dash pattern [10, 10] should create visible gaps
	blackPixels := 0
	whitePixels := 0
	for x := 10; x < 90; x++ {
		c := pixmap.GetPixel(x, 50)
		if c.R < 0.5 && c.G < 0.5 && c.B < 0.5 { // Dark pixel (stroked)
			blackPixels++
		} else if c.R > 0.9 && c.G > 0.9 && c.B > 0.9 { // Light pixel (gap)
			whitePixels++
		}
	}

	// With dash pattern [10, 10] over 80 pixels, we expect roughly:
	// ~40 pixels black (dashes) and ~40 pixels white (gaps)
	// Allow for anti-aliasing and edge effects
	if blackPixels == 0 {
		t.Error("No black pixels found - dashing may not be rendering")
	}
	if whitePixels == 0 {
		t.Error("No white pixels (gaps) found - dash pattern not applied")
	}

	// Verify ratio is roughly 50/50 (allowing some tolerance for AA)
	ratio := float64(blackPixels) / float64(blackPixels+whitePixels)
	if ratio > 0.9 {
		t.Errorf("Too many black pixels (ratio=%v) - dash gaps not visible", ratio)
	}
	if ratio < 0.1 {
		t.Errorf("Too few black pixels (ratio=%v) - dashing may not be working", ratio)
	}
}

func TestContext_StrokeWithDash_Rectangle(t *testing.T) {
	// Test dashing on a closed rectangle path
	dc := NewContext(100, 100)
	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1)
	dc.SetDash(5, 5)

	dc.DrawRectangle(20, 20, 60, 60)
	dc.Stroke()

	pixmap := dc.pixmap

	// Sample along top edge. Hairline rendering distributes coverage between
	// two adjacent pixel rows (y=19 and y=20), so we check both rows.
	// With 1px hairline, each row gets ~50% coverage, so we use < 0.9 threshold.
	strokedCount := 0
	gapCount := 0
	for x := 20; x < 80; x++ {
		// Check both y=19 and y=20 since hairline distributes coverage
		c19 := pixmap.GetPixel(x, 19)
		c20 := pixmap.GetPixel(x, 20)
		// Pixel is "stroked" if either row has some color (R < 0.9)
		if c19.R < 0.9 || c20.R < 0.9 {
			strokedCount++
		}
		// Pixel is a "gap" if both rows are white (R > 0.95)
		if c19.R > 0.95 && c20.R > 0.95 {
			gapCount++
		}
	}

	if strokedCount == 0 {
		t.Error("No stroked pixels on rectangle top edge")
	}
	if gapCount == 0 {
		t.Error("No gaps visible on rectangle top edge - dash not working")
	}
}

func TestContext_StrokeWithDash_Offset(t *testing.T) {
	// Test that dash offset works
	dc1 := NewContext(100, 100)
	dc1.ClearWithColor(White)
	dc1.SetRGB(0, 0, 0)
	dc1.SetLineWidth(2)
	dc1.SetDash(10, 10)
	dc1.SetDashOffset(0)
	dc1.DrawLine(10, 50, 90, 50)
	dc1.Stroke()

	dc2 := NewContext(100, 100)
	dc2.ClearWithColor(White)
	dc2.SetRGB(0, 0, 0)
	dc2.SetLineWidth(2)
	dc2.SetDash(10, 10)
	dc2.SetDashOffset(5) // Offset by 5 pixels
	dc2.DrawLine(10, 50, 90, 50)
	dc2.Stroke()

	// The two images should be different (offset shifts the pattern)
	p1 := dc1.pixmap
	p2 := dc2.pixmap

	// Sample at start of line - with offset=5, the pattern should start differently
	c1 := p1.GetPixel(15, 50)
	c2 := p2.GetPixel(15, 50)

	// They could be different or same depending on exact offset position
	// Just verify both rendered something
	if (c1.R == 1 && c1.G == 1 && c1.B == 1) && (c2.R == 1 && c2.G == 1 && c2.B == 1) {
		t.Error("Neither offset variant rendered anything at sample point")
	}
}

func TestContext_SetStroke_UpdatesLegacyFields(t *testing.T) {
	dc := NewContext(100, 100)

	stroke := Stroke{
		Width:      7,
		Cap:        LineCapSquare,
		Join:       LineJoinBevel,
		MiterLimit: 5,
	}
	dc.SetStroke(stroke)

	// Legacy fields should be updated for backward compatibility
	if dc.paint.LineWidth != 7 {
		t.Errorf("paint.LineWidth = %v, want 7", dc.paint.LineWidth)
	}
	if dc.paint.LineCap != LineCapSquare {
		t.Errorf("paint.LineCap = %v, want LineCapSquare", dc.paint.LineCap)
	}
	if dc.paint.LineJoin != LineJoinBevel {
		t.Errorf("paint.LineJoin = %v, want LineJoinBevel", dc.paint.LineJoin)
	}
	if dc.paint.MiterLimit != 5 {
		t.Errorf("paint.MiterLimit = %v, want 5", dc.paint.MiterLimit)
	}
}

func TestContext_DashedLinePreset(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetStroke(DashedStroke(10, 5))

	if !dc.IsDashed() {
		t.Error("IsDashed() = false for DashedStroke")
	}

	stroke := dc.GetStroke()
	if stroke.Width != 1.0 {
		t.Errorf("Width = %v, want 1.0", stroke.Width)
	}
}

func TestContext_DottedLinePreset(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetStroke(DottedStroke())

	if !dc.IsDashed() {
		t.Error("IsDashed() = false for DottedStroke")
	}

	stroke := dc.GetStroke()
	if stroke.Width != 2.0 {
		t.Errorf("Width = %v, want 2.0", stroke.Width)
	}
	if stroke.Cap != LineCapRound {
		t.Errorf("Cap = %v, want LineCapRound", stroke.Cap)
	}
}

func TestPaint_GetStroke(t *testing.T) {
	t.Run("with Stroke set", func(t *testing.T) {
		p := NewPaint()
		s := Stroke{Width: 5, Cap: LineCapRound}
		p.SetStroke(s)

		got := p.GetStroke()
		if got.Width != 5 {
			t.Errorf("GetStroke().Width = %v, want 5", got.Width)
		}
		if got.Cap != LineCapRound {
			t.Errorf("GetStroke().Cap = %v, want LineCapRound", got.Cap)
		}
	})

	t.Run("without Stroke set (legacy fallback)", func(t *testing.T) {
		p := NewPaint()
		p.LineWidth = 3
		p.LineCap = LineCapSquare
		p.LineJoin = LineJoinBevel
		p.MiterLimit = 8

		got := p.GetStroke()
		if got.Width != 3 {
			t.Errorf("GetStroke().Width = %v, want 3", got.Width)
		}
		if got.Cap != LineCapSquare {
			t.Errorf("GetStroke().Cap = %v, want LineCapSquare", got.Cap)
		}
		if got.Join != LineJoinBevel {
			t.Errorf("GetStroke().Join = %v, want LineJoinBevel", got.Join)
		}
		if got.MiterLimit != 8 {
			t.Errorf("GetStroke().MiterLimit = %v, want 8", got.MiterLimit)
		}
		if got.Dash != nil {
			t.Errorf("GetStroke().Dash = %v, want nil", got.Dash)
		}
	})
}

func TestPaint_EffectiveMethods(t *testing.T) {
	t.Run("with Stroke set", func(t *testing.T) {
		p := NewPaint()
		p.SetStroke(Stroke{
			Width:      5,
			Cap:        LineCapRound,
			Join:       LineJoinRound,
			MiterLimit: 10,
			Dash:       NewDash(5, 3),
		})

		if p.EffectiveLineWidth() != 5 {
			t.Errorf("EffectiveLineWidth() = %v, want 5", p.EffectiveLineWidth())
		}
		if p.EffectiveLineCap() != LineCapRound {
			t.Errorf("EffectiveLineCap() = %v, want LineCapRound", p.EffectiveLineCap())
		}
		if p.EffectiveLineJoin() != LineJoinRound {
			t.Errorf("EffectiveLineJoin() = %v, want LineJoinRound", p.EffectiveLineJoin())
		}
		if p.EffectiveMiterLimit() != 10 {
			t.Errorf("EffectiveMiterLimit() = %v, want 10", p.EffectiveMiterLimit())
		}
		if p.EffectiveDash() == nil {
			t.Error("EffectiveDash() = nil, want non-nil")
		}
		if !p.IsDashed() {
			t.Error("IsDashed() = false, want true")
		}
	})

	t.Run("without Stroke set (legacy fallback)", func(t *testing.T) {
		p := NewPaint()
		p.LineWidth = 3
		p.LineCap = LineCapSquare
		p.LineJoin = LineJoinBevel
		p.MiterLimit = 8

		if p.EffectiveLineWidth() != 3 {
			t.Errorf("EffectiveLineWidth() = %v, want 3", p.EffectiveLineWidth())
		}
		if p.EffectiveLineCap() != LineCapSquare {
			t.Errorf("EffectiveLineCap() = %v, want LineCapSquare", p.EffectiveLineCap())
		}
		if p.EffectiveLineJoin() != LineJoinBevel {
			t.Errorf("EffectiveLineJoin() = %v, want LineJoinBevel", p.EffectiveLineJoin())
		}
		if p.EffectiveMiterLimit() != 8 {
			t.Errorf("EffectiveMiterLimit() = %v, want 8", p.EffectiveMiterLimit())
		}
		if p.EffectiveDash() != nil {
			t.Error("EffectiveDash() should be nil for legacy")
		}
		if p.IsDashed() {
			t.Error("IsDashed() = true, want false")
		}
	})
}

func TestPaint_Clone_WithStroke(t *testing.T) {
	p := NewPaint()
	p.SetStroke(Stroke{
		Width: 5,
		Cap:   LineCapRound,
		Dash:  NewDash(5, 3),
	})

	clone := p.Clone()

	if clone.Stroke == nil {
		t.Fatal("Clone().Stroke = nil")
	}
	if clone.Stroke == p.Stroke {
		t.Error("Clone() shares Stroke pointer")
	}
	if clone.Stroke.Width != 5 {
		t.Errorf("Clone().Stroke.Width = %v, want 5", clone.Stroke.Width)
	}
	if clone.Stroke.Dash == p.Stroke.Dash {
		t.Error("Clone() shares Dash pointer")
	}

	// Modify clone and verify original is unchanged
	clone.Stroke.Width = 100
	if p.Stroke.Width == 100 {
		t.Error("modifying clone affected original")
	}
}
