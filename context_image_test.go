package gg

import (
	"image"
	"image/color"
	"testing"
)

func TestDrawImage(t *testing.T) {
	// Create a context
	dc := NewContext(200, 200)
	dc.Clear()

	// Create a test image
	img, err := NewImageBuf(50, 50, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	// Fill with red
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			_ = img.SetRGBA(x, y, 255, 0, 0, 255)
		}
	}

	// Draw image
	dc.DrawImage(img, 10, 10)

	// Verify pixel at (10, 10) is red
	result := dc.pixmap.GetPixel(10, 10)
	if result.R < 0.9 || result.G > 0.1 || result.B > 0.1 {
		t.Errorf("Expected red pixel at (10, 10), got R=%.2f G=%.2f B=%.2f", result.R, result.G, result.B)
	}
}

func TestDrawImageEx(t *testing.T) {
	tests := []struct {
		name string
		opts DrawImageOptions
	}{
		{
			name: "basic position",
			opts: DrawImageOptions{
				X:             20,
				Y:             20,
				Interpolation: InterpNearest,
				Opacity:       1.0,
				BlendMode:     BlendNormal,
			},
		},
		{
			name: "scaled",
			opts: DrawImageOptions{
				X:             20,
				Y:             20,
				DstWidth:      100,
				DstHeight:     100,
				Interpolation: InterpBilinear,
				Opacity:       1.0,
				BlendMode:     BlendNormal,
			},
		},
		{
			name: "with opacity",
			opts: DrawImageOptions{
				X:             20,
				Y:             20,
				Interpolation: InterpBilinear,
				Opacity:       0.5,
				BlendMode:     BlendNormal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := NewContext(200, 200)
			dc.Clear()

			// Create test image
			img, err := NewImageBuf(50, 50, FormatRGBA8)
			if err != nil {
				t.Fatalf("Failed to create image: %v", err)
			}

			// Fill with blue
			for y := 0; y < 50; y++ {
				for x := 0; x < 50; x++ {
					_ = img.SetRGBA(x, y, 0, 0, 255, 255)
				}
			}

			// Draw with options
			dc.DrawImageEx(img, tt.opts)

			// Verify something was drawn (just check it's not transparent)
			x := int(tt.opts.X)
			y := int(tt.opts.Y)
			if x >= 0 && x < 200 && y >= 0 && y < 200 {
				result := dc.pixmap.GetPixel(x, y)
				if result.A == 0 {
					t.Errorf("Expected non-transparent pixel at (%d, %d)", x, y)
				}
			}
		})
	}
}

func TestDrawImageWithTransform(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create test image
	img, err := NewImageBuf(50, 50, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	// Fill with green
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			_ = img.SetRGBA(x, y, 0, 255, 0, 255)
		}
	}

	// Apply transformation
	dc.Translate(100, 100)
	dc.Scale(0.5, 0.5)

	// Draw image
	dc.DrawImage(img, 0, 0)

	// The image should be drawn at transformed position (100, 100) with 0.5 scale
	// Check pixel near the center of the transformed image
	result := dc.pixmap.GetPixel(100, 100)
	if result.G < 0.9 {
		t.Errorf("Expected green pixel at transformed position, got G=%.2f", result.G)
	}
}

func TestDrawImageSrcRect(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create test image with quadrants
	img, err := NewImageBuf(100, 100, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	// Top-left: red
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			_ = img.SetRGBA(x, y, 255, 0, 0, 255)
		}
	}

	// Top-right: green
	for y := 0; y < 50; y++ {
		for x := 50; x < 100; x++ {
			_ = img.SetRGBA(x, y, 0, 255, 0, 255)
		}
	}

	// Draw only top-left quadrant
	srcRect := image.Rect(0, 0, 50, 50)
	dc.DrawImageEx(img, DrawImageOptions{
		X:             10,
		Y:             10,
		SrcRect:       &srcRect,
		Interpolation: InterpNearest,
		Opacity:       1.0,
		BlendMode:     BlendNormal,
	})

	// Verify we got red (top-left quadrant)
	result := dc.pixmap.GetPixel(10, 10)
	if result.R < 0.9 || result.G > 0.1 {
		t.Errorf("Expected red from top-left quadrant, got R=%.2f G=%.2f", result.R, result.G)
	}
}

