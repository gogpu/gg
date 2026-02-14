//go:build !nogpu

package gpu

import "github.com/gogpu/gg"

// convexityEpsilon is the tolerance for cross product comparisons.
// Values below this threshold are treated as zero (collinear edges).
const convexityEpsilon = 1e-10

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
// vectors and verifies they all have the same sign.
func IsConvex(points []gg.Point) bool {
	return AnalyzeConvexity(points).Convex
}

// AnalyzeConvexity performs detailed convexity analysis of a polygon.
//
// Points should represent a single closed contour after curve flattening.
// The polygon is considered closed (last point connects back to first).
//
// The analysis checks that all cross products of consecutive edge vectors
// have the same sign, which guarantees convexity for simple polygons.
// Collinear edges (zero cross product) are permitted and do not break convexity.
//
// This is an O(n) algorithm.
func AnalyzeConvexity(points []gg.Point) ConvexityResult {
	n := len(points)
	result := ConvexityResult{NumPoints: n}

	// A polygon needs at least 3 vertices to be convex.
	if n < 3 {
		return result
	}

	// Walk all consecutive edge pairs and check cross product sign consistency.
	// Edge i goes from points[i] to points[(i+1)%n].
	// Cross product of edge[i] x edge[i+1] gives the turn direction.
	var positiveCount, negativeCount int

	for i := 0; i < n; i++ {
		// Three consecutive vertices forming two edges.
		p0 := points[i]
		p1 := points[(i+1)%n]
		p2 := points[(i+2)%n]

		// Edge vectors.
		e1 := gg.Point{X: p1.X - p0.X, Y: p1.Y - p0.Y}
		e2 := gg.Point{X: p2.X - p1.X, Y: p2.Y - p1.Y}

		// 2D cross product: e1.X * e2.Y - e1.Y * e2.X
		cross := e1.X*e2.Y - e1.Y*e2.X

		if cross > convexityEpsilon {
			positiveCount++
		} else if cross < -convexityEpsilon {
			negativeCount++
		}
		// cross ~= 0 means collinear edges, which are allowed.
	}

	// Degenerate: all edges are collinear (no turns at all).
	if positiveCount == 0 && negativeCount == 0 {
		return result
	}

	// Convex if all non-zero cross products have the same sign.
	if positiveCount > 0 && negativeCount > 0 {
		// Mixed signs: concave polygon.
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
