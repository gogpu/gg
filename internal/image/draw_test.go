package image

import (
	"math"
	"testing"
)

func TestBlendModeString(t *testing.T) {
	tests := []struct {
		mode BlendMode
		want string
	}{
		{BlendNormal, "Normal"},
		{BlendMultiply, "Multiply"},
		{BlendScreen, "Screen"},
		{BlendOverlay, "Overlay"},
		{BlendMode(99), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.mode.String()
		if got != tt.want {
			t.Errorf("BlendMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestDrawImageIdentity(t *testing.T) {
	// Create a simple source image: 2x2 red square
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	src.Fill(255, 0, 0, 255) // Red

	// Create destination: 4x4 black
	dst, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Clear()

	// Draw source at (1, 1) with identity transform
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 1, Y: 1, Width: 2, Height: 2},
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	})

	// Check that (1,1), (2,1), (1,2), (2,2) are red
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 2; x++ {
			r, g, b, a := dst.GetRGBA(x, y)
			if r != 255 || g != 0 || b != 0 || a != 255 {
				t.Errorf("Pixel (%d, %d) = (%d, %d, %d, %d), want red (255, 0, 0, 255)",
					x, y, r, g, b, a)
			}
		}
	}

	// Check that corners remain black
	corners := [][2]int{{0, 0}, {3, 0}, {0, 3}, {3, 3}}
	for _, c := range corners {
		r, g, b, a := dst.GetRGBA(c[0], c[1])
		if r != 0 || g != 0 || b != 0 || a != 0 {
			t.Errorf("Corner pixel (%d, %d) = (%d, %d, %d, %d), want black (0, 0, 0, 0)",
				c[0], c[1], r, g, b, a)
		}
	}
}

func TestDrawImageScale(t *testing.T) {
	// Create a 2x2 source with a checkboard pattern
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	_ = src.SetRGBA(0, 0, 255, 0, 0, 255)   // Red
	_ = src.SetRGBA(1, 0, 0, 255, 0, 255)   // Green
	_ = src.SetRGBA(0, 1, 0, 0, 255, 255)   // Blue
	_ = src.SetRGBA(1, 1, 255, 255, 255, 255) // White

	// Create 4x4 destination
	dst, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Clear()

	// Scale 2x2 source to 4x4 destination
	scale := Scale(2, 2)
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 4, Height: 4},
		Transform: &scale,
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	})

	// With 2x scale and nearest interpolation, each source pixel should
	// map to a 2x2 block in the destination

	// Top-left quadrant should be red (from src[0,0])
	r, g, b, _ := dst.GetRGBA(0, 0)
	if r != 255 || g != 0 || b != 0 {
		t.Errorf("Top-left quadrant should be red, got (%d, %d, %d)", r, g, b)
	}
}

func TestDrawImageRotate(t *testing.T) {
	// Create a 10x10 source with a red cross in the center
	src, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	src.Clear()

	// Draw horizontal line at y=5
	for x := 0; x < 10; x++ {
		_ = src.SetRGBA(x, 5, 255, 0, 0, 255)
	}
	// Draw vertical line at x=5
	for y := 0; y < 10; y++ {
		_ = src.SetRGBA(5, y, 255, 0, 0, 255)
	}

	// Create destination
	dst, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Clear()

	// Rotate 45 degrees around center (5, 5)
	rotate := RotateAt(math.Pi/4, 5, 5)
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 10, Height: 10},
		Transform: &rotate,
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	})

	// Center pixel should still be red
	r, g, b, a := dst.GetRGBA(5, 5)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("Center pixel should be red after rotation, got (%d, %d, %d, %d)", r, g, b, a)
	}

	// The cross should now be rotated 45 degrees (forming an X)
	// We can verify that some pixels along the diagonals are now red
	// This is a basic sanity check
}

