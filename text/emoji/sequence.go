package emoji

// unknownStrSeq is the string returned for unknown sequence enum values.
const unknownStrSeq = "Unknown"

// SequenceType indicates the type of emoji sequence.
type SequenceType int

const (
	// SequenceSimple is a single emoji character.
	SequenceSimple SequenceType = iota

	// SequenceZWJ is a Zero-Width Joiner sequence (family, profession, etc.).
	// Multiple emoji joined by U+200D.
	SequenceZWJ

	// SequenceFlag is a country flag formed by two regional indicators.
	// E.g., U+1F1FA U+1F1F8 = US flag.
	SequenceFlag

	// SequenceKeycap is a keycap sequence.
	// E.g., # + U+FE0F + U+20E3 = keycap number sign.
	SequenceKeycap

	// SequenceModified is a base emoji with skin tone modifier.
	// E.g., U+1F44B + U+1F3FB = waving hand, light skin.
	SequenceModified

	// SequenceTag is a subdivision flag sequence.
	// E.g., U+1F3F4 + tags + U+E007F = Scotland flag.
	SequenceTag

	// SequencePresentation is a text character with emoji variation selector.
	// E.g., U+2764 + U+FE0F = red heart emoji.
	SequencePresentation
)

// sequenceTypeNames maps SequenceType to string names.
var sequenceTypeNames = [...]string{
	SequenceSimple:       "Simple",
	SequenceZWJ:          "ZWJ",
	SequenceFlag:         "Flag",
	SequenceKeycap:       "Keycap",
	SequenceModified:     "Modified",
	SequenceTag:          "Tag",
	SequencePresentation: "Presentation",
}

// String returns the string name of the sequence type.
func (t SequenceType) String() string {
	if int(t) < len(sequenceTypeNames) {
		return sequenceTypeNames[t]
	}
	return unknownStrSeq
}

// Sequence represents an emoji sequence (single or multi-codepoint).
type Sequence struct {
	// Codepoints contains all runes forming this emoji.
	Codepoints []rune

	// Type indicates the sequence type.
	Type SequenceType

	// BaseCodepoint is the primary emoji character (for Modified sequences).
	// For other types, this equals Codepoints[0].
	BaseCodepoint rune

	// Modifier is the skin tone modifier, if present.
	// Zero if no modifier is applied.
	Modifier rune
}

// String returns a string representation of the sequence.
func (s Sequence) String() string {
	return string(s.Codepoints)
}

// Len returns the number of codepoints in the sequence.
func (s Sequence) Len() int {
	return len(s.Codepoints)
}

// HasModifier returns true if the sequence has a skin tone modifier.
func (s Sequence) HasModifier() bool {
	return s.Modifier != 0
}

// Parse parses emoji sequences from a slice of runes.
// Returns a slice of Sequence values, where each represents
// a complete emoji (possibly multi-codepoint).
func Parse(runes []rune) []Sequence {
	if len(runes) == 0 {
		return nil
	}

	sequences := make([]Sequence, 0, len(runes))
	i := 0

	for i < len(runes) {
		seq, consumed := parseSequenceAt(runes[i:])
		if consumed > 0 {
			sequences = append(sequences, seq)
			i += consumed
		} else {
			// Skip non-emoji character
			i++
		}
	}

	return sequences
}

// ParseString is a convenience function that parses emoji sequences from a string.
func ParseString(text string) []Sequence {
	return Parse([]rune(text))
}

// parseSequenceAt attempts to parse a complete emoji sequence at the start of runes.
// Returns the Sequence and number of runes consumed.
func parseSequenceAt(runes []rune) (Sequence, int) {
	if len(runes) == 0 {
		return Sequence{}, 0
	}

	r := runes[0]

	// Flag sequence (two regional indicators)
	if IsRegionalIndicator(r) && len(runes) >= 2 && IsRegionalIndicator(runes[1]) {
		return Sequence{
			Codepoints:    runes[:2],
			Type:          SequenceFlag,
			BaseCodepoint: r,
		}, 2
	}

	// Tag sequence (subdivision flags)
	if IsBlackFlag(r) {
		if seq, n := parseTagSequenceAt(runes); n > 0 {
			return seq, n
		}
	}

	// Keycap sequence
	if IsKeycapBase(r) {
		if seq, n := parseKeycapSequenceAt(runes); n > 0 {
			return seq, n
		}
	}

	// Check for basic emoji
	if !isEmojiBase(r) {
		return Sequence{}, 0
	}

	// Parse extended sequence
	return parseExtendedSequenceAt(runes)
}

// parseTagSequenceAt parses a subdivision flag tag sequence.
func parseTagSequenceAt(runes []rune) (Sequence, int) {
	if len(runes) < 3 || !IsBlackFlag(runes[0]) {
		return Sequence{}, 0
	}

	i := 1
	for i < len(runes) && IsTagCharacter(runes[i]) {
		i++
	}

	// Must have at least one tag character and end with cancel
	if i > 1 && i < len(runes) && IsCancelTag(runes[i]) {
		i++
		return Sequence{
			Codepoints:    runes[:i],
			Type:          SequenceTag,
			BaseCodepoint: runes[0],
		}, i
	}

	// Just a black flag
	if isEmojiPresentation(runes[0]) {
		return Sequence{
			Codepoints:    runes[:1],
			Type:          SequenceSimple,
			BaseCodepoint: runes[0],
		}, 1
	}

	return Sequence{}, 0
}

