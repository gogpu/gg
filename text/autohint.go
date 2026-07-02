// Package text provides GPU text rendering infrastructure.
//
// This file implements the main auto-hinter pipeline for Latin script.
// It is a Go port of the FreeType auto-hinter (aflatin.c) and skrifa
// (Rust fontations). The pipeline:
//
//  1. Compute standard stem widths from a reference glyph ('o')
//  2. Detect blue zones from reference characters (THEZOCQS, etc.)
//  3. Scale widths and blue zones to target pixel size
//  4. Per-glyph: detect segments, link into stems, group into edges
//  5. Match edges to blue zones
//  6. Grid-fit edges: blue-anchored → stems → serifs → singles
//  7. Propagate edge positions to all outline points
//
// References:
//   - FreeType aflatin.c (5102 LOC) — THE primary reference
//   - skrifa fontations (Rust) — cleaner architecture, same algorithms
//   - Skia SkAutoHinter — validates our approach
package text

import (
	"math"
	"sync"
)

// ============================================================
// Fixed-Point Arithmetic (FreeType/skrifa parity)
// ============================================================
//
// FreeType and skrifa use integer fixed-point arithmetic throughout
// the auto-hinter to ensure deterministic, platform-independent results.
//
// Two formats are used:
//   - 26.6 (F26Dot6): 1 unit = 1/64 pixel. For coordinates and positions.
//   - 16.16 (F16Dot16): 1 unit = 1/65536. For scale factors and interpolation.
//
// All hinting arithmetic operates in these integer formats. Float is used
// only at boundaries: font units → 26.6 on input, 26.6 → float32 on output.
//
// References:
//   - FreeType include/freetype/fttypes.h: FT_F26Dot6, FT_Fixed (16.16)
//   - FreeType ftcalc.h: FT_MulFix, FT_DivFix, FT_PIX_ROUND
//   - skrifa metrics/mod.rs: fixed_mul, fixed_div, pix_round, pix_floor

// f26dot6 is 26.6 fixed-point: 1 unit = 1/64 pixel.
// Used for coordinates and positions in the auto-hinter.
type f26dot6 = int32

// Note: 16.16 fixed-point (1 unit = 1/65536) is used implicitly via int32
// in fixedMul26dot6/fixedDiv26dot6. Not declared as a named type to avoid
// unused-type lint warnings — the int32 bit pattern is self-documenting.

// f26dot6FromFloat converts a pixel value to 26.6 fixed-point.
// Matches FreeType's float-to-fixed conversion: round(px * 64).
func f26dot6FromFloat(px float64) f26dot6 {
	return int32(math.Round(px * 64))
}

// f26dot6ToFloat converts 26.6 fixed-point to float32 pixels.
func f26dot6ToFloat(v f26dot6) float32 {
	return float32(v) / 64.0
}

// f26dot6Round rounds a 26.6 value to the nearest integer pixel.
// Equivalent to FreeType's FT_PIX_ROUND: (x + 32) & ~63.
// Matches skrifa metrics/mod.rs: pix_round.
func f26dot6Round(v f26dot6) f26dot6 {
	return (v + 32) & ^63
}

// f26dot6Floor rounds a 26.6 value down to the nearest integer pixel.
// Equivalent to FreeType's FT_PIX_FLOOR: x & ~63.
// Matches skrifa metrics/mod.rs: pix_floor.
func f26dot6Floor(v f26dot6) f26dot6 {
	return v & ^63
}

// Note: f26dot6Ceil, fixedMulFix, fixedDivFix removed — not needed by current
// pipeline. The pipeline uses fixedMul26dot6/fixedDiv26dot6 (bits-level 16.16
// operations matching skrifa's fixed_mul/fixed_div) instead of FT_MulFix/FT_DivFix.

// fixedMul26dot6 multiplies two values as 16.16 fixed-point.
// Matches skrifa Fixed::Mul (font-types/src/fixed.rs:189):
//
//	ab = a * b; result = (ab + 0x8000 - (1 if ab<0 else 0)) >> 16
//
// The sign adjustment ensures proper rounding for negative products.
func fixedMul26dot6(a, b int32) int32 {
	ab := int64(a) * int64(b)
	var signAdj int64
	if ab < 0 {
		signAdj = 1
	}
	return int32((ab + 0x8000 - signAdj) >> 16)
}

// fixedDiv26dot6 divides two values as 16.16 fixed-point with rounding.
// Matches skrifa Fixed::Div (font-types/src/fixed.rs:198):
//
//	Uses absolute values, adds half-divisor for rounding:
//	result = (|a| << 16 + |b|/2) / |b|, with sign applied.
func fixedDiv26dot6(a, b int32) int32 {
	if b == 0 {
		return 0
	}
	sign := (a < 0) != (b < 0)
	au := uint64(a)
	if a < 0 {
		au = uint64(-int64(a))
	}
	bu := uint64(b)
	if b < 0 {
		bu = uint64(-int64(b))
	}
	q := (au<<16 + bu/2) / bu
	if sign {
		return -int32(q)
	}
	return int32(q)
}

