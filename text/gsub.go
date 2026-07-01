// GSUB table parser and glyph substitution engine.
//
// Implements OpenType GSUB (Glyph Substitution) table parsing and application.
// Supported lookup types:
//
//   - Type 1: Single substitution (one-to-one glyph replacement)
//   - Type 2: Multiple substitution (one-to-many expansion)
//   - Type 3: Alternate substitution (one-of-many selection)
//   - Type 4: Ligature substitution (many-to-one contraction, e.g. fi, ffi)
//   - Type 7: Extension substitution (wrapper for above types in large fonts)
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/gsub
//
// This file is part of Phase 5 (ADR-048: Pure Go Font Stack).
package text

import "encoding/binary"

// gsubTable holds parsed GSUB data ready for substitution.
type gsubTable struct {
	scripts  []otScript
	features []otFeature
	lookups  []otLookup
}

// parseGSUB parses the raw GSUB table data.
func parseGSUB(data []byte) *gsubTable {
	hdr, ok := parseOTLayoutHeader(data)
	if !ok {
		return nil
	}

	var g gsubTable
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

// applyGSUB applies GSUB substitutions to a glyph buffer.
// scriptTag and langTag select the language system. desiredTags selects
// which features to apply (e.g. "liga", "smcp").
//
// The glyph buffer is modified in-place (glyphs may be added or removed).
// Returns the resulting glyph buffer.
func (g *gsubTable) applyGSUB(
	glyphs []shapingGlyph,
	scriptTag, langTag [4]byte,
	desiredTags [][4]byte,
) []shapingGlyph {
	lookupIndices := collectLookupIndices(g.scripts, g.features, scriptTag, langTag, desiredTags)
	for _, li := range lookupIndices {
		if int(li) >= len(g.lookups) {
			continue
		}
		glyphs = g.applyLookup(&g.lookups[li], glyphs)
	}
	return glyphs
}

// shapingGlyph represents a glyph being shaped with its cluster index.
type shapingGlyph struct {
	gid     uint16
	cluster int // source character index
}

// applyLookup applies a single GSUB lookup to the glyph buffer.
func (g *gsubTable) applyLookup(lu *otLookup, glyphs []shapingGlyph) []shapingGlyph {
	lookupType := lu.lookupType

	// Extension substitution (Type 7) wraps another lookup type.
	if lookupType == 7 {
		return g.applyExtensionLookup(lu, glyphs)
	}

	for _, st := range lu.subtables {
		glyphs = g.applySubtable(lookupType, st, glyphs)
	}
	return glyphs
}

// applyExtensionLookup handles GSUB Lookup Type 7 (Extension Substitution).
// Each subtable contains a pointer to the actual substitution subtable.
func (g *gsubTable) applyExtensionLookup(lu *otLookup, glyphs []shapingGlyph) []shapingGlyph {
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
		glyphs = g.applySubtable(extType, st[extOffset:], glyphs)
	}
	return glyphs
}

// applySubtable dispatches to the correct substitution function.
func (g *gsubTable) applySubtable(lookupType uint16, data []byte, glyphs []shapingGlyph) []shapingGlyph {
	switch lookupType {
	case 1:
		return applySingleSubst(data, glyphs)
	case 2:
		return applyMultipleSubst(data, glyphs)
	case 3:
		return applyAlternateSubst(data, glyphs)
	case 4:
		return applyLigatureSubst(data, glyphs)
	default:
		// Types 5, 6, 8 (contextual, chaining, reverse) not yet implemented.
		return glyphs
	}
}

// --- Lookup Type 1: Single Substitution ---

