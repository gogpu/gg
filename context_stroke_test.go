package gg

import (
	"testing"
)

func TestContext_SetStroke(t *testing.T) {
	ctx := NewContext(100, 100)

	stroke := DefaultStroke().WithWidth(3).WithCap(LineCapRound)
	ctx.SetStroke(stroke)

	got := ctx.GetStroke()
	if got.Width != 3 {
		t.Errorf("GetStroke().Width = %v, want 3", got.Width)
	}
	if got.Cap != LineCapRound {
		t.Errorf("GetStroke().Cap = %v, want LineCapRound", got.Cap)
	}
}

func TestContext_GetStroke_Legacy(t *testing.T) {
	ctx := NewContext(100, 100)

	// Set using legacy methods
	ctx.SetLineWidth(5)
	ctx.SetLineCap(LineCapSquare)
	ctx.SetLineJoin(LineJoinBevel)
	ctx.SetMiterLimit(8)

	got := ctx.GetStroke()
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
		ctx := NewContext(100, 100)
		ctx.SetDash(5, 3)

		if !ctx.IsDashed() {
			t.Error("IsDashed() = false, want true")
		}

		stroke := ctx.GetStroke()
		if stroke.Dash == nil {
			t.Fatal("GetStroke().Dash = nil")
		}
		if len(stroke.Dash.Array) != 2 {
			t.Errorf("Dash.Array length = %d, want 2", len(stroke.Dash.Array))
		}
	})

	t.Run("complex dash pattern", func(t *testing.T) {
		ctx := NewContext(100, 100)
		ctx.SetDash(10, 5, 2, 5)

		stroke := ctx.GetStroke()
		if stroke.Dash == nil {
			t.Fatal("GetStroke().Dash = nil")
		}
		if len(stroke.Dash.Array) != 4 {
			t.Errorf("Dash.Array length = %d, want 4", len(stroke.Dash.Array))
		}
	})

	t.Run("empty dash clears pattern", func(t *testing.T) {
		ctx := NewContext(100, 100)
		ctx.SetDash(5, 3)
		ctx.SetDash() // clear

		if ctx.IsDashed() {
			t.Error("IsDashed() = true after clear")
		}
	})

	t.Run("all zeros clears pattern", func(t *testing.T) {
		ctx := NewContext(100, 100)
		ctx.SetDash(5, 3)
		ctx.SetDash(0, 0, 0)

		if ctx.IsDashed() {
			t.Error("IsDashed() = true after all zeros")
		}
	})
}

func TestContext_SetDashOffset(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetDash(5, 3)
	ctx.SetDashOffset(2)

	stroke := ctx.GetStroke()
	if stroke.Dash == nil {
		t.Fatal("GetStroke().Dash = nil")
	}
	if stroke.Dash.Offset != 2 {
		t.Errorf("Dash.Offset = %v, want 2", stroke.Dash.Offset)
	}
}

func TestContext_SetDashOffset_NoDash(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetDashOffset(2) // No dash set - should not panic

	// Should still be solid line
	if ctx.IsDashed() {
		t.Error("IsDashed() = true, want false")
	}
}

func TestContext_ClearDash(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetDash(5, 3)

	if !ctx.IsDashed() {
		t.Fatal("IsDashed() = false before clear")
	}

	ctx.ClearDash()

	if ctx.IsDashed() {
		t.Error("IsDashed() = true after ClearDash()")
	}
}

func TestContext_ClearDash_NoStroke(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.ClearDash() // Should not panic
}

func TestContext_IsDashed(t *testing.T) {
	ctx := NewContext(100, 100)

	if ctx.IsDashed() {
		t.Error("IsDashed() = true for new context")
	}

	ctx.SetDash(5, 3)
	if !ctx.IsDashed() {
		t.Error("IsDashed() = false after SetDash")
	}

	ctx.ClearDash()
	if ctx.IsDashed() {
		t.Error("IsDashed() = true after ClearDash")
	}
}

func TestContext_StrokeWithDash(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetRGB(1, 0, 0)
	ctx.SetLineWidth(2)
	ctx.SetDash(5, 3)

	ctx.DrawLine(10, 50, 90, 50)
	ctx.Stroke()

	// Verify stroke was executed (basic sanity check)
	// The actual dashing implementation is in the renderer
}

func TestContext_SetStroke_UpdatesLegacyFields(t *testing.T) {
	ctx := NewContext(100, 100)

	stroke := Stroke{
		Width:      7,
		Cap:        LineCapSquare,
		Join:       LineJoinBevel,
		MiterLimit: 5,
	}
	ctx.SetStroke(stroke)

	// Legacy fields should be updated for backward compatibility
	if ctx.paint.LineWidth != 7 {
		t.Errorf("paint.LineWidth = %v, want 7", ctx.paint.LineWidth)
	}
	if ctx.paint.LineCap != LineCapSquare {
		t.Errorf("paint.LineCap = %v, want LineCapSquare", ctx.paint.LineCap)
	}
	if ctx.paint.LineJoin != LineJoinBevel {
		t.Errorf("paint.LineJoin = %v, want LineJoinBevel", ctx.paint.LineJoin)
	}
	if ctx.paint.MiterLimit != 5 {
		t.Errorf("paint.MiterLimit = %v, want 5", ctx.paint.MiterLimit)
	}
}

func TestContext_DashedLinePreset(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetStroke(DashedStroke(10, 5))

	if !ctx.IsDashed() {
		t.Error("IsDashed() = false for DashedStroke")
	}

	stroke := ctx.GetStroke()
	if stroke.Width != 1.0 {
		t.Errorf("Width = %v, want 1.0", stroke.Width)
	}
}

func TestContext_DottedLinePreset(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.SetStroke(DottedStroke())

	if !ctx.IsDashed() {
		t.Error("IsDashed() = false for DottedStroke")
	}

	stroke := ctx.GetStroke()
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
