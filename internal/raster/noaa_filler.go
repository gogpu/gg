// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"math"
)

// NoAAFiller performs non-anti-aliased (binary) scanline rasterization.
//
// This is a dedicated integer scanline walker that produces solid horizontal
// spans with full coverage (255) or zero coverage. No sub-pixel arithmetic,
// no fractional coverage, no AlphaRuns.
//
// The algorithm follows Skia's SkScan::FillPath (SkScan_Path.cpp) and
// tiny-skia's scan::path — a completely separate code path from the AA
// rasterizer, not a flag on the AnalyticFiller. Every enterprise 2D engine
// (Skia, Cairo, tiny-skia) uses separate non-AA code paths because:
//   - Integer arithmetic only (no fixed-point fractional bits for coverage)
//   - Solid span output (no alpha blending at edges)
//   - ~2-3x faster than analytic AA
//
// The filler reuses EdgeBuilder infrastructure and CurveAwareAET for edge
// management, but iterates at integer Y steps and rounds edge X positions
// to integer pixels via FixedRoundToInt.
//
// Usage:
//
//	filler := NewNoAAFiller(width, height)
//	filler.Fill(edgeBuilder, FillRuleNonZero, func(y, left, width int) {
//	    // Blit solid span: all pixels in [left, left+width) get coverage=255
//	})
type NoAAFiller struct {
	width, height int
}

// NewNoAAFiller creates a new non-AA filler for the given dimensions.
func NewNoAAFiller(width, height int) *NoAAFiller {
	return &NoAAFiller{
		width:  width,
		height: height,
	}
}

// Fill performs non-anti-aliased rasterization using integer scanline walking.
//
// The callback receives solid horizontal spans: (y, left, spanWidth) where
// every pixel in [left, left+spanWidth) should be blitted with full coverage.
// This matches Skia's blitter->blitH(left, curr_y, width) pattern.
//
// Parameters:
//   - eb: EdgeBuilder containing the path edges (aaShift=0 for non-AA)
//   - fillRule: NonZero or EvenOdd fill rule
//   - blitH: callback for each solid span (y, leftX, spanWidth)
func (nf *NoAAFiller) Fill(
	eb *EdgeBuilder,
	fillRule FillRule,
	blitH func(y, left, width int),
) {
	if eb.IsEmpty() {
		return
	}

	bounds := eb.Bounds()
	aaShift := eb.AAShift()

	yMin := int(math.Floor(float64(bounds.MinY)))
	yMax := int(math.Ceil(float64(bounds.MaxY)))

	if yMin < 0 {
		yMin = 0
	}
	if yMax > nf.height {
		yMax = nf.height
	}
	if yMin >= yMax {
		return
	}

	//nolint:gosec // G115: aaShift is bounded by MaxCoeffShift (6), safe conversion
	aaScale := int32(1) << uint(aaShift)

	sortedBuf := eb.sortedEdgesSlice()
	if len(sortedBuf) == 0 {
		return
	}

	// Build edge variant buffer (same pattern as AnalyticFiller).
	edgeBuf := make([]CurveEdgeVariant, len(sortedBuf))
	for i := range sortedBuf {
		edgeBuf[i] = sortedBuf[i].variant
	}

	// Winding mask: for EvenOdd, test bit 0 only; for NonZero, test != 0.
	// Skia uses windingMask = isEvenOdd ? 1 : -1, then (w & mask) == 0 means outside.
	windingMask := int32(-1) // NonZero
	if fillRule == FillRuleEvenOdd {
		windingMask = 1
	}

	aet := NewCurveAwareAET()
	edgeIdx := 0

	for y := yMin; y < yMax; y++ {
		//nolint:gosec // y is bounded by height which fits in int32
		ySubpixel := int32(y) * aaScale
		ySubpixelNext := ySubpixel + aaScale

		aet.RemoveExpiredSubpixel(ySubpixel)

		// Insert new edges whose top Y falls within this scanline.
		for edgeIdx < len(edgeBuf) {
			topY := edgeBuf[edgeIdx].TopY()
			if topY >= ySubpixelNext {
				break
			}
			aet.Insert(edgeBuf[edgeIdx])
			edgeIdx++
		}

		if aet.Len() == 0 {
			continue
		}

		aet.SortByX()

		// Walk edges left-to-right, tracking winding number.
		// When we enter a filled region (winding becomes non-zero for NonZero,
		// or odd for EvenOdd), record left X. When we exit, emit solid span.
		var w int32
		var left int

		for i := range aet.Len() {
			edge := aet.EdgeAt(i)
			line := edge.AsLine()
			if line == nil {
				continue
			}

			// Round edge X to nearest integer pixel (Skia: SkFixedRoundToInt).
			x := fixedRoundToInt(line.X)

			// Clamp to canvas bounds.
			if x < 0 {
				x = 0
			}
			if x > nf.width {
				x = nf.width
			}

			if (w & windingMask) == 0 {
				// Starting a filled interval.
				left = x
			}

			w += int32(line.Winding)

			if (w & windingMask) == 0 {
				// Finished a filled interval.
				spanWidth := x - left
				if spanWidth > 0 {
					blitH(y, left, spanWidth)
				}
			}
		}

		// Handle case where right edge was clipped away (winding still non-zero).
		if (w & windingMask) != 0 {
			spanWidth := nf.width - left
			if spanWidth > 0 {
				blitH(y, left, spanWidth)
			}
		}

		// Advance all edges: step X by slope, then transition curve segments.
		aet.AdvanceX()
		aet.StepCurves()
	}
}

// fixedRoundToInt rounds a FDot16 (16.16 fixed-point) value to the nearest
// integer. Matches Skia's SkFixedRoundToInt: (x + 0x8000) >> 16.
func fixedRoundToInt(x FDot16) int {
	return int((x + FDot16Half) >> FDot16Shift)
}
