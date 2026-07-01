// Package text provides GPU text rendering infrastructure.
//
// This file implements the OpenType gvar (Glyph Variations) table parser.
// The gvar table stores per-glyph variation deltas that modify outline
// points for variable fonts based on the current design-space coordinates.
//
// Reference: skrifa (Google fontations)
//   - read-fonts/src/tables/gvar.rs (gvar table parser)
//   - read-fonts/src/tables/variations.rs (tuple variation format)
//   - skrifa/src/outline/glyf/deltas.rs (IUP + delta application)
//
// Spec: https://learn.microsoft.com/en-us/typography/opentype/spec/gvar
package text

import (
	"encoding/binary"
	"fmt"
)

// gvarTable holds parsed gvar (Glyph Variations) header data.
//
// Binary layout:
//
//	uint16  majorVersion (1)
//	uint16  minorVersion (0)
//	uint16  axisCount
//	uint16  sharedTupleCount
//	Offset32 sharedTuplesOffset
//	uint16  glyphCount
//	uint16  flags (bit 0: long offsets if set)
//	Offset32 glyphVariationDataArrayOffset
//	Offset[glyphCount+1] offsets (uint16 or uint32)
type gvarTable struct {
	axisCount    int
	sharedTuples [][]int16 // [tupleIdx][axisIdx] F2.14 peak coordinates
	glyphOffsets []uint32  // byte offset per glyph into variation data
	varData      []byte    // raw variation data blob
}

// gvarTupleState tracks the parsing state while iterating through
// tuple variation headers in per-glyph variation data.
type gvarTupleState struct {
	headerPos  int
	serDataPos int
	data       []byte
	serialized []byte

	// Shared point numbers (parsed once from serialized data header).
	sharedPointIndices []uint16
	sharedAll          bool
	hasSharedPoints    bool
}

// parseGvar parses a gvar table from raw bytes.
func parseGvar(data []byte) (*gvarTable, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("gvar: data too short: %d bytes (need 20)", len(data))
	}

	major := binary.BigEndian.Uint16(data[0:2])
	if major != 1 {
		return nil, fmt.Errorf("gvar: unsupported version %d (expected 1)", major)
	}

	axisCount := int(binary.BigEndian.Uint16(data[4:6]))
	sharedTupleCount := int(binary.BigEndian.Uint16(data[6:8]))
	sharedTuplesOffset := binary.BigEndian.Uint32(data[8:12])
	glyphCount := int(binary.BigEndian.Uint16(data[12:14]))
	flags := binary.BigEndian.Uint16(data[14:16])
	varDataOffset := binary.BigEndian.Uint32(data[16:20])

	glyphOffsets, err := parseGvarOffsets(data, glyphCount, flags)
	if err != nil {
		return nil, err
	}

	sharedTuples := parseGvarSharedTuples(data, sharedTupleCount, axisCount, sharedTuplesOffset)

	var varData []byte
	if int(varDataOffset) < len(data) {
		varData = data[varDataOffset:]
	}

	return &gvarTable{
		axisCount:    axisCount,
		sharedTuples: sharedTuples,
		glyphOffsets: glyphOffsets,
		varData:      varData,
	}, nil
}

// parseGvarOffsets parses glyph variation data offsets from the gvar header.
func parseGvarOffsets(data []byte, glyphCount int, flags uint16) ([]uint32, error) {
	longOffsets := (flags & 0x0001) != 0
	offsetsStart := 20
	numOffsets := glyphCount + 1

	glyphOffsets := make([]uint32, numOffsets)
	if longOffsets {
		needed := offsetsStart + numOffsets*4
		if len(data) < needed {
			return nil, fmt.Errorf("gvar: data too short for %d long offsets", numOffsets)
		}
		for i := range numOffsets {
			glyphOffsets[i] = binary.BigEndian.Uint32(data[offsetsStart+i*4:])
		}
	} else {
		needed := offsetsStart + numOffsets*2
		if len(data) < needed {
			return nil, fmt.Errorf("gvar: data too short for %d short offsets", numOffsets)
		}
		for i := range numOffsets {
			glyphOffsets[i] = uint32(binary.BigEndian.Uint16(data[offsetsStart+i*2:])) * 2
		}
	}
	return glyphOffsets, nil
}

