// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"math"
)

// Skia AAA (Analytic Anti-Aliasing) trapezoid decomposition algorithm.
//
// Ported from Skia's SkScan_AAAPath.cpp — the sole AA algorithm in Chrome,
// Android, and Flutter since 2016.
//
// The key difference from the previous Vello-derived accumulator approach:
// coverage is computed per-trapezoid between paired edges (left/right), NOT
// by accumulating winding across the scanline. This eliminates BUG-RAST-011
// (unbounded float32 drift for near-horizontal edges).
//
// Algorithm overview:
//  1. Walk edges left-to-right, tracking winding number
//  2. When entering a filled span: record left edge
//  3. When exiting: compute trapezoid between left and right edges
//  4. Each trapezoid decomposed into per-pixel alpha contributions
//  5. Alpha values are ADDITIVE (supports concave paths)

// skFixed constants matching Skia's SkFixed (16.16 fixed-point).
const (
	skFixed1    int32 = 1 << 16
	skFixedHalf int32 = 1 << 15
)

// AnalyticFiller computes per-pixel coverage using Skia AAA trapezoid decomposition.
//
// For each scanline strip, edges are walked left-to-right in pairs (left edge,
// right edge) determined by winding number. Between each pair, coverage is
// computed geometrically from the trapezoid formed by the two edges and the
// strip's top/bottom scan lines.
//
// Adaptive Y stepping (Phase 2): each pixel row is split into sub-strips at
// edge endpoints within that row. An edge starting at y=10.3 produces strips
// [10.0, 10.3) with fullAlpha~77 and [10.3, 11.0) with fullAlpha~179.
// This matches Skia AAA's fractional Y iteration (SkScan_AAAPath.cpp:1455).
//
// Usage:
//
//	filler := NewAnalyticFiller(width, height)
//	filler.Fill(edgeBuilder, FillRuleNonZero, func(y int, runs *AlphaRuns) {
//	    // Blend alpha runs to the destination row
//	})
type AnalyticFiller struct {
	width, height int

	aet       *CurveAwareAET
	alphaRuns *AlphaRuns

	// coverage is the per-pixel additive alpha buffer for the current scanline.
	// Values accumulate in 0-255 range. Multiple trapezoids can ADD to the same pixel.
	// Sub-strips within a pixel row accumulate additively into this buffer.
	coverage []uint8

	edgeIdx int
	edgeBuf []CurveEdgeVariant

	// resolvedEdges is a reusable buffer for resolved edge states per sub-strip.
	resolvedEdges []edgeLineState

	// stripYBuf is a reusable buffer for sub-strip Y boundaries in SkFixed (16.16).
	// All Y tracking uses integer SkFixed to match Skia AAA — no float32 intermediary.
	stripYBuf []int32

	// WindingCallback, if set, is called after edge accumulation with (y, winding[])
	// before applyFillRule. Used by winding residual tests to verify contour closure.
	// For compatibility, we synthesize a float32 winding buffer from coverage.
	WindingCallback func(y int, winding []float32)

	// windingCompat is a float32 buffer for WindingCallback compatibility.
	windingCompat []float32
}

// NewAnalyticFiller creates a new analytic filler for the given dimensions.
func NewAnalyticFiller(width, height int) *AnalyticFiller {
	return &AnalyticFiller{
		width:     width,
		height:    height,
		aet:       NewCurveAwareAET(),
		alphaRuns: NewAlphaRuns(width),
		coverage:  make([]uint8, width),
	}
}

// Reset clears the filler state for reuse.
func (af *AnalyticFiller) Reset() {
	af.aet.Reset()
	af.alphaRuns.Reset()
	af.edgeIdx = 0
}

// Fill renders a path using Skia AAA trapezoid decomposition.
//
// Parameters:
//   - eb: EdgeBuilder containing the path edges
//   - fillRule: NonZero or EvenOdd fill rule
//   - callback: called for each scanline with the alpha runs
func (af *AnalyticFiller) Fill(
	eb *EdgeBuilder,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	if eb.IsEmpty() {
		return
	}

	bounds := eb.Bounds()
	aaShift := eb.AAShift()
	//nolint:gosec // G115: aaShift is bounded by MaxCoeffShift (6), safe conversion
	aaScale := int32(1) << uint(aaShift)

	yMin := int(math.Floor(float64(bounds.MinY)))
	yMax := int(math.Ceil(float64(bounds.MaxY)))

	if yMin < 0 {
		yMin = 0
	}
	if yMax > af.height {
		yMax = af.height
	}

	af.aet.Reset()
	af.edgeIdx = 0

	sortedBuf := eb.sortedEdgesSlice()
	if cap(af.edgeBuf) < len(sortedBuf) {
		af.edgeBuf = make([]CurveEdgeVariant, len(sortedBuf))
	} else {
		af.edgeBuf = af.edgeBuf[:len(sortedBuf)]
	}
	for i := range sortedBuf {
		af.edgeBuf[i] = sortedBuf[i].variant
	}

	for y := yMin; y < yMax; y++ {
		af.processScanlineAAA(y, aaScale, af.edgeBuf, fillRule, callback)
	}
}

