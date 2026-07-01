// OwnShaper — Pure Go text shaper with GSUB/GPOS support.
//
// OwnShaper implements the Shaper interface with direct binary parsing of
// OpenType GSUB and GPOS tables, replacing the legacy GoTextShaper
// as part of the Pure Go Font Stack (ADR-048, Phase 5).
//
// Shaping pipeline:
//  1. Character → Glyph (cmap lookup via ownParsedFont)
//  2. GSUB substitution (ligatures, single/multiple/alternate substitution)
//  3. GPOS positioning (kerning, single adjustment)
//  4. kern table fallback (if GPOS has no 'kern' feature)
//  5. Scaling (font units → pixels)
//
// Supported features:
//   - GSUB: single (Type 1), multiple (Type 2), alternate (Type 3),
//     ligature (Type 4), extension (Type 7)
//   - GPOS: single adjustment (Type 1), pair adjustment/kerning (Type 2),
//     extension (Type 9)
//   - kern: format 0 fallback pairs
//   - Default features: 'liga' (ligatures), 'kern' (kerning)
//
// OwnShaper is safe for concurrent use. Parsed table caches are protected
// by sync.Once (per FontSource) and sync.RWMutex (for the cache map).
//
// This file is part of Phase 5 (ADR-048: Pure Go Font Stack).
package text

import "sync"

// OwnShaper provides text shaping using Pure Go GSUB/GPOS parsing.
// It supports ligature substitution, kerning, and other OpenType features
// without external dependencies.
//
// OwnShaper caches parsed GSUB/GPOS/kern tables per FontSource. The
// cached data is read-only and safe for concurrent use.
type OwnShaper struct {
	mu    sync.RWMutex
	cache map[*FontSource]*ownShaperCache
}

// ownShaperCache holds parsed shaping tables for a single font.
type ownShaperCache struct {
	gsub     *gsubTable // nil if font has no GSUB
	gpos     *gposTable // nil if font has no GPOS
	kern     *kernTable // nil if font has no kern
	hasGPOS  bool       // true if GPOS was found (even if no kern feature)
	upem     int        // units per em
	cmap     *cmapLookup
	hmtxAdv  []uint16
	numHMtx  int
	numGlyph int
}

// NewOwnShaper creates a new OwnShaper.
func NewOwnShaper() *OwnShaper {
	return &OwnShaper{
		cache: make(map[*FontSource]*ownShaperCache),
	}
}

// Shape implements the Shaper interface.
// It converts text into positioned glyphs using Pure Go GSUB/GPOS shaping.
// The font size is obtained from face.Size().
func (s *OwnShaper) Shape(text string, face Face) []ShapedGlyph {
	if text == "" || face == nil {
		return nil
	}

	source := face.Source()
	if source == nil {
		return nil
	}

	parsed := source.Parsed()
	if parsed == nil {
		return nil
	}

	sc := s.getOrCreateCache(source)
	if sc == nil {
		return nil
	}

	size := face.Size()
	runes := []rune(text)

	// Step 1: Character → Glyph (cmap lookup).
	glyphs := runeToGlyphs(runes, sc)

	// Step 2: Determine script and language tags.
	scriptTag := detectOTScriptTag(runes)
	langTag := parseLangTag(face.Language())

	// Step 3: Determine which features to apply.
	desiredGSUB, desiredGPOS := collectDesiredFeatures(face.Features())

	// Step 4: Apply GSUB substitutions.
	if sc.gsub != nil && len(desiredGSUB) > 0 {
		glyphs = sc.gsub.applyGSUB(glyphs, scriptTag, langTag, desiredGSUB)
	}

	// Step 5: Apply GPOS positioning.
	var adjustments []gposAdjustment
	if sc.gpos != nil && len(desiredGPOS) > 0 {
		adjustments = sc.gpos.applyGPOS(glyphs, scriptTag, langTag, desiredGPOS)
	}

	// Step 6: kern table fallback (only if GPOS has no kern feature).
	var kernFallback bool
	if sc.kern != nil && !gposHasKern(sc.gpos, scriptTag, langTag) {
		kernFallback = true
	}

	// Step 7: Build positioned glyph output.
	return buildShapedGlyphs(glyphs, adjustments, sc, size, runes, kernFallback)
}

// ClearCache removes all cached shaping data.
func (s *OwnShaper) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[*FontSource]*ownShaperCache)
}

// RemoveSource removes cached data for a specific FontSource.
func (s *OwnShaper) RemoveSource(source *FontSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, source)
}