// parseGvarSharedTuples parses the shared tuple array from the gvar table.
func parseGvarSharedTuples(data []byte, count, axisCount int, offset uint32) [][]int16 {
	if count == 0 || int(offset) >= len(data) {
		return nil
	}
	tuples := make([][]int16, count)
	tupleSize := axisCount * 2
	start := int(offset)
	for i := range count {
		off := start + i*tupleSize
		if off+tupleSize > len(data) {
			break
		}
		tuple := make([]int16, axisCount)
		for j := range axisCount {
			tuple[j] = int16(binary.BigEndian.Uint16(data[off+j*2:]))
		}
		tuples[i] = tuple
	}
	return tuples
}

// glyphVariationDeltas computes total point deltas for a simple glyph
// at the given normalized coordinates.
//
// numPoints is the number of outline points (WITHOUT phantom points).
// The returned dx/dy arrays have length numPoints+4 (4 phantom points).
// contourEnds are the end-point indices for each contour (for IUP).
// outlinePoints are the original unscaled points as [x, y] pairs.
//
// Returns nil, nil if the glyph has no variation data.
//
// Matches skrifa deltas.rs:simple_glyph + compute_deltas_for_glyph.
func (g *gvarTable) glyphVariationDeltas(
	glyphID uint16,
	coords []int16,
	numPoints int,
	contourEnds []uint16,
	outlinePoints [][2]int32,
) ([]int32, []int32) {
	data := g.glyphVarData(glyphID)
	if data == nil || len(coords) == 0 || len(data) < 4 {
		return nil, nil
	}

	totalPoints := numPoints + 4
	tupleVarCountRaw := binary.BigEndian.Uint16(data[0:2])
	serializedDataOffset := int(binary.BigEndian.Uint16(data[2:4]))
	tupleCount := int(tupleVarCountRaw & 0x0FFF)

	if tupleCount == 0 || serializedDataOffset > len(data) {
		return nil, nil
	}

	state := &gvarTupleState{
		headerPos:       4,
		data:            data,
		serialized:      data[serializedDataOffset:],
		hasSharedPoints: (tupleVarCountRaw & 0x8000) != 0,
	}

	if state.hasSharedPoints {
		var consumed int
		state.sharedPointIndices, consumed = unpackPointNumbers(state.serialized)
		state.sharedAll = (state.sharedPointIndices == nil)
		state.serDataPos = consumed
	}

	dx := make([]int32, totalPoints)
	dy := make([]int32, totalPoints)

	for range tupleCount {
		g.processTuple(state, coords, totalPoints, contourEnds, outlinePoints, dx, dy)
	}

	return dx, dy
}

// glyphVarData returns the raw per-glyph variation data, or nil if absent.
func (g *gvarTable) glyphVarData(glyphID uint16) []byte {
	if g == nil {
		return nil
	}
	gid := int(glyphID)
	if gid+1 >= len(g.glyphOffsets) {
		return nil
	}
	startOff := g.glyphOffsets[gid]
	endOff := g.glyphOffsets[gid+1]
	if endOff <= startOff {
		return nil
	}
	dataLen := endOff - startOff
	if int(startOff)+int(dataLen) > len(g.varData) {
		return nil
	}
	return g.varData[startOff : startOff+dataLen]
}

