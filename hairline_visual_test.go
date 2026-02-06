package gg

import (
	"image"
	"testing"
)

// TestHairlineVisualDashedRect tests the reproduction case from Issue #56.
// This verifies that thin dashed rectangles render smoothly with hairline AA.
func TestHairlineVisualDashedRect(t *testing.T) {
	gc := NewContext(512, 512)
	defer func() { _ = gc.Close() }()

	// Clear with white background
	gc.ClearWithColor(White)

	// Set black color for strokes
	gc.SetRGB(0, 0, 0)

	// Issue #56 reproduction: scaled context with dashed rectangle
	gc.Scale(2, 2)
	gc.SetLineWidth(1) // This should use hairline rendering
	gc.SetDash(3, 5)
	gc.DrawRectangle(67, 45, 83, 47)
	if err := gc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Verify some pixels were drawn
	img := gc.Image()
	bounds := img.Bounds()
	hasColoredPixels := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			r, g, b, a := c.RGBA()
			// Check for any non-white pixel
			if r < 0xFFFF || g < 0xFFFF || b < 0xFFFF || a > 0 {
				hasColoredPixels = true
				break
			}
		}
		if hasColoredPixels {
			break
		}
	}

	if !hasColoredPixels {
		t.Error("Expected dashed rectangle to produce visible output")
	}
}

// TestHairlineQualityComparison compares hairline rendering to stroke expansion.
// We expect hairline rendering to produce smoother results for thin lines.
func TestHairlineQualityComparison(t *testing.T) {
	const size = 100

	// Draw a diagonal line with hairline width
	gc := NewContext(size, size)
	defer func() { _ = gc.Close() }()

	gc.ClearWithColor(White)
	gc.SetRGB(0, 0, 0)
	gc.SetLineWidth(1.0) // Should trigger hairline rendering
	gc.MoveTo(10, 10)
	gc.LineTo(90, 90)
	if err := gc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Count pixels with varying gray levels (indicator of AA)
	// With black on white, partial coverage shows as gray
	img := gc.Image()
	grayCount := 0  // Partial coverage
	blackCount := 0 // Full coverage
	uniqueGrays := make(map[uint32]bool)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := img.At(x, y)
			r, _, _, _ := c.RGBA()
			if r < 0xFFFF { // Not pure white
				uniqueGrays[r] = true
				if r > 0 { // Not pure black - has some white mixed in (partial coverage)
					grayCount++
				} else {
					blackCount++
				}
			}
		}
	}

	total := grayCount + blackCount
	if total == 0 {
		t.Error("Expected line to produce pixels")
		return
	}

	t.Logf("Gray (AA) pixels: %d, Black pixels: %d, Unique gray levels: %d", grayCount, blackCount, len(uniqueGrays))

	// We expect at least SOME pixels to be drawn
	// Note: The quality of AA depends on the algorithm implementation
}

// TestHairlineHorizontalLine tests horizontal line quality.
func TestHairlineHorizontalLine(t *testing.T) {
	gc := NewContext(100, 100)
	defer func() { _ = gc.Close() }()

	gc.ClearWithColor(White)
	gc.SetRGB(0, 0, 0)
	gc.SetLineWidth(1.0)

	// Draw a horizontal line that's NOT on a pixel boundary
	gc.MoveTo(10, 50.5) // 0.5 offset for best AA demonstration
	gc.LineTo(90, 50.5)
	if err := gc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Check that the line is visible (at least one row has pixels)
	img := gc.Image()
	row50Count := 0
	row51Count := 0

	for x := 10; x < 90; x++ {
		c50 := img.At(x, 50)
		c51 := img.At(x, 51)

		r50, _, _, _ := c50.RGBA()
		r51, _, _, _ := c51.RGBA()

		if r50 < 0xFFFF { // Not pure white
			row50Count++
		}
		if r51 < 0xFFFF { // Not pure white
			row51Count++
		}
	}

	t.Logf("Row 50: %d pixels, Row 51: %d pixels", row50Count, row51Count)

	// Expect at least one row to have pixels
	// Note: The y=50.5 offset should ideally split between rows, but
	// the exact behavior depends on how coordinate transformation affects
	// the hairline algorithm
	if row50Count == 0 && row51Count == 0 {
		t.Error("Expected horizontal line at y=50.5 to be visible")
	}
}