// getOrCreateCache returns the cached shaping tables for the given font source.
func (s *OwnShaper) getOrCreateCache(source *FontSource) *ownShaperCache {
	// Fast path: read lock.
	s.mu.RLock()
	if sc, ok := s.cache[source]; ok {
		s.mu.RUnlock()
		return sc
	}
	s.mu.RUnlock()

	// Slow path: parse and cache.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check.
	if sc, ok := s.cache[source]; ok {
		return sc
	}

	sc := buildShaperCache(source)
	s.cache[source] = sc
	return sc
}

// buildShaperCache parses GSUB, GPOS, and kern tables from the font data.
func buildShaperCache(source *FontSource) *ownShaperCache {
	sc := &ownShaperCache{}

	parsed := source.Parsed()
	if parsed == nil {
		return sc
	}

	sc.upem = parsed.UnitsPerEm()
	sc.numGlyph = parsed.NumGlyphs()

	// Try to get raw table data. The own parser stores tables directly;
	// for ximage parser we need the RawFontDataProvider interface.
	tables := getRawTables(source)
	if tables == nil {
		return sc
	}

	// Parse cmap.
	if cmapData, ok := tables["cmap"]; ok {
		sc.cmap = parseCmapTable(cmapData)
	}

	// Parse hmtx (for advance widths).
	parseHmtxFromTables(tables, sc)

	// Parse GSUB.
	if gsubData, ok := tables["GSUB"]; ok {
		sc.gsub = parseGSUB(gsubData)
	}

	// Parse GPOS.
	if gposData, ok := tables["GPOS"]; ok {
		sc.gpos = parseGPOS(gposData)
		sc.hasGPOS = sc.gpos != nil
	}

	// Parse kern (fallback).
	if kernData, ok := tables["kern"]; ok {
		sc.kern = parseKern(kernData)
	}

	return sc
}

// parseHmtxFromTables parses the hhea and hmtx tables into the shaper cache.
// Extracted to avoid deep nesting (nestif linter).
func parseHmtxFromTables(tables map[string][]byte, sc *ownShaperCache) {
	hheaData, ok := tables["hhea"]
	if !ok {
		return
	}
	hhea, hheaOK := parseHheaTable(hheaData)
	if !hheaOK || hhea.numberOfHMetrics == 0 {
		return
	}
	hmtxData, ok := tables["hmtx"]
	if !ok {
		return
	}
	adv, _, err := parseHmtx(hmtxData, hhea.numberOfHMetrics, sc.numGlyph)
	if err != nil {
		return
	}
	sc.hmtxAdv = adv
	sc.numHMtx = hhea.numberOfHMetrics
}

// getRawTables extracts raw table data from a FontSource.
// Prefers the own parser's direct table map; falls back to re-parsing
// raw font data if available via RawFontDataProvider.
func getRawTables(source *FontSource) map[string][]byte {
	parsed := source.Parsed()

	// Own parser: tables are directly available.
	if own, ok := parsed.(*ownParsedFont); ok {
		return own.tables
	}

	// RawFontDataProvider: re-parse table directory.
	if provider, ok := parsed.(RawFontDataProvider); ok {
		rawData := provider.RawFontData()
		if rawData != nil {
			tables, err := parseFontTables(rawData)
			if err == nil {
				return tables
			}
		}
	}

	return nil
}

// runeToGlyphs converts runes to glyph entries using the cmap.
func runeToGlyphs(runes []rune, sc *ownShaperCache) []shapingGlyph {
	glyphs := make([]shapingGlyph, 0, len(runes))
	for i, r := range runes {
		// Skip non-tab control characters.
		if r < 0x20 && r != '\t' {
			continue
		}
		var gid uint16
		if r == '\t' {
			// Map tab to space glyph.
			if sc.cmap != nil {
				gid = sc.cmap.glyphIndex(' ')
			}
		} else if sc.cmap != nil {
			gid = sc.cmap.glyphIndex(r)
		}
		glyphs = append(glyphs, shapingGlyph{gid: gid, cluster: i})
	}
	return glyphs
}

