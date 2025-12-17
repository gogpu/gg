package gg

import (
	"image/color"
	"testing"

	"github.com/gogpu/gg/internal/parallel"
)

// =============================================================================
// Visual Regression Tests for Parallel Rendering
// =============================================================================
//
// These tests compare parallel rendering output with expected results to ensure
// correctness of the parallel rendering implementation.
//
// =============================================================================

// TestVisualRegression_ParallelClear tests that parallel Clear produces
// pixel-perfect results matching a reference clear operation.
func TestVisualRegression_ParallelClear(t *testing.T) {
	const (
		width  = 256
		height = 256
	)

	testColors := []struct {
		name  string
		color color.Color
		want  [4]byte
	}{
		{"white", color.White, [4]byte{255, 255, 255, 255}},
		{"black", color.Black, [4]byte{0, 0, 0, 255}},
		{"red", color.RGBA{R: 255, G: 0, B: 0, A: 255}, [4]byte{255, 0, 0, 255}},
		{"green", color.RGBA{R: 0, G: 255, B: 0, A: 255}, [4]byte{0, 255, 0, 255}},
		{"blue", color.RGBA{R: 0, G: 0, B: 255, A: 255}, [4]byte{0, 0, 255, 255}},
		{"transparent", color.Transparent, [4]byte{0, 0, 0, 0}},
		{"semi_transparent", color.RGBA{R: 128, G: 64, B: 32, A: 128}, [4]byte{128, 64, 32, 128}},
	}

	for _, tc := range testColors {
		t.Run(tc.name, func(t *testing.T) {
			pr := parallel.NewParallelRasterizer(width, height)
			if pr == nil {
				t.Fatal("Failed to create parallel rasterizer")
			}
			defer pr.Close()

			// Perform parallel clear
			pr.Clear(tc.color)

			// Composite to buffer
			stride := width * 4
			dst := make([]byte, height*stride)
			pr.Composite(dst, stride)

			// Verify every pixel matches expected color (pixel-perfect, tolerance 0)
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					offset := y*stride + x*4
					got := [4]byte{dst[offset], dst[offset+1], dst[offset+2], dst[offset+3]}

					if got != tc.want {
						t.Errorf("Pixel (%d,%d) = %v, want %v", x, y, got, tc.want)
						return // Stop on first mismatch
					}
				}
			}
		})
	}
}

// TestVisualRegression_ParallelFillRect tests that parallel FillRect produces
// pixel-perfect results.
func TestVisualRegression_ParallelFillRect(t *testing.T) {
	const (
		width  = 256
		height = 256
	)

	testCases := []struct {
		name                   string
		bgColor                color.Color
		rectX, rectY, rectW, h int
		rectColor              color.Color
	}{
		{
			name:      "centered_red_rect",
			bgColor:   color.Black,
			rectX:     64,
			rectY:     64,
			rectW:     128,
			h:         128,
			rectColor: color.RGBA{R: 255, G: 0, B: 0, A: 255},
		},
		{
			name:      "corner_green_rect",
			bgColor:   color.White,
			rectX:     0,
			rectY:     0,
			rectW:     64,
			h:         64,
			rectColor: color.RGBA{R: 0, G: 255, B: 0, A: 255},
		},
		{
			name:      "spanning_multiple_tiles",
			bgColor:   color.Black,
			rectX:     32,
			rectY:     32,
			rectW:     192,
			h:         192,
			rectColor: color.RGBA{R: 0, G: 0, B: 255, A: 255},
		},
		{
			name:      "edge_tile_rect",
			bgColor:   color.White,
			rectX:     200,
			rectY:     200,
			rectW:     56, // Goes to edge
			h:         56,
			rectColor: color.RGBA{R: 255, G: 255, B: 0, A: 255},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pr := parallel.NewParallelRasterizer(width, height)
			if pr == nil {
				t.Fatal("Failed to create parallel rasterizer")
			}
			defer pr.Close()

			// Clear with background color
			pr.Clear(tc.bgColor)

			// Fill rectangle
			pr.FillRect(tc.rectX, tc.rectY, tc.rectW, tc.h, tc.rectColor)

			// Composite to buffer
			stride := width * 4
			dst := make([]byte, height*stride)
			pr.Composite(dst, stride)

			// Get expected colors
			bgR, bgG, bgB, bgA := tc.bgColor.RGBA()
			bgExpected := [4]byte{byte(bgR >> 8), byte(bgG >> 8), byte(bgB >> 8), byte(bgA >> 8)}

			rectR, rectG, rectB, rectA := tc.rectColor.RGBA()
			rectExpected := [4]byte{byte(rectR >> 8), byte(rectG >> 8), byte(rectB >> 8), byte(rectA >> 8)}

			// Verify pixels
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					offset := y*stride + x*4
					got := [4]byte{dst[offset], dst[offset+1], dst[offset+2], dst[offset+3]}

					insideRect := x >= tc.rectX && x < tc.rectX+tc.rectW &&
						y >= tc.rectY && y < tc.rectY+tc.h

					var want [4]byte
					if insideRect {
						want = rectExpected
					} else {
						want = bgExpected
					}

					if got != want {
						t.Errorf("Pixel (%d,%d) inside=%v: got %v, want %v",
							x, y, insideRect, got, want)
						return
					}
				}
			}
		})
	}
}

