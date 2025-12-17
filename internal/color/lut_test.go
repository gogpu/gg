package color

import (
	"math"
	"testing"
)

// TestSRGBToLinearAccuracy tests that LUT matches math.Pow implementation.
func TestSRGBToLinearAccuracy(t *testing.T) {
	maxError := float32(0.0)
	for i := 0; i < 256; i++ {
		fast := SRGBToLinearFast(uint8(i))
		slow := SRGBToLinearSlow(uint8(i))
		diff := float32(math.Abs(float64(fast - slow)))
		if diff > maxError {
			maxError = diff
		}
		// Error should be tiny (< 0.0001)
		if diff > 0.0001 {
			t.Errorf("sRGB %d: fast=%f, slow=%f, error=%f", i, fast, slow, diff)
		}
	}
	t.Logf("Max sRGB→Linear error: %f", maxError)
}

// TestLinearToSRGBAccuracy tests that LUT matches math.Pow implementation.
func TestLinearToSRGBAccuracy(t *testing.T) {
	maxError := 0
	errorCount := 0
	// Test 1000 evenly spaced points in [0, 1]
	for i := 0; i <= 1000; i++ {
		linear := float32(i) / 1000.0
		fast := LinearToSRGBFast(linear)
		slow := LinearToSRGBSlow(linear)
		diff := int(fast) - int(slow)
		if diff < 0 {
			diff = -diff
		}
		if diff > maxError {
			maxError = diff
		}
		if diff > 1 {
			errorCount++
			if errorCount <= 10 { // Only log first 10 errors
				t.Errorf("Linear %f: fast=%d, slow=%d, error=%d", linear, fast, slow, diff)
			}
		}
	}
	t.Logf("Max Linear→sRGB error: %d bytes (out of 255)", maxError)
	t.Logf("Errors > 1 byte: %d / 1001 = %.2f%%", errorCount, float64(errorCount)/1001.0*100.0)
	// We allow max 1-byte error due to rounding in 12-bit LUT
	if maxError > 1 {
		t.Errorf("Maximum error %d exceeds threshold of 1", maxError)
	}
}

// TestSRGBRoundTrip tests that sRGB → Linear → sRGB preserves values.
func TestSRGBRoundTrip(t *testing.T) {
	maxError := 0
	for i := 0; i < 256; i++ {
		srgb := uint8(i)
		linear := SRGBToLinearFast(srgb)
		result := LinearToSRGBFast(linear)
		diff := int(result) - int(srgb)
		if diff < 0 {
			diff = -diff
		}
		if diff > maxError {
			maxError = diff
		}
		// Allow 1-byte error due to quantization
		if diff > 1 {
			t.Errorf("Round trip %d → %f → %d (error=%d)", srgb, linear, result, diff)
		}
	}
	t.Logf("Max round-trip error: %d bytes", maxError)
}

// TestLinearRoundTrip tests that Linear → sRGB → Linear is close.
func TestLinearRoundTrip(t *testing.T) {
	maxError := float32(0.0)
	for i := 0; i <= 1000; i++ {
		linear := float32(i) / 1000.0
		srgb := LinearToSRGBFast(linear)
		result := SRGBToLinearFast(srgb)
		diff := float32(math.Abs(float64(result - linear)))
		if diff > maxError {
			maxError = diff
		}
		// Larger tolerance for linear round-trip due to 8-bit sRGB quantization
		if diff > 0.01 {
			t.Errorf("Round trip %f → %d → %f (error=%f)", linear, srgb, result, diff)
		}
	}
	t.Logf("Max linear round-trip error: %f", maxError)
}