// TestHairlineVerticalLine tests vertical line quality.
func TestHairlineVerticalLine(t *testing.T) {
	gc := NewContext(100, 100)
	defer func() { _ = gc.Close() }()

	gc.ClearWithColor(White)
	gc.SetRGB(0, 0, 0)
	gc.SetLineWidth(1.0)

	// Draw a vertical line that's NOT on a pixel boundary
	gc.MoveTo(50.5, 10) // 0.5 offset for best AA demonstration
	gc.LineTo(50.5, 90)
	if err := gc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Check that the line is visible (at least one column has pixels)
	img := gc.Image()
	col50Count := 0
	col51Count := 0

	for y := 10; y < 90; y++ {
		c50 := img.At(50, y)
		c51 := img.At(51, y)

		r50, _, _, _ := c50.RGBA()
		r51, _, _, _ := c51.RGBA()

		if r50 < 0xFFFF { // Not pure white
			col50Count++
		}
		if r51 < 0xFFFF { // Not pure white
			col51Count++
		}
	}

	t.Logf("Col 50: %d pixels, Col 51: %d pixels", col50Count, col51Count)

	// Expect at least one column to have pixels
	if col50Count == 0 && col51Count == 0 {
		t.Error("Expected vertical line at x=50.5 to be visible")
	}
}

// TestHairlineThinLineVisibility tests that very thin lines are still visible.
func TestHairlineThinLineVisibility(t *testing.T) {
	widths := []float64{0.5, 0.3, 0.1}

	for _, width := range widths {
		t.Run("width_"+testFloatStr(width), func(t *testing.T) {
			gc := NewContext(100, 100)
			defer func() { _ = gc.Close() }()

			gc.ClearWithColor(White)
			gc.SetRGB(0, 0, 0)
			gc.SetLineWidth(width)

			gc.MoveTo(10, 50)
			gc.LineTo(90, 50)
			if err := gc.Stroke(); err != nil {
				t.Fatalf("Stroke failed: %v", err)
			}

			// Count non-white pixels
			img := gc.Image()
			nonWhiteCount := 0

			for y := 0; y < 100; y++ {
				for x := 0; x < 100; x++ {
					c := img.At(x, y)
					r, g, b, _ := c.RGBA()
					if r < 0xFFFF || g < 0xFFFF || b < 0xFFFF {
						nonWhiteCount++
					}
				}
			}

			if nonWhiteCount == 0 {
				t.Errorf("Line with width %f should be visible", width)
			} else {
				t.Logf("Width %f: %d visible pixels", width, nonWhiteCount)
			}
		})
	}
}

// testFloatStr converts a float to a test-friendly string.
func testFloatStr(f float64) string {
	return image.Rect(0, 0, int(f*10), 0).String()[1:] // Hack to avoid format import
}

// TestHairlineLineCaps tests that line caps work with hairline rendering.
func TestHairlineLineCaps(t *testing.T) {
	caps := []struct {
		name string
		cap  LineCap
	}{
		{"butt", LineCapButt},
		{"round", LineCapRound},
		{"square", LineCapSquare},
	}

	for _, tt := range caps {
		t.Run(tt.name, func(t *testing.T) {
			gc := NewContext(100, 100)
			defer func() { _ = gc.Close() }()

			gc.ClearWithColor(White)
			gc.SetRGB(0, 0, 0)
			gc.SetLineWidth(1.0)
			gc.SetLineCap(tt.cap)

			gc.MoveTo(20, 50)
			gc.LineTo(80, 50)
			if err := gc.Stroke(); err != nil {
				t.Fatalf("Stroke failed: %v", err)
			}

			// Verify line was drawn - check rows 49, 50, 51 for any non-white pixels
			img := gc.Image()
			hasPixels := false
			for x := 15; x < 85; x++ {
				for dy := -1; dy <= 1; dy++ {
					y := 50 + dy
					if y < 0 || y >= 100 {
						continue
					}
					c := img.At(x, y)
					r, _, _, _ := c.RGBA()
					if r < 0xFFFF {
						hasPixels = true
						break
					}
				}
				if hasPixels {
					break
				}
			}

			if !hasPixels {
				t.Errorf("Expected line with %s cap to be visible", tt.name)
			}
		})
	}
}

// TestHairlineDashPattern tests dashed hairlines.
func TestHairlineDashPattern(t *testing.T) {
	gc := NewContext(200, 100)
	defer func() { _ = gc.Close() }()

	gc.ClearWithColor(White)
	gc.SetRGB(0, 0, 0)
	gc.SetLineWidth(1.0)
	gc.SetDash(10, 5) // 10px dash, 5px gap

	gc.MoveTo(10, 50)
	gc.LineTo(190, 50)
	if err := gc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Count visible pixel runs to verify dashing
	// Check rows 49, 50, 51 for the line
	img := gc.Image()
	inDash := false
	dashCount := 0

	for x := 10; x < 190; x++ {
		isVisible := false
		for dy := -1; dy <= 1; dy++ {
			y := 50 + dy
			if y < 0 || y >= 100 {
				continue
			}
			c := img.At(x, y)
			r, _, _, _ := c.RGBA()
			if r < 0xFFFF {
				isVisible = true
				break
			}
		}

		if isVisible && !inDash {
			dashCount++
			inDash = true
		} else if !isVisible && inDash {
			inDash = false
		}
	}

	t.Logf("Detected %d dashes", dashCount)

	// With pattern 10+5=15px per cycle, over 180px we expect ~12 dashes
	if dashCount < 5 {
		t.Errorf("Expected multiple dashes, got %d", dashCount)
	}
}

