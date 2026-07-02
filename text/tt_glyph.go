// TrueType bytecode interpreter — per-glyph outline for hinting.
//
// Port of skrifa glyf/mod.rs (glyph loading with phantom points).
// Loads raw contour points, appends phantom points, scales to 26.6,
// and prepares the outline for the TT bytecode interpreter.
//
// Phantom points encode glyph metrics:
//
//	[0] left side bearing origin  (xMin - lsb, 0)
//	[1] advance width endpoint    (phantom[0].x + advance, 0)
//	[2] top side bearing          (0, yMax + tsb)
//	[3] vertical advance endpoint (0, phantom[2].y - vadvance)
//
// After hinting, the hinted advance = phantom[1].x - phantom[0].x.
//
// Reference: skrifa glyf/mod.rs:529-549 (setup_phantom_points)
// Reference: skrifa glyf/mod.rs:584-782 (load_simple)
// Reference: FreeType ttgload.c:1365 (phantom point computation)
package text

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ttGlyphOutline holds the per-glyph data needed for TT bytecode hinting.
// It includes the outline points (plus 4 phantom points), contour endpoints,
// glyph instructions, and scratch buffers for the interpreter.
//
// This matches the data passed to skrifa's HintOutline struct.
//
// Reference: skrifa hint/mod.rs:30-47 (HintOutline)
type ttGlyphOutline struct {
	// unscaled contains point coordinates in font units, stored as flat
	// (x, y) pairs: [x0, y0, x1, y1, ...]. Length = 2 * (numPoints + 4).
	unscaled []int32

	// original contains the scaled but unhinted points (snapshot before
	// hinting). Indexed by point number. Length = numPoints + 4.
	original [][2]int32

	// points contains the current hinted points. Modified by the
	// interpreter. Length = numPoints + 4.
	points [][2]int32

	// flags contains per-point flags (on-curve, touched-X, touched-Y).
	// Length = numPoints + 4.
	flags []ttPointFlags

	// contours contains end-of-contour point indices.
	contours []uint16

	// bytecode is the glyph's TT instruction bytecode.
	bytecode []byte

	// phantoms holds the 4 phantom point values. After hinting, these
	// encode the hinted metrics.
	phantoms [ttPhantomPointCount][2]int32

	// isComposite indicates if this is a composite glyph.
	isComposite bool

	// glyphID is the glyph ID for error reporting.
	glyphID uint16
}

// hintedAdvance returns the hinted horizontal advance from the phantom
// points in 26.6 fixed-point. This is computed as phantom[1].x - phantom[0].x.
//
// Reference: skrifa glyf/mod.rs ScaledOutline::advance_width
func (o *ttGlyphOutline) hintedAdvance() int32 {
	return o.phantoms[1][0] - o.phantoms[0][0]
}

// ttGlyphLoader loads and prepares glyph outlines for TT hinting.
// It holds the parsed font tables needed for glyph loading.
type ttGlyphLoader struct {
	font    *ttFontProgram // font-level program data
	tables  map[string][]byte
	glyfOff []glyfOffset // per-glyph offset+length in glyf table
	hmtxAdv []uint16     // horizontal advance widths from hmtx
	hmtxLSB []int16      // left side bearings from hmtx
	numHMtx int          // number of long horizontal metrics
}

// glyfOffset holds the offset and length of a glyph within the glyf table.
type glyfOffset struct {
	offset uint32
	length uint32
}

// newTTGlyphLoader creates a glyph loader from raw font data.
// Returns nil, nil if the font has no TrueType instructions.
//
//nolint:nilnil // nil result = "no TrueType data", not an error
func newTTGlyphLoader(fontData []byte, font *ttFontProgram) (*ttGlyphLoader, error) {
	if font == nil {
		return nil, nil
	}

	tables, err := parseFontTables(fontData)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph loader: %w", err)
	}

	// Parse loca table for glyph offsets.
	locaData, ok := tables["loca"]
	if !ok {
		return nil, errors.New("tt: glyph loader: missing loca table")
	}
	headData, ok := tables["head"]
	if !ok {
		return nil, errors.New("tt: glyph loader: missing head table")
	}
	if len(headData) < 54 {
		return nil, errors.New("tt: glyph loader: head table too short")
	}
	isLongLoca := binary.BigEndian.Uint16(headData[50:52]) != 0

	offsets, err := parseLocaOffsets(locaData, font.numGlyphs, isLongLoca)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph loader: %w", err)
	}

	// Parse hmtx table for advance widths and LSBs.
	hheaData, ok := tables["hhea"]
	if !ok {
		return nil, errors.New("tt: glyph loader: missing hhea table")
	}
	if len(hheaData) < 36 {
		return nil, errors.New("tt: glyph loader: hhea table too short")
	}
	numHMtx := int(binary.BigEndian.Uint16(hheaData[34:36]))
	if numHMtx == 0 {
		return nil, errors.New("tt: glyph loader: numHMtx is zero")
	}

	hmtxData, ok := tables["hmtx"]
	if !ok {
		return nil, errors.New("tt: glyph loader: missing hmtx table")
	}
	advances, lsbs, err := parseHmtx(hmtxData, numHMtx, font.numGlyphs)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph loader: %w", err)
	}

	return &ttGlyphLoader{
		font:    font,
		tables:  tables,
		glyfOff: offsets,
		hmtxAdv: advances,
		hmtxLSB: lsbs,
		numHMtx: numHMtx,
	}, nil
}

