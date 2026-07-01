package text

import (
	"math"
	"sort"
	"strings"
)

// Blue zone detection and scaling for auto-hinting.
//
// Blue zones represent key vertical alignment features of a font:
//   - Capital top (T, H, E, Z, O, C, Q, S) — Latin
//   - Capital bottom (baseline) — Latin
//   - Small top (x-height: o, e, s, c) — Latin
//   - Hebrew top (ב ד ה ח ך כ ם ס) — Hebrew
//   - etc.
//
// Each zone has a reference position (flat features like "T" top) and
// an overshoot position (round features like "O" top). The auto-hinter
// snaps these to the pixel grid for crisp rendering.
//
// The algorithm is script-aware: detectFontScript() selects the script,
// then computeBlueZones() routes to computeDefaultBlues() or
// computeCJKBlues() based on the script group.
//
// For LONG-flagged zones (Hebrew), the algorithm uses raw TrueType contour
// points to detect long horizontal segments, avoiding being fooled by
// vertical serifs. This is ported from skrifa metrics/blues.rs
// compute_default_blues (lines 306-449).
//
// References:
//   - FreeType aflatin.c:311  af_latin_metrics_init_blues
//   - FreeType aflatin.c:641  long segment detection
//   - FreeType afblue.dat     blue zone reference characters
//   - skrifa metrics/blues.rs compute_unscaled_blues, compute_default_blues
//   - skrifa style.rs         ScriptGroup, BlueZones flags

// blueZone holds unscaled (font-unit) blue zone data.
// Position and overshoot are int32 (font units), matching skrifa's
// UnscaledBlue { position: i32, overshoot: i32 }.
type blueZone struct {
	position  int32         // flat feature median (font units)
	overshoot int32         // round feature median (font units)
	flags     blueZoneFlags // TOP, LONG, X_HEIGHT, ADJUSTMENT, etc.
}

// blueZoneFlags identifies special blue zone properties.
// Bit layout matches skrifa metrics/blues.rs BlueZones.
type blueZoneFlags uint32

const (
	blueZoneTop        blueZoneFlags = 1 << 1 // Top zone (matches skrifa TOP = 1 << 1)
	blueZoneSubTop     blueZoneFlags = 1 << 2 // Sub-top zone
	blueZoneNeutral    blueZoneFlags = 1 << 3 // Neutral zone (both directions)
	blueZoneAdjustment blueZoneFlags = 1 << 4 // Used for Y-scale adjustment (x-height)
	blueZoneXHeight    blueZoneFlags = 1 << 5 // X-height zone (implies ADJUSTMENT)
	blueZoneLong       blueZoneFlags = 1 << 6 // Long segment detection (Hebrew)
)

// isTopLike returns true if the zone is a top or sub-top zone.
func (f blueZoneFlags) isTopLike() bool {
	return f&(blueZoneTop|blueZoneSubTop) != 0
}

// scaledBlue holds scaled (pixel-space) blue zone data.
type scaledBlue struct {
	reference scaledWidth // reference position (scaled + fitted)
	overshoot scaledWidth // overshoot position (scaled + fitted)
	isActive  bool        // only zones < 3/4 pixel tall are active
	flags     blueZoneFlags

	// Unscaled positions in font units (for blue edge matching).
	// Skrifa compares edge.fpos (font units) to unscaled blue positions,
	// then scales the distance for threshold comparison.
	unscaledRef   int32
	unscaledShoot int32
}

// computeBlueZones detects blue zones for a font using script-aware
// character lists. Routes to computeDefaultBlues or computeCJKBlues
// based on the script group.
//
// See FreeType aflatin.c:311 af_latin_metrics_init_blues.
// See skrifa metrics/blues.rs compute_unscaled_blues.
func computeBlueZones(font ParsedFont, script *scriptClass) []blueZone {
	switch script.group {
	case scriptGroupCJK:
		return computeCJKBlues(font, script)
	case scriptGroupIndic:
		return nil // Indic scripts don't use blue values (yet).
	default:
		return computeDefaultBlues(font, script)
	}
}

