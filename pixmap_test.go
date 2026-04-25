package gg

import (
	"image"
	"testing"
)

// TestSetPixelPremul tests the SetPixelPremul method.
func TestSetPixelPremul(t *testing.T) {
	pm := NewPixmap(10, 10)
	pm.Clear(Transparent)

	// Set a known premultiplied pixel
	pm.SetPixelPremul(5, 5, 128, 64, 32, 255)

	// Verify raw data directly
	i := (5*10 + 5) * 4
	data := pm.Data()
	if data[i+0] != 128 || data[i+1] != 64 || data[i+2] != 32 || data[i+3] != 255 {
		t.Errorf("raw data mismatch: got (%d, %d, %d, %d), want (128, 64, 32, 255)",
			data[i+0], data[i+1], data[i+2], data[i+3])
	}

	// Verify via At() (returns premultiplied color.RGBA)
	c := pm.At(5, 5)
	r, g, b, a := c.RGBA()
	// color.RGBA returns values scaled to 0-65535
	if r != 128*257 || g != 64*257 || b != 32*257 || a != 255*257 {
		t.Errorf("At() mismatch: got (%d, %d, %d, %d), want (%d, %d, %d, %d)",
			r, g, b, a, 128*257, 64*257, 32*257, 255*257)
	}
}

// TestSetPixelPremul_OutOfBounds verifies out-of-bounds coordinates are silently ignored.
func TestSetPixelPremul_OutOfBounds(t *testing.T) {
	pm := NewPixmap(10, 10)
	pm.Clear(Black)

	// Save original data
	original := make([]uint8, len(pm.Data()))
	copy(original, pm.Data())

	// These should not panic and should not modify data
	oob := []struct{ x, y int }{
		{-1, 5}, {10, 5}, {5, -1}, {5, 10},
		{-100, -100}, {100, 100},
	}
	for _, c := range oob {
		pm.SetPixelPremul(c.x, c.y, 255, 0, 0, 255)
	}

	// Data should be unchanged
	for i, v := range pm.Data() {
		if v != original[i] {
			t.Fatalf("out-of-bounds write modified data at index %d: got %d, want %d", i, v, original[i])
		}
	}
}

// TestSetPixelPremul_ConsistentWithSetPixel verifies that SetPixelPremul produces
// the same result as SetPixel for an opaque color.
func TestSetPixelPremul_ConsistentWithSetPixel(t *testing.T) {
	pm1 := NewPixmap(10, 10)
	pm2 := NewPixmap(10, 10)

	// SetPixel with opaque red: R=1, G=0, B=0, A=1 → premul (255, 0, 0, 255)
	pm1.SetPixel(3, 7, Red)
	pm2.SetPixelPremul(3, 7, 255, 0, 0, 255)

	i := (7*10 + 3) * 4
	d1 := pm1.Data()
	d2 := pm2.Data()
	if d1[i] != d2[i] || d1[i+1] != d2[i+1] || d1[i+2] != d2[i+2] || d1[i+3] != d2[i+3] {
		t.Errorf("SetPixel(%d,%d,%d,%d) != SetPixelPremul(%d,%d,%d,%d)",
			d1[i], d1[i+1], d1[i+2], d1[i+3],
			d2[i], d2[i+1], d2[i+2], d2[i+3])
	}
}

// TestSetPixelPremul_SemiTransparent verifies premultiplied semi-transparent values.
func TestSetPixelPremul_SemiTransparent(t *testing.T) {
	pm := NewPixmap(10, 10)

	// 50% transparent red: premultiplied = (128, 0, 0, 128)
	pm.SetPixelPremul(0, 0, 128, 0, 0, 128)

	// GetPixel un-premultiplies: R=128/128=1.0, A=128/255≈0.502
	c := pm.GetPixel(0, 0)
	tolerance := 0.01
	if abs(c.R-1.0) > tolerance {
		t.Errorf("R: got %.4f, want 1.0", c.R)
	}
	if abs(c.G-0.0) > tolerance {
		t.Errorf("G: got %.4f, want 0.0", c.G)
	}
	if abs(c.B-0.0) > tolerance {
		t.Errorf("B: got %.4f, want 0.0", c.B)
	}
	if abs(c.A-128.0/255.0) > tolerance {
		t.Errorf("A: got %.4f, want %.4f", c.A, 128.0/255.0)
	}
}

// TestSetPixelPremul_ZeroAlpha verifies fully transparent pixel.
func TestSetPixelPremul_ZeroAlpha(t *testing.T) {
	pm := NewPixmap(10, 10)
	pm.Clear(White)

	pm.SetPixelPremul(0, 0, 0, 0, 0, 0)

	c := pm.GetPixel(0, 0)
	if c.A != 0 {
		t.Errorf("expected zero alpha, got %.4f", c.A)
	}
}