// loadGlyphOutline reads a glyph's contour points and instructions,
// appends phantom points, and scales everything to 26.6 fixed-point.
//
// For simple glyphs (numContours >= 1), parses contour points and instructions.
// For composite glyphs (numContours < 0), delegates to loadCompositeGlyphOutline
// which recursively loads components, applies transforms, and merges points.
// For empty glyphs (space, etc. with no glyf entry), returns a phantom-only
// outline with 0 contour points and 4 phantom points. The phantom points
// are scaled and rounded to the pixel grid, matching FreeType/skrifa behavior
// where hinted empty glyphs produce integer-pixel advances.
//
// The scale parameter is in 16.16 fixed-point (ppem * 64 * 65536 / upem).
//
// Reference: skrifa glyf/mod.rs:584-782 (load_simple)
// Reference: skrifa glyf/mod.rs:556-582 (load_empty — phantom-only scaling)
// Reference: skrifa glyf/mod.rs:784-960 (load_composite)
// Reference: FreeType ttgload.c:1555-1608 (empty glyph shortcut)
//
//nolint:nilnil // nil result = "no outline data", not an error
func (l *ttGlyphLoader) loadGlyphOutline(glyphID uint16, scale int32) (*ttGlyphOutline, error) {
	if int(glyphID) >= len(l.glyfOff) {
		return nil, fmt.Errorf("tt: glyph %d out of range (%d glyphs)", glyphID, len(l.glyfOff))
	}

	off := l.glyfOff[glyphID]
	if off.length == 0 {
		return l.loadEmptyGlyphOutline(glyphID, scale), nil
	}

	glyfData, ok := l.tables["glyf"]
	if !ok {
		return nil, errors.New("tt: missing glyf table")
	}

	end := off.offset + off.length
	if end > uint32(len(glyfData)) {
		return nil, fmt.Errorf("tt: glyph %d data out of bounds", glyphID)
	}

	data := glyfData[off.offset:end]
	if len(data) < 10 {
		return nil, nil // too short for glyph header
	}

	// Parse glyph header.
	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	if numContours < 0 {
		return l.loadCompositeGlyphOutline(glyphID, scale, data, 0)
	}
	if numContours == 0 {
		// Glyph exists in glyf but has no contours (rare — most empty glyphs
		// use off.length == 0 instead). Treat the same as empty glyph.
		return l.loadEmptyGlyphOutline(glyphID, scale), nil
	}

	xMin := int16(binary.BigEndian.Uint16(data[2:4]))
	yMin := int16(binary.BigEndian.Uint16(data[4:6]))
	// xMax not used directly (vertical phantom uses yMax).
	yMax := int16(binary.BigEndian.Uint16(data[8:10]))
	_ = yMin // used in vertical metrics if needed

	// Parse contour endpoints.
	nContours := int(numContours)
	if len(data) < 10+nContours*2 {
		return nil, fmt.Errorf("tt: glyph %d: truncated contour endpoints", glyphID)
	}
	contourEnds := make([]uint16, nContours)
	for i := range nContours {
		contourEnds[i] = binary.BigEndian.Uint16(data[10+i*2 : 12+i*2])
	}
	numPoints := int(contourEnds[nContours-1]) + 1

	// Skip instruction length and instructions.
	instrOff := 10 + nContours*2
	if instrOff+2 > len(data) {
		return nil, fmt.Errorf("tt: glyph %d: truncated instruction length", glyphID)
	}
	instrLen := int(binary.BigEndian.Uint16(data[instrOff : instrOff+2]))
	instrStart := instrOff + 2
	var instructions []byte
	if instrLen > 0 && instrStart+instrLen <= len(data) {
		instructions = data[instrStart : instrStart+instrLen]
	}

	// Parse point flags.
	flagStart := instrStart + instrLen
	flags, flagsEnd, err := parseGlyfFlags(data, flagStart, numPoints)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph %d: %w", glyphID, err)
	}

	// Parse X coordinates.
	xCoords, xEnd, err := parseGlyfCoords(data, flagsEnd, flags, numPoints, 0x02, 0x10)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph %d: x coords: %w", glyphID, err)
	}

	// Parse Y coordinates.
	yCoords, _, err := parseGlyfCoords(data, xEnd, flags, numPoints, 0x04, 0x20)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph %d: y coords: %w", glyphID, err)
	}

	// Get horizontal metrics.
	advance, lsb := l.glyphMetrics(glyphID)

	// Compute phantom points (in font units).
	// Matches FreeType / skrifa phantom point layout:
	//   [0] = (xMin - lsb, 0)             — horizontal origin
	//   [1] = (phantom[0].x + advance, 0)  — horizontal advance
	//   [2] = (0, yMax + tsb)              — vertical origin (= ascent)
	//   [3] = (0, phantom[2].y - vadvance) — vertical advance (= descent)
	// where tsb = ascent - yMax, vadvance = ascent - descent.
	// Reference: skrifa glyf/mod.rs:529-549 (setup_phantom_points)
	// Reference: FreeType ttgload.c:1365
	var phantomFU [ttPhantomPointCount][2]int32
	ascent := int32(l.font.os2Ascender)
	descent := int32(l.font.os2Descender)
	tsb := ascent - int32(yMax)
	vadvance := ascent - descent
	phantomFU[0] = [2]int32{int32(xMin) - int32(lsb), 0}
	phantomFU[1] = [2]int32{phantomFU[0][0] + int32(advance), 0}
	phantomFU[2] = [2]int32{0, int32(yMax) + tsb}          // = ascent
	phantomFU[3] = [2]int32{0, phantomFU[2][1] - vadvance} // = descent

	totalPoints := numPoints + ttPhantomPointCount

	// Build the outline.
	outline := &ttGlyphOutline{
		unscaled:    make([]int32, totalPoints*2),
		original:    make([][2]int32, totalPoints),
		points:      make([][2]int32, totalPoints),
		flags:       make([]ttPointFlags, totalPoints),
		contours:    contourEnds,
		bytecode:    instructions,
		isComposite: false,
		glyphID:     glyphID,
	}

	// Fill unscaled, original, points, and flags for contour points.
	for i := range numPoints {
		x := int32(xCoords[i])
		y := int32(yCoords[i])
		outline.unscaled[i*2] = x
		outline.unscaled[i*2+1] = y

		// Scale font units to 26.6 via rounded 16.16 multiply.
		// Matches skrifa Scale26Dot6::apply() which uses Fixed::mul (rounded).
		// Reference: skrifa glyf/mod.rs:399-401 (apply)
		// Reference: font-types/src/fixed.rs:189-192 (Fixed::mul)
		sx := ttMul16Dot16(x, scale)
		sy := ttMul16Dot16(y, scale)
		outline.original[i] = [2]int32{sx, sy}
		outline.points[i] = [2]int32{sx, sy}

		// Set on-curve flag from TrueType flags.
		if flags[i]&0x01 != 0 {
			outline.flags[i] = ttPointFlagOnCurve
		}
	}

	// Append phantom points.
	for j := range ttPhantomPointCount {
		idx := numPoints + j
		outline.unscaled[idx*2] = phantomFU[j][0]
		outline.unscaled[idx*2+1] = phantomFU[j][1]

		// Scale phantom points with rounded multiply (same as contour points).
		sx := ttMul16Dot16(phantomFU[j][0], scale)
		sy := ttMul16Dot16(phantomFU[j][1], scale)
		outline.original[idx] = [2]int32{sx, sy}
		outline.points[idx] = [2]int32{sx, sy}

		// Phantom points are off-curve; flags = 0 (already zero-initialized).

		// Round phantom points for hinting (FreeType pattern).
		// Reference: skrifa glyf/mod.rs:736-739
		outline.points[idx][0] = ttRound26Dot6(sx)
		outline.points[idx][1] = ttRound26Dot6(sy)
	}

	// Initialize phantom outputs from scaled (rounded) phantom points.
	for j := range ttPhantomPointCount {
		idx := numPoints + j
		outline.phantoms[j] = outline.points[idx]
	}

	return outline, nil
}

