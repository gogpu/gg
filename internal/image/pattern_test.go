package image

import (
	"math"
	"testing"
)

// TestNewImagePattern tests pattern creation with defaults.
func TestNewImagePattern(t *testing.T) {
	img, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	pattern := NewImagePattern(img)
	if pattern == nil {
		t.Fatal("expected non-nil pattern")
	}

	// Check defaults
	if pattern.Image() != img {
		t.Error("image not set correctly")
	}

	if pattern.SpreadMode() != SpreadPad {
		t.Errorf("expected SpreadPad, got %v", pattern.SpreadMode())
	}

	if pattern.Interpolation() != InterpBilinear {
		t.Errorf("expected InterpBilinear, got %v", pattern.Interpolation())
	}

	if pattern.Opacity() != 1.0 {
		t.Errorf("expected opacity 1.0, got %v", pattern.Opacity())
	}

	if pattern.Mipmaps() != nil {
		t.Error("expected nil mipmaps by default")
	}

	// Transform should be identity
	identity := Identity()
	if pattern.Transform() != identity {
		t.Error("expected identity transform by default")
	}
}

// TestNewImagePattern_NilImage tests pattern creation with nil image.
func TestNewImagePattern_NilImage(t *testing.T) {
	pattern := NewImagePattern(nil)
	if pattern != nil {
		t.Error("expected nil pattern for nil image")
	}
}

// TestImagePattern_BuilderChaining tests method chaining.
func TestImagePattern_BuilderChaining(t *testing.T) {
	img, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	transform := Scale(2, 2)
	mipmaps := GenerateMipmaps(img)

	pattern := NewImagePattern(img).
		WithTransform(transform).
		WithSpreadMode(SpreadRepeat).
		WithInterpolation(InterpBicubic).
		WithOpacity(0.5).
		WithMipmaps(mipmaps)

	if pattern == nil {
		t.Fatal("expected non-nil pattern")
	}

	if pattern.Transform() != transform {
		t.Error("transform not set correctly")
	}

	if pattern.SpreadMode() != SpreadRepeat {
		t.Error("spread mode not set correctly")
	}

	if pattern.Interpolation() != InterpBicubic {
		t.Error("interpolation not set correctly")
	}

	if pattern.Opacity() != 0.5 {
		t.Error("opacity not set correctly")
	}

	if pattern.Mipmaps() != mipmaps {
		t.Error("mipmaps not set correctly")
	}
}

// TestImagePattern_SpreadModePad tests pad/clamp spread mode.
func TestImagePattern_SpreadModePad(t *testing.T) {
	// Create a 2x2 image with distinct corner colors
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	// Set corner colors (R, G, B, A)
	_ = img.SetRGBA(0, 0, 255, 0, 0, 255)   // Top-left: red
	_ = img.SetRGBA(1, 0, 0, 255, 0, 255)   // Top-right: green
	_ = img.SetRGBA(0, 1, 0, 0, 255, 255)   // Bottom-left: blue
	_ = img.SetRGBA(1, 1, 255, 255, 0, 255) // Bottom-right: yellow

	pattern := NewImagePattern(img).
		WithSpreadMode(SpreadPad).
		WithInterpolation(InterpNearest)

	tests := []struct {
		name  string
		x, y  float64
		wantR byte
		wantG byte
		wantB byte
		wantA byte
	}{
		{"top-left corner", 0, 0, 255, 0, 0, 255},
		{"top-right corner", 1, 0, 0, 255, 0, 255},
		{"bottom-left corner", 0, 1, 0, 0, 255, 255},
		{"bottom-right corner", 1, 1, 255, 255, 0, 255},

		// Out of bounds - should clamp to edges
		{"left of image", -0.5, 0.25, 255, 0, 0, 255}, // Clamps to left edge
		{"right of image", 1.5, 0.25, 0, 255, 0, 255}, // Clamps to right edge
		{"above image", 0.25, -0.5, 255, 0, 0, 255},   // Clamps to top edge
		{"below image", 0.25, 1.5, 0, 0, 255, 255},    // Clamps to bottom edge
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := pattern.Sample(tt.x, tt.y)
			if r != tt.wantR || g != tt.wantG || b != tt.wantB || a != tt.wantA {
				t.Errorf("Sample(%v, %v) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.x, tt.y, r, g, b, a, tt.wantR, tt.wantG, tt.wantB, tt.wantA)
			}
		})
	}
}