// RawFontDataProvider is an optional interface that ParsedFont implementations
// may implement to provide access to the underlying raw font data. This is
// needed for the contour-based auto-hinter path, which reads raw TrueType
// glyf table points for FreeType/skrifa coordinate parity.
//
// Fonts that don't implement this interface will use the legacy outline-based
// auto-hinter path (which produces correct results but operates on a different
// point count than FreeType/skrifa due to pen decomposition).
type RawFontDataProvider interface {
	// RawFontData returns the raw font file bytes. Returns nil if the raw
	// data is not available (e.g., font was parsed from a stream).
	RawFontData() []byte
}

// autoHintOutline applies the full FreeType-level auto-hinting pipeline
// to the given outline using raw contour points (Y-UP convention).
//
// Returns true if hinting was successfully applied via the contour-based
// path. Returns false if raw font data is not available or the glyph
// has no outline (empty glyph, CFF font, TTC parse failure). Composite
// glyphs are handled transparently by ParseGlyfContours.
//
// When false is returned, the caller should fall back to gridFitOutline
// for basic grid-fitting. The legacy outline-based path is NOT used as
// fallback because sfnt-extracted outlines are in Y-DOWN convention,
// while the auto-hinter pipeline (segments, edges, blue zones) operates
// in Y-UP convention. Feeding Y-DOWN coordinates to Y-UP hinting
// collapses all Y coordinates to the baseline.
//
// The pipeline (when successful):
//  1. Parse raw contour points from glyf table (Y-UP font units)
//  2. Get (or compute) per-font metrics (standard widths, blue zones)
//  3. Scale metrics to the target pixel size
//  4. Analyze glyph topology (segments → stems → edges)
//  5. Match edges to blue zones
//  6. Grid-fit edges (blue-anchored, stems, serifs, singles)
//  7. Propagate edge adjustments to all outline points
//  8. Convert hinted contour points back to outline segments
func autoHintOutline(outline *GlyphOutline, font ParsedFont, ppem float64, hinting Hinting) bool {
	if outline == nil || len(outline.Segments) == 0 {
		return false
	}

	// Try contour-based path if raw font data is available.
	// This produces FreeType/skrifa coordinate parity by operating on
	// the exact raw N contour points (Y-UP) instead of the M pen-derived
	// points from sfnt extraction (Y-DOWN).
	if provider, ok := font.(RawFontDataProvider); ok {
		if rawData := provider.RawFontData(); rawData != nil {
			if autoHintViaContours(outline, rawData, font, ppem, hinting) {
				return true
			}
		}
	}

	return false
}

// autoHintOutlineVar applies auto-hinting to a variable font outline using
// pre-computed gvar-varied contour points. This is the variable-font
// counterpart of autoHintOutline.
//
// The critical difference: autoHintOutline re-reads contour points from
// the raw glyf table (which contains UNVARIED data). For variable fonts,
// the contour points must have gvar deltas already applied. This function
// uses the pre-varied contours directly, preserving the gvar modifications.
//
// variedContours may be nil (e.g., for empty glyphs or composite glyphs),
// in which case this falls back to the static autoHintOutline path.
func autoHintOutlineVar(outline *GlyphOutline, variedContours *GlyfContours, font ParsedFont, ppem float64, hinting Hinting) bool {
	if outline == nil || len(outline.Segments) == 0 {
		return false
	}

	// If we have pre-varied contour points, use them directly.
	if variedContours != nil && len(variedContours.Points) > 0 {
		return autoHintViaContoursPreloaded(outline, variedContours, font, ppem, hinting)
	}

	// Fallback: no pre-varied contours available (composite glyph, etc.)
	// Use the static path which re-reads from glyf.
	return autoHintOutline(outline, font, ppem, hinting)
}

// hintedEdgeMetrics captures the leftmost and rightmost horizontal edge
// positions (original and hinted) from the auto-hinting pipeline. These
// are used to compute adjusted advance widths per skrifa instance.rs:127-183.
//
// Values are in 26.6 fixed-point.
type hintedEdgeMetrics struct {
	leftOpos  int32 // leftmost H-edge original position (26.6)
	leftPos   int32 // leftmost H-edge hinted position (26.6)
	rightOpos int32 // rightmost H-edge original position (26.6)
	rightPos  int32 // rightmost H-edge hinted position (26.6)
	hasEdges  bool  // true if at least 2 H-edges were found
}