// TestPixmapFillSpan tests the FillSpan method with various span sizes.
func TestPixmapFillSpan(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		x1     int
		x2     int
		y      int
		color  RGBA
		pixels int // number of pixels that should be filled
	}{
		{
			name:   "short span (< 16 pixels)",
			width:  100,
			height: 100,
			x1:     10,
			x2:     20,
			y:      50,
			color:  Red,
			pixels: 10,
		},
		{
			name:   "medium span (16 pixels)",
			width:  100,
			height: 100,
			x1:     10,
			x2:     26,
			y:      50,
			color:  Green,
			pixels: 16,
		},
		{
			name:   "long span (100 pixels)",
			width:  200,
			height: 100,
			x1:     10,
			x2:     110,
			y:      50,
			color:  Blue,
			pixels: 100,
		},
		{
			name:   "full row",
			width:  100,
			height: 100,
			x1:     0,
			x2:     100,
			y:      50,
			color:  Yellow,
			pixels: 100,
		},
		{
			name:   "clipped left",
			width:  100,
			height: 100,
			x1:     -10,
			x2:     20,
			y:      50,
			color:  Cyan,
			pixels: 20,
		},
		{
			name:   "clipped right",
			width:  100,
			height: 100,
			x1:     90,
			x2:     120,
			y:      50,
			color:  Magenta,
			pixels: 10,
		},
		{
			name:   "out of bounds y (negative)",
			width:  100,
			height: 100,
			x1:     10,
			x2:     20,
			y:      -1,
			color:  Red,
			pixels: 0,
		},
		{
			name:   "out of bounds y (too large)",
			width:  100,
			height: 100,
			x1:     10,
			x2:     20,
			y:      100,
			color:  Red,
			pixels: 0,
		},
		{
			name:   "x1 >= x2",
			width:  100,
			height: 100,
			x1:     20,
			x2:     10,
			y:      50,
			color:  Red,
			pixels: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewPixmap(tt.width, tt.height)
			pm.Clear(Black)

			// Fill the span
			pm.FillSpan(tt.x1, tt.x2, tt.y, tt.color)

			// Count filled pixels
			filled := 0
			for x := 0; x < tt.width; x++ {
				c := pm.GetPixel(x, tt.y)
				if c.R == tt.color.R && c.G == tt.color.G && c.B == tt.color.B {
					filled++
				}
			}

			if filled != tt.pixels {
				t.Errorf("expected %d filled pixels, got %d", tt.pixels, filled)
			}

			// Verify pixels are in correct positions
			if tt.pixels > 0 && tt.y >= 0 && tt.y < tt.height {
				startX := tt.x1
				if startX < 0 {
					startX = 0
				}
				endX := tt.x2
				if endX > tt.width {
					endX = tt.width
				}

				for x := startX; x < endX; x++ {
					c := pm.GetPixel(x, tt.y)
					if c.R != tt.color.R || c.G != tt.color.G || c.B != tt.color.B {
						t.Errorf("pixel at (%d, %d) should be filled color, got R=%.2f G=%.2f B=%.2f",
							x, tt.y, c.R, c.G, c.B)
					}
				}
			}
		})
	}
}

// TestPixmapFillSpanPerformance tests that FillSpan is faster than multiple SetPixel calls.
func TestPixmapFillSpanPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	pm := NewPixmap(1000, 1000)
	color := Red
	y := 500

	// This is a basic smoke test - in a real benchmark we'd use testing.B
	// Just verify that FillSpan works for various sizes
	sizes := []int{10, 50, 100, 200, 500}
	for _, size := range sizes {
		pm.Clear(Black)
		pm.FillSpan(0, size, y, color)

		// Verify all pixels were filled
		for x := 0; x < size; x++ {
			c := pm.GetPixel(x, y)
			if c.R != color.R {
				t.Errorf("FillSpan failed to fill pixel at x=%d", x)
			}
		}
	}
}

