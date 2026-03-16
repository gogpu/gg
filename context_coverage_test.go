package gg

import (
	"testing"
)

// --- Context creation and Close ---

func TestNewContextClose(t *testing.T) {
	dc := NewContext(100, 100)
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}
	err := dc.Close()
	if err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestNewContextDoubleClose(t *testing.T) {
	dc := NewContext(100, 100)
	_ = dc.Close()
	err := dc.Close()
	if err != nil {
		t.Errorf("double Close() = %v, want nil", err)
	}
}

func TestNewContextWithDeviceScale(t *testing.T) {
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	if dc.deviceScale != 2.0 {
		t.Errorf("deviceScale = %f, want 2.0", dc.deviceScale)
	}
	// Physical pixel dimensions should be doubled
	if dc.pixmap.Width() != 200 || dc.pixmap.Height() != 200 {
		t.Errorf("pixmap = %dx%d, want 200x200", dc.pixmap.Width(), dc.pixmap.Height())
	}
}

// --- Drawing operations ---

func TestContextDrawRectangle(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(10, 10, 80, 80)
	dc.Fill()

	// Center should be red
	center := dc.pixmap.GetPixel(50, 50)
	if center.R < 0.9 {
		t.Errorf("center R = %f, want >= 0.9", center.R)
	}
}

func TestContextDrawCircle(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 1)
	dc.DrawCircle(50, 50, 30)
	dc.Fill()

	// Center should be blue
	center := dc.pixmap.GetPixel(50, 50)
	if center.B < 0.8 {
		t.Errorf("center B = %f, want >= 0.8", center.B)
	}

	// Corner should be white
	corner := dc.pixmap.GetPixel(5, 5)
	if corner.R < 0.9 || corner.G < 0.9 || corner.B < 0.9 {
		t.Errorf("corner = %+v, want white", corner)
	}
}

func TestContextDrawEllipse(t *testing.T) {
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 1, 0)
	dc.DrawEllipse(100, 100, 60, 30)
	dc.Fill()

	// Center should be green
	center := dc.pixmap.GetPixel(100, 100)
	if center.G < 0.8 {
		t.Errorf("center G = %f, want >= 0.8", center.G)
	}
}

func TestContextDrawRoundedRectangle(t *testing.T) {
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(1, 0, 0)
	dc.DrawRoundedRectangle(20, 20, 160, 160, 20)
	dc.Fill()

	center := dc.pixmap.GetPixel(100, 100)
	if center.R < 0.8 {
		t.Errorf("center R = %f, want >= 0.8", center.R)
	}
}

// --- Push/Pop ---

func TestContextPushPop(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.Push()
	dc.Translate(50, 50)
	dc.Pop()

	// After pop, transform should be restored to identity
	m := dc.GetTransform()
	id := Identity()
	if m != id {
		t.Errorf("after Pop, matrix = %+v, want %+v", m, id)
	}
}

func TestContextPushPopTransform(t *testing.T) {
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.Push()
	dc.Translate(50, 50)
	dc.Scale(2, 2)
	dc.Pop()

	// After pop, transform should be identity
	m := dc.GetTransform()
	id := Identity()
	if m != id {
		t.Errorf("after Pop, matrix = %+v, want %+v", m, id)
	}
}

// --- Color API ---

func TestContextSetRGB(t *testing.T) {
	dc := NewContext(10, 10)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(0.5, 0.6, 0.7)
	dc.DrawRectangle(0, 0, 10, 10)
	dc.Fill()

	p := dc.pixmap.GetPixel(5, 5)
	if abs(p.R-0.5) > 0.05 || abs(p.G-0.6) > 0.05 || abs(p.B-0.7) > 0.05 {
		t.Errorf("pixel = (%f,%f,%f), want ~(0.5,0.6,0.7)", p.R, p.G, p.B)
	}
}

func TestContextSetRGBA(t *testing.T) {
	dc := NewContext(10, 10)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGBA(1, 0, 0, 0.5)
	dc.DrawRectangle(0, 0, 10, 10)
	dc.Fill()

	p := dc.pixmap.GetPixel(5, 5)
	// Semi-transparent red over white
	if p.R < 0.8 || p.G > 0.6 || p.B > 0.6 {
		t.Errorf("pixel = (%f,%f,%f), want pinkish", p.R, p.G, p.B)
	}
}

func TestContextSetHexColor(t *testing.T) {
	dc := NewContext(10, 10)
	defer func() { _ = dc.Close() }()

	dc.SetHexColor("#FF0000")
	dc.DrawRectangle(0, 0, 10, 10)
	dc.Fill()

	p := dc.pixmap.GetPixel(5, 5)
	if p.R < 0.9 || p.G > 0.1 || p.B > 0.1 {
		t.Errorf("pixel = (%f,%f,%f), want red", p.R, p.G, p.B)
	}
}

// --- Transform API ---

func TestContextIdentity(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.Translate(50, 50)
	dc.Identity()

	m := dc.GetTransform()
	id := Identity()
	if m != id {
		t.Errorf("after Identity, matrix = %+v, want %+v", m, id)
	}
}

func TestContextRotate(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.Rotate(0) // zero rotation should be identity
	m := dc.GetTransform()
	if abs(m.A-1) > 0.01 || abs(m.E-1) > 0.01 {
		t.Errorf("after Rotate(0), matrix = %+v, want identity", m)
	}
}

// --- Path API ---

