package gg

import (
	"image"
	"testing"
)

func TestPathBounds_Empty(t *testing.T) {
	p := NewPath()
	if b := p.Bounds(); !b.Empty() {
		t.Errorf("empty path bounds should be empty, got %v", b)
	}
}

func TestPathBounds_SinglePoint(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	b := p.Bounds()
	if b.Min.X != 10 || b.Min.Y != 20 {
		t.Errorf("single point bounds min = (%d,%d), want (10,20)", b.Min.X, b.Min.Y)
	}
}

func TestPathBounds_Line(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 10)
	p.LineTo(50, 60)
	b := p.Bounds()
	expected := image.Rect(10, 10, 50, 60)
	if b != expected {
		t.Errorf("line bounds = %v, want %v", b, expected)
	}
}

func TestPathBounds_Rectangle(t *testing.T) {
	p := NewPath()
	p.MoveTo(100, 200)
	p.LineTo(300, 200)
	p.LineTo(300, 400)
	p.LineTo(100, 400)
	p.Close()
	b := p.Bounds()
	expected := image.Rect(100, 200, 300, 400)
	if b != expected {
		t.Errorf("rect bounds = %v, want %v", b, expected)
	}
}

func TestPathBounds_CubicBezier(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.CubicTo(100, -50, 200, 150, 300, 100)
	b := p.Bounds()
	// Control points expand bounds: min Y = -50, max X = 300, max Y = 150
	if b.Min.Y > -50 {
		t.Errorf("cubic bounds minY = %d, should be <= -50", b.Min.Y)
	}
	if b.Max.X < 300 {
		t.Errorf("cubic bounds maxX = %d, should be >= 300", b.Max.X)
	}
	if b.Max.Y < 150 {
		t.Errorf("cubic bounds maxY = %d, should be >= 150", b.Max.Y)
	}
}

func TestPathBounds_QuadraticBezier(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(50, -30, 100, 0)
	b := p.Bounds()
	if b.Min.Y > -30 {
		t.Errorf("quad bounds minY = %d, should be <= -30", b.Min.Y)
	}
}

func TestPathBounds_ClearResets(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 10)
	p.LineTo(100, 100)
	p.Clear()
	if b := p.Bounds(); !b.Empty() {
		t.Errorf("cleared path bounds should be empty, got %v", b)
	}
}

func TestPathBounds_IncrementalAccuracy(t *testing.T) {
	// Build path incrementally — bounds should match
	p := NewPath()
	p.MoveTo(5.5, 3.2)
	p.LineTo(10.8, 7.9)
	p.LineTo(2.1, 15.6)
	b := p.Bounds()
	// Floor min, ceil max
	if b.Min.X != 2 {
		t.Errorf("minX = %d, want 2 (floor of 2.1)", b.Min.X)
	}
	if b.Min.Y != 3 {
		t.Errorf("minY = %d, want 3 (floor of 3.2)", b.Min.Y)
	}
	if b.Max.X != 11 {
		t.Errorf("maxX = %d, want 11 (ceil of 10.8)", b.Max.X)
	}
	if b.Max.Y != 16 {
		t.Errorf("maxY = %d, want 16 (ceil of 15.6)", b.Max.Y)
	}
}

func TestContextFrameDamage_Empty(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	if d := dc.FrameDamage(); !d.Empty() {
		t.Errorf("new context frameDamage should be empty, got %v", d)
	}
}

func TestContextFrameDamage_AfterFill(t *testing.T) {
	dc := NewContext(200, 200)
	defer dc.Close()

	dc.DrawCircle(50, 50, 20)
	dc.Fill()

	d := dc.FrameDamage()
	if d.Empty() {
		t.Fatal("frameDamage should not be empty after Fill")
	}
	// Circle at (50,50) r=20 → bounds ~(30,30)-(70,70)
	if d.Min.X > 30 || d.Min.Y > 30 {
		t.Errorf("damage min too large: %v, circle at (50,50) r=20", d)
	}
	if d.Max.X < 70 || d.Max.Y < 70 {
		t.Errorf("damage max too small: %v, circle at (50,50) r=20", d)
	}
}

func TestContextFrameDamage_MultipleFillsUnion(t *testing.T) {
	dc := NewContext(400, 400)
	defer dc.Close()

	// First fill: top-left
	dc.DrawRectangle(10, 10, 30, 30)
	dc.Fill()

	// Second fill: bottom-right
	dc.DrawRectangle(300, 300, 50, 50)
	dc.Fill()

	d := dc.FrameDamage()
	// Union should span from (10,10) to (350,350)
	if d.Min.X > 10 || d.Min.Y > 10 {
		t.Errorf("damage should start at (10,10), got min %v", d.Min)
	}
	if d.Max.X < 350 || d.Max.Y < 350 {
		t.Errorf("damage should extend to (350,350), got max %v", d.Max)
	}
}

func TestContextFrameDamage_ResetClears(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	dc.DrawRectangle(10, 10, 50, 50)
	dc.Fill()
	dc.ResetFrameDamage()

	if d := dc.FrameDamage(); !d.Empty() {
		t.Errorf("after ResetFrameDamage, should be empty, got %v", d)
	}
}
