package emoji

// IsEmoji returns true if the rune is an emoji character.
// This includes characters with Emoji_Presentation=Yes or that are
// commonly used as emoji with variation selector.
func IsEmoji(r rune) bool {
	return isEmojiPresentation(r) || isEmojiComponent(r) || isTextPresentationEmoji(r)
}

// IsEmojiPresentation returns true if the rune defaults to emoji presentation.
// These characters display as emoji without requiring U+FE0F.
func IsEmojiPresentation(r rune) bool {
	return isEmojiPresentation(r)
}

// IsEmojiModifier returns true if the rune is a skin tone modifier.
// Fitzpatrick scale modifiers: U+1F3FB - U+1F3FF.
func IsEmojiModifier(r rune) bool {
	return r >= 0x1F3FB && r <= 0x1F3FF
}

// IsEmojiModifierBase returns true if the rune can be modified by a skin tone.
// This includes humans, body parts, and human activities.
func IsEmojiModifierBase(r rune) bool {
	// Common emoji modifier bases
	switch {
	// People and body parts
	case r >= 0x1F466 && r <= 0x1F469: // Boy, Girl, Man, Woman
		return true
	case r >= 0x1F46E && r <= 0x1F478: // Police, Guard, etc.
		return true
	case r >= 0x1F47C && r <= 0x1F47C: // Baby Angel
		return true
	case r >= 0x1F481 && r <= 0x1F487: // Information Desk, etc.
		return true
	case r >= 0x1F4AA && r <= 0x1F4AA: // Flexed Biceps
		return true
	case r >= 0x1F574 && r <= 0x1F575: // Man in Suit, Detective
		return true
	case r >= 0x1F57A && r <= 0x1F57A: // Man Dancing
		return true
	case r >= 0x1F590 && r <= 0x1F590: // Hand with Fingers Splayed
		return true
	case r >= 0x1F595 && r <= 0x1F596: // Middle Finger, Vulcan
		return true
	case r >= 0x1F645 && r <= 0x1F64F: // Gestures and Person
		return true
	case r >= 0x1F6A3 && r <= 0x1F6A3: // Rowing
		return true
	case r >= 0x1F6B4 && r <= 0x1F6B6: // Cycling, Walking
		return true
	case r >= 0x1F6C0 && r <= 0x1F6C0: // Bath
		return true
	case r >= 0x1F918 && r <= 0x1F91F: // Hand signs
		return true
	case r >= 0x1F926 && r <= 0x1F926: // Face Palm
		return true
	case r >= 0x1F930 && r <= 0x1F939: // Pregnancy, etc.
		return true
	case r >= 0x1F93C && r <= 0x1F93E: // Wrestling, etc.
		return true
	// Hands from dingbats range
	case r >= 0x261D && r <= 0x261D: // Index Pointing Up
		return true
	case r >= 0x26F9 && r <= 0x26F9: // Person Bouncing Ball
		return true
	case r >= 0x270A && r <= 0x270D: // Fists, Writing Hand
		return true
	}
	return false
}

// IsZWJ returns true if the rune is Zero-Width Joiner (U+200D).
// ZWJ is used to join emoji into composite sequences.
func IsZWJ(r rune) bool {
	return r == 0x200D
}

// IsRegionalIndicator returns true if the rune is a Regional Indicator (A-Z).
// Two regional indicators form a flag emoji (e.g., U+1F1FA U+1F1F8 = US flag).
func IsRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

// IsVariationSelector returns true for emoji-related variation selectors.
// U+FE0E forces text presentation, U+FE0F forces emoji presentation.
func IsVariationSelector(r rune) bool {
	return r == 0xFE0E || r == 0xFE0F
}

// IsTextPresentation returns true for the text variation selector (U+FE0E).
func IsTextPresentation(r rune) bool {
	return r == 0xFE0E
}

// IsEmojiVariation returns true for the emoji variation selector (U+FE0F).
func IsEmojiVariation(r rune) bool {
	return r == 0xFE0F
}

// IsKeycapBase returns true if the rune can form a keycap emoji.
// Digits 0-9, # and * can be followed by U+FE0F U+20E3 to form keycaps.
func IsKeycapBase(r rune) bool {
	return (r >= '0' && r <= '9') || r == '#' || r == '*'
}

// IsCombiningEnclosingKeycap returns true for the keycap combining mark.
func IsCombiningEnclosingKeycap(r rune) bool {
	return r == 0x20E3
}

// IsTagCharacter returns true for emoji tag characters.
// Tags U+E0020-U+E007E are used in subdivision flag sequences.
func IsTagCharacter(r rune) bool {
	return r >= 0xE0020 && r <= 0xE007E
}