// computeDefaultBlues computes blue zones for the Default script group
// (Latin, Hebrew, Arabic, Greek, Cyrillic, etc.).
//
// For each blue zone specification in the script, it loads the reference
// characters, finds their Y-extrema, classifies them as flat or round,
// and computes the median position for each category.
//
// For LONG-flagged zones (Hebrew), the algorithm uses raw contour points
// to detect long horizontal segments, avoiding being fooled by vertical
// serifs. The algorithm walks contour points to find segments near the
// extremum that exceed a length threshold (UPM/25).
//
// See FreeType aflatin.c:314-800 af_latin_metrics_init_blues.
// See skrifa metrics/blues.rs compute_default_blues.
func computeDefaultBlues(font ParsedFont, script *scriptClass) []blueZone {
	upm := font.UnitsPerEm()
	if upm == 0 {
		return nil
	}
	flatThreshold := int32(upm / 14)

	// Get raw font data for contour-based analysis.
	var rawFontData []byte
	if provider, ok := font.(RawFontDataProvider); ok {
		rawFontData = provider.RawFontData()
	}
	if rawFontData == nil {
		return nil
	}

	var zones []blueZone

	for _, spec := range script.blues {
		var flats, rounds []int32
		isTop := spec.flags.isTopLike()

		// Split the blue character string into individual characters.
		chars := strings.Fields(spec.chars)

		for _, ch := range chars {
			r := []rune(ch)
			if len(r) == 0 {
				continue
			}

			gid := font.GlyphIndex(r[0])
			if gid == 0 {
				continue
			}

			// Measure blue character using raw contour points.
			bestY, isRound, measured := measureBlueCharContour(rawFontData, GlyphID(gid), isTop, flatThreshold, int32(upm))
			if !measured {
				continue
			}
			if isRound {
				rounds = append(rounds, bestY)
			} else {
				flats = append(flats, bestY)
			}
		}

		if len(flats) == 0 && len(rounds) == 0 {
			continue
		}

		// Sort and compute medians.
		sort.Slice(flats, func(i, j int) bool { return flats[i] < flats[j] })
		sort.Slice(rounds, func(i, j int) bool { return rounds[i] < rounds[j] })

		blueRef, blueShoot := computeBlueMedians(flats, rounds)

		// Skrifa: if shoot != ref and the direction is unexpected,
		// collapse to average.
		if blueShoot != blueRef {
			overRef := blueShoot > blueRef
			if isTop != overRef {
				val := (blueShoot + blueRef) / 2
				blueRef = val
				blueShoot = val
			}
		}

		// Build zone flags: retain top-like and neutral, add ADJUSTMENT if X_HEIGHT.
		zoneFlags := spec.flags & (blueZoneTop | blueZoneSubTop | blueZoneNeutral)
		if spec.flags&blueZoneXHeight != 0 {
			zoneFlags |= blueZoneAdjustment
		}

		zones = append(zones, blueZone{
			position:  blueRef,
			overshoot: blueShoot,
			flags:     zoneFlags,
		})
	}

	// Adjust overlapping zones using index-based sort (skrifa pattern).
	// The zones remain in spec insertion order — only the overlap
	// adjustment uses sorted indices.
	adjustBlueZonesByIndex(zones)

	return zones
}

// computeBlueMedians computes the reference and overshoot median values
// from the flat and round extremum values.
func computeBlueMedians(flats, rounds []int32) (blueRef, blueShoot int32) {
	switch {
	case len(flats) == 0:
		val := rounds[len(rounds)/2]
		return val, val
	case len(rounds) == 0:
		val := flats[len(flats)/2]
		return val, val
	default:
		return flats[len(flats)/2], rounds[len(rounds)/2]
	}
}

