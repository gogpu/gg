// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// AnalyticFillerAdapter adapts the native AnalyticFiller to gg.AnalyticFillerInterface.
// This allows the SoftwareRenderer to use analytic anti-aliasing without
// creating an import cycle.
//
// Usage:
//
//	renderer := gg.NewSoftwareRenderer(800, 600)
//	adapter := native.NewAnalyticFillerAdapter(800, 600)
//	renderer.SetAnalyticFiller(adapter)
type AnalyticFillerAdapter struct {
	edgeBuilder    *EdgeBuilder
	analyticFiller *AnalyticFiller
	width, height  int
}

// NewAnalyticFillerAdapter creates a new adapter for analytic anti-aliasing.
// Parameters:
//   - width, height: the dimensions of the rendering surface
//
// The adapter is reusable for multiple Fill calls.
func NewAnalyticFillerAdapter(width, height int) *AnalyticFillerAdapter {
	return &AnalyticFillerAdapter{
		edgeBuilder:    NewEdgeBuilder(2), // 4x AA quality
		analyticFiller: NewAnalyticFiller(width, height),
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
	// Convert gg.Path to scene.Path
	scenePath := convertGGPathToScenePath(path)

	// Build edges
	a.edgeBuilder.Reset()
	a.edgeBuilder.BuildFromScenePath(scenePath, scene.IdentityAffine())

	// If no edges, nothing to fill
	if a.edgeBuilder.IsEmpty() {
		return
	}

	// Convert fill rule
	nativeFillRule := FillRuleNonZero
	if fillRule == gg.FillRuleEvenOdd {
		nativeFillRule = FillRuleEvenOdd
	}

	// Fill with analytic coverage
	a.analyticFiller.Fill(a.edgeBuilder, nativeFillRule, func(y int, runs *AlphaRuns) {
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

// convertGGPathToScenePath converts a gg.Path to a scene.Path.
func convertGGPathToScenePath(p *gg.Path) *scene.Path {
	result := scene.NewPath()

	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case gg.MoveTo:
			result.MoveTo(float32(e.Point.X), float32(e.Point.Y))
		case gg.LineTo:
			result.LineTo(float32(e.Point.X), float32(e.Point.Y))
		case gg.QuadTo:
			result.QuadTo(
				float32(e.Control.X), float32(e.Control.Y),
				float32(e.Point.X), float32(e.Point.Y),
			)
		case gg.CubicTo:
			result.CubicTo(
				float32(e.Control1.X), float32(e.Control1.Y),
				float32(e.Control2.X), float32(e.Control2.Y),
				float32(e.Point.X), float32(e.Point.Y),
			)
		case gg.Close:
			result.Close()
		}
	}

	return result
}

// Verify that AnalyticFillerAdapter implements gg.AnalyticFillerInterface.
var _ gg.AnalyticFillerInterface = (*AnalyticFillerAdapter)(nil)
