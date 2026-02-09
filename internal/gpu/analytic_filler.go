// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package gpu

import (
	"github.com/gogpu/gg/internal/raster"
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
//	filler.Fill(edgeBuilder, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
//	    // Blend alpha runs to the destination row
//	})
type AnalyticFiller struct {
	width, height int

	// aet is the Active Edge Table for scanline processing.
	aet *raster.CurveAwareAET

	// alphaRuns stores RLE-encoded coverage for the current scanline.
	alphaRuns *raster.AlphaRuns

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
		aet:       raster.NewCurveAwareAET(),
		alphaRuns: raster.NewAlphaRuns(width),
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
// The callback receives the Y coordinate and raster.AlphaRuns for that scanline.
// The caller is responsible for blending the runs to the destination.
func (af *AnalyticFiller) Fill(
	eb *raster.EdgeBuilder,
	fillRule raster.FillRule,
	callback func(y int, runs *raster.AlphaRuns),
) {
	if eb.IsEmpty() {
		return
	}

	bounds := eb.Bounds()

	// Get AA scaling factor from edge builder.
	// Edge coordinates are in sub-pixel space: pixel * (1 << aaShift)
	aaShift := eb.AAShift()
	//nolint:gosec // G115: aaShift is bounded by raster.MaxCoeffShift (6), safe conversion
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
	allEdges := make([]raster.CurveEdgeVariant, 0, eb.EdgeCount())
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
	allEdges []raster.CurveEdgeVariant,
	fillRule raster.FillRule,
	callback func(y int, runs *raster.AlphaRuns),
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
	af.aet.ForEach(func(edge *raster.CurveEdgeVariant) bool {
		af.accumulateCoverageSubpixel(edge, ySubpixel, aaScale, fillRule)
		return true
	})

	// Apply fill rule and convert to alpha
	af.applyFillRule(fillRule)

	// Convert coverage to alpha runs
	af.coverageToRuns()

	// NOTE: AdvanceX removed - X is now computed directly in computeSegmentCoverage
	// based on firstY and current yPixel, avoiding accumulation errors

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
	edge *raster.CurveEdgeVariant,
	ySubpixel int32,
	aaScale int32,
	_ raster.FillRule,
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
func (af *AnalyticFiller) stepCurveSegment(edge *raster.CurveEdgeVariant) bool {
	switch edge.Type {
	case raster.EdgeTypeQuadratic:
		if edge.Quadratic.CurveCount() > 0 {
			return edge.Quadratic.Update()
		}
	case raster.EdgeTypeCubic:
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
// CRITICAL: We must process ALL pixels from 0 to width, not just starting from
// where the line enters. This ensures correct backdrop accumulation - pixels
// to the LEFT of the line get acc=0, pixels to the RIGHT get the accumulated
// winding from the line crossing.
//
// This matches the algorithm in fine.go which processes all pixels in each tile row.
func (af *AnalyticFiller) computeSegmentCoverage(
	line *raster.LineEdge,
	_, _ int32, // ySubpixel, ySubpixelEnd - reserved for future precision improvements
	yPixel, yPixelEnd, aaScaleF float32,
) {
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

	// Compute X at firstY from the edge's stored X value
	// line.X is the X coordinate at Y=firstY (in raster.FDot16, scaled by aaScale)
	xAtFirstY := raster.FDot16ToFloat32(line.X) / aaScaleF
	// dx is slope (dimensionless): dX_subpixel / dY_subpixel = dX_pixel / dY_pixel
	dx := raster.FDot16ToFloat32(line.DX)

	// Compute line X at any Y: x(y) = xAtFirstY + dx * (y - firstY)
	lineTopY := yTop
	lineBottomY := yBot
	lineTopX := xAtFirstY + dx*(lineTopY-firstY)
	lineBottomX := xAtFirstY + dx*(lineBottomY-firstY)

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

	// Accumulate winding contribution from left edge
	// CRITICAL: Start from 0, not from the line's X position!
	// This ensures correct backdrop accumulation across the entire scanline.
	acc := float32(0)

	// Process each pixel column from left to right (0 to width)
	// This matches fine.go which processes all pixels in the tile row
	for xIdx := 0; xIdx < af.width; xIdx++ {
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
}

// applyFillRule converts accumulated winding values to coverage.
func (af *AnalyticFiller) applyFillRule(fillRule raster.FillRule) {
	switch fillRule {
	case raster.FillRuleNonZero:
		// Non-zero: coverage = clamp(abs(winding), 0, 1)
		for i, w := range af.winding {
			if w < 0 {
				w = -w
			}
			af.coverage[i] = clamp32(w, 0, 1)
		}

	case raster.FillRuleEvenOdd:
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

// coverageToRuns converts the coverage buffer to raster.AlphaRuns.
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
func (af *AnalyticFiller) AlphaRuns() *raster.AlphaRuns {
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
	eb *raster.EdgeBuilder,
	width, height int,
	fillRule raster.FillRule,
	callback func(y int, runs *raster.AlphaRuns),
) {
	filler := NewAnalyticFiller(width, height)
	filler.Fill(eb, fillRule, callback)
}

// FillToBuffer fills a path and writes coverage to a buffer.
// The buffer must have width * height elements.
// Coverage values are written as 0-255 alpha values.
func FillToBuffer(
	eb *raster.EdgeBuilder,
	width, height int,
	fillRule raster.FillRule,
	buffer []uint8,
) {
	if len(buffer) < width*height {
		return
	}

	filler := NewAnalyticFiller(width, height)
	filler.Fill(eb, fillRule, func(y int, runs *raster.AlphaRuns) {
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