// measureBlueCharContour measures a blue reference character using raw
// contour points. This implements the LONG segment detection algorithm
// from FreeType/skrifa for Hebrew-style blue zones.
//
// The algorithm:
//  1. Find the Y-extremum point and its contour
//  2. Walk backward/forward from the extremum to find the segment span
//  3. For LONG zones: if the segment is too short, search for adjacent
//     long segments to avoid being fooled by vertical serifs
//  4. Classify the segment as round (off-curve endpoints) or flat
//
// See skrifa metrics/blues.rs compute_default_blues, lines 200-470.
// See FreeType aflatin.c:641 long segment detection.
//
//nolint:gocognit,gocyclo,cyclop,funlen // FreeType/skrifa long segment detection — algorithmic complexity is inherent
func measureBlueCharContour(fontData []byte, gid GlyphID, isTop bool, flatThreshold, upm int32) (int32, bool, bool) {
	contours, err := ParseGlyfContours(fontData, gid)
	if err != nil || contours == nil || len(contours.Points) <= 2 {
		return 0, false, false
	}

	// Find the extremum point and its contour.
	bestContourIdx := -1
	bestPointIdx := -1
	bestYVal := int32(0)
	if isTop {
		bestYVal = math.MinInt32
	} else {
		bestYVal = math.MaxInt32
	}

	for ci := range contours.NumContours() {
		pts := contours.ContourPoints(ci)
		if pts == nil {
			continue
		}
		for pi, pt := range pts {
			y := int32(pt.Y) // raw glyf Y-up coordinates (font units)
			if isTop {
				if y > bestYVal {
					bestYVal = y
					bestContourIdx = ci
					bestPointIdx = pi
				}
			} else {
				if y < bestYVal {
					bestYVal = y
					bestContourIdx = ci
					bestPointIdx = pi
				}
			}
		}
	}

	if bestContourIdx < 0 {
		return 0, false, false
	}

	bestContour := contours.ContourPoints(bestContourIdx)
	n := len(bestContour)
	bestX := int32(bestContour[bestPointIdx].X)
	bestY := bestYVal

	// Walk backward from the extremum to find segment_first.
	// A point leaves the segment if Y-distance > 5 and X-distance <= 20*Y-distance.
	var onPointFirst, onPointLast int
	onPointFirstSet := false
	onPointLastSet := false

	if bestContour[bestPointIdx].OnCurve {
		onPointFirst = bestPointIdx
		onPointLast = bestPointIdx
		onPointFirstSet = true
		onPointLastSet = true
	}

	segmentFirst := bestPointIdx
	segmentLast := bestPointIdx

	// Walk backward.
	for steps := 1; steps < n; steps++ {
		ix := (bestPointIdx - steps + n) % n
		prev := &bestContour[ix]
		dist := int32(prev.Y) - bestY
		if dist < 0 {
			dist = -dist
		}
		xDist := int32(prev.X) - bestX
		if xDist < 0 {
			xDist = -xDist
		}
		if dist > 5 && xDist <= 20*dist {
			break
		}
		segmentFirst = ix
		if prev.OnCurve {
			onPointFirst = ix
			onPointFirstSet = true
			if !onPointLastSet {
				onPointLast = ix
				onPointLastSet = true
			}
		}
	}

	// Walk forward.
	nextIx := 0
	for steps := 1; steps < n; steps++ {
		ix := (bestPointIdx + steps) % n
		nextIx = ix
		next := &bestContour[ix]
		dist := int32(next.Y) - bestY
		if dist < 0 {
			dist = -dist
		}
		xDist := int32(next.X) - bestX
		if xDist < 0 {
			xDist = -xDist
		}
		if dist > 5 && xDist <= 20*dist {
			break
		}
		segmentLast = ix
		if next.OnCurve {
			onPointLast = ix
			onPointLastSet = true
			if !onPointFirstSet {
				onPointFirst = ix
				onPointFirstSet = true
			}
		}
	}

	// LONG segment detection (Hebrew).
	// If the initial segment at the extremum is short, search for a longer
	// adjacent segment to avoid being fooled by vertical serifs.
	// See skrifa metrics/blues.rs:306-449.
	// See FreeType aflatin.c:641.
	longResult := longSegmentDetection(bestContour, n, bestPointIdx, bestX, bestY, nextIx,
		segmentFirst, segmentLast, onPointFirst, onPointLast, onPointFirstSet, onPointLastSet, upm)
	bestY = longResult.bestY
	segmentFirst = longResult.segmentFirst
	segmentLast = longResult.segmentLast
	onPointFirst = longResult.onPointFirst
	onPointLast = longResult.onPointLast
	onPointFirstSet = longResult.onPointFirstSet
	onPointLastSet = longResult.onPointLastSet

	// Classify round vs flat.
	isRound := classifyRoundFlatContour(bestContour, onPointFirst, onPointLast,
		segmentFirst, segmentLast, onPointFirstSet, onPointLastSet, flatThreshold)

	return bestY, isRound, true
}

// longSegmentResult holds the output of the long segment detection algorithm.
type longSegmentResult struct {
	bestY           int32
	segmentFirst    int
	segmentLast     int
	onPointFirst    int
	onPointLast     int
	onPointFirstSet bool
	onPointLastSet  bool
}