// loadCompositeGlyphOutline loads a composite glyph by recursively loading
// each component glyph, applying transforms/offsets, and merging the result
// into a single ttGlyphOutline with phantom points.
//
// This follows the skrifa load_composite pattern (glyf/mod.rs:784-960):
// for each component, load its glyph (recursively), apply the 2x2 transform
// and XY offset, then merge points and contour endpoints.
//
// Reference: skrifa glyf/mod.rs:784-960 (FreeTypeScaler::load_composite)
// Reference: FreeType ttgload.c:1259-1330 (composite glyph loading)
func (l *ttGlyphLoader) loadCompositeGlyphOutline(glyphID uint16, scale int32, data []byte, depth int) (*ttGlyphOutline, error) {
	if depth > compositeRecursionLimit {
		return nil, fmt.Errorf("tt: composite recursion limit exceeded for glyph %d", glyphID)
	}

	xMin := int16(binary.BigEndian.Uint16(data[2:4]))
	yMax := int16(binary.BigEndian.Uint16(data[8:10]))

	// Parse all components using shared parser.
	components, afterPos, err := parseCompositeComponents(data, 10)
	if err != nil {
		return nil, fmt.Errorf("tt: composite glyph %d: %w", glyphID, err)
	}

	// Collect all component points and contour endpoints in 26.6 scaled space.
	var allX, allY []int32
	var allOnCurve []bool
	var allContourEnds []uint16
	var compositeInstructions []byte

	for ci, comp := range components {
		// Recursively load the component glyph.
		componentOutline, compErr := l.loadGlyphOutlineRecursive(comp.glyphID, scale, depth+1)
		if compErr != nil {
			return nil, fmt.Errorf("tt: composite glyph %d component %d: %w", glyphID, comp.glyphID, compErr)
		}
		if componentOutline == nil {
			continue
		}

		// Extract component's contour points (exclude phantom points).
		numComponentPts := len(componentOutline.points) - ttPhantomPointCount
		if numComponentPts < 0 {
			numComponentPts = 0
		}

		baseIdx := uint16(len(allX))

		for i := range numComponentPts {
			// Work with scaled 26.6 values from the component.
			sx := componentOutline.points[i][0]
			sy := componentOutline.points[i][1]

			if comp.hasTransform {
				// Apply F2.14 transform in 26.6 space.
				// F2.14 * 26.6 → result needs >>14 to stay in 26.6.
				// Matches skrifa load_composite transform application.
				newSX := (int64(sx)*int64(comp.xx) + int64(sy)*int64(comp.xy) + (1 << 13)) >> 14
				newSY := (int64(sx)*int64(comp.yx) + int64(sy)*int64(comp.yy) + (1 << 13)) >> 14
				sx = int32(newSX)
				sy = int32(newSY)
			}

			// Apply offset scaled to 26.6.
			sx += ttMul16Dot16(comp.dx, scale)
			sy += ttMul16Dot16(comp.dy, scale)

			allX = append(allX, sx)
			allY = append(allY, sy)
			allOnCurve = append(allOnCurve, componentOutline.flags[i]&ttPointFlagOnCurve != 0)
		}

		// Shift contour endpoints.
		for _, endPt := range componentOutline.contours {
			allContourEnds = append(allContourEnds, endPt+baseIdx)
		}

		// After the last component, check for composite instructions.
		if ci == len(components)-1 && comp.flags&compositeHaveInstr != 0 && afterPos+2 <= len(data) {
			instrLen := int(binary.BigEndian.Uint16(data[afterPos : afterPos+2]))
			instrStart := afterPos + 2
			if instrLen > 0 && instrStart+instrLen <= len(data) {
				compositeInstructions = data[instrStart : instrStart+instrLen]
			}
		}
	}

	numPoints := len(allX)
	if numPoints == 0 {
		return l.loadEmptyGlyphOutline(glyphID, scale), nil
	}

	// Get horizontal metrics for phantom points.
	advance, lsb := l.glyphMetrics(glyphID)

	var phantomFU [ttPhantomPointCount][2]int32
	ascent := int32(l.font.os2Ascender)
	descent := int32(l.font.os2Descender)
	tsb := ascent - int32(yMax)
	vadvance := ascent - descent
	phantomFU[0] = [2]int32{int32(xMin) - int32(lsb), 0}
	phantomFU[1] = [2]int32{phantomFU[0][0] + int32(advance), 0}
	phantomFU[2] = [2]int32{0, int32(yMax) + tsb}
	phantomFU[3] = [2]int32{0, phantomFU[2][1] - vadvance}

	totalPoints := numPoints + ttPhantomPointCount

	outline := &ttGlyphOutline{
		unscaled:    make([]int32, totalPoints*2),
		original:    make([][2]int32, totalPoints),
		points:      make([][2]int32, totalPoints),
		flags:       make([]ttPointFlags, totalPoints),
		contours:    allContourEnds,
		bytecode:    compositeInstructions,
		isComposite: true,
		glyphID:     glyphID,
	}

	// Fill merged component points. Since we already have them in 26.6,
	// we store the unscaled as font units (reverse scale) for unscaled array,
	// and the 26.6 values in points/original.
	for i := range numPoints {
		// For composites, store unscaled as best-effort inverse from 26.6 back
		// to font units. This is approximate but sufficient for the interpreter's
		// unscaledToPixels() which returns 1.0 for composites anyway (skrifa pattern).
		outline.unscaled[i*2] = allX[i]
		outline.unscaled[i*2+1] = allY[i]

		outline.original[i] = [2]int32{allX[i], allY[i]}
		outline.points[i] = [2]int32{allX[i], allY[i]}

		if allOnCurve[i] {
			outline.flags[i] = ttPointFlagOnCurve
		}
	}

	// Append phantom points.
	for j := range ttPhantomPointCount {
		idx := numPoints + j
		outline.unscaled[idx*2] = phantomFU[j][0]
		outline.unscaled[idx*2+1] = phantomFU[j][1]

		sx := ttMul16Dot16(phantomFU[j][0], scale)
		sy := ttMul16Dot16(phantomFU[j][1], scale)
		outline.original[idx] = [2]int32{sx, sy}
		outline.points[idx] = [2]int32{ttRound26Dot6(sx), ttRound26Dot6(sy)}
	}

	for j := range ttPhantomPointCount {
		idx := numPoints + j
		outline.phantoms[j] = outline.points[idx]
	}

	return outline, nil
}

