package recording

import (
	"math"
	"testing"
)

const epsilon = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestIdentity(t *testing.T) {
	m := Identity()

	if m.A != 1 || m.B != 0 || m.C != 0 ||
		m.D != 0 || m.E != 1 || m.F != 0 {
		t.Errorf("Identity() = %+v, want identity matrix", m)
	}

	if !m.IsIdentity() {
		t.Error("Identity().IsIdentity() = false, want true")
	}
}

func TestTranslate(t *testing.T) {
	m := Translate(10, 20)

	if m.A != 1 || m.B != 0 || m.C != 10 ||
		m.D != 0 || m.E != 1 || m.F != 20 {
		t.Errorf("Translate(10, 20) = %+v", m)
	}

	if !m.IsTranslation() {
		t.Error("Translate().IsTranslation() = false, want true")
	}

	// Test transform point
	x, y := m.TransformPoint(5, 5)
	if x != 15 || y != 25 {
		t.Errorf("TransformPoint(5, 5) = (%v, %v), want (15, 25)", x, y)
	}
}

func TestScale(t *testing.T) {
	m := Scale(2, 3)

	if m.A != 2 || m.B != 0 || m.C != 0 ||
		m.D != 0 || m.E != 3 || m.F != 0 {
		t.Errorf("Scale(2, 3) = %+v", m)
	}

	// Test transform point
	x, y := m.TransformPoint(10, 10)
	if x != 20 || y != 30 {
		t.Errorf("TransformPoint(10, 10) = (%v, %v), want (20, 30)", x, y)
	}

	// Test scale factor
	sf := m.ScaleFactor()
	if sf != 3 {
		t.Errorf("ScaleFactor() = %v, want 3", sf)
	}
}

func TestRotate(t *testing.T) {
	m := Rotate(math.Pi / 2) // 90 degrees

	// Rotating (1, 0) by 90 degrees should give (0, 1)
	x, y := m.TransformPoint(1, 0)
	if !almostEqual(x, 0) || !almostEqual(y, 1) {
		t.Errorf("Rotate(90deg).TransformPoint(1, 0) = (%v, %v), want (0, 1)", x, y)
	}

	// Rotating (0, 1) by 90 degrees should give (-1, 0)
	x, y = m.TransformPoint(0, 1)
	if !almostEqual(x, -1) || !almostEqual(y, 0) {
		t.Errorf("Rotate(90deg).TransformPoint(0, 1) = (%v, %v), want (-1, 0)", x, y)
	}
}

func TestShear(t *testing.T) {
	m := Shear(0.5, 0)

	x, y := m.TransformPoint(10, 10)
	if !almostEqual(x, 15) || !almostEqual(y, 10) {
		t.Errorf("Shear(0.5, 0).TransformPoint(10, 10) = (%v, %v), want (15, 10)", x, y)
	}
}

func TestMultiply(t *testing.T) {
	// Test: Scale then Translate
	scale := Scale(2, 2)
	translate := Translate(10, 10)
	combined := translate.Multiply(scale)

	// Point (5, 5) scaled by 2 = (10, 10), then translated by (10, 10) = (20, 20)
	x, y := combined.TransformPoint(5, 5)
	if !almostEqual(x, 20) || !almostEqual(y, 20) {
		t.Errorf("combined.TransformPoint(5, 5) = (%v, %v), want (20, 20)", x, y)
	}
}

func TestInvert(t *testing.T) {
	// Test invertible matrix
	m := Scale(2, 3).Multiply(Translate(10, 20))
	inv := m.Invert()

	// m * inv should be identity
	result := m.Multiply(inv)
	if !result.IsIdentity() {
		t.Errorf("m.Multiply(m.Invert()) = %+v, want identity", result)
	}

	// inv * m should also be identity
	result = inv.Multiply(m)
	if !result.IsIdentity() {
		t.Errorf("m.Invert().Multiply(m) = %+v, want identity", result)
	}
}

func TestInvert_Singular(t *testing.T) {
	// Singular matrix (determinant = 0)
	m := Matrix{A: 1, B: 2, C: 0, D: 2, E: 4, F: 0}
	inv := m.Invert()

	// Should return identity for singular matrix
	if !inv.IsIdentity() {
		t.Errorf("Singular matrix inversion should return identity, got %+v", inv)
	}
}

func TestTransformVector(t *testing.T) {
	// Transform vector should ignore translation
	m := Translate(100, 100).Multiply(Scale(2, 3))

	px, py := m.TransformPoint(10, 10)
	vx, vy := m.TransformVector(10, 10)

	// Point should include translation
	if !almostEqual(px, 120) || !almostEqual(py, 130) {
		t.Errorf("TransformPoint(10, 10) = (%v, %v), want (120, 130)", px, py)
	}

	// Vector should NOT include translation
	if !almostEqual(vx, 20) || !almostEqual(vy, 30) {
		t.Errorf("TransformVector(10, 10) = (%v, %v), want (20, 30)", vx, vy)
	}
}

