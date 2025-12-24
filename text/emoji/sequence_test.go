package emoji

import (
	"testing"
)

func TestSequenceType_String(t *testing.T) {
	tests := []struct {
		typ  SequenceType
		want string
	}{
		{SequenceSimple, "Simple"},
		{SequenceZWJ, "ZWJ"},
		{SequenceFlag, "Flag"},
		{SequenceKeycap, "Keycap"},
		{SequenceModified, "Modified"},
		{SequenceTag, "Tag"},
		{SequencePresentation, "Presentation"},
		{SequenceType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.typ.String()
			if got != tt.want {
				t.Errorf("SequenceType(%d).String() = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    []rune
		wantLen  int
		wantType SequenceType
	}{
		{
			name:     "empty",
			input:    nil,
			wantLen:  0,
			wantType: 0,
		},
		{
			name:     "single emoji",
			input:    []rune{0x1F600},
			wantLen:  1,
			wantType: SequenceSimple,
		},
		{
			name:     "flag US",
			input:    []rune{0x1F1FA, 0x1F1F8},
			wantLen:  1,
			wantType: SequenceFlag,
		},
		{
			name:     "keycap 1",
			input:    []rune{'1', 0xFE0F, 0x20E3},
			wantLen:  1,
			wantType: SequenceKeycap,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sequences := Parse(tt.input)
			if len(sequences) != tt.wantLen {
				t.Errorf("Parse(%v) returned %d sequences, want %d", tt.input, len(sequences), tt.wantLen)
				return
			}
			if tt.wantLen > 0 && sequences[0].Type != tt.wantType {
				t.Errorf("Parse(%v)[0].Type = %v, want %v", tt.input, sequences[0].Type, tt.wantType)
			}
		})
	}
}

func TestParseString(t *testing.T) {
	text := "\U0001F600" // Grinning face
	sequences := ParseString(text)

	if len(sequences) != 1 {
		t.Fatalf("ParseString(%q) returned %d sequences, want 1", text, len(sequences))
	}

	if sequences[0].Type != SequenceSimple {
		t.Errorf("sequence.Type = %v, want SequenceSimple", sequences[0].Type)
	}
}

func TestSequence_String(t *testing.T) {
	seq := Sequence{
		Codepoints:    []rune{0x1F600},
		Type:          SequenceSimple,
		BaseCodepoint: 0x1F600,
	}

	got := seq.String()
	want := "\U0001F600"
	if got != want {
		t.Errorf("Sequence.String() = %q, want %q", got, want)
	}
}

func TestSequence_Len(t *testing.T) {
	tests := []struct {
		name       string
		codepoints []rune
		wantLen    int
	}{
		{"empty", nil, 0},
		{"single", []rune{0x1F600}, 1},
		{"flag", []rune{0x1F1FA, 0x1F1F8}, 2},
		{"keycap", []rune{'1', 0xFE0F, 0x20E3}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := Sequence{Codepoints: tt.codepoints}
			if got := seq.Len(); got != tt.wantLen {
				t.Errorf("Sequence.Len() = %d, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestSequence_HasModifier(t *testing.T) {
	tests := []struct {
		name     string
		modifier rune
		want     bool
	}{
		{"no modifier", 0, false},
		{"with modifier", 0x1F3FB, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := Sequence{Modifier: tt.modifier}
			if got := seq.HasModifier(); got != tt.want {
				t.Errorf("Sequence.HasModifier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input Sequence
		check func(Sequence) bool
	}{
		{
			name:  "empty",
			input: Sequence{},
			check: func(s Sequence) bool { return len(s.Codepoints) == 0 },
		},
		{
			name: "simple passthrough",
			input: Sequence{
				Codepoints: []rune{0x1F600},
				Type:       SequenceSimple,
			},
			check: func(s Sequence) bool {
				return len(s.Codepoints) == 1 && s.Codepoints[0] == 0x1F600
			},
		},
		{
			name: "remove text VS",
			input: Sequence{
				Codepoints: []rune{0x2764, 0xFE0E}, // Heart + text VS
				Type:       SequencePresentation,
			},
			check: func(s Sequence) bool {
				// Text VS should be removed
				for _, r := range s.Codepoints {
					if r == 0xFE0E {
						return false
					}
				}
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if !tt.check(result) {
				t.Errorf("Normalize() check failed, got codepoints: %v", result.Codepoints)
			}
		})
	}
}

func TestIsValidSequence(t *testing.T) {
	tests := []struct {
		name  string
		seq   Sequence
		valid bool
	}{
		{
			name:  "empty",
			seq:   Sequence{},
			valid: false,
		},
		{
			name: "simple emoji",
			seq: Sequence{
				Codepoints: []rune{0x1F600},
				Type:       SequenceSimple,
			},
			valid: true,
		},
		{
			name: "valid flag",
			seq: Sequence{
				Codepoints: []rune{0x1F1FA, 0x1F1F8},
				Type:       SequenceFlag,
			},
			valid: true,
		},
		{
			name: "invalid flag - wrong length",
			seq: Sequence{
				Codepoints: []rune{0x1F1FA},
				Type:       SequenceFlag,
			},
			valid: false,
		},
		{
			name: "valid keycap",
			seq: Sequence{
				Codepoints: []rune{'1', 0xFE0F, 0x20E3},
				Type:       SequenceKeycap,
			},
			valid: true,
		},
		{
			name: "invalid keycap - no enclosing",
			seq: Sequence{
				Codepoints: []rune{'1', 0xFE0F},
				Type:       SequenceKeycap,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidSequence(tt.seq)
			if got != tt.valid {
				t.Errorf("IsValidSequence() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestGetFlagCode(t *testing.T) {
	tests := []struct {
		name string
		seq  Sequence
		want string
	}{
		{
			name: "US flag",
			seq: Sequence{
				Codepoints: []rune{0x1F1FA, 0x1F1F8},
				Type:       SequenceFlag,
			},
			want: "US",
		},
		{
			name: "JP flag",
			seq: Sequence{
				Codepoints: []rune{0x1F1EF, 0x1F1F5},
				Type:       SequenceFlag,
			},
			want: "JP",
		},
		{
			name: "not a flag",
			seq: Sequence{
				Codepoints: []rune{0x1F600},
				Type:       SequenceSimple,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFlagCode(tt.seq)
			if got != tt.want {
				t.Errorf("GetFlagCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetTagSequenceCode(t *testing.T) {
	tests := []struct {
		name string
		seq  Sequence
		want string
	}{
		{
			name: "Scotland flag",
			seq: Sequence{
				Codepoints: []rune{
					0x1F3F4, // Black flag
					0xE0067, // g
					0xE0062, // b
					0xE0073, // s
					0xE0063, // c
					0xE0074, // t
					0xE007F, // Cancel
				},
				Type: SequenceTag,
			},
			want: "gbsct",
		},
		{
			name: "not a tag",
			seq: Sequence{
				Codepoints: []rune{0x1F600},
				Type:       SequenceSimple,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTagSequenceCode(tt.seq)
			if got != tt.want {
				t.Errorf("GetTagSequenceCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSkinTone(t *testing.T) {
	tests := []struct {
		rune rune
		tone SkinTone
	}{
		{0x1F3FB, SkinToneLight},
		{0x1F3FC, SkinToneMediumLight},
		{0x1F3FD, SkinToneMedium},
		{0x1F3FE, SkinToneMediumDark},
		{0x1F3FF, SkinToneDark},
		{'A', SkinToneNone},
	}

	for _, tt := range tests {
		t.Run(tt.tone.String(), func(t *testing.T) {
			got := GetSkinTone(tt.rune)
			if got != tt.tone {
				t.Errorf("GetSkinTone(%U) = %v, want %v", tt.rune, got, tt.tone)
			}

			// Test reverse
			if tt.tone != SkinToneNone {
				reverse := SkinToneRune(tt.tone)
				if reverse != tt.rune {
					t.Errorf("SkinToneRune(%v) = %U, want %U", tt.tone, reverse, tt.rune)
				}
			}
		})
	}
}

func TestSkinTone_String(t *testing.T) {
	tests := []struct {
		tone SkinTone
		want string
	}{
		{SkinToneNone, "None"},
		{SkinToneLight, "Light"},
		{SkinToneMediumLight, "MediumLight"},
		{SkinToneMedium, "Medium"},
		{SkinToneMediumDark, "MediumDark"},
		{SkinToneDark, "Dark"},
		{SkinTone(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.tone.String()
			if got != tt.want {
				t.Errorf("SkinTone(%d).String() = %q, want %q", tt.tone, got, tt.want)
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
	runes := []rune{0x1F600, 0x1F601, 0x1F1FA, 0x1F1F8, 0x1F468, 0x200D, 0x1F469, 0x200D, 0x1F467}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Parse(runes)
	}
}
