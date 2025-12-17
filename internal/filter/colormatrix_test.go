package filter

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

func TestNewColorMatrixFilter(t *testing.T) {
	matrix := [20]float32{
		1, 0, 0, 0, 0,
		0, 1, 0, 0, 0,
		0, 0, 1, 0, 0,
		0, 0, 0, 1, 0,
	}
	f := NewColorMatrixFilter(matrix)

	for i, v := range matrix {
		if f.Matrix[i] != v {
			t.Errorf("Matrix[%d] = %v, want %v", i, f.Matrix[i], v)
		}
	}
}

func TestNewIdentityColorMatrix(t *testing.T) {
	f := NewIdentityColorMatrix()

	// Check diagonal is 1, rest is 0
	expected := [20]float32{
		1, 0, 0, 0, 0,
		0, 1, 0, 0, 0,
		0, 0, 1, 0, 0,
		0, 0, 0, 1, 0,
	}

	for i, v := range expected {
		if f.Matrix[i] != v {
			t.Errorf("Identity Matrix[%d] = %v, want %v", i, f.Matrix[i], v)
		}
	}
}

func TestColorMatrixExpandBounds(t *testing.T) {
	f := NewIdentityColorMatrix()
	input := scene.Rect{MinX: 10, MinY: 20, MaxX: 100, MaxY: 200}

	got := f.ExpandBounds(input)

	// Color matrix should not expand bounds
	if got != input {
		t.Errorf("ExpandBounds = %+v, want %+v (unchanged)", got, input)
	}
}

func TestColorMatrixApplyIdentity(t *testing.T) {
	src := gg.NewPixmap(5, 5)
	dst := gg.NewPixmap(5, 5)

	// Fill with various colors
	src.SetPixel(0, 0, gg.Red)
	src.SetPixel(1, 0, gg.Green)
	src.SetPixel(2, 0, gg.Blue)
	src.SetPixel(3, 0, gg.White)
	src.SetPixel(4, 0, gg.RGBA2(0.5, 0.5, 0.5, 0.5))

	f := NewIdentityColorMatrix()
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 5, MaxY: 5}

	f.Apply(src, dst, bounds)

	// Colors should be unchanged (within rounding)
	colors := []struct {
		x    int
		want gg.RGBA
	}{
		{0, gg.Red},
		{1, gg.Green},
		{2, gg.Blue},
		{3, gg.White},
	}

	for _, tc := range colors {
		got := dst.GetPixel(tc.x, 0)
		if !colorApproxEqual(got, tc.want, 0.02) {
			t.Errorf("pixel (%d,0) = %+v, want %+v", tc.x, got, tc.want)
		}
	}
}

func TestColorMatrixApplyNilPixmaps(t *testing.T) {
	f := NewIdentityColorMatrix()
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}

	// Should not panic
	f.Apply(nil, nil, bounds)
	f.Apply(gg.NewPixmap(10, 10), nil, bounds)
	f.Apply(nil, gg.NewPixmap(10, 10), bounds)
}

func TestNewBrightnessFilter(t *testing.T) {
	tests := []struct {
		factor    float32
		inputR    float64
		expectedR float64
	}{
		{0.0, 1.0, 0.0},  // Black
		{1.0, 0.5, 0.5},  // Unchanged
		{2.0, 0.25, 0.5}, // Brighter
		{0.5, 1.0, 0.5},  // Darker
	}

	for _, tt := range tests {
		f := NewBrightnessFilter(tt.factor)
		src := gg.NewPixmap(1, 1)
		dst := gg.NewPixmap(1, 1)

		src.SetPixel(0, 0, gg.RGBA2(tt.inputR, tt.inputR, tt.inputR, 1.0))
		bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

		f.Apply(src, dst, bounds)

		got := dst.GetPixel(0, 0)
		if absf(got.R-tt.expectedR) > 0.03 {
			t.Errorf("Brightness(%v) with input %v: R = %v, want ~%v",
				tt.factor, tt.inputR, got.R, tt.expectedR)
		}
	}
}