// longSegmentDetection implements the LONG segment detection from skrifa/FreeType.
// Returns the (potentially updated) bestY value.
//
// When the initial segment at the extremum is shorter than UPM/25, the algorithm
// walks forward around the contour looking for a longer segment that:
//   - Goes in the same horizontal direction
//   - Has vertical distance from the extremum less than UPM/4
//   - Has horizontal length >= UPM/25
//
// This corrects Hebrew blue zones where vertical serifs produce misleading
// extremum points (e.g., Y=647 serif tip vs Y=592 main body top).
//
//nolint:gocognit,gocyclo,cyclop,funlen // FreeType aflatin.c:641 port — long segment detection logic
func longSegmentDetection(bestContour []ContourPoint, n int,
	bestPointIdx int, bestX, bestY int32, nextIx int,
	segmentFirst, segmentLast int,
	onPointFirst, onPointLast int,
	onPointFirstSet, onPointLastSet bool,
	upm int32) longSegmentResult {
	// Default result: no change.
	result := longSegmentResult{
		bestY: bestY, segmentFirst: segmentFirst, segmentLast: segmentLast,
		onPointFirst: onPointFirst, onPointLast: onPointLast,
		onPointFirstSet: onPointFirstSet, onPointLastSet: onPointLastSet,
	}

	lengthThreshold := upm / 25

	// Check if the current segment is long enough.
	dist := int32(bestContour[segmentLast].X) - int32(bestContour[segmentFirst].X)
	if dist < 0 {
		dist = -dist
	}
	if dist >= lengthThreshold {
		return result // Already long enough.
	}

	// Check satisfies_min_long_segment_len.
	contourLast := n - 1
	if !satisfiesMinLongSegmentLen(segmentFirst, segmentLast, contourLast) {
		return result
	}

	heightThreshold := upm / 4

	// Find previous point with different X value to determine direction.
	prevIx := bestPointIdx
	for steps := 1; steps < n; steps++ {
		ix := (bestPointIdx - steps + n) % n
		if int32(bestContour[ix].X) != bestX {
			prevIx = ix
			break
		}
	}
	if prevIx == bestPointIdx {
		return result // Degenerate case — skip.
	}

	isLTR := int32(bestContour[prevIx].X) < bestX

	// Search forward from segmentLast for a long segment.
	first := segmentLast
	last := first
	var pFirst, pLast int
	pFirstSet := false
	pLastSet := false
	hit := false

	for {
		if !hit {
			first = last
			if bestContour[first].OnCurve {
				pFirst = first
				pFirstSet = true
				pLast = first
				pLastSet = true
			} else {
				pFirstSet = false
				pLastSet = false
			}
			hit = true
		}

		last = (last + 1) % n

		// Check vertical distance from first to extremum.
		yDist := bestY - int32(bestContour[first].Y)
		if yDist < 0 {
			yDist = -yDist
		}
		if yDist > heightThreshold {
			hit = false
			if last == segmentFirst {
				break
			}
			continue
		}

		// Check if last deviates too much from first (angle check).
		dy := int32(bestContour[last].Y) - int32(bestContour[first].Y)
		if dy < 0 {
			dy = -dy
		}
		dx := int32(bestContour[last].X) - int32(bestContour[first].X)
		if dx < 0 {
			dx = -dx
		}
		if dy > 5 && dx <= 20*dy {
			hit = false
			if last == segmentFirst {
				break
			}
			continue
		}

		if bestContour[last].OnCurve {
			pLast = last
			pLastSet = true
			if !pFirstSet {
				pFirst = last
				pFirstSet = true
			}
		}

		firstX := int32(bestContour[first].X)
		lastX := int32(bestContour[last].X)
		isCurLTR := firstX < lastX
		segDist := lastX - firstX
		if segDist < 0 {
			segDist = -segDist
		}

		if isCurLTR == isLTR && segDist >= lengthThreshold {
			// Found a long segment! Extend it forward.
			extendLongSegment(bestContour, n, first, &last, nextIx, segmentFirst, dist,
				&pFirst, &pLast, &pFirstSet, &pLastSet)
			result.bestY = int32(bestContour[first].Y)
			result.segmentFirst = first
			result.segmentLast = last
			result.onPointFirst = pFirst
			result.onPointLast = pLast
			result.onPointFirstSet = pFirstSet
			result.onPointLastSet = pLastSet
			break
		}

		if last == segmentFirst {
			break
		}
	}

	return result
}