// processScanlineAAA processes a single pixel scanline using Skia AAA with
// adaptive Y stepping.
//
// Skia AAA (SkScan_AAAPath.cpp:1451-1601) does NOT iterate integer Y rows.
// Instead, it tracks fractional Y positions where edge endpoints fall and
// processes sub-strips between those boundaries. For an edge starting at
// y=10.3, the pixel row y=10 is split into [10.0, 10.3) with fullAlpha~77
// and [10.3, 11.0) with fullAlpha~179. This gives correct fractional alpha
// at shape boundaries.
//
// Implementation:
//  1. Clear per-pixel alpha buffer
//  2. Insert new edges, collect fractional Y boundaries within [y, y+1)
//  3. For each sub-strip [stripTop, stripBot):
//     a. Resolve edge positions at the sub-strip midpoint Y
//     b. Sort by X, paired-edge walk (winding + fill rule)
//     c. Blit trapezoid rows with fullAlpha proportional to strip height
//  4. Sub-strip alphas accumulate additively into coverage buffer
//  5. Convert coverage to AlphaRuns and invoke callback
func (af *AnalyticFiller) processScanlineAAA(
	y int,
	aaScale int32,
	allEdges []CurveEdgeVariant,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	for i := range af.coverage {
		af.coverage[i] = 0
	}

	//nolint:gosec // y is bounded by height which fits in int32
	ySubpixel := int32(y) * aaScale
	ySubpixelNext := ySubpixel + aaScale

	af.aet.RemoveExpiredSubpixel(ySubpixel)

	for af.edgeIdx < len(allEdges) {
		edge := allEdges[af.edgeIdx]
		topY := edge.TopY()
		if topY >= ySubpixelNext {
			break
		}
		af.aet.Insert(edge)
		af.edgeIdx++
	}

	// All Y tracking in SkFixed (16.16) — matches Skia's aaa_walk_edges exactly.
	// No float32 intermediary for Y positions.
	yFixed := intToSkFixed(int32(y))        // pixel row start in SkFixed
	yFixedEnd := intToSkFixed(int32(y) + 1) // pixel row end in SkFixed

	// Collect fractional Y boundaries from active edges within this pixel row.
	// Returns sorted SkFixed values defining sub-strip boundaries.
	stripYs := af.collectStripBoundariesFixed(yFixed, yFixedEnd, aaScale)

	// Process each sub-strip. Coverage accumulates additively.
	for si := 0; si < len(stripYs)-1; si++ {
		stripTop := stripYs[si]
		stripBot := stripYs[si+1]
		if stripBot <= stripTop {
			continue
		}

		af.processSubStripFixed(aaScale, stripTop, stripBot, fillRule)
	}

	// WindingCallback compatibility: synthesize winding from coverage
	if af.WindingCallback != nil {
		if len(af.windingCompat) < af.width {
			af.windingCompat = make([]float32, af.width)
		}
		for i := 0; i < af.width; i++ {
			af.windingCompat[i] = float32(af.coverage[i]) / 255.0
		}
		af.WindingCallback(y, af.windingCompat)
	}

	af.coverageToRunsFromBuffer()
	callback(y, af.alphaRuns)
}

// collectStripBoundariesFixed gathers unique SkFixed Y values from active edge
// endpoints that fall within [yTopFixed, yBotFixed), plus the row boundaries.
// Returns sorted, deduplicated SkFixed values defining sub-strip boundaries.
//
// All computation is in SkFixed (16.16) integer arithmetic — no float32.
// This matches Skia's update_next_next_y / nextY tracking.
//
// Parameters:
//   - yTopFixed, yBotFixed: pixel row boundaries in SkFixed
//   - aaScale: AA subdivision factor (1, 2, or 4)
func (af *AnalyticFiller) collectStripBoundariesFixed(yTopFixed, yBotFixed, aaScale int32) []int32 {
	af.stripYBuf = af.stripYBuf[:0]
	af.stripYBuf = append(af.stripYBuf, yTopFixed, yBotFixed)

	n := af.aet.Len()
	for i := 0; i < n; i++ {
		edge := af.aet.EdgeAt(i)

		line := edge.AsLine()
		if line == nil {
			continue
		}

		// Convert edge Y endpoints to pixel-space SkFixed.
		// UpperY/LowerY are already pixel-space SkFixed (from snapY in NewLineEdge).
		// FirstY/LastY are integer sub-pixel rows — convert to pixel-space SkFixed.
		var segTopFixed, segBotFixed int32
		if line.UpperY != 0 || line.LowerY != 0 {
			segTopFixed = line.UpperY
			segBotFixed = line.LowerY
		} else {
			segTopFixed = int32(int64(line.FirstY) * int64(skFixed1) / int64(aaScale))
			segBotFixed = int32(int64(line.LastY+1) * int64(skFixed1) / int64(aaScale))
		}

		// Also consider the edge's overall bounds (for curve edges).
		edgeTopFixed := int32(int64(edge.TopY()) * int64(skFixed1) / int64(aaScale))
		edgeBotFixed := int32(int64(edge.BottomY()) * int64(skFixed1) / int64(aaScale))

		for _, ey := range [4]int32{segTopFixed, segBotFixed, edgeTopFixed, edgeBotFixed} {
			if ey > yTopFixed && ey < yBotFixed {
				af.stripYBuf = append(af.stripYBuf, ey)
			}
		}
	}

	// Also check edges not yet in the AET but starting within this pixel row.
	for idx := af.edgeIdx; idx < len(af.edgeBuf); idx++ {
		edge := &af.edgeBuf[idx]
		topFixed := int32(int64(edge.TopY()) * int64(skFixed1) / int64(aaScale))
		if topFixed >= yBotFixed {
			break
		}
		if topFixed > yTopFixed {
			af.stripYBuf = append(af.stripYBuf, topFixed)
		}
	}

	sortInt32s(af.stripYBuf)
	af.stripYBuf = deduplicateInt32s(af.stripYBuf)

	return af.stripYBuf
}

