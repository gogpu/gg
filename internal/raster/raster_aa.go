// Package raster provides scanline rasterization for 2D paths.
// This file implements anti-aliased filling using 4x supersampling.
package raster

import "math"

// PathEdge represents an edge from external path processing.
// This is used to receive edges from path.EdgeIter which correctly
// handles subpath boundaries.
type PathEdge struct {
	P0, P1 Point
}

// FillAAFromEdges rasterizes edges with anti-aliasing onto a pixmap.
// This function accepts pre-computed edges from path.EdgeIter, which
// correctly handles subpath boundaries (no connecting edges between subpaths).
// Uses 4x supersampling for smooth edges.
func (r *Rasterizer) FillAAFromEdges(pixmap AAPixmap, pathEdges []PathEdge, fillRule FillRule, color RGBA) {
	if len(pathEdges) < 2 {
		return
	}

	// Build edge list from path edges
	edges := make([]Edge, 0, len(pathEdges))
	for _, pe := range pathEdges {
		// Skip horizontal edges
		if math.Abs(pe.P1.Y-pe.P0.Y) < 0.001 {
			continue
		}
		edges = append(edges, NewEdge(pe.P0, pe.P1))
	}

	r.fillAAWithEdges(pixmap, edges, fillRule, color)
}

// FillAA rasterizes a filled path with anti-aliasing onto a pixmap.
// Uses 4x supersampling for smooth edges.
//
// Deprecated: This function creates edges from consecutive point pairs,
// which incorrectly connects separate subpaths. Use FillAAFromEdges with
// path.EdgeIter for correct subpath handling.
func (r *Rasterizer) FillAA(pixmap AAPixmap, points []Point, fillRule FillRule, color RGBA) {
	if len(points) < 2 {
		return
	}

	// Build edge list from consecutive point pairs
	edges := make([]Edge, 0, len(points))
	for i := range points {
		if i+1 >= len(points) {
			break
		}
		p0 := points[i]
		p1 := points[i+1]

		// Skip horizontal edges
		if math.Abs(p1.Y-p0.Y) < 0.001 {
			continue
		}

		edges = append(edges, NewEdge(p0, p1))
	}

	r.fillAAWithEdges(pixmap, edges, fillRule, color)
}

// fillAAWithEdges is the internal implementation shared by FillAA and FillAAFromEdges.
func (r *Rasterizer) fillAAWithEdges(pixmap AAPixmap, edges []Edge, fillRule FillRule, color RGBA) {
	if len(edges) == 0 {
		return
	}

	// Find y bounds
	yMin := math.MaxFloat64
	yMax := -math.MaxFloat64
	xMin := math.MaxFloat64
	xMax := -math.MaxFloat64
	for _, e := range edges {
		yMin = math.Min(yMin, e.y0)
		yMax = math.Max(yMax, e.y1)
		xMin = math.Min(xMin, math.Min(e.x0, e.x1))
		xMax = math.Max(xMax, math.Max(e.x0, e.x1))
	}

	yMinInt := int(math.Floor(yMin))
	yMaxInt := int(math.Ceil(yMax))
	xMinInt := int(math.Floor(xMin))
	xMaxInt := int(math.Ceil(xMax))

	// Clamp to pixmap bounds
	if yMinInt < 0 {
		yMinInt = 0
	}
	if yMaxInt > pixmap.Height() {
		yMaxInt = pixmap.Height()
	}
	if xMinInt < 0 {
		xMinInt = 0
	}
	if xMaxInt > pixmap.Width() {
		xMaxInt = pixmap.Width()
	}

	// Create super blitter
	sb := NewSuperBlitter(
		pixmap, color,
		xMinInt, yMinInt, xMaxInt, yMaxInt, // bounds
		0, 0, pixmap.Width(), pixmap.Height(), // clip
	)
	if sb == nil {
		return // clipped out
	}

	// Scanline rasterization in supersampled coordinates
	superYMin := yMinInt << SupersampleShift
	superYMax := yMaxInt << SupersampleShift

	for superY := superYMin; superY < superYMax; superY++ {
		scanY := (float64(superY) + 0.5) / float64(SupersampleScale)
		//nolint:gosec // superY is bounded by pixmap dimensions * 4, fits in uint32
		r.scanlineAA(sb, edges, scanY, uint32(superY), fillRule)
	}

	// Final flush
	sb.Flush()
}

// scanlineAA processes a single supersampled scanline.
func (r *Rasterizer) scanlineAA(sb *SuperBlitter, edges []Edge, y float64, superY uint32, fillRule FillRule) {
	r.aet.Clear()

	// Add edges that intersect this scanline with correct x position
	for _, edge := range edges {
		if edge.y0 <= y && y < edge.y1 {
			r.aet.AddAtY(edge, y)
		}
	}

	if len(r.aet.Edges()) == 0 {
		return
	}

	// Sort edges by x coordinate
	r.aet.Sort()

	// Fill spans based on fill rule
	activeEdges := r.aet.Edges()
	if fillRule == FillRuleNonZero {
		r.fillNonZeroAA(sb, activeEdges, y, superY)
	} else {
		r.fillEvenOddAA(sb, activeEdges, y, superY)
	}
}

// fillNonZeroAA fills using the non-zero winding rule with supersampled coordinates.
func (r *Rasterizer) fillNonZeroAA(sb *SuperBlitter, edges []ActiveEdge, _ float64, superY uint32) {
	winding := 0
	var x1 float64

	for i := 0; i < len(edges); i++ {
		edge := edges[i]

		if winding == 0 {
			x1 = edge.x
		}

		winding += edge.dir

		if winding == 0 {
			x2 := edge.x
			r.blitSpanAA(sb, x1, x2, superY)
		}
	}
}

// fillEvenOddAA fills using the even-odd rule with supersampled coordinates.
func (r *Rasterizer) fillEvenOddAA(sb *SuperBlitter, edges []ActiveEdge, _ float64, superY uint32) {
	for i := 0; i+1 < len(edges); i += 2 {
		x1 := edges[i].x
		x2 := edges[i+1].x
		r.blitSpanAA(sb, x1, x2, superY)
	}
}

// blitSpanAA sends a span to the super blitter in supersampled coordinates.
func (r *Rasterizer) blitSpanAA(sb *SuperBlitter, x1, x2 float64, superY uint32) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}

	// Clamp to pixmap bounds (in pixel space) and convert back
	if x1 < 0 {
		x1 = 0
	}
	pixelWidth := float64(r.width)
	if x2 > pixelWidth {
		x2 = pixelWidth
	}

	// Convert to supersampled coordinates
	superX1 := int(x1 * float64(SupersampleScale))
	superX2 := int(x2 * float64(SupersampleScale))

	if superX1 >= superX2 {
		return
	}

	//nolint:gosec // superX1 is bounded by pixmap width * 4, fits in uint32
	sb.BlitH(uint32(superX1), superY, superX2-superX1)
}