// computeAdjustedAdvance computes the hinted advance width from raw advance
// and edge metrics. This is a faithful port of skrifa instance.rs:127-183.
//
// The algorithm:
//  1. Scale the font-unit advance to 26.6 via fixed-point multiply
//  2. If H-edges exist, compute phantom points pp1x/pp2x from edge shifts
//  3. Apply "prefer too much space" bias for small bearings (<24 in 26.6)
//  4. Round phantom points to pixel grid
//  5. Ensure phantom points don't collapse past the edges
//  6. Return pp2x - pp1x as the adjusted advance
//
// For fixed-width fonts or when advance is zero, special handling applies.
//
// References:
//   - skrifa instance.rs:127-183 (advance adjustment)
//   - FreeType afloader.c:422 (FT_PIX_ROUND adjustment)
func computeAdjustedAdvance(fontUnitAdvance int32, xScale16dot16 int32, metrics hintedEdgeMetrics) (advance int32, pp1x int32) {
	pp2x := fixedMul26dot6(fontUnitAdvance, xScale16dot16)

	if !metrics.hasEdges {
		// No H-edges: just round the scaled advance.
		return f26dot6Round(pp2x), 0
	}

	oldRSB := pp2x - metrics.rightOpos
	oldLSB := metrics.leftOpos
	newLSB := metrics.leftPos

	pp1xUH := newLSB - oldLSB
	pp2xUH := metrics.rightPos + oldRSB

	// "Prefer too much space over too little" bias for small sizes.
	// FreeType/skrifa: if bearing < 24/64 px (~0.375 px), add 8/64 px padding.
	if oldLSB < 24 {
		pp1xUH -= 8
	}
	if oldRSB < 24 {
		pp2xUH += 8
	}

	pp1x = f26dot6Round(pp1xUH)
	pp2x = f26dot6Round(pp2xUH)

	// Compensation: ensure left phantom doesn't exceed hinted left edge.
	if pp1x >= newLSB && oldLSB > 0 {
		pp1x -= 64
	}
	// Compensation: ensure right phantom doesn't fall behind hinted right edge.
	if pp2x <= metrics.rightPos && oldRSB > 0 {
		pp2x += 64
	}

	return pp2x - pp1x, pp1x
}

// autoHintViaContours applies auto-hinting via the raw contour point path.
// Returns true if successful, false if the contour path is not applicable
// (e.g., empty glyph, CFF font). Composite glyphs are handled by
// ParseGlyfContours which recursively flattens components.
//
// In addition to hinting coordinates, this computes adjusted advance widths
// using the skrifa advance adjustment algorithm (instance.rs:127-183). The
// adjusted advance is stored in outline.Advance and the outline is translated
// by -pp1x to account for the shifted left side bearing.
func autoHintViaContours(outline *GlyphOutline, fontData []byte, font ParsedFont, ppem float64, hinting Hinting) bool {
	// Parse raw contour points.
	contours, err := ParseGlyfContours(fontData, outline.GID)
	if err != nil || contours == nil {
		return false // Not a simple TrueType glyph — use legacy path.
	}

	// Apply auto-hinting on raw contour points.
	hinted, edgeMetrics := autoHintContourPoints(contours, font, ppem, hinting)
	if hinted == nil {
		return false
	}

	// Compute adjusted advance width (skrifa instance.rs:127-183).
	// Get font-unit advance from ParsedFont. GlyphAdvance at UPM returns
	// the font-unit value as a float (fontUnitAdvance * upm/upm = fontUnits).
	upm := font.UnitsPerEm()
	fontUnitAdvance := int32(math.Round(font.GlyphAdvance(uint16(outline.GID), float64(upm))))
	xScale := computeScale16dot16(ppem / float64(upm))

	adjustedAdvance, pp1x := computeAdjustedAdvance(fontUnitAdvance, xScale, edgeMetrics)

	// Translate outline points by -pp1x if the left phantom shifted.
	// This matches skrifa instance.rs:165-168.
	if pp1x != 0 {
		for i := range hinted.Points {
			hinted.Points[i].X -= int16(pp1x)
		}
	}

	// Convert hinted contour points to outline segments for rendering.
	// The hinted contours have coordinates in pixel space (26.6 integer-like).
	hintedOutline := contoursToOutline(hinted)

	// Transfer results: replace the outline's segments and bounds.
	outline.Segments = hintedOutline.Segments
	refreshOutlineBounds(outline)

	// Set adjusted advance (convert from 26.6 to pixels, then round).
	// Matches skrifa instance.rs:183: F26Dot6::from_bits(pix_round(advance)).to_f32()
	outline.Advance = f26dot6ToFloat(f26dot6Round(adjustedAdvance))

	return true
}