// processSubStripFixed resolves edges and blits trapezoids for a single sub-strip
// within a pixel row. Coverage is added to the existing coverage buffer.
// All Y parameters are in SkFixed (16.16) — no float32 conversion.
func (af *AnalyticFiller) processSubStripFixed(
	aaScale int32, stripTopFixed, stripBotFixed int32,
	fillRule FillRule,
) {
	n := af.aet.Len()
	if cap(af.resolvedEdges) < n {
		af.resolvedEdges = make([]edgeLineState, n)
	}
	af.resolvedEdges = af.resolvedEdges[:0]

	// Compute fullAlpha from SkFixed Y difference — Skia's fixed_to_alpha.
	// fullAlpha = get_partial_alpha(0xFF, nextY - y) = SkFixedRoundToInt(255 * (nextY - y))
	yDiff := stripBotFixed - stripTopFixed
	fullAlpha := fixedToAlpha(yDiff)
	if fullAlpha == 0 {
		return
	}

	for i := 0; i < n; i++ {
		edge := af.aet.EdgeAt(i)
		lineState := af.resolveEdgeLineFixed(edge, aaScale, stripTopFixed, stripBotFixed, fullAlpha)
		if lineState.valid {
			af.resolvedEdges = append(af.resolvedEdges, lineState)
		}
	}

	sortEdgesByMidX(af.resolvedEdges)

	// Paired-edge walk: Skia AAA pattern.
	winding := int32(0)
	inInterval := false
	var leftEdgeState edgeLineState

	for i := range af.resolvedEdges {
		lineState := af.resolvedEdges[i]

		winding += int32(lineState.winding)
		prevInInterval := inInterval
		if fillRule == FillRuleEvenOdd {
			inInterval = (winding & 1) != 0
		} else {
			inInterval = winding != 0
		}

		isLeft := inInterval && !prevInInterval
		isRight := !inInterval && prevInInterval

		if isRight {
			af.blitTrapezoidBetweenEdges(leftEdgeState, lineState)
		}
		if isLeft {
			leftEdgeState = lineState
		}
	}

	// NOTE: Skia's aaa_walk_edges fills to rightClip when winding doesn't return
	// to zero ("right-edge culled away"). We omit this for non-inverse fills because
	// uncancelled winding from imprecise edge sorting would fill to the canvas edge,
	// creating visible artifacts (e.g., rotated text with curved glyphs).
	// For properly closed paths, winding always returns to zero.
	_ = inInterval
}

// sortInt32s sorts a slice of int32 in ascending order (insertion sort).
func sortInt32s(s []int32) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}

// deduplicateInt32s removes duplicate values from a sorted int32 slice.
// Values within 128 units of each other (< 1/512 pixel in SkFixed) are considered equal.
func deduplicateInt32s(s []int32) []int32 {
	if len(s) <= 1 {
		return s
	}
	const eps int32 = 128 // ~1/512 pixel in SkFixed (65536 per pixel)
	n := 1
	for i := 1; i < len(s); i++ {
		if s[i]-s[n-1] > eps {
			s[n] = s[i]
			n++
		}
	}
	return s[:n]
}

// sortEdgesByMidX sorts resolved edges by their midpoint X position (SkFixed).
func sortEdgesByMidX(edges []edgeLineState) {
	// Simple insertion sort — edge count per scanline is typically small (<20)
	for i := 1; i < len(edges); i++ {
		key := edges[i]
		// Midpoint in SkFixed — use int64 to avoid overflow in addition.
		keyX := int64(key.topX) + int64(key.botX)
		j := i - 1
		for j >= 0 && int64(edges[j].topX)+int64(edges[j].botX) > keyX {
			edges[j+1] = edges[j]
			j--
		}
		edges[j+1] = key
	}
}