func TestDeterminant(t *testing.T) {
	tests := []struct {
		name string
		m    Matrix
		want float64
	}{
		{"identity", Identity(), 1.0},
		{"scale 2x3", Scale(2, 3), 6.0},
		{"rotation", Rotate(math.Pi / 4), 1.0},
		{"shear", Shear(1, 0), 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.Determinant()
			if !almostEqual(got, tt.want) {
				t.Errorf("Determinant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranslation(t *testing.T) {
	m := Translate(15, 25)
	x, y := m.Translation()

	if x != 15 || y != 25 {
		t.Errorf("Translation() = (%v, %v), want (15, 25)", x, y)
	}
}

// --- Rect tests ---

func TestNewRect(t *testing.T) {
	r := NewRect(10, 20, 100, 50)

	if r.X() != 10 {
		t.Errorf("X() = %v, want 10", r.X())
	}
	if r.Y() != 20 {
		t.Errorf("Y() = %v, want 20", r.Y())
	}
	if r.Width() != 100 {
		t.Errorf("Width() = %v, want 100", r.Width())
	}
	if r.Height() != 50 {
		t.Errorf("Height() = %v, want 50", r.Height())
	}
}

func TestNewRectFromPoints(t *testing.T) {
	// Test with points in order
	r1 := NewRectFromPoints(10, 20, 110, 70)
	if r1.MinX != 10 || r1.MinY != 20 || r1.MaxX != 110 || r1.MaxY != 70 {
		t.Errorf("NewRectFromPoints(10, 20, 110, 70) = %+v", r1)
	}

	// Test with points reversed
	r2 := NewRectFromPoints(110, 70, 10, 20)
	if r2.MinX != 10 || r2.MinY != 20 || r2.MaxX != 110 || r2.MaxY != 70 {
		t.Errorf("NewRectFromPoints(110, 70, 10, 20) = %+v", r2)
	}
}

func TestRect_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		r    Rect
		want bool
	}{
		{"normal", NewRect(0, 0, 100, 100), false},
		{"zero width", NewRect(0, 0, 0, 100), true},
		{"zero height", NewRect(0, 0, 100, 0), true},
		{"negative width", NewRect(100, 0, -50, 100), true},
		{"negative height", NewRect(0, 100, 100, -50), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRect_Contains(t *testing.T) {
	r := NewRect(10, 20, 100, 50)

	tests := []struct {
		name string
		x, y float64
		want bool
	}{
		{"inside", 50, 40, true},
		{"top-left corner", 10, 20, true},
		{"bottom-right corner", 110, 70, true},
		{"outside left", 5, 40, false},
		{"outside right", 115, 40, false},
		{"outside top", 50, 15, false},
		{"outside bottom", 50, 75, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Contains(tt.x, tt.y); got != tt.want {
				t.Errorf("Contains(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestRect_Union(t *testing.T) {
	r1 := NewRect(10, 20, 100, 50)
	r2 := NewRect(50, 60, 100, 50)

	u := r1.Union(r2)

	if u.MinX != 10 || u.MinY != 20 || u.MaxX != 150 || u.MaxY != 110 {
		t.Errorf("Union() = %+v, want (10, 20, 150, 110)", u)
	}
}

func TestRect_Intersect(t *testing.T) {
	r1 := NewRect(10, 20, 100, 50)
	r2 := NewRect(50, 40, 100, 50)

	i := r1.Intersect(r2)

	if i.MinX != 50 || i.MinY != 40 || i.MaxX != 110 || i.MaxY != 70 {
		t.Errorf("Intersect() = %+v, want (50, 40, 110, 70)", i)
	}
}

func TestRect_Intersect_NoOverlap(t *testing.T) {
	r1 := NewRect(10, 20, 30, 30)
	r2 := NewRect(100, 100, 50, 50)

	i := r1.Intersect(r2)

	if !i.IsEmpty() {
		t.Errorf("Intersect() with no overlap should be empty, got %+v", i)
	}
}

func TestRect_Inset(t *testing.T) {
	r := NewRect(10, 20, 100, 50)
	inset := r.Inset(5, 10)

	if inset.MinX != 15 || inset.MinY != 30 || inset.MaxX != 105 || inset.MaxY != 60 {
		t.Errorf("Inset(5, 10) = %+v, want (15, 30, 105, 60)", inset)
	}
}

func TestRect_Offset(t *testing.T) {
	r := NewRect(10, 20, 100, 50)
	offset := r.Offset(15, -5)

	if offset.MinX != 25 || offset.MinY != 15 || offset.MaxX != 125 || offset.MaxY != 65 {
		t.Errorf("Offset(15, -5) = %+v, want (25, 15, 125, 65)", offset)
	}
}
