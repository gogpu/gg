package image

import (
	"math"
	"testing"
)

const epsilon = 1e-10

func TestIdentity(t *testing.T) {
	a := Identity()

	// Test that identity doesn't change points
	x, y := a.TransformPoint(10, 20)
	if math.Abs(x-10) > epsilon || math.Abs(y-20) > epsilon {
		t.Errorf("Identity transform failed: got (%f, %f), want (10, 20)", x, y)
	}

	// Test identity matrix values
	if a.a != 1 || a.e != 1 {
		t.Errorf("Identity diagonal should be 1: got a=%f, e=%f", a.a, a.e)
	}
	if a.b != 0 || a.c != 0 || a.d != 0 || a.f != 0 {
		t.Errorf("Identity off-diagonal should be 0")
	}
}

func TestTranslate(t *testing.T) {
	tests := []struct {
		name string
		tx   float64
		ty   float64
		inX  float64
		inY  float64
		outX float64
		outY float64
	}{
		{"positive", 5, 10, 0, 0, 5, 10},
		{"negative", -5, -10, 10, 20, 5, 10},
		{"mixed", 3, -4, 2, 8, 5, 4},
		{"zero", 0, 0, 10, 20, 10, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Translate(tt.tx, tt.ty)
			x, y := a.TransformPoint(tt.inX, tt.inY)

			if math.Abs(x-tt.outX) > epsilon || math.Abs(y-tt.outY) > epsilon {
				t.Errorf("Translate(%f, %f).TransformPoint(%f, %f) = (%f, %f), want (%f, %f)",
					tt.tx, tt.ty, tt.inX, tt.inY, x, y, tt.outX, tt.outY)
			}
		})
	}
}

func TestScale(t *testing.T) {
	tests := []struct {
		name string
		sx   float64
		sy   float64
		inX  float64
		inY  float64
		outX float64
		outY float64
	}{
		{"uniform", 2, 2, 10, 20, 20, 40},
		{"non-uniform", 3, 0.5, 4, 10, 12, 5},
		{"flip-x", -1, 1, 5, 10, -5, 10},
		{"flip-y", 1, -1, 5, 10, 5, -10},
		{"zero", 1, 1, 7, 9, 7, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Scale(tt.sx, tt.sy)
			x, y := a.TransformPoint(tt.inX, tt.inY)

			if math.Abs(x-tt.outX) > epsilon || math.Abs(y-tt.outY) > epsilon {
				t.Errorf("Scale(%f, %f).TransformPoint(%f, %f) = (%f, %f), want (%f, %f)",
					tt.sx, tt.sy, tt.inX, tt.inY, x, y, tt.outX, tt.outY)
			}
		})
	}
}

func TestRotate(t *testing.T) {
	tests := []struct {
		name  string
		angle float64
		inX   float64
		inY   float64
		outX  float64
		outY  float64
	}{
		{"90deg", math.Pi / 2, 1, 0, 0, 1},
		{"180deg", math.Pi, 1, 0, -1, 0},
		{"270deg", 3 * math.Pi / 2, 1, 0, 0, -1},
		{"360deg", 2 * math.Pi, 1, 0, 1, 0},
		{"45deg", math.Pi / 4, 1, 0, math.Sqrt(2) / 2, math.Sqrt(2) / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Rotate(tt.angle)
			x, y := a.TransformPoint(tt.inX, tt.inY)

			if math.Abs(x-tt.outX) > epsilon || math.Abs(y-tt.outY) > epsilon {
				t.Errorf("Rotate(%f).TransformPoint(%f, %f) = (%f, %f), want (%f, %f)",
					tt.angle, tt.inX, tt.inY, x, y, tt.outX, tt.outY)
			}
		})
	}
}

func TestShear(t *testing.T) {
	tests := []struct {
		name string
		sx   float64
		sy   float64
		inX  float64
		inY  float64
		outX float64
		outY float64
	}{
		{"shear-x", 2, 0, 1, 3, 7, 3},  // x' = x + 2*y = 1 + 6 = 7
		{"shear-y", 0, 2, 3, 1, 3, 7},  // y' = y + 2*x = 1 + 6 = 7
		{"shear-both", 1, 1, 2, 3, 5, 5}, // x' = 2+3=5, y' = 3+2=5
		{"no-shear", 0, 0, 5, 7, 5, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Shear(tt.sx, tt.sy)
			x, y := a.TransformPoint(tt.inX, tt.inY)

			if math.Abs(x-tt.outX) > epsilon || math.Abs(y-tt.outY) > epsilon {
				t.Errorf("Shear(%f, %f).TransformPoint(%f, %f) = (%f, %f), want (%f, %f)",
					tt.sx, tt.sy, tt.inX, tt.inY, x, y, tt.outX, tt.outY)
			}
		})
	}
}