// autoHintViaContoursPreloaded applies auto-hinting using pre-loaded contour
// points (typically gvar-varied for variable fonts). This avoids re-reading
// contour data from the raw glyf table, which would lose gvar deltas.
//
// The logic is identical to autoHintViaContours except the contour source:
// instead of ParseGlyfContours(fontData, gid), we use the provided contours.
func autoHintViaContoursPreloaded(outline *GlyphOutline, contours *GlyfContours, font ParsedFont, ppem float64, hinting Hinting) bool {
	if contours == nil || len(contours.Points) == 0 {
		return false
	}

	// Apply auto-hinting on the (possibly gvar-varied) contour points.
	hinted, edgeMetrics := autoHintContourPoints(contours, font, ppem, hinting)
	if hinted == nil {
		return false
	}

	// Compute adjusted advance width (skrifa instance.rs:127-183).
	upm := font.UnitsPerEm()
	fontUnitAdvance := int32(math.Round(font.GlyphAdvance(uint16(outline.GID), float64(upm))))
	xScale := computeScale16dot16(ppem / float64(upm))

	adjustedAdvance, pp1x := computeAdjustedAdvance(fontUnitAdvance, xScale, edgeMetrics)

	// Translate outline points by -pp1x if the left phantom shifted.
	if pp1x != 0 {
		for i := range hinted.Points {
			hinted.Points[i].X -= int16(pp1x)
		}
	}

	// Convert hinted contour points to outline segments for rendering.
	hintedOutline := contoursToOutline(hinted)

	// Transfer results: replace the outline's segments and bounds.
	outline.Segments = hintedOutline.Segments
	refreshOutlineBounds(outline)

	// Set adjusted advance.
	outline.Advance = f26dot6ToFloat(f26dot6Round(adjustedAdvance))

	return true
}

// autoHintContourPoints applies the full auto-hinting pipeline to raw TrueType
// contour points. This is the FreeType/skrifa-correct path that operates on
// the original N points (e.g., 32 for NotoSerifHebrew glyph 9), not on the
// pen-derived M points (42 from outline segments).
//
// The pipeline is identical to autoHintOutline, but inputs/outputs are
// GlyfContours instead of GlyphOutline:
//
//  1. Scale raw contour points to pixel space
//  2. Build hintPointArray from scaled contour points
//  3. Run segment detection, edge grouping, hinting (same code)
//  4. Propagate hinted edge positions to all contour points
//  5. Write hinted pixel coordinates back to contour points (still scaled)
//
// Returns the hinted contours (coordinates in PIXEL space, 26.6) and the
// horizontal edge metrics for advance width adjustment. The caller uses the
// edge metrics to compute the adjusted advance per skrifa instance.rs:127-183.
func autoHintContourPoints(contours *GlyfContours, font ParsedFont, ppem float64, hinting Hinting) (*GlyfContours, hintedEdgeMetrics) {
	var metrics hintedEdgeMetrics

	if contours == nil || len(contours.Points) == 0 {
		return contours, metrics
	}

	// Get or compute font-level metrics (cached per font).
	unscaled := getAutoHintMetrics(font)
	if unscaled == nil {
		return contours, metrics
	}

	// Scale: ppem / unitsPerEm.
	upm := font.UnitsPerEm()
	scale := ppem / float64(upm)
	scaled := unscaled.scaleWithUPM(scale, upm)

	// Build the point array from raw contour points.
	// Stores font-unit coords (fx/fy) and scaled coords (ox/oy/x/y).
	points := buildHintPointsFromContours(contours, scale, upm)
	if len(points.pts) == 0 {
		return contours, metrics
	}

	// Process each dimension (same logic as autoHintOutline).
	dims := []hintDimension{dimVertical}
	if hinting == HintingFull {
		dims = append([]hintDimension{dimHorizontal}, dims...)
	}

	group := unscaled.group

	for _, dim := range dims {
		axisMetrics := &scaled.axes[dim]

		// Detect segments along this axis.
		segments := computeSegments(&points, dim)
		if len(segments) == 0 {
			continue
		}

		// Adjust segment heights (skrifa parity).
		adjustSegmentHeights(&points, segments, dim)

		// Link segments into stems.
		linkSegments(segments, axisMetrics, group)

		// Group segments into edges.
		edges := computeEdges(segments, axisMetrics, dim, group)
		if len(edges) == 0 {
			continue
		}

		// Match edges to blue zones.
		// Default: vertical dimension only. CJK: both dimensions.
		if dim == dimVertical || group == scriptGroupCJK {
			computeBlueEdges(edges, axisMetrics, group)
		}

		// Grid-fit edges.
		hintEdges(edges, axisMetrics, group)

		// Propagate edge positions to points.
		alignEdgePoints(&points, segments, edges, dim, group)
		alignStrongPoints(&points, edges, dim)
		alignWeakPoints(&points, dim)

		// Capture horizontal edge metrics for advance adjustment.
		// Matches skrifa hint/mod.rs:175-184: after H-dimension complete,
		// record leftmost and rightmost edge opos/pos.
		if dim == dimHorizontal && len(edges) > 1 {
			metrics.hasEdges = true
			metrics.leftOpos = edges[0].opos
			metrics.leftPos = edges[0].pos
			metrics.rightOpos = edges[len(edges)-1].opos
			metrics.rightPos = edges[len(edges)-1].pos
		}
	}

	// Build a result with hinted coordinates in pixel space (26.6-like).
	// We keep the same structure (EndPts, bounding box) but update point coordinates.
	// The hinted coordinates are in Y-UP convention (matching FreeType/skrifa).
	result := &GlyfContours{
		Points: make([]ContourPoint, len(contours.Points)),
		EndPts: make([]uint16, len(contours.EndPts)),
		XMin:   contours.XMin,
		YMin:   contours.YMin,
		XMax:   contours.XMax,
		YMax:   contours.YMax,
	}
	copy(result.EndPts, contours.EndPts)

	// Copy hinted coordinates from 26.6 fixed-point to int16.
	// Coordinates are in Y-UP convention (matching the skrifa golden dump
	// where coordinates are in 26.6 integer units with Y pointing up).
	// The int16 coordinates ARE the 26.6 values (not pixel values) — the
	// downstream contoursToOutline converts to float32 pixels.
	for i := range result.Points {
		result.Points[i] = ContourPoint{
			X:       int16(points.pts[i].x),
			Y:       int16(points.pts[i].y),
			OnCurve: contours.Points[i].OnCurve,
		}
	}

	return result, metrics
}

