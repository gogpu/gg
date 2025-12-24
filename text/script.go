package text

// Script represents a Unicode script for text segmentation.
// Scripts are used to identify runs of text that should be shaped together.
type Script uint32

// Script constants for common Unicode scripts.
// The values are based on Unicode script codes but simplified for our use case.
const (
	// ScriptCommon is used for punctuation, numbers, and symbols shared across scripts.
	ScriptCommon Script = iota
	// ScriptInherited is used for combining marks that inherit the script of the base character.
	ScriptInherited
	// ScriptLatin is used for Latin-based scripts (English, French, German, etc.)
	ScriptLatin
	// ScriptCyrillic is used for Cyrillic script (Russian, Ukrainian, Bulgarian, etc.)
	ScriptCyrillic
	// ScriptGreek is used for Greek script.
	ScriptGreek
	// ScriptArabic is used for Arabic script (Arabic, Persian, Urdu, etc.)
	ScriptArabic
	// ScriptHebrew is used for Hebrew script.
	ScriptHebrew
	// ScriptHan is used for Chinese/Japanese Kanji characters.
	ScriptHan
	// ScriptHiragana is used for Japanese Hiragana.
	ScriptHiragana
	// ScriptKatakana is used for Japanese Katakana.
	ScriptKatakana
	// ScriptHangul is used for Korean script.
	ScriptHangul
	// ScriptDevanagari is used for Devanagari script (Hindi, Sanskrit, etc.)
	ScriptDevanagari
	// ScriptThai is used for Thai script.
	ScriptThai
	// ScriptGeorgian is used for Georgian script.
	ScriptGeorgian
	// ScriptArmenian is used for Armenian script.
	ScriptArmenian
	// ScriptBengali is used for Bengali script.
	ScriptBengali
	// ScriptTamil is used for Tamil script.
	ScriptTamil
	// ScriptTelugu is used for Telugu script.
	ScriptTelugu
	// ScriptKannada is used for Kannada script.
	ScriptKannada
	// ScriptMalayalam is used for Malayalam script.
	ScriptMalayalam
	// ScriptGujarati is used for Gujarati script.
	ScriptGujarati
	// ScriptOriya is used for Oriya script.
	ScriptOriya
	// ScriptGurmukhi is used for Gurmukhi script (Punjabi).
	ScriptGurmukhi
	// ScriptSinhala is used for Sinhala script.
	ScriptSinhala
	// ScriptKhmer is used for Khmer script (Cambodian).
	ScriptKhmer
	// ScriptLao is used for Lao script.
	ScriptLao
	// ScriptMyanmar is used for Myanmar (Burmese) script.
	ScriptMyanmar
	// ScriptTibetan is used for Tibetan script.
	ScriptTibetan
	// ScriptEthiopic is used for Ethiopic script.
	ScriptEthiopic
	// ScriptUnknown is used for unrecognized scripts.
	ScriptUnknown
)

// scriptNames maps Script values to their string names.
var scriptNames = [...]string{
	ScriptCommon:     "Common",
	ScriptInherited:  "Inherited",
	ScriptLatin:      "Latin",
	ScriptCyrillic:   "Cyrillic",
	ScriptGreek:      "Greek",
	ScriptArabic:     "Arabic",
	ScriptHebrew:     "Hebrew",
	ScriptHan:        "Han",
	ScriptHiragana:   "Hiragana",
	ScriptKatakana:   "Katakana",
	ScriptHangul:     "Hangul",
	ScriptDevanagari: "Devanagari",
	ScriptThai:       "Thai",
	ScriptGeorgian:   "Georgian",
	ScriptArmenian:   "Armenian",
	ScriptBengali:    "Bengali",
	ScriptTamil:      "Tamil",
	ScriptTelugu:     "Telugu",
	ScriptKannada:    "Kannada",
	ScriptMalayalam:  "Malayalam",
	ScriptGujarati:   "Gujarati",
	ScriptOriya:      "Oriya",
	ScriptGurmukhi:   "Gurmukhi",
	ScriptSinhala:    "Sinhala",
	ScriptKhmer:      "Khmer",
	ScriptLao:        "Lao",
	ScriptMyanmar:    "Myanmar",
	ScriptTibetan:    "Tibetan",
	ScriptEthiopic:   "Ethiopic",
	ScriptUnknown:    "Unknown",
}

