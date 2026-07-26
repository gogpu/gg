package gg

import (
	"testing"
)

// TestDualPaint_FillStroke_DifferentColors verifies that SetFillBrush and
// SetStrokeBrush produce different colors in the same draw operation.
func TestDualPaint_FillStroke_DifferentColors(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)

	// Set different colors for fill and stroke
	dc.SetFillBrush(Solid(Red))
	dc.SetStrokeBrush(Solid(Blue))
	dc.SetLineWidth(5)

	dc.DrawRectangle(20, 20, 60, 60)
	_ = dc.FillPreserve()
	_ = dc.Stroke()

	// Center should be red (fill)
	pFill := dc.pixmap.GetPixel(50, 50)
	if pixelByte(pFill.R) < 200 || pixelByte(pFill.B) > 50 {
		t.Errorf("Fill center: expected red, got RGB(%d,%d,%d)",
			pixelByte(pFill.R), pixelByte(pFill.G), pixelByte(pFill.B))
	}

	// Edge should be blue (stroke) — check at x=20 (left border, stroke width=5)
	pStroke := dc.pixmap.GetPixel(21, 50)
	if pixelByte(pStroke.B) < 150 || pixelByte(pStroke.R) > 100 {
		t.Errorf("Stroke edge: expected blue, got RGB(%d,%d,%d)",
			pixelByte(pStroke.R), pixelByte(pStroke.G), pixelByte(pStroke.B))
	}
}

// TestDualPaint_SetColor_SetsBoth verifies backward compat: SetColor sets both.
func TestDualPaint_SetColor_SetsBoth(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(0.5, 0, 0) // should set BOTH fill and stroke

	dc.ClearWithColor(White)
	dc.DrawRectangle(20, 20, 60, 60)
	_ = dc.FillPreserve()
	_ = dc.Stroke()

	// Both fill and stroke should be the same dark red
	pFill := dc.pixmap.GetPixel(50, 50)
	pStroke := dc.pixmap.GetPixel(21, 50)

	if abs(float64(pixelByte(pFill.R))-float64(pixelByte(pStroke.R))) > 10 {
		t.Errorf("SetRGB should set both: fill R=%d, stroke R=%d",
			pixelByte(pFill.R), pixelByte(pStroke.R))
	}
}

// TestDualPaint_PushPop_SavesRestore verifies Push/Pop saves and restores paint.
func TestDualPaint_PushPop_SavesRestore(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	// Set initial colors
	dc.SetFillBrush(Solid(Red))
	dc.SetStrokeBrush(Solid(Blue))

	dc.Push()

	// Change colors inside Push
	dc.SetFillBrush(Solid(Green))
	dc.SetStrokeBrush(Solid(White))

	// Verify inner state
	innerFill, _ := dc.paint.FillSolidColor()
	if pixelByte(innerFill.G) < 200 {
		t.Errorf("Inside Push: fill should be green, got R=%.2f G=%.2f B=%.2f",
			innerFill.R, innerFill.G, innerFill.B)
	}

	dc.Pop()

	// Verify outer state restored
	outerFill, _ := dc.paint.FillSolidColor()
	outerStroke, _ := dc.paint.StrokeSolidColor()

	if pixelByte(outerFill.R) < 200 {
		t.Errorf("After Pop: fill should be red, got R=%.2f G=%.2f B=%.2f",
			outerFill.R, outerFill.G, outerFill.B)
	}
	if pixelByte(outerStroke.B) < 200 {
		t.Errorf("After Pop: stroke should be blue, got R=%.2f G=%.2f B=%.2f",
			outerStroke.R, outerStroke.G, outerStroke.B)
	}
}

// TestDualPaint_PushPop_BlendMode verifies Push/Pop saves blend mode too.
func TestDualPaint_PushPop_BlendMode(t *testing.T) {
	dc := NewContext(50, 50)
	defer func() { _ = dc.Close() }()

	dc.SetBlendMode(BlendDarken) // internal value 17
	dc.Push()
	dc.SetBlendMode(BlendLighten) // internal value 18

	// Verify changed inside Push
	dc.ClearWithColor(White)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 50, 50)
	_ = dc.Fill()

	dc.Pop()

	// After Pop: verify blend mode was restored by drawing with it.
	// Darken on white bg = source color (min per channel).
	dc.ClearWithColor(White)
	dc.SetRGB(0.5, 0, 0)
	dc.DrawRectangle(0, 0, 50, 50)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(25, 25)
	// Darken(white, 0.5,0,0) = 0.5,0,0 — red channel ~128
	if pixelByte(p.R) < 100 || pixelByte(p.R) > 160 {
		t.Errorf("After Pop: darken blend not restored, R=%d (expected ~128)", pixelByte(p.R))
	}
}