// contoursToOutline converts hinted contour points (in pixel space) into a
// GlyphOutline suitable for rendering. The contour points are in Y-UP
// convention (as returned by autoHintContourPoints) and must be converted
// to Y-DOWN for the rendering pipeline.
//
// This reconstructs the same sequence of MoveTo/LineTo/QuadTo segments that
// the TrueType outline specification implies:
//   - On-curve points produce LineTo segments
//   - Off-curve points produce QuadTo segments (with implied on-curve midpoints
//     between consecutive off-curve points, per TrueType spec)
//   - The first point of each contour produces a MoveTo
//
// References:
//   - TrueType glyf table spec: consecutive off-curve points imply an on-curve
//     midpoint between them.
//   - FreeType FT_Outline_Decompose (ftoutln.c) — canonical outline decomposition
func contoursToOutline(contours *GlyfContours) *GlyphOutline {
	if contours == nil || len(contours.Points) == 0 {
		return &GlyphOutline{}
	}

	outline := &GlyphOutline{
		Segments: make([]OutlineSegment, 0, len(contours.Points)),
		Type:     GlyphTypeOutline,
	}

	numContours := len(contours.EndPts)
	start := 0

	for ci := 0; ci < numContours; ci++ {
		end := int(contours.EndPts[ci])
		if end >= len(contours.Points) {
			break
		}
		n := end - start + 1
		if n < 2 {
			start = end + 1
			continue
		}

		pts := contours.Points[start : end+1]
		decomposeContour(outline, pts)
		start = end + 1
	}

	// Compute bounds from the generated segments.
	refreshOutlineBounds(outline)

	return outline
}

