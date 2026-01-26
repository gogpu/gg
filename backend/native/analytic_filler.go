// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"math"
)

// AnalyticFiller computes per-pixel coverage using exact geometric calculations.
//
// Unlike supersampling approaches that sample multiple points per pixel,
// analytic AA computes the exact area of the shape within each pixel using
// trapezoidal integration. This provides higher quality anti-aliasing with
// no supersampling overhead.
//
// The algorithm is based on vello's CPU fine rasterizer (fine.rs), which
// uses the following approach:
//
//  1. For each edge crossing a pixel row, compute the Y range it covers
//  2. Find the X intersections at the top and bottom of the pixel
//  3. Compute the trapezoidal area within the pixel bounds
//  4. Accumulate coverage based on winding direction
//
// Usage:
//
//	filler := NewAnalyticFiller(width, height)
//	filler.Fill(edgeBuilder, FillRuleNonZero, func(y int, runs *AlphaRuns) {
//	    // Blend alpha runs to the destination row
//	})
type AnalyticFiller struct {
	width, height int

	// aet is the Active Edge Table for scanline processing.
	aet *CurveAwareAET

	// alphaRuns stores RLE-encoded coverage for the current scanline.
	alphaRuns *AlphaRuns

	// coverage is the per-pixel coverage buffer for the current scanline.
	// Values are in [0, 1] range representing the fraction of pixel covered.
	coverage []float32

	// winding accumulates winding numbers for coverage calculation.
	winding []float32

	// edgeIdx tracks which edges we've processed from the EdgeBuilder.
	edgeIdx int
}

// NewAnalyticFiller creates a new analytic filler for the given dimensions.
func NewAnalyticFiller(width, height int) *AnalyticFiller {
	return &AnalyticFiller{
		width:     width,
		height:    height,
		aet:       NewCurveAwareAET(),
		alphaRuns: NewAlphaRuns(width),
		coverage:  make([]float32, width),
		winding:   make([]float32, width),
	}
}

// Reset clears the filler state for reuse.
func (af *AnalyticFiller) Reset() {
	af.aet.Reset()
	af.alphaRuns.Reset()
	af.edgeIdx = 0
}

// Fill renders a path using analytic coverage calculation.
//
// Parameters:
//   - eb: EdgeBuilder containing the path edges
//   - fillRule: NonZero or EvenOdd fill rule
//   - callback: called for each scanline with the alpha runs
//
// The callback receives the Y coordinate and AlphaRuns for that scanline.
// The caller is responsible for blending the runs to the destination.
func (af *AnalyticFiller) Fill(
	eb *EdgeBuilder,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	if eb.IsEmpty() {
		return
	}

	bounds := eb.Bounds()

	// Get AA scaling factor from edge builder.
	// Edge coordinates are in sub-pixel space: pixel * (1 << aaShift)
	aaShift := eb.AAShift()
	//nolint:gosec // G115: aaShift is bounded by MaxCoeffShift (6), safe conversion
	aaScale := int32(1) << uint(aaShift)

	// Compute scanline range in pixel coordinates
	yMin := int(math.Floor(float64(bounds.MinY)))
	yMax := int(math.Ceil(float64(bounds.MaxY)))

	if yMin < 0 {
		yMin = 0
	}
	if yMax > af.height {
		yMax = af.height
	}

	// Reset state
	af.aet.Reset()
	af.edgeIdx = 0

	// Collect all edges sorted by top Y (edges are sorted in sub-pixel space)
	allEdges := make([]CurveEdgeVariant, 0, eb.EdgeCount())
	for edge := range eb.AllEdges() {
		allEdges = append(allEdges, edge)
	}

	// Process each scanline in pixel space
	for y := yMin; y < yMax; y++ {
		af.processScanlineWithScale(y, aaScale, allEdges, fillRule, callback)
	}
}

