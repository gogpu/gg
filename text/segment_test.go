package text

import (
	"testing"
)

func TestDetectScript(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want Script
	}{
		// ASCII Latin
		{"Latin uppercase A", 'A', ScriptLatin},
		{"Latin lowercase a", 'a', ScriptLatin},
		{"Latin uppercase Z", 'Z', ScriptLatin},
		{"Latin lowercase z", 'z', ScriptLatin},

		// ASCII Common (numbers, punctuation)
		{"digit 0", '0', ScriptCommon},
		{"digit 9", '9', ScriptCommon},
		{"space", ' ', ScriptCommon},
		{"period", '.', ScriptCommon},
		{"comma", ',', ScriptCommon},
		{"exclamation", '!', ScriptCommon},

		// Latin Extended
		{"Latin e-acute", '\u00E9', ScriptLatin},    // e
		{"Latin n-tilde", '\u00F1', ScriptLatin},    // n
		{"Latin o-umlaut", '\u00F6', ScriptLatin},   // o
		{"Latin A-grave", '\u00C0', ScriptLatin},    // A
		{"Latin s-caron", '\u0161', ScriptLatin},    // s (Latin Extended-A)
		{"Latin o-macron", '\u014D', ScriptLatin},   // o (Latin Extended-A)
		{"Latin dotless i", '\u0131', ScriptLatin},  // i (Latin Extended-A)
		{"Latin a-breve", '\u0103', ScriptLatin},    // a (Latin Extended-A)
		{"Latin o-dblacute", '\u0151', ScriptLatin}, // o (Latin Extended-A)
		{"Latin w-grave", '\u1E81', ScriptLatin},    // w (Latin Extended Additional)
		{"Latin y-hook", '\u1EF5', ScriptLatin},     // y (Latin Extended Additional)

		// Latin-1 Supplement Common characters
		{"non-breaking space", '\u00A0', ScriptCommon},
		{"copyright sign", '\u00A9', ScriptCommon},
		{"registered sign", '\u00AE', ScriptCommon},
		{"degree sign", '\u00B0', ScriptCommon},
		{"section sign", '\u00A7', ScriptCommon},

		// Cyrillic
		{"Cyrillic A", '\u0410', ScriptCyrillic},
		{"Cyrillic a", '\u0430', ScriptCyrillic},
		{"Cyrillic Ya", '\u042F', ScriptCyrillic},
		{"Cyrillic ya", '\u044F', ScriptCyrillic},
		{"Cyrillic Zhe", '\u0416', ScriptCyrillic},
		{"Cyrillic zhe", '\u0436', ScriptCyrillic},
		{"Cyrillic Io", '\u0401', ScriptCyrillic}, // E
		{"Cyrillic io", '\u0451', ScriptCyrillic}, // e
		{"Cyrillic supplement", '\u0500', ScriptCyrillic},

		// Greek
		{"Greek Alpha", '\u0391', ScriptGreek},
		{"Greek alpha", '\u03B1', ScriptGreek},
		{"Greek Omega", '\u03A9', ScriptGreek},
		{"Greek omega", '\u03C9', ScriptGreek},
		{"Greek pi", '\u03C0', ScriptGreek},
		{"Greek extended", '\u1F00', ScriptGreek},

		// Arabic
		{"Arabic Alef", '\u0627', ScriptArabic},
		{"Arabic Ba", '\u0628', ScriptArabic},
		{"Arabic Ya", '\u064A', ScriptArabic},
		{"Arabic comma", '\u060C', ScriptArabic},
		{"Arabic supplement", '\u0750', ScriptArabic},
		{"Arabic Presentation", '\uFB50', ScriptArabic},
		{"Arabic Presentation B", '\uFE70', ScriptArabic},

		// Hebrew
		{"Hebrew Alef", '\u05D0', ScriptHebrew},
		{"Hebrew Bet", '\u05D1', ScriptHebrew},
		{"Hebrew Tav", '\u05EA', ScriptHebrew},
		{"Hebrew mark", '\u05B0', ScriptHebrew},
		{"Hebrew presentation", '\uFB1D', ScriptHebrew},

		// Han/CJK
		{"CJK ideograph yi", '\u4E00', ScriptHan},
		{"CJK ideograph", '\u4E2D', ScriptHan},
		{"CJK ideograph", '\u56FD', ScriptHan},
		{"CJK Extension A", '\u3400', ScriptHan},
		{"CJK Compatibility", '\uF900', ScriptHan},
		{"CJK Radical", '\u2F00', ScriptHan},

		// Hiragana
		{"Hiragana a", '\u3042', ScriptHiragana},
		{"Hiragana i", '\u3044', ScriptHiragana},
		{"Hiragana n", '\u3093', ScriptHiragana},

		// Katakana
		{"Katakana a", '\u30A2', ScriptKatakana},
		{"Katakana i", '\u30A4', ScriptKatakana},
		{"Katakana n", '\u30F3', ScriptKatakana},
		{"Halfwidth Katakana", '\uFF66', ScriptKatakana},

		// Hangul (Korean)
		{"Hangul syllable ga", '\uAC00', ScriptHangul},
		{"Hangul syllable ha", '\uD558', ScriptHangul},
		{"Hangul Jamo", '\u1100', ScriptHangul},
		{"Hangul Compatibility", '\u3130', ScriptHangul},

		// Devanagari
		{"Devanagari Ka", '\u0915', ScriptDevanagari},
		{"Devanagari A", '\u0905', ScriptDevanagari},
		{"Devanagari Virama", '\u094D', ScriptDevanagari},

		// Thai
		{"Thai Ko Kai", '\u0E01', ScriptThai},
		{"Thai Sara A", '\u0E30', ScriptThai},

		// Armenian
		{"Armenian Ayb", '\u0531', ScriptArmenian},
		{"Armenian ayb", '\u0561', ScriptArmenian},

		// Georgian
		{"Georgian An", '\u10D0', ScriptGeorgian},

		// Combining marks (Inherited)
		{"Combining acute", '\u0301', ScriptInherited},
		{"Combining grave", '\u0300', ScriptInherited},
		{"Combining tilde", '\u0303', ScriptInherited},
		{"Combining diaeresis", '\u0308', ScriptInherited},

		// General punctuation
		{"Em dash", '\u2014', ScriptCommon},
		{"En dash", '\u2013', ScriptCommon},
		{"Bullet", '\u2022', ScriptCommon},
		{"Ellipsis", '\u2026', ScriptCommon},

		// Currency and symbols
		{"Euro sign", '\u20AC', ScriptCommon},
		{"Yen sign", '\u00A5', ScriptCommon},
		{"Dollar sign", '$', ScriptCommon},

		// CJK punctuation
		{"CJK period", '\u3002', ScriptCommon},
		{"CJK comma", '\u3001', ScriptCommon},

		// Unknown script
		{"Unknown high plane", '\U0001F600', ScriptUnknown}, // Emoji
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectScript(tt.r)
			if got != tt.want {
				t.Errorf("DetectScript(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestScriptString(t *testing.T) {
	tests := []struct {
		script Script
		want   string
	}{
		{ScriptCommon, "Common"},
		{ScriptInherited, "Inherited"},
		{ScriptLatin, "Latin"},
		{ScriptCyrillic, "Cyrillic"},
		{ScriptGreek, "Greek"},
		{ScriptArabic, "Arabic"},
		{ScriptHebrew, "Hebrew"},
		{ScriptHan, "Han"},
		{ScriptHiragana, "Hiragana"},
		{ScriptKatakana, "Katakana"},
		{ScriptHangul, "Hangul"},
		{ScriptDevanagari, "Devanagari"},
		{ScriptThai, "Thai"},
		{ScriptGeorgian, "Georgian"},
		{ScriptArmenian, "Armenian"},
		{ScriptUnknown, "Unknown"},
		{Script(9999), "Unknown"}, // Invalid script
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.script.String()
			if got != tt.want {
				t.Errorf("Script(%d).String() = %q, want %q", tt.script, got, tt.want)
			}
		})
	}
}

