package blend

import (
	"testing"
)

// TestDiv255Fast tests the fast shift-based division.
func TestDiv255Fast(t *testing.T) {
	// Test all possible values from alpha blending (0 to 255*255)
	maxErrors := 0
	for x := 0; x <= 255*255; x++ {
		expected := x / 255
		got := int(div255(uint16(x)))

		// Fast div255 can be +1 higher than exact division
		diff := got - expected
		if diff < 0 || diff > 1 {
			t.Errorf("div255(%d) = %d, want %d (diff=%d)", x, got, expected, diff)
			maxErrors++
			if maxErrors > 10 {
				t.Fatal("Too many errors")
			}
		}
	}
}

// TestDiv255Exact tests Alvy Ray Smith's exact formula.
func TestDiv255Exact(t *testing.T) {
	// Test all possible values from alpha blending
	for x := 0; x <= 255*255; x++ {
		expected := x / 255
		got := int(div255Exact(uint16(x)))

		if got != expected {
			t.Errorf("div255Exact(%d) = %d, want %d", x, got, expected)
		}
	}
}

// TestMulDiv255 tests multiplication with fast division.
func TestMulDiv255(t *testing.T) {
	// Test key alpha blending cases
	tests := []struct {
		a, b     byte
		expected byte // Using exact formula as reference
	}{
		{0, 0, 0},
		{255, 255, 255},
		{0, 255, 0},
		{255, 0, 0},
		{128, 128, 64}, // ~128*128/255 = 64.25
		{200, 100, 78}, // ~200*100/255 = 78.43
		{1, 255, 1},
		{255, 1, 1},
		{127, 127, 63}, // ~127*127/255 = 63.25
	}

	for _, tt := range tests {
		got := mulDiv255(tt.a, tt.b)
		exactRef := mulDiv255Exact(tt.a, tt.b)

		// Allow difference of 1 from expected due to rounding
		diffExact := int(got) - int(exactRef)
		if diffExact < -1 || diffExact > 1 {
			t.Errorf("mulDiv255(%d, %d) = %d, exact = %d, diff = %d",
				tt.a, tt.b, got, exactRef, diffExact)
		}
	}
}

// TestMulDiv255AllValues exhaustively tests all byte combinations.
func TestMulDiv255AllValues(t *testing.T) {
	errors := 0
	for a := 0; a <= 255; a++ {
		for b := 0; b <= 255; b++ {
			exact := (a * b) / 255
			got := int(mulDiv255(byte(a), byte(b)))

			// Allow +1 error from fast approximation
			diff := got - exact
			if diff < 0 || diff > 1 {
				errors++
				if errors <= 5 {
					t.Errorf("mulDiv255(%d, %d) = %d, want %d (diff=%d)",
						a, b, got, exact, diff)
				}
			}
		}
	}
	if errors > 0 {
		t.Errorf("Total errors: %d out of 65536", errors)
	}
}

// TestInv255 tests alpha inversion.
func TestInv255(t *testing.T) {
	tests := []struct {
		x        byte
		expected byte
	}{
		{0, 255},
		{255, 0},
		{128, 127},
		{1, 254},
		{254, 1},
	}

	for _, tt := range tests {
		got := inv255(tt.x)
		if got != tt.expected {
			t.Errorf("inv255(%d) = %d, want %d", tt.x, got, tt.expected)
		}
	}
}

// TestClamp255 tests clamping to byte range.
func TestClamp255(t *testing.T) {
	tests := []struct {
		x        uint16
		expected byte
	}{
		{0, 0},
		{255, 255},
		{256, 255},
		{1000, 255},
		{65535, 255},
		{128, 128},
	}

	for _, tt := range tests {
		got := clamp255(tt.x)
		if got != tt.expected {
			t.Errorf("clamp255(%d) = %d, want %d", tt.x, got, tt.expected)
		}
	}
}

// TestAddClamp tests clamped addition.
func TestAddClamp(t *testing.T) {
	tests := []struct {
		a, b     byte
		expected byte
	}{
		{0, 0, 0},
		{100, 100, 200},
		{200, 100, 255}, // Would overflow to 300, clamped to 255
		{255, 255, 255},
		{255, 0, 255},
		{0, 255, 255},
	}

	for _, tt := range tests {
		got := addClamp(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("addClamp(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

// TestSubClamp tests clamped subtraction.
func TestSubClamp(t *testing.T) {
	tests := []struct {
		a, b     byte
		expected byte
	}{
		{100, 50, 50},
		{50, 100, 0}, // Would underflow, clamped to 0
		{0, 0, 0},
		{255, 255, 0},
		{255, 0, 255},
		{0, 255, 0},
	}

	for _, tt := range tests {
		got := subClamp(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("subClamp(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

// Benchmarks

// BenchmarkDiv255_Fast benchmarks the fast shift-based division.
func BenchmarkDiv255_Fast(b *testing.B) {
	x := uint16(32768) // Middle value
	var result uint16
	for i := 0; i < b.N; i++ {
		result = div255(x)
	}
	_ = result
}

// BenchmarkDiv255_Exact benchmarks Alvy Ray Smith's exact formula.
func BenchmarkDiv255_Exact(b *testing.B) {
	x := uint16(32768)
	var result uint16
	for i := 0; i < b.N; i++ {
		result = div255Exact(x)
	}
	_ = result
}

// BenchmarkDiv255_Division benchmarks actual integer division.
func BenchmarkDiv255_Division(b *testing.B) {
	x := uint16(32768)
	var result uint16
	for i := 0; i < b.N; i++ {
		result = x / 255
	}
	_ = result
}

// BenchmarkMulDiv255_Fast benchmarks fast multiply-divide.
func BenchmarkMulDiv255_Fast(b *testing.B) {
	a, c := byte(200), byte(150)
	var result byte
	for i := 0; i < b.N; i++ {
		result = mulDiv255(a, c)
	}
	_ = result
}

// BenchmarkMulDiv255_Exact benchmarks exact multiply-divide.
func BenchmarkMulDiv255_Exact(b *testing.B) {
	a, c := byte(200), byte(150)
	var result byte
	for i := 0; i < b.N; i++ {
		result = mulDiv255Exact(a, c)
	}
	_ = result
}

// BenchmarkMulDiv255_Old benchmarks the old division-based formula.
func BenchmarkMulDiv255_Old(b *testing.B) {
	a, c := byte(200), byte(150)
	var result byte
	for i := 0; i < b.N; i++ {
		// Old implementation: (a * b + 127) / 255
		result = byte((uint16(a)*uint16(c) + 127) / 255)
	}
	_ = result
}

// BenchmarkBlendSourceOver_Fast compares blend performance.
func BenchmarkBlendSourceOver_1000Pixels(b *testing.B) {
	// Simulate blending 1000 pixels
	src := make([]byte, 4000) // 1000 RGBA pixels
	dst := make([]byte, 4000)

	// Fill with semi-transparent colors
	for i := 0; i < 4000; i += 4 {
		src[i] = 200   // R
		src[i+1] = 100 // G
		src[i+2] = 50  // B
		src[i+3] = 128 // A (50% transparent)

		dst[i] = 50    // R
		dst[i+1] = 100 // G
		dst[i+2] = 200 // B
		dst[i+3] = 255 // A (opaque)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 4000; j += 4 {
			dst[j], dst[j+1], dst[j+2], dst[j+3] = blendSourceOver(
				src[j], src[j+1], src[j+2], src[j+3],
				dst[j], dst[j+1], dst[j+2], dst[j+3],
			)
		}
	}
}
