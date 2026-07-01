// OpenType Layout shared structures — ScriptList, FeatureList, LookupList, Coverage.
//
// These structures are shared between GSUB and GPOS table parsers. They follow
// the OpenType specification binary layout:
//
//   - ScriptList → Script → LangSys → feature indices
//   - FeatureList → Feature → lookup indices
//   - LookupList → Lookup → subtables
//   - Coverage → glyph containment check (Format 1: list, Format 2: ranges)
//   - ClassDef → glyph class assignment (Format 1: array, Format 2: ranges)
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/chapter2
//
// This file is part of Phase 5 (ADR-048: Pure Go Font Stack).
package text

import (
	"encoding/binary"
	"sort"
)

// otLayoutHeader holds the parsed header of a GSUB or GPOS table.
// Both tables share the same header format (OpenType Layout Common Table Formats).
type otLayoutHeader struct {
	scriptListOffset  uint16
	featureListOffset uint16
	lookupListOffset  uint16
}

// parseOTLayoutHeader parses the shared GSUB/GPOS header.
// The table must start with: majorVersion(2) + minorVersion(2) + 3 offsets(6).
func parseOTLayoutHeader(data []byte) (otLayoutHeader, bool) {
	if len(data) < 10 {
		return otLayoutHeader{}, false
	}
	major := binary.BigEndian.Uint16(data[0:2])
	if major != 1 {
		return otLayoutHeader{}, false
	}
	return otLayoutHeader{
		scriptListOffset:  binary.BigEndian.Uint16(data[4:6]),
		featureListOffset: binary.BigEndian.Uint16(data[6:8]),
		lookupListOffset:  binary.BigEndian.Uint16(data[8:10]),
	}, true
}

// otLangSys represents a parsed LangSys table.
type otLangSys struct {
	requiredFeatureIndex uint16   // 0xFFFF if none
	featureIndices       []uint16 // indices into FeatureList
}

// otScript represents a parsed Script table with its default and language-specific LangSys.
type otScript struct {
	tag        [4]byte
	defaultLan *otLangSys             // may be nil
	langSys    map[[4]byte]*otLangSys // keyed by langSysTag
}

// parseScriptList parses the ScriptList starting at scriptListData.
func parseScriptList(data []byte) []otScript {
	if len(data) < 2 {
		return nil
	}
	scriptCount := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+scriptCount*6 {
		return nil
	}

	scripts := make([]otScript, 0, scriptCount)
	for i := range scriptCount {
		off := 2 + i*6
		var tag [4]byte
		copy(tag[:], data[off:off+4])
		scriptOffset := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
		if scriptOffset >= len(data) {
			continue
		}
		s := parseScript(data[scriptOffset:], tag)
		scripts = append(scripts, s)
	}
	return scripts
}

// parseScript parses a single Script table.
func parseScript(data []byte, tag [4]byte) otScript {
	s := otScript{tag: tag, langSys: make(map[[4]byte]*otLangSys)}
	if len(data) < 4 {
		return s
	}

	defaultOffset := int(binary.BigEndian.Uint16(data[0:2]))
	langSysCount := int(binary.BigEndian.Uint16(data[2:4]))

	if defaultOffset != 0 && defaultOffset < len(data) {
		ls := parseLangSys(data[defaultOffset:])
		s.defaultLan = &ls
	}

	if len(data) < 4+langSysCount*6 {
		return s
	}
	for i := range langSysCount {
		off := 4 + i*6
		var ltag [4]byte
		copy(ltag[:], data[off:off+4])
		lsOffset := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
		if lsOffset >= len(data) {
			continue
		}
		ls := parseLangSys(data[lsOffset:])
		s.langSys[ltag] = &ls
	}
	return s
}

// parseLangSys parses a LangSys table.
func parseLangSys(data []byte) otLangSys {
	if len(data) < 6 {
		return otLangSys{requiredFeatureIndex: 0xFFFF}
	}
	// Skip lookupOrder (2 bytes, reserved).
	reqIdx := binary.BigEndian.Uint16(data[2:4])
	featureCount := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+featureCount*2 {
		return otLangSys{requiredFeatureIndex: reqIdx}
	}
	indices := make([]uint16, featureCount)
	for i := range featureCount {
		indices[i] = binary.BigEndian.Uint16(data[6+i*2 : 6+i*2+2])
	}
	return otLangSys{
		requiredFeatureIndex: reqIdx,
		featureIndices:       indices,
	}
}