func TestScriptIsRTL(t *testing.T) {
	tests := []struct {
		script Script
		want   bool
	}{
		{ScriptArabic, true},
		{ScriptHebrew, true},
		{ScriptLatin, false},
		{ScriptCyrillic, false},
		{ScriptHan, false},
		{ScriptCommon, false},
	}

	for _, tt := range tests {
		t.Run(tt.script.String(), func(t *testing.T) {
			got := tt.script.IsRTL()
			if got != tt.want {
				t.Errorf("Script(%v).IsRTL() = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

func TestScriptRequiresComplexShaping(t *testing.T) {
	tests := []struct {
		script Script
		want   bool
	}{
		{ScriptArabic, true},
		{ScriptHebrew, true},
		{ScriptDevanagari, true},
		{ScriptThai, true},
		{ScriptLatin, false},
		{ScriptCyrillic, false},
		{ScriptHan, false},
		{ScriptHiragana, false},
		{ScriptKatakana, false},
		{ScriptHangul, false},
		{ScriptCommon, false},
	}

	for _, tt := range tests {
		t.Run(tt.script.String(), func(t *testing.T) {
			got := tt.script.RequiresComplexShaping()
			if got != tt.want {
				t.Errorf("Script(%v).RequiresComplexShaping() = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

func TestSegmentText_Empty(t *testing.T) {
	segments := SegmentText("")
	if segments != nil {
		t.Errorf("SegmentText(\"\") = %v, want nil", segments)
	}
}

func TestSegmentText_PureLatin(t *testing.T) {
	text := "Hello World"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Text != text {
		t.Errorf("segment text = %q, want %q", seg.Text, text)
	}
	if seg.Direction != DirectionLTR {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionLTR)
	}
	if seg.Script != ScriptLatin {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptLatin)
	}
	if seg.Start != 0 {
		t.Errorf("segment start = %d, want 0", seg.Start)
	}
	if seg.End != len(text) {
		t.Errorf("segment end = %d, want %d", seg.End, len(text))
	}
}

func TestSegmentText_PureCyrillic(t *testing.T) {
	text := "Привет мир"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d: %+v", len(segments), segments)
	}

	seg := segments[0]
	if seg.Text != text {
		t.Errorf("segment text = %q, want %q", seg.Text, text)
	}
	if seg.Direction != DirectionLTR {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionLTR)
	}
	if seg.Script != ScriptCyrillic {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptCyrillic)
	}
}

func TestSegmentText_PureArabic(t *testing.T) {
	text := "مرحبا"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Text != text {
		t.Errorf("segment text = %q, want %q", seg.Text, text)
	}
	if seg.Direction != DirectionRTL {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionRTL)
	}
	if seg.Script != ScriptArabic {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptArabic)
	}
}

func TestSegmentText_PureHebrew(t *testing.T) {
	text := "שלום"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Text != text {
		t.Errorf("segment text = %q, want %q", seg.Text, text)
	}
	if seg.Direction != DirectionRTL {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionRTL)
	}
	if seg.Script != ScriptHebrew {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptHebrew)
	}
}

func TestSegmentText_MixedLatinArabic(t *testing.T) {
	// "Hello مرحبا World"
	text := "Hello مرحبا World"
	segments := SegmentText(text)

	// Should have at least 3 segments: Latin, Arabic, Latin
	if len(segments) < 3 {
		t.Fatalf("expected at least 3 segments, got %d: %+v", len(segments), segments)
	}

	// Check that we have Latin and Arabic segments
	hasLatin := false
	hasArabic := false
	for _, seg := range segments {
		if seg.Script == ScriptLatin {
			hasLatin = true
			if seg.Direction != DirectionLTR {
				t.Errorf("Latin segment direction = %v, want LTR", seg.Direction)
			}
		}
		if seg.Script == ScriptArabic {
			hasArabic = true
			if seg.Direction != DirectionRTL {
				t.Errorf("Arabic segment direction = %v, want RTL", seg.Direction)
			}
		}
	}

	if !hasLatin {
		t.Error("expected Latin segment")
	}
	if !hasArabic {
		t.Error("expected Arabic segment")
	}

	// Verify total text reconstruction
	reconstructed := ""
	for _, seg := range segments {
		reconstructed += seg.Text
	}
	if reconstructed != text {
		t.Errorf("reconstructed text = %q, want %q", reconstructed, text)
	}
}

func TestSegmentText_HebrewWithNumbers(t *testing.T) {
	// Hebrew text with numbers: "Price: 100"
	text := "מחיר: 100"
	segments := SegmentText(text)

	// Verify text reconstruction
	reconstructed := ""
	for _, seg := range segments {
		reconstructed += seg.Text
	}
	if reconstructed != text {
		t.Errorf("reconstructed text = %q, want %q", reconstructed, text)
	}

	// Should have at least one Hebrew segment with RTL direction
	// Note: Numbers inherit Hebrew script but remain LTR per Unicode bidi rules
	hasHebrewRTL := false
	for _, seg := range segments {
		if seg.Script == ScriptHebrew && seg.Direction == DirectionRTL {
			hasHebrewRTL = true
			break
		}
	}
	if !hasHebrewRTL {
		t.Error("expected Hebrew segment with RTL direction")
	}

	// First segment should be the Hebrew text (RTL)
	if len(segments) > 0 {
		first := segments[0]
		if first.Script != ScriptHebrew {
			t.Errorf("first segment script = %v, want Hebrew", first.Script)
		}
		if first.Direction != DirectionRTL {
			t.Errorf("first segment direction = %v, want RTL", first.Direction)
		}
	}
}

func TestSegmentText_JapaneseMixed(t *testing.T) {
	// Japanese with kanji, hiragana, katakana: "Tokyo" in Japanese
	text := "東京はトウキョウです"
	segments := SegmentText(text)

	if len(segments) == 0 {
		t.Fatal("expected at least 1 segment")
	}

	// All should be LTR
	for i, seg := range segments {
		if seg.Direction != DirectionLTR {
			t.Errorf("segment %d direction = %v, want LTR", i, seg.Direction)
		}
	}

	// Check for expected scripts
	scripts := make(map[Script]bool)
	for _, seg := range segments {
		scripts[seg.Script] = true
	}

	if !scripts[ScriptHan] && !scripts[ScriptHiragana] && !scripts[ScriptKatakana] {
		t.Error("expected at least one CJK script (Han, Hiragana, or Katakana)")
	}

	// Verify text reconstruction
	reconstructed := ""
	for _, seg := range segments {
		reconstructed += seg.Text
	}
	if reconstructed != text {
		t.Errorf("reconstructed text = %q, want %q", reconstructed, text)
	}
}

func TestSegmentText_LatinCyrillic(t *testing.T) {
	// Mixed Latin and Cyrillic
	text := "Hello Привет"
	segments := SegmentText(text)

	if len(segments) < 2 {
		t.Fatalf("expected at least 2 segments, got %d: %+v", len(segments), segments)
	}

	hasLatin := false
	hasCyrillic := false
	for _, seg := range segments {
		if seg.Script == ScriptLatin {
			hasLatin = true
		}
		if seg.Script == ScriptCyrillic {
			hasCyrillic = true
		}
		// Both should be LTR
		if seg.Direction != DirectionLTR {
			t.Errorf("segment direction = %v, want LTR", seg.Direction)
		}
	}

	if !hasLatin {
		t.Error("expected Latin segment")
	}
	if !hasCyrillic {
		t.Error("expected Cyrillic segment")
	}
}

func TestSegmentText_Greek(t *testing.T) {
	text := "Γειά σου"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Script != ScriptGreek {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptGreek)
	}
	if seg.Direction != DirectionLTR {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionLTR)
	}
}

func TestSegmentText_Korean(t *testing.T) {
	text := "안녕하세요"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Script != ScriptHangul {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptHangul)
	}
	if seg.Direction != DirectionLTR {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionLTR)
	}
}