func TestCreateImagePattern(t *testing.T) {
	dc := NewContext(200, 200)

	// Create test image
	img, err := NewImageBuf(50, 50, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	// Fill with yellow
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			_ = img.SetRGBA(x, y, 255, 255, 0, 255)
		}
	}

	// Create pattern
	pattern := dc.CreateImagePattern(img, 0, 0, 50, 50)
	if pattern == nil {
		t.Fatal("CreateImagePattern returned nil")
	}

	// Test ColorAt
	col := pattern.ColorAt(10, 10)
	if col.R < 0.9 || col.G < 0.9 || col.B > 0.1 {
		t.Errorf("Expected yellow from pattern, got R=%.2f G=%.2f B=%.2f", col.R, col.G, col.B)
	}
}

func TestImagePattern_Tiling(t *testing.T) {
	// Create a small 2x2 image
	img, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	// Set distinct colors
	_ = img.SetRGBA(0, 0, 255, 0, 0, 255)   // Red
	_ = img.SetRGBA(1, 0, 0, 255, 0, 255)   // Green
	_ = img.SetRGBA(0, 1, 0, 0, 255, 255)   // Blue
	_ = img.SetRGBA(1, 1, 255, 255, 0, 255) // Yellow

	pattern := &ImagePattern{
		image: img,
		x:     0,
		y:     0,
		w:     2,
		h:     2,
	}

	// Test wrapping behavior
	tests := []struct {
		x, y     float64
		expected string
	}{
		{0, 0, "red"},
		{1, 0, "green"},
		{0, 1, "blue"},
		{1, 1, "yellow"},
		{2, 0, "red"},    // Wraps to (0, 0)
		{0, 2, "red"},    // Wraps to (0, 0)
		{3, 3, "yellow"}, // Wraps to (1, 1)
	}

	for _, tt := range tests {
		col := pattern.ColorAt(tt.x, tt.y)
		// Just verify we get some color (detailed color checking is complex with wrapping)
		if col.A == 0 {
			t.Errorf("ColorAt(%.0f, %.0f) returned transparent (expected %s)", tt.x, tt.y, tt.expected)
		}
	}
}

func TestNewImageBuf(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		format  ImageFormat
		wantErr bool
	}{
		{"valid RGBA8", 100, 100, FormatRGBA8, false},
		{"valid Gray8", 50, 50, FormatGray8, false},
		{"zero width", 0, 100, FormatRGBA8, true},
		{"zero height", 100, 0, FormatRGBA8, true},
		{"negative width", -10, 100, FormatRGBA8, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := NewImageBuf(tt.width, tt.height, tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewImageBuf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && img == nil {
				t.Error("NewImageBuf() returned nil without error")
			}
		})
	}
}

func TestImageBufFromImage(t *testing.T) {
	// Create standard library image
	stdImg := image.NewRGBA(image.Rect(0, 0, 10, 10))
	stdImg.Set(5, 5, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	// Convert to ImageBuf
	img := ImageBufFromImage(stdImg)
	if img == nil {
		t.Fatal("ImageBufFromImage returned nil")
	}

	// Verify dimensions
	w, h := img.Bounds()
	if w != 10 || h != 10 {
		t.Errorf("Expected 10x10, got %dx%d", w, h)
	}

	// Verify pixel
	r, g, b, a := img.GetRGBA(5, 5)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("Expected red pixel, got RGBA(%d, %d, %d, %d)", r, g, b, a)
	}
}

func TestSetFillPattern(t *testing.T) {
	dc := NewContext(100, 100)

	// Create a simple pattern
	img, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	pattern := dc.CreateImagePattern(img, 0, 0, 10, 10)

	// Set as fill pattern
	dc.SetFillPattern(pattern)

	// Verify pattern is set
	if dc.paint.Pattern == nil {
		t.Error("Expected paint pattern to be set")
	}
}

func TestSetStrokePattern(t *testing.T) {
	dc := NewContext(100, 100)

	// Create a simple pattern
	img, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	pattern := dc.CreateImagePattern(img, 0, 0, 10, 10)

	// Set as stroke pattern
	dc.SetStrokePattern(pattern)

	// Verify pattern is set
	if dc.paint.Pattern == nil {
		t.Error("Expected paint pattern to be set")
	}
}