// TestImagePattern_SpreadModeRepeat tests repeat/tile spread mode.
func TestImagePattern_SpreadModeRepeat(t *testing.T) {
	// Create a 2x2 image with distinct corner colors
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	_ = img.SetRGBA(0, 0, 255, 0, 0, 255)   // Top-left: red
	_ = img.SetRGBA(1, 0, 0, 255, 0, 255)   // Top-right: green
	_ = img.SetRGBA(0, 1, 0, 0, 255, 255)   // Bottom-left: blue
	_ = img.SetRGBA(1, 1, 255, 255, 0, 255) // Bottom-right: yellow

	pattern := NewImagePattern(img).
		WithSpreadMode(SpreadRepeat).
		WithInterpolation(InterpNearest)

	tests := []struct {
		name  string
		x, y  float64
		wantR byte
		wantG byte
		wantB byte
		wantA byte
	}{
		// First tile (0 to 1)
		{"first tile TL", 0, 0, 255, 0, 0, 255},
		{"first tile TR", 1, 0, 255, 0, 0, 255}, // Wraps to 0
		{"first tile BL", 0, 1, 255, 0, 0, 255}, // Wraps to 0
		{"first tile BR", 1, 1, 255, 0, 0, 255}, // Wraps to 0,0

		// Second tile (1 to 2) - should repeat
		{"second tile TL", 1.0, 0.25, 255, 0, 0, 255}, // u=0, v=0.25
		{"second tile TR", 2.0, 0.25, 255, 0, 0, 255}, // u=0, v=0.25

		// Negative coords - should also repeat
		{"negative tile", -0.5, -0.5, 255, 255, 0, 255}, // u=0.5, v=0.5 -> pixel (1,1) = yellow
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := pattern.Sample(tt.x, tt.y)
			if r != tt.wantR || g != tt.wantG || b != tt.wantB || a != tt.wantA {
				t.Errorf("Sample(%v, %v) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.x, tt.y, r, g, b, a, tt.wantR, tt.wantG, tt.wantB, tt.wantA)
			}
		})
	}
}

// TestImagePattern_SpreadModeReflect tests reflect/mirror spread mode.
func TestImagePattern_SpreadModeReflect(t *testing.T) {
	// Create a simple gradient image: 0 -> 255
	img, err := NewImageBuf(4, 1, FormatGray8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	_ = img.SetRGBA(0, 0, 0, 0, 0, 255)
	_ = img.SetRGBA(1, 0, 85, 85, 85, 255)
	_ = img.SetRGBA(2, 0, 170, 170, 170, 255)
	_ = img.SetRGBA(3, 0, 255, 255, 255, 255)

	pattern := NewImagePattern(img).
		WithSpreadMode(SpreadReflect).
		WithInterpolation(InterpNearest)

	tests := []struct {
		name string
		x    float64
		want byte // Grayscale value
	}{
		// First period (0 to 1) - normal
		{"period 0 start", 0.0, 0},
		{"period 0 mid", 0.5, 170}, // u=0.5 -> pixel floor(0.5*4) = 2 -> 170
		{"period 0 end", 0.99, 255},

		// Second period (1 to 2) - reflected
		{"period 1 start", 1.0, 255}, // Reflects
		{"period 1 mid", 1.5, 170},
		{"period 1 end", 1.99, 0},

		// Third period (2 to 3) - normal again
		{"period 2 start", 2.0, 0},
		{"period 2 mid", 2.5, 170}, // u=0.5 -> pixel 2 -> 170
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _, _, _ := pattern.Sample(tt.x, 0)
			// Allow some tolerance for nearest-neighbor sampling
			if absDiff(int(r), int(tt.want)) > 30 {
				t.Errorf("Sample(%v, 0) = %d, want ~%d", tt.x, r, tt.want)
			}
		})
	}
}