// processScanlineWithScale processes a single pixel scanline, accounting for AA scaling.
//
// Edge coordinates are in sub-pixel space (multiplied by aaScale).
// The callback receives pixel coordinates.
//
// Parameters:
//   - y: pixel Y coordinate
//   - aaScale: sub-pixel scale factor (1 << aaShift)
//   - allEdges: all edges sorted by TopY (in sub-pixel space)
//   - fillRule: fill rule to apply
//   - callback: receives pixel Y and alpha runs
func (af *AnalyticFiller) processScanlineWithScale(
	y int,
	aaScale int32,
	allEdges []CurveEdgeVariant,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	// Clear coverage buffer
	for i := range af.coverage {
		af.coverage[i] = 0
	}
	for i := range af.winding {
		af.winding[i] = 0
	}

	// Convert pixel Y to sub-pixel space for edge comparisons
	//nolint:gosec // y is bounded by height which fits in int32
	ySubpixel := int32(y) * aaScale
	ySubpixelNext := ySubpixel + aaScale

	// Remove edges that have ended (edges whose BottomY <= current sub-pixel Y)
	af.aet.RemoveExpiredSubpixel(ySubpixel)

	// Add new edges that start at or before this scanline
	// Edge TopY is in sub-pixel coordinates, compare with sub-pixel Y
	for af.edgeIdx < len(allEdges) {
		edge := allEdges[af.edgeIdx]

		// Use TopY() which returns the curve's overall top Y (not current segment)
		topY := edge.TopY()

		// Edges are sorted by TopY, stop when we hit edges starting below this pixel
		if topY >= ySubpixelNext {
			break
		}

		// Add edge to AET
		af.aet.Insert(edge)
		af.edgeIdx++
	}

	// NOTE: We don't call StepCurves() here anymore.
	// Curve segments are stepped on-demand inside accumulateCoverageSubpixel
	// when segments end within the current scanline.

	// Sort edges by X for scanline processing
	af.aet.SortByX()

	// Process each edge, accumulating coverage
	// Pass sub-pixel Y range for accurate coverage calculation
	af.aet.ForEach(func(edge *CurveEdgeVariant) bool {
		af.accumulateCoverageSubpixel(edge, ySubpixel, aaScale, fillRule)
		return true
	})

	// Apply fill rule and convert to alpha
	af.applyFillRule(fillRule)

	// Convert coverage to alpha runs
	af.coverageToRuns()

	// Advance edges for next scanline
	af.aet.AdvanceX()

	// Callback with the alpha runs (in pixel coordinates)
	callback(y, af.alphaRuns)
}

// accumulateCoverageSubpixel computes coverage with sub-pixel edge coordinates.
//
// This version handles edges that use sub-pixel Y coordinates (multiplied by aaScale).
// The X coordinates are also scaled by the AA factor.
//
// IMPORTANT: This function steps through curve segments as needed when a segment
// ends within the current scanline. This ensures full coverage across the scanline.
//
// Parameters:
//   - edge: the edge to process
//   - ySubpixel: current scanline in sub-pixel coordinates
//   - aaScale: sub-pixel scale factor (1 << aaShift)
//   - fillRule: fill rule (unused, for interface compatibility)
func (af *AnalyticFiller) accumulateCoverageSubpixel(
	edge *CurveEdgeVariant,
	ySubpixel int32,
	aaScale int32,
	_ FillRule,
) {
	aaScaleF := float32(aaScale)
	ySubpixelEnd := ySubpixel + aaScale

	// Pixel Y range for this scanline
	yPixel := float32(ySubpixel) / aaScaleF
	yPixelEnd := yPixel + 1.0

	// Process all segments that intersect this scanline
	// Segments can end mid-scanline, so we may need to step through multiple
	for {
		line := edge.AsLine()
		if line == nil {
			return
		}

		// Check if current segment intersects the scanline
		segmentFirstY := line.FirstY
		segmentLastY := line.LastY + 1 // Exclusive end

		// Skip if segment is entirely after this scanline
		if segmentFirstY >= ySubpixelEnd {
			return
		}

		// Skip if segment is entirely before this scanline
		if segmentLastY <= ySubpixel {
			// Try to step to next segment
			if !af.stepCurveSegment(edge) {
				return
			}
			continue
		}

		// Segment intersects scanline - compute coverage
		af.computeSegmentCoverage(line, ySubpixel, ySubpixelEnd, yPixel, yPixelEnd, aaScaleF)

		// If segment ends within this scanline, step to next segment
		if segmentLastY < ySubpixelEnd {
			if !af.stepCurveSegment(edge) {
				return // No more segments
			}
			// Continue to process next segment for remaining coverage
			continue
		}

		// Segment extends past this scanline, we're done
		return
	}
}

// stepCurveSegment advances a curve edge to its next segment.
// Returns true if a new segment was produced.
func (af *AnalyticFiller) stepCurveSegment(edge *CurveEdgeVariant) bool {
	switch edge.Type {
	case EdgeTypeQuadratic:
		if edge.Quadratic.CurveCount() > 0 {
			return edge.Quadratic.Update()
		}
	case EdgeTypeCubic:
		// Cubic uses negative count, increments toward 0
		if edge.Cubic.CurveCount() < 0 {
			return edge.Cubic.Update()
		}
	}
	return false
}