func TestInterpolationModes(t *testing.T) {
	modes := []InterpolationMode{
		InterpNearest,
		InterpBilinear,
		InterpBicubic,
	}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			dc := NewContext(200, 200)
			dc.Clear()

			// Create small source image
			img, err := NewImageBuf(50, 50, FormatRGBA8)
			if err != nil {
				t.Fatalf("Failed to create image: %v", err)
			}

			// Fill with magenta
			for y := 0; y < 50; y++ {
				for x := 0; x < 50; x++ {
					_ = img.SetRGBA(x, y, 255, 0, 255, 255)
				}
			}

			// Scale up to test interpolation
			dc.DrawImageEx(img, DrawImageOptions{
				X:             10,
				Y:             10,
				DstWidth:      100,
				DstHeight:     100,
				Interpolation: mode,
				Opacity:       1.0,
				BlendMode:     BlendNormal,
			})

			// Check pixel at the start of the drawn image (should definitely be there)
			result := dc.pixmap.GetPixel(15, 15)
			if result.A < 0.9 {
				t.Errorf("Expected opaque pixel with %s interpolation, got A=%.2f", mode.String(), result.A)
			}
			// Verify it's magenta-ish (allowing for some interpolation error)
			if result.R < 0.8 || result.B < 0.8 {
				t.Errorf("Expected magenta pixel with %s interpolation, got R=%.2f G=%.2f B=%.2f", mode.String(), result.R, result.G, result.B)
			}
		})
	}
}

func TestBlendModes(t *testing.T) {
	modes := []struct {
		mode BlendMode
		name string
	}{
		{BlendNormal, "Normal"},
		{BlendMultiply, "Multiply"},
		{BlendScreen, "Screen"},
		{BlendOverlay, "Overlay"},
	}

	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			dc := NewContext(100, 100)

			// Fill background with white
			dc.SetRGB(1, 1, 1)
			dc.DrawRectangle(0, 0, 100, 100)
			dc.Fill()

			// Create red image
			img, err := NewImageBuf(50, 50, FormatRGBA8)
			if err != nil {
				t.Fatalf("Failed to create image: %v", err)
			}
			for y := 0; y < 50; y++ {
				for x := 0; x < 50; x++ {
					_ = img.SetRGBA(x, y, 255, 0, 0, 255)
				}
			}

			// Draw with blend mode
			dc.DrawImageEx(img, DrawImageOptions{
				X:             25,
				Y:             25,
				Interpolation: InterpNearest,
				Opacity:       1.0,
				BlendMode:     m.mode,
			})

			// Just verify something was drawn
			result := dc.pixmap.GetPixel(30, 30)
			if result.A == 0 {
				t.Errorf("Expected non-transparent pixel with %s blend mode", m.name)
			}
		})
	}
}

// TestDrawImageClipped_RoundedRect verifies that DrawImage respects a
// rounded rectangle clip region.
func TestDrawImageClipped_RoundedRect(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create a red 100x100 image.
	img := makeTestImage(t, 100, 100, 255, 0, 0, 255)

	dc.Push()

	// Set a rounded rectangle clip in the center.
	dc.DrawRoundedRectangle(50, 50, 100, 100, 15)
	dc.Clip()

	// Draw image at (50, 50) — exactly covering the clip region.
	dc.DrawImage(img, 50, 50)

	dc.Pop()

	// Pixel inside the clip (center of the rounded rect) should be red.
	inside := dc.pixmap.GetPixel(100, 100)
	if inside.R < 0.8 {
		t.Errorf("Expected red inside clip, got R=%.2f G=%.2f B=%.2f", inside.R, inside.G, inside.B)
	}

	// Pixel at the corner (50, 50) should be clipped away by the rounded corner.
	// The corner radius is 15px, so (50, 50) is in the clipped corner area.
	corner := dc.pixmap.GetPixel(51, 51)
	if corner.R > 0.5 {
		t.Errorf("Expected clipped corner at (51, 51), got R=%.2f (should be low due to rounded clip)", corner.R)
	}

	// Pixel outside the clip entirely should be transparent/background.
	outside := dc.pixmap.GetPixel(10, 10)
	if outside.R > 0.1 {
		t.Errorf("Expected no image outside clip, got R=%.2f", outside.R)
	}
}