func TestMultiply(t *testing.T) {
	// Test that translate then scale works correctly
	translate := Translate(10, 20)
	scale := Scale(2, 2)
	combined := scale.Multiply(translate)

	// Point (0,0) -> translate to (10, 20) -> scale to (20, 40)
	x, y := combined.TransformPoint(0, 0)
	if math.Abs(x-20) > epsilon || math.Abs(y-40) > epsilon {
		t.Errorf("Scale.Multiply(Translate).TransformPoint(0, 0) = (%f, %f), want (20, 40)", x, y)
	}

	// Test that the order matters
	combined2 := translate.Multiply(scale)
	x2, y2 := combined2.TransformPoint(0, 0)
	if math.Abs(x2-10) > epsilon || math.Abs(y2-20) > epsilon {
		t.Errorf("Translate.Multiply(Scale).TransformPoint(0, 0) = (%f, %f), want (10, 20)", x2, y2)
	}
}

func TestInvert(t *testing.T) {
	tests := []struct {
		name      string
		transform Affine
		shouldOK  bool
	}{
		{"identity", Identity(), true},
		{"translate", Translate(10, 20), true},
		{"scale", Scale(2, 3), true},
		{"rotate", Rotate(math.Pi / 4), true},
		{"singular", Affine{a: 1, b: 2, d: 2, e: 4}, false}, // Rows are proportional
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, ok := tt.transform.Invert()

			if ok != tt.shouldOK {
				t.Errorf("Invert() ok = %v, want %v", ok, tt.shouldOK)
				return
			}

			if !ok {
				return // Expected to fail, test passed
			}

			// Test that transform * inverse = identity
			identity := tt.transform.Multiply(inv)

			// Transform a test point
			x, y := identity.TransformPoint(5, 7)
			if math.Abs(x-5) > epsilon || math.Abs(y-7) > epsilon {
				t.Errorf("Transform * Inverse should give identity, got (%f, %f), want (5, 7)", x, y)
			}
		})
	}
}

func TestInvertComposition(t *testing.T) {
	// Create a complex transform: translate, rotate, scale
	transform := Translate(10, 20).Multiply(Rotate(math.Pi / 6)).Multiply(Scale(2, 3))

	inv, ok := transform.Invert()
	if !ok {
		t.Fatal("Failed to invert complex transform")
	}

	// Test that applying transform then inverse returns to original point
	testPoints := [][2]float64{
		{0, 0},
		{10, 20},
		{-5, 15},
		{100, 200},
	}

	for _, pt := range testPoints {
		// Apply forward transform
		x1, y1 := transform.TransformPoint(pt[0], pt[1])

		// Apply inverse transform
		x2, y2 := inv.TransformPoint(x1, y1)

		if math.Abs(x2-pt[0]) > epsilon || math.Abs(y2-pt[1]) > epsilon {
			t.Errorf("Transform -> Inverse did not preserve point (%f, %f): got (%f, %f)",
				pt[0], pt[1], x2, y2)
		}
	}
}

func TestRotateAt(t *testing.T) {
	// Rotate 90 degrees around point (10, 10)
	a := RotateAt(math.Pi/2, 10, 10)

	// Point (10, 10) should stay the same (rotation center)
	x, y := a.TransformPoint(10, 10)
	if math.Abs(x-10) > epsilon || math.Abs(y-10) > epsilon {
		t.Errorf("RotateAt center should not move: got (%f, %f), want (10, 10)", x, y)
	}

	// Point (11, 10) should rotate to (10, 11)
	x, y = a.TransformPoint(11, 10)
	if math.Abs(x-10) > epsilon || math.Abs(y-11) > epsilon {
		t.Errorf("RotateAt(90°, 10, 10).TransformPoint(11, 10) = (%f, %f), want (10, 11)", x, y)
	}
}

func TestScaleAt(t *testing.T) {
	// Scale by 2x around point (10, 10)
	a := ScaleAt(2, 2, 10, 10)

	// Point (10, 10) should stay the same (scale center)
	x, y := a.TransformPoint(10, 10)
	if math.Abs(x-10) > epsilon || math.Abs(y-10) > epsilon {
		t.Errorf("ScaleAt center should not move: got (%f, %f), want (10, 10)", x, y)
	}

	// Point (12, 14) is 2 units right, 4 units down from center
	// After 2x scale: should be 4 units right, 8 units down: (14, 18)
	x, y = a.TransformPoint(12, 14)
	if math.Abs(x-14) > epsilon || math.Abs(y-18) > epsilon {
		t.Errorf("ScaleAt(2, 2, 10, 10).TransformPoint(12, 14) = (%f, %f), want (14, 18)", x, y)
	}
}

func BenchmarkTransformPoint(b *testing.B) {
	a := Translate(10, 20).Multiply(Rotate(math.Pi / 4)).Multiply(Scale(2, 3))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = a.TransformPoint(float64(i), float64(i*2))
	}
}

func BenchmarkInvert(b *testing.B) {
	a := Translate(10, 20).Multiply(Rotate(math.Pi / 4)).Multiply(Scale(2, 3))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = a.Invert()
	}
}

func BenchmarkMultiply(b *testing.B) {
	a := Translate(10, 20)
	c := Rotate(math.Pi / 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Multiply(c)
	}
}
