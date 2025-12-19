package gg

import (
	"math"
	"testing"
)

func TestVec2_Creation(t *testing.T) {
	tests := []struct {
		name string
		x, y float64
	}{
		{"zero", 0, 0},
		{"positive", 3, 4},
		{"negative", -1, -2},
		{"mixed", -5, 10},
		{"fractional", 1.5, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := V2(tt.x, tt.y)
			if v.X != tt.x || v.Y != tt.y {
				t.Errorf("V2(%v, %v) = %v, want (%v, %v)", tt.x, tt.y, v, tt.x, tt.y)
			}
		})
	}
}

func TestVec2_Add(t *testing.T) {
	tests := []struct {
		name   string
		v, w   Vec2
		expect Vec2
	}{
		{"zero+zero", V2(0, 0), V2(0, 0), V2(0, 0)},
		{"positive", V2(1, 2), V2(3, 4), V2(4, 6)},
		{"negative", V2(-1, -2), V2(-3, -4), V2(-4, -6)},
		{"mixed", V2(1, -2), V2(-3, 4), V2(-2, 2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Add(tt.w)
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Add(%v) = %v, want %v", tt.v, tt.w, result, tt.expect)
			}
		})
	}
}

func TestVec2_Sub(t *testing.T) {
	tests := []struct {
		name   string
		v, w   Vec2
		expect Vec2
	}{
		{"zero-zero", V2(0, 0), V2(0, 0), V2(0, 0)},
		{"positive", V2(5, 7), V2(2, 3), V2(3, 4)},
		{"negative", V2(-1, -2), V2(-3, -4), V2(2, 2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Sub(tt.w)
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Sub(%v) = %v, want %v", tt.v, tt.w, result, tt.expect)
			}
		})
	}
}

func TestVec2_Mul(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		s      float64
		expect Vec2
	}{
		{"zero scalar", V2(1, 2), 0, V2(0, 0)},
		{"positive", V2(1, 2), 3, V2(3, 6)},
		{"negative", V2(1, 2), -2, V2(-2, -4)},
		{"fractional", V2(4, 6), 0.5, V2(2, 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Mul(tt.s)
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Mul(%v) = %v, want %v", tt.v, tt.s, result, tt.expect)
			}
		})
	}
}

func TestVec2_Dot(t *testing.T) {
	tests := []struct {
		name   string
		v, w   Vec2
		expect float64
	}{
		{"orthogonal", V2(1, 0), V2(0, 1), 0},
		{"parallel", V2(1, 0), V2(2, 0), 2},
		{"same", V2(3, 4), V2(3, 4), 25},
		{"opposite", V2(1, 0), V2(-1, 0), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Dot(tt.w)
			if math.Abs(result-tt.expect) > 1e-10 {
				t.Errorf("%v.Dot(%v) = %v, want %v", tt.v, tt.w, result, tt.expect)
			}
		})
	}
}

func TestVec2_Cross(t *testing.T) {
	tests := []struct {
		name   string
		v, w   Vec2
		expect float64
	}{
		{"parallel", V2(1, 0), V2(2, 0), 0},
		{"orthogonal", V2(1, 0), V2(0, 1), 1},
		{"reverse orthogonal", V2(0, 1), V2(1, 0), -1},
		{"general", V2(3, 4), V2(5, 6), 3*6 - 4*5}, // -2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Cross(tt.w)
			if math.Abs(result-tt.expect) > 1e-10 {
				t.Errorf("%v.Cross(%v) = %v, want %v", tt.v, tt.w, result, tt.expect)
			}
		})
	}
}

func TestVec2_Length(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect float64
	}{
		{"zero", V2(0, 0), 0},
		{"unit x", V2(1, 0), 1},
		{"unit y", V2(0, 1), 1},
		{"3-4-5", V2(3, 4), 5},
		{"negative", V2(-3, -4), 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Length()
			if math.Abs(result-tt.expect) > 1e-10 {
				t.Errorf("%v.Length() = %v, want %v", tt.v, result, tt.expect)
			}
		})
	}
}