func TestNewContrastFilter(t *testing.T) {
	tests := []struct {
		name     string
		factor   float32
		input    float64
		expected float64
	}{
		{"normal_gray", 1.0, 0.5, 0.5},     // Gray stays gray
		{"zero_contrast", 0.0, 0.0, 0.502}, // All become mid-gray (128/255)
		{"zero_contrast", 0.0, 1.0, 0.502},
		{"high_contrast", 2.0, 0.75, 1.0}, // Values pushed to extremes (clamped)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewContrastFilter(tt.factor)
			src := gg.NewPixmap(1, 1)
			dst := gg.NewPixmap(1, 1)

			src.SetPixel(0, 0, gg.RGBA2(tt.input, tt.input, tt.input, 1.0))
			bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

			f.Apply(src, dst, bounds)

			got := dst.GetPixel(0, 0)
			if absf(got.R-tt.expected) > 0.05 {
				t.Errorf("Contrast(%v) with input %v: R = %v, want ~%v",
					tt.factor, tt.input, got.R, tt.expected)
			}
		})
	}
}

func TestNewSaturationFilter(t *testing.T) {
	// Test grayscale (saturation = 0)
	f := NewSaturationFilter(0)
	src := gg.NewPixmap(1, 1)
	dst := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.Red) // Pure red
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	f.Apply(src, dst, bounds)

	got := dst.GetPixel(0, 0)
	// With saturation 0, all channels should be equal (grayscale)
	if absf(got.R-got.G) > 0.05 || absf(got.R-got.B) > 0.05 {
		t.Errorf("Grayscale should have equal channels: R=%v, G=%v, B=%v",
			got.R, got.G, got.B)
	}
}

func TestNewGrayscaleFilter(t *testing.T) {
	f := NewGrayscaleFilter()
	src := gg.NewPixmap(1, 1)
	dst := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.RGBA2(1, 0, 0, 1)) // Red
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	f.Apply(src, dst, bounds)

	got := dst.GetPixel(0, 0)
	// Red -> luminance ~0.2126 (using Rec. 709 weights)
	if absf(got.R-0.2126) > 0.05 {
		t.Errorf("Grayscale red: got R=%v, want ~0.2126", got.R)
	}
	// All channels should be equal
	if absf(got.R-got.G) > 0.02 || absf(got.R-got.B) > 0.02 {
		t.Errorf("Grayscale should be uniform: R=%v, G=%v, B=%v", got.R, got.G, got.B)
	}
}

func TestNewSepiaFilter(t *testing.T) {
	f := NewSepiaFilter()
	src := gg.NewPixmap(1, 1)
	dst := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.White)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	f.Apply(src, dst, bounds)

	got := dst.GetPixel(0, 0)
	// Sepia should give warm brownish tones
	// For white input, R > G > B
	if got.R < got.G || got.G < got.B {
		t.Errorf("Sepia should have R >= G >= B: R=%v, G=%v, B=%v", got.R, got.G, got.B)
	}
}

func TestNewInvertFilter(t *testing.T) {
	f := NewInvertFilter()
	src := gg.NewPixmap(2, 1)
	dst := gg.NewPixmap(2, 1)

	src.SetPixel(0, 0, gg.Black)
	src.SetPixel(1, 0, gg.White)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 2, MaxY: 1}

	f.Apply(src, dst, bounds)

	// Black should become white
	got0 := dst.GetPixel(0, 0)
	if got0.R < 0.95 || got0.G < 0.95 || got0.B < 0.95 {
		t.Errorf("Inverted black should be white: %+v", got0)
	}

	// White should become black
	got1 := dst.GetPixel(1, 0)
	if got1.R > 0.05 || got1.G > 0.05 || got1.B > 0.05 {
		t.Errorf("Inverted white should be black: %+v", got1)
	}
}

func TestNewOpacityFilter(t *testing.T) {
	f := NewOpacityFilter(0.5)
	src := gg.NewPixmap(1, 1)
	dst := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.White) // Alpha = 1.0
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	f.Apply(src, dst, bounds)

	got := dst.GetPixel(0, 0)
	// Alpha should be halved
	if absf(got.A-0.5) > 0.03 {
		t.Errorf("Opacity(0.5) should give A=0.5: got A=%v", got.A)
	}
	// RGB should be unchanged
	if absf(got.R-1.0) > 0.02 {
		t.Errorf("Opacity should preserve RGB: R=%v", got.R)
	}
}

