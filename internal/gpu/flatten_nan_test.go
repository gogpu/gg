//go:build !nogpu

package gpu

import (
	"math"
	"testing"
)

// TestFlattenQuadratic_NaN_NoCrash verifies that flattenQuadratic with NaN
// control points terminates via depth guard rather than infinite recursion.
func TestFlattenQuadratic_NaN_NoCrash(t *testing.T) {
	tests := []struct {
		name                   string
		x0, y0, cx, cy, x1, y1 float32
	}{
		{
			name: "NaN control point",
			x0:   0, y0: 0,
			cx: float32(math.NaN()), cy: float32(math.NaN()),
			x1: 100, y1: 100,
		},
		{
			name: "NaN endpoint",
			x0:   0, y0: 0,
			cx: 50, cy: 50,
			x1: float32(math.NaN()), y1: float32(math.NaN()),
		},
		{
			name: "NaN start point",
			x0:   float32(math.NaN()), y0: float32(math.NaN()),
			cx: 50, cy: 50,
			x1: 100, y1: 100,
		},
		{
			name: "all NaN",
			x0:   float32(math.NaN()), y0: float32(math.NaN()),
			cx: float32(math.NaN()), cy: float32(math.NaN()),
			x1: float32(math.NaN()), y1: float32(math.NaN()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := NewSegmentList()
			// Must not panic or infinite-loop.
			flattenQuadratic(segments, tt.x0, tt.y0, tt.cx, tt.cy, tt.x1, tt.y1, FlattenTolerance)
		})
	}
}

// TestFlattenCubic_NaN_NoCrash verifies that flattenCubic with NaN control
// points terminates via depth guard rather than infinite recursion.
func TestFlattenCubic_NaN_NoCrash(t *testing.T) {
	tests := []struct {
		name                               string
		x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32
	}{
		{
			name: "NaN control point 1",
			x0:   0, y0: 0,
			c1x: float32(math.NaN()), c1y: 0,
			c2x: 50, c2y: 50,
			x1: 100, y1: 100,
		},
		{
			name: "NaN control point 2",
			x0:   0, y0: 0,
			c1x: 25, c1y: 25,
			c2x: float32(math.NaN()), c2y: float32(math.NaN()),
			x1: 100, y1: 100,
		},
		{
			name: "NaN both control points",
			x0:   0, y0: 0,
			c1x: float32(math.NaN()), c1y: float32(math.NaN()),
			c2x: float32(math.NaN()), c2y: float32(math.NaN()),
			x1: 100, y1: 100,
		},
		{
			name: "NaN endpoint",
			x0:   0, y0: 0,
			c1x: 25, c1y: 25,
			c2x: 75, c2y: 75,
			x1: float32(math.NaN()), y1: float32(math.NaN()),
		},
		{
			name: "all NaN",
			x0:   float32(math.NaN()), y0: float32(math.NaN()),
			c1x: float32(math.NaN()), c1y: float32(math.NaN()),
			c2x: float32(math.NaN()), c2y: float32(math.NaN()),
			x1: float32(math.NaN()), y1: float32(math.NaN()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := NewSegmentList()
			// Must not panic or infinite-loop.
			flattenCubic(segments, tt.x0, tt.y0, tt.c1x, tt.c1y, tt.c2x, tt.c2y, tt.x1, tt.y1, FlattenTolerance)
		})
	}
}

// TestFlattenQuadratic_Inf_NoCrash verifies that Inf coordinates are also
// handled safely by the depth guard.
func TestFlattenQuadratic_Inf_NoCrash(t *testing.T) {
	segments := NewSegmentList()
	flattenQuadratic(segments, 0, 0,
		float32(math.Inf(1)), float32(math.Inf(-1)),
		100, 100, FlattenTolerance)
	// Must not panic.
}

// TestFlattenCubic_Inf_NoCrash verifies that Inf coordinates are also
// handled safely by the depth guard.
func TestFlattenCubic_Inf_NoCrash(t *testing.T) {
	segments := NewSegmentList()
	flattenCubic(segments, 0, 0,
		float32(math.Inf(1)), 0,
		0, float32(math.Inf(-1)),
		100, 100, FlattenTolerance)
	// Must not panic.
}