// processTuple processes a single tuple variation, accumulating deltas.
func (g *gvarTable) processTuple(
	st *gvarTupleState,
	coords []int16,
	totalPoints int,
	contourEnds []uint16,
	outlinePoints [][2]int32,
	dx, dy []int32,
) {
	if st.headerPos+4 > len(st.data) {
		return
	}

	variationDataSize := int(binary.BigEndian.Uint16(st.data[st.headerPos:]))
	tupleIndexRaw := binary.BigEndian.Uint16(st.data[st.headerPos+2:])
	st.headerPos += 4

	peakTuple, ok := g.readPeakTuple(st, tupleIndexRaw)
	if !ok {
		st.serDataPos += variationDataSize
		return
	}

	interStart, interEnd := g.readIntermediateRegion(st, tupleIndexRaw)

	scalar := computeTupleScalar(peakTuple, coords, interStart, interEnd)
	if scalar == 0 {
		st.serDataPos += variationDataSize
		return
	}

	if st.serDataPos+variationDataSize > len(st.serialized) {
		return
	}
	tupleSerData := st.serialized[st.serDataPos : st.serDataPos+variationDataSize]

	pointIndices, allPoints, deltaDataStart := g.resolvePointIndices(st, tupleSerData, tupleIndexRaw)
	deltaData := tupleSerData[deltaDataStart:]

	if allPoints {
		accumulateDenseDeltas(deltaData, totalPoints, scalar, dx, dy)
	} else {
		accumulateSparseDeltas(deltaData, pointIndices, totalPoints, scalar, contourEnds, outlinePoints, dx, dy)
	}

	st.serDataPos += variationDataSize
}

// readPeakTuple reads the peak tuple from either the embedded data or shared tuples.
func (g *gvarTable) readPeakTuple(st *gvarTupleState, tupleIndexRaw uint16) ([]int16, bool) {
	embeddedPeak := (tupleIndexRaw & 0x8000) != 0

	if embeddedPeak {
		needed := g.axisCount * 2
		if st.headerPos+needed > len(st.data) {
			return nil, false
		}
		peak := make([]int16, g.axisCount)
		for j := range g.axisCount {
			peak[j] = int16(binary.BigEndian.Uint16(st.data[st.headerPos+j*2:]))
		}
		st.headerPos += needed
		return peak, true
	}

	sharedTupleIdx := int(tupleIndexRaw & 0x0FFF)
	if sharedTupleIdx < len(g.sharedTuples) {
		tuple := g.sharedTuples[sharedTupleIdx]
		// Matches skrifa check (variations.rs:1298): peak.len() must equal axisCount.
		if len(tuple) != g.axisCount {
			return nil, false
		}
		return tuple, true
	}
	return nil, false
}

// readIntermediateRegion reads optional intermediate start/end tuples.
func (g *gvarTable) readIntermediateRegion(st *gvarTupleState, tupleIndexRaw uint16) ([]int16, []int16) {
	if (tupleIndexRaw & 0x4000) == 0 {
		return nil, nil
	}
	needed := g.axisCount * 4
	if st.headerPos+needed > len(st.data) {
		return nil, nil
	}
	interStart := make([]int16, g.axisCount)
	interEnd := make([]int16, g.axisCount)
	for j := range g.axisCount {
		interStart[j] = int16(binary.BigEndian.Uint16(st.data[st.headerPos+j*2:]))
	}
	st.headerPos += g.axisCount * 2
	for j := range g.axisCount {
		interEnd[j] = int16(binary.BigEndian.Uint16(st.data[st.headerPos+j*2:]))
	}
	st.headerPos += g.axisCount * 2
	return interStart, interEnd
}

// resolvePointIndices determines which points have deltas in this tuple.
func (g *gvarTable) resolvePointIndices(
	st *gvarTupleState, tupleSerData []byte, tupleIndexRaw uint16,
) (pointIndices []uint16, allPoints bool, deltaDataStart int) {
	privatePointNumbers := (tupleIndexRaw & 0x2000) != 0

	switch {
	case privatePointNumbers:
		var consumed int
		pointIndices, consumed = unpackPointNumbers(tupleSerData)
		allPoints = (pointIndices == nil)
		deltaDataStart = consumed
	case st.hasSharedPoints:
		pointIndices = st.sharedPointIndices
		allPoints = st.sharedAll
	default:
		allPoints = true
	}
	return
}

// accumulateDenseDeltas unpacks and accumulates dense (all-point) deltas.
func accumulateDenseDeltas(deltaData []byte, totalPoints int, scalar int32, dx, dy []int32) {
	tupleDX, xConsumed := unpackDeltas(deltaData, totalPoints)
	tupleDY, _ := unpackDeltas(deltaData[xConsumed:], totalPoints)

	for j := range totalPoints {
		if j < len(tupleDX) {
			dx[j] += applyScalar(tupleDX[j], scalar)
		}
		if j < len(tupleDY) {
			dy[j] += applyScalar(tupleDY[j], scalar)
		}
	}
}