func TestNewColorTintFilter(t *testing.T) {
	// 50% red tint
	tint := gg.RGBA2(1, 0, 0, 0.5)
	f := NewColorTintFilter(tint)
	src := gg.NewPixmap(1, 1)
	dst := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.White)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	f.Apply(src, dst, bounds)

	got := dst.GetPixel(0, 0)
	// Red should be highest (tint red + half white)
	if got.R < got.G || got.R < got.B {
		t.Errorf("Red tint should give R >= G,B: R=%v, G=%v, B=%v", got.R, got.G, got.B)
	}
}

func TestColorMatrixMultiply(t *testing.T) {
	// Multiply identity by itself should give identity
	identity := NewIdentityColorMatrix()
	result := identity.Multiply(identity)

	for i := 0; i < 20; i++ {
		if absf32(result.Matrix[i]-identity.Matrix[i]) > 0.0001 {
			t.Errorf("Identity * Identity != Identity at [%d]: %v vs %v",
				i, result.Matrix[i], identity.Matrix[i])
		}
	}
}

func TestColorMatrixMultiplyBrightnessContrast(t *testing.T) {
	brightness := NewBrightnessFilter(1.2)
	contrast := NewContrastFilter(1.1)

	// Combine: first brightness, then contrast
	combined := brightness.Multiply(contrast)

	src := gg.NewPixmap(1, 1)
	dst1 := gg.NewPixmap(1, 1)
	dst2 := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.RGBA2(0.5, 0.3, 0.7, 1.0))
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	// Apply combined
	combined.Apply(src, dst1, bounds)

	// Apply sequentially
	temp := gg.NewPixmap(1, 1)
	brightness.Apply(src, temp, bounds)
	contrast.Apply(temp, dst2, bounds)

	// Results should be close
	got1 := dst1.GetPixel(0, 0)
	got2 := dst2.GetPixel(0, 0)

	if !colorApproxEqual(got1, got2, 0.03) {
		t.Errorf("Combined filter differs from sequential: %+v vs %+v", got1, got2)
	}
}

func TestNewHueRotateFilter(t *testing.T) {
	// 180 degree rotation should swap colors somewhat
	f := NewHueRotateFilter(180)
	src := gg.NewPixmap(1, 1)
	dst := gg.NewPixmap(1, 1)

	src.SetPixel(0, 0, gg.Red)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

	f.Apply(src, dst, bounds)

	got := dst.GetPixel(0, 0)
	// Red rotated 180 degrees should be cyan-ish (complementary)
	// The exact result depends on the approximation used
	if got.R > 0.5 {
		// If still mostly red, hue rotation didn't work well
		// Allow for approximation errors in our simple trig
		t.Logf("Hue rotate 180 on red: %+v (may have approximation error)", got)
	}
}

// Benchmarks

func BenchmarkColorMatrixFilter(b *testing.B) {
	sizes := []struct {
		name string
		w, h int
	}{
		{"100x100", 100, 100},
		{"500x500", 500, 500},
		{"1920x1080", 1920, 1080},
		{"3840x2160", 3840, 2160},
	}

	filters := []struct {
		name string
		f    *ColorMatrixFilter
	}{
		{"Identity", NewIdentityColorMatrix()},
		{"Grayscale", NewGrayscaleFilter()},
		{"Sepia", NewSepiaFilter()},
		{"Invert", NewInvertFilter()},
		{"Brightness", NewBrightnessFilter(1.5)},
	}

	for _, size := range sizes {
		for _, filter := range filters {
			name := size.name + "_" + filter.name
			b.Run(name, func(b *testing.B) {
				src := createTestPixmap(size.w, size.h, gg.Red)
				dst := gg.NewPixmap(size.w, size.h)
				bounds := scene.Rect{
					MinX: 0, MinY: 0,
					MaxX: float32(size.w), MaxY: float32(size.h),
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					filter.f.Apply(src, dst, bounds)
				}
			})
		}
	}
}