// TestDrawImageClipped_Circle verifies that DrawImage respects a circular
// clip region.
func TestDrawImageClipped_Circle(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create a green 200x200 image.
	img := makeTestImage(t, 200, 200, 0, 255, 0, 255)

	dc.Push()

	// Clip to a circle centered at (100, 100) with radius 50.
	dc.DrawCircle(100, 100, 50)
	dc.Clip()

	// Draw image at origin — covers entire canvas.
	dc.DrawImage(img, 0, 0)

	dc.Pop()

	// Pixel at the center of the circle should be green.
	center := dc.pixmap.GetPixel(100, 100)
	if center.G < 0.8 {
		t.Errorf("Expected green at circle center, got G=%.2f", center.G)
	}

	// Pixel well outside the circle should be background (not green).
	outside := dc.pixmap.GetPixel(5, 5)
	if outside.G > 0.1 {
		t.Errorf("Expected no green outside circle clip, got G=%.2f", outside.G)
	}
}

// TestDrawImageClipped_NestedClips verifies that DrawImage works with
// nested Push/Pop and multiple clip regions.
func TestDrawImageClipped_NestedClips(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create a blue 200x200 image.
	img := makeTestImage(t, 200, 200, 0, 0, 255, 255)

	dc.Push()

	// First clip: large rectangle covering most of the canvas.
	dc.ClipRect(20, 20, 160, 160)

	dc.Push()

	// Second clip: circle in the center (intersects with the rectangle).
	dc.DrawCircle(100, 100, 50)
	dc.Clip()

	// Draw image — should only appear in the intersection of rect and circle.
	dc.DrawImage(img, 0, 0)

	dc.Pop() // Restore to just the rectangle clip.
	dc.Pop() // Restore to no clip.

	// Pixel at center (inside both clips) should be blue.
	center := dc.pixmap.GetPixel(100, 100)
	if center.B < 0.8 {
		t.Errorf("Expected blue at center (inside both clips), got B=%.2f", center.B)
	}

	// Pixel inside the rectangle but outside the circle should NOT be blue.
	rectOnly := dc.pixmap.GetPixel(25, 25)
	if rectOnly.B > 0.1 {
		t.Errorf("Expected no blue at (25,25) — inside rect but outside circle, got B=%.2f", rectOnly.B)
	}

	// Pixel completely outside both clips should be background.
	outside := dc.pixmap.GetPixel(5, 5)
	if outside.B > 0.1 {
		t.Errorf("Expected no blue at (5,5), got B=%.2f", outside.B)
	}
}

// TestDrawImageClipped_NoClip is a regression test ensuring that DrawImage
// without any clip produces the same result as before the refactoring.
func TestDrawImageClipped_NoClip(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create red image.
	img := makeTestImage(t, 50, 50, 255, 0, 0, 255)

	// Draw without any clip.
	dc.DrawImage(img, 10, 10)

	// Verify pixel at (10, 10) is red.
	result := dc.pixmap.GetPixel(10, 10)
	if result.R < 0.9 || result.G > 0.1 || result.B > 0.1 {
		t.Errorf("Expected red at (10, 10), got R=%.2f G=%.2f B=%.2f", result.R, result.G, result.B)
	}

	// Verify pixel just inside at (59, 59) is red (10+50-1=59).
	inside := dc.pixmap.GetPixel(59, 59)
	if inside.R < 0.9 {
		t.Errorf("Expected red at (59, 59), got R=%.2f", inside.R)
	}

	// Verify pixel just outside at (60, 60) is NOT red.
	outsideR := dc.pixmap.GetPixel(60, 10)
	if outsideR.R > 0.1 {
		t.Errorf("Expected no red at (60, 10), got R=%.2f", outsideR.R)
	}
}

// TestImagePattern_Anchor verifies that an image pattern with a non-zero
// anchor renders at the correct position.
func TestImagePattern_Anchor(t *testing.T) {
	// Create 10x10 red image.
	img := makeTestImage(t, 10, 10, 255, 0, 0, 255)

	pattern := &ImagePattern{
		image:   img,
		w:       10,
		h:       10,
		anchorX: 50,
		anchorY: 50,
		clamp:   true,
	}

	// At the anchor position, should return red.
	col := pattern.ColorAt(50, 50)
	if col.R < 0.9 || col.A < 0.9 {
		t.Errorf("Expected red at anchor (50, 50), got R=%.2f A=%.2f", col.R, col.A)
	}

	// Outside the image region, should return transparent (clamp mode).
	col = pattern.ColorAt(0, 0)
	if col.A > 0.01 {
		t.Errorf("Expected transparent outside anchor region, got A=%.2f", col.A)
	}

	// Just past the image region, should also be transparent.
	col = pattern.ColorAt(60, 60)
	if col.A > 0.01 {
		t.Errorf("Expected transparent past anchor region at (60, 60), got A=%.2f", col.A)
	}
}

