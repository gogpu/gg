// GPOS table parser and glyph positioning engine.
//
// Implements OpenType GPOS (Glyph Positioning) table parsing and application.
// Supported lookup types:
//
//   - Type 1: Single adjustment (position a single glyph)
//   - Type 2: Pair adjustment (kerning: position two adjacent glyphs)
//   - Format 1: Specific pair list
//   - Format 2: Class-based pairs
//   - Type 9: Extension positioning (wrapper for above types in large fonts)
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/gpos
//
// This file is part of Phase 5 (ADR-048: Pure Go Font Stack).
package text

import "encoding/binary"

// gposTable holds parsed GPOS data ready for positioning.
type gposTable struct {
	scripts  []otScript
	features []otFeature
	lookups  []otLookup
}

// parseGPOS parses the raw GPOS table data.
func parseGPOS(data []byte) *gposTable {
	hdr, ok := parseOTLayoutHeader(data)
	if !ok {
		return nil
	}

	var g gposTable
	if int(hdr.scriptListOffset) < len(data) {
		g.scripts = parseScriptList(data[hdr.scriptListOffset:])
	}
	if int(hdr.featureListOffset) < len(data) {
		g.features = parseFeatureList(data[hdr.featureListOffset:])
	}
	if int(hdr.lookupListOffset) < len(data) {
		g.lookups = parseLookupList(data[hdr.lookupListOffset:])
	}
	return &g
}

// gposAdjustment holds the positioning adjustments for a glyph.
type gposAdjustment struct {
	xPlacement int16
	yPlacement int16
	xAdvance   int16
	yAdvance   int16
}

// applyGPOS applies GPOS positioning to a glyph buffer.
// Returns per-glyph adjustments in font units.
func (g *gposTable) applyGPOS(
	glyphs []shapingGlyph,
	scriptTag, langTag [4]byte,
	desiredTags [][4]byte,
) []gposAdjustment {
	adjustments := make([]gposAdjustment, len(glyphs))

	lookupIndices := collectLookupIndices(g.scripts, g.features, scriptTag, langTag, desiredTags)
	for _, li := range lookupIndices {
		if int(li) >= len(g.lookups) {
			continue
		}
		g.applyLookup(&g.lookups[li], glyphs, adjustments)
	}
	return adjustments
}

// applyLookup applies a single GPOS lookup.
func (g *gposTable) applyLookup(lu *otLookup, glyphs []shapingGlyph, adj []gposAdjustment) {
	lookupType := lu.lookupType

	// Extension positioning (Type 9) wraps another lookup type.
	if lookupType == 9 {
		g.applyExtensionLookup(lu, glyphs, adj)
		return
	}

	for _, st := range lu.subtables {
		g.applySubtable(lookupType, st, glyphs, adj)
	}
}

// applyExtensionLookup handles GPOS Lookup Type 9 (Extension Positioning).
func (g *gposTable) applyExtensionLookup(lu *otLookup, glyphs []shapingGlyph, adj []gposAdjustment) {
	for _, st := range lu.subtables {
		if len(st) < 8 {
			continue
		}
		format := binary.BigEndian.Uint16(st[0:2])
		if format != 1 {
			continue
		}
		extType := binary.BigEndian.Uint16(st[2:4])
		extOffset := binary.BigEndian.Uint32(st[4:8])
		if int(extOffset) >= len(st) {
			continue
		}
		g.applySubtable(extType, st[extOffset:], glyphs, adj)
	}
}

// applySubtable dispatches to the correct positioning function.
func (g *gposTable) applySubtable(lookupType uint16, data []byte, glyphs []shapingGlyph, adj []gposAdjustment) {
	switch lookupType {
	case 1:
		applySinglePos(data, glyphs, adj)
	case 2:
		applyPairPos(data, glyphs, adj)
	default:
		// Types 3-8 (cursive, mark-to-base, mark-to-ligature, mark-to-mark,
		// context, chaining context) not yet implemented.
	}
}

// --- Lookup Type 1: Single Adjustment ---

// applySinglePos applies single glyph position adjustment.
func applySinglePos(data []byte, glyphs []shapingGlyph, adj []gposAdjustment) {
	if len(data) < 6 {
		return
	}
	format := binary.BigEndian.Uint16(data[0:2])
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	if covOffset >= len(data) {
		return
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return
	}
	valueFormat := binary.BigEndian.Uint16(data[4:6])

	switch format {
	case 1:
		// Format 1: same ValueRecord for all covered glyphs.
		vr, _ := parseValueRecord(data, 6, valueFormat)
		for i := range glyphs {
			if _, ok := cov.contains(glyphs[i].gid); ok {
				adj[i].xPlacement += vr.xPlacement
				adj[i].yPlacement += vr.yPlacement
				adj[i].xAdvance += vr.xAdvance
				adj[i].yAdvance += vr.yAdvance
			}
		}
	case 2:
		// Format 2: array of ValueRecords, one per covered glyph.
		vrSize := valueRecordSize(valueFormat)
		valCount := int(binary.BigEndian.Uint16(data[6:8]))
		for i := range glyphs {
			idx, ok := cov.contains(glyphs[i].gid)
			if !ok || idx >= valCount {
				continue
			}
			vr, _ := parseValueRecord(data, 8+idx*vrSize, valueFormat)
			adj[i].xPlacement += vr.xPlacement
			adj[i].yPlacement += vr.yPlacement
			adj[i].xAdvance += vr.xAdvance
			adj[i].yAdvance += vr.yAdvance
		}
	}
}

// --- Lookup Type 2: Pair Adjustment (Kerning) ---