// IsCancelTag returns true for the cancel tag character (U+E007F).
// This terminates subdivision flag sequences.
func IsCancelTag(r rune) bool {
	return r == 0xE007F
}

// IsBlackFlag returns true for the black flag emoji.
// This is the base for subdivision flag sequences.
func IsBlackFlag(r rune) bool {
	return r == 0x1F3F4
}

// Run represents a contiguous run of text with uniform emoji status.
type Run struct {
	// Text is the substring of the run.
	Text string

	// IsEmoji is true if this run contains emoji characters.
	// For ZWJ sequences, this will be a single Run with multiple codepoints.
	IsEmoji bool

	// Codepoints contains the individual runes for emoji runs.
	// For non-emoji runs, this is nil.
	Codepoints []rune

	// Start is the byte offset in the original string.
	Start int

	// End is the byte offset of the end (exclusive).
	End int
}

// Segment splits text into emoji and non-emoji runs.
// ZWJ sequences and other multi-codepoint emoji are kept together.
// Flag sequences (regional indicators) are combined.
// Keycap sequences are combined.
// Tag sequences are combined.
func Segment(text string) []Run {
	if text == "" {
		return nil
	}

	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	runs := make([]Run, 0, 4)
	byteOffsets := computeByteOffsets(text, runes)

	i := 0
	for i < len(runes) {
		// Try to parse a complete emoji sequence
		emojiLen, codepoints := parseEmojiSequence(runes[i:])

		if emojiLen > 0 {
			// Found an emoji sequence
			run := Run{
				Text:       text[byteOffsets[i]:byteOffsets[i+emojiLen]],
				IsEmoji:    true,
				Codepoints: codepoints,
				Start:      byteOffsets[i],
				End:        byteOffsets[i+emojiLen],
			}
			runs = appendOrMergeRun(runs, run)
			i += emojiLen
		} else {
			// Regular text character
			run := Run{
				Text:    text[byteOffsets[i]:byteOffsets[i+1]],
				IsEmoji: false,
				Start:   byteOffsets[i],
				End:     byteOffsets[i+1],
			}
			runs = appendOrMergeRun(runs, run)
			i++
		}
	}

	return runs
}

// parseEmojiSequence attempts to parse a complete emoji sequence starting at runes[0].
// Returns the number of runes consumed and the codepoints, or (0, nil) if not an emoji.
func parseEmojiSequence(runes []rune) (int, []rune) {
	if len(runes) == 0 {
		return 0, nil
	}

	r := runes[0]

	// Check for flag sequence (two regional indicators)
	if IsRegionalIndicator(r) && len(runes) >= 2 && IsRegionalIndicator(runes[1]) {
		return 2, runes[:2]
	}

	// Check for tag sequence (black flag + tags + cancel)
	if IsBlackFlag(r) {
		return parseTagSequence(runes)
	}

	// Check for keycap sequence
	if IsKeycapBase(r) {
		return parseKeycapSequence(runes)
	}

	// Check for basic emoji
	if !isEmojiBase(r) {
		return 0, nil
	}

	// Parse possible extended sequence (modifiers, ZWJ, variation selectors)
	return parseExtendedSequence(runes)
}

// parseTagSequence parses a subdivision flag sequence.
// Format: BLACK_FLAG + TAG_CHARACTERS + CANCEL_TAG
func parseTagSequence(runes []rune) (int, []rune) {
	if len(runes) < 3 || !IsBlackFlag(runes[0]) {
		return 0, nil
	}

	i := 1
	// Consume tag characters
	for i < len(runes) && IsTagCharacter(runes[i]) {
		i++
	}

	// Must end with cancel tag
	if i < len(runes) && IsCancelTag(runes[i]) {
		i++
		return i, runes[:i]
	}

	// Invalid sequence, just return the flag
	if isEmojiPresentation(runes[0]) {
		return 1, runes[:1]
	}
	return 0, nil
}

// parseKeycapSequence parses a keycap emoji sequence.
// Format: KEYCAP_BASE + [FE0F] + 20E3
func parseKeycapSequence(runes []rune) (int, []rune) {
	if len(runes) < 2 || !IsKeycapBase(runes[0]) {
		return 0, nil
	}

	i := 1
	// Optional variation selector
	if i < len(runes) && IsEmojiVariation(runes[i]) {
		i++
	}

	// Must have combining enclosing keycap
	if i < len(runes) && IsCombiningEnclosingKeycap(runes[i]) {
		i++
		return i, runes[:i]
	}

	// Not a keycap sequence, check if base is emoji
	return 0, nil
}