// TestImagePattern_Transform tests transform application.
func TestImagePattern_Transform(t *testing.T) {
	// Create a 4x4 image with a red 2x2 square in top-left
	img, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	// Fill with blue
	img.Fill(0, 0, 255, 255)

	// Draw red square in top-left quadrant
	_ = img.SetRGBA(0, 0, 255, 0, 0, 255)
	_ = img.SetRGBA(1, 0, 255, 0, 0, 255)
	_ = img.SetRGBA(0, 1, 255, 0, 0, 255)
	_ = img.SetRGBA(1, 1, 255, 0, 0, 255)

	// Scale by 2 - should make the pattern larger (red square covers more)
	pattern := NewImagePattern(img).
		WithTransform(Scale(0.5, 0.5)). // Inverse: 2x scale in pattern space
		WithInterpolation(InterpNearest)

	// Sample at (0.125, 0.125) in pattern space
	// With 0.5 scale: image coords = (0.25, 0.25) which should be red
	r, g, b, _ := pattern.Sample(0.125, 0.125)
	if r != 255 || g != 0 || b != 0 {
		t.Errorf("Sample(0.125, 0.125) = (%d,%d,%d), want red (255,0,0)", r, g, b)
	}

	// Sample at (0.75, 0.75) in pattern space
	// With 0.5 scale: image coords = (1.5, 1.5) which should be blue
	r, g, b, _ = pattern.Sample(0.75, 0.75)
	if r != 0 || g != 0 || b != 255 {
		t.Errorf("Sample(0.75, 0.75) = (%d,%d,%d), want blue (0,0,255)", r, g, b)
	}
}

// TestImagePattern_Opacity tests opacity blending.
func TestImagePattern_Opacity(t *testing.T) {
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	// Solid red with full alpha
	img.Fill(255, 0, 0, 255)

	tests := []struct {
		name    string
		opacity float64
		wantA   byte
	}{
		{"full opacity", 1.0, 255},
		{"half opacity", 0.5, 127},    // 255 * 0.5 = 127.5 -> 127
		{"quarter opacity", 0.25, 63}, // 255 * 0.25 = 63.75 -> 63
		{"zero opacity", 0.0, 0},
		{"over 1.0 clamped", 1.5, 255}, // Should clamp to 1.0
		{"negative clamped", -0.5, 0},  // Should clamp to 0.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := NewImagePattern(img).
				WithOpacity(tt.opacity).
				WithInterpolation(InterpNearest)

			_, _, _, a := pattern.Sample(0.5, 0.5)
			if absDiff(int(a), int(tt.wantA)) > 1 {
				t.Errorf("opacity %v: got alpha %d, want %d", tt.opacity, a, tt.wantA)
			}
		})
	}
}

// TestImagePattern_WithMipmaps tests mipmap integration.
func TestImagePattern_WithMipmaps(t *testing.T) {
	img, err := NewImageBuf(16, 16, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	img.Fill(255, 0, 0, 255) // Red

	mipmaps := GenerateMipmaps(img)
	if mipmaps == nil {
		t.Fatal("failed to generate mipmaps")
	}

	pattern := NewImagePattern(img).WithMipmaps(mipmaps)

	if pattern.Mipmaps() != mipmaps {
		t.Error("mipmaps not set correctly")
	}

	// Test SampleWithScale with different scales
	tests := []struct {
		name  string
		scale float64
		level int
	}{
		{"full size", 1.0, 0},
		{"half size", 0.5, 1},
		{"quarter size", 0.25, 2},
		{"eighth size", 0.125, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't crash and returns red
			r, g, b, a := pattern.SampleWithScale(0.5, 0.5, tt.scale)
			if r != 255 || g != 0 || b != 0 || a != 255 {
				t.Errorf("SampleWithScale(scale=%v) = (%d,%d,%d,%d), want red",
					tt.scale, r, g, b, a)
			}
		})
	}
}

// TestImagePattern_SampleWithScale_NoMipmaps tests fallback when no mipmaps.
func TestImagePattern_SampleWithScale_NoMipmaps(t *testing.T) {
	img, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	img.Fill(0, 255, 0, 255) // Green

	pattern := NewImagePattern(img)

	// Should fall back to regular sampling
	r, g, b, a := pattern.SampleWithScale(0.5, 0.5, 0.25)
	if r != 0 || g != 255 || b != 0 || a != 255 {
		t.Errorf("SampleWithScale without mipmaps = (%d,%d,%d,%d), want green", r, g, b, a)
	}
}