// applyPairPos applies pair positioning (kerning) to adjacent glyph pairs.
func applyPairPos(data []byte, glyphs []shapingGlyph, adj []gposAdjustment) {
	if len(data) < 2 {
		return
	}
	format := binary.BigEndian.Uint16(data[0:2])
	switch format {
	case 1:
		applyPairPosFormat1(data, glyphs, adj)
	case 2:
		applyPairPosFormat2(data, glyphs, adj)
	}
}

// applyPairPosFormat1 applies pair adjustment using specific glyph pairs.
func applyPairPosFormat1(data []byte, glyphs []shapingGlyph, adj []gposAdjustment) {
	if len(data) < 10 {
		return
	}
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	valueFormat1 := binary.BigEndian.Uint16(data[4:6])
	valueFormat2 := binary.BigEndian.Uint16(data[6:8])
	pairSetCount := int(binary.BigEndian.Uint16(data[8:10]))
	if len(data) < 10+pairSetCount*2 || covOffset >= len(data) {
		return
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return
	}
	vr1Size := valueRecordSize(valueFormat1)
	vr2Size := valueRecordSize(valueFormat2)

	for i := 0; i+1 < len(glyphs); i++ {
		idx, ok := cov.contains(glyphs[i].gid)
		if !ok || idx >= pairSetCount {
			continue
		}
		psOffset := int(binary.BigEndian.Uint16(data[10+idx*2 : 10+idx*2+2]))
		if psOffset >= len(data) {
			continue
		}
		ps := data[psOffset:]
		if len(ps) < 2 {
			continue
		}
		pairValueCount := int(binary.BigEndian.Uint16(ps[0:2]))
		recordSize := 2 + vr1Size + vr2Size // uint16 secondGlyph + vr1 + vr2

		secondGID := glyphs[i+1].gid
		// Binary search for secondGlyph in the PairSet.
		found := searchPairSet(ps[2:], pairValueCount, recordSize, secondGID)
		if found < 0 {
			continue
		}
		recordOff := 2 + found*recordSize

		// Parse value records.
		vr1, n1 := parseValueRecord(ps, recordOff+2, valueFormat1)
		adj[i].xPlacement += vr1.xPlacement
		adj[i].yPlacement += vr1.yPlacement
		adj[i].xAdvance += vr1.xAdvance
		adj[i].yAdvance += vr1.yAdvance

		if valueFormat2 != 0 {
			vr2, _ := parseValueRecord(ps, recordOff+2+n1, valueFormat2)
			adj[i+1].xPlacement += vr2.xPlacement
			adj[i+1].yPlacement += vr2.yPlacement
			adj[i+1].xAdvance += vr2.xAdvance
			adj[i+1].yAdvance += vr2.yAdvance
		}
	}
}

// searchPairSet binary-searches for secondGlyph in a PairSet's PairValueRecords.
// The records start at data[0] and each is recordSize bytes. The secondGlyph
// is the first uint16 in each record.
func searchPairSet(data []byte, count, recordSize int, secondGlyph uint16) int {
	lo, hi := 0, count-1
	for lo <= hi {
		mid := (lo + hi) / 2
		off := mid * recordSize
		if off+2 > len(data) {
			return -1
		}
		gid := binary.BigEndian.Uint16(data[off : off+2])
		if gid == secondGlyph {
			return mid
		}
		if gid < secondGlyph {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return -1
}

// applyPairPosFormat2 applies pair adjustment using glyph class pairs.
func applyPairPosFormat2(data []byte, glyphs []shapingGlyph, adj []gposAdjustment) {
	if len(data) < 16 {
		return
	}
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	valueFormat1 := binary.BigEndian.Uint16(data[4:6])
	valueFormat2 := binary.BigEndian.Uint16(data[6:8])
	classDef1Offset := int(binary.BigEndian.Uint16(data[8:10]))
	classDef2Offset := int(binary.BigEndian.Uint16(data[10:12]))
	class1Count := int(binary.BigEndian.Uint16(data[12:14]))
	class2Count := int(binary.BigEndian.Uint16(data[14:16]))

	if covOffset >= len(data) || classDef1Offset >= len(data) || classDef2Offset >= len(data) {
		return
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return
	}
	cd1 := parseClassDef(data[classDef1Offset:])
	cd2 := parseClassDef(data[classDef2Offset:])
	if cd1 == nil || cd2 == nil {
		return
	}

	vr1Size := valueRecordSize(valueFormat1)
	vr2Size := valueRecordSize(valueFormat2)
	recordSize := vr1Size + vr2Size
	arrayStart := 16

	for i := 0; i+1 < len(glyphs); i++ {
		if _, ok := cov.contains(glyphs[i].gid); !ok {
			continue
		}
		c1 := int(cd1.classOf(glyphs[i].gid))
		c2 := int(cd2.classOf(glyphs[i+1].gid))
		if c1 >= class1Count || c2 >= class2Count {
			continue
		}

		recordIndex := c1*class2Count + c2
		recordOff := arrayStart + recordIndex*recordSize

		vr1, n1 := parseValueRecord(data, recordOff, valueFormat1)
		adj[i].xPlacement += vr1.xPlacement
		adj[i].yPlacement += vr1.yPlacement
		adj[i].xAdvance += vr1.xAdvance
		adj[i].yAdvance += vr1.yAdvance

		if valueFormat2 != 0 {
			vr2, _ := parseValueRecord(data, recordOff+n1, valueFormat2)
			adj[i+1].xPlacement += vr2.xPlacement
			adj[i+1].yPlacement += vr2.yPlacement
			adj[i+1].xAdvance += vr2.xAdvance
			adj[i+1].yAdvance += vr2.yAdvance
		}
	}
}