func TestSegmentText_Thai(t *testing.T) {
	text := "สวัสดี"
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Script != ScriptThai {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptThai)
	}
	if seg.Direction != DirectionLTR {
		t.Errorf("segment direction = %v, want %v", seg.Direction, DirectionLTR)
	}
}

func TestSegmentText_RTLBase(t *testing.T) {
	// Test with RTL base direction
	text := "Hello World"
	segments := SegmentTextRTL(text)

	if len(segments) == 0 {
		t.Fatal("expected at least 1 segment")
	}

	// With RTL base, Latin text should still be LTR within an RTL context
	// The bidi algorithm handles embedding
}

func TestSegmentRuneCount(t *testing.T) {
	seg := Segment{
		Text: "Hello",
	}
	if seg.RuneCount() != 5 {
		t.Errorf("RuneCount() = %d, want 5", seg.RuneCount())
	}

	seg.Text = "мир"
	if seg.RuneCount() != 3 {
		t.Errorf("RuneCount() = %d, want 3", seg.RuneCount())
	}

	seg.Text = "日本"
	if seg.RuneCount() != 2 {
		t.Errorf("RuneCount() = %d, want 2", seg.RuneCount())
	}
}

func TestBuiltinSegmenter_NewWithDirection(t *testing.T) {
	segLTR := NewBuiltinSegmenter()
	if segLTR.BaseDirection != DirectionLTR {
		t.Errorf("BaseDirection = %v, want %v", segLTR.BaseDirection, DirectionLTR)
	}

	segRTL := NewBuiltinSegmenterWithDirection(DirectionRTL)
	if segRTL.BaseDirection != DirectionRTL {
		t.Errorf("BaseDirection = %v, want %v", segRTL.BaseDirection, DirectionRTL)
	}
}

