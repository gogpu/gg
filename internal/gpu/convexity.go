//go:build !nogpu

package gpu

import "github.com/gogpu/gg"

// convexityEpsilon is the tolerance for cross product comparisons.
// Values below this threshold are treated as zero (collinear edges).
const convexityEpsilon = 1e-10

// maxConvexDirectionFlips is the maximum number of direction sign changes
// allowed per axis for a convex polygon. A simple convex polygon traversed
// once changes direction at most twice on each axis (e.g., X goes right then
// left, Y goes up then down). Stroke-expanded outlines with inner join
// pivots can produce more flips while still having consistent cross-product
// signs — this check catches those self-intersecting false positives.
//
// Matches Skia IsConcaveBySign (SkPathPriv.cpp:445, threshold 3) and
// femtovg (path/cache.rs:864, requires exactly 2).
const maxConvexDirectionFlips = 3

// ConvexityResult provides detailed convexity analysis of a polygon.
type ConvexityResult struct {
	// Convex is true if the polygon is strictly convex (all turns in the same direction).
	Convex bool

	// Winding is the winding direction: +1 for counter-clockwise, -1 for clockwise,
	// 0 for degenerate polygons (fewer than 3 non-collinear points).
	Winding int

	// NumPoints is the number of points analyzed.
	NumPoints int
}

// IsConvex checks if a sequence of points forms a convex polygon.
//
// Points should represent a single closed contour after curve flattening.
// The polygon is considered closed (last point connects back to first).
// Returns true if the polygon is strictly convex, false if concave, self-intersecting,
// or degenerate (fewer than 3 points or all points collinear).
//
// This is an O(n) algorithm that computes the cross product of consecutive edge
// vectors and verifies they all have the same sign, combined with a direction-flip
// check per axis (Skia IsConcaveBySign / femtovg pattern) to reject self-intersecting
// stroke outlines that pass the cross-product check.
func IsConvex(points []gg.Point) bool {
	return AnalyzeConvexity(points).Convex
}

// AnalyzeConvexity performs detailed convexity analysis of a polygon.
//
// The analysis performs two checks (both must pass for convexity):
//  1. Cross-product sign consistency: all turns in the same direction.
//  2. Direction-flip count: each axis (X, Y) changes sign at most
//     maxConvexDirectionFlips times. This rejects self-intersecting stroke
//     outlines that pass the cross-product check due to near-duplicate
//     inner join pivot points.
//
// This is an O(n) algorithm.
func AnalyzeConvexity(points []gg.Point) ConvexityResult {
	n := len(points)
	result := ConvexityResult{NumPoints: n}

	if n < 3 {
		return result
	}

	positiveCount, negativeCount := countCrossProductSigns(points)

	if positiveCount == 0 && negativeCount == 0 {
		return result
	}

	if positiveCount > 0 && negativeCount > 0 {
		return result
	}

	xFlips, yFlips := countDirectionFlips(points)
	if xFlips > maxConvexDirectionFlips || yFlips > maxConvexDirectionFlips {
		return result
	}

	result.Convex = true
	if positiveCount > 0 {
		result.Winding = 1 // CCW
	} else {
		result.Winding = -1 // CW
	}
	return result
}

// countCrossProductSigns counts how many consecutive edge pairs have
// positive vs negative cross products around the polygon boundary.
func countCrossProductSigns(points []gg.Point) (positive, negative int) {
	n := len(points)
	for i := 0; i < n; i++ {
		p0 := points[i]
		p1 := points[(i+1)%n]
		p2 := points[(i+2)%n]

		e1x := p1.X - p0.X
		e1y := p1.Y - p0.Y
		e2x := p2.X - p1.X
		e2y := p2.Y - p1.Y

		cross := e1x*e2y - e1y*e2x

		if cross > convexityEpsilon {
			positive++
		} else if cross < -convexityEpsilon {
			negative++
		}
	}
	return positive, negative
}

// countDirectionFlips counts how many times the edge direction changes sign
// on each axis (X and Y). A simple convex polygon changes direction at most
// twice per axis (going around the boundary once). Self-intersecting stroke
// outlines with inner join V-shapes produce many more flips.
//
// Matches Skia IsConcaveBySign (SkPathPriv.cpp:445).
func countDirectionFlips(points []gg.Point) (xFlips, yFlips int) {
	n := len(points)
	var prevDX, prevDY int

	for i := 0; i < n; i++ {
		p0 := points[i]
		p1 := points[(i+1)%n]

		dx := sign(p1.X - p0.X)
		dy := sign(p1.Y - p0.Y)

		if i > 0 {
			if dx != 0 && prevDX != 0 && dx != prevDX {
				xFlips++
			}
			if dy != 0 && prevDY != 0 && dy != prevDY {
				yFlips++
			}
		}
		if dx != 0 {
			prevDX = dx
		}
		if dy != 0 {
			prevDY = dy
		}
	}
	return xFlips, yFlips
}

func sign(v float64) int {
	if v > convexityEpsilon {
		return 1
	}
	if v < -convexityEpsilon {
		return -1
	}
	return 0
}