// otFeature represents a parsed feature with its tag and lookup indices.
type otFeature struct {
	tag           [4]byte
	lookupIndices []uint16
}

// parseFeatureList parses the FeatureList.
func parseFeatureList(data []byte) []otFeature {
	if len(data) < 2 {
		return nil
	}
	featureCount := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+featureCount*6 {
		return nil
	}

	features := make([]otFeature, 0, featureCount)
	for i := range featureCount {
		off := 2 + i*6
		var tag [4]byte
		copy(tag[:], data[off:off+4])
		featOffset := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
		if featOffset >= len(data) {
			continue
		}
		f := parseFeatureTable(data[featOffset:], tag)
		features = append(features, f)
	}
	return features
}

// parseFeatureTable parses a single Feature table.
func parseFeatureTable(data []byte, tag [4]byte) otFeature {
	f := otFeature{tag: tag}
	if len(data) < 4 {
		return f
	}
	// Skip featureParams offset (2 bytes).
	lookupCount := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+lookupCount*2 {
		return f
	}
	f.lookupIndices = make([]uint16, lookupCount)
	for i := range lookupCount {
		f.lookupIndices[i] = binary.BigEndian.Uint16(data[4+i*2 : 4+i*2+2])
	}
	return f
}

// otLookup represents a parsed Lookup table header.
// The subtable data slices are relative to the beginning of each subtable.
type otLookup struct {
	lookupType uint16
	lookupFlag uint16
	subtables  [][]byte // raw subtable data slices
}

// parseLookupList parses the LookupList and returns raw lookup entries.
func parseLookupList(data []byte) []otLookup {
	if len(data) < 2 {
		return nil
	}
	lookupCount := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+lookupCount*2 {
		return nil
	}

	lookups := make([]otLookup, 0, lookupCount)
	for i := range lookupCount {
		lookupOffset := int(binary.BigEndian.Uint16(data[2+i*2 : 2+i*2+2]))
		if lookupOffset >= len(data) {
			continue
		}
		lu := parseLookupTable(data[lookupOffset:])
		lookups = append(lookups, lu)
	}
	return lookups
}

// parseLookupTable parses a single Lookup table.
func parseLookupTable(data []byte) otLookup {
	lu := otLookup{}
	if len(data) < 6 {
		return lu
	}
	lu.lookupType = binary.BigEndian.Uint16(data[0:2])
	lu.lookupFlag = binary.BigEndian.Uint16(data[2:4])
	subtableCount := int(binary.BigEndian.Uint16(data[4:6]))

	headerSize := 6
	// If UseMarkFilteringSet flag (bit 4) is set, there is an extra uint16 after subtable offsets.
	// We skip it — it is used for mark filtering which is not yet implemented.

	if len(data) < headerSize+subtableCount*2 {
		return lu
	}

	lu.subtables = make([][]byte, 0, subtableCount)
	for i := range subtableCount {
		stOffset := int(binary.BigEndian.Uint16(data[headerSize+i*2 : headerSize+i*2+2]))
		if stOffset >= len(data) {
			continue
		}
		lu.subtables = append(lu.subtables, data[stOffset:])
	}
	return lu
}

// --- Coverage Tables ---

// otCoverage provides glyph containment check.
// Returns (coverage index, true) if the glyph is covered, (-1, false) otherwise.
type otCoverage interface {
	contains(glyphID uint16) (int, bool)
}

// parseCoverage parses a Coverage table from the given data slice.
func parseCoverage(data []byte) otCoverage {
	if len(data) < 4 {
		return nil
	}
	format := binary.BigEndian.Uint16(data[0:2])
	switch format {
	case 1:
		return parseCoverageFormat1(data)
	case 2:
		return parseCoverageFormat2(data)
	default:
		return nil
	}
}

// coverageFormat1 is a sorted glyph list.
type coverageFormat1 struct {
	glyphs []uint16 // sorted
}

func parseCoverageFormat1(data []byte) *coverageFormat1 {
	if len(data) < 4 {
		return nil
	}
	glyphCount := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+glyphCount*2 {
		return nil
	}
	c := &coverageFormat1{glyphs: make([]uint16, glyphCount)}
	for i := range glyphCount {
		c.glyphs[i] = binary.BigEndian.Uint16(data[4+i*2 : 4+i*2+2])
	}
	return c
}