// loadGlyphOutlineRecursive loads a glyph outline, supporting recursion for
// composite glyph components. This is the internal recursive entry point.
//
//nolint:nilnil // nil result = "no outline", not an error
func (l *ttGlyphLoader) loadGlyphOutlineRecursive(glyphID uint16, scale int32, depth int) (*ttGlyphOutline, error) {
	if depth > compositeRecursionLimit {
		return nil, fmt.Errorf("tt: composite recursion limit exceeded for glyph %d", glyphID)
	}
	if int(glyphID) >= len(l.glyfOff) {
		return nil, fmt.Errorf("tt: glyph %d out of range (%d glyphs)", glyphID, len(l.glyfOff))
	}

	off := l.glyfOff[glyphID]
	if off.length == 0 {
		return l.loadEmptyGlyphOutline(glyphID, scale), nil
	}

	glyfData, ok := l.tables["glyf"]
	if !ok {
		return nil, errors.New("tt: missing glyf table")
	}

	end := off.offset + off.length
	if end > uint32(len(glyfData)) {
		return nil, fmt.Errorf("tt: glyph %d data out of bounds", glyphID)
	}

	data := glyfData[off.offset:end]
	if len(data) < 10 {
		return nil, nil
	}

	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	if numContours < 0 {
		return l.loadCompositeGlyphOutline(glyphID, scale, data, depth)
	}

	// Simple glyph — delegate to the main loader (which won't recurse since numContours >= 0).
	return l.loadGlyphOutline(glyphID, scale)
}