func TestSegmentText_ByteOffsets(t *testing.T) {
	// Text with multi-byte characters
	text := "ABCшюя123"
	segments := SegmentText(text)

	// Verify byte offsets are correct
	lastEnd := 0
	for i, seg := range segments {
		if seg.Start != lastEnd {
			t.Errorf("segment %d start = %d, want %d", i, seg.Start, lastEnd)
		}
		if seg.End <= seg.Start {
			t.Errorf("segment %d end (%d) <= start (%d)", i, seg.End, seg.Start)
		}
		if text[seg.Start:seg.End] != seg.Text {
			t.Errorf("segment %d text mismatch: offsets give %q, Text is %q",
				i, text[seg.Start:seg.End], seg.Text)
		}
		lastEnd = seg.End
	}

	if lastEnd != len(text) {
		t.Errorf("last segment end = %d, want %d", lastEnd, len(text))
	}
}

func TestIsWhitespace(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'\u00A0', true}, // Non-breaking space
		{'a', false},
		{'1', false},
		{'.', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := IsWhitespace(tt.r)
			if got != tt.want {
				t.Errorf("IsWhitespace(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestIsPunctuation(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'.', true},
		{',', true},
		{'!', true},
		{'?', true},
		{';', true},
		{':', true},
		{'-', true},
		{'(', true},
		{')', true},
		{'a', false},
		{'1', false},
		{' ', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := IsPunctuation(tt.r)
			if got != tt.want {
				t.Errorf("IsPunctuation(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestSegmentText_CombiningMarks(t *testing.T) {
	// "e" with combining acute accent
	text := "cafe\u0301" // cafe with combining acute on the 'e'
	segments := SegmentText(text)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d: %+v", len(segments), segments)
	}

	seg := segments[0]
	if seg.Script != ScriptLatin {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptLatin)
	}

	// Verify the combining mark is included
	if seg.Text != text {
		t.Errorf("segment text = %q, want %q", seg.Text, text)
	}
}

func TestSegmentText_EmptyRunes(t *testing.T) {
	segmenter := NewBuiltinSegmenter()
	segments := segmenter.Segment("")
	if segments != nil {
		t.Errorf("Segment(\"\") = %v, want nil", segments)
	}
}

func TestSegmentText_PunctInContext(t *testing.T) {
	// Test punctuation surrounded by same script
	text := "Hello, World!"
	segments := SegmentText(text)

	// All should merge into Latin because punctuation is surrounded by Latin
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment (punctuation resolved to Latin), got %d: %+v",
			len(segments), segments)
	}

	seg := segments[0]
	if seg.Script != ScriptLatin {
		t.Errorf("segment script = %v, want %v", seg.Script, ScriptLatin)
	}
}

func TestDetectScript_ExtendedRanges(t *testing.T) {
	// Test various script ranges for coverage
	tests := []struct {
		name   string
		ranges []rune
		want   Script
	}{
		{"IPA Extensions", []rune{0x0250, 0x02AF}, ScriptLatin},
		{"Cyrillic Extended-A", []rune{0x2DE0}, ScriptCyrillic},
		{"Cyrillic Extended-B", []rune{0xA640}, ScriptCyrillic},
		{"Greek Extended", []rune{0x1F00}, ScriptGreek},
		{"Bengali", []rune{0x0980, 0x09FF}, ScriptBengali},
		{"Gurmukhi", []rune{0x0A00}, ScriptGurmukhi},
		{"Gujarati", []rune{0x0A80}, ScriptGujarati},
		{"Oriya", []rune{0x0B00}, ScriptOriya},
		{"Tamil", []rune{0x0B80}, ScriptTamil},
		{"Telugu", []rune{0x0C00}, ScriptTelugu},
		{"Kannada", []rune{0x0C80}, ScriptKannada},
		{"Malayalam", []rune{0x0D00}, ScriptMalayalam},
		{"Sinhala", []rune{0x0D80}, ScriptSinhala},
		{"Lao", []rune{0x0E80}, ScriptLao},
		{"Tibetan", []rune{0x0F00}, ScriptTibetan},
		{"Myanmar", []rune{0x1000}, ScriptMyanmar},
		{"Georgian", []rune{0x10A0, 0x10D0}, ScriptGeorgian},
		{"Ethiopic", []rune{0x1200}, ScriptEthiopic},
		{"Khmer", []rune{0x1780}, ScriptKhmer},
		{"Hangul Jamo", []rune{0x1100}, ScriptHangul},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, r := range tt.ranges {
				got := DetectScript(r)
				if got != tt.want {
					t.Errorf("DetectScript(%#x) = %v, want %v", r, got, tt.want)
				}
			}
		})
	}
}