// extendLongSegment extends a found long segment forward, accumulating
// on-curve point information. This is the inner extension loop from
// skrifa metrics/blues.rs:404-436.
func extendLongSegment(contour []ContourPoint, n int,
	first int, last *int, nextIx, segmentFirst int, dist int32,
	pFirst, pLast *int, pFirstSet, pLastSet *bool) {
	for {
		*last = (*last + 1) % n
		extDY := int32(contour[*last].Y) - int32(contour[first].Y)
		if extDY < 0 {
			extDY = -extDY
		}
		extDX := int32(contour[nextIx].X) - int32(contour[first].X)
		if extDX < 0 {
			extDX = -extDX
		}
		if extDY > 5 && extDX <= 20*dist {
			// Step back.
			*last = (*last - 1 + n) % n
			break
		}
		if contour[*last].OnCurve {
			*pLast = *last
			*pLastSet = true
			if !*pFirstSet {
				*pFirst = *last
				*pFirstSet = true
			}
		}
		if *last == segmentFirst {
			break
		}
	}
}

// satisfiesMinLongSegmentLen checks if a segment has enough points
// to reliably detect bumps for LONG blue zone detection.
// Matches skrifa: inclusive_diff + 2 <= contour_last.
//
// See skrifa metrics/blues.rs:585-600.
// See FreeType aflatin.c:663.
func satisfiesMinLongSegmentLen(firstIdx, lastIdx, contourLast int) bool {
	var inclusiveDiff int
	if firstIdx <= lastIdx {
		inclusiveDiff = lastIdx - firstIdx
	} else {
		// Wraps around: [firstIdx, contourLast] + [0, lastIdx].
		inclusiveDiff = contourLast - firstIdx + 1 + lastIdx
	}
	return inclusiveDiff+2 <= contourLast
}

// classifyRoundFlatContour determines if the segment at the extremum
// is round (off-curve) or flat (straight line) using raw contour points.
// Matches skrifa's round/flat classification in compute_default_blues.
func classifyRoundFlatContour(contour []ContourPoint,
	onPointFirst, onPointLast int,
	segmentFirst, segmentLast int,
	onPointFirstSet, onPointLastSet bool,
	flatThreshold int32) bool {
	if onPointFirstSet && onPointLastSet {
		dx := int32(contour[onPointLast].X) - int32(contour[onPointFirst].X)
		if dx < 0 {
			dx = -dx
		}
		if dx > flatThreshold {
			return false // flat
		}
	}

	// If segment endpoints are off-curve, it's round.
	return !contour[segmentFirst].OnCurve || !contour[segmentLast].OnCurve
}

// computeCJKBlues computes blue zones for the CJK script group.
// CJK blues use a fill/flat character split (separated by '|' in the
// character string). Only vertical blues are active — horizontal blues
// have been disabled in FreeType since 2004.
//
// See skrifa metrics/blues.rs compute_cjk_blues.
// See FreeType afcjk.c:277.
//
//nolint:gocognit // FreeType afcjk.c port — CJK blue zone detection
func computeCJKBlues(font ParsedFont, script *scriptClass) []blueZone {
	upm := font.UnitsPerEm()
	if upm == 0 {
		return nil
	}

	// Get raw font data for contour-based measurement.
	var rawFontData []byte
	if provider, ok := font.(RawFontDataProvider); ok {
		rawFontData = provider.RawFontData()
	}
	if rawFontData == nil {
		return nil
	}

	var zones []blueZone

	for _, spec := range script.blues {
		isTop := spec.flags.isTopLike()

		// Split by '|' to get fills and flats.
		parts := strings.SplitN(spec.chars, "|", 2)
		fillChars := strings.Fields(strings.TrimSpace(parts[0]))
		var flatChars []string
		if len(parts) > 1 {
			flatChars = strings.Fields(strings.TrimSpace(parts[1]))
		}

		// Measure fills → position, flats → overshoot.
		var fills, flatsSlice []int32

		for _, ch := range fillChars {
			r := []rune(ch)
			if len(r) == 0 {
				continue
			}
			gid := font.GlyphIndex(r[0])
			if gid == 0 {
				continue
			}
			if bestY, ok := findBestYContour(rawFontData, GlyphID(gid), isTop); ok {
				fills = append(fills, bestY)
			}
		}

		for _, ch := range flatChars {
			r := []rune(ch)
			if len(r) == 0 {
				continue
			}
			gid := font.GlyphIndex(r[0])
			if gid == 0 {
				continue
			}
			if bestY, ok := findBestYContour(rawFontData, GlyphID(gid), isTop); ok {
				flatsSlice = append(flatsSlice, bestY)
			}
		}

		if len(fills) == 0 && len(flatsSlice) == 0 {
			continue
		}

		sort.Slice(fills, func(i, j int) bool { return fills[i] < fills[j] })
		sort.Slice(flatsSlice, func(i, j int) bool { return flatsSlice[i] < flatsSlice[j] })

		var blueRef, blueShoot int32
		if len(fills) > 0 {
			blueRef = fills[len(fills)/2]
		}
		if len(flatsSlice) > 0 {
			blueShoot = flatsSlice[len(flatsSlice)/2]
		}
		if len(fills) == 0 {
			blueRef = blueShoot
		}
		if len(flatsSlice) == 0 {
			blueShoot = blueRef
		}

		zoneFlags := spec.flags & (blueZoneTop | blueZoneSubTop | blueZoneNeutral)

		zones = append(zones, blueZone{
			position:  blueRef,
			overshoot: blueShoot,
			flags:     zoneFlags,
		})
	}

	return zones
}

