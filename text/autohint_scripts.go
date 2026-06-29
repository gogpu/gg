package text

// Script detection and classification for the auto-hinter.
//
// The auto-hinter operates on a per-script basis: different scripts have
// different reference characters for measuring stem widths and defining
// blue zones. This file implements script detection and defines the
// script-specific character lists.
//
// Architecture (matching FreeType/skrifa):
//
//	Font loaded → detect primary script via cmap → per-script blues + widths
//
// Three script groups determine the algorithm:
//   - Default (Latin, Hebrew, Arabic, Greek, Cyrillic, ...) → computeDefaultBlues
//   - CJK (HANI) → computeCJKBlues
//   - Indic → no blues (skip)
//
// References:
//   - FreeType afscript.h — script definitions
//   - FreeType afblue.dat — blue zone reference characters
//   - skrifa style.rs — ScriptClass, ScriptGroup, SCRIPT_CLASSES
//   - skrifa generated/generated_autohint_styles.rs — script data

// scriptGroup determines which hinting algorithm to use.
type scriptGroup int

const (
	// scriptGroupDefault handles Latin, Hebrew, Arabic, Greek, Cyrillic,
	// and most other scripts. Uses computeDefaultBlues.
	scriptGroupDefault scriptGroup = iota

	// scriptGroupCJK handles CJK ideographs (HANI). Uses computeCJKBlues.
	scriptGroupCJK

	// scriptGroupIndic handles Indic scripts. No blue zones (yet).
	scriptGroupIndic
)

// blueSpec defines a blue zone by its reference characters and flags.
// Matches skrifa's (blue_str, BlueZones) tuple in ScriptClass.blues.
type blueSpec struct {
	// chars contains space-separated reference characters to measure.
	// For CJK, the '|' separator divides fill chars from flat chars.
	chars string

	// flags defines the zone properties (TOP, LONG, X_HEIGHT, etc.).
	flags blueZoneFlags
}

// scriptClass defines the hinting properties for a script.
// Matches skrifa's ScriptClass struct.
type scriptClass struct {
	// name is the human-readable script name (for diagnostics).
	name string

	// group determines the hinting algorithm.
	group scriptGroup

	// stdChars lists characters for standard stem width measurement.
	// The first character with a glyph in the font is used.
	stdChars []rune

	// blues defines blue zone specifications for this script.
	blues []blueSpec
}

// Script class definitions, matching skrifa generated_autohint_styles.rs.
// Only scripts we actively test and support are defined here. Additional
// scripts can be added following the same pattern.

var scriptLatin = scriptClass{
	name:     "Latin",
	group:    scriptGroupDefault,
	stdChars: []rune{'o', 'O', '0'},
	blues: []blueSpec{
		{chars: "T H E Z O C Q S", flags: blueZoneTop},
		{chars: "H E Z L O C U S", flags: 0},
		{chars: "f i j k d b h", flags: blueZoneTop},
		{chars: "u v x z o e s c", flags: blueZoneTop | blueZoneXHeight},
		{chars: "n r x z o e s c", flags: 0},
		{chars: "p q g j y", flags: 0},
	},
}

var scriptHebrew = scriptClass{
	name:     "Hebrew",
	group:    scriptGroupDefault,
	stdChars: []rune{'\u05DD'}, // Final Mem (ם)
	blues: []blueSpec{
		{chars: "\u05D1 \u05D3 \u05D4 \u05D7 \u05DA \u05DB \u05DD \u05E1", flags: blueZoneTop | blueZoneLong},
		{chars: "\u05D1 \u05D8 \u05DB \u05DD \u05E1 \u05E6", flags: 0},
		{chars: "\u05E7 \u05DA \u05DF \u05E3 \u05E5", flags: 0},
	},
}

var scriptCyrillic = scriptClass{
	name:     "Cyrillic",
	group:    scriptGroupDefault,
	stdChars: []rune{'\u043E', '\u041E'}, // о О
	blues: []blueSpec{
		{chars: "\u0411 \u0412 \u0415 \u041F \u0417 \u041E \u0421 \u042D", flags: blueZoneTop},
		{chars: "\u0411 \u0412 \u0415 \u0428 \u0417 \u041E \u0421 \u042D", flags: 0},
		{chars: "\u0445 \u043F \u043D \u0448 \u0435 \u0437 \u043E \u0441", flags: blueZoneTop | blueZoneXHeight},
		{chars: "\u0445 \u043F \u043D \u0448 \u0435 \u0437 \u043E \u0441", flags: 0},
		{chars: "\u0440 \u0443 \u0444", flags: 0},
	},
}

