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

	// edgeStates is a persistent per-edge incremental X state, indexed by edgeBuf position.
	// Matches Skia's fX lifetime: initialized once from origin when the edge enters the AET
	// (goY slow path), then accumulated incrementally via fX += fDX >> yShift across ALL
	// subsequent sub-strips and pixel rows. NEVER recomputed from origin for active edges.
	// This eliminates SkFixedMul truncation differences vs Skia's incremental path.
	edgeStates []edgeYState

	// aetToState maps AET index -> edgeStates[] index for the current pixel row.
	// Rebuilt each row since the AET can remove/reorder entries.
	aetToState []int

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

	// Allocate persistent per-edge X state indexed by edgeBuf position.
	// This matches Skia's lifetime of fX: initialized once when the edge enters
	// the AET (goY from origin), then accumulated incrementally across all
	// subsequent pixel rows via fX += fDX >> yShift. Skia NEVER recomputes fX
	// from origin for an active edge (except on full-pixel-step fast path which
	// gives identical results). Re-initializing from origin each row introduces
	// a single SkFixedMul truncation that differs from the accumulated truncation,
	// causing 1-unit coverage differences on edge pixels.
	nEdges := len(af.edgeBuf)
	if cap(af.edgeStates) < nEdges {
		af.edgeStates = make([]edgeYState, nEdges)
	} else {
		af.edgeStates = af.edgeStates[:nEdges]
	}
	for i := range af.edgeStates {
		af.edgeStates[i] = edgeYState{}
	}

	for y := yMin; y < yMax; y++ {
		af.processScanlineAAA(y, aaScale, af.edgeBuf, fillRule, callback)
	}
}

