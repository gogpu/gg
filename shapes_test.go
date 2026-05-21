package gg

import (
	"math"
	"testing"
)

func TestDrawRegularPolygon_TriangleVertexUp(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	dc.DrawRegularPolygon(3, 50, 50, 30, 0)

	// Triangle with rotation=0 should have top vertex at (50, 20) = center - radius
	// i.e., first vertex at top (12 o'clock), matching fogleman/gg
	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Fatal("no current point after DrawRegularPolygon")
	}
	// After ClosePath, current point is at the first vertex
	// First vertex: rotation adjusted by -π/2, so angle = -π/2
	// px = 50 + 30*cos(-π/2) = 50 + 0 = 50
	// py = 50 + 30*sin(-π/2) = 50 - 30 = 20
	expectedX, expectedY := 50.0, 20.0
	if math.Abs(x-expectedX) > 0.01 || math.Abs(y-expectedY) > 0.01 {
		t.Errorf("triangle first vertex = (%.2f, %.2f); want (%.2f, %.2f) (top)",
			x, y, expectedX, expectedY)
	}
}

func TestDrawRegularPolygon_SquareFlatTop(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	dc.DrawRegularPolygon(4, 50, 50, 30, 0)

	// Square with rotation=0 should have flat top (not diamond).
	// Even-sided: rotation adjusted by -π/2 + π/4 = -π/4
	// First vertex at angle -π/4: top-right corner
	// px = 50 + 30*cos(-π/4) ≈ 50 + 21.21 ≈ 71.21
	// py = 50 + 30*sin(-π/4) ≈ 50 - 21.21 ≈ 28.79
	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Fatal("no current point")
	}
	expectedX := 50.0 + 30.0*math.Cos(-math.Pi/4)
	expectedY := 50.0 + 30.0*math.Sin(-math.Pi/4)
	if math.Abs(x-expectedX) > 0.01 || math.Abs(y-expectedY) > 0.01 {
		t.Errorf("square first vertex = (%.2f, %.2f); want (%.2f, %.2f) (top-right)",
			x, y, expectedX, expectedY)
	}
}

func TestDrawRegularPolygon_HexagonFlatTop(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	dc.DrawRegularPolygon(6, 50, 50, 30, 0)

	// Hexagon (even): flat top. First vertex at angle -π/2 + π/6 = -π/3
	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Fatal("no current point")
	}
	expectedX := 50.0 + 30.0*math.Cos(-math.Pi/3)
	expectedY := 50.0 + 30.0*math.Sin(-math.Pi/3)
	if math.Abs(x-expectedX) > 0.01 || math.Abs(y-expectedY) > 0.01 {
		t.Errorf("hexagon first vertex = (%.2f, %.2f); want (%.2f, %.2f)",
			x, y, expectedX, expectedY)
	}
}

func TestDrawRegularPolygon_PentagonVertexUp(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	dc.DrawRegularPolygon(5, 50, 50, 30, 0)

	// Pentagon (odd): vertex at top. First vertex at angle -π/2
	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Fatal("no current point")
	}
	expectedX := 50.0
	expectedY := 50.0 - 30.0
	if math.Abs(x-expectedX) > 0.01 || math.Abs(y-expectedY) > 0.01 {
		t.Errorf("pentagon first vertex = (%.2f, %.2f); want (%.2f, %.2f) (top)",
			x, y, expectedX, expectedY)
	}
}

func TestDrawRegularPolygon_CustomRotation(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	// rotation=π/2 on triangle: vertex should point right (3 o'clock)
	dc.DrawRegularPolygon(3, 50, 50, 30, math.Pi/2)

	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Fatal("no current point")
	}
	// rotation π/2 offset by -π/2 (odd) = 0 → first vertex at 0 radians = right
	expectedX := 50.0 + 30.0
	expectedY := 50.0
	if math.Abs(x-expectedX) > 0.01 || math.Abs(y-expectedY) > 0.01 {
		t.Errorf("rotated triangle first vertex = (%.2f, %.2f); want (%.2f, %.2f) (right)",
			x, y, expectedX, expectedY)
	}
}