func TestSegmentText_ComplexMixed(t *testing.T) {
	// Complex mixed text: English, Arabic, Hebrew
	text := "Start مرحبا Middle שלום End"
	segments := SegmentText(text)

	// Reconstruct and verify
	reconstructed := ""
	for _, seg := range segments {
		reconstructed += seg.Text
	}
	if reconstructed != text {
		t.Errorf("reconstructed = %q, want %q", reconstructed, text)
	}

	// Count scripts
	scriptCount := make(map[Script]int)
	for _, seg := range segments {
		scriptCount[seg.Script]++
	}

	// Should have Latin, Arabic, and Hebrew
	if scriptCount[ScriptLatin] < 1 {
		t.Error("expected Latin segments")
	}
	if scriptCount[ScriptArabic] < 1 {
		t.Error("expected Arabic segment")
	}
	if scriptCount[ScriptHebrew] < 1 {
		t.Error("expected Hebrew segment")
	}
}

func TestDetectScript_NumbersAndSymbols(t *testing.T) {
	// Numbers and symbols should be Common
	tests := []rune{
		'0', '1', '9',
		'+', '-', '*', '/',
		'=', '<', '>',
		'@', '#', '$', '%', '^', '&',
		'[', ']', '{', '}',
	}

	for _, r := range tests {
		got := DetectScript(r)
		if got != ScriptCommon {
			t.Errorf("DetectScript(%q) = %v, want %v", r, got, ScriptCommon)
		}
	}
}