// String returns the name of the script.
func (s Script) String() string {
	if int(s) < len(scriptNames) {
		return scriptNames[s]
	}
	return unknownStr
}

// IsRTL returns true if the script is typically written right-to-left.
func (s Script) IsRTL() bool {
	return s == ScriptArabic || s == ScriptHebrew
}

// RequiresComplexShaping returns true if the script typically needs
// advanced shaping features (ligatures, contextual forms, etc.)
// that are not supported by BuiltinShaper.
func (s Script) RequiresComplexShaping() bool {
	switch s {
	case ScriptArabic, ScriptHebrew, ScriptDevanagari, ScriptBengali,
		ScriptTamil, ScriptTelugu, ScriptKannada, ScriptMalayalam,
		ScriptGujarati, ScriptOriya, ScriptGurmukhi, ScriptSinhala,
		ScriptKhmer, ScriptLao, ScriptMyanmar, ScriptTibetan, ScriptThai:
		return true
	default:
		return false
	}
}

// DetectScript returns the Unicode script for a given rune.
// This uses hardcoded Unicode ranges for common scripts to avoid
// external dependencies.
//
// For characters that appear in multiple scripts or are shared
// (like punctuation and numbers), ScriptCommon is returned.
// For combining marks, ScriptInherited is returned.
func DetectScript(r rune) Script {
	// ASCII optimization for common case
	if r < 0x0080 {
		return detectASCII(r)
	}

	// Check script ranges in order of frequency/usage
	if script := detectEuropean(r); script != ScriptUnknown {
		return script
	}
	if script := detectMiddleEastern(r); script != ScriptUnknown {
		return script
	}
	if script := detectSouthAsian(r); script != ScriptUnknown {
		return script
	}
	if script := detectEastAsian(r); script != ScriptUnknown {
		return script
	}
	if script := detectSoutheastAsian(r); script != ScriptUnknown {
		return script
	}
	if script := detectOther(r); script != ScriptUnknown {
		return script
	}
	if script := detectCommonSymbols(r); script != ScriptUnknown {
		return script
	}

	return ScriptUnknown
}

// detectASCII handles the ASCII range (0x0000-0x007F) which is the most common case.
func detectASCII(r rune) Script {
	if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
		return ScriptLatin
	}
	// Digits, punctuation, control characters
	return ScriptCommon
}

// detectEuropean handles European scripts: Latin, Greek, Cyrillic, Armenian, Georgian.
func detectEuropean(r rune) Script {
	switch {
	// Latin Extended blocks
	case r >= 0x0080 && r <= 0x00FF:
		return detectLatin1Supplement(r)
	case r >= 0x0100 && r <= 0x024F: // Latin Extended-A and B
		return ScriptLatin
	case r >= 0x0250 && r <= 0x02AF: // IPA Extensions (Latin-based)
		return ScriptLatin
	case r >= 0x1E00 && r <= 0x1EFF: // Latin Extended Additional
		return ScriptLatin
	case r >= 0x2C60 && r <= 0x2C7F: // Latin Extended-C
		return ScriptLatin
	case r >= 0xA720 && r <= 0xA7FF: // Latin Extended-D
		return ScriptLatin

	// Combining marks (Inherited script)
	case r >= 0x0300 && r <= 0x036F: // Combining Diacritical Marks
		return ScriptInherited
	case r >= 0x1AB0 && r <= 0x1AFF: // Combining Diacritical Marks Extended
		return ScriptInherited
	case r >= 0x1DC0 && r <= 0x1DFF: // Combining Diacritical Marks Supplement
		return ScriptInherited
	case r >= 0x20D0 && r <= 0x20FF: // Combining Diacritical Marks for Symbols
		return ScriptInherited
	case r >= 0xFE20 && r <= 0xFE2F: // Combining Half Marks
		return ScriptInherited

	// Greek
	case r >= 0x0370 && r <= 0x03FF: // Greek and Coptic
		return ScriptGreek
	case r >= 0x1F00 && r <= 0x1FFF: // Greek Extended
		return ScriptGreek

	// Cyrillic
	case r >= 0x0400 && r <= 0x04FF: // Cyrillic
		return ScriptCyrillic
	case r >= 0x0500 && r <= 0x052F: // Cyrillic Supplement
		return ScriptCyrillic
	case r >= 0x2DE0 && r <= 0x2DFF: // Cyrillic Extended-A
		return ScriptCyrillic
	case r >= 0xA640 && r <= 0xA69F: // Cyrillic Extended-B
		return ScriptCyrillic

	// Armenian
	case r >= 0x0530 && r <= 0x058F:
		return ScriptArmenian

	// Georgian
	case r >= 0x10A0 && r <= 0x10FF:
		return ScriptGeorgian
	case r >= 0x2D00 && r <= 0x2D2F: // Georgian Supplement
		return ScriptGeorgian

	default:
		return ScriptUnknown
	}
}