// TestVisualRegression_ParallelComposite tests that Composite correctly
// assembles tiles into a contiguous buffer.
func TestVisualRegression_ParallelComposite(t *testing.T) {
	const (
		width  = 256
		height = 256
	)

	pr := parallel.NewParallelRasterizer(width, height)
	if pr == nil {
		t.Fatal("Failed to create parallel rasterizer")
	}
	defer pr.Close()

	// Create a gradient pattern: color depends on tile position
	tiles := pr.Grid().AllTiles()
	for _, tile := range tiles {
		tileX, tileY, tileW, tileH := tile.Bounds()

		for py := 0; py < tileH; py++ {
			for px := 0; px < tileW; px++ {
				canvasX := tileX + px
				canvasY := tileY + py

				offset := (py*tileW + px) * 4
				tile.Data[offset] = byte(canvasX)   // R = x
				tile.Data[offset+1] = byte(canvasY) // G = y
				tile.Data[offset+2] = 128           // B = constant
				tile.Data[offset+3] = 255           // A = opaque
			}
		}
	}

	// Composite to buffer
	stride := width * 4
	dst := make([]byte, height*stride)
	pr.Composite(dst, stride)

	// Verify the gradient pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := y*stride + x*4

			gotR := dst[offset]
			gotG := dst[offset+1]
			gotB := dst[offset+2]
			gotA := dst[offset+3]

			wantR := byte(x)
			wantG := byte(y)
			wantB := byte(128)
			wantA := byte(255)

			if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
				t.Errorf("Pixel (%d,%d) = [%d,%d,%d,%d], want [%d,%d,%d,%d]",
					x, y, gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
				return
			}
		}
	}
}

// TestVisualRegression_ParallelVsSerial compares parallel rendering output
// with a reference serial implementation.
func TestVisualRegression_ParallelVsSerial(t *testing.T) {
	const (
		width  = 256
		height = 256
	)

	// Test case: Clear + FillRect
	pr := parallel.NewParallelRasterizer(width, height)
	if pr == nil {
		t.Fatal("Failed to create parallel rasterizer")
	}
	defer pr.Close()

	// Perform parallel operations
	bgColor := color.RGBA{R: 50, G: 100, B: 150, A: 255}
	rectColor := color.RGBA{R: 200, G: 50, B: 100, A: 255}

	pr.Clear(bgColor)
	pr.FillRect(50, 50, 100, 100, rectColor)

	// Composite parallel result
	stride := width * 4
	parallelResult := make([]byte, height*stride)
	pr.Composite(parallelResult, stride)

	// Generate reference result serially
	serialResult := make([]byte, height*stride)

	// Serial clear
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := y*stride + x*4
			serialResult[offset] = 50
			serialResult[offset+1] = 100
			serialResult[offset+2] = 150
			serialResult[offset+3] = 255
		}
	}

	// Serial FillRect
	for y := 50; y < 150; y++ {
		for x := 50; x < 150; x++ {
			offset := y*stride + x*4
			serialResult[offset] = 200
			serialResult[offset+1] = 50
			serialResult[offset+2] = 100
			serialResult[offset+3] = 255
		}
	}

	// Compare results (pixel-perfect, tolerance 0)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := y*stride + x*4

			pR := parallelResult[offset]
			pG := parallelResult[offset+1]
			pB := parallelResult[offset+2]
			pA := parallelResult[offset+3]

			sR := serialResult[offset]
			sG := serialResult[offset+1]
			sB := serialResult[offset+2]
			sA := serialResult[offset+3]

			if pR != sR || pG != sG || pB != sB || pA != sA {
				t.Errorf("Pixel (%d,%d) parallel=[%d,%d,%d,%d], serial=[%d,%d,%d,%d]",
					x, y, pR, pG, pB, pA, sR, sG, sB, sA)
				return
			}
		}
	}
}

