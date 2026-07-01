package text

import (
	"testing"
)

// TestWithFeatures verifies that WithFeatures correctly stores features on a face.
func TestWithFeatures(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16, WithFeatures(TabularNums, NoLigatures))
	features := face.Features()

	if len(features) != 2 {
		t.Fatalf("Features() returned %d features, want 2", len(features))
	}

	if features[0] != TabularNums {
		t.Errorf("features[0] = %+v, want TabularNums %+v", features[0], TabularNums)
	}
	if features[1] != NoLigatures {
		t.Errorf("features[1] = %+v, want NoLigatures %+v", features[1], NoLigatures)
	}
}

// TestWithFeatures_Single verifies a single feature.
func TestWithFeatures_Single(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(12, WithFeatures(ProportionalNums))
	features := face.Features()

	if len(features) != 1 {
		t.Fatalf("Features() returned %d features, want 1", len(features))
	}
	if features[0] != ProportionalNums {
		t.Errorf("features[0] = %+v, want ProportionalNums %+v", features[0], ProportionalNums)
	}
}

// TestWithFeatures_None verifies that no features is the default.
func TestWithFeatures_None(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	features := face.Features()

	if len(features) != 0 {
		t.Errorf("Features() returned %d features, want 0 (default)", len(features))
	}
}

// TestWithFeatures_Empty verifies that WithFeatures() with no args clears features.
func TestWithFeatures_Empty(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16, WithFeatures())
	features := face.Features()

	if len(features) != 0 {
		t.Errorf("Features() returned %d features, want 0 for empty WithFeatures()", len(features))
	}
}

// TestWithFeatures_PreservedBySource verifies features survive face creation
// and are independent per face from the same source.
func TestWithFeatures_PreservedBySource(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face1 := source.Face(16, WithFeatures(TabularNums))
	face2 := source.Face(16, WithFeatures(NoLigatures))
	face3 := source.Face(16) // No features.

	if len(face1.Features()) != 1 || face1.Features()[0] != TabularNums {
		t.Errorf("face1 features: got %+v, want [TabularNums]", face1.Features())
	}
	if len(face2.Features()) != 1 || face2.Features()[0] != NoLigatures {
		t.Errorf("face2 features: got %+v, want [NoLigatures]", face2.Features())
	}
	if len(face3.Features()) != 0 {
		t.Errorf("face3 features: got %+v, want []", face3.Features())
	}
}

// --------------------------------------------------------------------------
// NewFontFeature string constructor
// --------------------------------------------------------------------------

// TestNewFontFeature verifies the string-based constructor produces correct tags.
func TestNewFontFeature(t *testing.T) {
	tests := []struct {
		tag     string
		value   uint32
		wantTag [4]byte
	}{
		{"tnum", 1, [4]byte{'t', 'n', 'u', 'm'}},
		{"liga", 0, [4]byte{'l', 'i', 'g', 'a'}},
		{"smcp", 1, [4]byte{'s', 'm', 'c', 'p'}},
		{"kern", 1, [4]byte{'k', 'e', 'r', 'n'}},
		{"onum", 1, [4]byte{'o', 'n', 'u', 'm'}},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			f := NewFontFeature(tt.tag, tt.value)
			if f.Tag != tt.wantTag {
				t.Errorf("NewFontFeature(%q, %d).Tag = %v, want %v", tt.tag, tt.value, f.Tag, tt.wantTag)
			}
			if f.Value != tt.value {
				t.Errorf("NewFontFeature(%q, %d).Value = %d, want %d", tt.tag, tt.value, f.Value, tt.value)
			}
		})
	}
}

// TestNewFontFeature_EquivalentToConstant verifies NewFontFeature produces
// the same value as the predefined constants.
func TestNewFontFeature_EquivalentToConstant(t *testing.T) {
	if NewFontFeature("tnum", 1) != TabularNums {
		t.Error("NewFontFeature(\"tnum\", 1) != TabularNums")
	}
	if NewFontFeature("pnum", 1) != ProportionalNums {
		t.Error("NewFontFeature(\"pnum\", 1) != ProportionalNums")
	}
	if NewFontFeature("liga", 0) != NoLigatures {
		t.Error("NewFontFeature(\"liga\", 0) != NoLigatures")
	}
	if NewFontFeature("kern", 1) != Kerning {
		t.Error("NewFontFeature(\"kern\", 1) != Kerning")
	}
	if NewFontFeature("kern", 0) != NoKerning {
		t.Error("NewFontFeature(\"kern\", 0) != NoKerning")
	}
	if NewFontFeature("smcp", 1) != SmallCaps {
		t.Error("NewFontFeature(\"smcp\", 1) != SmallCaps")
	}
	if NewFontFeature("onum", 1) != OldstyleNums {
		t.Error("NewFontFeature(\"onum\", 1) != OldstyleNums")
	}
	if NewFontFeature("dlig", 0) != NoDLigatures {
		t.Error("NewFontFeature(\"dlig\", 0) != NoDLigatures")
	}
}