var scriptGreek = scriptClass{
	name:     "Greek",
	group:    scriptGroupDefault,
	stdChars: []rune{'\u03BF', '\u039F'}, // ο Ο
	blues: []blueSpec{
		{chars: "\u0393 \u0392 \u0395 \u0396 \u0398 \u039F \u03A9", flags: blueZoneTop},
		{chars: "\u0392 \u0394 \u0396 \u039E \u0398 \u039F", flags: 0},
		{chars: "\u03B2 \u03B8 \u03B4 \u03B6 \u03BB \u03BE", flags: blueZoneTop},
		{chars: "\u03B1 \u03B5 \u03B9 \u03BF \u03C0 \u03C3 \u03C4 \u03C9", flags: blueZoneTop | blueZoneXHeight},
		{chars: "\u03B1 \u03B5 \u03B9 \u03BF \u03C0 \u03C3 \u03C4 \u03C9", flags: 0},
		{chars: "\u03B2 \u03B3 \u03B7 \u03BC \u03C1 \u03C6 \u03C7 \u03C8", flags: 0},
	},
}

var scriptArabic = scriptClass{
	name:     "Arabic",
	group:    scriptGroupDefault,
	stdChars: []rune{'\u0644', '\u062D', '\u0640'}, // ل ح ـ
	blues: []blueSpec{
		{chars: "\u0627 \u0625 \u0644 \u0643 \u0637 \u0638", flags: blueZoneTop},
		{chars: "\u062A \u062B \u0637 \u0638 \u0643", flags: 0},
		{chars: "\u0640", flags: blueZoneNeutral},
	},
}

var scriptCJK = scriptClass{
	name:     "CJKV ideographs",
	group:    scriptGroupCJK,
	stdChars: []rune{'\u7530', '\u56D7'}, // 田 囗
	blues: []blueSpec{
		// CJK vertical blues (the only ones active — horizontal is disabled
		// per FreeType since 2004). The '|' separator divides fill chars from
		// flat chars within the string.
		{
			chars: "\u4ED6 \u4EEC \u4F60 \u4F86 \u5011 \u5230 \u548C \u5730 " +
				"\u5BF9 \u5C0D \u5C31 \u5E2D \u6211 \u65F6 \u6642 \u6703 " +
				"\u6765 \u7232 \u80FD \u8230 \u8AAA \u8BF4 \u8FD9 \u9019 \u9F4A" +
				" | " +
				"\u519B \u540C \u5DF2 \u613F \u65E2 \u661F \u662F \u666F " +
				"\u6C11 \u7167 \u73B0 \u73FE \u7406 \u7528 \u7F6E \u8981 " +
				"\u8ECD \u90A3 \u914D \u91CC \u958B \u96F7 \u9732 \u9762 \u9867",
			flags: blueZoneTop,
		},
		{
			chars: "\u4E2A \u4E3A \u4EBA \u4ED6 \u4EE5 \u4EEC \u4F60 \u4F86 " +
				"\u500B \u5011 \u5230 \u548C \u5927 \u5BF9 \u5C0D \u5C31 " +
				"\u6211 \u65F6 \u6642 \u6709 \u6765 \u7232 \u8981 \u8AAA \u8BF4" +
				" | " +
				"\u4E3B \u4E9B \u56E0 \u5B83 \u60F3 \u610F \u7406 \u751F " +
				"\u7576 \u770B \u7740 \u7F6E \u8005 \u81EA \u8457 \u88E1 " +
				"\u8FC7 \u8FD8 \u8FDB \u9032 \u904E \u9053 \u9084 \u91CC \u9762",
			flags: 0,
		},
	},
}

// scriptClasses lists all supported scripts in priority order for detection.
// Higher priority scripts are checked first. Hebrew/CJK/Cyrillic/Greek/Arabic
// are checked before Latin because Latin characters appear as fallback in many
// non-Latin fonts.
var scriptClasses = []*scriptClass{
	&scriptHebrew,
	&scriptCyrillic,
	&scriptGreek,
	&scriptArabic,
	&scriptCJK,
	&scriptLatin, // default fallback
}

// detectFontScript determines the primary script for a font by scanning
// its cmap for script-specific characters. The first script whose standard
// characters are found in the font wins.
//
// This is a simplified heuristic compared to FreeType/skrifa's full
// per-glyph style mapping, but sufficient for selecting the correct blue
// zone and width characters. The full style mapping would require GSUB
// lookups and per-glyph classification which we don't need for metric
// computation.
//
// Priority order: Hebrew > Cyrillic > Greek > Arabic > CJK > Latin (default).
func detectFontScript(font ParsedFont) *scriptClass {
	for _, sc := range scriptClasses {
		// Check if the font has glyphs for this script's standard characters.
		for _, ch := range sc.stdChars {
			if font.GlyphIndex(ch) != 0 {
				return sc
			}
		}
	}
	// Absolute fallback: Latin (should always match for any reasonable font).
	return &scriptLatin
}