// loadGlyphOutlineVar loads a simple glyph's contour points with variation
// deltas applied, then scales to 26.6 fixed-point — exactly matching skrifa
// load_simple with gvar delta application (lines 647-706).
//
// This is the unified path for variable font TT hinting. The flow:
//  1. Parse raw contour points (font units) + phantom points
//  2. Apply gvar deltas to unscaled points (including phantoms)
//  3. Scale varied points to 26.6 (same formula as static path)
//  4. The returned outline is ready for TT bytecode hinting
//
// When variations is nil or empty, this produces identical output to
// loadGlyphOutline (no deltas applied).
//
// Reference: skrifa glyf/mod.rs:584-782 (load_simple with gvar)
//
//nolint:nilnil // nil result = "no simple outline", not an error
func (l *ttGlyphLoader) loadGlyphOutlineVar(
	glyphID uint16,
	scale int32,
	font *ownParsedFont,
	variations []FontVariation,
) (*ttGlyphOutline, error) {
	// If no variations, delegate to the static path.
	if len(variations) == 0 || font == nil {
		return l.loadGlyphOutline(glyphID, scale)
	}

	if int(glyphID) >= len(l.glyfOff) {
		return nil, fmt.Errorf("tt: glyph %d out of range (%d glyphs)", glyphID, len(l.glyfOff))
	}

	off := l.glyfOff[glyphID]
	if off.length == 0 {
		// Empty glyph with variations: apply gvar phantom deltas, then
		// scale and round. Matches skrifa load_empty (glyf/mod.rs:556-582).
		return l.loadEmptyGlyphOutlineVar(glyphID, scale, font, variations), nil
	}

	glyfData, ok := l.tables["glyf"]
	if !ok {
		return nil, errors.New("tt: missing glyf table")
	}

	end := off.offset + off.length
	if end > uint32(len(glyfData)) {
		return nil, fmt.Errorf("tt: glyph %d data out of bounds", glyphID)
	}

	data := glyfData[off.offset:end]
	if len(data) < 10 {
		return nil, nil // too short for glyph header
	}

	// Parse glyph header.
	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	if numContours < 0 {
		// Composite glyph with variations: load composite without gvar
		// (gvar for composites is applied to component offsets, which requires
		// a more complex path). The component glyphs themselves get their own
		// gvar deltas via recursive loadGlyphOutlineVar calls.
		return l.loadCompositeGlyphOutline(glyphID, scale, data, 0)
	}
	if numContours == 0 {
		// Glyph exists with header but no contours — same as empty.
		return l.loadEmptyGlyphOutlineVar(glyphID, scale, font, variations), nil
	}

	xMin := int16(binary.BigEndian.Uint16(data[2:4]))
	yMax := int16(binary.BigEndian.Uint16(data[8:10]))

	// Parse contour endpoints.
	nContours := int(numContours)
	if len(data) < 10+nContours*2 {
		return nil, fmt.Errorf("tt: glyph %d: truncated contour endpoints", glyphID)
	}
	contourEnds := make([]uint16, nContours)
	for i := range nContours {
		contourEnds[i] = binary.BigEndian.Uint16(data[10+i*2 : 12+i*2])
	}
	numPoints := int(contourEnds[nContours-1]) + 1

	// Skip instruction length and instructions.
	instrOff := 10 + nContours*2
	if instrOff+2 > len(data) {
		return nil, fmt.Errorf("tt: glyph %d: truncated instruction length", glyphID)
	}
	instrLen := int(binary.BigEndian.Uint16(data[instrOff : instrOff+2]))
	instrStart := instrOff + 2
	var instructions []byte
	if instrLen > 0 && instrStart+instrLen <= len(data) {
		instructions = data[instrStart : instrStart+instrLen]
	}

	// Parse point flags.
	flagStart := instrStart + instrLen
	flags, flagsEnd, err := parseGlyfFlags(data, flagStart, numPoints)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph %d: %w", glyphID, err)
	}

	// Parse X coordinates.
	xCoords, xEnd, err := parseGlyfCoords(data, flagsEnd, flags, numPoints, 0x02, 0x10)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph %d: x coords: %w", glyphID, err)
	}

	// Parse Y coordinates.
	yCoords, _, err := parseGlyfCoords(data, xEnd, flags, numPoints, 0x04, 0x20)
	if err != nil {
		return nil, fmt.Errorf("tt: glyph %d: y coords: %w", glyphID, err)
	}

	// Get horizontal metrics.
	advance, lsb := l.glyphMetrics(glyphID)

	// Compute phantom points (in font units).
	var phantomFU [ttPhantomPointCount][2]int32
	ascent := int32(l.font.os2Ascender)
	descent := int32(l.font.os2Descender)
	tsb := ascent - int32(yMax)
	vadvance := ascent - descent
	phantomFU[0] = [2]int32{int32(xMin) - int32(lsb), 0}
	phantomFU[1] = [2]int32{phantomFU[0][0] + int32(advance), 0}
	phantomFU[2] = [2]int32{0, int32(yMax) + tsb}
	phantomFU[3] = [2]int32{0, phantomFU[2][1] - vadvance}

	totalPoints := numPoints + ttPhantomPointCount

	// Build points array for gvar: [x, y] pairs including phantom points.
	// This matches the structure used by ownParsedFont.applyVariations.
	gvarPoints := make([][2]int32, totalPoints)
	for i := range numPoints {
		gvarPoints[i] = [2]int32{int32(xCoords[i]), int32(yCoords[i])}
	}
	for j := range ttPhantomPointCount {
		gvarPoints[numPoints+j] = phantomFU[j]
	}

	// Apply gvar deltas to unscaled points (including phantom points).
	// This is the critical step that skrifa does at lines 647-668:
	//   if gvar present && coords non-empty → compute and apply deltas.
	font.applyVariations(glyphID, gvarPoints, contourEnds, variations)

	// Build the outline from varied unscaled points.
	outline := &ttGlyphOutline{
		unscaled:    make([]int32, totalPoints*2),
		original:    make([][2]int32, totalPoints),
		points:      make([][2]int32, totalPoints),
		flags:       make([]ttPointFlags, totalPoints),
		contours:    contourEnds,
		bytecode:    instructions,
		isComposite: false,
		glyphID:     glyphID,
	}

	// Fill unscaled, original, points, and flags for contour points.
	// The unscaled array now contains gvar-varied font unit values.
	for i := range numPoints {
		x := gvarPoints[i][0]
		y := gvarPoints[i][1]
		outline.unscaled[i*2] = x
		outline.unscaled[i*2+1] = y

		// Scale varied font units to 26.6 (same formula as static path).
		sx := ttMul16Dot16(x, scale)
		sy := ttMul16Dot16(y, scale)
		outline.original[i] = [2]int32{sx, sy}
		outline.points[i] = [2]int32{sx, sy}

		// Set on-curve flag from TrueType flags.
		if flags[i]&0x01 != 0 {
			outline.flags[i] = ttPointFlagOnCurve
		}
	}

	// Append phantom points (now with gvar deltas applied).
	for j := range ttPhantomPointCount {
		idx := numPoints + j
		outline.unscaled[idx*2] = gvarPoints[idx][0]
		outline.unscaled[idx*2+1] = gvarPoints[idx][1]

		sx := ttMul16Dot16(gvarPoints[idx][0], scale)
		sy := ttMul16Dot16(gvarPoints[idx][1], scale)
		outline.original[idx] = [2]int32{sx, sy}
		outline.points[idx] = [2]int32{sx, sy}

		// Round phantom points for hinting (FreeType pattern).
		outline.points[idx][0] = ttRound26Dot6(sx)
		outline.points[idx][1] = ttRound26Dot6(sy)
	}

	// Initialize phantom outputs from scaled (rounded) phantom points.
	for j := range ttPhantomPointCount {
		idx := numPoints + j
		outline.phantoms[j] = outline.points[idx]
	}

	return outline, nil
}

