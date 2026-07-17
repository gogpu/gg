package text

import (
	"testing"
)

// --------------------------------------------------------------------------
// ADR-054: Unified Variable Font Outline Extraction
//
// Enterprise tests verifying that gvar deltas are applied unconditionally
// to ALL outline extraction paths — matching Skia/skrifa/FreeType invariant.
// --------------------------------------------------------------------------

// TestADR054_ExtractOutlineHintedVar_ProducesDifferentOutline verifies that
// variations produce geometrically different outlines from the default instance.
func TestADR054_ExtractOutlineHintedVar_ProducesDifferentOutline(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	ext := &OutlineExtractor{}
	gid := GlyphID(parsed.GlyphIndex('A'))
	if gid == 0 {
		t.Skip("font has no glyph for 'A'")
	}

	defaultOutline, err := ext.ExtractOutlineHintedVar(parsed, gid, 24, HintingNone, nil)
	if err != nil || defaultOutline == nil {
		t.Fatalf("default outline: err=%v, outline=%v", err, defaultOutline)
	}

	boldOutline, err := ext.ExtractOutlineHintedVar(parsed, gid, 24, HintingNone,
		[]FontVariation{NewFontVariation("wght", 700)})
	if err != nil || boldOutline == nil {
		t.Fatalf("bold outline: err=%v, outline=%v", err, boldOutline)
	}

	if outlineSegmentsEqual(defaultOutline, boldOutline) {
		t.Error("wght=700 outline is identical to default — gvar deltas not applied")
	}
}

// TestADR054_ExtractOutlineHintedVar_NilVariations_MatchesStatic verifies that
// nil variations produce the same result as ExtractOutlineHinted (zero overhead).
func TestADR054_ExtractOutlineHintedVar_NilVariations_MatchesStatic(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	ext := &OutlineExtractor{}
	gid := GlyphID(parsed.GlyphIndex('H'))
	if gid == 0 {
		t.Skip("font has no glyph for 'H'")
	}

	staticOutline, err := ext.ExtractOutlineHinted(parsed, gid, 24, HintingNone)
	if err != nil || staticOutline == nil {
		t.Fatalf("static outline: err=%v", err)
	}

	varOutline, err := ext.ExtractOutlineHintedVar(parsed, gid, 24, HintingNone, nil)
	if err != nil || varOutline == nil {
		t.Fatalf("var(nil) outline: err=%v", err)
	}

	if !outlineSegmentsEqual(staticOutline, varOutline) {
		t.Error("ExtractOutlineHintedVar(nil) differs from ExtractOutlineHinted — should be identical")
	}
}

// TestADR054_CacheKey_VariationHash_Differentiates verifies that cache keys
// with different VariationHash values are distinct — prevents cross-variation
// cache poisoning.
func TestADR054_CacheKey_VariationHash_Differentiates(t *testing.T) {
	regular := OutlineCacheKey{
		FontID:        12345,
		GID:           65,
		Size:          24,
		Hinting:       HintingNone,
		VariationHash: 0,
	}
	bold := OutlineCacheKey{
		FontID:        12345,
		GID:           65,
		Size:          24,
		Hinting:       HintingNone,
		VariationHash: VariationHash([]FontVariation{NewFontVariation("wght", 700)}),
	}

	if regular == bold {
		t.Error("cache keys with different VariationHash should not be equal")
	}
	if bold.VariationHash == 0 {
		t.Error("VariationHash for wght=700 should not be zero")
	}
}

// TestADR054_CacheKey_SameVariations_SameHash verifies deterministic hashing.
func TestADR054_CacheKey_SameVariations_SameHash(t *testing.T) {
	v1 := []FontVariation{NewFontVariation("wght", 700)}
	v2 := []FontVariation{NewFontVariation("wght", 700)}

	h1 := VariationHash(v1)
	h2 := VariationHash(v2)

	if h1 != h2 {
		t.Errorf("same variations should produce same hash: %d != %d", h1, h2)
	}
}

// TestADR054_CacheKey_DifferentWeights_DifferentHash verifies that different
// weight values produce different cache keys.
func TestADR054_CacheKey_DifferentWeights_DifferentHash(t *testing.T) {
	h400 := VariationHash([]FontVariation{NewFontVariation("wght", 400)})
	h700 := VariationHash([]FontVariation{NewFontVariation("wght", 700)})

	if h400 == h700 {
		t.Error("wght=400 and wght=700 should produce different hashes")
	}
}

