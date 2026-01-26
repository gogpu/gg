package main

import (
	"fmt"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/backend/native"
	"github.com/gogpu/gg/scene"
)

func TestAnalyticFillerDebug(t *testing.T) {
	// Match the GREEN filled shape from bezier_test main.go (Section 3)
	// dc.MoveTo(100, 300)
	// dc.QuadraticTo(200, 220, 300, 300)
	// dc.QuadraticTo(200, 380, 100, 300)
	path := gg.NewPath()
	path.MoveTo(100, 300)
	path.QuadraticTo(200, 220, 300, 300)
	path.QuadraticTo(200, 380, 100, 300)
	path.Close()

	fmt.Printf("Path elements count: %d\n", len(path.Elements()))
	for i, elem := range path.Elements() {
		fmt.Printf("  [%d] %T: %+v\n", i, elem, elem)
	}

	// Convert to scene.Path
	scenePath := scene.NewPath()
	for _, elem := range path.Elements() {
		switch e := elem.(type) {
		case gg.MoveTo:
			scenePath.MoveTo(float32(e.Point.X), float32(e.Point.Y))
		case gg.LineTo:
			scenePath.LineTo(float32(e.Point.X), float32(e.Point.Y))
		case gg.QuadTo:
			scenePath.QuadTo(float32(e.Control.X), float32(e.Control.Y),
				float32(e.Point.X), float32(e.Point.Y))
		case gg.CubicTo:
			scenePath.CubicTo(float32(e.Control1.X), float32(e.Control1.Y),
				float32(e.Control2.X), float32(e.Control2.Y),
				float32(e.Point.X), float32(e.Point.Y))
		case gg.Close:
			scenePath.Close()
		}
	}

	fmt.Printf("\nScene path verbs: %d, points: %d\n", len(scenePath.Verbs()), len(scenePath.Points()))

	// Build edges
	eb := native.NewEdgeBuilder(2)
	eb.BuildFromScenePath(scenePath, scene.IdentityAffine())

	fmt.Printf("\nEdgeBuilder stats:\n")
	fmt.Printf("  IsEmpty: %v\n", eb.IsEmpty())
	fmt.Printf("  EdgeCount: %d\n", eb.EdgeCount())
	fmt.Printf("  LineEdgeCount: %d\n", eb.LineEdgeCount())
	fmt.Printf("  QuadraticEdgeCount: %d\n", eb.QuadraticEdgeCount())
	fmt.Printf("  CubicEdgeCount: %d\n", eb.CubicEdgeCount())
	fmt.Printf("  Bounds: %+v\n", eb.Bounds())

	// Print edge details
	aaScale := int32(1) << eb.AAShift()
	fmt.Printf("\nEdge details (aaScale=%d):\n", aaScale)
	idx := 0
	for edge := range eb.AllEdges() {
		topY := edge.TopY()
		bottomY := edge.BottomY()
		line := edge.AsLine()
		if line != nil {
			// Convert to pixel coords for readability
			aaScaleF := float32(aaScale)
			fmt.Printf("  [%d] Type=%d, TopY=%d (%.1f px), BottomY=%d (%.1f px), Segment: FirstY=%d, LastY=%d, Winding=%d\n",
				idx, edge.Type,
				topY, float32(topY)/aaScaleF,
				bottomY, float32(bottomY)/aaScaleF,
				line.FirstY, line.LastY, line.Winding)
		} else {
			fmt.Printf("  [%d] Type=%d, AsLine()=nil\n", idx, edge.Type)
		}
		idx++
	}

	// Fill - match bezier_test dimensions (800x600)
	filler := native.NewAnalyticFiller(800, 600)
	scanlineCount := 0
	pixelCount := 0

	filler.Fill(eb, native.FillRuleNonZero, func(y int, runs *native.AlphaRuns) {
		scanlineCount++
		linePixels := 0
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				pixelCount++
				linePixels++
				if scanlineCount <= 5 || (scanlineCount%20 == 0) {
					fmt.Printf("  y=%d, x=%d, alpha=%d\n", y, x, alpha)
				}
			}
		}
		if scanlineCount <= 5 || (scanlineCount%20 == 0) {
			fmt.Printf("  >> y=%d total pixels: %d\n", y, linePixels)
		}
	})

	fmt.Printf("\nFill results:\n")
	fmt.Printf("  Scanlines processed: %d\n", scanlineCount)
	fmt.Printf("  Pixels with alpha > 0: %d\n", pixelCount)
}