// TestDualPaint_GradientFill_SolidStroke verifies gradient fill with solid stroke.
func TestDualPaint_GradientFill_SolidStroke(t *testing.T) {
	dc := NewContext(200, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)

	grad := NewLinearGradientBrush(20, 0, 180, 0)
	grad.AddColorStop(0, Red)
	grad.AddColorStop(1, Blue)
	dc.SetFillBrush(grad)
	dc.SetStrokeBrush(Solid(Black))
	dc.SetLineWidth(3)

	dc.DrawRectangle(20, 20, 160, 60)
	_ = dc.FillPreserve()
	_ = dc.Stroke()

	// Left fill should be reddish
	pLeft := dc.pixmap.GetPixel(30, 50)
	if pixelByte(pLeft.R) < 150 {
		t.Errorf("Gradient left: expected red, got R=%d", pixelByte(pLeft.R))
	}

	// Right fill should be bluish
	pRight := dc.pixmap.GetPixel(170, 50)
	if pixelByte(pRight.B) < 150 {
		t.Errorf("Gradient right: expected blue, got B=%d", pixelByte(pRight.B))
	}

	// Stroke should be black — check at the very edge of the border
	pEdge := dc.pixmap.GetPixel(19, 50)
	if pixelByte(pEdge.R) > 80 || pixelByte(pEdge.G) > 80 || pixelByte(pEdge.B) > 80 {
		t.Errorf("Stroke edge: expected dark/black, got RGB(%d,%d,%d)",
			pixelByte(pEdge.R), pixelByte(pEdge.G), pixelByte(pEdge.B))
	}
}

// TestDualPaint_Circle_FillAndStroke verifies circle with dual colors.
func TestDualPaint_Circle_FillAndStroke(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)

	dc.SetFillBrush(SolidHex("#3498db"))   // blue fill
	dc.SetStrokeBrush(SolidHex("#2c3e50")) // dark stroke
	dc.SetLineWidth(4)

	dc.DrawCircle(50, 50, 30)
	_ = dc.FillPreserve()
	_ = dc.Stroke()

	// Center = blue fill
	pCenter := dc.pixmap.GetPixel(50, 50)
	if pixelByte(pCenter.B) < 150 {
		t.Errorf("Circle center: expected blue, got RGB(%d,%d,%d)",
			pixelByte(pCenter.R), pixelByte(pCenter.G), pixelByte(pCenter.B))
	}

	// Just outside circle = white
	pOut := dc.pixmap.GetPixel(5, 5)
	if pixelByte(pOut.R) < 240 {
		t.Errorf("Outside circle: expected white, got R=%d", pixelByte(pOut.R))
	}
}

// TestDualPaint_SetFillBrush_DoesNotAffectStroke verifies independence.
func TestDualPaint_SetFillBrush_DoesNotAffectStroke(t *testing.T) {
	dc := NewContext(50, 50)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 1, 1) // white for both
	dc.SetFillBrush(Solid(Red))

	// Stroke should still be white
	strokeColor, _ := dc.paint.StrokeSolidColor()
	if pixelByte(strokeColor.R) < 240 || pixelByte(strokeColor.G) < 240 || pixelByte(strokeColor.B) < 240 {
		t.Errorf("SetFillBrush should not affect stroke: got RGB(%.2f,%.2f,%.2f)",
			strokeColor.R, strokeColor.G, strokeColor.B)
	}

	// Fill should be red
	fillColor, _ := dc.paint.FillSolidColor()
	if pixelByte(fillColor.R) < 240 || pixelByte(fillColor.G) > 10 {
		t.Errorf("Fill should be red: got RGB(%.2f,%.2f,%.2f)",
			fillColor.R, fillColor.G, fillColor.B)
	}
}

// TestDualPaint_NestedPushPop verifies nested Push/Pop with paint changes.
func TestDualPaint_NestedPushPop(t *testing.T) {
	dc := NewContext(50, 50)
	defer func() { _ = dc.Close() }()

	dc.SetFillBrush(Solid(Red))

	dc.Push()
	dc.SetFillBrush(Solid(Green))

	dc.Push()
	dc.SetFillBrush(Solid(Blue))

	// Inner: blue
	inner, _ := dc.paint.FillSolidColor()
	if pixelByte(inner.B) < 200 {
		t.Error("Nested inner: should be blue")
	}

	dc.Pop()
	// Mid: green
	mid, _ := dc.paint.FillSolidColor()
	if pixelByte(mid.G) < 200 {
		t.Error("Nested mid: should be green")
	}

	dc.Pop()
	// Outer: red
	outer, _ := dc.paint.FillSolidColor()
	if pixelByte(outer.R) < 200 {
		t.Error("Nested outer: should be red")
	}
}