// accumulateSparseDeltas unpacks sparse deltas, applies IUP, and accumulates.
func accumulateSparseDeltas(
	deltaData []byte, pointIndices []uint16, totalPoints int, scalar int32,
	contourEnds []uint16, outlinePoints [][2]int32, dx, dy []int32,
) {
	nExplicit := len(pointIndices)
	sparseX, xConsumed := unpackDeltas(deltaData, nExplicit)
	sparseY, _ := unpackDeltas(deltaData[xConsumed:], nExplicit)

	scaledDX := make([]int32, nExplicit)
	scaledDY := make([]int32, nExplicit)
	for j := range nExplicit {
		if j < len(sparseX) {
			scaledDX[j] = applyScalar(sparseX[j], scalar)
		}
		if j < len(sparseY) {
			scaledDY[j] = applyScalar(sparseY[j], scalar)
		}
	}

	fullDX := gvarIUPInterpolate(scaledDX, pointIndices, totalPoints, contourEnds, outlinePoints, 0)
	fullDY := gvarIUPInterpolate(scaledDY, pointIndices, totalPoints, contourEnds, outlinePoints, 1)

	for j := range totalPoints {
		dx[j] += fullDX[j]
		dy[j] += fullDY[j]
	}
}

// computeTupleScalar computes the interpolation scalar for a tuple at
// the given normalized coordinates.
//
// Returns a Fixed 16.16 value where 0x10000 = 1.0, or 0 if inactive.
// Matches skrifa compute_scalar (variations.rs:1285-1341).
func computeTupleScalar(peak, coords []int16, interStart, interEnd []int16) int32 {
	scalar := int32(0x10000) // 1.0 in 16.16
	hasIntermediate := len(interStart) > 0 && len(interEnd) > 0

	for i, p := range peak {
		if p == 0 {
			continue
		}

		coord := int32(0)
		if i < len(coords) {
			coord = int32(coords[i])
		}
		if coord == 0 {
			return 0
		}

		pi := int32(p)
		if pi == coord {
			continue
		}

		scalar = computeAxisScalar(scalar, coord, pi, i, hasIntermediate, interStart, interEnd)
		if scalar == 0 {
			return 0
		}
	}

	return scalar
}

// computeAxisScalar computes the per-axis contribution to the tuple scalar.
func computeAxisScalar(scalar, coord, peak int32, axisIdx int, hasInter bool, interStart, interEnd []int16) int32 {
	if hasInter && axisIdx < len(interStart) && axisIdx < len(interEnd) {
		return computeAxisScalarIntermediate(scalar, coord, peak, int32(interStart[axisIdx]), int32(interEnd[axisIdx]))
	}
	// No intermediate: coord must be between 0 and peak.
	if coord < min(peak, 0) || coord > max(peak, 0) {
		return 0
	}
	return mulDiv(scalar, coord, peak)
}

// computeAxisScalarIntermediate handles the intermediate region case.
//
// Matches skrifa compute_scalar (variations.rs:1316-1330):
//
//	if coord <= start || coord >= end { return None }
//	if coord < peak { scalar *= (coord - start) / (peak - start) }
//	else            { scalar *= (end - coord)   / (end - peak)   }
func computeAxisScalarIntermediate(scalar, coord, peak, start, end int32) int32 {
	if coord <= start || coord >= end {
		return 0
	}
	if coord < peak {
		if peak == start {
			return scalar // avoid division by zero
		}
		return mulDiv(scalar, coord-start, peak-start)
	}
	if coord > peak {
		if end == peak {
			return scalar // avoid division by zero
		}
		return mulDiv(scalar, end-coord, end-peak)
	}
	// coord == peak
	return scalar
}

// applyScalar multiplies a delta by a Fixed 16.16 scalar, with rounding.
func applyScalar(delta, scalar int32) int32 {
	if scalar == 0x10000 {
		return delta
	}
	return int32((int64(delta)*int64(scalar) + 0x8000) >> 16)
}
