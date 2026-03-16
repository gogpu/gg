package emoji

import (
	"testing"
)

func TestIsEmoji(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"grinning face", 0x1F600, true},
		{"thumbs up", 0x1F44D, true},
		{"red heart", 0x2764, true}, // text presentation but still emoji
		{"letter A", 'A', false},
		{"digit 1", '1', false},
		{"space", ' ', false},
		{"skin tone light", 0x1F3FB, true},
		{"regional A", 0x1F1E6, true},
		{"ZWJ", 0x200D, true},
		{"variation selector", 0xFE0F, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmoji(tt.rune)
			if got != tt.want {
				t.Errorf("IsEmoji(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsEmojiModifier(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"light skin", 0x1F3FB, true},
		{"medium-light", 0x1F3FC, true},
		{"medium", 0x1F3FD, true},
		{"medium-dark", 0x1F3FE, true},
		{"dark", 0x1F3FF, true},
		{"before range", 0x1F3FA, false},
		{"after range", 0x1F400, false},
		{"letter", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmojiModifier(tt.rune)
			if got != tt.want {
				t.Errorf("IsEmojiModifier(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsEmojiModifierBase(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"man", 0x1F468, true},
		{"woman", 0x1F469, true},
		{"waving hand", 0x1F44B, false}, // Not in our simplified list
		{"thumbs up", 0x1F44D, false},   // Not in our simplified list
		{"letter", 'A', false},
		{"person facepalm", 0x1F926, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmojiModifierBase(tt.rune)
			if got != tt.want {
				t.Errorf("IsEmojiModifierBase(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsZWJ(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"ZWJ", 0x200D, true},
		{"ZWNJ", 0x200C, false},
		{"space", ' ', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsZWJ(tt.rune)
			if got != tt.want {
				t.Errorf("IsZWJ(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsRegionalIndicator(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"RI A", 0x1F1E6, true},
		{"RI Z", 0x1F1FF, true},
		{"before range", 0x1F1E5, false},
		{"after range", 0x1F200, false},
		{"letter A", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRegionalIndicator(tt.rune)
			if got != tt.want {
				t.Errorf("IsRegionalIndicator(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsVariationSelector(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"text VS", 0xFE0E, true},
		{"emoji VS", 0xFE0F, true},
		{"before range", 0xFE0D, false},
		{"after range", 0xFE10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsVariationSelector(tt.rune)
			if got != tt.want {
				t.Errorf("IsVariationSelector(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsKeycapBase(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"digit 0", '0', true},
		{"digit 5", '5', true},
		{"digit 9", '9', true},
		{"hash", '#', true},
		{"asterisk", '*', true},
		{"letter A", 'A', false},
		{"at sign", '@', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsKeycapBase(tt.rune)
			if got != tt.want {
				t.Errorf("IsKeycapBase(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsTagCharacter(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"tag space", 0xE0020, true},
		{"tag A", 0xE0041, true},
		{"tag z", 0xE007A, true},
		{"tag tilde", 0xE007E, true},
		{"cancel tag", 0xE007F, false}, // Cancel is separate
		{"before range", 0xE001F, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTagCharacter(tt.rune)
			if got != tt.want {
				t.Errorf("IsTagCharacter(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestSegment(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantRuns int
		check    func([]Run) bool
	}{
		{
			name:     "empty string",
			text:     "",
			wantRuns: 0,
		},
		{
			name:     "plain text",
			text:     "Hello",
			wantRuns: 1,
			check: func(runs []Run) bool {
				return !runs[0].IsEmoji && runs[0].Text == "Hello"
			},
		},
		{
			name:     "single emoji",
			text:     "\U0001F600",
			wantRuns: 1,
			check: func(runs []Run) bool {
				return runs[0].IsEmoji && len(runs[0].Codepoints) == 1
			},
		},
		{
			name:     "text then emoji",
			text:     "Hi \U0001F600",
			wantRuns: 2,
			check: func(runs []Run) bool {
				return !runs[0].IsEmoji && runs[1].IsEmoji
			},
		},
		{
			name:     "flag sequence",
			text:     "\U0001F1FA\U0001F1F8", // US flag
			wantRuns: 1,
			check: func(runs []Run) bool {
				return runs[0].IsEmoji && len(runs[0].Codepoints) == 2
			},
		},
		{
			name:     "multiple emoji",
			text:     "\U0001F600\U0001F601\U0001F602",
			wantRuns: 1, // All merged into one emoji run
			check: func(runs []Run) bool {
				return runs[0].IsEmoji && len(runs[0].Codepoints) == 3
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs := Segment(tt.text)
			if len(runs) != tt.wantRuns {
				t.Errorf("Segment(%q) = %d runs, want %d", tt.text, len(runs), tt.wantRuns)
				for i, r := range runs {
					t.Logf("  run[%d]: text=%q emoji=%v codepoints=%d", i, r.Text, r.IsEmoji, len(r.Codepoints))
				}
				return
			}
			if tt.check != nil && len(runs) > 0 && !tt.check(runs) {
				t.Errorf("Segment(%q) check failed", tt.text)
				for i, r := range runs {
					t.Logf("  run[%d]: text=%q emoji=%v codepoints=%d", i, r.Text, r.IsEmoji, len(r.Codepoints))
				}
			}
		})
	}
}

func TestSegmentMixed(t *testing.T) {
	text := "Hello \U0001F600 World \U0001F1FA\U0001F1F8!"

	runs := Segment(text)

	// Should have: "Hello " (text), emoji, " World " (text), flag, "!" (text)
	if len(runs) < 3 {
		t.Errorf("Expected at least 3 runs, got %d", len(runs))
		for i, r := range runs {
			t.Logf("  run[%d]: text=%q emoji=%v", i, r.Text, r.IsEmoji)
		}
	}

	// First run should be text
	if len(runs) > 0 && runs[0].IsEmoji {
		t.Error("First run should be text, got emoji")
	}
}

func TestIsEmojiPresentation(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"grinning face", 0x1F600, true},
		{"rocket", 0x1F680, true},
		{"playing card", 0x1F0A1, true},
		{"mahjong tile", 0x1F004, true},
		{"letter A", 'A', false},
		{"space", ' ', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmojiPresentation(tt.rune)
			if got != tt.want {
				t.Errorf("IsEmojiPresentation(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsTextPresentation(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"text VS", 0xFE0E, true},
		{"emoji VS", 0xFE0F, false},
		{"letter A", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTextPresentation(tt.rune)
			if got != tt.want {
				t.Errorf("IsTextPresentation(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsEmojiVariation(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"emoji VS", 0xFE0F, true},
		{"text VS", 0xFE0E, false},
		{"letter A", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmojiVariation(tt.rune)
			if got != tt.want {
				t.Errorf("IsEmojiVariation(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsCombiningEnclosingKeycap(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"keycap", 0x20E3, true},
		{"not keycap", 0x20E4, false},
		{"letter", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCombiningEnclosingKeycap(tt.rune)
			if got != tt.want {
				t.Errorf("IsCombiningEnclosingKeycap(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsCancelTag(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"cancel tag", 0xE007F, true},
		{"tag char", 0xE007E, false},
		{"letter", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCancelTag(tt.rune)
			if got != tt.want {
				t.Errorf("IsCancelTag(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsBlackFlag(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"black flag", 0x1F3F4, true},
		{"not flag", 0x1F3F5, false},
		{"letter", 'A', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBlackFlag(tt.rune)
			if got != tt.want {
				t.Errorf("IsBlackFlag(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestIsEmojiModifierBaseExtended(t *testing.T) {
	tests := []struct {
		name string
		rune rune
		want bool
	}{
		{"boy", 0x1F466, true},
		{"girl", 0x1F467, true},
		{"police", 0x1F46E, true},
		{"guard", 0x1F473, true},
		{"baby angel", 0x1F47C, true},
		{"info desk", 0x1F481, true},
		{"flexed biceps", 0x1F4AA, true},
		{"man in suit", 0x1F574, true},
		{"man dancing", 0x1F57A, true},
		{"hand splayed", 0x1F590, true},
		{"middle finger", 0x1F595, true},
		{"vulcan", 0x1F596, true},
		{"person gesture", 0x1F645, true},
		{"rowing", 0x1F6A3, true},
		{"cycling", 0x1F6B4, true},
		{"walking", 0x1F6B6, true},
		{"bath", 0x1F6C0, true},
		{"hand sign", 0x1F918, true},
		{"pregnancy", 0x1F930, true},
		{"wrestling", 0x1F93C, true},
		{"index pointing up", 0x261D, true},
		{"bouncing ball", 0x26F9, true},
		{"fist", 0x270A, true},
		{"writing hand", 0x270D, true},
		{"not modifier base", 0x1F600, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmojiModifierBase(tt.rune)
			if got != tt.want {
				t.Errorf("IsEmojiModifierBase(%U) = %v, want %v", tt.rune, got, tt.want)
			}
		})
	}
}

func TestSegmentKeycap(t *testing.T) {
	// Keycap sequence: 1 + FE0F + 20E3
	text := string([]rune{'1', 0xFE0F, 0x20E3})
	runs := Segment(text)

	if len(runs) != 1 {
		t.Fatalf("Segment(keycap) = %d runs, want 1", len(runs))
	}
	if !runs[0].IsEmoji {
		t.Error("keycap run should be emoji")
	}
}

func TestSegmentTagSequence(t *testing.T) {
	// Tag sequence: Black flag + tags + cancel
	runes := []rune{0x1F3F4, 0xE0067, 0xE0062, 0xE0073, 0xE0063, 0xE0074, 0xE007F}
	text := string(runes)
	runs := Segment(text)

	if len(runs) != 1 {
		t.Fatalf("Segment(tag) = %d runs, want 1", len(runs))
	}
	if !runs[0].IsEmoji {
		t.Error("tag sequence should be emoji")
	}
}

func TestSegmentZWJSequence(t *testing.T) {
	// Family: man + ZWJ + woman + ZWJ + girl
	runes := []rune{0x1F468, 0x200D, 0x1F469, 0x200D, 0x1F467}
	text := string(runes)
	runs := Segment(text)

	if len(runs) != 1 {
		t.Fatalf("Segment(ZWJ) = %d runs, want 1", len(runs))
	}
	if !runs[0].IsEmoji {
		t.Error("ZWJ sequence should be emoji")
	}
}

func TestSegmentSkinToneModifier(t *testing.T) {
	// Man + light skin tone
	runes := []rune{0x1F468, 0x1F3FB}
	text := string(runes)
	runs := Segment(text)

	if len(runs) != 1 {
		t.Fatalf("Segment(skin tone) = %d runs, want 1", len(runs))
	}
	if !runs[0].IsEmoji {
		t.Error("skin tone modified emoji should be emoji")
	}
}

func TestSegmentEmojiVariationSelector(t *testing.T) {
	// Heart + emoji VS
	runes := []rune{0x2764, 0xFE0F}
	text := string(runes)
	runs := Segment(text)

	if len(runs) != 1 {
		t.Fatalf("Segment(emoji VS) = %d runs, want 1", len(runs))
	}
	if !runs[0].IsEmoji {
		t.Error("emoji with VS should be emoji")
	}
}

func TestSegmentTextVariationSelector(t *testing.T) {
	// Heart + text VS — should NOT be emoji
	runes := []rune{0x2764, 0xFE0E}
	text := string(runes)
	runs := Segment(text)

	if len(runs) < 1 {
		t.Fatal("expected at least 1 run")
	}
	// With text VS, should NOT be treated as emoji
	if runs[0].IsEmoji {
		t.Error("emoji with text VS should not be emoji")
	}
}

func TestSegmentBlackFlagNoCancel(t *testing.T) {
	// Black flag followed by tag chars but no cancel tag — incomplete sequence
	runes := []rune{0x1F3F4, 0xE0067, 0xE0062}
	text := string(runes)
	runs := Segment(text)

	// Should still parse (black flag is emoji presentation)
	if len(runs) == 0 {
		t.Fatal("expected at least 1 run")
	}
}

func TestSegmentKeycapWithoutEnclosing(t *testing.T) {
	// Digit followed by emoji VS but no enclosing keycap — not a keycap
	runes := []rune{'1', 0xFE0F}
	text := string(runes)
	runs := Segment(text)

	// Should produce at least one run
	if len(runs) == 0 {
		t.Fatal("expected at least 1 run")
	}
}

func TestIsTextPresentationEmojiBranches(t *testing.T) {
	// Exercise many branches of isTextPresentationEmoji
	textEmoji := []rune{
		// Dingbats
		0x2702, 0x2708, 0x270D, 0x270F, 0x2712, 0x2714, 0x2716, 0x271D, 0x2721, 0x2728,
		0x2733, 0x2734, 0x2744, 0x2747, 0x274C, 0x274E, 0x2753, 0x2755, 0x2757,
		0x2763, 0x2764, 0x2795, 0x2797, 0x27A1, 0x27B0, 0x27BF,
		// Misc symbols
		0x2600, 0x2615, 0x2620, 0x2622, 0x2623, 0x2626, 0x262A, 0x262E, 0x262F,
		0x2638, 0x263A, 0x2640, 0x2642, 0x2648, 0x2653, 0x265F, 0x2660, 0x2663,
		0x2665, 0x2666, 0x2668, 0x267B, 0x267E, 0x267F, 0x2692, 0x2697, 0x2699,
		0x269B, 0x269C, 0x26A0, 0x26A1, 0x26AA, 0x26AB, 0x26B0, 0x26B1, 0x26BD,
		0x26BE, 0x26C4, 0x26C5, 0x26C8, 0x26CE, 0x26CF, 0x26D1, 0x26D3, 0x26D4,
		0x26E9, 0x26EA, 0x26F0, 0x26F5, 0x26F7, 0x26FA, 0x26FD,
		// Arrows
		0x2194, 0x2195, 0x2196, 0x2199, 0x21A9, 0x21AA,
		// Punctuation
		0x203C, 0x2049,
		// Info symbol
		0x2139,
		// Circled letter
		0x24C2,
		// Misc technical
		0x23E9, 0x23F3, 0x23F8, 0x23F9, 0x23FA,
		// Math/symbols
		0x2611, 0x2614, 0x2618, 0x261D,
		// Numbers symbols
		0x2934, 0x2935, 0x2B05, 0x2B07, 0x2B1B, 0x2B1C, 0x2B50, 0x2B55,
		// CJK
		0x3030, 0x303D, 0x3297, 0x3299,
		// Copyright
		0x00A9, 0x00AE, 0x2122,
		// Numbers misc
		0x2705,
	}

	for _, r := range textEmoji {
		if !IsEmoji(r) {
			t.Errorf("IsEmoji(%U) = false, want true (text presentation emoji)", r)
		}
	}
}

func TestIsEmojiPresentationBranches(t *testing.T) {
	// Exercise various branches of isEmojiPresentation
	presentationEmoji := []rune{
		0x1F600, // Emoticons
		0x1F300, // Misc symbols pictographs start
		0x1F5FF, // Misc symbols pictographs end
		0x1F680, // Transport start
		0x1F6FF, // Transport end
		0x1F900, // Supplemental start
		0x1F9FF, // Supplemental end
		0x1FA00, // Extended-A start
		0x1FA6F, // Extended-A end
		0x1FA70, // Extended-B start
		0x1FAFF, // Extended-B end
		0x1F3FB, // Skin tone
		0x1F1E6, // Regional indicator
		0x1F000, // Mahjong start
		0x1F0A0, // Playing cards start
	}

	for _, r := range presentationEmoji {
		if !IsEmojiPresentation(r) {
			t.Errorf("IsEmojiPresentation(%U) = false, want true", r)
		}
	}
}

func TestIsEmojiComponentBranches(t *testing.T) {
	// Exercise branches of isEmojiComponent
	components := []rune{
		0x1F3FB, // Skin tone
		0x1F1E6, // Regional indicator
		0xE0020, // Tag space
		0xE007F, // Cancel tag
		0x200D,  // ZWJ
		0xFE0E,  // Text VS
		0xFE0F,  // Emoji VS
		0x20E3,  // Keycap
	}

	for _, r := range components {
		if !IsEmoji(r) {
			t.Errorf("IsEmoji(%U) = false, want true (component)", r)
		}
	}
}

func TestSegmentEmojiAfterZWJ(t *testing.T) {
	// ZWJ + skin tone modifier after base
	runes := []rune{0x1F468, 0x1F3FD, 0x200D, 0x1F4BB} // Man medium skin + computer
	text := string(runes)
	runs := Segment(text)

	if len(runs) != 1 {
		t.Errorf("Segment(ZWJ+modifier) = %d runs, want 1", len(runs))
	}
}

func BenchmarkIsEmoji(b *testing.B) {
	runes := []rune{'A', 0x1F600, 0x1F1FA, 0x200D, ' ', 0x2764}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, r := range runes {
			_ = IsEmoji(r)
		}
	}
}

func BenchmarkSegment(b *testing.B) {
	text := "Hello \U0001F600 World \U0001F1FA\U0001F1F8 test \U0001F44D\U0001F3FB!"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Segment(text)
	}
}