// applySingleSubst applies single substitution (one glyph → one glyph).
func applySingleSubst(data []byte, glyphs []shapingGlyph) []shapingGlyph {
	if len(data) < 6 {
		return glyphs
	}
	format := binary.BigEndian.Uint16(data[0:2])
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	if covOffset >= len(data) {
		return glyphs
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return glyphs
	}

	switch format {
	case 1:
		// Format 1: add delta to covered glyphs.
		delta := int16(binary.BigEndian.Uint16(data[4:6]))
		for i := range glyphs {
			if _, ok := cov.contains(glyphs[i].gid); ok {
				glyphs[i].gid = uint16(int32(glyphs[i].gid) + int32(delta))
			}
		}
	case 2:
		// Format 2: substitute from array.
		glyphCount := int(binary.BigEndian.Uint16(data[4:6]))
		if len(data) < 6+glyphCount*2 {
			return glyphs
		}
		for i := range glyphs {
			idx, ok := cov.contains(glyphs[i].gid)
			if ok && idx < glyphCount {
				glyphs[i].gid = binary.BigEndian.Uint16(data[6+idx*2 : 6+idx*2+2])
			}
		}
	}
	return glyphs
}

// --- Lookup Type 2: Multiple Substitution ---

// applyMultipleSubst replaces one glyph with a sequence of glyphs.
func applyMultipleSubst(data []byte, glyphs []shapingGlyph) []shapingGlyph {
	if len(data) < 6 {
		return glyphs
	}
	format := binary.BigEndian.Uint16(data[0:2])
	if format != 1 {
		return glyphs
	}
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	if covOffset >= len(data) {
		return glyphs
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return glyphs
	}
	seqCount := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+seqCount*2 {
		return glyphs
	}

	// Process from right to left so index shifts do not affect earlier glyphs.
	for i := len(glyphs) - 1; i >= 0; i-- {
		idx, ok := cov.contains(glyphs[i].gid)
		if !ok || idx >= seqCount {
			continue
		}
		seqOffset := int(binary.BigEndian.Uint16(data[6+idx*2 : 6+idx*2+2]))
		if seqOffset >= len(data) {
			continue
		}
		seq := data[seqOffset:]
		if len(seq) < 2 {
			continue
		}
		subCount := int(binary.BigEndian.Uint16(seq[0:2]))
		if len(seq) < 2+subCount*2 || subCount == 0 {
			continue
		}

		cluster := glyphs[i].cluster
		replacement := make([]shapingGlyph, subCount)
		for j := range subCount {
			replacement[j] = shapingGlyph{
				gid:     binary.BigEndian.Uint16(seq[2+j*2 : 2+j*2+2]),
				cluster: cluster,
			}
		}

		// Replace glyphs[i] with replacement.
		glyphs = sliceReplace(glyphs, i, 1, replacement)
	}
	return glyphs
}

// --- Lookup Type 3: Alternate Substitution ---

// applyAlternateSubst replaces a glyph with an alternate form.
// Always selects the first alternate (index 0). Full alternate selection
// would require user-facing API to choose which alternate.
func applyAlternateSubst(data []byte, glyphs []shapingGlyph) []shapingGlyph {
	if len(data) < 6 {
		return glyphs
	}
	format := binary.BigEndian.Uint16(data[0:2])
	if format != 1 {
		return glyphs
	}
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	if covOffset >= len(data) {
		return glyphs
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return glyphs
	}
	altSetCount := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+altSetCount*2 {
		return glyphs
	}

	for i := range glyphs {
		idx, ok := cov.contains(glyphs[i].gid)
		if !ok || idx >= altSetCount {
			continue
		}
		altSetOffset := int(binary.BigEndian.Uint16(data[6+idx*2 : 6+idx*2+2]))
		if altSetOffset >= len(data) {
			continue
		}
		altSet := data[altSetOffset:]
		if len(altSet) < 4 {
			continue
		}
		altCount := int(binary.BigEndian.Uint16(altSet[0:2]))
		if altCount > 0 && len(altSet) >= 2+altCount*2 {
			// Select first alternate.
			glyphs[i].gid = binary.BigEndian.Uint16(altSet[2:4])
		}
	}
	return glyphs
}

// --- Lookup Type 4: Ligature Substitution ---