// TestEdgeCases tests boundary values.
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		srgb  uint8
		wantL float32
	}{
		{"black", 0, 0.0},
		{"white", 255, 1.0},
		{"mid-gray", 128, 0.21586}, // Close to 0.2159
		{"quarter", 64, 0.05087},
		{"three-quarter", 192, 0.52733},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SRGBToLinearFast(tt.srgb)
			// Allow 0.01 tolerance
			if math.Abs(float64(got-tt.wantL)) > 0.01 {
				t.Errorf("SRGBToLinearFast(%d) = %f, want ~%f", tt.srgb, got, tt.wantL)
			}
		})
	}

	// Test Linear → sRGB edge cases
	linearTests := []struct {
		name   string
		linear float32
		wantS  uint8
	}{
		{"black", 0.0, 0},
		{"white", 1.0, 255},
		{"below-zero", -0.5, 0},  // Should clamp
		{"above-one", 1.5, 255},  // Should clamp
		{"mid-linear", 0.5, 188}, // Close to 188
	}

	for _, tt := range linearTests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinearToSRGBFast(tt.linear)
			// Allow 1-byte tolerance
			diff := int(got) - int(tt.wantS)
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				t.Errorf("LinearToSRGBFast(%f) = %d, want ~%d", tt.linear, got, tt.wantS)
			}
		})
	}
}

// TestLUTInitialization verifies the LUT tables are initialized correctly.
func TestLUTInitialization(t *testing.T) {
	// Check sRGB → Linear table
	if sRGBToLinearLUT[0] != 0.0 {
		t.Errorf("sRGBToLinearLUT[0] = %f, want 0.0", sRGBToLinearLUT[0])
	}
	if sRGBToLinearLUT[255] < 0.99 || sRGBToLinearLUT[255] > 1.01 {
		t.Errorf("sRGBToLinearLUT[255] = %f, want ~1.0", sRGBToLinearLUT[255])
	}

	// Check Linear → sRGB table
	if linearToSRGBLUT[0] != 0 {
		t.Errorf("linearToSRGBLUT[0] = %d, want 0", linearToSRGBLUT[0])
	}
	if linearToSRGBLUT[4095] != 255 {
		t.Errorf("linearToSRGBLUT[4095] = %d, want 255", linearToSRGBLUT[4095])
	}

	// Check monotonicity (tables should be strictly increasing)
	for i := 1; i < 256; i++ {
		if sRGBToLinearLUT[i] < sRGBToLinearLUT[i-1] {
			t.Errorf("sRGBToLinearLUT[%d] < sRGBToLinearLUT[%d]: not monotonic", i, i-1)
		}
	}
	for i := 1; i < 4096; i++ {
		if linearToSRGBLUT[i] < linearToSRGBLUT[i-1] {
			t.Errorf("linearToSRGBLUT[%d] < linearToSRGBLUT[%d]: not monotonic", i, i-1)
		}
	}
}

// BenchmarkSRGBToLinearFast benchmarks the LUT-based conversion.
func BenchmarkSRGBToLinearFast(b *testing.B) {
	var result float32
	for i := 0; i < b.N; i++ {
		result = SRGBToLinearFast(uint8(i & 0xFF))
	}
	_ = result
}

// BenchmarkSRGBToLinearSlow benchmarks the math.Pow-based conversion.
func BenchmarkSRGBToLinearSlow(b *testing.B) {
	var result float32
	for i := 0; i < b.N; i++ {
		result = SRGBToLinearSlow(uint8(i & 0xFF))
	}
	_ = result
}

// BenchmarkLinearToSRGBFast benchmarks the LUT-based conversion.
func BenchmarkLinearToSRGBFast(b *testing.B) {
	var result uint8
	for i := 0; i < b.N; i++ {
		result = LinearToSRGBFast(float32(i&0xFF) / 255.0)
	}
	_ = result
}

// BenchmarkLinearToSRGBSlow benchmarks the math.Pow-based conversion.
func BenchmarkLinearToSRGBSlow(b *testing.B) {
	var result uint8
	for i := 0; i < b.N; i++ {
		result = LinearToSRGBSlow(float32(i&0xFF) / 255.0)
	}
	_ = result
}

// BenchmarkRoundTrip benchmarks full sRGB → Linear → sRGB conversion.
func BenchmarkRoundTrip(b *testing.B) {
	var result uint8
	for i := 0; i < b.N; i++ {
		srgb := uint8(i & 0xFF)
		linear := SRGBToLinearFast(srgb)
		result = LinearToSRGBFast(linear)
	}
	_ = result
}