// loadEmptyGlyphOutline creates a phantom-only outline for empty glyphs
// (space, etc.). Empty glyphs have no contour points but still need hinted
// advances — the phantom points encode the advance width.
//
// When TT hinting is active, phantom points are rounded to the pixel grid,
// producing integer-pixel advances. This matches FreeType and skrifa behavior:
//   - FreeType ttgload.c:1555-1608: scales phantom points for empty glyphs
//   - skrifa glyf/mod.rs:556-582 (load_empty): scales phantom points
//   - skrifa glyf/mod.rs:762-773: rounds phantoms when hinting active
//
// For empty glyphs, bounds are [0,0,0,0], so phantom[0] = (0-lsb, 0) and
// phantom[1] = (phantom[0]+advance, 0). After scaling+rounding, the advance
// = phantom[1].x - phantom[0].x is an integer pixel count.
func (l *ttGlyphLoader) loadEmptyGlyphOutline(glyphID uint16, scale int32) *ttGlyphOutline {
	// Get horizontal metrics.
	advance, lsb := l.glyphMetrics(glyphID)

	// Compute phantom points in font units.
	// For empty glyphs, bounds = [0,0,0,0] (no outline data).
	// Reference: skrifa glyf/mod.rs:289-291 (None => bounds = [0;4])
	// Reference: FreeType ttgload.c:1261-1262
	var phantomFU [ttPhantomPointCount][2]int32
	ascent := int32(l.font.os2Ascender)
	descent := int32(l.font.os2Descender)
	tsb := ascent // tsb = ascent - yMax, yMax=0 for empty glyph
	vadvance := ascent - descent
	phantomFU[0] = [2]int32{-int32(lsb), 0}                  // xMin(0) - lsb
	phantomFU[1] = [2]int32{-int32(lsb) + int32(advance), 0} // phantom[0].x + advance
	phantomFU[2] = [2]int32{0, tsb}                          // yMax(0) + tsb = ascent
	phantomFU[3] = [2]int32{0, tsb - vadvance}               // phantom[2].y - vadvance = descent

	totalPoints := ttPhantomPointCount // 0 contour points + 4 phantoms

	outline := &ttGlyphOutline{
		unscaled:    make([]int32, totalPoints*2),
		original:    make([][2]int32, totalPoints),
		points:      make([][2]int32, totalPoints),
		flags:       make([]ttPointFlags, totalPoints),
		contours:    nil, // no contours
		bytecode:    nil, // no instructions
		isComposite: false,
		glyphID:     glyphID,
	}

	// Scale phantom points and round to pixel grid.
	// Rounding matches skrifa load_simple lines 736-738 and lines 762-773:
	// when hinting is active, phantom points are rounded regardless of
	// whether instructions exist.
	for j := range ttPhantomPointCount {
		outline.unscaled[j*2] = phantomFU[j][0]
		outline.unscaled[j*2+1] = phantomFU[j][1]

		sx := ttMul16Dot16(phantomFU[j][0], scale)
		sy := ttMul16Dot16(phantomFU[j][1], scale)
		outline.original[j] = [2]int32{sx, sy}

		// Round to pixel grid (same as non-empty glyph phantom rounding).
		outline.points[j] = [2]int32{ttRound26Dot6(sx), ttRound26Dot6(sy)}
	}

	// Initialize phantom outputs.
	for j := range ttPhantomPointCount {
		outline.phantoms[j] = outline.points[j]
	}

	return outline
}

