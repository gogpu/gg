package msdf

import (
	"testing"
)

// TestADR054_GlyphKey_VariationHash verifies that GlyphKey with different
// VariationHash produces distinct map keys — prevents MSDF atlas poisoning.
func TestADR054_GlyphKey_VariationHash_Distinct(t *testing.T) {
	key1 := GlyphKey{FontID: 1, GlyphID: 65, Size: 64, VariationHash: 0}
	key2 := GlyphKey{FontID: 1, GlyphID: 65, Size: 64, VariationHash: 12345}

	if key1 == key2 {
		t.Error("GlyphKey with different VariationHash should not be equal")
	}

	m := map[GlyphKey]int{key1: 1, key2: 2}
	if m[key1] != 1 || m[key2] != 2 {
		t.Error("GlyphKey map should store separate entries for different VariationHash")
	}
}

// TestADR054_GlyphKey_ZeroVariation_BackCompat verifies that zero VariationHash
// (static fonts) produces identical behavior to pre-ADR-054 keys.
func TestADR054_GlyphKey_ZeroVariation_BackCompat(t *testing.T) {
	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 64, VariationHash: 0}
	keyOld := GlyphKey{FontID: 1, GlyphID: 65, Size: 64}

	if key != keyOld {
		t.Error("GlyphKey with VariationHash=0 should equal key without it (zero value)")
	}
}
