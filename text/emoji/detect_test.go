package emoji

import (
	"testing"
)

func TestIsEmoji(t *testing.T) {
	tests := []struct {
		name  string
		rune  rune
		want  bool
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