// TestImagePattern_NilPattern tests nil-safety.
func TestImagePattern_NilPattern(t *testing.T) {
	var pattern *ImagePattern

	// All methods should be nil-safe
	r, g, b, a := pattern.Sample(0.5, 0.5)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Error("nil pattern Sample should return (0,0,0,0)")
	}

	r, g, b, a = pattern.SampleWithScale(0.5, 0.5, 0.5)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Error("nil pattern SampleWithScale should return (0,0,0,0)")
	}

	if pattern.Image() != nil {
		t.Error("nil pattern Image should return nil")
	}

	if pattern.Transform() != Identity() {
		t.Error("nil pattern Transform should return identity")
	}

	if pattern.SpreadMode() != SpreadPad {
		t.Error("nil pattern SpreadMode should return SpreadPad")
	}

	if pattern.Interpolation() != InterpBilinear {
		t.Error("nil pattern Interpolation should return InterpBilinear")
	}

	if pattern.Opacity() != 1.0 {
		t.Error("nil pattern Opacity should return 1.0")
	}

	if pattern.Mipmaps() != nil {
		t.Error("nil pattern Mipmaps should return nil")
	}
}

// TestReflectCoord tests the reflection coordinate calculation.
func TestReflectCoord(t *testing.T) {
	tests := []struct {
		name  string
		coord float64
		want  float64
	}{
		// Period 0 (0 to 1) - even
		{"period 0 start", 0.0, 0.0},
		{"period 0 mid", 0.5, 0.5},
		{"period 0 end", 0.9, 0.9},

		// Period 1 (1 to 2) - odd (reflected)
		{"period 1 start", 1.0, 1.0},
		{"period 1 quarter", 1.25, 0.75},
		{"period 1 half", 1.5, 0.5},
		{"period 1 end", 1.9, 0.1},

		// Period 2 (2 to 3) - even
		{"period 2 start", 2.0, 0.0},
		{"period 2 mid", 2.5, 0.5},

		// Period 3 (3 to 4) - odd (reflected)
		{"period 3 start", 3.0, 1.0},
		{"period 3 mid", 3.5, 0.5},

		// Negative coords
		{"negative period -1", -0.5, 0.5}, // Period -1 is odd (reflected)
		{"negative period -2", -1.5, 0.5}, // Period -2 is even
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reflectCoord(tt.coord)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("reflectCoord(%v) = %v, want %v", tt.coord, got, tt.want)
			}
		})
	}
}

// TestSpreadMode_String tests string representation.
func TestSpreadMode_String(t *testing.T) {
	tests := []struct {
		mode SpreadMode
		want string
	}{
		{SpreadPad, "Pad"},
		{SpreadRepeat, "Repeat"},
		{SpreadReflect, "Reflect"},
		{SpreadMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("SpreadMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// Benchmarks

// BenchmarkImagePattern_Sample benchmarks basic sampling.
func BenchmarkImagePattern_Sample(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	img.Fill(128, 128, 128, 255)

	pattern := NewImagePattern(img).
		WithInterpolation(InterpBilinear)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := float64(i%256) / 256.0
		y := float64(i/256%256) / 256.0
		_, _, _, _ = pattern.Sample(x, y)
	}
}

// BenchmarkImagePattern_SampleWithScale benchmarks mipmap sampling.
func BenchmarkImagePattern_SampleWithScale(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	img.Fill(128, 128, 128, 255)

	mipmaps := GenerateMipmaps(img)
	pattern := NewImagePattern(img).
		WithMipmaps(mipmaps).
		WithInterpolation(InterpBilinear)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := float64(i%256) / 256.0
		y := float64(i/256%256) / 256.0
		scale := 0.5
		_, _, _, _ = pattern.SampleWithScale(x, y, scale)
	}
}

// BenchmarkImagePattern_SpreadRepeat benchmarks repeat spread mode.
func BenchmarkImagePattern_SpreadRepeat(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	img.Fill(128, 128, 128, 255)

	pattern := NewImagePattern(img).
		WithSpreadMode(SpreadRepeat).
		WithInterpolation(InterpBilinear)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := float64(i%512) / 256.0 // Goes beyond [0,1]
		y := float64(i/512%512) / 256.0
		_, _, _, _ = pattern.Sample(x, y)
	}
}

// BenchmarkImagePattern_SpreadReflect benchmarks reflect spread mode.
func BenchmarkImagePattern_SpreadReflect(b *testing.B) {
	img, _ := NewImageBuf(256, 256, FormatRGBA8)
	img.Fill(128, 128, 128, 255)

	pattern := NewImagePattern(img).
		WithSpreadMode(SpreadReflect).
		WithInterpolation(InterpBilinear)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := float64(i%512) / 256.0 // Goes beyond [0,1]
		y := float64(i/512%512) / 256.0
		_, _, _, _ = pattern.Sample(x, y)
	}
}

// Helper functions

// absDiff returns the absolute difference between two integers.
func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