// edgeLineState holds resolved line parameters for one edge.
// All positions are in SkFixed (16.16 fixed-point pixel coordinates) to match Skia's
// integer-only pipeline. No float32 intermediary — avoids round-trip precision loss.
type edgeLineState struct {
	valid     bool
	topX      int32 // X position at top of strip (SkFixed pixel coords)
	botX      int32 // X position at bottom of strip (SkFixed pixel coords)
	dy        int32 // Skia fDY: abs(1/slope) in SkFixed. Used by partialTriangleToAlpha.
	fullAlpha uint8 // strip height as alpha [0, 255], computed from SkFixed Y difference
	winding   int8
}

// resolveEdgeLineFixed resolves an edge to its line parameters for the current
// scanline strip. All computation is in SkFixed (16.16) integer math — no float32.
//
// Matches Skia's goY(): fX = fUpperX + SkFixedMul(fDX, y - fUpperY)
//
// Our edge stores X and DX in sub-pixel FDot16 space (4x pixel for aaShift=2).
// Coordinate conversion:
//   - line.X is sub-pixel FDot16 at line.FirstY. Pixel SkFixed = line.X / aaScale.
//   - line.DX is FDot6Div(dx, dy) = slope ratio, same in pixel and sub-pixel space.
//   - X_at_Y = upperX_pixel + SkFixedMul(DX, Y_pixel - upperY_pixel)
func (af *AnalyticFiller) resolveEdgeLineFixed(
	edge *CurveEdgeVariant,
	aaScale int32, yTopFixed, yBotFixed int32, fullAlpha uint8,
) edgeLineState {
	for {
		line := edge.AsLine()
		if line == nil {
			return edgeLineState{}
		}

		// Determine segment Y range in pixel-space SkFixed.
		var segTopFixed, segBotFixed int32
		hasPrecise := line.UpperY != 0 || line.LowerY != 0
		if hasPrecise {
			segTopFixed = line.UpperY
			segBotFixed = line.LowerY
		} else {
			segTopFixed = int32(int64(line.FirstY) * int64(skFixed1) / int64(aaScale))
			segBotFixed = int32(int64(line.LastY+1) * int64(skFixed1) / int64(aaScale))
		}

		// Quick cull: segment entirely after this strip.
		if segTopFixed >= yBotFixed {
			return edgeLineState{}
		}

		// Quick cull: segment entirely before this strip — step curve.
		if segBotFixed <= yTopFixed {
			if !af.stepCurveSegment(edge) {
				return edgeLineState{}
			}
			continue
		}

		// Clamp strip Y to segment range (all SkFixed).
		clampedTop := yTopFixed
		clampedBot := yBotFixed
		if clampedTop < segTopFixed {
			clampedTop = segTopFixed
		}
		if clampedBot > segBotFixed {
			clampedBot = segBotFixed
		}

		if clampedBot <= clampedTop {
			return edgeLineState{}
		}

		// Recompute fullAlpha if we clamped the strip to the segment range.
		// This happens when the edge starts/ends within the sub-strip.
		edgeAlpha := fullAlpha
		if clampedTop != yTopFixed || clampedBot != yBotFixed {
			edgeAlpha = fixedToAlpha(clampedBot - clampedTop)
			if edgeAlpha == 0 {
				return edgeLineState{}
			}
		}

		topX, botX := computeEdgeX(line, aaScale, hasPrecise, clampedTop, clampedBot)

		// Use pixel-space slope for fDY when available (line edges from NewLineEdge).
		// For curve sub-segments, fall back to the sub-pixel slope.
		slopeForDY := line.DX
		if hasPrecise {
			slopeForDY = line.PixelDX
		}
		fDY := computeEdgeDY(slopeForDY)

		return edgeLineState{
			valid:     true,
			topX:      topX,
			botX:      botX,
			dy:        fDY,
			fullAlpha: edgeAlpha,
			winding:   line.Winding,
		}
	}
}

// computeEdgeX computes X positions at clampedTop and clampedBot for an edge.
// All values are in pixel-space SkFixed (16.16).
//
// For line edges (hasPrecise=true), this uses the pre-computed pixel-space fields
// (UpperX, PixelDX) directly — matching Skia's goY() exactly:
//
//	fX = fUpperX + SkFixedMul(fDX, y - fUpperY)
//
// For curve sub-segments (hasPrecise=false), pixel-space fields are not available,
// so we derive pixel-space X from the sub-pixel FDot16 fields via aaScale division.
func computeEdgeX(line *LineEdge, aaScale int32, hasPrecise bool, clampedTop, clampedBot int32) (topX, botX int32) {
	if hasPrecise {
		// Skia goY() exact path: use pixel-space UpperX + PixelDX * (Y - UpperY).
		// No sub-pixel→pixel division — zero rounding error vs Skia.
		topX = line.UpperX + skFixedMul(line.PixelDX, clampedTop-line.UpperY)
		botX = line.UpperX + skFixedMul(line.PixelDX, clampedBot-line.UpperY)
		return topX, botX
	}

	// Curve sub-segment fallback: derive pixel-space X from sub-pixel fields.
	slope := line.DX
	refXPixel := int32(int64(line.X) / int64(aaScale))
	refYPixel := int32((int64(line.FirstY)*int64(skFixed1) + int64(skFixedHalf)) / int64(aaScale))

	topX = refXPixel + skFixedMul(slope, clampedTop-refYPixel)
	botX = refXPixel + skFixedMul(slope, clampedBot-refYPixel)
	return topX, botX
}

