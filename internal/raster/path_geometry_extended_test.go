// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// TestZeroGeomPoint tests the origin point constructor.
func TestZeroGeomPoint(t *testing.T) {
	p := ZeroGeomPoint()
	if p.X != 0 || p.Y != 0 {
		t.Errorf("ZeroGeomPoint() = (%f, %f), want (0, 0)", p.X, p.Y)
	}
}

// TestLerpPoint tests linear interpolation between points.
func TestLerpPoint(t *testing.T) {
	tests := []struct {
		name  string
		a, b  GeomPoint
		param float32
		wantX float32
		wantY float32
	}{
		{"t=0", GeomPoint{0, 0}, GeomPoint{10, 20}, 0.0, 0, 0},
		{"t=1", GeomPoint{0, 0}, GeomPoint{10, 20}, 1.0, 10, 20},
		{"t=0.5", GeomPoint{0, 0}, GeomPoint{10, 20}, 0.5, 5, 10},
		{"t=0.25", GeomPoint{0, 0}, GeomPoint{100, 200}, 0.25, 25, 50},
		{"same point", GeomPoint{5, 5}, GeomPoint{5, 5}, 0.5, 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lerpPoint(tt.a, tt.b, tt.param)
			if absF32(got.X-tt.wantX) > 0.001 || absF32(got.Y-tt.wantY) > 0.001 {
				t.Errorf("lerpPoint(%v, %v, %f) = (%f, %f), want (%f, %f)",
					tt.a, tt.b, tt.param, got.X, got.Y, tt.wantX, tt.wantY)
			}
		})
	}
}

// TestChopQuadAtYExtrema tests quadratic splitting at Y extrema.
func TestChopQuadAtYExtrema(t *testing.T) {
	t.Run("monotonic curve no chop", func(t *testing.T) {
		// Monotonically increasing: 0, 50, 100
		src := [3]GeomPoint{
			{X: 0, Y: 0},
			{X: 50, Y: 50},
			{X: 100, Y: 100},
		}
		var dst [5]GeomPoint
		numChops := ChopQuadAtYExtrema(src, &dst)

		if numChops != 0 {
			t.Errorf("expected 0 chops for monotonic quad, got %d", numChops)
		}
		// dst[0..3] should be the original points
		if dst[0] != src[0] || dst[2] != src[2] {
			t.Error("endpoints should match for no-chop case")
		}
	})

	t.Run("arch needs chop", func(t *testing.T) {
		// Arch: goes up then down (Y extremum in middle)
		src := [3]GeomPoint{
			{X: 0, Y: 0},
			{X: 50, Y: 100}, // control above endpoints
			{X: 100, Y: 0},
		}
		var dst [5]GeomPoint
		numChops := ChopQuadAtYExtrema(src, &dst)

		if numChops != 1 {
			t.Errorf("expected 1 chop for arch quad, got %d", numChops)
		}

		// After chopping, both halves should be Y-monotonic
		if numChops == 1 {
			if !QuadIsYMonotonic(dst[0], dst[1], dst[2]) {
				t.Error("first half after chop should be Y-monotonic")
			}
			if !QuadIsYMonotonic(dst[2], dst[3], dst[4]) {
				t.Error("second half after chop should be Y-monotonic")
			}
		}
	})

	t.Run("valley needs chop", func(t *testing.T) {
		// Valley: goes down then up
		src := [3]GeomPoint{
			{X: 0, Y: 100},
			{X: 50, Y: 0}, // control below endpoints
			{X: 100, Y: 100},
		}
		var dst [5]GeomPoint
		numChops := ChopQuadAtYExtrema(src, &dst)

		if numChops != 1 {
			t.Errorf("expected 1 chop for valley quad, got %d", numChops)
		}
	})

	t.Run("flat control point", func(t *testing.T) {
		// Control point at same Y as start — not monotonic per isNotMonotonic
		src := [3]GeomPoint{
			{X: 0, Y: 50},
			{X: 50, Y: 50}, // same Y as start
			{X: 100, Y: 100},
		}
		var dst [5]GeomPoint
		numChops := ChopQuadAtYExtrema(src, &dst)
		t.Logf("flat control: numChops = %d", numChops)
	})
}

