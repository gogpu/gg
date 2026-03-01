package gg

import (
	"math"
	"testing"
)

// Tests for software.go helper functions and dashed stroke paths.

func TestAdaptiveThreshold(t *testing.T) {
	tests := []struct {
		name     string
		bboxArea float64
	}{
		{"zero area", 0},
		{"negative area", -100},
		{"100x100", 10000},
		{"50x50", 2500},
		{"10x10", 100},
		{"1000x1000", 1000000},
		{"very large", 100000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveThreshold(tt.bboxArea)
			if got < minElementThreshold || got > maxElementThreshold {
				t.Errorf("adaptiveThreshold(%v) = %d, out of range [%d, %d]",
					tt.bboxArea, got, minElementThreshold, maxElementThreshold)
			}
		})
	}

	// Verify monotonicity: larger area -> smaller (or equal) threshold
	small := adaptiveThreshold(100)
	large := adaptiveThreshold(1000000)
	if large > small {
		t.Errorf("threshold should decrease with area: area=100 -> %d, area=1e6 -> %d", small, large)
	}
}

func TestPathBounds(t *testing.T) {
	tests := []struct {
		name               string
		buildPath          func() *Path
		wantMinX, wantMinY float64
		wantMaxX, wantMaxY float64
	}{
		{
			name:      "empty path",
			buildPath: NewPath,
			wantMinX:  0, wantMinY: 0, wantMaxX: 0, wantMaxY: 0,
		},
		{
			name: "simple rectangle",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(10, 20)
				p.LineTo(30, 20)
				p.LineTo(30, 40)
				p.LineTo(10, 40)
				p.Close()
				return p
			},
			wantMinX: 10, wantMinY: 20, wantMaxX: 30, wantMaxY: 40,
		},
		{
			name: "with quadratic",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(50, 100, 100, 0)
				return p
			},
			wantMinX: 0, wantMinY: 0, wantMaxX: 100, wantMaxY: 100,
		},
		{
			name: "with cubic",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(10, 50, 90, 50, 100, 0)
				return p
			},
			wantMinX: 0, wantMinY: 0, wantMaxX: 100, wantMaxY: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			minX, minY, maxX, maxY := pathBounds(p)
			if minX != tt.wantMinX || minY != tt.wantMinY || maxX != tt.wantMaxX || maxY != tt.wantMaxY {
				t.Errorf("pathBounds() = (%v, %v, %v, %v), want (%v, %v, %v, %v)",
					minX, minY, maxX, maxY, tt.wantMinX, tt.wantMinY, tt.wantMaxX, tt.wantMaxY)
			}
		})
	}
}

func TestShouldUseTileRasterizer(t *testing.T) {
	// Empty path should NOT use tile rasterizer
	empty := NewPath()
	if shouldUseTileRasterizer(empty) {
		t.Error("empty path should not use tile rasterizer")
	}

	// Simple path (few elements, small bbox) should NOT use tile rasterizer
	simple := NewPath()
	simple.MoveTo(0, 0)
	simple.LineTo(10, 0)
	simple.LineTo(10, 10)
	simple.Close()
	if shouldUseTileRasterizer(simple) {
		t.Error("simple triangle should not use tile rasterizer")
	}

	// Complex path (many elements, large bbox) should use tile rasterizer
	complexPath := NewPath()
	complexPath.MoveTo(0, 0)
	for i := 0; i < 500; i++ {
		x := float64(i) * 5
		y := 100 * math.Sin(float64(i)*0.1)
		complexPath.LineTo(x, y)
	}
	complexPath.Close()
	// This has 500+ elements in a ~2500x200 bbox, should trigger tile rasterizer
	if !shouldUseTileRasterizer(complexPath) {
		t.Error("complex path with 500+ elements in large bbox should use tile rasterizer")
	}
}

// TestDashedStroke_Lines tests dashed strokes with simple lines (no curves).
func TestDashedStroke_Lines(t *testing.T) {
	dc := NewContext(200, 200)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2)
	dc.SetDash(5, 3)

	dc.MoveTo(10, 100)
	dc.LineTo(190, 100)
	err := dc.Stroke()
	if err != nil {
		t.Errorf("Stroke with dashed line failed: %v", err)
	}
}

func TestContext_SavePNG(t *testing.T) {
	dc := NewContext(10, 10)
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 10, 10)
	_ = dc.Fill()

	err := dc.SavePNG("tmp/test_coverage_save.png")
	if err != nil {
		t.Errorf("SavePNG() = %v", err)
	}
}

func TestPixmap_SavePNG(t *testing.T) {
	pm := NewPixmap(10, 10)
	pm.SetPixel(5, 5, Red)

	err := pm.SavePNG("tmp/test_coverage_pixmap.png")
	if err != nil {
		t.Errorf("SavePNG() = %v", err)
	}
}