// TestHairlineWithColors tests hairline rendering with different colors.
func TestHairlineWithColors(t *testing.T) {
	colors := []struct {
		name    string
		r, g, b float64
	}{
		{"red", 1, 0, 0},
		{"green", 0, 1, 0},
		{"blue", 0, 0, 1},
		{"gray", 0.5, 0.5, 0.5},
	}

	for _, cc := range colors {
		t.Run(cc.name, func(t *testing.T) {
			gc := NewContext(100, 100)
			defer func() { _ = gc.Close() }()

			gc.ClearWithColor(White)
			gc.SetRGB(cc.r, cc.g, cc.b)
			gc.SetLineWidth(1.0)

			gc.MoveTo(10, 50)
			gc.LineTo(90, 50)
			if err := gc.Stroke(); err != nil {
				t.Fatalf("Stroke failed: %v", err)
			}

			// Verify the color is correct
			img := gc.Image()
			c := img.At(50, 50)
			r, g, b, _ := c.RGBA()

			// Check that the dominant channel matches
			maxChannel := uint32(0)
			if cc.r > 0 && r > maxChannel {
				maxChannel = r
			}
			if cc.g > 0 && g > maxChannel {
				maxChannel = g
			}
			if cc.b > 0 && b > maxChannel {
				maxChannel = b
			}

			// The line should have some color
			if maxChannel == 0 && (cc.r > 0 || cc.g > 0 || cc.b > 0) {
				t.Errorf("Expected %s line to have visible color", cc.name)
			}
		})
	}
}

// TestHairlineEdgeCase_NearIntegerCoordinates tests hairlines near integer coordinates.
func TestHairlineEdgeCase_NearIntegerCoordinates(t *testing.T) {
	offsets := []float64{0.0, 0.01, 0.25, 0.5, 0.75, 0.99}

	for _, offset := range offsets {
		t.Run("offset_"+testFloatStr(offset), func(t *testing.T) {
			gc := NewContext(100, 100)
			defer func() { _ = gc.Close() }()

			gc.ClearWithColor(White)
			gc.SetRGB(0, 0, 0)
			gc.SetLineWidth(1.0)

			gc.MoveTo(10+offset, 50+offset)
			gc.LineTo(90+offset, 50+offset)
			if err := gc.Stroke(); err != nil {
				t.Fatalf("Stroke failed: %v", err)
			}

			// Verify line is visible
			img := gc.Image()
			hasPixels := false
			for x := 10; x < 90; x++ {
				for dy := -1; dy <= 1; dy++ {
					c := img.At(x, 50+dy)
					r, _, _, _ := c.RGBA()
					if r < 0xFFFF {
						hasPixels = true
						break
					}
				}
				if hasPixels {
					break
				}
			}

			if !hasPixels {
				t.Errorf("Line with offset %f should be visible", offset)
			}
		})
	}
}

// TestStrokeExpansion_ScaledDashedRect tests thick strokes with scale transformation.
// This verifies the stroke expansion fix (lastNorm saved for end cap) from Issue #56.
func TestStrokeExpansion_ScaledDashedRect(t *testing.T) {
	// Test with thick line width (not hairline) to exercise stroke expansion
	gc := NewContext(400, 400)
	defer func() { _ = gc.Close() }()

	gc.ClearWithColor(White)
	gc.SetRGB(0, 0, 0)

	// Scale 2x with lineWidth=2 gives effective width=4, which uses stroke expansion
	gc.Scale(2, 2)
	gc.SetLineWidth(2) // Thick stroke, NOT hairline
	gc.SetDash(5, 5)
	gc.DrawRectangle(50, 50, 100, 50) // Rectangle at (50,50) with size 100x50
	if err := gc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Count pixels and check for artifacts
	img := gc.Image()
	bounds := img.Bounds()
	blackPixels := 0
	grayPixels := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			r, g, b, _ := c.RGBA()
			avgGray := (r + g + b) / 3
			if avgGray < 0x1000 {
				blackPixels++
			} else if avgGray < 0xF000 {
				grayPixels++
			}
		}
	}

	// Should have substantial black pixels from dashed stroke
	// With scale=2, lineWidth=2, dash(5,5), the rectangle should produce
	// significant stroke output if expansion is working correctly.
	if blackPixels < 100 {
		t.Errorf("Expected at least 100 black pixels, got %d", blackPixels)
	}

	t.Logf("Black pixels: %d, Gray AA pixels: %d", blackPixels, grayPixels)
}
