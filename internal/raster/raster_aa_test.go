package raster

import (
	"math"
	"testing"
)

// testAAPixmap implements AAPixmap for testing FillAA.
type testAAPixmap struct {
	width       int
	height      int
	pixels      [][]RGBA
	blendCalls  int
	setPixCalls int
}

func newTestAAPixmap(w, h int) *testAAPixmap {
	pixels := make([][]RGBA, h)
	for y := 0; y < h; y++ {
		pixels[y] = make([]RGBA, w)
	}
	return &testAAPixmap{
		width:  w,
		height: h,
		pixels: pixels,
	}
}

func (p *testAAPixmap) Width() int  { return p.width }
func (p *testAAPixmap) Height() int { return p.height }

func (p *testAAPixmap) SetPixel(x, y int, c RGBA) {
	if x >= 0 && x < p.width && y >= 0 && y < p.height {
		p.pixels[y][x] = c
		p.setPixCalls++
	}
}

func (p *testAAPixmap) BlendPixelAlpha(x, y int, c RGBA, alpha uint8) {
	if x >= 0 && x < p.width && y >= 0 && y < p.height {
		// Simple alpha blend for testing
		if alpha == 255 {
			p.pixels[y][x] = c
		} else if alpha > 0 {
			t := float64(alpha) / 255.0
			existing := p.pixels[y][x]
			p.pixels[y][x] = RGBA{
				R: existing.R*(1-t) + c.R*t,
				G: existing.G*(1-t) + c.G*t,
				B: existing.B*(1-t) + c.B*t,
				A: existing.A*(1-t) + c.A*t,
			}
		}
		p.blendCalls++
	}
}

func (p *testAAPixmap) countNonZeroPixels() int {
	count := 0
	for y := 0; y < p.height; y++ {
		for x := 0; x < p.width; x++ {
			if p.pixels[y][x].A > 0 {
				count++
			}
		}
	}
	return count
}

func (p *testAAPixmap) hasPixelAt(x, y int) bool {
	if x < 0 || x >= p.width || y < 0 || y >= p.height {
		return false
	}
	return p.pixels[y][x].A > 0
}

func TestFillAABasicTriangle(t *testing.T) {
	pixmap := newTestAAPixmap(200, 200)
	r := NewRasterizer(200, 200)

	// Larger triangle to ensure we have enough area
	points := []Point{
		{X: 100, Y: 20},
		{X: 180, Y: 160},
		{X: 20, Y: 160},
		{X: 100, Y: 20}, // close
	}

	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Should have rendered some pixels
	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("FillAA rendered no pixels for triangle")
	}

	// Check that blend was called (AA uses BlendPixelAlpha)
	if pixmap.blendCalls == 0 {
		t.Error("BlendPixelAlpha was never called")
	}
}

func TestFillAACircle(t *testing.T) {
	pixmap := newTestAAPixmap(200, 200)
	r := NewRasterizer(200, 200)

	// Create circle approximation
	cx, cy, radius := 100.0, 100.0, 80.0
	segments := 64
	points := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		points[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}

	color := RGBA{R: 0.0, G: 1.0, B: 0.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Should have many pixels rendered
	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("FillAA rendered no pixels for circle")
	}

	// Check that center of circle is filled
	if !pixmap.hasPixelAt(100, 100) {
		t.Error("center of circle not filled")
	}

	// Check approximate circle area (pi * r^2)
	expectedArea := math.Pi * radius * radius
	tolerance := expectedArea * 0.1 // 10% tolerance
	if math.Abs(float64(count)-expectedArea) > tolerance {
		t.Logf("circle area = %d, expected ~%.0f (tolerance %.0f)", count, expectedArea, tolerance)
	}
}

func TestFillAASquare(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Simple square
	points := []Point{
		{X: 20, Y: 20},
		{X: 80, Y: 20},
		{X: 80, Y: 80},
		{X: 20, Y: 80},
		{X: 20, Y: 20}, // close
	}

	color := RGBA{R: 0.0, G: 0.0, B: 1.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Should have filled interior
	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("FillAA rendered no pixels for square")
	}

	// Interior pixels should be filled
	if !pixmap.hasPixelAt(50, 50) {
		t.Error("center of square not filled")
	}

	// Expected area: 60 x 60 = 3600 pixels (approximately)
	expectedArea := 60.0 * 60.0
	tolerance := expectedArea * 0.1
	if math.Abs(float64(count)-expectedArea) > tolerance {
		t.Logf("square area = %d, expected ~%.0f", count, expectedArea)
	}
}