// parseKeycapSequenceAt parses a keycap emoji sequence.
func parseKeycapSequenceAt(runes []rune) (Sequence, int) {
	if len(runes) < 2 || !IsKeycapBase(runes[0]) {
		return Sequence{}, 0
	}

	i := 1
	// Optional variation selector
	if i < len(runes) && IsEmojiVariation(runes[i]) {
		i++
	}

	// Must have combining enclosing keycap
	if i < len(runes) && IsCombiningEnclosingKeycap(runes[i]) {
		i++
		return Sequence{
			Codepoints:    runes[:i],
			Type:          SequenceKeycap,
			BaseCodepoint: runes[0],
		}, i
	}

	return Sequence{}, 0
}

// parseExtendedSequenceAt parses a potentially extended emoji sequence.
func parseExtendedSequenceAt(runes []rune) (Sequence, int) {
	if len(runes) == 0 || !isEmojiBase(runes[0]) {
		return Sequence{}, 0
	}

	base := runes[0]
	i := 1
	seqType := SequenceSimple
	var modifier rune

	// Optional variation selector
	if i < len(runes) && IsVariationSelector(runes[i]) {
		if IsTextPresentation(runes[i]) {
			return Sequence{}, 0
		}
		seqType = SequencePresentation
		i++
	}

	// Optional skin tone modifier
	if i < len(runes) && IsEmojiModifier(runes[i]) {
		if IsEmojiModifierBase(base) {
			modifier = runes[i]
			seqType = SequenceModified
			i++
		}
	}

	// Check for ZWJ sequences
	zwjCount := 0
	for i+1 < len(runes) && IsZWJ(runes[i]) {
		// Parse emoji after ZWJ
		afterLen := parseAfterZWJ(runes[i+1:])
		if afterLen > 0 {
			i += 1 + afterLen
			zwjCount++
		} else {
			break
		}
	}

	if zwjCount > 0 {
		seqType = SequenceZWJ
	}

	return Sequence{
		Codepoints:    runes[:i],
		Type:          seqType,
		BaseCodepoint: base,
		Modifier:      modifier,
	}, i
}

// parseAfterZWJ parses an emoji that can follow ZWJ.
func parseAfterZWJ(runes []rune) int {
	if len(runes) == 0 {
		return 0
	}

	r := runes[0]
	if !isEmojiOrEmojiComponent(r) {
		return 0
	}

	i := 1

	// Optional variation selector
	if i < len(runes) && IsVariationSelector(runes[i]) {
		if IsTextPresentation(runes[i]) {
			return 0
		}
		i++
	}

	// Optional skin tone modifier
	if i < len(runes) && IsEmojiModifier(runes[i]) {
		if IsEmojiModifierBase(r) {
			i++
		}
	}

	return i
}

// Normalize normalizes an emoji sequence by:
//   - Removing text variation selectors (U+FE0E)
//   - Ensuring emoji variation selector (U+FE0F) where needed
//   - Validating skin tone modifiers are only on valid bases
func Normalize(seq Sequence) Sequence {
	if len(seq.Codepoints) == 0 {
		return seq
	}

	normalized := make([]rune, 0, len(seq.Codepoints))
	prevBase := rune(0)

	for _, r := range seq.Codepoints {
		// Skip text variation selector
		if IsTextPresentation(r) {
			continue
		}

		// Skip emoji variation selector (we'll add it back if needed)
		if IsEmojiVariation(r) {
			// Add FE0F for text-presentation emoji that need it
			if isTextPresentationEmoji(prevBase) {
				normalized = append(normalized, r)
			}
			continue
		}

		// Skip invalid modifiers
		if IsEmojiModifier(r) {
			if !IsEmojiModifierBase(prevBase) {
				continue
			}
		}

		normalized = append(normalized, r)
		if !IsZWJ(r) && !IsEmojiModifier(r) && !IsVariationSelector(r) {
			prevBase = r
		}
	}

	return Sequence{
		Codepoints:    normalized,
		Type:          seq.Type,
		BaseCodepoint: seq.BaseCodepoint,
		Modifier:      seq.Modifier,
	}
}

