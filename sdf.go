package gg

import "math"

// sdfAntialiasWidth controls the smoothstep transition width in pixels.
// A value of 0.7 produces smooth anti-aliasing at standard DPI.
const sdfAntialiasWidth = 0.7

// SDFFilledCircleCoverage computes anti-aliased coverage for a filled circle
// using a signed distance field approach.
//
// Parameters:
//   - px, py: pixel center coordinates
//   - cx, cy: circle center
//   - radius: circle radius
//
// Returns a coverage value in [0, 1] where 1 means fully inside.
func SDFFilledCircleCoverage(px, py, cx, cy, radius float64) float64 {
	dist := math.Hypot(px-cx, py-cy)
	sdf := dist - radius
	return smoothstepCoverage(sdf)
}

// SDFCircleCoverage computes anti-aliased coverage for a stroked circle
// using a signed distance field approach.
//
// Parameters:
//   - px, py: pixel center coordinates
//   - cx, cy: circle center
//   - radius: circle radius (to center of stroke)
//   - halfStrokeWidth: half the stroke width
//
// Returns a coverage value in [0, 1] where 1 means fully inside the stroke.
func SDFCircleCoverage(px, py, cx, cy, radius, halfStrokeWidth float64) float64 {
	dist := math.Hypot(px-cx, py-cy)
	sdf := math.Abs(dist-radius) - halfStrokeWidth
	return smoothstepCoverage(sdf)
}

// SDFFilledRRectCoverage computes anti-aliased coverage for a filled rounded
// rectangle using a signed distance field approach.
//
// Parameters:
//   - px, py: pixel center coordinates
//   - cx, cy: rectangle center
//   - halfW, halfH: half-width and half-height of the rectangle
//   - cornerRadius: radius of the rounded corners
//
// Returns a coverage value in [0, 1] where 1 means fully inside.
func SDFFilledRRectCoverage(px, py, cx, cy, halfW, halfH, cornerRadius float64) float64 {
	dist := sdfRRect(px, py, cx, cy, halfW, halfH, cornerRadius)
	return smoothstepCoverage(dist)
}

// SDFRRectCoverage computes anti-aliased coverage for a stroked rounded
// rectangle using a signed distance field approach.
//
// Parameters:
//   - px, py: pixel center coordinates
//   - cx, cy: rectangle center
//   - halfW, halfH: half-width and half-height of the rectangle
//   - cornerRadius: radius of the rounded corners
//   - halfStrokeWidth: half the stroke width
//
// Returns a coverage value in [0, 1] where 1 means fully inside the stroke.
func SDFRRectCoverage(px, py, cx, cy, halfW, halfH, cornerRadius, halfStrokeWidth float64) float64 {
	dist := sdfRRect(px, py, cx, cy, halfW, halfH, cornerRadius)
	sdf := math.Abs(dist) - halfStrokeWidth
	return smoothstepCoverage(sdf)
}

// sdfRRect computes the signed distance from a point to a rounded rectangle.
// Negative values are inside, positive values are outside.
func sdfRRect(px, py, cx, cy, halfW, halfH, cornerRadius float64) float64 {
	// Translate to center and use symmetry (work in first quadrant).
	dx := math.Abs(px-cx) - halfW + cornerRadius
	dy := math.Abs(py-cy) - halfH + cornerRadius

	// Outside the corner region: max(dx, dy) gives the distance to the edge.
	// Inside the corner region: the Euclidean distance to the corner circle.
	outside := math.Sqrt(math.Max(dx, 0)*math.Max(dx, 0) + math.Max(dy, 0)*math.Max(dy, 0))
	inside := math.Min(math.Max(dx, dy), 0)

	return outside + inside - cornerRadius
}

// smoothstepCoverage converts a signed distance to an anti-aliased coverage
// value using a Hermite smoothstep function.
//
// sdf < -afwidth => 1.0 (fully inside)
// sdf > +afwidth => 0.0 (fully outside)
// Otherwise       => smooth transition
func smoothstepCoverage(sdf float64) float64 {
	if sdf >= sdfAntialiasWidth {
		return 0
	}
	if sdf <= -sdfAntialiasWidth {
		return 1
	}
	t := (sdf + sdfAntialiasWidth) / (2 * sdfAntialiasWidth)
	// Hermite smoothstep: 3t^2 - 2t^3
	return 1 - (t * t * (3 - 2*t))
}