// computeSegmentCoverage computes coverage for a single line segment.
func (af *AnalyticFiller) computeSegmentCoverage(
	line *LineEdge,
	_, _ int32, // ySubpixel, ySubpixelEnd - reserved for future precision improvements
	yPixel, yPixelEnd, aaScaleF float32,
) {
	// Convert fixed-point coordinates to float
	// Line.X is in FDot16 (16.16 fixed-point) and scaled by aaScale
	// Line.DX is the slope (dimensionless ratio), NOT scaled
	x := FDot16ToFloat32(line.X) / aaScaleF // Convert X to pixel space
	dx := FDot16ToFloat32(line.DX)          // Slope is dimensionless, don't divide!

	// Edge's Y range is in sub-pixel coordinates, convert to pixel
	firstY := float32(line.FirstY) / aaScaleF
	lastY := float32(line.LastY+1) / aaScaleF // LastY is inclusive

	// Clamp to scanline's Y range
	yTop := yPixel
	yBot := yPixelEnd
	if yTop < firstY {
		yTop = firstY
	}
	if yBot > lastY {
		yBot = lastY
	}

	// Skip if segment doesn't intersect this scanline (shouldn't happen here)
	dy := yBot - yTop
	if dy <= 0 {
		return
	}

	// Winding direction
	sign := float32(line.Winding)

	// Compute X coordinates at yTop and yBot
	// In sub-pixel space: x_sub = x0_sub + DX * (y_sub - y0_sub)
	// In pixel space: x_pix = x_sub/scale = x0_pix + DX * (y_pix - y0_pix)
	// So DX remains unchanged when converting to pixel coordinates
	tTop := yTop - firstY // Pixel delta
	tBot := yBot - firstY
	xTop := x + dx*tTop
	xBot := x + dx*tBot

	// Ensure xMin <= xMax for the algorithm
	xMin := xTop
	xMax := xBot
	if xMin > xMax {
		xMin, xMax = xMax, xMin
	}

	// Process each pixel that this edge crosses
	pixelMin := int(math.Floor(float64(xMin)))
	pixelMax := int(math.Ceil(float64(xMax)))

	// Clamp to valid range
	if pixelMin < 0 {
		pixelMin = 0
	}
	if pixelMax > af.width {
		pixelMax = af.width
	}

	// For each pixel, compute trapezoidal coverage
	for px := pixelMin; px < pixelMax; px++ {
		coverage := af.computeTrapezoidalCoverage(
			xTop, yTop,
			xBot, yBot,
			float32(px), yPixel,
		)

		// Accumulate coverage with winding direction
		if px < len(af.winding) {
			af.winding[px] += sign * coverage * dy
		}
	}

	// Handle backdrop contribution (y_edge in vello)
	// This accounts for edges that pass through pixels to the left
	xInt := int(math.Floor(float64(xMin)))
	if xInt >= 0 && xInt < af.width {
		// Backdrop: add sign * dy for all pixels to the right
		for px := xInt; px < af.width; px++ {
			af.winding[px] += sign * dy
		}
	}
}

// computeTrapezoidalCoverage computes the exact coverage of an edge segment
// within a single pixel using trapezoidal integration.
//
// This implements the core algorithm from vello's fine.rs:
//
//	a = (b + 0.5 * (d*d - c*c) - xmin) / (xmax - xmin)
//
// Where:
//   - xmin, xmax are the X extents of the edge within the pixel's Y range
//   - b, c, d are derived from clamping to pixel X bounds
func (af *AnalyticFiller) computeTrapezoidalCoverage(
	x0, y0, x1, y1 float32, // Edge segment endpoints
	pixelX, pixelY float32, // Pixel coordinates
) float32 {
	// Pixel bounds
	pxMin := pixelX
	pxMax := pixelX + 1.0
	pyMin := pixelY
	pyMax := pixelY + 1.0

	// Clip edge to pixel Y bounds
	if y0 > y1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}

	// Y clipping
	if y1 <= pyMin || y0 >= pyMax {
		return 0 // Edge doesn't intersect pixel Y range
	}

	// Compute t values for intersection with pixel Y bounds
	dy := y1 - y0
	if dy == 0 {
		return 0 // Horizontal edge
	}

	dyRecip := 1.0 / dy
	t0 := clamp32((pyMin-y0)*dyRecip, 0, 1)
	t1 := clamp32((pyMax-y0)*dyRecip, 0, 1)

	// Compute X coordinates at clipped Y bounds
	dx := x1 - x0
	xTop := x0 + t0*dx
	xBot := x0 + t1*dx

	// Ensure xTop <= xBot
	xmin := xTop
	xmax := xBot
	if xmin > xmax {
		xmin, xmax = xmax, xmin
	}

	// Skip if edge is entirely outside pixel X range
	if xmax <= pxMin || xmin >= pxMax {
		return 0
	}

	// Trapezoidal area calculation (from vello)
	// This computes the area of the trapezoid formed by the edge
	// within the pixel bounds.
	b := min32f(xmax, pxMax)
	c := max32f(min32f(xTop, xBot), pxMin)
	c = min32f(c, pxMax)
	d := max32f(min32f(xTop, xBot), pxMin)

	// Guard against division by zero
	xRange := xmax - xmin
	if xRange < 1e-6 {
		// Nearly vertical edge - use simple area
		return (t1 - t0) * clamp32(1.0-(xmin-pxMin), 0, 1)
	}

	// Vello's trapezoidal formula
	// a = (b + 0.5 * (d*d - c*c) - xmin) / (xmax - xmin)
	cNorm := (c - pxMin) // Normalize to [0, 1]
	dNorm := (d - pxMin)
	bNorm := (b - pxMin)
	area := (bNorm + 0.5*(dNorm*dNorm-cNorm*cNorm)/1.0 - (xmin - pxMin)) / xRange

	// Scale by Y coverage
	return clamp32(area*(t1-t0), 0, 1)
}