// collectDesiredFeatures determines which GSUB and GPOS feature tags to apply.
// User features can enable/disable individual features.
//
// Default GSUB features: ccmp, liga, clig, rlig, dlig.
//
// Why 'dlig': Some major fonts (e.g. Times New Roman) place standard Latin
// ligatures (fi, fl, ffi) under 'dlig' rather than 'liga'. Without 'dlig',
// these common ligatures would not be applied. Microsoft DirectWrite and
// most desktop applications enable these ligatures by default.
// Users who want strictly HarfBuzz-compatible behavior can disable 'dlig'
// explicitly with text.NoDLigatures.
//
// Default GPOS features: kern.
func collectDesiredFeatures(userFeatures []FontFeature) (gsubTags, gposTags [][4]byte) {
	// Default features.
	ccmp := [4]byte{'c', 'c', 'm', 'p'}
	liga := [4]byte{'l', 'i', 'g', 'a'}
	kern := [4]byte{'k', 'e', 'r', 'n'}
	clig := [4]byte{'c', 'l', 'i', 'g'}
	rlig := [4]byte{'r', 'l', 'i', 'g'}
	dlig := [4]byte{'d', 'l', 'i', 'g'}

	// GSUB defaults.
	gsubEnabled := map[[4]byte]bool{
		ccmp: true,
		liga: true,
		clig: true,
		rlig: true,
		dlig: true,
	}

	// GPOS defaults.
	gposEnabled := map[[4]byte]bool{
		kern: true,
	}

	// Apply user overrides.
	for _, f := range userFeatures {
		tag := f.Tag
		if f.Value == 0 {
			// Disable.
			delete(gsubEnabled, tag)
			delete(gposEnabled, tag)
		} else {
			// Enable — add to the appropriate category.
			if isGSUBFeature(tag) {
				gsubEnabled[tag] = true
			} else {
				gposEnabled[tag] = true
			}
		}
	}

	gsubTags = make([][4]byte, 0, len(gsubEnabled))
	for tag := range gsubEnabled {
		gsubTags = append(gsubTags, tag)
	}
	gposTags = make([][4]byte, 0, len(gposEnabled))
	for tag := range gposEnabled {
		gposTags = append(gposTags, tag)
	}
	return gsubTags, gposTags
}

// isGSUBFeature returns true if the feature tag is typically a GSUB feature.
func isGSUBFeature(tag [4]byte) bool {
	gsubFeatures := map[[4]byte]bool{
		{'c', 'c', 'm', 'p'}: true, // Glyph composition/decomposition
		{'l', 'i', 'g', 'a'}: true, // Standard ligatures
		{'c', 'l', 'i', 'g'}: true, // Contextual ligatures
		{'r', 'l', 'i', 'g'}: true, // Required ligatures
		{'d', 'l', 'i', 'g'}: true, // Discretionary ligatures
		{'s', 'm', 'c', 'p'}: true, // Small caps
		{'c', '2', 's', 'c'}: true, // Capitals to small caps
		{'s', 'w', 's', 'h'}: true, // Swash
		{'s', 'a', 'l', 't'}: true, // Stylistic alternates
		{'c', 'a', 'l', 't'}: true, // Contextual alternates
	}
	return gsubFeatures[tag]
}

// gposHasKern checks whether the GPOS table has a 'kern' feature
// available for the given script and language.
func gposHasKern(gpos *gposTable, scriptTag, langTag [4]byte) bool {
	if gpos == nil {
		return false
	}
	kernTag := [4]byte{'k', 'e', 'r', 'n'}
	indices := collectLookupIndices(gpos.scripts, gpos.features, scriptTag, langTag, [][4]byte{kernTag})
	return len(indices) > 0
}

// buildShapedGlyphs converts internal glyph entries + adjustments to ShapedGlyph output.
func buildShapedGlyphs(
	glyphs []shapingGlyph,
	adjustments []gposAdjustment,
	sc *ownShaperCache,
	size float64,
	runes []rune,
	kernFallback bool,
) []ShapedGlyph {
	if len(glyphs) == 0 {
		return nil
	}

	scale := size / float64(sc.upem)
	result := make([]ShapedGlyph, len(glyphs))
	var x, y float64

	for i := range glyphs {
		ge := &glyphs[i]

		// Get base advance (font units).
		var advFU uint16
		if sc.hmtxAdv != nil {
			advFU = hmtxAdvance(sc.hmtxAdv, sc.numHMtx, ge.gid)
		}

		// Tab handling: use tab-stop advance.
		if ge.cluster < len(runes) && runes[ge.cluster] == '\t' {
			tabGID, tabAdv := ownTabAdvance(sc, size)
			ge.gid = tabGID
			result[i] = ShapedGlyph{
				GID:      GlyphID(ge.gid),
				Cluster:  ge.cluster,
				X:        x,
				Y:        y,
				XAdvance: tabAdv,
			}
			x += tabAdv
			continue
		}

		// GPOS adjustments.
		var adj gposAdjustment
		if i < len(adjustments) {
			adj = adjustments[i]
		}

		// kern table fallback.
		if kernFallback && sc.kern != nil && i+1 < len(glyphs) {
			kv := sc.kern.kernValue(ge.gid, glyphs[i+1].gid)
			if kv != 0 {
				adj.xAdvance += kv
			}
		}

		// Scale to pixels.
		xAdv := float64(advFU)*scale + float64(adj.xAdvance)*scale
		yAdv := float64(adj.yAdvance) * scale
		xOff := float64(adj.xPlacement) * scale
		yOff := float64(adj.yPlacement) * scale

		var cjk bool
		if ge.cluster < len(runes) {
			cjk = IsCJKRune(runes[ge.cluster])
		}

		result[i] = ShapedGlyph{
			GID:      GlyphID(ge.gid),
			Cluster:  ge.cluster,
			X:        x + xOff,
			Y:        y + yOff,
			XAdvance: xAdv,
			YAdvance: yAdv,
			IsCJK:    cjk,
		}

		x += xAdv
		y += yAdv
	}

	return result
}