func TestVec2_LengthSq(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect float64
	}{
		{"zero", V2(0, 0), 0},
		{"3-4-5", V2(3, 4), 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.LengthSq()
			if math.Abs(result-tt.expect) > 1e-10 {
				t.Errorf("%v.LengthSq() = %v, want %v", tt.v, result, tt.expect)
			}
		})
	}
}

func TestVec2_Normalize(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect Vec2
	}{
		{"zero", V2(0, 0), V2(0, 0)},
		{"unit x", V2(5, 0), V2(1, 0)},
		{"unit y", V2(0, 3), V2(0, 1)},
		{"diagonal", V2(3, 4), V2(0.6, 0.8)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Normalize()
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Normalize() = %v, want %v", tt.v, result, tt.expect)
			}
		})
	}
}

func TestVec2_Lerp(t *testing.T) {
	tests := []struct {
		name   string
		v, w   Vec2
		t      float64
		expect Vec2
	}{
		{"t=0", V2(0, 0), V2(10, 10), 0, V2(0, 0)},
		{"t=1", V2(0, 0), V2(10, 10), 1, V2(10, 10)},
		{"t=0.5", V2(0, 0), V2(10, 10), 0.5, V2(5, 5)},
		{"t=0.25", V2(0, 0), V2(8, 4), 0.25, V2(2, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Lerp(tt.w, tt.t)
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Lerp(%v, %v) = %v, want %v", tt.v, tt.w, tt.t, result, tt.expect)
			}
		})
	}
}

func TestVec2_Rotate(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		angle  float64
		expect Vec2
	}{
		{"zero angle", V2(1, 0), 0, V2(1, 0)},
		{"90 deg", V2(1, 0), math.Pi / 2, V2(0, 1)},
		{"180 deg", V2(1, 0), math.Pi, V2(-1, 0)},
		{"270 deg", V2(1, 0), 3 * math.Pi / 2, V2(0, -1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Rotate(tt.angle)
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Rotate(%v) = %v, want %v", tt.v, tt.angle, result, tt.expect)
			}
		})
	}
}

func TestVec2_Perp(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect Vec2
	}{
		{"x axis", V2(1, 0), V2(0, 1)},
		{"y axis", V2(0, 1), V2(-1, 0)},
		{"diagonal", V2(3, 4), V2(-4, 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Perp()
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Perp() = %v, want %v", tt.v, result, tt.expect)
			}
			// Perp should be orthogonal
			if math.Abs(tt.v.Dot(result)) > 1e-10 {
				t.Errorf("Perp should be orthogonal: %v.Dot(%v) != 0", tt.v, result)
			}
		})
	}
}

func TestVec2_Atan2(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect float64
	}{
		{"x axis", V2(1, 0), 0},
		{"y axis", V2(0, 1), math.Pi / 2},
		{"negative x", V2(-1, 0), math.Pi},
		{"negative y", V2(0, -1), -math.Pi / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Atan2()
			if math.Abs(result-tt.expect) > 1e-10 {
				t.Errorf("%v.Atan2() = %v, want %v", tt.v, result, tt.expect)
			}
		})
	}
}

func TestVec2_IsZero(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect bool
	}{
		{"zero", V2(0, 0), true},
		{"non-zero x", V2(1, 0), false},
		{"non-zero y", V2(0, 1), false},
		{"tiny", V2(1e-100, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.IsZero()
			if result != tt.expect {
				t.Errorf("%v.IsZero() = %v, want %v", tt.v, result, tt.expect)
			}
		})
	}
}

func TestVec2_ToPoint(t *testing.T) {
	v := V2(3.5, 4.5)
	p := v.ToPoint()

	if p.X != v.X || p.Y != v.Y {
		t.Errorf("ToPoint() = %v, want (%v, %v)", p, v.X, v.Y)
	}
}

func TestPointToVec2(t *testing.T) {
	p := Pt(3.5, 4.5)
	v := PointToVec2(p)

	if v.X != p.X || v.Y != p.Y {
		t.Errorf("PointToVec2(%v) = %v, want (%v, %v)", p, v, p.X, p.Y)
	}
}