// parseExtendedSequence parses an extended emoji sequence with modifiers, ZWJ, etc.
func parseExtendedSequence(runes []rune) (int, []rune) {
	if len(runes) == 0 || !isEmojiBase(runes[0]) {
		return 0, nil
	}

	i := 1

	// Optional variation selector
	if i < len(runes) && IsVariationSelector(runes[i]) {
		// If text presentation, not an emoji
		if IsTextPresentation(runes[i]) {
			return 0, nil
		}
		i++
	}

	// Optional skin tone modifier
	if i < len(runes) && IsEmojiModifier(runes[i]) {
		// Only apply if base supports modifiers
		if IsEmojiModifierBase(runes[0]) {
			i++
		}
	}

	// ZWJ sequences
	for i+1 < len(runes) && IsZWJ(runes[i]) {
		// Look ahead to see if there's a valid emoji after ZWJ
		nextLen, _ := parseEmojiSequenceAfterZWJ(runes[i+1:])
		if nextLen > 0 {
			i += 1 + nextLen // ZWJ + next emoji
		} else {
			break
		}
	}

	return i, runes[:i]
}

// parseEmojiSequenceAfterZWJ parses emoji that can appear after ZWJ.
func parseEmojiSequenceAfterZWJ(runes []rune) (int, []rune) {
	if len(runes) == 0 {
		return 0, nil
	}

	r := runes[0]

	// After ZWJ, many characters can be emoji
	if !isEmojiOrEmojiComponent(r) {
		return 0, nil
	}

	i := 1

	// Optional variation selector
	if i < len(runes) && IsVariationSelector(runes[i]) {
		if IsTextPresentation(runes[i]) {
			return 0, nil
		}
		i++
	}

	// Optional skin tone modifier
	if i < len(runes) && IsEmojiModifier(runes[i]) {
		if IsEmojiModifierBase(runes[0]) {
			i++
		}
	}

	return i, runes[:i]
}

// appendOrMergeRun appends a run, merging with the previous run if they have the same emoji status.
func appendOrMergeRun(runs []Run, run Run) []Run {
	if len(runs) == 0 {
		return append(runs, run)
	}

	last := &runs[len(runs)-1]
	if last.IsEmoji == run.IsEmoji {
		// Merge runs of the same type
		last.Text += run.Text
		last.End = run.End
		if run.Codepoints != nil {
			last.Codepoints = append(last.Codepoints, run.Codepoints...)
		}
		return runs
	}

	return append(runs, run)
}

// computeByteOffsets returns the byte offset for each rune index.
func computeByteOffsets(text string, runes []rune) []int {
	offsets := make([]int, len(runes)+1)
	offset := 0
	for i, r := range runes {
		offsets[i] = offset
		offset += len(string(r))
	}
	offsets[len(runes)] = len(text)
	return offsets
}

// isEmojiBase returns true if the rune can start an emoji sequence.
func isEmojiBase(r rune) bool {
	return isEmojiPresentation(r) || isTextPresentationEmoji(r)
}

// isEmojiOrEmojiComponent returns true if the rune is an emoji or component.
func isEmojiOrEmojiComponent(r rune) bool {
	return isEmojiPresentation(r) || isEmojiComponent(r) || isTextPresentationEmoji(r)
}

// isEmojiComponent returns true for emoji component characters.
func isEmojiComponent(r rune) bool {
	// Skin tone modifiers
	if r >= 0x1F3FB && r <= 0x1F3FF {
		return true
	}
	// Regional indicators
	if r >= 0x1F1E6 && r <= 0x1F1FF {
		return true
	}
	// Tag characters
	if r >= 0xE0020 && r <= 0xE007F {
		return true
	}
	// ZWJ
	if r == 0x200D {
		return true
	}
	// Variation selectors
	if r == 0xFE0E || r == 0xFE0F {
		return true
	}
	// Combining enclosing keycap
	if r == 0x20E3 {
		return true
	}
	return false
}

// isEmojiPresentation returns true for characters with Emoji_Presentation=Yes.
func isEmojiPresentation(r rune) bool {
	switch {
	// Emoticons
	case r >= 0x1F600 && r <= 0x1F64F:
		return true
	// Miscellaneous Symbols and Pictographs
	case r >= 0x1F300 && r <= 0x1F5FF:
		return true
	// Transport and Map Symbols
	case r >= 0x1F680 && r <= 0x1F6FF:
		return true
	// Supplemental Symbols and Pictographs
	case r >= 0x1F900 && r <= 0x1F9FF:
		return true
	// Symbols and Pictographs Extended-A
	case r >= 0x1FA00 && r <= 0x1FA6F:
		return true
	// Symbols and Pictographs Extended-B (Unicode 14+)
	case r >= 0x1FA70 && r <= 0x1FAFF:
		return true
	// Skin tone modifiers
	case r >= 0x1F3FB && r <= 0x1F3FF:
		return true
	// Regional Indicators (flags)
	case r >= 0x1F1E6 && r <= 0x1F1FF:
		return true
	// Mahjong tiles
	case r >= 0x1F000 && r <= 0x1F02F:
		return true
	// Playing cards
	case r >= 0x1F0A0 && r <= 0x1F0FF:
		return true
	default:
		return false
	}
}