// ownTabAdvance returns the space glyph ID and tab-stop advance for a font
// using the cached shaper data (without going through ParsedFont interface).
func ownTabAdvance(sc *ownShaperCache, size float64) (uint16, float64) {
	var spaceGID uint16
	if sc.cmap != nil {
		spaceGID = sc.cmap.glyphIndex(' ')
	}

	var spaceAdvFU uint16
	if sc.hmtxAdv != nil {
		spaceAdvFU = hmtxAdvance(sc.hmtxAdv, sc.numHMtx, spaceGID)
	}

	if sc.upem == 0 {
		return spaceGID, 0
	}
	spaceAdv := float64(spaceAdvFU) * size / float64(sc.upem)
	return spaceGID, spaceAdv * float64(globalTabWidth)
}

// --- Script detection ---

// detectOTScriptTag returns the OpenType script tag for the dominant
// script in the given runes. Falls back to 'latn' (Latin).
func detectOTScriptTag(runes []rune) [4]byte {
	for _, r := range runes {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		return runeToOTScript(r)
	}
	return [4]byte{'l', 'a', 't', 'n'} // Latin
}

// scriptRange maps a Unicode code point range to an OpenType script tag.
type scriptRange struct {
	lo, hi rune
	tag    [4]byte
}

// otScriptRanges maps Unicode ranges to OpenType script tags.
// Ranges must be non-overlapping and sorted by lo. The first match wins.
// This covers the most common scripts; complex scripts will need
// more detailed mapping in the future.
var otScriptRanges = []scriptRange{
	{0x0370, 0x03FF, [4]byte{'g', 'r', 'e', 'k'}}, // Greek
	{0x0400, 0x04FF, [4]byte{'c', 'y', 'r', 'l'}}, // Cyrillic
	{0x0590, 0x05FF, [4]byte{'h', 'e', 'b', 'r'}}, // Hebrew
	{0x0600, 0x06FF, [4]byte{'a', 'r', 'a', 'b'}}, // Arabic
	{0x0900, 0x097F, [4]byte{'d', 'e', 'v', '2'}}, // Devanagari
	{0x3000, 0x30FF, [4]byte{'k', 'a', 'n', 'a'}}, // Hiragana + Katakana
	{0x3100, 0x9FFF, [4]byte{'h', 'a', 'n', 'i'}}, // CJK
	{0xAC00, 0xD7AF, [4]byte{'h', 'a', 'n', 'g'}}, // Hangul
}

// runeToOTScript maps a rune to its OpenType script tag.
func runeToOTScript(r rune) [4]byte {
	latn := [4]byte{'l', 'a', 't', 'n'}
	if r <= 0x024F {
		return latn // Latin (Basic + Extended)
	}
	for i := range otScriptRanges {
		sr := &otScriptRanges[i]
		if r >= sr.lo && r <= sr.hi {
			return sr.tag
		}
	}
	return latn
}

// parseLangTag converts a BCP 47 language tag (e.g. "en") to an OpenType
// language system tag. The OpenType spec uses uppercase 4-byte tags with
// trailing space padding. For simplicity, we return a zero tag (which
// will match the default LangSys) for most languages.
func parseLangTag(lang string) [4]byte {
	// Map common languages to OpenType language tags.
	// Most fonts only define a default LangSys, so an empty tag is fine.
	switch lang {
	case "tr":
		return [4]byte{'T', 'R', 'K', ' '}
	case "az":
		return [4]byte{'A', 'Z', 'E', ' '}
	case "ro":
		return [4]byte{'R', 'O', 'M', ' '}
	case "nl":
		return [4]byte{'N', 'L', 'D', ' '}
	default:
		// For most languages, the default LangSys is used.
		// Return a zero tag — collectLookupIndices will fall back to defaultLan.
		return [4]byte{}
	}
}

// init is intentionally empty — OwnShaper is now the default shaper,
// initialized via defaultOwnShaper in shaper.go (ADR-048 Phase 6).