// detectMiddleEastern handles Middle Eastern scripts: Hebrew, Arabic.
func detectMiddleEastern(r rune) Script {
	switch {
	// Hebrew
	case r >= 0x0590 && r <= 0x05FF:
		return ScriptHebrew
	case r >= 0xFB00 && r <= 0xFB4F: // Alphabetic Presentation Forms
		if r >= 0xFB1D {
			return ScriptHebrew
		}
		return ScriptLatin // Latin ligatures fi, fl, etc.

	// Arabic
	case r >= 0x0600 && r <= 0x06FF: // Arabic
		return ScriptArabic
	case r >= 0x0750 && r <= 0x077F: // Arabic Supplement
		return ScriptArabic
	case r >= 0x08A0 && r <= 0x08FF: // Arabic Extended-A
		return ScriptArabic
	case r >= 0xFB50 && r <= 0xFDFF: // Arabic Presentation Forms-A
		return ScriptArabic
	case r >= 0xFE70 && r <= 0xFEFF: // Arabic Presentation Forms-B
		return ScriptArabic

	default:
		return ScriptUnknown
	}
}

// detectSouthAsian handles South Asian scripts: Devanagari, Bengali, etc.
func detectSouthAsian(r rune) Script {
	switch {
	// Devanagari
	case r >= 0x0900 && r <= 0x097F:
		return ScriptDevanagari
	case r >= 0xA8E0 && r <= 0xA8FF: // Devanagari Extended
		return ScriptDevanagari

	// Bengali
	case r >= 0x0980 && r <= 0x09FF:
		return ScriptBengali

	// Gurmukhi
	case r >= 0x0A00 && r <= 0x0A7F:
		return ScriptGurmukhi

	// Gujarati
	case r >= 0x0A80 && r <= 0x0AFF:
		return ScriptGujarati

	// Oriya
	case r >= 0x0B00 && r <= 0x0B7F:
		return ScriptOriya

	// Tamil
	case r >= 0x0B80 && r <= 0x0BFF:
		return ScriptTamil

	// Telugu
	case r >= 0x0C00 && r <= 0x0C7F:
		return ScriptTelugu

	// Kannada
	case r >= 0x0C80 && r <= 0x0CFF:
		return ScriptKannada

	// Malayalam
	case r >= 0x0D00 && r <= 0x0D7F:
		return ScriptMalayalam

	// Sinhala
	case r >= 0x0D80 && r <= 0x0DFF:
		return ScriptSinhala

	default:
		return ScriptUnknown
	}
}

// detectEastAsian handles East Asian scripts: CJK, Hiragana, Katakana, Hangul.
func detectEastAsian(r rune) Script {
	switch {
	// Hangul (Korean)
	case r >= 0x1100 && r <= 0x11FF: // Hangul Jamo
		return ScriptHangul
	case r >= 0x3130 && r <= 0x318F: // Hangul Compatibility Jamo
		return ScriptHangul
	case r >= 0xA960 && r <= 0xA97F: // Hangul Jamo Extended-A
		return ScriptHangul
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
		return ScriptHangul
	case r >= 0xD7B0 && r <= 0xD7FF: // Hangul Jamo Extended-B
		return ScriptHangul

	// Hiragana
	case r >= 0x3040 && r <= 0x309F:
		return ScriptHiragana
	case r >= 0x1B000 && r <= 0x1B0FF: // Kana Supplement
		return ScriptHiragana

	// Katakana
	case r >= 0x30A0 && r <= 0x30FF:
		return ScriptKatakana
	case r >= 0x31F0 && r <= 0x31FF: // Katakana Phonetic Extensions
		return ScriptKatakana
	case r >= 0xFF65 && r <= 0xFF9F: // Halfwidth Katakana
		return ScriptKatakana

	// CJK/Han
	case r >= 0x2E80 && r <= 0x2EFF: // CJK Radicals Supplement
		return ScriptHan
	case r >= 0x2F00 && r <= 0x2FDF: // Kangxi Radicals
		return ScriptHan
	case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
		return ScriptHan
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return ScriptHan
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
		return ScriptHan
	case r >= 0x20000 && r <= 0x2A6DF: // CJK Extension B
		return ScriptHan
	case r >= 0x2A700 && r <= 0x2B73F: // CJK Extension C
		return ScriptHan
	case r >= 0x2B740 && r <= 0x2B81F: // CJK Extension D
		return ScriptHan

	default:
		return ScriptUnknown
	}
}