// TestPixmapFillSpanBlend tests the FillSpanBlend method.
func TestPixmapFillSpanBlend(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		x1         int
		x2         int
		y          int
		background RGBA
		foreground RGBA
		expectFG   bool // true if we expect pure foreground (alpha=1.0)
	}{
		{
			name:       "opaque color (no blending)",
			width:      100,
			height:     100,
			x1:         10,
			x2:         20,
			y:          50,
			background: White,
			foreground: Red,
			expectFG:   true,
		},
		{
			name:       "semi-transparent color (short span)",
			width:      100,
			height:     100,
			x1:         10,
			x2:         20,
			y:          50,
			background: White,
			foreground: RGBA2(1, 0, 0, 0.5),
			expectFG:   false,
		},
		{
			name:       "semi-transparent color (long span)",
			width:      100,
			height:     100,
			x1:         10,
			x2:         60,
			y:          50,
			background: White,
			foreground: RGBA2(0, 1, 0, 0.5),
			expectFG:   false,
		},
		{
			name:       "fully transparent (no effect)",
			width:      100,
			height:     100,
			x1:         10,
			x2:         20,
			y:          50,
			background: Red,
			foreground: RGBA2(0, 0, 1, 0.0),
			expectFG:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewPixmap(tt.width, tt.height)
			pm.Clear(tt.background)

			// Fill the span with blending
			pm.FillSpanBlend(tt.x1, tt.x2, tt.y, tt.foreground)

			// Check a pixel in the middle of the span
			midX := (tt.x1 + tt.x2) / 2
			c := pm.GetPixel(midX, tt.y)

			tolerance := 0.01
			switch {
			case tt.expectFG:
				// Should be the foreground color (opaque)
				if abs(c.R-tt.foreground.R) > tolerance ||
					abs(c.G-tt.foreground.G) > tolerance ||
					abs(c.B-tt.foreground.B) > tolerance {
					t.Errorf("expected foreground color (%.2f, %.2f, %.2f), got (%.2f, %.2f, %.2f)",
						tt.foreground.R, tt.foreground.G, tt.foreground.B, c.R, c.G, c.B)
				}
			case tt.foreground.A == 0.0:
				// Should be unchanged (background)
				if abs(c.R-tt.background.R) > tolerance ||
					abs(c.G-tt.background.G) > tolerance ||
					abs(c.B-tt.background.B) > tolerance {
					t.Errorf("expected background color (%.2f, %.2f, %.2f), got (%.2f, %.2f, %.2f)",
						tt.background.R, tt.background.G, tt.background.B, c.R, c.G, c.B)
				}
			default:
				// Should be blended (neither pure foreground nor pure background)
				isForeground := abs(c.R-tt.foreground.R) < tolerance &&
					abs(c.G-tt.foreground.G) < tolerance &&
					abs(c.B-tt.foreground.B) < tolerance
				isBackground := abs(c.R-tt.background.R) < tolerance &&
					abs(c.G-tt.background.G) < tolerance &&
					abs(c.B-tt.background.B) < tolerance

				if isForeground || isBackground {
					t.Errorf("expected blended color, got (%.2f, %.2f, %.2f) which matches %s",
						c.R, c.G, c.B, func() string {
							if isForeground {
								return "foreground"
							}
							return "background"
						}())
				}
			}
		})
	}
}

// TestPixmapFillSpanBounds tests boundary conditions for FillSpan.
func TestPixmapFillSpanBounds(t *testing.T) {
	pm := NewPixmap(100, 100)
	pm.Clear(Black)

	// Test various boundary conditions
	testCases := []struct {
		name string
		x1   int
		x2   int
		y    int
	}{
		{"negative x1", -10, 10, 50},
		{"x2 beyond width", 90, 150, 50},
		{"both out of bounds", -10, 150, 50},
		{"negative y", 10, 20, -1},
		{"y beyond height", 10, 20, 100},
		{"x1 == x2", 10, 10, 50},
		{"x1 > x2", 20, 10, 50},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			pm.FillSpan(tc.x1, tc.x2, tc.y, Red)
		})
	}
}

func TestPixmapFillRect(t *testing.T) {
	pm := NewPixmap(10, 10)

	pm.FillRect(image.Rect(2, 3, 5, 6), 255, 0, 0, 255)

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			i := (y*10 + x) * 4
			inside := x >= 2 && x < 5 && y >= 3 && y < 6
			if inside {
				if pm.data[i] != 255 || pm.data[i+1] != 0 || pm.data[i+2] != 0 || pm.data[i+3] != 255 {
					t.Errorf("pixel (%d,%d) inside rect: got [%d,%d,%d,%d], want [255,0,0,255]",
						x, y, pm.data[i], pm.data[i+1], pm.data[i+2], pm.data[i+3])
				}
			} else {
				if pm.data[i] != 0 || pm.data[i+3] != 0 {
					t.Errorf("pixel (%d,%d) outside rect: got [%d,%d,%d,%d], want [0,0,0,0]",
						x, y, pm.data[i], pm.data[i+1], pm.data[i+2], pm.data[i+3])
				}
			}
		}
	}
}