func TestDrawImageOpacity(t *testing.T) {
	// Create solid red source
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	src.Fill(255, 0, 0, 255)

	// Create solid blue destination
	dst, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Fill(0, 0, 255, 255)

	// Draw with 50% opacity
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 2, Height: 2},
		Interp:    InterpNearest,
		Opacity:   0.5,
		BlendMode: BlendNormal,
	})

	// Result should be a blend of red and blue
	r, g, b, _ := dst.GetRGBA(0, 0)

	// With 50% opacity, we expect some red and some blue
	if r == 0 || b == 0 {
		t.Errorf("Expected blend of red and blue, got (%d, %d, %d)", r, g, b)
	}
	if r == 255 || b == 255 {
		t.Errorf("Expected partial blend, got (%d, %d, %d)", r, g, b)
	}
}

func TestDrawImageBlendMultiply(t *testing.T) {
	// Create source: gray (128, 128, 128)
	src, err := NewImageBuf(1, 1, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	_ = src.SetRGBA(0, 0, 128, 128, 128, 255)

	// Create destination: gray (128, 128, 128)
	dst, err := NewImageBuf(1, 1, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	_ = dst.SetRGBA(0, 0, 128, 128, 128, 255)

	// Multiply blend: 128 * 128 / 255 ≈ 64
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 1, Height: 1},
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendMultiply,
	})

	r, g, b, _ := dst.GetRGBA(0, 0)
	expected := uint8(64) // 128 * 128 / 255 ≈ 64

	// Allow some tolerance for rounding
	tolerance := uint8(2)
	if abs(int(r)-int(expected)) > int(tolerance) ||
		abs(int(g)-int(expected)) > int(tolerance) ||
		abs(int(b)-int(expected)) > int(tolerance) {
		t.Errorf("BlendMultiply result = (%d, %d, %d), want ≈(%d, %d, %d)",
			r, g, b, expected, expected, expected)
	}
}

func TestDrawImageBlendScreen(t *testing.T) {
	// Create source: gray (128, 128, 128)
	src, err := NewImageBuf(1, 1, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	_ = src.SetRGBA(0, 0, 128, 128, 128, 255)

	// Create destination: gray (128, 128, 128)
	dst, err := NewImageBuf(1, 1, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	_ = dst.SetRGBA(0, 0, 128, 128, 128, 255)

	// Screen blend: 255 - (127 * 127 / 255) = 255 - 63 = 192
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 1, Height: 1},
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendScreen,
	})

	r, g, b, _ := dst.GetRGBA(0, 0)

	// Screen should make the result lighter than both inputs (all channels)
	if r <= 128 || g <= 128 || b <= 128 {
		t.Errorf("BlendScreen should lighten all channels: got (%d, %d, %d), want all > 128", r, g, b)
	}
}

func TestDrawImageSrcRect(t *testing.T) {
	// Create a 4x4 source with different colors in each quadrant
	src, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	src.Clear()

	// Top-left quadrant: red
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			_ = src.SetRGBA(x, y, 255, 0, 0, 255)
		}
	}

	// Top-right quadrant: green
	for y := 0; y < 2; y++ {
		for x := 2; x < 4; x++ {
			_ = src.SetRGBA(x, y, 0, 255, 0, 255)
		}
	}

	// Create destination
	dst, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Clear()

	// Draw only the top-left quadrant of source
	srcRect := Rect{X: 0, Y: 0, Width: 2, Height: 2}
	DrawImage(dst, src, DrawParams{
		SrcRect:   &srcRect,
		DstRect:   Rect{X: 0, Y: 0, Width: 2, Height: 2},
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	})

	// All destination pixels should be red (from top-left quadrant of source)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			r, g, b, _ := dst.GetRGBA(x, y)
			if r != 255 || g != 0 || b != 0 {
				t.Errorf("Pixel (%d, %d) = (%d, %d, %d), want red (255, 0, 0)",
					x, y, r, g, b)
			}
		}
	}
}

func TestDrawImageClipping(t *testing.T) {
	// Create a small source
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	src.Fill(255, 0, 0, 255)

	// Create destination
	dst, err := NewImageBuf(4, 4, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Clear()

	// Try to draw outside bounds (should be clipped)
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: -1, Y: -1, Width: 3, Height: 3},
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	})

	// Only the portion inside bounds should be drawn
	// Top-left corner (0, 0) should have some color
	r, g, b, a := dst.GetRGBA(0, 0)
	if r == 0 && g == 0 && b == 0 && a == 0 {
		t.Error("Expected some drawing at (0, 0) after clipping")
	}
}