// detectSoutheastAsian handles Southeast Asian scripts: Thai, Lao, Khmer, Myanmar, Tibetan.
func detectSoutheastAsian(r rune) Script {
	switch {
	// Thai
	case r >= 0x0E00 && r <= 0x0E7F:
		return ScriptThai

	// Lao
	case r >= 0x0E80 && r <= 0x0EFF:
		return ScriptLao

	// Tibetan
	case r >= 0x0F00 && r <= 0x0FFF:
		return ScriptTibetan

	// Myanmar
	case r >= 0x1000 && r <= 0x109F:
		return ScriptMyanmar
	case r >= 0xAA60 && r <= 0xAA7F: // Myanmar Extended-A
		return ScriptMyanmar

	// Khmer
	case r >= 0x1780 && r <= 0x17FF:
		return ScriptKhmer
	case r >= 0x19E0 && r <= 0x19FF: // Khmer Symbols
		return ScriptKhmer

	default:
		return ScriptUnknown
	}
}

// detectOther handles other scripts: Ethiopic.
func detectOther(r rune) Script {
	switch {
	// Ethiopic
	case r >= 0x1200 && r <= 0x137F:
		return ScriptEthiopic
	case r >= 0x1380 && r <= 0x139F: // Ethiopic Supplement
		return ScriptEthiopic
	case r >= 0x2D80 && r <= 0x2DDF: // Ethiopic Extended
		return ScriptEthiopic

	default:
		return ScriptUnknown
	}
}

// detectCommonSymbols handles common symbols, punctuation, and technical characters.
func detectCommonSymbols(r rune) Script {
	switch {
	case r >= 0x2000 && r <= 0x206F: // General Punctuation
		return ScriptCommon
	case r >= 0x2070 && r <= 0x209F: // Superscripts and Subscripts
		return ScriptCommon
	case r >= 0x20A0 && r <= 0x20CF: // Currency Symbols
		return ScriptCommon
	case r >= 0x2100 && r <= 0x214F: // Letterlike Symbols
		return ScriptCommon
	case r >= 0x2150 && r <= 0x218F: // Number Forms
		return ScriptCommon
	case r >= 0x2190 && r <= 0x21FF: // Arrows
		return ScriptCommon
	case r >= 0x2200 && r <= 0x22FF: // Mathematical Operators
		return ScriptCommon
	case r >= 0x2300 && r <= 0x23FF: // Miscellaneous Technical
		return ScriptCommon
	case r >= 0x2500 && r <= 0x257F: // Box Drawing
		return ScriptCommon
	case r >= 0x2580 && r <= 0x259F: // Block Elements
		return ScriptCommon
	case r >= 0x25A0 && r <= 0x25FF: // Geometric Shapes
		return ScriptCommon
	case r >= 0x2600 && r <= 0x26FF: // Miscellaneous Symbols
		return ScriptCommon
	case r >= 0x2700 && r <= 0x27BF: // Dingbats
		return ScriptCommon
	case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation
		return ScriptCommon
	case r >= 0xFF00 && r <= 0xFF64: // Halfwidth/Fullwidth (punctuation)
		return ScriptCommon
	case r >= 0xFFA0 && r <= 0xFFEF: // Halfwidth/Fullwidth (rest)
		return ScriptCommon

	default:
		return ScriptUnknown
	}
}

// detectLatin1Supplement handles the Latin-1 Supplement block (0x0080-0x00FF)
// which contains a mix of Latin letters and punctuation/symbols.
func detectLatin1Supplement(r rune) Script {
	// Latin letters with diacritics
	if (r >= 0x00C0 && r <= 0x00D6) ||
		(r >= 0x00D8 && r <= 0x00F6) ||
		(r >= 0x00F8 && r <= 0x00FF) {
		return ScriptLatin
	}
	// Punctuation, symbols, control characters
	return ScriptCommon
}