// TestADR054_GlyphCache_VariationsProduceDifferentEntries verifies that the
// glyph cache stores separate entries for different variation instances.
func TestADR054_GlyphCache_VariationsProduceDifferentEntries(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	ext := &OutlineExtractor{}
	gid := GlyphID(parsed.GlyphIndex('O'))
	if gid == 0 {
		t.Skip("font has no glyph for 'O'")
	}

	cache := NewGlyphCache()

	regularVars := []FontVariation{NewFontVariation("wght", 400)}
	boldVars := []FontVariation{NewFontVariation("wght", 700)}

	keyRegular := OutlineCacheKey{
		FontID:        1,
		GID:           gid,
		Size:          24,
		Hinting:       HintingNone,
		VariationHash: VariationHash(regularVars),
	}
	keyBold := OutlineCacheKey{
		FontID:        1,
		GID:           gid,
		Size:          24,
		Hinting:       HintingNone,
		VariationHash: VariationHash(boldVars),
	}

	regularOutline := cache.GetOrCreate(keyRegular, func() *GlyphOutline {
		o, _ := ext.ExtractOutlineHintedVar(parsed, gid, 24, HintingNone, regularVars)
		return o
	})
	boldOutline := cache.GetOrCreate(keyBold, func() *GlyphOutline {
		o, _ := ext.ExtractOutlineHintedVar(parsed, gid, 24, HintingNone, boldVars)
		return o
	})

	if regularOutline == nil || boldOutline == nil {
		t.Fatalf("outlines: regular=%v, bold=%v", regularOutline, boldOutline)
	}
	if outlineSegmentsEqual(regularOutline, boldOutline) {
		t.Error("cached regular and bold outlines should differ — gvar deltas not applied or cache poisoned")
	}

	// Verify cache hit returns same pointer (not re-extracted).
	cachedRegular := cache.Get(keyRegular)
	if cachedRegular != regularOutline {
		t.Error("cache hit should return same outline pointer")
	}
	cachedBold := cache.Get(keyBold)
	if cachedBold != boldOutline {
		t.Error("cache hit should return same outline pointer")
	}
}

// TestADR054_GlyphMaskKey_VariationHash verifies that GlyphMaskKey includes
// variation hash and produces distinct keys for different weights.
func TestADR054_GlyphMaskKey_VariationHash(t *testing.T) {
	key1 := MakeGlyphMaskKey(1, 65, 24, 0, 0)
	key1.VariationHash = 0

	key2 := MakeGlyphMaskKey(1, 65, 24, 0, 0)
	key2.VariationHash = VariationHash([]FontVariation{NewFontVariation("wght", 700)})

	if key1 == key2 {
		t.Error("GlyphMaskKey with different VariationHash should not be equal")
	}
}

// TestADR054_RasterizeHintedVar_ProducesDifferentMask verifies that the glyph
// mask rasterizer applies gvar deltas when called with variations.
func TestADR054_RasterizeHintedVar_ProducesDifferentMask(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	rast := NewGlyphMaskRasterizer()
	gid := GlyphID(parsed.GlyphIndex('I'))
	if gid == 0 {
		t.Skip("font has no glyph for 'I'")
	}

	regularResult, err := rast.RasterizeHintedVar(parsed, gid, 24, 0, 0, HintingFull, nil)
	if err != nil {
		t.Fatalf("RasterizeHintedVar(nil): %v", err)
	}

	boldResult, err := rast.RasterizeHintedVar(parsed, gid, 24, 0, 0, HintingFull,
		[]FontVariation{NewFontVariation("wght", 700)})
	if err != nil {
		t.Fatalf("RasterizeHintedVar(700): %v", err)
	}

	if regularResult == nil || boldResult == nil {
		t.Skip("glyph 'I' has no rasterizable outline at this size")
	}

	if regularResult.Width == boldResult.Width && maskBytesEqual(regularResult.Mask, boldResult.Mask) {
		t.Error("wght=700 mask should differ from default — gvar deltas not applied in rasterizer")
	}
}

// TestADR054_RasterizeAliasedVar_ProducesDifferentMask verifies aliased path too.
func TestADR054_RasterizeAliasedVar_ProducesDifferentMask(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	rast := NewGlyphMaskRasterizer()
	gid := GlyphID(parsed.GlyphIndex('H'))
	if gid == 0 {
		t.Skip("font has no glyph for 'H'")
	}

	regularResult, err := rast.RasterizeAliasedVar(parsed, gid, 24, 0, 0, HintingFull, nil)
	if err != nil {
		t.Fatalf("RasterizeAliasedVar(nil): %v", err)
	}

	boldResult, err := rast.RasterizeAliasedVar(parsed, gid, 24, 0, 0, HintingFull,
		[]FontVariation{NewFontVariation("wght", 700)})
	if err != nil {
		t.Fatalf("RasterizeAliasedVar(700): %v", err)
	}

	if regularResult == nil || boldResult == nil {
		t.Skip("glyph 'H' has no rasterizable outline at this size")
	}

	if regularResult.Width == boldResult.Width && maskBytesEqual(regularResult.Mask, boldResult.Mask) {
		t.Error("aliased wght=700 mask should differ from default — gvar deltas not applied")
	}
}

// TestADR054_RenderParams_Variations verifies that RenderParams carries variations
// and GlyphRenderer uses them.
func TestADR054_RenderParams_Variations(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	renderer := NewGlyphRenderer()
	gid := GlyphID(parsed.GlyphIndex('M'))
	if gid == 0 {
		t.Skip("font has no glyph for 'M'")
	}

	glyph := &ShapedGlyph{GID: gid, X: 0, Y: 0}

	regularParams := DefaultRenderParams()
	boldParams := DefaultRenderParams()
	boldParams.Variations = []FontVariation{NewFontVariation("wght", 700)}

	regularOutline := renderer.RenderGlyph(glyph, parsed, 24, regularParams)
	boldOutline := renderer.RenderGlyph(glyph, parsed, 24, boldParams)

	if regularOutline == nil || boldOutline == nil {
		t.Skip("glyph 'M' produced nil outline")
	}

	if outlineSegmentsEqual(regularOutline, boldOutline) {
		t.Error("GlyphRenderer with wght=700 should produce different outline — variations not propagated")
	}
}

// maskBytesEqual compares two byte slices for exact equality.
func maskBytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
