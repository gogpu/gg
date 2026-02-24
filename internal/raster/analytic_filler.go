// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

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

	// NOTE: Do NOT call aet.AdvanceX() here.
	// computeSegmentCoverage computes line positions analytically via:
	//   lineTopX = x + dx*(lineTopY - firstY)
	// where x = FDot16ToFloat32(line.X)/aaScale is the position at firstY.
	// AdvanceX() modifies line.X by DX (one sub-pixel step), but the outer
	// loop processes one PIXEL scanline (= aaScale sub-pixel steps). This
	// causes line.X to drift, while firstY stays fixed, leading to progressive
	// position error: slope * N / aaScale per pixel scanline.
	// Since the line equation already computes correct positions from the
	// original line.X and firstY, advancing line.X is both unnecessary and harmful.

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
//
// This implements the analytic AA algorithm from fine.go, adapted for scanline processing.
// The key insight is that coverage accumulates LEFT-TO-RIGHT within each pixel row:
//
//  1. For each pixel, compute the trapezoidal area (partial coverage)
//  2. Add the accumulated coverage from all pixels to the LEFT (backdrop)
//  3. Update the accumulator for the NEXT pixel
//
// The algorithm processes pixels left-to-right, accumulating winding contributions.
// Pixels LEFT of the edge get acc=0, pixels RIGHT get accumulated winding.
//
// X-bounds clipping: edges extending beyond the canvas (common during window resize
// when shapes are positioned relative to canvas center) are handled by:
//   - Edges entirely off-screen right: skipped (no visible contribution)
//   - Edges entirely off-screen left: full winding applied to all visible pixels
//   - Edges partially off-screen left: winding pre-accumulated for the off-screen portion
//
// This matches the algorithm in fine.go which processes all pixels in each tile row.
func (af *AnalyticFiller) computeSegmentCoverage( //nolint:funlen // performance-critical rasterization loop, splitting hurts cache locality
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

	// Skip if segment doesn't intersect this scanline
	lineDY := yBot - yTop
	if lineDY <= 0 {
		return
	}

	// Winding direction
	sign := float32(line.Winding)

	// Compute line parameters relative to this scanline
	// Line equation: lineX(y) = x + dx * (y - firstY)
	lineTopY := yTop
	lineBottomY := yBot
	lineTopX := x + dx*(lineTopY-firstY)
	lineBottomX := x + dx*(lineBottomY-firstY)

	// Calculate slopes for pixel-row intersection
	lineDX := lineBottomX - lineTopX

	var ySlope float32
	if lineDX == 0 {
		// Vertical line
		if lineDY > 0 {
			ySlope = 1e10
		} else {
			ySlope = -1e10
		}
	} else {
		ySlope = lineDY / lineDX // dy/dx
	}
	xSlope := 1.0 / ySlope // dx/dy

	// --- X-bounds clipping ---
	// Edges can extend beyond the canvas in X (e.g., large circles partially off-screen).
	// Y-bounds clipping is handled above. Here we handle X-bounds:
	// 1. Edges entirely off-screen right: no contribution to any visible pixel.
	// 2. Edges entirely off-screen left: full winding applied to all visible pixels.
	// 3. Edges partially off-screen left: pre-accumulate winding for the off-screen portion.
	widthF := float32(af.width)

	// Determine X range of the edge on this scanline
	minLineX := lineTopX
	maxLineX := lineBottomX
	if minLineX > maxLineX {
		minLineX, maxLineX = maxLineX, minLineX
	}

	// Case 1: Edge entirely off-screen right — no visible contribution
	if minLineX >= widthF {
		return
	}

	// Case 2: Edge entirely off-screen left — full winding to all visible pixels
	if maxLineX < 0 {
		fullAcc := lineDY * sign
		for xIdx := 0; xIdx < af.width; xIdx++ {
			af.winding[xIdx] += fullAcc
		}
		return
	}

	// Pre-accumulate winding for the off-screen-left portion of the edge.
	// When part of the edge is at X < 0, pixels at X >= 0 are to the RIGHT
	// of that portion and must receive its winding contribution.
	acc := offscreenLeftWinding(lineTopX, lineBottomX, lineTopY, lineBottomY, ySlope, yTop, yBot, sign)

	// Compute the pixel range that needs detailed per-pixel processing.
	// Pixels outside this range get only the accumulated winding (acc).
	xStart := int(minLineX)
	if xStart < 0 {
		xStart = 0
	}
	xEnd := int(maxLineX) + 2 // +2 for partial pixel coverage at the boundary
	if xEnd > af.width {
		xEnd = af.width
	}

	// Apply pre-accumulated winding to pixels before the edge's range
	for xIdx := 0; xIdx < xStart; xIdx++ {
		af.winding[xIdx] += acc
	}

	// Process pixels in the edge's range with detailed coverage computation
	for xIdx := xStart; xIdx < xEnd; xIdx++ {
		pxLeftX := float32(xIdx)
		pxRightX := pxLeftX + 1.0

		// Calculate Y coordinates where line intersects pixel left and right edges
		// Using: y = lineTopY + (x - lineTopX) * ySlope
		linePxLeftY := lineTopY + (pxLeftX-lineTopX)*ySlope
		linePxRightY := lineTopY + (pxRightX-lineTopX)*ySlope

		// Clamp to scanline Y bounds and line Y bounds
		linePxLeftY = clamp32(linePxLeftY, yTop, yBot)
		linePxRightY = clamp32(linePxRightY, yTop, yBot)

		// Calculate X coordinates at the clamped Y values
		// Using: x = lineTopX + (y - lineTopY) * xSlope
		linePxLeftYX := lineTopX + (linePxLeftY-lineTopY)*xSlope
		linePxRightYX := lineTopX + (linePxRightY-lineTopY)*xSlope

		// Height of line segment within this pixel's row
		pixelH := linePxRightY - linePxLeftY
		if pixelH < 0 {
			pixelH = -pixelH
		}

		// Trapezoidal area: the area enclosed between the line and pixel's right edge
		// This is 0.5 * height * (width1 + width2) where widths are distances from
		// line to right edge at top and bottom of segment within pixel
		//
		// IMPORTANT: Do NOT clamp area! The algorithm relies on area values outside [0,1]
		// for correct anti-aliasing. The final winding->coverage conversion handles clamping.
		area := 0.5 * pixelH * (2*pxRightX - linePxRightYX - linePxLeftYX)

		// Add area contribution plus accumulated winding from left
		// This is the core of the analytic AA algorithm from fine.go
		af.winding[xIdx] += (area*sign + acc)

		// Update accumulator for NEXT pixel
		acc += pixelH * sign
	}

	// Apply remaining accumulated winding to pixels after the edge's range
	for xIdx := xEnd; xIdx < af.width; xIdx++ {
		af.winding[xIdx] += acc
	}
}

// offscreenLeftWinding computes the winding contribution from the portion of
// an edge that extends past the left canvas boundary (X < 0). Pixels at X >= 0
// are to the right of that portion and must receive its winding.
func offscreenLeftWinding(lineTopX, lineBottomX, lineTopY, lineBottomY, ySlope, yTop, yBot, sign float32) float32 {
	if lineTopX >= 0 && lineBottomX >= 0 {
		return 0
	}
	// Find Y where the edge crosses X=0.
	y0 := clamp32(lineTopY-lineTopX*ySlope, yTop, yBot)
	var h float32
	if lineTopX < 0 {
		// Top of edge is off-screen left; off-screen portion: lineTopY → y0.
		h = y0 - lineTopY
	} else {
		// Bottom of edge is off-screen left; off-screen portion: y0 → lineBottomY.
		h = lineBottomY - y0
	}
	if h < 0 {
		h = -h
	}
	return h * sign
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
