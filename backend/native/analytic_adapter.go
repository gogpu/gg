// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/core"
)

// AnalyticFillerAdapter adapts the core.AnalyticFiller to gg.AnalyticFillerInterface.
// This allows the SoftwareRenderer to use analytic anti-aliasing without
// creating an import cycle.
//
// Usage:
//
//	renderer := gg.NewSoftwareRenderer(800, 600)
//	adapter := native.NewAnalyticFillerAdapter(800, 600)
//	renderer.SetAnalyticFiller(adapter)
type AnalyticFillerAdapter struct {
	edgeBuilder    *core.EdgeBuilder
	analyticFiller *core.AnalyticFiller
	width, height  int
}

// NewAnalyticFillerAdapter creates a new adapter for analytic anti-aliasing.
// Parameters:
//   - width, height: the dimensions of the rendering surface
//
// The adapter is reusable for multiple Fill calls.
func NewAnalyticFillerAdapter(width, height int) *AnalyticFillerAdapter {
	eb := core.NewEdgeBuilder(2) // 4x AA quality
	// Enable curve flattening for reliable analytic AA rendering.
	// Curves are converted to line segments at build time, which the
	// AnalyticFiller handles correctly.
	eb.SetFlattenCurves(true)
	return &AnalyticFillerAdapter{
		edgeBuilder:    eb,
		analyticFiller: core.NewAnalyticFiller(width, height),
		width:          width,
		height:         height,
	}
}

// Fill renders the path using analytic coverage calculation.
// This implements gg.AnalyticFillerInterface.
func (a *AnalyticFillerAdapter) Fill(
	path *gg.Path,
	fillRule gg.FillRule,
	callback func(y int, iter func(yield func(x int, alpha uint8) bool)),
) {
	// Convert gg.Path to core.PathLike
	pathAdapter := convertGGPathToCorePath(path)

	// Build edges
	a.edgeBuilder.Reset()
	a.edgeBuilder.BuildFromPath(pathAdapter, core.IdentityTransform{})

	// If no edges, nothing to fill
	if a.edgeBuilder.IsEmpty() {
		return
	}

	// Convert fill rule
	coreFillRule := core.FillRuleNonZero
	if fillRule == gg.FillRuleEvenOdd {
		coreFillRule = core.FillRuleEvenOdd
	}

	// Fill with analytic coverage
	a.analyticFiller.Fill(a.edgeBuilder, coreFillRule, func(y int, runs *core.AlphaRuns) {
		// Create iterator wrapper for the callback
		callback(y, func(yield func(x int, alpha uint8) bool) {
			for x, alpha := range runs.Iter() {
				if !yield(x, alpha) {
					return
				}
			}
		})
	})
}

// Reset clears the adapter state for reuse.
// This implements gg.AnalyticFillerInterface.
func (a *AnalyticFillerAdapter) Reset() {
	a.edgeBuilder.Reset()
	a.analyticFiller.Reset()
}

// convertGGPathToCorePath converts a gg.Path to core.PathLike.
func convertGGPathToCorePath(p *gg.Path) core.PathLike {
	// Build verb and point arrays in core format
	var verbs []core.PathVerb
	var points []float32

	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case gg.MoveTo:
			verbs = append(verbs, core.VerbMoveTo)
			points = append(points, float32(e.Point.X), float32(e.Point.Y))
		case gg.LineTo:
			verbs = append(verbs, core.VerbLineTo)
			points = append(points, float32(e.Point.X), float32(e.Point.Y))
		case gg.QuadTo:
			verbs = append(verbs, core.VerbQuadTo)
			points = append(points,
				float32(e.Control.X), float32(e.Control.Y),
				float32(e.Point.X), float32(e.Point.Y),
			)
		case gg.CubicTo:
			verbs = append(verbs, core.VerbCubicTo)
			points = append(points,
				float32(e.Control1.X), float32(e.Control1.Y),
				float32(e.Control2.X), float32(e.Control2.Y),
				float32(e.Point.X), float32(e.Point.Y),
			)
		case gg.Close:
			verbs = append(verbs, core.VerbClose)
		}
	}

	return core.NewScenePathAdapter(len(verbs) == 0, verbs, points)
}

// Verify that AnalyticFillerAdapter implements gg.AnalyticFillerInterface.
var _ gg.AnalyticFillerInterface = (*AnalyticFillerAdapter)(nil)