func TestDrawImageSingularTransform(t *testing.T) {
	// Create source and destination
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	src.Fill(255, 0, 0, 255)

	dst, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}
	dst.Fill(0, 0, 255, 255) // Blue

	// Create a singular (non-invertible) transform
	singular := Affine{a: 1, b: 2, d: 2, e: 4} // Rows are proportional

	// Drawing should have no effect (transform can't be inverted)
	DrawImage(dst, src, DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 2, Height: 2},
		Transform: &singular,
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	})

	// Destination should remain unchanged (blue)
	r, g, b, _ := dst.GetRGBA(0, 0)
	if r != 0 || g != 0 || b != 255 {
		t.Errorf("Destination should be unchanged with singular transform, got (%d, %d, %d)", r, g, b)
	}
}

func TestDrawImageInterpolation(t *testing.T) {
	// Create a 2x2 checkerboard source
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	_ = src.SetRGBA(0, 0, 0, 0, 0, 255)       // Black
	_ = src.SetRGBA(1, 0, 255, 255, 255, 255) // White
	_ = src.SetRGBA(0, 1, 255, 255, 255, 255) // White
	_ = src.SetRGBA(1, 1, 0, 0, 0, 255)       // Black

	// Scale up to 4x4 with different interpolation modes
	modes := []InterpolationMode{InterpNearest, InterpBilinear, InterpBicubic}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			dst, err := NewImageBuf(4, 4, FormatRGBA8)
			if err != nil {
				t.Fatalf("Failed to create destination: %v", err)
			}
			dst.Clear()

			scale := Scale(2, 2)
			DrawImage(dst, src, DrawParams{
				DstRect:   Rect{X: 0, Y: 0, Width: 4, Height: 4},
				Transform: &scale,
				Interp:    mode,
				Opacity:   1.0,
				BlendMode: BlendNormal,
			})

			// Just verify that something was drawn
			hasNonZero := false
			for y := 0; y < 4; y++ {
				for x := 0; x < 4; x++ {
					r, g, b, a := dst.GetRGBA(x, y)
					if r != 0 || g != 0 || b != 0 || a != 0 {
						hasNonZero = true
						break
					}
				}
			}

			if !hasNonZero {
				t.Errorf("Expected non-zero pixels with %s interpolation", mode)
			}
		})
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func BenchmarkDrawImageNearest(b *testing.B) {
	src, _ := NewImageBuf(100, 100, FormatRGBA8)
	src.Fill(255, 0, 0, 255)

	dst, _ := NewImageBuf(200, 200, FormatRGBA8)

	scale := Scale(2, 2)
	params := DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 200, Height: 200},
		Transform: &scale,
		Interp:    InterpNearest,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawImage(dst, src, params)
	}
}

func BenchmarkDrawImageBilinear(b *testing.B) {
	src, _ := NewImageBuf(100, 100, FormatRGBA8)
	src.Fill(255, 0, 0, 255)

	dst, _ := NewImageBuf(200, 200, FormatRGBA8)

	scale := Scale(2, 2)
	params := DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 200, Height: 200},
		Transform: &scale,
		Interp:    InterpBilinear,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawImage(dst, src, params)
	}
}

func BenchmarkDrawImageBicubic(b *testing.B) {
	src, _ := NewImageBuf(100, 100, FormatRGBA8)
	src.Fill(255, 0, 0, 255)

	dst, _ := NewImageBuf(200, 200, FormatRGBA8)

	scale := Scale(2, 2)
	params := DrawParams{
		DstRect:   Rect{X: 0, Y: 0, Width: 200, Height: 200},
		Transform: &scale,
		Interp:    InterpBicubic,
		Opacity:   1.0,
		BlendMode: BlendNormal,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawImage(dst, src, params)
	}
}
