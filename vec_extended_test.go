package gg

import (
	"math"
	"testing"
)

func TestVec2_Div(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		s      float64
		expect Vec2
	}{
		{"halve", V2(6, 8), 2, V2(3, 4)},
		{"by one", V2(3, 4), 1, V2(3, 4)},
		{"negative", V2(6, 8), -2, V2(-3, -4)},
		{"fractional", V2(5, 10), 0.5, V2(10, 20)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Div(tt.s)
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Div(%v) = %v, want %v", tt.v, tt.s, result, tt.expect)
			}
		})
	}
}

func TestVec2_Neg(t *testing.T) {
	tests := []struct {
		name   string
		v      Vec2
		expect Vec2
	}{
		{"positive", V2(3, 4), V2(-3, -4)},
		{"negative", V2(-1, -2), V2(1, 2)},
		{"zero", V2(0, 0), V2(0, 0)},
		{"mixed", V2(5, -7), V2(-5, 7)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Neg()
			if !result.Approx(tt.expect, 1e-10) {
				t.Errorf("%v.Neg() = %v, want %v", tt.v, result, tt.expect)
			}
		})
	}
}

func TestVec2_Angle(t *testing.T) {
	tests := []struct {
		name   string
		v, w   Vec2
		expect float64
	}{
		{"same direction", V2(1, 0), V2(2, 0), 0},
		{"90 deg", V2(1, 0), V2(0, 1), math.Pi / 2},
		{"opposite", V2(1, 0), V2(-1, 0), math.Pi},
		{"-90 deg", V2(1, 0), V2(0, -1), -math.Pi / 2},
		{"45 deg", V2(1, 0), V2(1, 1), math.Pi / 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v.Angle(tt.w)
			if math.Abs(result-tt.expect) > 1e-10 {
				t.Errorf("%v.Angle(%v) = %v, want %v", tt.v, tt.w, result, tt.expect)
			}
		})
	}
}

func TestVec2_Approx(t *testing.T) {
	v := V2(1.0, 2.0)
	w := V2(1.0+1e-12, 2.0+1e-12)

	if !v.Approx(w, 1e-10) {
		t.Errorf("%v should be approximately equal to %v", v, w)
	}

	far := V2(2.0, 3.0)
	if v.Approx(far, 1e-10) {
		t.Errorf("%v should NOT be approximately equal to %v", v, far)
	}
}
