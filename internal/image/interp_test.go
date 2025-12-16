package image

import (
	"math"
	"testing"
)

// TestSampleNearest tests nearest-neighbor sampling.
func TestSampleNearest(t *testing.T) {
	// Create a 4x4 test image with distinct colors
	img, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	// Fill with gradient pattern
	for y := range 4 {
		for x := range 4 {
			r := byte(x * 64)
			g := byte(y * 64)
			b := byte(128)
			a := byte(255)
			_ = img.SetRGBA(x, y, r, g, b, a)
		}
	}

	tests := []struct {
		name  string
		u, v  float64
		wantX int
		wantY int
		wantR byte
		wantG byte
		wantB byte
		wantA byte
	}{
		{
			name: "top-left corner",
			u:    0.0, v: 0.0,
			wantX: 0, wantY: 0,
			wantR: 0, wantG: 0, wantB: 128, wantA: 255,
		},
		{
			name: "top-right corner",
			u:    1.0, v: 0.0,
			wantX: 3, wantY: 0,
			wantR: 192, wantG: 0, wantB: 128, wantA: 255,
		},
		{
			name: "center pixel (1,1)",
			u:    0.375, v: 0.375,
			wantX: 1, wantY: 1,
			wantR: 64, wantG: 64, wantB: 128, wantA: 255,
		},
		{
			name: "near pixel (2,2)",
			u:    0.625, v: 0.625,
			wantX: 2, wantY: 2,
			wantR: 128, wantG: 128, wantB: 128, wantA: 255,
		},
		{
			name: "bottom-right corner",
			u:    1.0, v: 1.0,
			wantX: 3, wantY: 3,
			wantR: 192, wantG: 192, wantB: 128, wantA: 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := SampleNearest(img, tt.u, tt.v)

			// Verify against expected pixel
			wantR, wantG, wantB, wantA := img.GetRGBA(tt.wantX, tt.wantY)
			if r != wantR || g != wantG || b != wantB || a != wantA {
				t.Errorf("SampleNearest(%v, %v) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.u, tt.v, r, g, b, a, wantR, wantG, wantB, wantA)
			}
		})
	}
}

// TestSampleNearestEdgeClamping tests that out-of-bounds coordinates are clamped.
func TestSampleNearestEdgeClamping(t *testing.T) {
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	// Fill corners with distinct colors
	_ = img.SetRGBA(0, 0, 255, 0, 0, 255)   // Red
	_ = img.SetRGBA(1, 0, 0, 255, 0, 255)   // Green
	_ = img.SetRGBA(0, 1, 0, 0, 255, 255)   // Blue
	_ = img.SetRGBA(1, 1, 255, 255, 0, 255) // Yellow

	tests := []struct {
		name  string
		u, v  float64
		wantR byte
		wantG byte
		wantB byte
	}{
		{"before top-left", -0.5, -0.5, 255, 0, 0},    // Clamps to (0,0) = red
		{"after bottom-right", 1.5, 1.5, 255, 255, 0}, // Clamps to (1,1) = yellow
		{"left edge", -0.1, 0.5, 0, 0, 255},           // Clamps to (0,1) = blue
		{"right edge", 1.1, 0.5, 255, 255, 0},         // Clamps to (1,1) = yellow
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, _ := SampleNearest(img, tt.u, tt.v)
			if r != tt.wantR || g != tt.wantG || b != tt.wantB {
				t.Errorf("SampleNearest(%v, %v) = (%d,%d,%d), want (%d,%d,%d)",
					tt.u, tt.v, r, g, b, tt.wantR, tt.wantG, tt.wantB)
			}
		})
	}
}