// computeEdgeDY computes Skia's fDY = abs(1/slope) in SkFixed.
// Used by partialTriangleToAlpha for coverage computation.
func computeEdgeDY(slope int32) int32 {
	absSlope := slope
	if absSlope < 0 {
		absSlope = -absSlope
	}
	absSlopeFDot6 := absSlope >> (FDot16Shift - FDot6Shift)
	if absSlopeFDot6 == 0 {
		return 0x7FFFFFFF
	}
	fDY := FDot6Div(FDot6One, absSlopeFDot6)
	if fDY < 0 {
		return 0x7FFFFFFF
	}
	return fDY
}

// blitTrapezoidBetweenEdges computes per-pixel alpha for the trapezoid formed
// between a left edge and a right edge within the current scanline strip.
//
// This is the Go port of Skia's blit_trapezoid_row. The trapezoid is defined by:
//   - Upper-left (ul), upper-right (ur): edge X positions at strip top
//   - Lower-left (ll), lower-right (lr): edge X positions at strip bottom
//   - fullAlpha: strip height as alpha (255 for full-height strip)
func (af *AnalyticFiller) blitTrapezoidBetweenEdges(left, right edgeLineState) {
	if !left.valid || !right.valid {
		return
	}

	ul := left.topX
	ll := left.botX
	ur := right.topX
	lr := right.botX

	// Use the minimum fullAlpha of the two edges. When both edges span the
	// full sub-strip, they have the same fullAlpha. When one edge starts/ends
	// mid-strip (clamped), it has a smaller fullAlpha.
	fullAlpha := left.fullAlpha
	if right.fullAlpha < fullAlpha {
		fullAlpha = right.fullAlpha
	}
	if fullAlpha == 0 {
		return
	}

	lDY := left.dy
	rDY := right.dy

	af.blitTrapezoidRow(ul, ur, ll, lr, lDY, rDY, fullAlpha)
}

// blitTrapezoidRow is the Go port of Skia's blit_trapezoid_row.
//
// The trapezoid is defined by four X coordinates in 16.16 fixed-point:
//
//	ul, ur = upper-left, upper-right (at strip top Y)
//	ll, lr = lower-left, lower-right (at strip bottom Y)
//
// Coverage for each pixel is computed by subtracting excluded triangular
// regions from fullAlpha. No accumulator spans the scanline.
func (af *AnalyticFiller) blitTrapezoidRow(
	ul, ur, ll, lr int32,
	lDY, rDY int32,
	fullAlpha uint8,
) {
	if lDY < 0 {
		lDY = -lDY
	}
	if rDY < 0 {
		rDY = -rDY
	}

	// Edge crossing at top: precision-induced at vertices where edges
	// share the same start point. Clamp to midpoint (degenerate triangle).
	if ul > ur {
		mid := (ul + ur) / 2
		ul = mid
		ur = mid
	}

	// Edge crossing at bottom: precision-induced.
	if ll > lr {
		mid := approximateIntersection(ul, ll, ur, lr)
		ll = mid
		lr = mid
	}

	if ul == ur && ll == lr {
		return // empty trapezoid
	}

	// Normalize: ensure top <= bottom for each edge
	if ul > ll {
		ul, ll = ll, ul
	}
	if ur > lr {
		ur, lr = lr, ur
	}

	// Determine if there's a "join" region — a full-coverage rectangle
	// between the left edge's rightmost X and the right edge's leftmost X.
	joinLeft := skFixedCeilToFixed(ll)
	joinRite := skFixedFloorToFixed(ur)

	if joinLeft > joinRite {
		af.blitAaaTrapezoidRow(ul, ur, ll, lr, lDY, rDY, fullAlpha)
		return
	}

	// Left partial region: ul to joinLeft
	af.blitLeftPartial(ul, ll, joinLeft, lDY, fullAlpha)

	// Full-coverage middle region
	if joinLeft < joinRite {
		startX := skFixedFloorToInt(joinLeft)
		count := skFixedFloorToInt(joinRite - joinLeft)
		for i := int32(0); i < count; i++ {
			af.safeAddAlpha(startX+i, fullAlpha)
		}
	}

	// Right partial region: joinRite to lr
	af.blitRightPartial(ur, lr, joinRite, rDY, fullAlpha)
}

