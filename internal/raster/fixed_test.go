// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"math"
	"testing"
)

// TestFDot6FromInt tests integer to FDot6 conversion.
func TestFDot6FromInt(t *testing.T) {
	tests := []struct {
		name  string
		input int32
		want  FDot6
	}{
		{"zero", 0, 0},
		{"one", 1, 64},
		{"negative one", -1, -64},
		{"ten", 10, 640},
		{"large positive", 1000, 64000},
		{"large negative", -1000, -64000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6FromInt(tt.input)
			if got != tt.want {
				t.Errorf("FDot6FromInt(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6FromFloat32 tests float32 to FDot6 conversion.
func TestFDot6FromFloat32(t *testing.T) {
	tests := []struct {
		name  string
		input float32
		want  FDot6
	}{
		{"zero", 0.0, 0},
		{"one", 1.0, 64},
		{"half", 0.5, 32},
		{"quarter", 0.25, 16},
		{"negative", -1.5, -96},
		{"small fraction", 0.015625, 1}, // 1/64
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6FromFloat32(tt.input)
			if got != tt.want {
				t.Errorf("FDot6FromFloat32(%f) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6FromFloat64 tests float64 to FDot6 conversion.
func TestFDot6FromFloat64(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  FDot6
	}{
		{"zero", 0.0, 0},
		{"one", 1.0, 64},
		{"half", 0.5, 32},
		{"negative", -2.0, -128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6FromFloat64(tt.input)
			if got != tt.want {
				t.Errorf("FDot6FromFloat64(%f) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6ToFloat32 tests FDot6 to float32 conversion.
func TestFDot6ToFloat32(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  float32
	}{
		{"zero", 0, 0.0},
		{"one", 64, 1.0},
		{"half", 32, 0.5},
		{"negative", -128, -2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6ToFloat32(tt.input)
			if got != tt.want {
				t.Errorf("FDot6ToFloat32(%d) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6ToFloat64 tests FDot6 to float64 conversion.
func TestFDot6ToFloat64(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  float64
	}{
		{"zero", 0, 0.0},
		{"one", 64, 1.0},
		{"negative half", -32, -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6ToFloat64(tt.input)
			if got != tt.want {
				t.Errorf("FDot6ToFloat64(%d) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6Floor tests floor operation on FDot6 values.
func TestFDot6Floor(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  int32
	}{
		{"zero", 0, 0},
		{"one", 64, 1},
		{"one and a half", 96, 1},
		{"just above zero", 1, 0},
		{"just below one", 63, 0},
		{"negative", -64, -1},
		{"negative fraction", -32, -1}, // -0.5 floors to -1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Floor(tt.input)
			if got != tt.want {
				t.Errorf("FDot6Floor(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6Ceil tests ceiling operation on FDot6 values.
func TestFDot6Ceil(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  int32
	}{
		{"zero", 0, 0},
		{"one", 64, 1},
		{"just above zero", 1, 1},
		{"half", 32, 1},
		{"negative", -64, -1},
		{"negative fraction", -32, 0}, // -0.5 ceils to 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Ceil(tt.input)
			if got != tt.want {
				t.Errorf("FDot6Ceil(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6Round tests rounding operation on FDot6 values.
func TestFDot6Round(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  int32
	}{
		{"zero", 0, 0},
		{"one", 64, 1},
		{"half rounds up", 32, 1},
		{"just below half", 31, 0},
		{"one and a quarter", 80, 1},
		{"one and three quarters", 112, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Round(tt.input)
			if got != tt.want {
				t.Errorf("FDot6Round(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot6RoundTrip verifies round-trip conversion accuracy.
func TestFDot6RoundTrip(t *testing.T) {
	testValues := []float32{0.0, 0.5, 1.0, 2.5, -3.25, 10.75, -100.125}

	for _, v := range testValues {
		fixed := FDot6FromFloat32(v)
		back := FDot6ToFloat32(fixed)
		diff := v - back
		if diff < 0 {
			diff = -diff
		}
		// FDot6 has 1/64 precision, so max error is 1/128
		if diff > 1.0/64.0 {
			t.Errorf("FDot6 round-trip for %f: got %f, diff %f exceeds 1/64", v, back, diff)
		}
	}
}

// TestFDot6Div tests FDot6 division returning FDot16.
func TestFDot6Div(t *testing.T) {
	tests := []struct {
		name string
		a, b FDot6
		want float32 // approximate expected value as float
	}{
		{"simple division", 128, 64, 2.0},            // 2/1 = 2
		{"half", 64, 128, 0.5},                       // 1/2 = 0.5
		{"division by zero positive", 64, 0, 0},      // special: max positive
		{"division by zero negative", -64, 0, 0},     // special: max negative
		{"small numerator fast path", 32, 64, 0.5},   // fits in 16 bits
		{"large numerator slow path", 100000, 64, 0}, // doesn't fit in 16 bits
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Div(tt.a, tt.b)
			if tt.b == 0 {
				// Division by zero should return max magnitude value
				if tt.a >= 0 && got != 0x7FFFFFFF {
					t.Errorf("FDot6Div(%d, 0) = %d, want 0x7FFFFFFF", tt.a, got)
				}
				if tt.a < 0 && got != -0x7FFFFFFF {
					t.Errorf("FDot6Div(%d, 0) = %d, want -0x7FFFFFFF", tt.a, got)
				}
				return
			}
			if tt.want != 0 {
				gotF := FDot16ToFloat32(got)
				diff := gotF - tt.want
				if diff < 0 {
					diff = -diff
				}
				if diff > 0.01 {
					t.Errorf("FDot6Div(%d, %d) = %f, want %f", tt.a, tt.b, gotF, tt.want)
				}
			}
		})
	}
}

// TestFDot6CanConvertToFDot16 tests overflow detection for FDot6->FDot16.
func TestFDot6CanConvertToFDot16(t *testing.T) {
	tests := []struct {
		name string
		v    FDot6
		want bool
	}{
		{"zero", 0, true},
		{"one", 64, true},
		{"small positive", 1000, true},
		{"large positive", 0x7FFFFFFF >> 10, true},
		{"overflow positive", 0x7FFFFFFF, false},
		{"overflow negative", -0x7FFFFFFF, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6CanConvertToFDot16(tt.v)
			if got != tt.want {
				t.Errorf("FDot6CanConvertToFDot16(%d) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

// TestFDot6SmallScale tests alpha scaling by FDot6 factor.
func TestFDot6SmallScale(t *testing.T) {
	tests := []struct {
		name  string
		value uint8
		dot6  FDot6
		want  uint8
	}{
		{"zero scale", 255, 0, 0},
		{"full scale", 255, 64, 255},
		{"half scale", 200, 32, 100},
		{"quarter scale", 200, 16, 50},
		{"zero value", 0, 64, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6SmallScale(tt.value, tt.dot6)
			if got != tt.want {
				t.Errorf("FDot6SmallScale(%d, %d) = %d, want %d", tt.value, tt.dot6, got, tt.want)
			}
		})
	}
}

// TestFDot16FromFloat32 tests float32 to FDot16 conversion with saturation.
func TestFDot16FromFloat32(t *testing.T) {
	tests := []struct {
		name string
		f    float32
		want float32 // approximate
	}{
		{"zero", 0.0, 0.0},
		{"one", 1.0, 1.0},
		{"half", 0.5, 0.5},
		{"negative", -2.5, -2.5},
		{"very large saturates", 100000, 0}, // saturated to max int32
		{"very negative saturates", -100000, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot16FromFloat32(tt.f)
			if tt.want != 0 {
				gotF := FDot16ToFloat32(got)
				diff := gotF - tt.want
				if diff < 0 {
					diff = -diff
				}
				if diff > 0.001 {
					t.Errorf("FDot16FromFloat32(%f) round-trip = %f, want %f", tt.f, gotF, tt.want)
				}
			}
		})
	}
}

// TestFDot16FromFloat64 tests float64 to FDot16 conversion with saturation.
func TestFDot16FromFloat64(t *testing.T) {
	tests := []struct {
		name string
		f    float64
		want float64
	}{
		{"zero", 0.0, 0.0},
		{"one", 1.0, 1.0},
		{"negative", -3.5, -3.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot16FromFloat64(tt.f)
			gotF := FDot16ToFloat64(got)
			diff := gotF - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.001 {
				t.Errorf("FDot16FromFloat64(%f) round-trip = %f, want %f", tt.f, gotF, tt.want)
			}
		})
	}
}

// TestFDot16FloorCeilRound tests FDot16 floor, ceil, round operations.
func TestFDot16FloorCeilRound(t *testing.T) {
	tests := []struct {
		name      string
		v         FDot16
		wantFloor int32
		wantCeil  int32
		wantRound int32
	}{
		{"zero", 0, 0, 0, 0},
		{"one", FDot16One, 1, 1, 1},
		{"one and half", FDot16One + FDot16Half, 1, 2, 2},
		{"just above one", FDot16One + 1, 1, 2, 1},
		{"negative one", -FDot16One, -1, -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FDot16FloorToInt(tt.v); got != tt.wantFloor {
				t.Errorf("FDot16FloorToInt(%d) = %d, want %d", tt.v, got, tt.wantFloor)
			}
			if got := FDot16CeilToInt(tt.v); got != tt.wantCeil {
				t.Errorf("FDot16CeilToInt(%d) = %d, want %d", tt.v, got, tt.wantCeil)
			}
			if got := FDot16RoundToInt(tt.v); got != tt.wantRound {
				t.Errorf("FDot16RoundToInt(%d) = %d, want %d", tt.v, got, tt.wantRound)
			}
		})
	}
}

// TestFDot16Mul tests FDot16 multiplication.
func TestFDot16Mul(t *testing.T) {
	tests := []struct {
		name string
		a, b FDot16
		want float32
	}{
		{"zero", 0, FDot16One, 0.0},
		{"one times one", FDot16One, FDot16One, 1.0},
		{"two times three", 2 * FDot16One, 3 * FDot16One, 6.0},
		{"half times half", FDot16Half, FDot16Half, 0.25},
		{"negative", -FDot16One, FDot16One, -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot16Mul(tt.a, tt.b)
			gotF := FDot16ToFloat32(got)
			diff := gotF - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("FDot16Mul(%d, %d) = %f, want %f", tt.a, tt.b, gotF, tt.want)
			}
		})
	}
}

// TestFDot16Div tests FDot16 division.
func TestFDot16Div(t *testing.T) {
	tests := []struct {
		name        string
		numer       int32
		denom       int32
		wantApprox  float32
		wantSpecial bool // if true, check for max value
	}{
		{"simple", FDot16One, FDot16One, 1.0, false},
		{"half", FDot16One, 2 * FDot16One, 0.5, false},
		{"div by zero positive", 1, 0, 0, true},
		{"div by zero negative", -1, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot16Div(tt.numer, tt.denom)
			if tt.wantSpecial {
				if tt.numer >= 0 && got != 0x7FFFFFFF {
					t.Errorf("FDot16Div(%d, 0) = %d, want max", tt.numer, got)
				}
				if tt.numer < 0 && got != -0x7FFFFFFF {
					t.Errorf("FDot16Div(%d, 0) = %d, want -max", tt.numer, got)
				}
				return
			}
			gotF := FDot16ToFloat32(got)
			diff := gotF - tt.wantApprox
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("FDot16Div(%d, %d) = %f, want %f", tt.numer, tt.denom, gotF, tt.wantApprox)
			}
		})
	}
}

// TestFDot16FastDiv tests fast FDot6 division.
func TestFDot16FastDiv(t *testing.T) {
	tests := []struct {
		name string
		a, b FDot6
		want float32
	}{
		{"simple", 128, 64, 2.0},
		{"half", 64, 128, 0.5},
		{"div by zero pos", 64, 0, 0},
		{"div by zero neg", -64, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot16FastDiv(tt.a, tt.b)
			if tt.b == 0 {
				if tt.a >= 0 && got != 0x7FFFFFFF {
					t.Errorf("FDot16FastDiv(%d, 0) = %d, want max", tt.a, got)
				}
				if tt.a < 0 && got != -0x7FFFFFFF {
					t.Errorf("FDot16FastDiv(%d, 0) = %d, want -max", tt.a, got)
				}
				return
			}
			gotF := FDot16ToFloat32(got)
			diff := gotF - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("FDot16FastDiv(%d, %d) = %f, want %f", tt.a, tt.b, gotF, tt.want)
			}
		})
	}
}

// TestFDot8FromFDot16 tests FDot16 to FDot8 conversion with rounding.
func TestFDot8FromFDot16(t *testing.T) {
	tests := []struct {
		name string
		v    FDot16
		want FDot8
	}{
		{"zero", 0, 0},
		{"one", FDot16One, 256},   // 1.0 in FDot8
		{"half", FDot16Half, 128}, // 0.5 in FDot8
		{"small", 0x80, 1},        // rounds to 1
		{"just below rounding", 0x7F, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot8FromFDot16(tt.v)
			if got != tt.want {
				t.Errorf("FDot8FromFDot16(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

// TestLeftShift tests left shift with sign preservation.
func TestLeftShift(t *testing.T) {
	tests := []struct {
		name  string
		v     int32
		shift int
		want  int32
	}{
		{"positive shift", 1, 4, 16},
		{"zero shift", 42, 0, 42},
		{"negative shift (right shift)", 16, -4, 1},
		{"negative value positive shift", -1, 4, -16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := leftShift(tt.v, tt.shift)
			if got != tt.want {
				t.Errorf("leftShift(%d, %d) = %d, want %d", tt.v, tt.shift, got, tt.want)
			}
		})
	}
}

// TestLeftShift64 tests 64-bit left shift with sign preservation.
func TestLeftShift64(t *testing.T) {
	tests := []struct {
		name  string
		v     int64
		shift int
		want  int64
	}{
		{"positive shift", 1, 10, 1024},
		{"zero shift", 100, 0, 100},
		{"negative shift", 1024, -10, 1},
		{"large value", 1, 32, int64(1) << 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := leftShift64(tt.v, tt.shift)
			if got != tt.want {
				t.Errorf("leftShift64(%d, %d) = %d, want %d", tt.v, tt.shift, got, tt.want)
			}
		})
	}
}

// TestSaturateInt32 tests int64 to int32 saturation.
func TestSaturateInt32(t *testing.T) {
	tests := []struct {
		name string
		v    int64
		want int32
	}{
		{"zero", 0, 0},
		{"positive fits", 1000, 1000},
		{"negative fits", -1000, -1000},
		{"max int32", math.MaxInt32, math.MaxInt32},
		{"min int32", math.MinInt32, math.MinInt32},
		{"overflow positive", math.MaxInt32 + 1, math.MaxInt32},
		{"overflow negative", math.MinInt32 - 1, math.MinInt32},
		{"very large", 1 << 40, math.MaxInt32},
		{"very negative", -(1 << 40), math.MinInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := saturateInt32(tt.v)
			if got != tt.want {
				t.Errorf("saturateInt32(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

// TestFDot6ToFixedDiv2 tests the half-precision FDot6->FDot16 conversion.
func TestFDot6ToFixedDiv2(t *testing.T) {
	tests := []struct {
		name string
		v    FDot6
		want float32 // approximate expected value
	}{
		{"zero", 0, 0.0},
		{"one in fdot6 / 2", FDot6One, 0.5},
		{"two in fdot6 / 2", 2 * FDot6One, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6ToFixedDiv2(tt.v)
			gotF := FDot16ToFloat32(got)
			diff := gotF - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("FDot6ToFixedDiv2(%d) = %f, want %f", tt.v, gotF, tt.want)
			}
		})
	}
}

// BenchmarkFDot16Mul benchmarks FDot16 multiplication.
func BenchmarkFDot16Mul(b *testing.B) {
	a := FDot16(12345)
	c := FDot16(67890)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FDot16Mul(a, c)
	}
}