// decomposeContour converts a single TrueType contour (raw points with
// on-curve/off-curve flags) into MoveTo/LineTo/QuadTo outline segments.
//
// The input contour points are in Y-UP convention (from the auto-hinter).
// The Y axis is negated during conversion to match the rendering pipeline's
// Y-DOWN convention (where positive Y goes downward on screen).
//
// TrueType implicit midpoint rule: when two consecutive off-curve points
// appear, an implicit on-curve point is inserted at their midpoint.
// This is how TrueType represents smooth curves with fewer points than
// cubic Beziers.
func decomposeContour(outline *GlyphOutline, pts []ContourPoint) {
	n := len(pts)
	if n == 0 {
		return
	}

	// Find the first on-curve point to start the contour.
	// If all points are off-curve, compute the implicit start.
	firstOnIdx := -1
	for i, p := range pts {
		if p.OnCurve {
			firstOnIdx = i
			break
		}
	}

	var startX, startY float32
	var startIdx int

	// contourPtX converts a 26.6 fixed-point X to float32 pixels.
	contourPtX := func(x int16) float32 { return float32(x) / 64.0 }
	// contourPtY converts a 26.6 fixed-point Y-UP to rendering Y-DOWN pixels.
	contourPtY := func(y int16) float32 { return -float32(y) / 64.0 }

	if firstOnIdx >= 0 {
		startX = contourPtX(pts[firstOnIdx].X)
		startY = contourPtY(pts[firstOnIdx].Y)
		startIdx = firstOnIdx
	} else {
		// All off-curve: start at midpoint of first and last.
		startX = float32(int32(pts[0].X)+int32(pts[n-1].X)) / 2.0 / 64.0
		startY = contourPtY((pts[0].Y + pts[n-1].Y) / 2)
		startIdx = 0
	}

	// Emit MoveTo.
	outline.Segments = append(outline.Segments, OutlineSegment{
		Op:     OutlineOpMoveTo,
		Points: [3]OutlinePoint{{X: startX, Y: startY}},
	})

	// Walk through points starting after the start point.
	i := (startIdx + 1) % n
	for steps := 0; steps < n; steps++ {
		curr := &pts[i]
		nextI := (i + 1) % n

		if curr.OnCurve {
			// On-curve: emit LineTo.
			outline.Segments = append(outline.Segments, OutlineSegment{
				Op:     OutlineOpLineTo,
				Points: [3]OutlinePoint{{X: contourPtX(curr.X), Y: contourPtY(curr.Y)}},
			})
		} else {
			// Off-curve: emit QuadTo with resolved endpoint.
			endX, endY, advExtra := resolveQuadEndpoint(pts, i, nextI, contourPtX, contourPtY)
			outline.Segments = append(outline.Segments, OutlineSegment{
				Op: OutlineOpQuadTo,
				Points: [3]OutlinePoint{
					{X: contourPtX(curr.X), Y: contourPtY(curr.Y)}, // control point
					{X: endX, Y: endY}, // on-curve endpoint
				},
			})
			if advExtra {
				i = nextI
				steps++
			}
		}

		i = (i + 1) % n
		// If we've reached the start point, we're done.
		if i == (startIdx+1)%n && steps > 0 {
			break
		}
	}

	// Close the contour: emit a LineTo back to the start if needed.
	lastSeg := &outline.Segments[len(outline.Segments)-1]
	lastPtCount := segPointCount(lastSeg.Op)
	lastPt := lastSeg.Points[lastPtCount-1]
	if lastPt.X != startX || lastPt.Y != startY {
		outline.Segments = append(outline.Segments, OutlineSegment{
			Op:     OutlineOpLineTo,
			Points: [3]OutlinePoint{{X: startX, Y: startY}},
		})
	}
}

// resolveQuadEndpoint determines the on-curve endpoint for a QuadTo segment
// when the current point is off-curve. If the next point is on-curve, it
// becomes the endpoint directly. If the next point is also off-curve, the
// TrueType implicit midpoint rule is applied.
// Returns (endX, endY, advanceExtra) where advanceExtra indicates that the
// caller should skip the next point (because it was consumed as the endpoint).
func resolveQuadEndpoint(pts []ContourPoint, currI int, nextI int, xConvert, yConvert func(int16) float32) (float32, float32, bool) {
	curr := &pts[currI]
	next := &pts[nextI]

	if next.OnCurve {
		return xConvert(next.X), yConvert(next.Y), true
	}
	// Next is also off-curve: implicit midpoint (compute in 26.6, then convert).
	midX := float32(int32(curr.X)+int32(next.X)) / 2.0 / 64.0
	midY := yConvert((curr.Y + next.Y) / 2)
	return midX, midY, false
}

// hintDimension identifies the axis for hinting.
type hintDimension int

const (
	// dimHorizontal analyzes vertical stems (X-axis features).
	// Segment position = X, segment extent = Y range.
	dimHorizontal hintDimension = iota

	// dimVertical analyzes horizontal stems (Y-axis features).
	// Segment position = Y, segment extent = X range.
	dimVertical
)

// autoHintMetricsKey is the cache key for auto-hint metrics.
type autoHintMetricsKey struct {
	fontName   string
	unitsPerEm int
}

// autoHintCache caches computed metrics per font.
var autoHintCache struct {
	mu    sync.RWMutex
	cache map[autoHintMetricsKey]*unscaledStyleMetrics
}

func init() {
	autoHintCache.cache = make(map[autoHintMetricsKey]*unscaledStyleMetrics)
}

// getAutoHintMetrics returns cached auto-hint metrics for a font,
// computing them if necessary. Thread-safe.
func getAutoHintMetrics(font ParsedFont) *unscaledStyleMetrics {
	key := autoHintMetricsKey{
		fontName:   font.FullName(),
		unitsPerEm: font.UnitsPerEm(),
	}

	// Fast path: read lock.
	autoHintCache.mu.RLock()
	m, ok := autoHintCache.cache[key]
	autoHintCache.mu.RUnlock()
	if ok {
		return m
	}

	// Slow path: compute and cache.
	autoHintCache.mu.Lock()
	defer autoHintCache.mu.Unlock()

	// Double-check after acquiring write lock.
	if m, ok := autoHintCache.cache[key]; ok {
		return m
	}

	m = computeUnscaledMetrics(font)
	autoHintCache.cache[key] = m
	return m
}