func TestContextNewSubPath(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.MoveTo(10, 10)
	dc.LineTo(50, 10)
	dc.NewSubPath()
	dc.MoveTo(60, 60)
	dc.LineTo(90, 90)
	dc.ClosePath()

	elems := dc.path.Elements()
	if len(elems) == 0 {
		t.Error("expected path elements after NewSubPath")
	}
}

func TestContextClearPath(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.MoveTo(10, 10)
	dc.LineTo(50, 50)
	dc.ClearPath()

	elems := dc.path.Elements()
	if len(elems) != 0 {
		t.Errorf("after ClearPath, elements = %d, want 0", len(elems))
	}
}

// --- Stroke API ---

func TestContextStrokeSimple(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(1, 0, 0)
	dc.SetLineWidth(4)
	dc.MoveTo(10, 50)
	dc.LineTo(90, 50)
	dc.Stroke()

	// On the line
	p := dc.pixmap.GetPixel(50, 50)
	if p.R < 0.5 {
		t.Errorf("stroke pixel R = %f, want >= 0.5", p.R)
	}
}

func TestContextSetLineWidth(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetLineWidth(5.0)
	if dc.paint.LineWidth != 5.0 {
		t.Errorf("LineWidth = %f, want 5.0", dc.paint.LineWidth)
	}
}

func TestContextSetLineCap(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetLineCap(LineCapRound)
	if dc.paint.LineCap != LineCapRound {
		t.Errorf("LineCap = %d, want LineCapRound", dc.paint.LineCap)
	}
}

func TestContextSetLineJoin(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetLineJoin(LineJoinBevel)
	if dc.paint.LineJoin != LineJoinBevel {
		t.Errorf("LineJoin = %d, want LineJoinBevel", dc.paint.LineJoin)
	}
}

// --- Dash API ---

func TestContextSetDashCoverage(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetDash(10, 5)
	if !dc.paint.IsDashed() {
		t.Error("expected dashed after SetDash")
	}
}

func TestContextClearDashCoverage(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetDash(10, 5)
	dc.ClearDash()
	if dc.paint.IsDashed() {
		t.Error("expected no dash after ClearDash")
	}
}

// --- FillRule ---

func TestContextSetFillRule(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetFillRule(FillRuleEvenOdd)
	if dc.paint.FillRule != FillRuleEvenOdd {
		t.Errorf("FillRule = %d, want FillRuleEvenOdd", dc.paint.FillRule)
	}
}

// --- Multiple shapes in one context ---

func TestContextMultipleShapes(t *testing.T) {
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)

	// Red rectangle
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(10, 10, 80, 80)
	dc.Fill()

	// Blue circle
	dc.SetRGB(0, 0, 1)
	dc.DrawCircle(150, 150, 30)
	dc.Fill()

	// Check red rect center
	r := dc.pixmap.GetPixel(50, 50)
	if r.R < 0.8 {
		t.Errorf("red rect center R = %f, want >= 0.8", r.R)
	}

	// Check blue circle center
	b := dc.pixmap.GetPixel(150, 150)
	if b.B < 0.8 {
		t.Errorf("blue circle center B = %f, want >= 0.8", b.B)
	}
}

// --- DeviceScale rendering ---

func TestContextDeviceScaleRendering(t *testing.T) {
	// At 2x scale, a 100x100 logical context = 200x200 physical pixels
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(25, 25, 50, 50)
	dc.Fill()

	// Physical pixel at (100, 100) = logical (50, 50) should be red
	center := dc.pixmap.GetPixel(100, 100)
	if center.R < 0.8 {
		t.Errorf("2x scale center R = %f, want >= 0.8", center.R)
	}

	// Physical pixel at (10, 10) = logical (5, 5) should be white
	corner := dc.pixmap.GetPixel(10, 10)
	if corner.R < 0.9 || corner.G < 0.9 || corner.B < 0.9 {
		t.Errorf("2x scale corner = %+v, want white", corner)
	}
}

// --- Image output ---

func TestContextImage(t *testing.T) {
	dc := NewContext(50, 50)
	defer func() { _ = dc.Close() }()

	img := dc.Image()
	if img == nil {
		t.Fatal("Image() returned nil")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 50 || bounds.Dy() != 50 {
		t.Errorf("Image bounds = %v, want 50x50", bounds)
	}
}

// --- FlushGPU (without GPU) ---

func TestContextFlushGPUNoAccelerator(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	err := dc.FlushGPU()
	if err != nil {
		t.Errorf("FlushGPU() without GPU = %v, want nil", err)
	}
}

// --- TextMode ---

func TestContextSetTextMode(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetTextMode(TextModeVector)
	if dc.textMode != TextModeVector {
		t.Errorf("textMode = %d, want TextModeVector", dc.textMode)
	}

	dc.SetTextMode(TextModeBitmap)
	if dc.textMode != TextModeBitmap {
		t.Errorf("textMode = %d, want TextModeBitmap", dc.textMode)
	}
}

// --- Brush API ---

func TestContextSetFillBrushCoverage(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetFillBrush(Solid(Red))
	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	if p.R < 0.8 {
		t.Errorf("fill brush red R = %f, want >= 0.8", p.R)
	}
}

func TestContextSetStrokeBrushCoverage(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetStrokeBrush(Solid(Blue))
	dc.SetLineWidth(4)
	dc.MoveTo(10, 50)
	dc.LineTo(90, 50)
	dc.Stroke()

	p := dc.pixmap.GetPixel(50, 50)
	if p.B < 0.5 {
		t.Errorf("stroke brush blue B = %f, want >= 0.5", p.B)
	}
}
