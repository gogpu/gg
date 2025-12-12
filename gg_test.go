package gg

import (
	"math"
	"testing"
)

func TestNewContext(t *testing.T) {
	ctx := NewContext(100, 100)
	if ctx == nil {
		t.Fatal("NewContext returned nil")
	}
	if ctx.Width() != 100 {
		t.Errorf("Width = %d, want 100", ctx.Width())
	}
	if ctx.Height() != 100 {
		t.Errorf("Height = %d, want 100", ctx.Height())
	}
}

func TestClear(t *testing.T) {
	ctx := NewContext(10, 10)
	ctx.ClearWithColor(Red)

	// Check a pixel
	pixel := ctx.pixmap.GetPixel(5, 5)
	if pixel.R != 1.0 || pixel.G != 0.0 || pixel.B != 0.0 {
		t.Errorf("Pixel color = %+v, want Red", pixel)
	}
}

func TestDrawRectangle(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.ClearWithColor(White)
	ctx.SetRGB(1, 0, 0)
	ctx.DrawRectangle(10, 10, 50, 50)
	ctx.Fill()

	// Check pixel inside rectangle (should be red)
	pixel := ctx.pixmap.GetPixel(30, 30)
	if pixel.R < 0.9 { // Allow some tolerance
		t.Errorf("Pixel inside rectangle not red: %+v", pixel)
	}

	// Check pixel outside rectangle (should still be white)
	pixel = ctx.pixmap.GetPixel(5, 5)
	if pixel.R < 0.9 || pixel.G < 0.9 || pixel.B < 0.9 {
		t.Errorf("Pixel outside rectangle not white: %+v", pixel)
	}
}

func TestDrawCircle(t *testing.T) {
	ctx := NewContext(100, 100)
	ctx.ClearWithColor(White)
	ctx.SetRGB(0, 0, 1)
	ctx.DrawCircle(50, 50, 25)
	ctx.Fill()

	// Check pixel inside circle
	pixel := ctx.pixmap.GetPixel(50, 50)
	if pixel.B < 0.9 {
		t.Errorf("Pixel at center not blue: %+v", pixel)
	}

	// Check pixel outside circle (should still be white)
	pixel = ctx.pixmap.GetPixel(10, 10)
	if pixel.R < 0.9 || pixel.G < 0.9 || pixel.B < 0.9 {
		t.Errorf("Pixel outside circle not white: %+v", pixel)
	}
}

func TestTransformations(t *testing.T) {
	ctx := NewContext(100, 100)

	// Push/Pop
	ctx.Push()
	ctx.Translate(50, 50)
	if ctx.matrix.C != 50 || ctx.matrix.F != 50 {
		t.Errorf("Translate failed: %+v", ctx.matrix)
	}
	ctx.Pop()
	if !ctx.matrix.IsIdentity() {
		t.Errorf("Pop didn't restore identity: %+v", ctx.matrix)
	}

	// Rotate
	ctx.Rotate(math.Pi / 2)
	if math.Abs(ctx.matrix.A) > 0.001 { // Should be ~0
		t.Errorf("Rotation incorrect: %+v", ctx.matrix)
	}

	// Scale
	ctx.Identity()
	ctx.Scale(2, 3)
	if ctx.matrix.A != 2 || ctx.matrix.E != 3 {
		t.Errorf("Scale failed: %+v", ctx.matrix)
	}
}

func TestColors(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want RGBA
	}{
		{"red", "#FF0000", RGB(1, 0, 0)},
		{"green", "#00FF00", RGB(0, 1, 0)},
		{"blue", "#0000FF", RGB(0, 0, 1)},
		{"short", "#F00", RGB(1, 0, 0)},
		{"with alpha", "#FF000080", RGBA2(1, 0, 0, 0.5019607843137255)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hex(tt.hex)
			if math.Abs(got.R-tt.want.R) > 0.01 ||
				math.Abs(got.G-tt.want.G) > 0.01 ||
				math.Abs(got.B-tt.want.B) > 0.01 ||
				math.Abs(got.A-tt.want.A) > 0.01 {
				t.Errorf("Hex(%q) = %+v, want %+v", tt.hex, got, tt.want)
			}
		})
	}
}

func TestHSL(t *testing.T) {
	// Test red
	red := HSL(0, 1, 0.5)
	if math.Abs(red.R-1) > 0.01 || math.Abs(red.G) > 0.01 || math.Abs(red.B) > 0.01 {
		t.Errorf("HSL(0, 1, 0.5) = %+v, want red", red)
	}

	// Test green
	green := HSL(120, 1, 0.5)
	if math.Abs(green.G-1) > 0.01 || math.Abs(green.R) > 0.01 || math.Abs(green.B) > 0.01 {
		t.Errorf("HSL(120, 1, 0.5) = %+v, want green", green)
	}
}

func TestPoint(t *testing.T) {
	p1 := Pt(3, 4)
	p2 := Pt(0, 0)

	// Distance
	dist := p1.Distance(p2)
	if math.Abs(dist-5) > 0.001 {
		t.Errorf("Distance = %f, want 5", dist)
	}

	// Add
	p3 := p1.Add(Pt(1, 1))
	if p3.X != 4 || p3.Y != 5 {
		t.Errorf("Add = %+v, want (4, 5)", p3)
	}

	// Normalize
	p4 := p1.Normalize()
	length := p4.Length()
	if math.Abs(length-1) > 0.001 {
		t.Errorf("Normalize length = %f, want 1", length)
	}
}

func TestMatrix(t *testing.T) {
	// Multiply
	m1 := Translate(10, 20)
	m2 := Scale(2, 3)
	m3 := m1.Multiply(m2)

	p := m3.TransformPoint(Pt(5, 5))
	if p.X != 20 || p.Y != 35 {
		t.Errorf("Transform = %+v, want (20, 35)", p)
	}

	// Invert
	m := Translate(10, 20)
	inv := m.Invert()
	identity := m.Multiply(inv)

	if math.Abs(identity.A-1) > 0.001 ||
		math.Abs(identity.E-1) > 0.001 ||
		math.Abs(identity.C) > 0.001 ||
		math.Abs(identity.F) > 0.001 {
		t.Errorf("Invert failed: %+v", identity)
	}
}

func BenchmarkDrawCircle(b *testing.B) {
	ctx := NewContext(512, 512)
	ctx.SetRGB(1, 0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.ClearPath()
		ctx.DrawCircle(256, 256, 100)
		ctx.Fill()
	}
}

func BenchmarkDrawRectangle(b *testing.B) {
	ctx := NewContext(512, 512)
	ctx.SetRGB(0, 1, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.ClearPath()
		ctx.DrawRectangle(100, 100, 300, 300)
		ctx.Fill()
	}
}