// ClearAutoHintCache clears the auto-hinter metrics cache.
// This should be called when fonts are unloaded or when font data changes
// (e.g., variable font axis values change).
func ClearAutoHintCache() {
	autoHintCache.mu.Lock()
	autoHintCache.cache = make(map[autoHintMetricsKey]*unscaledStyleMetrics)
	autoHintCache.mu.Unlock()
}

// unscaledStyleMetrics holds per-font unscaled metrics.
// This is computed once per font and cached.
type unscaledStyleMetrics struct {
	axes  [2]unscaledAxisMetrics // [dimHorizontal, dimVertical]
	group scriptGroup            // script group (Default, CJK, Indic)
}

// unscaledAxisMetrics holds unscaled metrics for one axis.
type unscaledAxisMetrics struct {
	widths            []int32    // standard stem widths in font units
	standardWidth     int32      // primary stem width (first in widths)
	edgeDistThreshold int32      // edge distance grouping threshold
	blues             []blueZone // blue zones (vertical axis only)
}

// scaledStyleMetrics holds per-size scaled metrics.
type scaledStyleMetrics struct {
	axes  [2]scaledAxisMetrics
	scale float64 // ppem / unitsPerEm
}

// scaledAxisMetrics holds scaled metrics for one axis.
type scaledAxisMetrics struct {
	widths            []scaledWidth
	standardWidth     int32   // standard width in font units
	maxWidth          int32   // maximum width in font units (for segment linking)
	edgeDistThreshold float32 // in font units (for edge grouping threshold)
	scale             float64 // ppem / unitsPerEm
	scale16dot16      int32   // scale as 16.16 fixed-point
	unitsPerEm        int     // font UPM (for derived constants in segment linking)
	blues             []scaledBlue
	isExtraLight      bool
	majorDir          hintDirection // major direction for blue edge matching
}

// scaledWidth holds a width value in both scaled and fitted forms.
// Values are in 26.6 fixed-point (1 unit = 1/64 pixel).
type scaledWidth struct {
	scaled int32 // scaled to pixels, 26.6 fixed-point
	fitted int32 // grid-fitted, 26.6 fixed-point
}

// computeUnscaledMetrics computes the unscaled style metrics for a font.
// This detects the font's primary script, then uses script-specific
// reference characters for stem width and blue zone computation.
//
// Script detection: Hebrew > Cyrillic > Greek > Arabic > CJK > Latin (default).
func computeUnscaledMetrics(font ParsedFont) *unscaledStyleMetrics {
	m := &unscaledStyleMetrics{}
	upm := font.UnitsPerEm()
	if upm <= 0 {
		return m
	}

	// Detect the font's primary script.
	script := detectFontScript(font)
	m.group = script.group

	// Compute standard widths from script-specific reference glyph.
	m.axes[dimHorizontal] = computeStandardWidths(font, dimHorizontal, script)
	m.axes[dimVertical] = computeStandardWidths(font, dimVertical, script)

	// Compute blue zones using script-specific reference characters.
	// Blue zones are on the vertical axis for Default group scripts,
	// and potentially on both axes for CJK.
	m.axes[dimVertical].blues = computeBlueZones(font, script)

	return m
}

// scale returns scaled metrics for the given scale factor.
// Routes to CJK-specific scaling for CJK script group, which differs
// from Default in width and blue zone handling.
//
// See skrifa metrics/scale.rs scale_style_metrics.
func (m *unscaledStyleMetrics) scale(scaleFactor float64) *scaledStyleMetrics {
	sm := &scaledStyleMetrics{scale: scaleFactor}
	if m.group == scriptGroupCJK {
		for dim := range 2 {
			sm.axes[dim] = m.axes[dim].scaleToCJK(scaleFactor)
		}
		sm.axes[dimVertical].blues = scaleBlueZonesCJK(m.axes[dimVertical].blues, scaleFactor)
	} else {
		for dim := range 2 {
			sm.axes[dim] = m.axes[dim].scaleTo(scaleFactor)
		}
		// Scale blue zones with possible Y-scale adjustment.
		sm.axes[dimVertical].blues = scaleBlueZones(m.axes[dimVertical].blues, scaleFactor)
	}
	return sm
}