// applyFillRule converts accumulated winding values to coverage.
func (af *AnalyticFiller) applyFillRule(fillRule FillRule) {
	switch fillRule {
	case FillRuleNonZero:
		// Non-zero: coverage = clamp(abs(winding), 0, 1)
		for i, w := range af.winding {
			if w < 0 {
				w = -w
			}
			af.coverage[i] = clamp32(w, 0, 1)
		}

	case FillRuleEvenOdd:
		// Even-odd: coverage based on fractional part of winding
		for i, w := range af.winding {
			// Map winding to [0, 2] cycle, then to coverage
			w = float32(math.Abs(float64(w)))
			w = float32(math.Mod(float64(w), 2.0))
			if w > 1.0 {
				w = 2.0 - w
			}
			af.coverage[i] = w
		}
	}
}

// coverageToRuns converts the coverage buffer to AlphaRuns.
func (af *AnalyticFiller) coverageToRuns() {
	af.alphaRuns.Reset()

	// Find runs of similar coverage
	var currentAlpha uint8
	runStart := 0

	for i := 0; i < af.width; i++ {
		alpha := uint8(clamp32(af.coverage[i], 0, 1) * 255.0)

		if i == 0 {
			currentAlpha = alpha
			runStart = 0
			continue
		}

		// If alpha changed significantly, emit the run
		if alpha != currentAlpha {
			if currentAlpha > 0 {
				runLen := i - runStart
				af.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
			}
			currentAlpha = alpha
			runStart = i
		}
	}

	// Emit final run
	if currentAlpha > 0 {
		runLen := af.width - runStart
		af.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
	}
}

// Width returns the filler width.
func (af *AnalyticFiller) Width() int {
	return af.width
}

// Height returns the filler height.
func (af *AnalyticFiller) Height() int {
	return af.height
}

// Coverage returns the raw coverage buffer for the last processed scanline.
// Values are in [0, 1] range. The buffer is reused between scanlines.
func (af *AnalyticFiller) Coverage() []float32 {
	return af.coverage
}

// AlphaRuns returns the alpha runs for the last processed scanline.
func (af *AnalyticFiller) AlphaRuns() *AlphaRuns {
	return af.alphaRuns
}

// Helper functions

// clamp32 clamps a float32 value to [min, max].
func clamp32(v, minV, maxV float32) float32 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// min32f returns the minimum of two float32 values.
func min32f(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// max32f returns the maximum of two float32 values.
func max32f(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// FillPath is a convenience function that creates a filler and fills a path.
// For repeated fills, create a filler once and reuse it.
func FillPath(
	eb *EdgeBuilder,
	width, height int,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	filler := NewAnalyticFiller(width, height)
	filler.Fill(eb, fillRule, callback)
}

// FillToBuffer fills a path and writes coverage to a buffer.
// The buffer must have width * height elements.
// Coverage values are written as 0-255 alpha values.
func FillToBuffer(
	eb *EdgeBuilder,
	width, height int,
	fillRule FillRule,
	buffer []uint8,
) {
	if len(buffer) < width*height {
		return
	}

	filler := NewAnalyticFiller(width, height)
	filler.Fill(eb, fillRule, func(y int, runs *AlphaRuns) {
		// Copy coverage to buffer row
		offset := y * width
		if offset+width > len(buffer) {
			return
		}

		// Clear row first
		row := buffer[offset : offset+width]
		for i := range row {
			row[i] = 0
		}

		// Copy from runs
		runs.CopyTo(row)
	})
}