// findBestYContour finds the Y-extremum from raw glyf contour points.
// This is the contour-based equivalent of findBestY (which uses sfnt segments).
// Coordinates are in Y-UP font units (raw glyf data, no conversion needed).
func findBestYContour(fontData []byte, gid GlyphID, isTop bool) (int32, bool) {
	contours, err := ParseGlyfContours(fontData, gid)
	if err != nil || contours == nil || len(contours.Points) == 0 {
		return 0, false
	}

	bestY := int32(0)
	hasPoints := false

	for _, pt := range contours.Points {
		if !pt.OnCurve {
			continue // Only on-curve points, matching findBestY behavior.
		}
		y := int32(pt.Y) // Y-UP font units (same as sfnt path after negation+div64)
		switch {
		case !hasPoints:
			bestY = y
			hasPoints = true
		case isTop && y > bestY:
			bestY = y
		case !isTop && y < bestY:
			bestY = y
		}
	}

	return bestY, hasPoints
}

// adjustBlueZonesByIndex adjusts overlapping blue zones using an index-based
// sort, keeping the zones in their original insertion (spec) order.
// This matches skrifa metrics/blues.rs:528-578 exactly:
//  1. Build sorted_indices array (sorted bottom-to-top by the relevant position)
//  2. Walk adjacent pairs in sorted order, clamping overlaps
//
// The zones are modified in-place but their ORDER is preserved (spec order).
func adjustBlueZonesByIndex(zones []blueZone) {
	n := len(zones)
	if n < 2 {
		return
	}

	// Build sorted indices (insertion sort for stability, matching skrifa).
	sortedIndices := make([]int, n)
	for i := range n {
		sortedIndices[i] = i
	}

	// blueZoneSortKey returns the value used for sorting a zone.
	blueZoneSortKey := func(z *blueZone) int32 {
		if z.flags.isTopLike() {
			return z.position
		}
		return z.overshoot
	}

	// Insertion sort (matches skrifa exactly).
	for i := 1; i < n; i++ {
		for j := i; j >= 1; j-- {
			a := blueZoneSortKey(&zones[sortedIndices[j-1]])
			b := blueZoneSortKey(&zones[sortedIndices[j]])
			if b >= a {
				break
			}
			sortedIndices[j-1], sortedIndices[j] = sortedIndices[j], sortedIndices[j-1]
		}
	}

	// Adjust overlapping tops: walk adjacent pairs in sorted order.
	for i := range n - 1 {
		idx1 := sortedIndices[i]
		idx2 := sortedIndices[i+1]

		var a, b int32
		if zones[idx1].flags.isTopLike() {
			a = zones[idx1].overshoot
		} else {
			a = zones[idx1].position
		}
		if zones[idx2].flags.isTopLike() {
			b = zones[idx2].overshoot
		} else {
			b = zones[idx2].position
		}

		if a > b {
			if zones[idx1].flags.isTopLike() {
				zones[idx1].overshoot = b
			} else {
				zones[idx1].position = b
			}
		}
	}
}