// blitLeftPartial handles the left edge's partial-coverage pixels.
func (af *AnalyticFiller) blitLeftPartial(ul, ll, joinLeft, lDY int32, fullAlpha uint8) {
	if ul >= joinLeft {
		return
	}
	switch skFixedCeilToInt(joinLeft - ul) {
	case 1:
		alpha := trapezoidToAlpha(joinLeft-ul, joinLeft-ll)
		af.safeAddAlpha(skFixedFloorToInt(ul), getPartialAlpha8(alpha, fullAlpha))
	case 2:
		first := joinLeft - skFixed1 - ul
		second := ll - ul - first
		a1 := partialTriangleToAlpha(first, lDY)
		a2 := saturatingSub8(fullAlpha, partialTriangleToAlpha(second, lDY))
		af.safeAddAlpha(skFixedFloorToInt(ul), getPartialAlpha8(a1, fullAlpha))
		af.safeAddAlpha(skFixedFloorToInt(ul)+1, getPartialAlpha8(a2, fullAlpha))
	default:
		af.blitAaaTrapezoidRow(ul, joinLeft, ll, joinLeft, lDY, 0x7FFFFFFF, fullAlpha)
	}
}

// blitRightPartial handles the right edge's partial-coverage pixels.
func (af *AnalyticFiller) blitRightPartial(ur, lr, joinRite, rDY int32, fullAlpha uint8) {
	if lr <= joinRite {
		return
	}
	switch skFixedCeilToInt(lr - joinRite) {
	case 1:
		alpha := trapezoidToAlpha(ur-joinRite, lr-joinRite)
		af.safeAddAlpha(skFixedFloorToInt(joinRite), getPartialAlpha8(alpha, fullAlpha))
	case 2:
		first := joinRite + skFixed1 - ur
		second := lr - ur - first
		a1 := saturatingSub8(fullAlpha, partialTriangleToAlpha(first, rDY))
		a2 := partialTriangleToAlpha(second, rDY)
		af.safeAddAlpha(skFixedFloorToInt(joinRite), getPartialAlpha8(a1, fullAlpha))
		af.safeAddAlpha(skFixedFloorToInt(joinRite)+1, getPartialAlpha8(a2, fullAlpha))
	default:
		af.blitAaaTrapezoidRow(joinRite, ur, joinRite, lr, 0x7FFFFFFF, rDY, fullAlpha)
	}
}

// blitAaaTrapezoidRow handles the general case where left and right edges
// may both have partial coverage across multiple pixels.
//
// Port of Skia's blit_aaa_trapezoid_row.
func (af *AnalyticFiller) blitAaaTrapezoidRow(
	ul, ur, ll, lr int32,
	lDY, rDY int32,
	fullAlpha uint8,
) {
	baseX := skFixedFloorToInt(ul)
	endX := skFixedCeilToInt(lr)
	length := endX - baseX

	if length <= 0 {
		return
	}

	if length == 1 {
		alpha := trapezoidToAlpha(ur-ul, lr-ll)
		af.safeAddAlpha(baseX, getPartialAlpha8(alpha, fullAlpha))
		return
	}

	// Allocate per-pixel alpha array for this span
	alphas := make([]uint8, length)
	for i := range alphas {
		alphas[i] = fullAlpha
	}
	tempAlphas := make([]uint8, length)

	// Subtract the left edge's excluded region (below the left line)
	uL := skFixedFloorToInt(ul)
	lL := skFixedCeilToInt(ll)
	if uL+2 == lL {
		first := intToSkFixed(uL) + skFixed1 - ul
		second := ll - ul - first
		a1 := saturatingSub8(fullAlpha, partialTriangleToAlpha(first, lDY))
		a2 := partialTriangleToAlpha(second, lDY)
		alphas[0] = saturatingSub8(alphas[0], a1)
		alphas[1] = saturatingSub8(alphas[1], a2)
	} else {
		computeAlphaBelowLine(tempAlphas[uL-baseX:], ul-intToSkFixed(uL), ll-intToSkFixed(uL), lDY, fullAlpha)
		for i := uL; i < lL && i-baseX < length; i++ {
			idx := i - baseX
			if idx >= 0 && idx < length {
				alphas[idx] = saturatingSub8(alphas[idx], tempAlphas[idx])
			}
		}
	}

	// Subtract the right edge's excluded region (above the right line)
	uR := skFixedFloorToInt(ur)
	lR := skFixedCeilToInt(lr)
	for i := range tempAlphas {
		tempAlphas[i] = 0
	}
	af.subtractRightExclusion(alphas, tempAlphas, uR, lR, baseX, length, ur, lr, rDY, fullAlpha)

	// Write to coverage buffer
	for i := int32(0); i < length; i++ {
		af.safeAddAlpha(baseX+i, alphas[i])
	}
}