// TestSampleBilinear tests bilinear interpolation.
func TestSampleBilinear(t *testing.T) {
	// Create a 2x2 test image
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	// Fill corners with known values
	_ = img.SetRGBA(0, 0, 0, 0, 0, 255)     // Black
	_ = img.SetRGBA(1, 0, 255, 0, 0, 255)   // Red
	_ = img.SetRGBA(0, 1, 0, 255, 0, 255)   // Green
	_ = img.SetRGBA(1, 1, 255, 255, 0, 255) // Yellow

	tests := []struct {
		name      string
		u, v      float64
		checkFunc func(r, g, b, a byte) bool
		desc      string
	}{
		{
			name: "exact top-left corner",
			u:    0.0, v: 0.0,
			checkFunc: func(r, g, b, a byte) bool {
				return r == 0 && g == 0 && b == 0 && a == 255
			},
			desc: "should be black (0,0,0)",
		},
		{
			name: "exact bottom-right corner",
			u:    1.0, v: 1.0,
			checkFunc: func(r, g, b, a byte) bool {
				return r == 255 && g == 255 && b == 0 && a == 255
			},
			desc: "should be yellow (255,255,0)",
		},
		{
			name: "center between all 4 pixels",
			u:    0.5, v: 0.5,
			checkFunc: func(r, g, b, a byte) bool {
				// Average of (0,0,0), (255,0,0), (0,255,0), (255,255,0)
				// R: (0+255+0+255)/4 = 127.5 ≈ 127 or 128
				// G: (0+0+255+255)/4 = 127.5 ≈ 127 or 128
				// B: 0
				return (r >= 127 && r <= 128) && (g >= 127 && g <= 128) && b == 0 && a == 255
			},
			desc: "should be average of all corners (~127,~127,0)",
		},
		{
			name: "halfway between top corners",
			u:    0.5, v: 0.0,
			checkFunc: func(r, g, b, a byte) bool {
				// Average of (0,0,0) and (255,0,0)
				return (r >= 127 && r <= 128) && g == 0 && b == 0 && a == 255
			},
			desc: "should be between black and red",
		},
		{
			name: "halfway between left corners",
			u:    0.0, v: 0.5,
			checkFunc: func(r, g, b, a byte) bool {
				// Average of (0,0,0) and (0,255,0)
				return r == 0 && (g >= 127 && g <= 128) && b == 0 && a == 255
			},
			desc: "should be between black and green",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := SampleBilinear(img, tt.u, tt.v)
			if !tt.checkFunc(r, g, b, a) {
				t.Errorf("SampleBilinear(%v, %v) = (%d,%d,%d,%d), %s",
					tt.u, tt.v, r, g, b, a, tt.desc)
			}
		})
	}
}

// TestSampleBilinearSmooth tests that bilinear produces smooth gradients.
func TestSampleBilinearSmooth(t *testing.T) {
	// Create a 2x2 image: black -> white gradient
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	_ = img.SetRGBA(0, 0, 0, 0, 0, 255)
	_ = img.SetRGBA(1, 0, 255, 255, 255, 255)
	_ = img.SetRGBA(0, 1, 0, 0, 0, 255)
	_ = img.SetRGBA(1, 1, 255, 255, 255, 255)

	// Sample along a horizontal line
	prevR := byte(0)
	for i := 0; i <= 10; i++ {
		u := float64(i) / 10.0
		r, _, _, _ := SampleBilinear(img, u, 0.5)

		// Values should be monotonically increasing
		if i > 0 && r < prevR {
			t.Errorf("Non-monotonic gradient at u=%v: r=%d, prevR=%d", u, r, prevR)
		}
		prevR = r
	}
}

// TestSampleBicubic tests bicubic interpolation.
func TestSampleBicubic(t *testing.T) {
	// Create a 4x4 test image
	img, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	// Fill with gradient
	for y := range 4 {
		for x := range 4 {
			val := byte((x + y) * 32)
			_ = img.SetRGBA(x, y, val, val, val, 255)
		}
	}

	tests := []struct {
		name      string
		u, v      float64
		checkFunc func(r, g, b, a byte) bool
		desc      string
	}{
		{
			name: "exact pixel center",
			u:    0.375, v: 0.375,
			checkFunc: func(r, g, b, a byte) bool {
				// Should be close to pixel (1,1) = 64
				return r >= 60 && r <= 68 && a == 255
			},
			desc: "should be close to pixel value",
		},
		{
			name: "between pixels",
			u:    0.5, v: 0.5,
			checkFunc: func(r, g, b, a byte) bool {
				// Should produce smooth interpolation
				return r > 0 && r < 255 && a == 255
			},
			desc: "should interpolate smoothly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := SampleBicubic(img, tt.u, tt.v)
			if !tt.checkFunc(r, g, b, a) {
				t.Errorf("SampleBicubic(%v, %v) = (%d,%d,%d,%d), %s",
					tt.u, tt.v, r, g, b, a, tt.desc)
			}
		})
	}
}

// TestSampleBicubicSmooth tests that bicubic produces smooth gradients.
func TestSampleBicubicSmooth(t *testing.T) {
	// Create a 4x4 image with linear gradient
	img, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	for y := range 4 {
		for x := range 4 {
			val := byte(x * 64)
			_ = img.SetRGBA(x, y, val, 0, 0, 255)
		}
	}

	// Sample along a line and check for smoothness
	samples := make([]byte, 20)
	for i := range 20 {
		u := float64(i) / 19.0
		r, _, _, _ := SampleBicubic(img, u, 0.5)
		samples[i] = r
	}

	// Check that values don't oscillate wildly
	for i := 1; i < len(samples)-1; i++ {
		// Second derivative shouldn't be too large
		d2 := math.Abs(float64(samples[i+1]) - 2*float64(samples[i]) + float64(samples[i-1]))
		if d2 > 50 {
			t.Errorf("Large oscillation at sample %d: d2=%v", i, d2)
		}
	}
}