// scaleBlueZones scales blue zones to pixel coordinates and applies
// grid-fitting. Only zones with height < 3/4 pixel are activated.
//
// See FreeType aflatin.c:1168 and skrifa metrics/scale.rs.
func scaleBlueZones(zones []blueZone, scale float64) []scaledBlue {
	result := make([]scaledBlue, 0, len(zones))

	for _, z := range zones {
		// Scale to 26.6 fixed-point.
		scaledRef := f26dot6FromFloat(float64(z.position) * scale)
		scaledShoot := f26dot6FromFloat(float64(z.overshoot) * scale)

		blue := scaledBlue{
			reference:     scaledWidth{scaled: scaledRef, fitted: scaledRef},
			overshoot:     scaledWidth{scaled: scaledShoot, fitted: scaledShoot},
			flags:         z.flags,
			unscaledRef:   z.position,
			unscaledShoot: z.overshoot,
		}

		// Only activate zones where |ref - shoot| < 3/4 pixel = 48 in 26.6.
		dist := scaledRef - scaledShoot
		if dist < 0 {
			dist = -dist
		}

		if dist <= 48 { //nolint:nestif // FreeType aflatin.c port — algorithmic complexity is inherent
			// Discretize the overshoot delta in 26.6.
			var delta int32
			if dist < 32 { //nolint:gocritic // FreeType aflatin.c port — value range if-else chain // < 0.5px
				delta = 0
			} else if dist < 48 { // < 0.75px
				delta = 32 // 0.5px
			} else {
				delta = 64 // 1.0px
			}
			if scaledRef-scaledShoot < 0 {
				delta = -delta
			}

			blue.reference.fitted = f26dot6Round(scaledRef)
			blue.overshoot.fitted = blue.reference.fitted - delta
			blue.isActive = true
		}

		result = append(result, blue)
	}

	return result
}

// scaleBlueZonesCJK scales blue zones using the CJK-specific algorithm.
// Unlike Default scaling, CJK uses unscale-and-compare delta computation
// rather than the simple 3-level quantized delta.
//
// See skrifa metrics/scale.rs scale_cjk_axis_metrics, lines 289-323.
// See FreeType afcjk.c:661.
func scaleBlueZonesCJK(zones []blueZone, scale float64) []scaledBlue {
	result := make([]scaledBlue, 0, len(zones))
	// 16.16 fixed-point scale, matching skrifa's axis.scale.
	// Uses truncation (not rounding) to match skrifa/FreeType integer division.
	scale16dot16 := computeScale16dot16(scale)

	for _, z := range zones {
		// Scale to 26.6 fixed-point via 16.16 multiplication.
		scaledRef := fixedMul26dot6(z.position, scale16dot16)
		scaledShoot := fixedMul26dot6(z.overshoot, scale16dot16)

		blue := scaledBlue{
			reference:     scaledWidth{scaled: scaledRef, fitted: scaledRef},
			overshoot:     scaledWidth{scaled: scaledShoot, fitted: scaledShoot},
			flags:         z.flags,
			unscaledRef:   z.position,
			unscaledShoot: z.overshoot,
		}

		// Only activate zones where |ref - shoot| < 3/4 pixel = 48 in 26.6.
		dist := fixedMul26dot6(z.position-z.overshoot, scale16dot16)
		if dist >= -48 && dist <= 48 { //nolint:nestif // skrifa scale_cjk_axis_metrics port — algorithmic complexity is inherent
			blue.reference.fitted = f26dot6Round(scaledRef)

			// CJK delta: unscale the fitted position, compare to overshoot,
			// then re-scale the difference. This differs from Default which
			// uses a simple 3-level quantized delta.
			delta1 := fixedDiv26dot6(blue.reference.fitted, scale16dot16) - z.overshoot
			absDelta1 := delta1
			if absDelta1 < 0 {
				absDelta1 = -absDelta1
			}
			delta2 := fixedMul26dot6(absDelta1, scale16dot16)
			if delta2 < 32 {
				delta2 = 0
			} else {
				delta2 = f26dot6Round(delta2)
			}
			if delta1 < 0 {
				delta2 = -delta2
			}
			blue.overshoot.fitted = blue.reference.fitted - delta2
			blue.isActive = true
		}

		result = append(result, blue)
	}

	return result
}

// medianFloat32 returns the median of a float32 slice.
// Returns 0 if the slice is empty.
func medianFloat32(vals []float32) float32 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float32, len(vals))
	copy(sorted, vals)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// absF32 returns the absolute value of a float32.
func absF32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