// subtractRightExclusion subtracts the right edge's excluded region from the alpha array.
func (af *AnalyticFiller) subtractRightExclusion(
	alphas, tempAlphas []uint8,
	uR, lR, baseX, length int32,
	ur, lr, rDY int32,
	fullAlpha uint8,
) {
	if uR+2 == lR {
		first := intToSkFixed(uR) + skFixed1 - ur
		second := lr - ur - first
		a1 := partialTriangleToAlpha(first, rDY)
		a2 := saturatingSub8(fullAlpha, partialTriangleToAlpha(second, rDY))
		if idx := length - 2; idx >= 0 {
			alphas[idx] = saturatingSub8(alphas[idx], a1)
		}
		if idx := length - 1; idx >= 0 {
			alphas[idx] = saturatingSub8(alphas[idx], a2)
		}
		return
	}
	computeAlphaAboveLine(tempAlphas[uR-baseX:], ur-intToSkFixed(uR), lr-intToSkFixed(uR), rDY, fullAlpha)
	for i := uR; i < lR && i-baseX < length; i++ {
		idx := i - baseX
		if idx >= 0 && idx < length {
			alphas[idx] = saturatingSub8(alphas[idx], tempAlphas[idx])
		}
	}
}

// safeAddAlpha adds alpha to the coverage buffer with bounds checking and clamping.
func (af *AnalyticFiller) safeAddAlpha(x int32, alpha uint8) {
	if x < 0 || int(x) >= af.width || alpha == 0 {
		return
	}
	sum := uint16(af.coverage[x]) + uint16(alpha)
	if sum > 255 {
		sum = 255
	}
	af.coverage[x] = uint8(sum) //nolint:gosec // clamped to 255
}

// coverageToRunsFromBuffer converts the uint8 coverage buffer to AlphaRuns.
func (af *AnalyticFiller) coverageToRunsFromBuffer() {
	af.alphaRuns.Reset()

	var currentAlpha uint8
	runStart := 0

	for i := 0; i < af.width; i++ {
		alpha := af.coverage[i]

		if i == 0 {
			currentAlpha = alpha
			continue
		}

		if alpha != currentAlpha {
			if currentAlpha > 0 {
				runLen := i - runStart
				af.alphaRuns.AddWithCoverage(runStart, currentAlpha, runLen-1, 0, currentAlpha)
			}
			currentAlpha = alpha
			runStart = i
		}
	}

	if currentAlpha > 0 {
		runLen := af.width - runStart
		af.alphaRuns.AddWithCoverage(runStart, currentAlpha, runLen-1, 0, currentAlpha)
	}
}

// stepCurveSegment advances a curve edge to its next segment.
func (af *AnalyticFiller) stepCurveSegment(edge *CurveEdgeVariant) bool {
	switch edge.Type {
	case EdgeTypeQuadratic:
		if edge.Quadratic.CurveCount() > 0 {
			return edge.Quadratic.Update()
		}
	case EdgeTypeCubic:
		if edge.Cubic.CurveCount() < 0 {
			return edge.Cubic.Update()
		}
	}
	return false
}

// --- Skia AAA coverage helper functions ---

// trapezoidToAlpha returns the alpha of a trapezoid whose height is 1 (full strip).
// The two sides have lengths l1 and l2 in 16.16 fixed-point.
// Port of Skia's trapezoid_to_alpha.
func trapezoidToAlpha(l1, l2 int32) uint8 {
	if l1 < 0 {
		l1 = 0
	}
	if l2 < 0 {
		l2 = 0
	}
	area := (l1 + l2) / 2
	result := area >> 8
	if result > 255 {
		return 255
	}
	if result < 0 {
		return 0
	}
	return uint8(result) //nolint:gosec // clamped above
}

// partialTriangleToAlpha returns the alpha of a right-triangle with legs a and a*b.
// Both a and b are in 16.16 fixed-point, where a <= SK_Fixed1.
// Port of Skia's partial_triangle_to_alpha.
func partialTriangleToAlpha(a, b int32) uint8 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > skFixed1 {
		a = skFixed1
	}
	// Approximation matching Skia: area = (a >> 11) * (a >> 11) * (b >> 11)
	a11 := a >> 11
	b11 := b >> 11
	area := a11 * a11 * b11
	result := (area >> 8) & 0xFF
	if result < 0 {
		return 0
	}
	return uint8(result) //nolint:gosec // masked to 8 bits
}

// getPartialAlpha8 scales an alpha by a fullAlpha factor.
// Port of Skia's get_partial_alpha(SkAlpha, SkAlpha).
func getPartialAlpha8(alpha, fullAlpha uint8) uint8 {
	if fullAlpha == 255 {
		return alpha
	}
	return uint8((uint16(alpha) * uint16(fullAlpha)) >> 8) //nolint:gosec // product fits in uint16
}