// scaleWithUPM returns scaled metrics, also storing UPM and major direction
// for segment linking and blue edge matching.
func (m *unscaledStyleMetrics) scaleWithUPM(scaleFactor float64, upm int) *scaledStyleMetrics {
	sm := m.scale(scaleFactor)
	for dim := range 2 {
		sm.axes[dim].unitsPerEm = upm
	}
	// Set major direction per axis. TrueType default orientation (None):
	//   - H-axis (vertical stems): major = dirUp
	//   - V-axis (horizontal stems): major = dirLeft
	// See skrifa topo/mod.rs:96-101 Axis::reset.
	sm.axes[dimHorizontal].majorDir = dirUp
	sm.axes[dimVertical].majorDir = dirLeft
	return sm
}

// scaleTo scales axis metrics to the given scale factor.
func (a *unscaledAxisMetrics) scaleTo(scale float64) scaledAxisMetrics {
	sa := scaledAxisMetrics{
		standardWidth:     a.standardWidth,
		edgeDistThreshold: float32(float64(a.edgeDistThreshold) * scale),
		scale:             scale,
		scale16dot16:      computeScale16dot16(scale),
	}

	// Set max width from unscaled widths.
	for _, w := range a.widths {
		if w > sa.maxWidth {
			sa.maxWidth = w
		}
	}

	// Scale widths to 26.6 fixed-point.
	// Matches skrifa: scaled = fixed_mul(width, axis_scale) where axis_scale
	// is in 16.16 and width is in font units (treated as 26.6 * 64 = integer).
	sa.widths = make([]scaledWidth, len(a.widths))
	for i, w := range a.widths {
		scaled := f26dot6FromFloat(float64(w) * scale)
		sa.widths[i] = scaledWidth{scaled: scaled, fitted: scaled}
	}

	// Extra light: standard width < 5/8 pixel = 40 in 26.6.
	if len(a.widths) > 0 {
		sa.isExtraLight = float64(a.standardWidth)*scale < 0.625
	}

	return sa
}

// scaleToCJK scales CJK axis metrics. Unlike Default scaling, CJK never
// computes scaled width values — they are always zeroed. Width metrics
// (standardWidth, edgeDistThreshold) are preserved for segment linking.
//
// See skrifa metrics/scale.rs scale_cjk_axis_metrics, line 326:
// "FreeType never seems to compute scaled width values."
func (a *unscaledAxisMetrics) scaleToCJK(scale float64) scaledAxisMetrics {
	sa := scaledAxisMetrics{
		standardWidth:     a.standardWidth,
		edgeDistThreshold: float32(float64(a.edgeDistThreshold) * scale),
		scale:             scale,
		scale16dot16:      computeScale16dot16(scale),
	}

	// Set max width from unscaled widths.
	for _, w := range a.widths {
		if w > sa.maxWidth {
			sa.maxWidth = w
		}
	}

	// CJK: zero all scaled widths. FreeType/skrifa never computes
	// scaled width values for CJK — they are always {0, 0}.
	sa.widths = make([]scaledWidth, len(a.widths))

	// CJK is never extra light.

	return sa
}

// refreshOutlineBounds recalculates the bounding box of an outline
// from its segments.
func refreshOutlineBounds(outline *GlyphOutline) {
	minX, minY := float64(1e10), float64(1e10)
	maxX, maxY := float64(-1e10), float64(-1e10)
	for i := range outline.Segments {
		seg := &outline.Segments[i]
		for j := range segPointCount(seg.Op) {
			updateBounds(seg.Points[j], &minX, &minY, &maxX, &maxY)
		}
	}
	if len(outline.Segments) > 0 {
		outline.Bounds = Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
	}
}

// pixRound rounds a pixel value to the nearest integer pixel (float32 version).
// Used by the legacy path. The contour path uses f26dot6Round instead.
func pixRound(x float32) float32 {
	return float32(math.Round(float64(x)))
}

// computeScale16dot16 computes a 16.16 fixed-point scale factor from a
// floating-point scale (ppem / unitsPerEm).
//
// Matches skrifa's Scale::new computation exactly:
//
//	scale = (Fixed::from_bits((size * 64.0) as i32) / Fixed::from_bits(units_per_em)).to_bits()
//
// which is: ((ppem * 64) << 16) / upm using integer division (truncation).
// We receive scale = ppem/upm, so we compute: int(scale * 64 * 65536) with
// truncation toward zero (not rounding) to match skrifa/FreeType.
func computeScale16dot16(scale float64) int32 {
	return int32(scale * 64 * 65536)
}

// derivedConstant computes a scaled constant from units_per_em.
// Matches FreeType's AF_LATIN_CONSTANT macro with the standard value of 50.
func derivedConstant(unitsPerEm int) int32 {
	return int32(50 * unitsPerEm / 2048)
}