// loadEmptyGlyphOutlineVar creates a phantom-only outline for empty glyphs
// in variable fonts. This applies gvar phantom point deltas before scaling.
//
// Reference: skrifa glyf/mod.rs:556-582 (FreeTypeScaler::load_empty with gvar)
// Reference: FreeType ttgload.c:1563-1589 (empty glyph with GX_VAR_SUPPORT)
func (l *ttGlyphLoader) loadEmptyGlyphOutlineVar(
	glyphID uint16,
	scale int32,
	font *ownParsedFont,
	variations []FontVariation,
) *ttGlyphOutline {
	// Get horizontal metrics.
	advance, lsb := l.glyphMetrics(glyphID)

	// Compute phantom points in font units (bounds = [0,0,0,0]).
	var phantomFU [ttPhantomPointCount][2]int32
	ascent := int32(l.font.os2Ascender)
	descent := int32(l.font.os2Descender)
	tsb := ascent
	vadvance := ascent - descent
	phantomFU[0] = [2]int32{-int32(lsb), 0}
	phantomFU[1] = [2]int32{-int32(lsb) + int32(advance), 0}
	phantomFU[2] = [2]int32{0, tsb}
	phantomFU[3] = [2]int32{0, tsb - vadvance}

	// Build gvar point array: 0 contour points + 4 phantom points.
	gvarPoints := make([][2]int32, ttPhantomPointCount)
	for j := range ttPhantomPointCount {
		gvarPoints[j] = phantomFU[j]
	}

	// Apply gvar deltas to phantom points.
	// FreeType ttgload.c:1584: TT_Vary_Apply_Glyph_Deltas with outline.n_points=0
	// skrifa glyf/mod.rs:561-570: phantom_point_deltas for empty glyph
	if font != nil {
		font.applyVariations(glyphID, gvarPoints, nil, variations)
	}

	totalPoints := ttPhantomPointCount

	outline := &ttGlyphOutline{
		unscaled:    make([]int32, totalPoints*2),
		original:    make([][2]int32, totalPoints),
		points:      make([][2]int32, totalPoints),
		flags:       make([]ttPointFlags, totalPoints),
		contours:    nil,
		bytecode:    nil,
		isComposite: false,
		glyphID:     glyphID,
	}

	// Scale varied phantom points and round to pixel grid.
	for j := range ttPhantomPointCount {
		outline.unscaled[j*2] = gvarPoints[j][0]
		outline.unscaled[j*2+1] = gvarPoints[j][1]

		sx := ttMul16Dot16(gvarPoints[j][0], scale)
		sy := ttMul16Dot16(gvarPoints[j][1], scale)
		outline.original[j] = [2]int32{sx, sy}
		outline.points[j] = [2]int32{ttRound26Dot6(sx), ttRound26Dot6(sy)}
	}

	for j := range ttPhantomPointCount {
		outline.phantoms[j] = outline.points[j]
	}

	return outline
}