// computeAlphaAboveLine computes per-pixel alpha for the region above a line
// within a strip. The line goes from (l, strip_top) to (r, strip_bottom).
// Port of Skia's compute_alpha_above_line.
func computeAlphaAboveLine(alphas []uint8, l, r, dY int32, fullAlpha uint8) {
	if l < 0 {
		l = 0
	}
	if l > r {
		l, r = r, l
	}
	R := skFixedCeilToInt(r)
	if R <= 0 || int(R) > len(alphas) {
		return
	}
	if R == 1 {
		alphas[0] = getPartialAlpha8(uint8(clampAlpha32(((R<<17)-l-r)>>9)), fullAlpha)
		return
	}

	first := skFixed1 - l
	last := r - intToSkFixed(R-1)
	firstH := skFixedMul(first, dY)
	alphas[0] = uint8(clampAlpha32(skFixedMul(first, firstH) >> 9)) //nolint:gosec // clamped

	alpha16 := sk32SatAdd(firstH, dY>>1)
	for i := int32(1); i < R-1; i++ {
		alphas[i] = uint8(clampAlpha32(alpha16 >> 8)) //nolint:gosec // clamped
		alpha16 = sk32SatAdd(alpha16, dY)
	}
	alphas[R-1] = saturatingSub8(fullAlpha, partialTriangleToAlpha(last, dY))
}

// computeAlphaBelowLine computes per-pixel alpha for the region below a line
// within a strip. Port of Skia's compute_alpha_below_line.
func computeAlphaBelowLine(alphas []uint8, l, r, dY int32, fullAlpha uint8) {
	if l < 0 {
		l = 0
	}
	if l > r {
		l, r = r, l
	}
	R := skFixedCeilToInt(r)
	if R <= 0 || int(R) > len(alphas) {
		return
	}
	if R == 1 {
		alphas[0] = getPartialAlpha8(trapezoidToAlpha(l, r), fullAlpha)
		return
	}

	last := r - intToSkFixed(R-1)
	lastH := skFixedMul(last, dY)
	alphas[R-1] = uint8(clampAlpha32(skFixedMul(last, lastH) >> 9)) //nolint:gosec // clamped

	alpha16 := sk32SatAdd(lastH, dY>>1)
	for i := R - 2; i > 0; i-- {
		alphas[i] = uint8(clampAlpha32(alpha16 >> 8)) //nolint:gosec // clamped
		alpha16 = sk32SatAdd(alpha16, dY)
	}

	first := skFixed1 - l
	alphas[0] = saturatingSub8(fullAlpha, partialTriangleToAlpha(first, dY))
}

// approximateIntersection approximates the X coordinate of the intersection
// of two lines: (l1, y)-(r1, y+1) and (l2, y)-(r2, y+1).
// Port of Skia's approximate_intersection.
func approximateIntersection(l1, r1, l2, r2 int32) int32 {
	if l1 > r1 {
		l1, r1 = r1, l1
	}
	if l2 > r2 {
		l2, r2 = r2, l2
	}
	maxL := l1
	if l2 > maxL {
		maxL = l2
	}
	minR := r1
	if r2 < minR {
		minR = r2
	}
	return (maxL + minR) / 2
}

// --- Fixed-point helper functions ---

// fixedToAlpha converts a SkFixed height (16.16) to an alpha value [0, 255].
// Port of Skia's fixed_to_alpha: get_partial_alpha(0xFF, f) = SkFixedRoundToInt(255 * f).
func fixedToAlpha(f int32) uint8 {
	if f <= 0 {
		return 0
	}
	if f >= skFixed1 {
		return 255
	}
	// SkFixedRoundToInt(255 * f) = (255 * f + SK_FixedHalf) >> 16
	v := (int64(255)*int64(f) + int64(skFixedHalf)) >> 16
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return uint8(v) //nolint:gosec // clamped above
}

func intToSkFixed(n int32) int32 {
	return n << 16
}

func skFixedFloorToInt(v int32) int32 {
	return v >> 16
}

func skFixedCeilToInt(v int32) int32 {
	return (v + skFixed1 - 1) >> 16
}

func skFixedFloorToFixed(v int32) int32 {
	return v & ^(skFixed1 - 1) // clear fractional bits
}

func skFixedCeilToFixed(v int32) int32 {
	return skFixedFloorToFixed(v + skFixed1 - 1)
}

func skFixedMul(a, b int32) int32 {
	return int32((int64(a) * int64(b)) >> 16)
}

func sk32SatAdd(a, b int32) int32 {
	sum := int64(a) + int64(b)
	if sum > 0x7FFFFFFF {
		return 0x7FFFFFFF
	}
	if sum < -0x80000000 {
		return -0x80000000
	}
	return int32(sum)
}

func saturatingSub8(a, b uint8) uint8 {
	if b >= a {
		return 0
	}
	return a - b
}

func clampAlpha32(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
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
	result := make([]float32, af.width)
	for i := 0; i < af.width; i++ {
		result[i] = float32(af.coverage[i]) / 255.0
	}
	return result
}

// AlphaRuns returns the alpha runs for the last processed scanline.
func (af *AnalyticFiller) AlphaRuns() *AlphaRuns {
	return af.alphaRuns
}

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
		offset := y * width
		if offset+width > len(buffer) {
			return
		}

		row := buffer[offset : offset+width]
		for i := range row {
			row[i] = 0
		}

		runs.CopyTo(row)
	})
}