// applyLigatureSubst applies ligature substitution (many glyphs → one glyph).
// For example, 'f' + 'i' → 'fi' ligature glyph.
func applyLigatureSubst(data []byte, glyphs []shapingGlyph) []shapingGlyph {
	if len(data) < 6 {
		return glyphs
	}
	format := binary.BigEndian.Uint16(data[0:2])
	if format != 1 {
		return glyphs
	}
	covOffset := int(binary.BigEndian.Uint16(data[2:4]))
	if covOffset >= len(data) {
		return glyphs
	}
	cov := parseCoverage(data[covOffset:])
	if cov == nil {
		return glyphs
	}
	ligSetCount := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+ligSetCount*2 {
		return glyphs
	}

	// Process left-to-right; on successful match remove consumed glyphs
	// and advance past the ligature.
	i := 0
	for i < len(glyphs) {
		idx, ok := cov.contains(glyphs[i].gid)
		if !ok || idx >= ligSetCount {
			i++
			continue
		}
		ligSetOffset := int(binary.BigEndian.Uint16(data[6+idx*2 : 6+idx*2+2]))
		if ligSetOffset >= len(data) {
			i++
			continue
		}

		matched := tryLigatureSet(data[ligSetOffset:], glyphs, i)
		if matched > 0 {
			// Remove the consumed component glyphs (all except the first).
			glyphs = sliceReplace(glyphs, i+1, matched-1, nil)
			i++
		} else {
			i++
		}
	}
	return glyphs
}

// tryLigatureSet tries all ligatures in a LigatureSet for a match starting at glyphs[pos].
// Returns the number of glyphs consumed (including the first) on match, or 0 on no match.
// On match, glyphs[pos].gid is updated to the ligature glyph.
func tryLigatureSet(data []byte, glyphs []shapingGlyph, pos int) int {
	if len(data) < 2 {
		return 0
	}
	ligCount := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+ligCount*2 {
		return 0
	}

	for li := range ligCount {
		ligOffset := int(binary.BigEndian.Uint16(data[2+li*2 : 2+li*2+2]))
		if ligOffset >= len(data) {
			continue
		}
		ligData := data[ligOffset:]
		if len(ligData) < 4 {
			continue
		}
		ligGlyph := binary.BigEndian.Uint16(ligData[0:2])
		compCount := int(binary.BigEndian.Uint16(ligData[2:4]))
		if compCount < 2 {
			continue // ligature must have at least 2 components
		}
		numExtra := compCount - 1
		if len(ligData) < 4+numExtra*2 {
			continue
		}
		// Check remaining glyphs.
		if pos+compCount > len(glyphs) {
			continue
		}

		match := true
		for j := range numExtra {
			compGlyph := binary.BigEndian.Uint16(ligData[4+j*2 : 4+j*2+2])
			if glyphs[pos+1+j].gid != compGlyph {
				match = false
				break
			}
		}
		if match {
			glyphs[pos].gid = ligGlyph
			return compCount
		}
	}
	return 0
}

// --- Slice helpers ---

// sliceReplace replaces glyphs[pos:pos+removeCount] with replacement.
// If replacement is nil/empty, it is a pure deletion.
func sliceReplace(glyphs []shapingGlyph, pos, removeCount int, replacement []shapingGlyph) []shapingGlyph {
	end := pos + removeCount
	if end > len(glyphs) {
		end = len(glyphs)
	}

	// Calculate new length.
	newLen := len(glyphs) - (end - pos) + len(replacement)
	if newLen <= 0 {
		return glyphs[:0]
	}

	// If we are shrinking, shift left and truncate.
	if len(replacement) <= end-pos {
		copy(glyphs[pos:], replacement)
		copy(glyphs[pos+len(replacement):], glyphs[end:])
		return glyphs[:newLen]
	}

	// Expanding: need to grow.
	result := make([]shapingGlyph, newLen)
	copy(result, glyphs[:pos])
	copy(result[pos:], replacement)
	copy(result[pos+len(replacement):], glyphs[end:])
	return result
}
