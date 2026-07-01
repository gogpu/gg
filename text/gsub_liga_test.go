package text

import (
	"os"
	"testing"
)

// TestGSUB_Ligatures_NotoSerif verifies that standard fi/ff/ffi ligatures are
// applied correctly for the NotoSerif test font.
// NotoSerif has 'liga' feature for 'latn' script with Type 4 ligature lookup.
func TestGSUB_Ligatures_NotoSerif(t *testing.T) {
	data, err := os.ReadFile("testdata/notoserif_autohint_shaping.ttf")
	if err != nil {
		t.Skip("NotoSerif test font not available")
	}

	source, err := NewFontSource(data, WithParser("own"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	shaper := NewOwnShaper()
	face := source.Face(16.0)

	tests := []struct {
		name    string
		text    string
		wantLen int
	}{
		{"fi", "fi", 1},
		{"ff", "ff", 1},
		{"fi_in_word", "office", 4}, // o-f-fi_lig-c-e → depends on font; at least fewer than 6
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shaper.Shape(tt.text, face)
			if len(result) == 0 {
				t.Fatal("Shape returned no glyphs")
			}
			for i, g := range result {
				t.Logf("  [%d] GID=%d XAdvance=%.2f Cluster=%d", i, g.GID, g.XAdvance, g.Cluster)
			}
			if len(result) > tt.wantLen {
				t.Errorf("Shape(%q) = %d glyphs, want <= %d (ligature not applied)",
					tt.text, len(result), tt.wantLen)
			}
		})
	}
}

// TestGSUB_Ligatures_TimesNewRoman verifies fi/fl ligatures for Times New Roman.
// Times New Roman places Latin fi/fl under 'dlig' (discretionary ligatures)
// rather than 'liga'. Our shaper enables 'dlig' by default to handle this.
//
// Skipped on CI (system font not available).
func TestGSUB_Ligatures_TimesNewRoman(t *testing.T) {
	timesData, err := os.ReadFile("C:/Windows/Fonts/times.ttf")
	if err != nil {
		t.Skip("Times New Roman not available (system font)")
	}

	source, err := NewFontSource(timesData, WithParser("own"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	shaper := NewOwnShaper()
	face := source.Face(16.0)

	tests := []struct {
		name    string
		text    string
		wantLen int
	}{
		{"fi", "fi", 1},
		{"fl", "fl", 1},
		{"ffi", "ffi", 1},
		{"office", "office", 4}, // o-ffi_lig-c-e
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shaper.Shape(tt.text, face)
			if len(result) == 0 {
				t.Fatal("Shape returned no glyphs")
			}
			for i, g := range result {
				t.Logf("  [%d] GID=%d XAdvance=%.2f Cluster=%d", i, g.GID, g.XAdvance, g.Cluster)
			}
			if len(result) > tt.wantLen {
				t.Errorf("Shape(%q) = %d glyphs, want <= %d (ligature not applied)",
					tt.text, len(result), tt.wantLen)
			}
		})
	}
}

// TestGSUB_NoLigatures verifies that NoLigatures disables standard ligatures.
func TestGSUB_NoLigatures(t *testing.T) {
	data, err := os.ReadFile("testdata/notoserif_autohint_shaping.ttf")
	if err != nil {
		t.Skip("NotoSerif test font not available")
	}

	source, err := NewFontSource(data, WithParser("own"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	shaper := NewOwnShaper()

	// With ligatures disabled, "fi" must stay as 2 separate glyphs.
	face := source.Face(16.0, WithFeatures(NoLigatures))
	result := shaper.Shape("fi", face)
	if len(result) != 2 {
		t.Errorf("Shape(\"fi\") with NoLigatures: got %d glyphs, want 2", len(result))
	}
}

// TestGSUB_NoDLigatures verifies that NoDLigatures disables discretionary ligatures
// while keeping standard ligatures enabled.
func TestGSUB_NoDLigatures(t *testing.T) {
	timesData, err := os.ReadFile("C:/Windows/Fonts/times.ttf")
	if err != nil {
		t.Skip("Times New Roman not available (system font)")
	}

	source, err := NewFontSource(timesData, WithParser("own"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	shaper := NewOwnShaper()

	// With dlig disabled, Times New Roman fi should stay as 2 glyphs
	// (since Times New Roman places fi under 'dlig' for Latin).
	face := source.Face(16.0, WithFeatures(NoDLigatures))
	result := shaper.Shape("fi", face)
	if len(result) != 2 {
		t.Errorf("Shape(\"fi\") with NoDLigatures on Times New Roman: got %d glyphs, want 2", len(result))
	}
}

// TestGSUB_DefaultFeatures verifies the default feature set.
func TestGSUB_DefaultFeatures(t *testing.T) {
	gsubTags, gposTags := collectDesiredFeatures(nil)

	// GSUB must include ccmp, liga, clig, rlig, dlig.
	wantGSUB := map[[4]byte]bool{
		{'c', 'c', 'm', 'p'}: true,
		{'l', 'i', 'g', 'a'}: true,
		{'c', 'l', 'i', 'g'}: true,
		{'r', 'l', 'i', 'g'}: true,
		{'d', 'l', 'i', 'g'}: true,
	}
	gotGSUB := make(map[[4]byte]bool, len(gsubTags))
	for _, tag := range gsubTags {
		gotGSUB[tag] = true
	}
	for tag := range wantGSUB {
		if !gotGSUB[tag] {
			t.Errorf("default GSUB features missing '%s'", string(tag[:]))
		}
	}

	// GPOS must include kern.
	wantGPOS := map[[4]byte]bool{
		{'k', 'e', 'r', 'n'}: true,
	}
	gotGPOS := make(map[[4]byte]bool, len(gposTags))
	for _, tag := range gposTags {
		gotGPOS[tag] = true
	}
	for tag := range wantGPOS {
		if !gotGPOS[tag] {
			t.Errorf("default GPOS features missing '%s'", string(tag[:]))
		}
	}
}

// TestGSUB_FeatureDisable verifies that user features can disable defaults.
func TestGSUB_FeatureDisable(t *testing.T) {
	// Disable liga.
	gsubTags, _ := collectDesiredFeatures([]FontFeature{NoLigatures})
	ligaTag := [4]byte{'l', 'i', 'g', 'a'}
	for _, tag := range gsubTags {
		if tag == ligaTag {
			t.Error("NoLigatures should remove 'liga' from GSUB features")
		}
	}

	// Disable dlig.
	gsubTags, _ = collectDesiredFeatures([]FontFeature{NoDLigatures})
	dligTag := [4]byte{'d', 'l', 'i', 'g'}
	for _, tag := range gsubTags {
		if tag == dligTag {
			t.Error("NoDLigatures should remove 'dlig' from GSUB features")
		}
	}
}