// TestNewFontFeature_PanicShortTag verifies panic on too-short tag.
func TestNewFontFeature_PanicShortTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewFontFeature(\"ab\", 1) did not panic")
		}
	}()
	NewFontFeature("ab", 1)
}

// TestNewFontFeature_PanicLongTag verifies panic on too-long tag.
func TestNewFontFeature_PanicLongTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewFontFeature(\"abcde\", 1) did not panic")
		}
	}()
	NewFontFeature("abcde", 1)
}

// TestNewFontFeature_PanicEmptyTag verifies panic on empty tag.
func TestNewFontFeature_PanicEmptyTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewFontFeature(\"\", 1) did not panic")
		}
	}()
	NewFontFeature("", 1)
}

// --------------------------------------------------------------------------
// Predefined constant tag verification
// --------------------------------------------------------------------------

// TestPredefinedConstants_Tags verifies all predefined constants have correct tags.
func TestPredefinedConstants_Tags(t *testing.T) {
	tests := []struct {
		name    string
		feature FontFeature
		wantTag [4]byte
		wantVal uint32
	}{
		{"TabularNums", TabularNums, [4]byte{'t', 'n', 'u', 'm'}, 1},
		{"ProportionalNums", ProportionalNums, [4]byte{'p', 'n', 'u', 'm'}, 1},
		{"NoLigatures", NoLigatures, [4]byte{'l', 'i', 'g', 'a'}, 0},
		{"Kerning", Kerning, [4]byte{'k', 'e', 'r', 'n'}, 1},
		{"NoKerning", NoKerning, [4]byte{'k', 'e', 'r', 'n'}, 0},
		{"SmallCaps", SmallCaps, [4]byte{'s', 'm', 'c', 'p'}, 1},
		{"OldstyleNums", OldstyleNums, [4]byte{'o', 'n', 'u', 'm'}, 1},
		{"NoDLigatures", NoDLigatures, [4]byte{'d', 'l', 'i', 'g'}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.feature.Tag != tt.wantTag {
				t.Errorf("%s.Tag = %v, want %v", tt.name, tt.feature.Tag, tt.wantTag)
			}
			if tt.feature.Value != tt.wantVal {
				t.Errorf("%s.Value = %d, want %d", tt.name, tt.feature.Value, tt.wantVal)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Language
// --------------------------------------------------------------------------

// TestFaceLanguage_Default verifies the default language is "en".
func TestFaceLanguage_Default(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	if face.Language() != "en" {
		t.Errorf("Language() = %q, want \"en\"", face.Language())
	}
}

// TestFaceLanguage_WithLanguage verifies WithLanguage sets the language tag.
func TestFaceLanguage_WithLanguage(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	tests := []struct {
		lang string
	}{
		{"ja"},
		{"ar"},
		{"de"},
		{"zh-Hans"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			face := source.Face(16, WithLanguage(tt.lang))
			if face.Language() != tt.lang {
				t.Errorf("Language() = %q, want %q", face.Language(), tt.lang)
			}
		})
	}
}

// TestFaceLanguage_IndependentPerFace verifies language is independent per face.
func TestFaceLanguage_IndependentPerFace(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	faceEN := source.Face(16)
	faceJA := source.Face(16, WithLanguage("ja"))
	faceAR := source.Face(16, WithLanguage("ar"))

	if faceEN.Language() != "en" {
		t.Errorf("faceEN.Language() = %q, want \"en\"", faceEN.Language())
	}
	if faceJA.Language() != "ja" {
		t.Errorf("faceJA.Language() = %q, want \"ja\"", faceJA.Language())
	}
	if faceAR.Language() != "ar" {
		t.Errorf("faceAR.Language() = %q, want \"ar\"", faceAR.Language())
	}
}