// glyphMetrics returns the advance width and left side bearing for a glyph.
// Matches hmtx table semantics: glyphs beyond numHMtx use the last advance.
func (l *ttGlyphLoader) glyphMetrics(glyphID uint16) (advance uint16, lsb int16) {
	gid := int(glyphID)
	if gid < l.numHMtx {
		return l.hmtxAdv[gid], l.hmtxLSB[gid]
	}
	// Beyond numHMtx: use last advance, LSB from monospace array.
	lastAdv := l.hmtxAdv[l.numHMtx-1]
	if gid < len(l.hmtxLSB) {
		return lastAdv, l.hmtxLSB[gid]
	}
	return lastAdv, 0
}

// parseLocaOffsets parses the loca table into per-glyph offsets.
func parseLocaOffsets(data []byte, numGlyphs int, isLong bool) ([]glyfOffset, error) {
	offsets := make([]glyfOffset, numGlyphs)
	if isLong {
		// Long format: uint32 offsets.
		need := (numGlyphs + 1) * 4
		if len(data) < need {
			return nil, errors.New("loca table too short (long)")
		}
		for i := range numGlyphs {
			start := binary.BigEndian.Uint32(data[i*4 : i*4+4])
			end := binary.BigEndian.Uint32(data[(i+1)*4 : (i+1)*4+4])
			if end > start {
				offsets[i] = glyfOffset{offset: start, length: end - start}
			}
		}
	} else {
		// Short format: uint16 offsets * 2.
		need := (numGlyphs + 1) * 2
		if len(data) < need {
			return nil, errors.New("loca table too short (short)")
		}
		for i := range numGlyphs {
			start := uint32(binary.BigEndian.Uint16(data[i*2:i*2+2])) * 2
			end := uint32(binary.BigEndian.Uint16(data[(i+1)*2:(i+1)*2+2])) * 2
			if end > start {
				offsets[i] = glyfOffset{offset: start, length: end - start}
			}
		}
	}
	return offsets, nil
}

// parseHmtx parses the hmtx table into advance widths and left side bearings.
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/hmtx
func parseHmtx(data []byte, numHMtx, numGlyphs int) ([]uint16, []int16, error) {
	longSize := numHMtx * 4
	if len(data) < longSize {
		return nil, nil, errors.New("hmtx table too short for long metrics")
	}

	advances := make([]uint16, numHMtx)
	lsbs := make([]int16, numGlyphs)

	// Parse long horizontal metrics (advance + lsb).
	for i := range numHMtx {
		advances[i] = binary.BigEndian.Uint16(data[i*4 : i*4+2])
		lsbs[i] = int16(binary.BigEndian.Uint16(data[i*4+2 : i*4+4]))
	}

	// Parse leftover LSBs for glyphs beyond numHMtx.
	remaining := numGlyphs - numHMtx
	if remaining > 0 {
		lsbStart := longSize
		need := lsbStart + remaining*2
		if len(data) < need {
			// Tolerate truncated hmtx — remaining LSBs default to 0.
			return advances, lsbs, nil
		}
		for i := range remaining {
			lsbs[numHMtx+i] = int16(binary.BigEndian.Uint16(data[lsbStart+i*2 : lsbStart+i*2+2]))
		}
	}

	return advances, lsbs, nil
}

// parseGlyfFlags parses the TrueType simple glyph point flags.
// Flags support repeat counts via bit 3 (0x08).
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#simple-glyph-description
func parseGlyfFlags(data []byte, offset, numPoints int) ([]byte, int, error) {
	flags := make([]byte, numPoints)
	pos := offset
	i := 0
	for i < numPoints {
		if pos >= len(data) {
			return nil, 0, errors.New("truncated flags")
		}
		f := data[pos]
		pos++
		flags[i] = f
		i++

		// Bit 3: repeat flag.
		if f&0x08 != 0 {
			if pos >= len(data) {
				return nil, 0, errors.New("truncated repeat count")
			}
			repeatCount := int(data[pos])
			pos++
			for j := 0; j < repeatCount && i < numPoints; j++ {
				flags[i] = f
				i++
			}
		}
	}
	return flags, pos, nil
}

// parseGlyfCoords parses TrueType glyph coordinates (X or Y).
// shortBit and sameBit select the appropriate flag bits for the axis.
//
// For X: shortBit = 0x02, sameBit = 0x10
// For Y: shortBit = 0x04, sameBit = 0x20
func parseGlyfCoords(data []byte, offset int, flags []byte, numPoints int, shortBit, sameBit byte) ([]int16, int, error) {
	coords := make([]int16, numPoints)
	pos := offset
	var prev int16
	for i := range numPoints {
		f := flags[i]
		if f&shortBit != 0 {
			// 1-byte unsigned delta.
			if pos >= len(data) {
				return nil, 0, errors.New("truncated coordinate")
			}
			delta := int16(data[pos])
			pos++
			if f&sameBit == 0 {
				delta = -delta // negative
			}
			prev += delta
		} else if f&sameBit == 0 {
			// 2-byte signed delta.
			if pos+1 >= len(data) {
				return nil, 0, errors.New("truncated coordinate")
			}
			delta := int16(binary.BigEndian.Uint16(data[pos : pos+2]))
			pos += 2
			prev += delta
		}
		// else: same as previous (delta = 0)
		coords[i] = prev
	}
	return coords, pos, nil
}