// isTextPresentationEmoji returns true for characters that can be emoji
// with variation selector (Emoji=Yes but Emoji_Presentation=No).
func isTextPresentationEmoji(r rune) bool {
	switch {
	// Dingbats that can be emoji
	case r >= 0x2702 && r <= 0x27B0:
		return true
	// Miscellaneous Symbols
	case r >= 0x2600 && r <= 0x26FF:
		return true
	// Arrows (some are emoji)
	case r == 0x2194 || r == 0x2195 || (r >= 0x2196 && r <= 0x2199):
		return true
	case r == 0x21A9 || r == 0x21AA:
		return true
	// Punctuation with emoji variants
	case r == 0x203C || r == 0x2049:
		return true
	// Information symbols
	case r == 0x2139:
		return true
	// Circled letters
	case r == 0x24C2:
		return true
	// Misc technical
	case r >= 0x23E9 && r <= 0x23F3:
		return true
	case r == 0x23F8 || r == 0x23F9 || r == 0x23FA:
		return true
	// Math symbols with emoji use
	case r == 0x2611 || r == 0x2614 || r == 0x2615:
		return true
	case r == 0x2618 || r == 0x261D || r == 0x2620:
		return true
	case r == 0x2622 || r == 0x2623 || r == 0x2626:
		return true
	case r == 0x262A || r == 0x262E || r == 0x262F:
		return true
	case r >= 0x2638 && r <= 0x263A:
		return true
	case r == 0x2640 || r == 0x2642:
		return true
	case r >= 0x2648 && r <= 0x2653:
		return true
	case r == 0x265F || r == 0x2660 || r == 0x2663:
		return true
	case r == 0x2665 || r == 0x2666 || r == 0x2668:
		return true
	case r == 0x267B || r == 0x267E || r == 0x267F:
		return true
	case r >= 0x2692 && r <= 0x2697:
		return true
	case r == 0x2699 || r == 0x269B || r == 0x269C:
		return true
	case r >= 0x26A0 && r <= 0x26A1:
		return true
	case r == 0x26AA || r == 0x26AB:
		return true
	case r >= 0x26B0 && r <= 0x26B1:
		return true
	case r == 0x26BD || r == 0x26BE:
		return true
	case r >= 0x26C4 && r <= 0x26C5:
		return true
	case r == 0x26C8 || r == 0x26CE || r == 0x26CF:
		return true
	case r == 0x26D1 || r == 0x26D3 || r == 0x26D4:
		return true
	case r == 0x26E9 || r == 0x26EA:
		return true
	case r >= 0x26F0 && r <= 0x26F5:
		return true
	case r >= 0x26F7 && r <= 0x26FA:
		return true
	case r == 0x26FD:
		return true
	// Numbers and symbols
	case r == 0x2702 || r == 0x2705:
		return true
	case r >= 0x2708 && r <= 0x270D:
		return true
	case r == 0x270F:
		return true
	case r == 0x2712 || r == 0x2714 || r == 0x2716:
		return true
	case r == 0x271D || r == 0x2721:
		return true
	case r == 0x2728:
		return true
	case r >= 0x2733 && r <= 0x2734:
		return true
	case r == 0x2744 || r == 0x2747:
		return true
	case r == 0x274C || r == 0x274E:
		return true
	case r >= 0x2753 && r <= 0x2755:
		return true
	case r == 0x2757:
		return true
	case r >= 0x2763 && r <= 0x2764:
		return true
	case r >= 0x2795 && r <= 0x2797:
		return true
	case r == 0x27A1:
		return true
	case r == 0x27B0:
		return true
	case r == 0x27BF:
		return true
	case r >= 0x2934 && r <= 0x2935:
		return true
	case r >= 0x2B05 && r <= 0x2B07:
		return true
	case r == 0x2B1B || r == 0x2B1C:
		return true
	case r == 0x2B50 || r == 0x2B55:
		return true
	case r == 0x3030 || r == 0x303D:
		return true
	case r == 0x3297 || r == 0x3299:
		return true
	// Copyright, registered, trademark
	case r == 0x00A9 || r == 0x00AE:
		return true
	case r == 0x2122:
		return true
	default:
		return false
	}
}