// processScanlineAAA processes a single pixel scanline using Skia AAA with
// adaptive Y stepping and persistent incremental edge X state.
//
// Skia AAA (SkScan_AAAPath.cpp:1451-1601) does NOT iterate integer Y rows.
// Instead, it tracks fractional Y positions where edge endpoints fall and
// processes sub-strips between those boundaries. For an edge starting at
// y=10.3, the pixel row y=10 is split into [10.0, 10.3) with fullAlpha~77
// and [10.3, 11.0) with fullAlpha~179. This gives correct fractional alpha
// at shape boundaries.
//
// Edge X state (fX) is maintained PERSISTENTLY across pixel rows in
// af.edgeStates[], indexed by edgeBuf position. This matches Skia's fX
// accumulation: initialized once from origin when the edge enters the AET,
// then stepped incrementally via fX += fDX >> yShift for every sub-strip.
// Never recomputed from origin for active edges.
//
// Implementation:
//  1. Clear per-pixel alpha buffer
//  2. Insert new edges (with goY from-origin init), collect sub-strip boundaries
//  3. For each sub-strip [stripTop, stripBot):
//     a. Use persistent edgeStates for X positions
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

	// All Y tracking in SkFixed (16.16) — matches Skia's aaa_walk_edges exactly.
	yFixed := intToSkFixed(int32(y))        // pixel row start in SkFixed
	yFixedEnd := intToSkFixed(int32(y) + 1) // pixel row end in SkFixed

	// Insert new edges with source index tracking.
	// Initialize their persistent edgeState from origin (goY slow path).
	for af.edgeIdx < len(allEdges) {
		edge := allEdges[af.edgeIdx]
		topY := edge.TopY()
		if topY >= ySubpixelNext {
			break
		}
		af.aet.InsertWithIndex(edge, af.edgeIdx)
		af.initSingleEdgeState(af.edgeIdx, aaScale, yFixed)
		af.edgeIdx++
	}

	// Collect fractional Y boundaries from active edges within this pixel row.
	stripYs := af.collectStripBoundariesFixed(yFixed, yFixedEnd, aaScale)

	// Build the AET-index to edgeState-index mapping for this row.
	// Each AET entry has a srcIdx pointing to its edgeStates[] slot.
	n := af.aet.Len()
	if cap(af.aetToState) < n {
		af.aetToState = make([]int, n)
	}
	af.aetToState = af.aetToState[:n]
	for i := 0; i < n; i++ {
		af.aetToState[i] = af.aet.EdgeSrcIdx(i)
	}

	// Process each sub-strip using persistent incremental edge stepping.
	for si := 0; si < len(stripYs)-1; si++ {
		stripTop := stripYs[si]
		stripBot := stripYs[si+1]
		if stripBot <= stripTop {
			continue
		}

		af.processSubStripIncremental(stripTop, stripBot, fillRule)
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

// initSingleEdgeState initializes one edge's persistent X state when it first
// enters the AET. This matches Skia's goY(y) from-origin path (SkAnalyticEdge.h:59-68):
//
//	fX = fUpperX + SkFixedMul(fDX, y - fUpperY)
//
// Called ONCE per edge lifetime — the fX is then accumulated incrementally
// via fX += fDX >> yShift for all subsequent sub-strips and pixel rows.
//
// For edges starting within the pixel row (fUpperY > yRowFixed), fX is initialized
// at fUpperY rather than yRowFixed, matching Skia's insert_new_edges behavior.
func (af *AnalyticFiller) initSingleEdgeState(edgeBufIdx int, aaScale int32, yRowFixed int32) {
	edge := &af.edgeBuf[edgeBufIdx]
	line := edge.AsLine()
	if line == nil {
		af.edgeStates[edgeBufIdx] = edgeYState{}
		return
	}

	hasPrecise := line.UpperY != 0 || line.LowerY != 0

	var st edgeYState
	st.winding = line.Winding

	if hasPrecise {
		// Line edge with pixel-space fields — use Skia's exact goY() path.
		st.fUpperX = line.UpperX
		st.fUpperY = line.UpperY
		st.fLowerY = line.LowerY
		st.fDX = line.PixelDX
		st.fDY = computeEdgeDY(line.PixelDX)

		// Initialize fX at the edge's entry Y.
		// If edge starts at or before the row, compute at row start.
		// If edge starts within the row, compute at fUpperY (matching Skia's
		// insert_new_edges which calls goY at the edge's fUpperY).
		initY := yRowFixed
		if st.fUpperY > yRowFixed {
			initY = st.fUpperY
		}
		st.fX = line.UpperX + skFixedMul(line.PixelDX, initY-line.UpperY)
	} else {
		// Curve sub-segment: derive pixel-space from sub-pixel fields.
		st.fDX = line.DX
		refXPixel := int32(int64(line.X) / int64(aaScale))
		refYPixel := int32((int64(line.FirstY)*int64(skFixed1) + int64(skFixedHalf)) / int64(aaScale))
		st.fUpperX = refXPixel
		st.fUpperY = refYPixel
		st.fLowerY = int32(int64(line.LastY+1) * int64(skFixed1) / int64(aaScale))
		st.fDY = computeEdgeDY(line.DX)

		initY := yRowFixed
		if st.fUpperY > yRowFixed {
			initY = st.fUpperY
		}
		st.fX = refXPixel + skFixedMul(line.DX, initY-refYPixel)
	}

	st.valid = true
	af.edgeStates[edgeBufIdx] = st
}

// processSubStripIncremental resolves edges using incremental X stepping and blits
// trapezoids for a single sub-strip. Uses persistent per-edge edgeYState maintained
// across sub-strips AND pixel rows via af.edgeStates[srcIdx].
//
// Edge X positions are tracked incrementally matching Skia's goY(nextY, yShift)
// which steps fX += fDX >> yShift. The fX state persists across pixel rows to match
// Skia's accumulated incremental truncation pattern (vs recomputing from origin).
//
// For edges that start/end within the sub-strip (clamped boundaries), the slow path
// computes X from origin to match Skia's goY(y) (non-yShift overload).
func (af *AnalyticFiller) processSubStripIncremental(
	stripTopFixed, stripBotFixed int32,
	fillRule FillRule,
) {
	// Compute fullAlpha from SkFixed Y difference — Skia's fixed_to_alpha.
	yDiff := stripBotFixed - stripTopFixed
	fullAlpha := fixedToAlpha(yDiff)
	if fullAlpha == 0 {
		// Even if we can't blit, we must advance fX for edges that span this sub-strip.
		af.advanceEdgeStates(stripTopFixed, stripBotFixed, yDiff)
		return
	}

	n := len(af.aetToState)
	if cap(af.resolvedEdges) < n {
		af.resolvedEdges = make([]edgeLineState, n)
	}
	af.resolvedEdges = af.resolvedEdges[:0]

	for i := 0; i < n; i++ {
		srcIdx := af.aetToState[i]
		if srcIdx < 0 || srcIdx >= len(af.edgeStates) {
			continue
		}
		st := &af.edgeStates[srcIdx]
		if !st.valid {
			continue
		}

		// Check if edge segment covers this sub-strip.
		if st.fUpperY >= stripBotFixed || st.fLowerY <= stripTopFixed {
			continue
		}

		// Clamp to edge segment boundaries.
		clampedTop := stripTopFixed
		clampedBot := stripBotFixed
		if clampedTop < st.fUpperY {
			clampedTop = st.fUpperY
		}
		if clampedBot > st.fLowerY {
			clampedBot = st.fLowerY
		}
		if clampedBot <= clampedTop {
			continue
		}

		edgeAlpha := fullAlpha
		if clampedTop != stripTopFixed || clampedBot != stripBotFixed {
			edgeAlpha = fixedToAlpha(clampedBot - clampedTop)
			if edgeAlpha == 0 {
				continue
			}
		}

		// Compute topX and botX. Two cases:
		//
		// 1. Edge spans the full sub-strip without clamping: use incremental stepping.
		//    topX = current fX, botX = fX + (fDX >> yShift) matching Skia goY(nextY, yShift).
		//
		// 2. Edge starts/ends within the sub-strip (clamped): use from-origin slow path.
		//    This matches Skia's goY(y) (non-yShift overload).
		topX := st.fX
		var botX int32
		fullSpan := clampedTop == stripTopFixed && clampedBot == stripBotFixed

		if fullSpan {
			// Incremental step: matches Skia's goY(nextY, yShift).
			// fX += fDX >> yShift for standard sub-strip heights.
			yShift := computeYShift(yDiff)
			if yShift >= 0 {
				botX = topX + (st.fDX >> uint(yShift))
			} else {
				// Non-standard height: use from-origin formula.
				botX = st.fUpperX + skFixedMul(st.fDX, stripBotFixed-st.fUpperY)
			}
			// Advance fX for next sub-strip.
			st.fX = botX
		} else {
			// Clamped: compute from origin (Skia goY slow path).
			topX = st.fUpperX + skFixedMul(st.fDX, clampedTop-st.fUpperY)
			botX = st.fUpperX + skFixedMul(st.fDX, clampedBot-st.fUpperY)
			// Set fX to the strip bottom position for subsequent sub-strips.
			st.fX = st.fUpperX + skFixedMul(st.fDX, stripBotFixed-st.fUpperY)
		}

		af.resolvedEdges = append(af.resolvedEdges, edgeLineState{
			valid:     true,
			topX:      topX,
			botX:      botX,
			dy:        st.fDY,
			fullAlpha: edgeAlpha,
			winding:   st.winding,
		})
	}

	sortEdgesByTopX(af.resolvedEdges)

	// Paired-edge walk: Skia AAA pattern (SkScan_AAAPath.cpp:1490-1530).
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

	_ = inInterval
}

// advanceEdgeStates advances fX for all active edges across a sub-strip,
// even when fullAlpha is 0 (no visible output). This keeps the incremental
// X state consistent for subsequent sub-strips and pixel rows.
func (af *AnalyticFiller) advanceEdgeStates(stripTopFixed, stripBotFixed, yDiff int32) {
	yShift := computeYShift(yDiff)
	for i := range af.aetToState {
		srcIdx := af.aetToState[i]
		if srcIdx < 0 || srcIdx >= len(af.edgeStates) {
			continue
		}
		st := &af.edgeStates[srcIdx]
		if !st.valid || st.fUpperY >= stripBotFixed || st.fLowerY <= stripTopFixed {
			continue
		}
		if st.fUpperY <= stripTopFixed && st.fLowerY >= stripBotFixed {
			// Full span: incremental step.
			if yShift >= 0 {
				st.fX += st.fDX >> uint(yShift)
			} else {
				st.fX = st.fUpperX + skFixedMul(st.fDX, stripBotFixed-st.fUpperY)
			}
		} else {
			// Partial: from-origin.
			st.fX = st.fUpperX + skFixedMul(st.fDX, stripBotFixed-st.fUpperY)
		}
	}
}

// computeYShift determines the yShift for a given sub-strip height,
// matching Skia's logic in aaa_walk_edges (SkScan_AAAPath.cpp:1466-1472).
//
// yShift=2 for quarter pixel (16384), yShift=1 for half pixel (32768),
// yShift=0 for full pixel (65536). Returns -1 for non-standard heights.
func computeYShift(yDiff int32) int {
	switch yDiff {
	case skFixed1 >> 2: // 16384 — quarter pixel
		return 2
	case skFixed1 >> 1: // 32768 — half pixel
		return 1
	case skFixed1: // 65536 — full pixel
		return 0
	default:
		return -1
	}
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

	// Detect crossing edges — if any edges cross X order within this row,
	// split into 4 sub-strips (1/4 pixel each, matching Skia accuracy=2).
	// Without crossing, use single full strip (fullAlpha=255) like Skia.
	if af.hasEdgeCrossing(yTopFixed, yBotFixed, aaScale) {
		af.stripYBuf = af.stripYBuf[:0]
		quarterPixel := skFixed1 / 4
		for y := yTopFixed; y < yBotFixed; y += quarterPixel {
			af.stripYBuf = append(af.stripYBuf, y)
		}
		af.stripYBuf = append(af.stripYBuf, yBotFixed)
	}

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

func (af *AnalyticFiller) hasEdgeCrossing(yTopFixed, yBotFixed, aaScale int32) bool {
	n := af.aet.Len()
	if n < 2 {
		return false
	}
	type xp struct{ topX, botX int32 }
	var sb [16]xp
	var ps []xp
	if n <= len(sb) {
		ps = sb[:0]
	} else {
		ps = make([]xp, 0, n)
	}
	for i := 0; i < n; i++ {
		edge := af.aet.EdgeAt(i)
		line := edge.AsLine()
		if line == nil {
			continue
		}
		hasPrecise := line.UpperY != 0 || line.LowerY != 0
		topX, botX := computeEdgeX(line, aaScale, hasPrecise, yTopFixed, yBotFixed)
		ps = append(ps, xp{topX, botX})
	}
	for i := 0; i < len(ps); i++ {
		for j := i + 1; j < len(ps); j++ {
			dt := int64(ps[i].topX) - int64(ps[j].topX)
			db := int64(ps[i].botX) - int64(ps[j].botX)
			if (dt > 0 && db < 0) || (dt < 0 && db > 0) {
				return true
			}
		}
	}
	return false
}

// processSubStripFixed resolves edges and blits trapezoids for a single sub-strip
// within a pixel row. Coverage is added to the existing coverage buffer.
// All Y parameters are in SkFixed (16.16) — no float32 conversion.
//
// Implements Skia's edges_too_close optimization (SkScan_AAAPath.cpp:1380-1397):
// when the right edge of a trapezoid is within 1 pixel of the next edge, the
// winding-to-zero transition is suppressed, merging adjacent trapezoids. This
// prevents double-subtraction of partial coverage at shared pixels where edges
// cross within a sub-strip (e.g., star vertex intersections at y=68).
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

	sortEdgesByTopX(af.resolvedEdges)

	// Paired-edge walk: Skia AAA pattern (SkScan_AAAPath.cpp:1490-1530).
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

// sortEdgesByTopX sorts resolved edges by their X position at the top of the
// sub-strip (topX), matching Skia's linked-list order sorted by fX.
//
// Skia maintains edges in a linked list sorted by fX (the current X position).
// At the start of each sub-strip iteration, edges are in fX order (the X at the
// current Y). Using topX (not midX) ensures our sort matches Skia's edge order.
//
// This matters for self-intersecting paths where edges cross within a sub-strip.
// With midX sort, edges that are close together can be reordered relative to Skia's
// fX sort, producing different winding transitions and different trapezoids.
// With topX sort, the winding walk produces the same trapezoids as Skia.
//
// Secondary sort by DX (slope) matches Skia's compare_edges for stability.
func sortEdgesByTopX(edges []edgeLineState) {
	// Simple insertion sort — edge count per scanline is typically small (<20).
	// Primary: topX (Skia's fX). Secondary: slope direction via (botX - topX),
	// matching Skia's compare_edges which uses fDX as tiebreaker.
	for i := 1; i < len(edges); i++ {
		key := edges[i]
		keyX := key.topX
		keySlope := key.botX - key.topX // proxy for fDX direction
		j := i - 1
		for j >= 0 {
			ejX := edges[j].topX
			if ejX > keyX {
				edges[j+1] = edges[j]
				j--
				continue
			}
			if ejX == keyX && (edges[j].botX-edges[j].topX) > keySlope {
				edges[j+1] = edges[j]
				j--
				continue
			}
			break
		}
		edges[j+1] = key
	}
}

// edgeYState tracks per-edge X state for Skia's incremental goY() stepping within a
// pixel row. Instead of recomputing X from origin at each sub-strip boundary (which
// introduces SkFixedMul truncation errors), we maintain fX and step incrementally
// with fX += fDX >> yShift — exactly matching Skia's goY(nextY, yShift) from
// SkAnalyticEdge.h:71-76.
//
// Lifecycle: initialized once at the start of each pixel row via goY(y) from origin,
// then stepped within each sub-strip. Discarded at end of pixel row.
type edgeYState struct {
	fX      int32 // current X position in pixel-space SkFixed (16.16)
	fDX     int32 // slope (pixel-space SkFixed), matches Skia's fDX
	fUpperX int32 // X at fUpperY (pixel-space SkFixed), for goY() slow path
	fUpperY int32 // upper Y boundary (pixel-space SkFixed)
	fLowerY int32 // lower Y boundary (pixel-space SkFixed)
	fDY     int32 // abs(1/slope) in SkFixed, for partialTriangleToAlpha
	winding int8
	valid   bool // false if edge not active in this pixel row
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

	// Edge crossing at top: Skia returns early (SkScan_AAAPath.cpp:819).
	// This happens due to precision limits at vertices where edges share
	// the same start point. Skia skips the entire trapezoid.
	if ul > ur {
		return
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
//
// Port of Skia's blit_trapezoid_row left partial (SkScan_AAAPath.cpp:847-883).
// In the 2-pixel case, a1 and a2 are added DIRECTLY without scaling by fullAlpha,
// because a2 = fullAlpha - partial_triangle_to_alpha(second, lDY) already
// incorporates fullAlpha. Skia's blit_two_alphas adds a1/a2 directly to the
// mask or additive blitter runs. Only the 1-pixel case uses get_partial_alpha
// because trapezoid_to_alpha returns a value in [0,255] independent of fullAlpha.
func (af *AnalyticFiller) blitLeftPartial(ul, ll, joinLeft, lDY int32, fullAlpha uint8) {
	if ul >= joinLeft {
		return
	}
	switch skFixedCeilToInt(joinLeft - ul) {
	case 1:
		af.safeAddAlpha(skFixedFloorToInt(ul), trapezoidToAlphaScaled(joinLeft-ul, joinLeft-ll, fullAlpha))
	case 2:
		// Skia blit_trapezoid_row 2-pixel case (SkScan_AAAPath.cpp:858-870):
		// a1 = partial_triangle_to_alpha(first, lDY)  -- small triangle at pixel edge
		// a2 = fullAlpha - partial_triangle_to_alpha(second, lDY)  -- rest of strip
		// blit_two_alphas adds a1/a2 DIRECTLY (no fullAlpha scaling).
		first := joinLeft - skFixed1 - ul
		second := ll - ul - first
		a1 := partialTriangleToAlpha(first, lDY)
		a2 := saturatingSub8(fullAlpha, partialTriangleToAlpha(second, lDY))
		af.safeAddAlpha(skFixedFloorToInt(ul), a1)
		af.safeAddAlpha(skFixedFloorToInt(ul)+1, a2)
	default:
		af.blitAaaTrapezoidRow(ul, joinLeft, ll, joinLeft, lDY, 0x7FFFFFFF, fullAlpha)
	}
}

// blitRightPartial handles the right edge's partial-coverage pixels.
//
// Port of Skia's blit_trapezoid_row right partial (SkScan_AAAPath.cpp:896-932).
// Same as blitLeftPartial: 2-pixel case adds a1/a2 directly, 1-pixel case scales.
func (af *AnalyticFiller) blitRightPartial(ur, lr, joinRite, rDY int32, fullAlpha uint8) {
	if lr <= joinRite {
		return
	}
	switch skFixedCeilToInt(lr - joinRite) {
	case 1:
		af.safeAddAlpha(skFixedFloorToInt(joinRite), trapezoidToAlphaScaled(ur-joinRite, lr-joinRite, fullAlpha))
	case 2:
		// Skia blit_trapezoid_row right 2-pixel case (SkScan_AAAPath.cpp:907-919):
		// a1 = fullAlpha - partial_triangle_to_alpha(first, rDY)
		// a2 = partial_triangle_to_alpha(second, rDY)
		// blit_two_alphas adds a1/a2 DIRECTLY.
		first := joinRite + skFixed1 - ur
		second := lr - ur - first
		a1 := saturatingSub8(fullAlpha, partialTriangleToAlpha(first, rDY))
		a2 := partialTriangleToAlpha(second, rDY)
		af.safeAddAlpha(skFixedFloorToInt(joinRite), a1)
		af.safeAddAlpha(skFixedFloorToInt(joinRite)+1, a2)
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
		af.safeAddAlpha(baseX, trapezoidToAlphaScaled(ur-ul, lr-ll, fullAlpha))
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

// trapezoidToAlphaScaled computes per-pixel trapezoid alpha scaled by fullAlpha,
// matching Skia's two code paths in blit_single_alpha (SkScan_AAAPath.cpp:644-664):
//
// When fullAlpha==255: Skia writes trapezoid_to_alpha directly (real blitter path).
// The formula area>>8 truncates, giving the same result as (255*area+32768)>>16
// for all valid area values. We use the rounding formula for consistency with both
// Skia code paths (aaa_walk_edges and aaa_walk_convex_edges).
//
// When fullAlpha<255: Skia applies get_partial_alpha(trapezoid_to_alpha, fullAlpha)
// = (area>>8 * fullAlpha) >> 8, which double-truncates. We must match this exactly.
func trapezoidToAlphaScaled(l1, l2 int32, fullAlpha uint8) uint8 {
	if l1 < 0 {
		l1 = 0
	}
	if l2 < 0 {
		l2 = 0
	}
	area := (int64(l1) + int64(l2)) / 2 // SkFixed area (16.16)

	if fullAlpha == 255 {
		// Single-pass: (255 * area + rounding) >> 16.
		// This matches both:
		// - aaa_walk_edges: trapezoid_to_alpha(l1,l2) written directly = area >> 8
		// - aaa_walk_convex_edges: fixed_to_alpha(area) = (255*area+32768)>>16
		// The two formulas agree for all valid area values [0, SK_Fixed1].
		v := (255*area + int64(skFixedHalf)) >> 16
		if v > 255 {
			return 255
		}
		if v < 0 {
			return 0
		}
		return uint8(v) //nolint:gosec // clamped above
	}

	// Match Skia's double-truncation: trapezoid_to_alpha then get_partial_alpha.
	alpha := int32(area >> 8)
	if alpha > 255 {
		alpha = 255
	}
	if alpha < 0 {
		alpha = 0
	}
	return uint8((uint16(alpha) * uint16(fullAlpha)) >> 8) //nolint:gosec // clamped
}

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
// Exact port of Skia's get_partial_alpha(SkAlpha, SkAlpha) (SkScan_AAAPath.cpp:565-567):
//
//	return (alpha * fullAlpha) >> 8;
//
// Uses truncation (NOT rounding). This is the overload for two SkAlpha arguments.
// The other overload (SkAlpha, SkFixed) uses rounding — see fixedToAlpha.
func getPartialAlpha8(alpha, fullAlpha uint8) uint8 {
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
// Exact port of Skia's fixed_to_alpha (SkScan_AAAPath.cpp:572-575):
//
//	get_partial_alpha(0xFF, f) = SkFixedRoundToInt(255 * f)
//	                           = (255 * f + SK_FixedHalf) >> 16
//
// Uses ROUNDING (adds SK_FixedHalf = 32768 before shift). This is the overload
// for (SkAlpha, SkFixed) — contrast with getPartialAlpha8 which uses truncation.
//
// Key values:
//
//	fixedToAlpha(16384) = 64   (1/4 pixel sub-strip, 4*64=256 → clamped to 255)
//	fixedToAlpha(32768) = 128  (1/2 pixel)
//	fixedToAlpha(65536) = 255  (full pixel)
func fixedToAlpha(f int32) uint8 {
	if f <= 0 {
		return 0
	}
	if f >= skFixed1 {
		return 255
	}
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