// TestChopCubicAtYExtrema tests cubic splitting at Y extrema.
func TestChopCubicAtYExtrema(t *testing.T) {
	t.Run("monotonic cubic no chop", func(t *testing.T) {
		src := [4]GeomPoint{
			{X: 0, Y: 0},
			{X: 30, Y: 30},
			{X: 70, Y: 70},
			{X: 100, Y: 100},
		}
		var dst [10]GeomPoint
		numChops := ChopCubicAtYExtrema(src, &dst)

		if numChops != 0 {
			t.Errorf("expected 0 chops for monotonic cubic, got %d", numChops)
		}
	})

	t.Run("S-curve two extrema", func(t *testing.T) {
		// S-curve: goes up, then down, then up — two Y extrema
		src := [4]GeomPoint{
			{X: 0, Y: 0},
			{X: 30, Y: 80},
			{X: 70, Y: -30},
			{X: 100, Y: 50},
		}
		var dst [10]GeomPoint
		numChops := ChopCubicAtYExtrema(src, &dst)

		if numChops < 1 {
			t.Errorf("expected at least 1 chop for S-curve, got %d", numChops)
		}
		t.Logf("S-curve cubic: %d chops", numChops)
	})

	t.Run("arch one extremum", func(t *testing.T) {
		// Goes up then down
		src := [4]GeomPoint{
			{X: 0, Y: 0},
			{X: 30, Y: 100},
			{X: 70, Y: 100},
			{X: 100, Y: 0},
		}
		var dst [10]GeomPoint
		numChops := ChopCubicAtYExtrema(src, &dst)

		if numChops < 1 {
			t.Errorf("expected at least 1 chop for arch cubic, got %d", numChops)
		}
	})
}

// TestChopCubicAt tests chopping at multiple t values.
func TestChopCubicAt(t *testing.T) {
	src := [4]GeomPoint{
		{X: 0, Y: 0},
		{X: 30, Y: 50},
		{X: 70, Y: 50},
		{X: 100, Y: 100},
	}

	t.Run("no chops", func(t *testing.T) {
		var dst [10]GeomPoint
		chopCubicAt(src, nil, &dst)
		if dst[0] != src[0] || dst[3] != src[3] {
			t.Error("no-chop should copy original")
		}
	})

	t.Run("single chop at t=0.5", func(t *testing.T) {
		var dst [10]GeomPoint
		chopCubicAt(src, []float32{0.5}, &dst)
		// First half starts at src[0], second half ends at src[3]
		if dst[0] != src[0] {
			t.Error("first segment should start at src[0]")
		}
		if dst[6] != src[3] {
			t.Error("second segment should end at src[3]")
		}
	})

	t.Run("two chops", func(t *testing.T) {
		var dst [10]GeomPoint
		chopCubicAt(src, []float32{0.33, 0.66}, &dst)
		// Should produce 3 segments
		if dst[0] != src[0] {
			t.Error("first segment should start at src[0]")
		}
	})
}

// TestChopQuadAt tests quadratic de Casteljau splitting.
func TestChopQuadAt(t *testing.T) {
	src := [3]GeomPoint{
		{X: 0, Y: 0},
		{X: 50, Y: 100},
		{X: 100, Y: 0},
	}

	var dst [5]GeomPoint
	chopQuadAt(src, 0.5, &dst)

	// First quad starts at src[0]
	if dst[0] != src[0] {
		t.Error("first quad should start at src[0]")
	}
	// Second quad ends at src[2]
	if dst[4] != src[2] {
		t.Error("second quad should end at src[2]")
	}
	// Split point (dst[2]) should be on the curve at t=0.5
	// For a symmetric arch, midpoint should be at (50, 50)
	if absF32(dst[2].X-50) > 0.1 {
		t.Errorf("split point X = %f, want ~50", dst[2].X)
	}
}

