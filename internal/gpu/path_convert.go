// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/gpu/tilecompute"
	"github.com/gogpu/gg/internal/stroke"
)

// flushCubics flattens accumulated cubic beziers into line segments and appends them.
func flushCubics(lines []tilecompute.LineSoup, cubics []tilecompute.CubicBezier) []tilecompute.LineSoup {
	if len(cubics) > 0 {
		lines = append(lines, tilecompute.FlattenFill(cubics)...)
	}
	return lines
}

// convertPathToPathDef converts a gg.Path with paint info to a tilecompute.PathDef.
// It iterates path elements, flattens curves using Euler spiral subdivision,
// and extracts color and fill rule from the paint.
//
// Lines are produced directly. Quadratic curves are elevated to cubic, and
// cubics are flattened via tilecompute.FlattenFill (Euler spiral adaptive
// subdivision — same algorithm used by golden tests).
func convertPathToPathDef(path *gg.Path, paint *gg.Paint) tilecompute.PathDef {
	if path == nil || path.Elements() == nil {
		return tilecompute.PathDef{}
	}

	elements := path.Elements()
	if len(elements) == 0 {
		return tilecompute.PathDef{}
	}

	var lines []tilecompute.LineSoup
	var cubics []tilecompute.CubicBezier

	var current [2]float32
	var subpathStart [2]float32
	hasMoveTo := false

	for _, elem := range elements {
		switch e := elem.(type) {
		case gg.MoveTo:
			lines = flushCubics(lines, cubics)
			cubics = cubics[:0]
			current = [2]float32{float32(e.Point.X), float32(e.Point.Y)}
			subpathStart = current
			hasMoveTo = true

		case gg.LineTo:
			if !hasMoveTo {
				continue
			}
			pt := [2]float32{float32(e.Point.X), float32(e.Point.Y)}
			if pt != current {
				lines = append(lines, tilecompute.LineSoup{P0: current, P1: pt})
			}
			current = pt

		case gg.QuadTo:
			if !hasMoveTo {
				continue
			}
			ctrl := [2]float32{float32(e.Control.X), float32(e.Control.Y)}
			end := [2]float32{float32(e.Point.X), float32(e.Point.Y)}
			cubics = append(cubics, tilecompute.CubicBezier{
				P0: current,
				P1: [2]float32{current[0] + 2.0/3.0*(ctrl[0]-current[0]), current[1] + 2.0/3.0*(ctrl[1]-current[1])},
				P2: [2]float32{end[0] + 2.0/3.0*(ctrl[0]-end[0]), end[1] + 2.0/3.0*(ctrl[1]-end[1])},
				P3: end,
			})
			current = end

		case gg.CubicTo:
			if !hasMoveTo {
				continue
			}
			cubics = append(cubics, tilecompute.CubicBezier{
				P0: current,
				P1: [2]float32{float32(e.Control1.X), float32(e.Control1.Y)},
				P2: [2]float32{float32(e.Control2.X), float32(e.Control2.Y)},
				P3: [2]float32{float32(e.Point.X), float32(e.Point.Y)},
			})
			current = [2]float32{float32(e.Point.X), float32(e.Point.Y)}

		case gg.Close:
			lines = flushCubics(lines, cubics)
			cubics = cubics[:0]
			if hasMoveTo && current != subpathStart {
				lines = append(lines, tilecompute.LineSoup{P0: current, P1: subpathStart})
			}
			current = subpathStart
		}
	}

	lines = flushCubics(lines, cubics)

	// Extract color from paint.
	color := extractColorU8(paint)

	// Extract fill rule.
	fillRule := tilecompute.FillRuleNonZero
	if paint != nil && paint.FillRule == gg.FillRuleEvenOdd {
		fillRule = tilecompute.FillRuleEvenOdd
	}

	return tilecompute.PathDef{
		Lines:    lines,
		Color:    color,
		FillRule: fillRule,
	}
}

// extractColorU8 extracts a [4]uint8 RGBA color from a Paint.
// The result is in straight (non-premultiplied) alpha, matching PathDef.Color.
func extractColorU8(paint *gg.Paint) [4]uint8 {
	if paint == nil {
		return [4]uint8{0, 0, 0, 255}
	}

	c := getColorFromPaint(paint)
	return [4]uint8{
		clampU8(c.R),
		clampU8(c.G),
		clampU8(c.B),
		clampU8(c.A),
	}
}

// clampU8 converts a float64 in [0,1] to uint8 in [0,255] with rounding.
func clampU8(v float64) uint8 {
	x := v*255.0 + 0.5
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return uint8(x)
}

// convertPathToStrokeElements converts gg.Path elements to stroke package elements.
func convertPathToStrokeElements(ggElems []gg.PathElement) []stroke.PathElement {
	strokeElems := make([]stroke.PathElement, 0, len(ggElems))
	for _, e := range ggElems {
		switch el := e.(type) {
		case gg.MoveTo:
			strokeElems = append(strokeElems, stroke.MoveTo{Point: stroke.Point{X: el.Point.X, Y: el.Point.Y}})
		case gg.LineTo:
			strokeElems = append(strokeElems, stroke.LineTo{Point: stroke.Point{X: el.Point.X, Y: el.Point.Y}})
		case gg.QuadTo:
			strokeElems = append(strokeElems, stroke.QuadTo{
				Control: stroke.Point{X: el.Control.X, Y: el.Control.Y},
				Point:   stroke.Point{X: el.Point.X, Y: el.Point.Y},
			})
		case gg.CubicTo:
			strokeElems = append(strokeElems, stroke.CubicTo{
				Control1: stroke.Point{X: el.Control1.X, Y: el.Control1.Y},
				Control2: stroke.Point{X: el.Control2.X, Y: el.Control2.Y},
				Point:    stroke.Point{X: el.Point.X, Y: el.Point.Y},
			})
		case gg.Close:
			strokeElems = append(strokeElems, stroke.Close{})
		}
	}
	return strokeElems
}

// convertShapeToPathDef converts a detected shape (circle, rect, rrect, ellipse)
// to a tilecompute.PathDef by building a gg.Path and then converting it.
func convertShapeToPathDef(shape gg.DetectedShape, paint *gg.Paint) tilecompute.PathDef {
	path := gg.NewPath()

	switch shape.Kind {
	case gg.ShapeCircle:
		path.Circle(shape.CenterX, shape.CenterY, shape.RadiusX)
	case gg.ShapeEllipse:
		path.Ellipse(shape.CenterX, shape.CenterY, shape.RadiusX, shape.RadiusY)
	case gg.ShapeRect:
		x := shape.CenterX - shape.Width/2
		y := shape.CenterY - shape.Height/2
		path.Rectangle(x, y, shape.Width, shape.Height)
	case gg.ShapeRRect:
		x := shape.CenterX - shape.Width/2
		y := shape.CenterY - shape.Height/2
		path.RoundedRectangle(x, y, shape.Width, shape.Height, shape.CornerRadius)
	default:
		return tilecompute.PathDef{}
	}

	return convertPathToPathDef(path, paint)
}