func (c *coverageFormat1) contains(glyphID uint16) (int, bool) {
	idx := sort.Search(len(c.glyphs), func(i int) bool {
		return c.glyphs[i] >= glyphID
	})
	if idx < len(c.glyphs) && c.glyphs[idx] == glyphID {
		return idx, true
	}
	return -1, false
}

// coverageFormat2 is a set of glyph ranges.
type coverageFormat2 struct {
	ranges []coverageRange
}

type coverageRange struct {
	startGlyphID       uint16
	endGlyphID         uint16
	startCoverageIndex uint16
}

func parseCoverageFormat2(data []byte) *coverageFormat2 {
	if len(data) < 4 {
		return nil
	}
	rangeCount := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+rangeCount*6 {
		return nil
	}
	c := &coverageFormat2{ranges: make([]coverageRange, rangeCount)}
	for i := range rangeCount {
		off := 4 + i*6
		c.ranges[i] = coverageRange{
			startGlyphID:       binary.BigEndian.Uint16(data[off : off+2]),
			endGlyphID:         binary.BigEndian.Uint16(data[off+2 : off+4]),
			startCoverageIndex: binary.BigEndian.Uint16(data[off+4 : off+6]),
		}
	}
	return c
}

func (c *coverageFormat2) contains(glyphID uint16) (int, bool) {
	// Binary search on sorted ranges.
	idx := sort.Search(len(c.ranges), func(i int) bool {
		return c.ranges[i].endGlyphID >= glyphID
	})
	if idx < len(c.ranges) {
		r := c.ranges[idx]
		if glyphID >= r.startGlyphID && glyphID <= r.endGlyphID {
			return int(r.startCoverageIndex) + int(glyphID-r.startGlyphID), true
		}
	}
	return -1, false
}

// --- ClassDef Tables ---

// otClassDef assigns glyphs to classes.
// Glyphs not listed are in class 0.
type otClassDef interface {
	classOf(glyphID uint16) uint16
}

// parseClassDef parses a ClassDef table.
func parseClassDef(data []byte) otClassDef {
	if len(data) < 4 {
		return nil
	}
	format := binary.BigEndian.Uint16(data[0:2])
	switch format {
	case 1:
		return parseClassDefFormat1(data)
	case 2:
		return parseClassDefFormat2(data)
	default:
		return nil
	}
}

// classDefFormat1 assigns classes via a contiguous array starting at startGlyphID.
type classDefFormat1 struct {
	startGlyphID uint16
	classes      []uint16
}

func parseClassDefFormat1(data []byte) *classDefFormat1 {
	if len(data) < 6 {
		return nil
	}
	startGlyph := binary.BigEndian.Uint16(data[2:4])
	glyphCount := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+glyphCount*2 {
		return nil
	}
	c := &classDefFormat1{
		startGlyphID: startGlyph,
		classes:      make([]uint16, glyphCount),
	}
	for i := range glyphCount {
		c.classes[i] = binary.BigEndian.Uint16(data[6+i*2 : 6+i*2+2])
	}
	return c
}

func (c *classDefFormat1) classOf(glyphID uint16) uint16 {
	if glyphID < c.startGlyphID {
		return 0
	}
	idx := int(glyphID - c.startGlyphID)
	if idx >= len(c.classes) {
		return 0
	}
	return c.classes[idx]
}

// classDefFormat2 assigns classes via glyph ranges.
type classDefFormat2 struct {
	ranges []classDefRange
}

type classDefRange struct {
	startGlyphID uint16
	endGlyphID   uint16
	class        uint16
}

func parseClassDefFormat2(data []byte) *classDefFormat2 {
	if len(data) < 4 {
		return nil
	}
	rangeCount := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+rangeCount*6 {
		return nil
	}
	c := &classDefFormat2{ranges: make([]classDefRange, rangeCount)}
	for i := range rangeCount {
		off := 4 + i*6
		c.ranges[i] = classDefRange{
			startGlyphID: binary.BigEndian.Uint16(data[off : off+2]),
			endGlyphID:   binary.BigEndian.Uint16(data[off+2 : off+4]),
			class:        binary.BigEndian.Uint16(data[off+4 : off+6]),
		}
	}
	return c
}