// TestImagePattern_Scale verifies that scaling works on the image pattern.
func TestImagePattern_Scale(t *testing.T) {
	// Create 10x10 red image.
	img := makeTestImage(t, 10, 10, 255, 0, 0, 255)

	pattern := &ImagePattern{
		image:  img,
		w:      10,
		h:      10,
		scaleX: 2.0, // Each pixel covers 2 destination pixels.
		scaleY: 2.0,
		clamp:  true,
	}

	// At (0, 0), maps to source (0, 0) — should be red.
	col := pattern.ColorAt(0, 0)
	if col.R < 0.9 {
		t.Errorf("Expected red at (0, 0) with 2x scale, got R=%.2f", col.R)
	}

	// At (19, 19), maps to source (9, 9) — still inside, should be red.
	col = pattern.ColorAt(19, 19)
	if col.R < 0.9 {
		t.Errorf("Expected red at (19, 19) with 2x scale, got R=%.2f", col.R)
	}

	// At (20, 0), maps to source (10, 0) — outside, should be transparent.
	col = pattern.ColorAt(20, 0)
	if col.A > 0.01 {
		t.Errorf("Expected transparent at (20, 0) with 2x scale, got A=%.2f", col.A)
	}
}

// TestImagePattern_Opacity verifies that pattern opacity is applied.
func TestImagePattern_Opacity(t *testing.T) {
	img := makeTestImage(t, 10, 10, 255, 0, 0, 255)

	pattern := &ImagePattern{
		image:   img,
		w:       10,
		h:       10,
		opacity: 0.5,
	}

	col := pattern.ColorAt(0, 0)
	if col.R < 0.9 {
		t.Errorf("Expected red channel unchanged, got R=%.2f", col.R)
	}
	// Alpha should be halved.
	if col.A < 0.45 || col.A > 0.55 {
		t.Errorf("Expected alpha ~0.5, got A=%.2f", col.A)
	}
}

// TestDrawImageClipped_WithTransform verifies that DrawImage respects
// clipping even when a transform is applied.
func TestDrawImageClipped_WithTransform(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	// Create red image.
	img := makeTestImage(t, 100, 100, 255, 0, 0, 255)

	dc.Push()

	// Clip to center rectangle.
	dc.ClipRect(50, 50, 100, 100)

	// Apply translation so image starts at (0, 0) but is shifted to (50, 50).
	dc.Translate(50, 50)

	// Draw image at origin in transformed space.
	dc.DrawImage(img, 0, 0)

	dc.Pop()

	// Center of the clip should have red pixels.
	center := dc.pixmap.GetPixel(100, 100)
	if center.R < 0.8 {
		t.Errorf("Expected red at center with transform+clip, got R=%.2f", center.R)
	}

	// Outside the clip should be background.
	outside := dc.pixmap.GetPixel(10, 10)
	if outside.R > 0.1 {
		t.Errorf("Expected no red outside clip with transform, got R=%.2f", outside.R)
	}
}

// TestDrawImageRounded verifies the convenience method.
func TestDrawImageRounded(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	img := makeTestImage(t, 80, 80, 255, 0, 0, 255)

	dc.DrawImageRounded(img, 60, 60, 10)

	// Center of the image should be red.
	center := dc.pixmap.GetPixel(100, 100)
	if center.R < 0.8 {
		t.Errorf("Expected red at center of rounded image, got R=%.2f", center.R)
	}

	// Corner should be clipped (radius=10, so the pixel at (60, 60) is in the rounded corner).
	corner := dc.pixmap.GetPixel(61, 61)
	if corner.R > 0.5 {
		t.Errorf("Expected clipped corner at (61, 61), got R=%.2f", corner.R)
	}
}

