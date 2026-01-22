// Package stroke provides stroke expansion algorithms for converting stroked paths to filled outlines.
//
// This package implements CPU-side stroke expansion following tiny-skia and kurbo patterns.
// The algorithm converts a path with stroke style into a filled path suitable for GPU rasterization.
//
// # Algorithm Overview
//
// Stroke expansion works by building two parallel offset paths:
//   - Forward path: Offset by +width/2 perpendicular to the tangent
//   - Backward path: Offset by -width/2 perpendicular to the tangent
//
// The final filled path is constructed by:
//  1. Forward path goes forward
//  2. End cap connects forward to backward
//  3. Backward path is reversed
//  4. Start cap connects backward to forward and closes
//
// # Line Caps
//
// Line caps define the shape of stroke endpoints:
//   - LineCapButt: Flat cap ending exactly at the endpoint
//   - LineCapRound: Semicircular cap with radius = width/2
//   - LineCapSquare: Square cap extending width/2 beyond the endpoint
//
// # Line Joins
//
// Line joins define how stroke segments connect:
//   - LineJoinMiter: Sharp corner (limited by miter limit)
//   - LineJoinRound: Circular arc at corners
//   - LineJoinBevel: Straight line across the corner
//
// # Usage
//
//	style := stroke.Stroke{
//	    Width:      2.0,
//	    Cap:        stroke.LineCapRound,
//	    Join:       stroke.LineJoinMiter,
//	    MiterLimit: 4.0,
//	}
//
//	expander := stroke.NewStrokeExpander(style)
//	expander.SetTolerance(0.1) // Optional: adjust curve flattening
//
//	inputPath := []stroke.PathElement{
//	    stroke.MoveTo{Point: stroke.Point{X: 0, Y: 0}},
//	    stroke.LineTo{Point: stroke.Point{X: 100, Y: 0}},
//	    stroke.LineTo{Point: stroke.Point{X: 100, Y: 100}},
//	}
//
//	filledPath := expander.Expand(inputPath)
//
// # Performance
//
// The expander is optimized for typical use cases:
//   - Simple lines: ~2 microseconds
//   - Complex paths (100 segments): ~50-60 microseconds
//   - Cubic curves are flattened to line segments based on tolerance
//
// # References
//
// The algorithm is based on:
//   - tiny-skia (Rust): path/src/stroker.rs
//   - kurbo (Rust): src/stroke.rs
//
// See GPU-STK-001 in docs/dev/kanban/ for the full task specification.
package stroke