func TestPixmapFillRect_Clamp(t *testing.T) {
	pm := NewPixmap(5, 5)

	pm.FillRect(image.Rect(-2, -2, 8, 8), 0, 255, 0, 255)

	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			i := (y*5 + x) * 4
			if pm.data[i+1] != 255 || pm.data[i+3] != 255 {
				t.Errorf("pixel (%d,%d): got [%d,%d,%d,%d], want [0,255,0,255]",
					x, y, pm.data[i], pm.data[i+1], pm.data[i+2], pm.data[i+3])
			}
		}
	}
}

func TestPixmapFillRect_OutOfBounds(t *testing.T) {
	pm := NewPixmap(5, 5)
	genBefore := pm.genID

	pm.FillRect(image.Rect(10, 10, 20, 20), 255, 0, 0, 255)

	if pm.genID != genBefore {
		t.Error("out-of-bounds FillRect should not change genID")
	}
}

func TestPixmapFillRect_GenID(t *testing.T) {
	pm := NewPixmap(5, 5)
	genBefore := pm.genID

	pm.FillRect(image.Rect(0, 0, 3, 3), 255, 0, 0, 255)

	if pm.genID == genBefore {
		t.Error("FillRect should increment genID")
	}
}

func TestFillRectCPU(t *testing.T) {
	dc := NewContext(20, 20)
	defer dc.Close()

	dc.FillRectCPU(5, 5, 10, 10, RGBA{1, 0, 0, 1})

	px := dc.ResizeTarget().GetPixel(10, 10)
	if px.R < 0.9 || px.A < 0.9 {
		t.Errorf("pixel inside FillRectCPU: got R=%f A=%f, want ~1.0", px.R, px.A)
	}

	px = dc.ResizeTarget().GetPixel(0, 0)
	if px.A > 0.01 {
		t.Errorf("pixel outside FillRectCPU: got A=%f, want ~0", px.A)
	}
}

func TestFillRectCPU_DoesNotQueueSDF(t *testing.T) {
	dc := NewContext(20, 20)
	defer dc.Close()

	dc.FillRectCPU(0, 0, 20, 20, RGBA{0.5, 0.5, 0.5, 1})

	rc := dc.GPURenderContext()
	if rc != nil {
		type pendingCounter interface{ PendingCount() int }
		if pc, ok := rc.(pendingCounter); ok {
			if pc.PendingCount() != 0 {
				t.Errorf("FillRectCPU should not queue GPU shapes, got %d pending", pc.PendingCount())
			}
		}
	}
}

func TestPixmap_FillRect(t *testing.T) {
	pm := NewPixmap(10, 10)

	pm.FillRect(image.Rect(2, 3, 5, 6), 255, 0, 0, 255)

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			i := (y*10 + x) * 4
			inside := x >= 2 && x < 5 && y >= 3 && y < 6
			if inside {
				if pm.data[i] != 255 || pm.data[i+1] != 0 || pm.data[i+2] != 0 || pm.data[i+3] != 255 {
					t.Errorf("pixel(%d,%d) = %v, want [255 0 0 255]", x, y, pm.data[i:i+4])
				}
			} else {
				if pm.data[i] != 0 || pm.data[i+1] != 0 || pm.data[i+2] != 0 || pm.data[i+3] != 0 {
					t.Errorf("pixel(%d,%d) = %v, want [0 0 0 0]", x, y, pm.data[i:i+4])
				}
			}
		}
	}
}

func TestPixmap_FillRect_Clamped(t *testing.T) {
	pm := NewPixmap(5, 5)

	pm.FillRect(image.Rect(-2, -2, 8, 8), 0, 255, 0, 255)

	i := 0
	if pm.data[i] != 0 || pm.data[i+1] != 255 || pm.data[i+2] != 0 || pm.data[i+3] != 255 {
		t.Errorf("pixel(0,0) = %v, want [0 255 0 255]", pm.data[i:i+4])
	}
	last := (4*5 + 4) * 4
	if pm.data[last] != 0 || pm.data[last+1] != 255 {
		t.Errorf("pixel(4,4) = %v, want [0 255 0 255]", pm.data[last:last+4])
	}
}

func TestPixmap_FillRect_Empty(t *testing.T) {
	pm := NewPixmap(5, 5)
	origGen := pm.genID

	pm.FillRect(image.Rect(10, 10, 20, 20), 255, 0, 0, 255)

	if pm.genID != origGen {
		t.Error("genID should not change for out-of-bounds rect")
	}
}

func TestPixmap_FillRect_GenID(t *testing.T) {
	pm := NewPixmap(5, 5)
	gen1 := pm.genID

	pm.FillRect(image.Rect(0, 0, 3, 3), 255, 0, 0, 255)

	if pm.genID == gen1 {
		t.Error("genID should change after FillRect")
	}
}