func TestDetectScript_CJKPunctuation(t *testing.T) {
	// CJK punctuation should be Common
	tests := []rune{
		'\u3000', // Ideographic space
		'\u3001', // Ideographic comma
		'\u3002', // Ideographic full stop
	}

	for _, r := range tests {
		got := DetectScript(r)
		if got != ScriptCommon {
			t.Errorf("DetectScript(%#x) = %v, want %v", r, got, ScriptCommon)
		}
	}
}

func TestDetectScript_Fullwidth(t *testing.T) {
	// Fullwidth punctuation should be Common
	tests := []rune{
		'\uFF01', // Fullwidth exclamation
		'\uFF0C', // Fullwidth comma
	}

	for _, r := range tests {
		got := DetectScript(r)
		if got != ScriptCommon {
			t.Errorf("DetectScript(%#x) = %v, want %v", r, got, ScriptCommon)
		}
	}
}

func TestSegmentText_SingleCharacter(t *testing.T) {
	tests := []struct {
		text   string
		script Script
		dir    Direction
	}{
		{"A", ScriptLatin, DirectionLTR},
		{"Я", ScriptCyrillic, DirectionLTR},
		{"α", ScriptGreek, DirectionLTR},
		{"ا", ScriptArabic, DirectionRTL},
		{"א", ScriptHebrew, DirectionRTL},
		{"中", ScriptHan, DirectionLTR},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			segments := SegmentText(tt.text)
			if len(segments) != 1 {
				t.Fatalf("expected 1 segment, got %d", len(segments))
			}
			seg := segments[0]
			if seg.Script != tt.script {
				t.Errorf("script = %v, want %v", seg.Script, tt.script)
			}
			if seg.Direction != tt.dir {
				t.Errorf("direction = %v, want %v", seg.Direction, tt.dir)
			}
		})
	}
}

// Benchmark tests
func BenchmarkDetectScript_ASCII(b *testing.B) {
	for b.Loop() {
		DetectScript('A')
	}
}

func BenchmarkDetectScript_Unicode(b *testing.B) {
	for b.Loop() {
		DetectScript('\u4E00') // CJK
	}
}

func BenchmarkSegmentText_Latin(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog."
	for b.Loop() {
		_ = SegmentText(text)
	}
}

func BenchmarkSegmentText_Mixed(b *testing.B) {
	text := "Hello مرحبا World שלום"
	for b.Loop() {
		_ = SegmentText(text)
	}
}

func BenchmarkSegmentText_Long(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog. " +
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
		"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."
	for b.Loop() {
		_ = SegmentText(text)
	}
}
