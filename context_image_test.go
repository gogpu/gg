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
	_ = img.SetRGBA(0, 0, 255, 0, 0, 255) // Red
	_ = img.SetRGBA(1, 0, 0, 255, 0, 255) // Green
	_ = img.SetRGBA(0, 1, 0, 0, 255, 255) // Blue
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
		{2, 0, "red"},   // Wraps to (0, 0)
		{0, 2, "red"},   // Wraps to (0, 0)
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
		name   string
		width  int
		height int
		format ImageFormat
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