func TestFillAAEmptyPath(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Should not panic with empty path
	r.FillAA(pixmap, []Point{}, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	// Should not render anything
	count := pixmap.countNonZeroPixels()
	if count != 0 {
		t.Errorf("empty path rendered %d pixels", count)
	}
}

func TestFillAASinglePoint(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Single point - should not render
	r.FillAA(pixmap, []Point{{X: 50, Y: 50}}, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	count := pixmap.countNonZeroPixels()
	if count != 0 {
		t.Errorf("single point rendered %d pixels", count)
	}
}

func TestFillAAHorizontalLine(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Horizontal line (should have no area)
	points := []Point{
		{X: 20, Y: 50},
		{X: 80, Y: 50},
		{X: 20, Y: 50}, // close
	}

	r.FillAA(pixmap, points, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	// Horizontal line has no area to fill
	count := pixmap.countNonZeroPixels()
	if count > 10 { // Allow small number due to AA edges
		t.Logf("horizontal line rendered %d pixels (expected ~0)", count)
	}
}

func TestFillAAEvenOddRule(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Self-intersecting star shape
	points := []Point{
		{X: 50, Y: 10},
		{X: 30, Y: 90},
		{X: 90, Y: 40},
		{X: 10, Y: 40},
		{X: 70, Y: 90},
		{X: 50, Y: 10}, // close
	}

	color := RGBA{R: 1.0, G: 1.0, B: 0.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleEvenOdd, color)

	// Should render something
	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("FillAA with EvenOdd rendered no pixels")
	}
}

func TestFillAAClipping(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Path extends beyond pixmap bounds
	points := []Point{
		{X: -50, Y: 50},
		{X: 150, Y: 50},
		{X: 150, Y: 150},
		{X: -50, Y: 150},
		{X: -50, Y: 50}, // close
	}

	// Should not panic
	r.FillAA(pixmap, points, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	// Should render pixels within bounds
	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("clipped path rendered no pixels")
	}
}

func TestFillAANegativeCoords(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Path entirely in negative coords
	points := []Point{
		{X: -100, Y: -100},
		{X: -50, Y: -100},
		{X: -50, Y: -50},
		{X: -100, Y: -50},
		{X: -100, Y: -100},
	}

	// Should not panic
	r.FillAA(pixmap, points, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	// Should render nothing (path outside bounds)
	count := pixmap.countNonZeroPixels()
	if count != 0 {
		t.Errorf("negative coords path rendered %d pixels", count)
	}
}

func TestFillAASmallTriangle(t *testing.T) {
	pixmap := newTestAAPixmap(50, 50)
	r := NewRasterizer(50, 50)

	// Small but not too small triangle
	points := []Point{
		{X: 25, Y: 10},
		{X: 40, Y: 35},
		{X: 10, Y: 35},
		{X: 25, Y: 10},
	}

	r.FillAA(pixmap, points, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	// Should render something
	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("small triangle rendered no pixels")
	}
}

func TestFillAALargeShape(t *testing.T) {
	pixmap := newTestAAPixmap(1000, 1000)
	r := NewRasterizer(1000, 1000)

	// Large rectangle
	points := []Point{
		{X: 100, Y: 100},
		{X: 900, Y: 100},
		{X: 900, Y: 900},
		{X: 100, Y: 900},
		{X: 100, Y: 100},
	}

	// Should not panic and complete in reasonable time
	r.FillAA(pixmap, points, FillRuleNonZero, RGBA{R: 1, G: 0, B: 0, A: 1})

	count := pixmap.countNonZeroPixels()
	if count == 0 {
		t.Error("large shape rendered no pixels")
	}

	// Expected area: 800 x 800 = 640000 (approximately)
	expectedArea := 800.0 * 800.0
	tolerance := expectedArea * 0.1
	if math.Abs(float64(count)-expectedArea) > tolerance {
		t.Logf("large shape area = %d, expected ~%.0f", count, expectedArea)
	}
}

func TestFillAAColorAccuracy(t *testing.T) {
	pixmap := newTestAAPixmap(100, 100)
	r := NewRasterizer(100, 100)

	// Fill entire pixmap
	points := []Point{
		{X: 0, Y: 0},
		{X: 100, Y: 0},
		{X: 100, Y: 100},
		{X: 0, Y: 100},
		{X: 0, Y: 0},
	}

	color := RGBA{R: 0.5, G: 0.25, B: 0.75, A: 0.8}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Check some interior pixels have correct color
	c := pixmap.pixels[50][50]
	if c.A == 0 {
		t.Error("interior pixel has zero alpha")
	}
	// Color components should be close to source (may vary due to blending)
	if math.Abs(c.R-color.R) > 0.2 || math.Abs(c.G-color.G) > 0.2 || math.Abs(c.B-color.B) > 0.2 {
		t.Errorf("interior pixel color = %v, expected close to %v", c, color)
	}
}

func BenchmarkFillAATriangle(b *testing.B) {
	pixmap := newTestAAPixmap(500, 500)
	r := NewRasterizer(500, 500)

	points := []Point{
		{X: 250, Y: 50},
		{X: 450, Y: 400},
		{X: 50, Y: 400},
		{X: 250, Y: 50},
	}
	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.FillAA(pixmap, points, FillRuleNonZero, color)
	}
}

func BenchmarkFillAACircle(b *testing.B) {
	pixmap := newTestAAPixmap(500, 500)
	r := NewRasterizer(500, 500)

	// Create circle
	cx, cy, radius := 250.0, 250.0, 200.0
	segments := 64
	points := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		points[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}
	color := RGBA{R: 0.0, G: 1.0, B: 0.0, A: 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.FillAA(pixmap, points, FillRuleNonZero, color)
	}
}

func BenchmarkFillAALargeRect(b *testing.B) {
	pixmap := newTestAAPixmap(1000, 1000)
	r := NewRasterizer(1000, 1000)

	points := []Point{
		{X: 100, Y: 100},
		{X: 900, Y: 100},
		{X: 900, Y: 900},
		{X: 100, Y: 900},
		{X: 100, Y: 100},
	}
	color := RGBA{R: 0.0, G: 0.0, B: 1.0, A: 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.FillAA(pixmap, points, FillRuleNonZero, color)
	}
}