// IsValidSequence returns true if the sequence is well-formed according
// to Unicode emoji specification.
func IsValidSequence(seq Sequence) bool {
	if len(seq.Codepoints) == 0 {
		return false
	}

	switch seq.Type {
	case SequenceSimple:
		return len(seq.Codepoints) == 1 && IsEmoji(seq.Codepoints[0])

	case SequenceFlag:
		return len(seq.Codepoints) == 2 &&
			IsRegionalIndicator(seq.Codepoints[0]) &&
			IsRegionalIndicator(seq.Codepoints[1])

	case SequenceKeycap:
		if len(seq.Codepoints) < 2 || len(seq.Codepoints) > 3 {
			return false
		}
		if !IsKeycapBase(seq.Codepoints[0]) {
			return false
		}
		last := seq.Codepoints[len(seq.Codepoints)-1]
		return IsCombiningEnclosingKeycap(last)

	case SequenceTag:
		if len(seq.Codepoints) < 3 {
			return false
		}
		if !IsBlackFlag(seq.Codepoints[0]) {
			return false
		}
		if !IsCancelTag(seq.Codepoints[len(seq.Codepoints)-1]) {
			return false
		}
		// All middle characters must be tags
		for i := 1; i < len(seq.Codepoints)-1; i++ {
			if !IsTagCharacter(seq.Codepoints[i]) {
				return false
			}
		}
		return true

	case SequenceModified:
		if len(seq.Codepoints) < 2 {
			return false
		}
		if !IsEmojiModifierBase(seq.Codepoints[0]) {
			return false
		}
		return seq.HasModifier()

	case SequenceZWJ:
		// Must contain at least one ZWJ
		hasZWJ := false
		for _, r := range seq.Codepoints {
			if IsZWJ(r) {
				hasZWJ = true
				break
			}
		}
		return hasZWJ

	case SequencePresentation:
		// Must have variation selector
		hasVS := false
		for _, r := range seq.Codepoints {
			if IsVariationSelector(r) {
				hasVS = true
				break
			}
		}
		return hasVS

	default:
		return false
	}
}

// GetFlagCode extracts the two-letter country code from a flag sequence.
// Returns empty string if not a valid flag sequence.
func GetFlagCode(seq Sequence) string {
	if seq.Type != SequenceFlag || len(seq.Codepoints) != 2 {
		return ""
	}

	a := seq.Codepoints[0]
	b := seq.Codepoints[1]

	if !IsRegionalIndicator(a) || !IsRegionalIndicator(b) {
		return ""
	}

	// Regional indicators A-Z map to U+1F1E6-U+1F1FF
	// Convert to ASCII letters
	letterA := 'A' + (a - 0x1F1E6)
	letterB := 'A' + (b - 0x1F1E6)

	return string([]rune{letterA, letterB})
}

// GetTagSequenceCode extracts the subdivision code from a tag sequence.
// Returns empty string if not a valid tag sequence.
func GetTagSequenceCode(seq Sequence) string {
	if seq.Type != SequenceTag || len(seq.Codepoints) < 3 {
		return ""
	}

	// Skip black flag and cancel tag, extract middle tag characters
	code := make([]rune, 0, len(seq.Codepoints)-2)
	for i := 1; i < len(seq.Codepoints)-1; i++ {
		r := seq.Codepoints[i]
		if !IsTagCharacter(r) {
			return ""
		}
		// Convert tag character to ASCII (E0020-E007E -> 0020-007E)
		code = append(code, r-0xE0000)
	}

	return string(code)
}

// SkinTone represents a Fitzpatrick skin tone modifier.
type SkinTone int

const (
	// SkinToneNone indicates no skin tone modifier.
	SkinToneNone SkinTone = iota
	// SkinToneLight is the light skin tone (Type I-II).
	SkinToneLight
	// SkinToneMediumLight is the medium-light skin tone (Type III).
	SkinToneMediumLight
	// SkinToneMedium is the medium skin tone (Type IV).
	SkinToneMedium
	// SkinToneMediumDark is the medium-dark skin tone (Type V).
	SkinToneMediumDark
	// SkinToneDark is the dark skin tone (Type VI).
	SkinToneDark
)

// skinToneNames maps SkinTone to string names.
var skinToneNames = [...]string{
	SkinToneNone:        "None",
	SkinToneLight:       "Light",
	SkinToneMediumLight: "MediumLight",
	SkinToneMedium:      "Medium",
	SkinToneMediumDark:  "MediumDark",
	SkinToneDark:        "Dark",
}

// String returns the string name of the skin tone.
func (t SkinTone) String() string {
	if int(t) < len(skinToneNames) {
		return skinToneNames[t]
	}
	return unknownStrSeq
}

// GetSkinTone returns the skin tone from a modifier rune.
// Returns SkinToneNone if the rune is not a skin tone modifier.
func GetSkinTone(r rune) SkinTone {
	switch r {
	case 0x1F3FB:
		return SkinToneLight
	case 0x1F3FC:
		return SkinToneMediumLight
	case 0x1F3FD:
		return SkinToneMedium
	case 0x1F3FE:
		return SkinToneMediumDark
	case 0x1F3FF:
		return SkinToneDark
	default:
		return SkinToneNone
	}
}

// SkinToneRune returns the rune for a skin tone.
// Returns 0 for SkinToneNone.
func SkinToneRune(tone SkinTone) rune {
	switch tone {
	case SkinToneLight:
		return 0x1F3FB
	case SkinToneMediumLight:
		return 0x1F3FC
	case SkinToneMedium:
		return 0x1F3FD
	case SkinToneMediumDark:
		return 0x1F3FE
	case SkinToneDark:
		return 0x1F3FF
	default:
		return 0
	}
}