// TestSampleDispatch tests the Sample dispatch function.
func TestSampleDispatch(t *testing.T) {
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf failed: %v", err)
	}

	_ = img.SetRGBA(0, 0, 100, 100, 100, 255)
	_ = img.SetRGBA(1, 0, 200, 200, 200, 255)
	_ = img.SetRGBA(0, 1, 100, 100, 100, 255)
	_ = img.SetRGBA(1, 1, 200, 200, 200, 255)

	tests := []struct {
		name string
		mode InterpolationMode
	}{
		{"nearest mode", InterpNearest},
		{"bilinear mode", InterpBilinear},
		{"bicubic mode", InterpBicubic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1, _, _, a1 := Sample(img, 0.5, 0.5, tt.mode)

			// Verify it produces valid output
			if a1 != 255 {
				t.Errorf("Sample with %s produced invalid alpha: %d", tt.mode, a1)
			}

			// Verify it's between the two extremes
			if r1 < 100 || r1 > 200 {
				t.Errorf("Sample with %s produced out-of-range value: %d", tt.mode, r1)
			}
		})
	}
}

// TestSampleAllFormats tests sampling with different pixel formats.
func TestSampleAllFormats(t *testing.T) {
	formats := []Format{
		FormatGray8,
		FormatRGB8,
		FormatRGBA8,
		FormatBGRA8,
	}

	for _, format := range formats {
		t.Run(format.String(), func(t *testing.T) {
			img, err := NewImageBuf(4, 4, format)
			if err != nil {
				t.Fatalf("NewImageBuf failed: %v", err)
			}

			// Fill with gradient
			for y := range 4 {
				for x := range 4 {
					val := byte((x + y) * 32)
					_ = img.SetRGBA(x, y, val, val, val, 255)
				}
			}

			// Test each interpolation mode
			modes := []InterpolationMode{InterpNearest, InterpBilinear, InterpBicubic}
			for _, mode := range modes {
				r, g, b, a := Sample(img, 0.5, 0.5, mode)

				// Basic sanity checks
				if !format.HasAlpha() && a != 255 {
					t.Errorf("Format %s should have alpha=255, got %d", format, a)
				}

				// For grayscale, r==g==b
				if format.IsGrayscale() && (r != g || r != b) {
					t.Errorf("Grayscale format should have r==g==b, got (%d,%d,%d)", r, g, b)
				}
			}
		})
	}
}

// TestInterpolationModeString tests the String method.
func TestInterpolationModeString(t *testing.T) {
	tests := []struct {
		mode InterpolationMode
		want string
	}{
		{InterpNearest, "Nearest"},
		{InterpBilinear, "Bilinear"},
		{InterpBicubic, "Bicubic"},
		{InterpolationMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("mode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// BenchmarkSampleNearest benchmarks nearest-neighbor sampling.
func BenchmarkSampleNearest(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	b.ResetTimer()
	for i := range b.N {
		u := float64(i%256) / 256.0
		v := float64((i/256)%256) / 256.0
		SampleNearest(img, u, v)
	}
}

// BenchmarkSampleBilinear benchmarks bilinear sampling.
func BenchmarkSampleBilinear(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	b.ResetTimer()
	for i := range b.N {
		u := float64(i%256) / 256.0
		v := float64((i/256)%256) / 256.0
		SampleBilinear(img, u, v)
	}
}

// BenchmarkSampleBicubic benchmarks bicubic sampling.
func BenchmarkSampleBicubic(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	b.ResetTimer()
	for i := range b.N {
		u := float64(i%256) / 256.0
		v := float64((i/256)%256) / 256.0
		SampleBicubic(img, u, v)
	}
}

// BenchmarkSampleDispatch benchmarks the dispatch function.
func BenchmarkSampleDispatch(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)

	modes := []InterpolationMode{InterpNearest, InterpBilinear, InterpBicubic}

	for _, mode := range modes {
		b.Run(mode.String(), func(b *testing.B) {
			b.ResetTimer()
			for i := range b.N {
				u := float64(i%256) / 256.0
				v := float64((i/256)%256) / 256.0
				Sample(img, u, v, mode)
			}
		})
	}
}