func (c *classDefFormat2) classOf(glyphID uint16) uint16 {
	idx := sort.Search(len(c.ranges), func(i int) bool {
		return c.ranges[i].endGlyphID >= glyphID
	})
	if idx < len(c.ranges) {
		r := c.ranges[idx]
		if glyphID >= r.startGlyphID && glyphID <= r.endGlyphID {
			return r.class
		}
	}
	return 0
}

// --- ValueRecord ---

// otValueRecord holds GPOS positioning adjustments.
type otValueRecord struct {
	xPlacement int16
	yPlacement int16
	xAdvance   int16
	yAdvance   int16
}

// valueRecordSize returns the byte size of a ValueRecord given its valueFormat bits.
func valueRecordSize(valueFormat uint16) int {
	size := 0
	for i := range 8 {
		if valueFormat&(1<<uint(i)) != 0 {
			size += 2
		}
	}
	return size
}

// parseValueRecord reads a ValueRecord from data at the given offset.
// Returns the parsed record and the number of bytes consumed.
func parseValueRecord(data []byte, offset int, valueFormat uint16) (otValueRecord, int) {
	var vr otValueRecord
	pos := offset

	if valueFormat&0x0001 != 0 {
		if pos+2 > len(data) {
			return vr, pos - offset
		}
		vr.xPlacement = int16(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
	}
	if valueFormat&0x0002 != 0 {
		if pos+2 > len(data) {
			return vr, pos - offset
		}
		vr.yPlacement = int16(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
	}
	if valueFormat&0x0004 != 0 {
		if pos+2 > len(data) {
			return vr, pos - offset
		}
		vr.xAdvance = int16(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
	}
	if valueFormat&0x0008 != 0 {
		if pos+2 > len(data) {
			return vr, pos - offset
		}
		vr.yAdvance = int16(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
	}
	// Skip device table offsets (bits 4-7).
	for i := 4; i < 8; i++ {
		if valueFormat&(1<<uint(i)) != 0 {
			pos += 2
		}
	}
	return vr, pos - offset
}

// --- Feature lookup helpers ---

// collectLookupIndices returns the set of lookup indices for the given
// script, language, and desired feature tags. This implements the OpenType
// feature application algorithm: find the script, find the language,
// gather all feature indices from the LangSys, then filter by desired tags.
func collectLookupIndices(
	scripts []otScript,
	features []otFeature,
	scriptTag, langTag [4]byte,
	desiredTags [][4]byte,
) []uint16 {
	// Find matching script (fall back to DFLT).
	var langSys *otLangSys
	for i := range scripts {
		if scripts[i].tag == scriptTag {
			if lang, ok := scripts[i].langSys[langTag]; ok {
				langSys = lang
			} else {
				langSys = scripts[i].defaultLan
			}
			break
		}
	}
	// If script not found, try DFLT.
	if langSys == nil {
		dfltTag := [4]byte{'D', 'F', 'L', 'T'}
		for i := range scripts {
			if scripts[i].tag == dfltTag {
				langSys = scripts[i].defaultLan
				break
			}
		}
	}
	if langSys == nil {
		return nil
	}

	// Build set of desired tags for fast lookup.
	tagSet := make(map[[4]byte]bool, len(desiredTags))
	for _, t := range desiredTags {
		tagSet[t] = true
	}

	// Collect lookup indices from matching features.
	var lookupIndices []uint16
	for _, fi := range langSys.featureIndices {
		if int(fi) >= len(features) {
			continue
		}
		f := &features[fi]
		if tagSet[f.tag] {
			lookupIndices = append(lookupIndices, f.lookupIndices...)
		}
	}

	// Also check required feature.
	if langSys.requiredFeatureIndex != 0xFFFF {
		ri := langSys.requiredFeatureIndex
		if int(ri) < len(features) {
			lookupIndices = append(lookupIndices, features[ri].lookupIndices...)
		}
	}

	// Sort and deduplicate (OpenType spec requires lookups applied in order).
	sort.Slice(lookupIndices, func(i, j int) bool {
		return lookupIndices[i] < lookupIndices[j]
	})
	return dedup(lookupIndices)
}

// dedup removes consecutive duplicates from a sorted slice.
func dedup(s []uint16) []uint16 {
	if len(s) <= 1 {
		return s
	}
	j := 0
	for i := 1; i < len(s); i++ {
		if s[i] != s[j] {
			j++
			s[j] = s[i]
		}
	}
	return s[:j+1]
}