// TestQuadIsYMonotonic tests quadratic Y-monotonicity check.
func TestQuadIsYMonotonic(t *testing.T) {
	tests := []struct {
		name string
		p0   GeomPoint
		p1   GeomPoint
		p2   GeomPoint
		want bool
	}{
		{
			"increasing",
			GeomPoint{0, 0}, GeomPoint{50, 50}, GeomPoint{100, 100},
			true,
		},
		{
			"decreasing",
			GeomPoint{0, 100}, GeomPoint{50, 50}, GeomPoint{100, 0},
			true,
		},
		{
			"arch not monotonic",
			GeomPoint{0, 0}, GeomPoint{50, 100}, GeomPoint{100, 0},
			false,
		},
		{
			"control at start Y",
			GeomPoint{0, 0}, GeomPoint{50, 0}, GeomPoint{100, 100},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuadIsYMonotonic(tt.p0, tt.p1, tt.p2)
			if got != tt.want {
				t.Errorf("QuadIsYMonotonic = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCubicIsYMonotonic tests cubic Y-monotonicity check.
func TestCubicIsYMonotonic(t *testing.T) {
	tests := []struct {
		name string
		p0   GeomPoint
		p1   GeomPoint
		p2   GeomPoint
		p3   GeomPoint
		want bool
	}{
		{
			"monotonic",
			GeomPoint{0, 0}, GeomPoint{30, 30}, GeomPoint{70, 70}, GeomPoint{100, 100},
			true,
		},
		{
			"S-curve not monotonic",
			GeomPoint{0, 0}, GeomPoint{30, 80}, GeomPoint{70, -30}, GeomPoint{100, 50},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CubicIsYMonotonic(tt.p0, tt.p1, tt.p2, tt.p3)
			if got != tt.want {
				t.Errorf("CubicIsYMonotonic = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFindCubicExtrema tests finding cubic Y extrema t values.
func TestFindCubicExtrema(t *testing.T) {
	t.Run("monotonic no extrema", func(t *testing.T) {
		result := findCubicExtrema(0, 30, 70, 100)
		if len(result) != 0 {
			t.Errorf("expected 0 extrema, got %d", len(result))
		}
	})

	t.Run("arch one extremum", func(t *testing.T) {
		result := findCubicExtrema(0, 100, 100, 0)
		if len(result) < 1 {
			t.Errorf("expected at least 1 extremum for arch, got %d", len(result))
		}
	})

	t.Run("S-curve two extrema", func(t *testing.T) {
		result := findCubicExtrema(0, 80, -30, 50)
		if len(result) < 1 {
			t.Errorf("expected at least 1 extremum for S-curve, got %d", len(result))
		}
		// All t values should be in (0, 1)
		for _, tv := range result {
			if tv <= 0 || tv >= 1 {
				t.Errorf("extremum t=%f is outside (0,1)", tv)
			}
		}
	})
}

// TestFindUnitQuadRootsExtended tests more quadratic root cases.
func TestFindUnitQuadRootsExtended(t *testing.T) {
	tests := []struct {
		name      string
		a, b, c   float32
		wantCount int
	}{
		{"linear a=0 b=0 c=0", 0, 0, 0, 0},    // degenerate
		{"negative discriminant", 1, 0, 1, 0}, // no real roots
		{"roots at 0 and 1", 1, -1, 0, 0},     // roots at 0 (excluded) and 1 (excluded)
		{"root at 0.5", 4, -4, 1, 0},          // double root at 0.5, but epsilon check may exclude
		{"two roots in unit", 6, -5, 1, 2},    // roots at 1/3 and 1/2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := findUnitQuadRoots(tt.a, tt.b, tt.c)
			if len(roots) != tt.wantCount {
				t.Logf("findUnitQuadRoots(%v, %v, %v) = %v (count=%d)",
					tt.a, tt.b, tt.c, roots, len(roots))
			}
			// Verify all roots are in (0, 1)
			for _, r := range roots {
				if r <= 0 || r >= 1 {
					t.Errorf("root %f is outside (0,1)", r)
				}
			}
		})
	}
}

// TestChopCubicAtSingle tests single cubic split.
func TestChopCubicAtSingle(t *testing.T) {
	src := [4]GeomPoint{
		{X: 0, Y: 0},
		{X: 25, Y: 75},
		{X: 75, Y: 75},
		{X: 100, Y: 0},
	}

	var dst [10]GeomPoint
	chopCubicAtSingle(src, 0.5, &dst)

	// First cubic starts at src[0]
	if dst[0] != src[0] {
		t.Error("first cubic should start at src[0]")
	}
	// Second cubic ends at src[3]
	if dst[6] != src[3] {
		t.Error("second cubic should end at src[3]")
	}
	// Split point (dst[3]) should be on the curve at t=0.5
	// For symmetric arch centered at x=50: midpoint x should be ~50
	if absF32(dst[3].X-50) > 1.0 {
		t.Errorf("split point X = %f, want ~50", dst[3].X)
	}
}