// TestVisualRegression_TileBoundaries tests that there are no visible seams
// at tile boundaries.
func TestVisualRegression_TileBoundaries(t *testing.T) {
	const (
		width  = 256
		height = 256
	)

	pr := parallel.NewParallelRasterizer(width, height)
	if pr == nil {
		t.Fatal("Failed to create parallel rasterizer")
	}
	defer pr.Close()

	// Fill with a solid color
	solidColor := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	pr.Clear(solidColor)

	// Composite to buffer
	stride := width * 4
	dst := make([]byte, height*stride)
	pr.Composite(dst, stride)

	// Check tile boundary pixels specifically
	tileBoundaries := []int{63, 64, 127, 128, 191, 192}

	for _, bx := range tileBoundaries {
		for _, by := range tileBoundaries {
			if bx >= width || by >= height {
				continue
			}

			offset := by*stride + bx*4

			got := [4]byte{dst[offset], dst[offset+1], dst[offset+2], dst[offset+3]}
			want := [4]byte{128, 128, 128, 255}

			if got != want {
				t.Errorf("Tile boundary pixel (%d,%d) = %v, want %v", bx, by, got, want)
			}
		}
	}
}

// TestVisualRegression_EdgeTiles tests that edge tiles (partial tiles at
// canvas boundaries) are rendered correctly.
func TestVisualRegression_EdgeTiles(t *testing.T) {
	// Use non-multiple-of-64 dimensions to create edge tiles
	const (
		width  = 200 // Creates 36-pixel-wide edge tile
		height = 200 // Creates 36-pixel-tall edge tile
	)

	pr := parallel.NewParallelRasterizer(width, height)
	if pr == nil {
		t.Fatal("Failed to create parallel rasterizer")
	}
	defer pr.Close()

	// Fill with gradient based on position
	tiles := pr.Grid().AllTiles()
	for _, tile := range tiles {
		tileX, tileY, tileW, tileH := tile.Bounds()

		for py := 0; py < tileH; py++ {
			for px := 0; px < tileW; px++ {
				canvasX := tileX + px
				canvasY := tileY + py

				offset := (py*tileW + px) * 4
				tile.Data[offset] = byte(canvasX)
				tile.Data[offset+1] = byte(canvasY)
				tile.Data[offset+2] = 0
				tile.Data[offset+3] = 255
			}
		}
	}

	// Composite to buffer
	stride := width * 4
	dst := make([]byte, height*stride)
	pr.Composite(dst, stride)

	// Verify edge pixels are correct
	edgePixels := []struct {
		x, y int
	}{
		{199, 0},   // Right edge
		{0, 199},   // Bottom edge
		{199, 199}, // Corner
		{128, 199}, // Bottom edge past first tile
		{199, 128}, // Right edge past first tile
	}

	for _, ep := range edgePixels {
		offset := ep.y*stride + ep.x*4

		gotR := dst[offset]
		gotG := dst[offset+1]

		wantR := byte(ep.x)
		wantG := byte(ep.y)

		if gotR != wantR || gotG != wantG {
			t.Errorf("Edge pixel (%d,%d) R,G = [%d,%d], want [%d,%d]",
				ep.x, ep.y, gotR, gotG, wantR, wantG)
		}
	}
}

// TestVisualRegression_MultipleOperations tests a sequence of operations
// produces correct results.
func TestVisualRegression_MultipleOperations(t *testing.T) {
	const (
		width  = 256
		height = 256
	)

	pr := parallel.NewParallelRasterizer(width, height)
	if pr == nil {
		t.Fatal("Failed to create parallel rasterizer")
	}
	defer pr.Close()

	// Perform a sequence of operations
	pr.Clear(color.Black)
	pr.FillRect(0, 0, 128, 128, color.RGBA{R: 255, G: 0, B: 0, A: 255})       // Red top-left
	pr.FillRect(128, 0, 128, 128, color.RGBA{R: 0, G: 255, B: 0, A: 255})     // Green top-right
	pr.FillRect(0, 128, 128, 128, color.RGBA{R: 0, G: 0, B: 255, A: 255})     // Blue bottom-left
	pr.FillRect(128, 128, 128, 128, color.RGBA{R: 255, G: 255, B: 0, A: 255}) // Yellow bottom-right

	// Composite to buffer
	stride := width * 4
	dst := make([]byte, height*stride)
	pr.Composite(dst, stride)

	// Define expected colors for each quadrant
	quadrants := []struct {
		name         string
		testX, testY int
		want         [4]byte
	}{
		{"top-left (red)", 64, 64, [4]byte{255, 0, 0, 255}},
		{"top-right (green)", 192, 64, [4]byte{0, 255, 0, 255}},
		{"bottom-left (blue)", 64, 192, [4]byte{0, 0, 255, 255}},
		{"bottom-right (yellow)", 192, 192, [4]byte{255, 255, 0, 255}},
	}

	for _, q := range quadrants {
		offset := q.testY*stride + q.testX*4
		got := [4]byte{dst[offset], dst[offset+1], dst[offset+2], dst[offset+3]}

		if got != q.want {
			t.Errorf("Quadrant %s at (%d,%d) = %v, want %v",
				q.name, q.testX, q.testY, got, q.want)
		}
	}
}
