package gg

import (
	"math"
	"testing"
)

func TestNewContext(t *testing.T) {
	dc := NewContext(100, 100)
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}
	if dc.Width() != 100 {
		t.Errorf("Width = %d, want 100", dc.Width())
	}
	if dc.Height() != 100 {
		t.Errorf("Height = %d, want 100", dc.Height())
	}
}

func TestClear(t *testing.T) {
	dc := NewContext(10, 10)
	dc.ClearWithColor(Red)

	// Check a pixel
	pixel := dc.pixmap.GetPixel(5, 5)
	if pixel.R != 1.0 || pixel.G != 0.0 || pixel.B != 0.0 {
		t.Errorf("Pixel color = %+v, want Red", pixel)
	}
}

func TestDrawRectangle(t *testing.T) {
	dc := NewContext(100, 100)
	dc.ClearWithColor(White)
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(10, 10, 50, 50)
	dc.Fill()

	// Check pixel inside rectangle (should be red)
	pixel := dc.pixmap.GetPixel(30, 30)
	if pixel.R < 0.9 { // Allow some tolerance
		t.Errorf("Pixel inside rectangle not red: %+v", pixel)
	}

	// Check pixel outside rectangle (should still be white)
	pixel = dc.pixmap.GetPixel(5, 5)
	if pixel.R < 0.9 || pixel.G < 0.9 || pixel.B < 0.9 {
		t.Errorf("Pixel outside rectangle not white: %+v", pixel)
	}
}

func TestDrawCircle(t *testing.T) {
	dc := NewContext(100, 100)
	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 1)
	dc.DrawCircle(50, 50, 25)
	dc.Fill()

	// Check pixel inside circle
	pixel := dc.pixmap.GetPixel(50, 50)
	if pixel.B < 0.9 {
		t.Errorf("Pixel at center not blue: %+v", pixel)
	}

	// Check pixel outside circle (should still be white)
	pixel = dc.pixmap.GetPixel(10, 10)
	if pixel.R < 0.9 || pixel.G < 0.9 || pixel.B < 0.9 {
		t.Errorf("Pixel outside circle not white: %+v", pixel)
	}
}

func TestTransformations(t *testing.T) {
	dc := NewContext(100, 100)

	// Push/Pop
	dc.Push()
	dc.Translate(50, 50)
	if dc.matrix.C != 50 || dc.matrix.F != 50 {
		t.Errorf("Translate failed: %+v", dc.matrix)
	}
	dc.Pop()
	if !dc.matrix.IsIdentity() {
		t.Errorf("Pop didn't restore identity: %+v", dc.matrix)
	}

	// Rotate
	dc.Rotate(math.Pi / 2)
	if math.Abs(dc.matrix.A) > 0.001 { // Should be ~0
		t.Errorf("Rotation incorrect: %+v", dc.matrix)
	}

	// Scale
	dc.Identity()
	dc.Scale(2, 3)
	if dc.matrix.A != 2 || dc.matrix.E != 3 {
		t.Errorf("Scale failed: %+v", dc.matrix)
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

func TestContextTransform(t *testing.T) {
	dc := NewContext(100, 100)

	// Start with identity
	if !dc.matrix.IsIdentity() {
		t.Fatal("Initial matrix should be identity")
	}

	// Transform should multiply by the given matrix
	dc.Transform(Translate(10, 20))
	if dc.matrix.C != 10 || dc.matrix.F != 20 {
		t.Errorf("Transform(Translate) failed: got C=%v, F=%v, want C=10, F=20",
			dc.matrix.C, dc.matrix.F)
	}

	// Transform again - should compound the transformations
	dc.Transform(Scale(2, 3))
	// After T(10,20) * S(2,3): C stays 10, F stays 20, but A=2, E=3
	if dc.matrix.A != 2 || dc.matrix.E != 3 {
		t.Errorf("Transform(Scale) failed: got A=%v, E=%v, want A=2, E=3",
			dc.matrix.A, dc.matrix.E)
	}
}

func TestContextSetTransform(t *testing.T) {
	dc := NewContext(100, 100)

	// Apply some transformations first
	dc.Translate(50, 50)
	dc.Scale(2, 2)

	// SetTransform should replace completely
	customMatrix := Matrix{A: 1, B: 0.5, C: 100, D: -0.5, E: 1, F: 200}
	dc.SetTransform(customMatrix)

	if dc.matrix != customMatrix {
		t.Errorf("SetTransform failed: got %+v, want %+v", dc.matrix, customMatrix)
	}

	// SetTransform with identity should reset
	dc.SetTransform(Identity())
	if !dc.matrix.IsIdentity() {
		t.Errorf("SetTransform(Identity()) failed: got %+v", dc.matrix)
	}
}

func TestContextGetTransform(t *testing.T) {
	dc := NewContext(100, 100)

	// Apply some transformations
	dc.Translate(30, 40)
	dc.Scale(1.5, 2.5)

	// GetTransform should return current matrix
	got := dc.GetTransform()
	if got != dc.matrix {
		t.Errorf("GetTransform returned wrong matrix: got %+v, want %+v", got, dc.matrix)
	}

	// GetTransform should return a COPY, not a reference
	// Modifying the returned matrix should NOT affect the context
	gotCopy := dc.GetTransform()
	gotCopy.A = 999
	gotCopy.C = 888

	// Original context matrix should be unchanged
	if dc.matrix.A == 999 || dc.matrix.C == 888 {
		t.Error("GetTransform returned reference instead of copy - modifying it affected context")
	}
}

func TestTransformSetTransformIntegration(t *testing.T) {
	dc := NewContext(100, 100)

	// Build up a complex transform
	dc.Translate(50, 50)
	dc.Rotate(math.Pi / 4)
	dc.Scale(2, 2)

	// Save it
	saved := dc.GetTransform()

	// Reset and do something else
	dc.Identity()
	dc.Translate(10, 10)

	// Restore using SetTransform
	dc.SetTransform(saved)
	current := dc.GetTransform()

	if current != saved {
		t.Errorf("SetTransform didn't restore correctly: got %+v, want %+v", current, saved)
	}
}

func BenchmarkDrawCircle(b *testing.B) {
	dc := NewContext(512, 512)
	dc.SetRGB(1, 0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc.ClearPath()
		dc.DrawCircle(256, 256, 100)
		dc.Fill()
	}
}

func BenchmarkDrawRectangle(b *testing.B) {
	dc := NewContext(512, 512)
	dc.SetRGB(0, 1, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc.ClearPath()
		dc.DrawRectangle(100, 100, 300, 300)
		dc.Fill()
	}
}