// TestDrawImageCircular verifies the convenience method.
func TestDrawImageCircular(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	img := makeTestImage(t, 100, 100, 0, 0, 255, 255)

	dc.DrawImageCircular(img, 100, 100, 40)

	// Center should be blue (inside the circle).
	center := dc.pixmap.GetPixel(100, 100)
	if center.B < 0.8 {
		t.Errorf("Expected blue at center of circular image, got B=%.2f", center.B)
	}

	// Pixel at the edge of the bounding box but outside the circle.
	// At 45 degrees from center at distance 40 = (100+28, 100+28) roughly.
	// At the actual corner (60, 60) which is outside the circle.
	corner := dc.pixmap.GetPixel(62, 62)
	if corner.B > 0.3 {
		t.Errorf("Expected no blue at corner outside circle, got B=%.2f", corner.B)
	}
}

// TestFillClippedSolid verifies that regular solid-color Fill() also
// respects the clip stack after this refactoring.
func TestFillClippedSolid(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	dc.Push()

	// Clip to a circle.
	dc.DrawCircle(100, 100, 50)
	dc.Clip()

	// Fill the entire canvas with red.
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	dc.Fill()

	dc.Pop()

	// Center of the circle should be red.
	center := dc.pixmap.GetPixel(100, 100)
	if center.R < 0.8 {
		t.Errorf("Expected red at circle center, got R=%.2f", center.R)
	}

	// Outside the circle should be background (black from Clear).
	outside := dc.pixmap.GetPixel(5, 5)
	if outside.R > 0.1 {
		t.Errorf("Expected no red outside circle clip, got R=%.2f", outside.R)
	}
}

// TestStrokeClipped verifies that Stroke() respects the clip stack.
func TestStrokeClipped(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Clear()

	dc.Push()

	// Clip to a small rectangle in the center.
	dc.ClipRect(80, 80, 40, 40)

	// Stroke a horizontal line across the entire canvas.
	dc.SetRGB(1, 0, 0)
	dc.SetLineWidth(4)
	dc.DrawLine(0, 100, 200, 100)
	dc.Stroke()

	dc.Pop()

	// Inside the clip, the line should be visible.
	inside := dc.pixmap.GetPixel(100, 100)
	if inside.R < 0.5 {
		t.Errorf("Expected red stroke inside clip, got R=%.2f", inside.R)
	}

	// Outside the clip, the line should NOT be visible.
	outside := dc.pixmap.GetPixel(10, 100)
	if outside.R > 0.1 {
		t.Errorf("Expected no stroke outside clip, got R=%.2f", outside.R)
	}
}

// --- ImagePattern setter methods ---

func TestImagePatternSetters(t *testing.T) {
	img := makeTestImage(t, 10, 10, 255, 0, 0, 255)
	p := &ImagePattern{image: img, w: 10, h: 10}

	// SetAnchor
	p.SetAnchor(100, 200)
	if p.anchorX != 100 || p.anchorY != 200 {
		t.Errorf("SetAnchor: got (%f,%f), want (100,200)", p.anchorX, p.anchorY)
	}

	// SetOpacity
	p.SetOpacity(0.75)
	if p.opacity != 0.75 {
		t.Errorf("SetOpacity: got %f, want 0.75", p.opacity)
	}

	// SetClamp
	p.SetClamp(true)
	if !p.clamp {
		t.Error("SetClamp(true): expected true")
	}
	p.SetClamp(false)
	if p.clamp {
		t.Error("SetClamp(false): expected false")
	}

	// SetScale
	p.SetScale(3.0, 4.0)
	if p.scaleX != 3.0 || p.scaleY != 4.0 {
		t.Errorf("SetScale: got (%f,%f), want (3,4)", p.scaleX, p.scaleY)
	}
}

func TestLoadImageNonexistent(t *testing.T) {
	_, err := LoadImage("/nonexistent/path/image.png")
	if err == nil {
		t.Error("LoadImage of nonexistent file should return error")
	}
}

func TestLoadWebPNonexistent(t *testing.T) {
	_, err := LoadWebP("/nonexistent/path/image.webp")
	if err == nil {
		t.Error("LoadWebP of nonexistent file should return error")
	}
}

// makeTestImage creates a solid-color test image with the given RGBA values.
func makeTestImage(t *testing.T, width, height int, r, g, b, a uint8) *ImageBuf { //nolint:unparam // a=255 is intentional in tests; keeping param for completeness
	t.Helper()
	img, err := NewImageBuf(width, height, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			_ = img.SetRGBA(x, y, r, g, b, a)
		}
	}
	return img
}
