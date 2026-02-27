package text

import (
	"strings"
	"unicode"
)

// WrapMode specifies how text is wrapped when it exceeds the maximum width.
type WrapMode uint8

const (
	// WrapWordChar breaks at word boundaries first,
	// then falls back to character boundaries for long words.
	// This is the default and most common mode (zero value for backward compatibility).
	WrapWordChar WrapMode = iota

	// WrapNone disables text wrapping; text may exceed MaxWidth.
	WrapNone

	// WrapWord breaks at word boundaries only.
	// Long words that exceed MaxWidth will overflow.
	WrapWord

	// WrapChar breaks at character boundaries.
	// Any character can be a break point.
	WrapChar
)

// String returns the string representation of the wrap mode.
func (m WrapMode) String() string {
	switch m {
	case WrapNone:
		return "None"
	case WrapWord:
		return "Word"
	case WrapChar:
		return "Char"
	case WrapWordChar:
		return "WordChar"
	default:
		return unknownStr
	}
}

// BreakClass represents Unicode line breaking classes (UAX #14 simplified).
type BreakClass uint8

const (
	// breakOther is the default class for most characters.
	breakOther BreakClass = iota
	// breakSpace is for space characters (break after).
	breakSpace
	// breakZero is for zero-width space (break opportunity).
	breakZero
	// breakOpen is for opening punctuation (no break after).
	breakOpen
	// breakClose is for closing punctuation (no break before).
	breakClose
	// breakHyphen is for hyphens (break after).
	breakHyphen
	// breakIdeographic is for CJK ideographs (break before/after).
	breakIdeographic
)

// classifyRune returns the break class of a rune.
// This is a simplified implementation of UAX #14.
func classifyRune(r rune) BreakClass {
	// Check specific characters first
	if class, ok := classifySpecificRune(r); ok {
		return class
	}
	// Check character categories
	if isCJKRune(r) {
		return breakIdeographic
	}
	return breakOther
}

// classifySpecificRune handles classification of specific characters.
func classifySpecificRune(r rune) (BreakClass, bool) {
	switch r {
	case ' ', '\t':
		return breakSpace, true
	case '\u200B': // Zero-width space
		return breakZero, true
	case '(', '[', '{', '\u201C', '\u2018':
		return breakOpen, true // Opening brackets and quotes
	case ')', ']', '}', '\u201D', '\u2019':
		return breakClose, true // Closing brackets and quotes
	case '-', '\u2010', '\u2011', '\u2013', '\u2014':
		return breakHyphen, true // Various hyphens and dashes
	default:
		return breakOther, false
	}
}

// isCJKRune returns true if the rune is a CJK character that allows breaking.
func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul Syllables
		(r >= 0xFF00 && r <= 0xFFEF) // Fullwidth forms
}

// BreakOpportunity represents a line break opportunity.
type BreakOpportunity uint8

const (
	// BreakNo means no break allowed here.
	BreakNo BreakOpportunity = iota
	// BreakAllowed means break is allowed here.
	BreakAllowed
	// BreakMandatory means break is required here (newline).
	BreakMandatory
)

// findBreakOpportunities analyzes text and returns break opportunities.
// Returns a slice where index i indicates break opportunity BEFORE character i.
// The first element (index 0) is always BreakNo (can't break before first char).
func findBreakOpportunities(text string, mode WrapMode) []BreakOpportunity {
	if text == "" {
		return nil
	}

	runes := []rune(text)
	n := len(runes)
	breaks := make([]BreakOpportunity, n)

	// First character cannot have a break before it
	breaks[0] = BreakNo

	if mode == WrapNone {
		// No breaks allowed
		return breaks
	}

	// Classify all runes
	classes := make([]BreakClass, n)
	for i, r := range runes {
		classes[i] = classifyRune(r)
	}

	// Apply break rules based on mode
	for i := 1; i < n; i++ {
		breaks[i] = computeBreak(runes, classes, i, mode)
	}

	return breaks
}

// computeBreak determines the break opportunity before position i.
func computeBreak(runes []rune, classes []BreakClass, i int, mode WrapMode) BreakOpportunity {
	if i <= 0 || i >= len(runes) {
		return BreakNo
	}

	prevRune := runes[i-1]
	currRune := runes[i]
	prevClass := classes[i-1]
	currClass := classes[i]

	// Mandatory breaks (newlines are handled at paragraph level)
	if prevRune == '\n' {
		return BreakMandatory
	}

	// No break before closing punctuation
	if currClass == breakClose {
		return BreakNo
	}

	// No break after opening punctuation
	if prevClass == breakOpen {
		return BreakNo
	}

	// Break after zero-width space
	if prevClass == breakZero {
		return BreakAllowed
	}

	// Mode-specific rules
	switch mode {
	case WrapChar:
		// Character mode: break anywhere except special cases
		return BreakAllowed

	case WrapWord:
		// Word mode: break only at word boundaries
		return computeWordBreak(prevRune, currRune, prevClass, currClass)

	case WrapWordChar:
		// WordChar mode: same as Word, but Char is allowed as fallback
		// The fallback is handled at the wrapping level, not here
		return computeWordBreak(prevRune, currRune, prevClass, currClass)

	default:
		return BreakNo
	}
}

// computeWordBreak determines break opportunity for word-based wrapping.
func computeWordBreak(prevRune, currRune rune, prevClass, currClass BreakClass) BreakOpportunity {
	// Break after space
	if prevClass == breakSpace {
		return BreakAllowed
	}

	// Break after hyphens (but not before)
	if prevClass == breakHyphen && currClass != breakHyphen {
		return BreakAllowed
	}

	// CJK: break before and after ideographs
	if currClass == breakIdeographic {
		return BreakAllowed
	}
	if prevClass == breakIdeographic && currClass != breakClose {
		return BreakAllowed
	}

	// Break at transitions between categories
	if isBreakBetweenCategories(prevRune, currRune) {
		return BreakAllowed
	}

	return BreakNo
}

// isBreakBetweenCategories checks for breaks between different character categories.
func isBreakBetweenCategories(prev, curr rune) bool {
	// Break before punctuation after letters/digits
	if (unicode.IsLetter(prev) || unicode.IsDigit(prev)) && unicode.IsPunct(curr) {
		// But not for apostrophes, periods in numbers, etc.
		if curr != '\'' && curr != '.' && curr != ',' {
			return true
		}
	}

	// Break after punctuation before letters (except apostrophe)
	if unicode.IsPunct(prev) && prev != '\'' && unicode.IsLetter(curr) {
		return true
	}

	return false
}

// wrapTextInfo contains information for line wrapping decisions.
type wrapTextInfo struct {
	// Original text
	text string
	// Runes of the text
	runes []rune
	// Break opportunities (index i = break before rune i)
	breaks []BreakOpportunity
	// Byte offsets for each rune
	byteOffsets []int
	// Wrap mode
	mode WrapMode
}

// newWrapTextInfo creates wrapping information for text.
func newWrapTextInfo(text string, mode WrapMode) *wrapTextInfo {
	runes := []rune(text)
	n := len(runes)

	// Calculate byte offsets
	offsets := make([]int, n+1)
	offset := 0
	for i, r := range runes {
		offsets[i] = offset
		offset += len(string(r))
	}
	offsets[n] = len(text)

	return &wrapTextInfo{
		text:        text,
		runes:       runes,
		breaks:      findBreakOpportunities(text, mode),
		byteOffsets: offsets,
		mode:        mode,
	}
}

// canBreakAt returns whether a break is allowed before rune index i.
func (w *wrapTextInfo) canBreakAt(i int) bool {
	if i <= 0 || i >= len(w.breaks) {
		return false
	}
	return w.breaks[i] != BreakNo
}

// mustBreakAt returns whether a break is required before rune index i.
func (w *wrapTextInfo) mustBreakAt(i int) bool {
	if i <= 0 || i >= len(w.breaks) {
		return false
	}
	return w.breaks[i] == BreakMandatory
}

// runeToByteOffset converts a rune index to a byte offset.
func (w *wrapTextInfo) runeToByteOffset(runeIdx int) int {
	if runeIdx < 0 {
		return 0
	}
	if runeIdx >= len(w.byteOffsets) {
		return w.byteOffsets[len(w.byteOffsets)-1]
	}
	return w.byteOffsets[runeIdx]
}

// substring returns the substring from rune start to end.
func (w *wrapTextInfo) substring(start, end int) string {
	startByte := w.runeToByteOffset(start)
	endByte := w.runeToByteOffset(end)
	return w.text[startByte:endByte]
}

// markBreakOpportunitiesEnhanced updates glyph info with proper break opportunities.
// This replaces the simple markBreakOpportunities function.
func markBreakOpportunitiesEnhanced(infos []glyphInfo, text string, mode WrapMode) {
	if len(infos) == 0 || text == "" {
		return
	}

	wrapInfo := newWrapTextInfo(text, mode)

	// Map cluster indices to break opportunities
	for i := range infos {
		if i == 0 {
			infos[i].isBreak = false
			continue
		}

		// Get the cluster index (byte offset in original text)
		clusterIdx := infos[i].glyph.Cluster

		// Find the rune index for this cluster
		runeIdx := findRuneIndexForCluster(wrapInfo, clusterIdx)

		// Check if break is allowed before this rune
		infos[i].isBreak = wrapInfo.canBreakAt(runeIdx)
	}
}

// findRuneIndexForCluster finds the rune index corresponding to a byte offset.
func findRuneIndexForCluster(w *wrapTextInfo, byteOffset int) int {
	for i := 0; i < len(w.byteOffsets)-1; i++ {
		if w.byteOffsets[i] <= byteOffset && byteOffset < w.byteOffsets[i+1] {
			return i
		}
	}
	return len(w.runes)
}

// WrapResult represents a wrapped line of text.
type WrapResult struct {
	// Text is the content of this line.
	Text string
	// Start is the byte offset in the original text.
	Start int
	// End is the byte offset in the original text.
	End int
}

// WrapText wraps text to fit within maxWidth using the specified face and options.
// Hard line breaks (\n, \r\n, \r) are respected — each paragraph is wrapped independently.
// The font size is obtained from face.Size().
// Returns a slice of wrapped line results.
func WrapText(text string, face Face, maxWidth float64, mode WrapMode) []WrapResult {
	if text == "" || maxWidth <= 0 {
		return []WrapResult{{Text: text, Start: 0, End: len(text)}}
	}

	if mode == WrapNone {
		return []WrapResult{{Text: text, Start: 0, End: len(text)}}
	}

	// Normalize line endings and split into paragraphs
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	paragraphs := strings.Split(normalized, "\n")

	results := make([]WrapResult, 0, len(paragraphs))
	byteOffset := 0

	for _, para := range paragraphs {
		if para == "" {
			// Empty paragraph (blank line) — preserve as empty line
			results = append(results, WrapResult{
				Text:  "",
				Start: byteOffset,
				End:   byteOffset,
			})
			byteOffset++ // skip the \n
			continue
		}

		// Wrap this paragraph
		paraResults := wrapParagraph(para, face, maxWidth, mode)

		// Adjust byte offsets relative to original text
		for i := range paraResults {
			paraResults[i].Start += byteOffset
			paraResults[i].End += byteOffset
		}

		results = append(results, paraResults...)
		byteOffset += len(para) + 1 // +1 for the \n separator
	}

	return results
}

// wrapParagraph wraps a single paragraph (no hard line breaks) to fit within maxWidth.
func wrapParagraph(para string, face Face, maxWidth float64, mode WrapMode) []WrapResult {
	wrapInfo := newWrapTextInfo(para, mode)
	if len(wrapInfo.runes) == 0 {
		return []WrapResult{{Text: para, Start: 0, End: len(para)}}
	}

	results := make([]WrapResult, 0, 4)
	lineStart := 0 // rune index

	for lineStart < len(wrapInfo.runes) {
		// Find the end of this line
		lineEnd := findLineEnd(wrapInfo, lineStart, face, maxWidth, mode)

		// Create result
		startByte := wrapInfo.runeToByteOffset(lineStart)
		endByte := wrapInfo.runeToByteOffset(lineEnd)

		results = append(results, WrapResult{
			Text:  wrapInfo.text[startByte:endByte],
			Start: startByte,
			End:   endByte,
		})

		// Skip trailing spaces for next line
		lineStart = lineEnd
		for lineStart < len(wrapInfo.runes) && unicode.IsSpace(wrapInfo.runes[lineStart]) {
			lineStart++
		}
	}

	return results
}

// findLineEnd finds the end rune index for a line starting at lineStart.
func findLineEnd(w *wrapTextInfo, lineStart int, face Face, maxWidth float64, mode WrapMode) int {
	if lineStart >= len(w.runes) {
		return lineStart
	}

	// Measure text incrementally to find where it exceeds maxWidth
	var width float64
	lastBreakPoint := -1

	for i := lineStart; i < len(w.runes); i++ {
		// Check for mandatory break
		if w.mustBreakAt(i) && i > lineStart {
			return i
		}

		// Measure the current rune
		runeWidth := measureRune(w.runes[i], face)
		newWidth := width + runeWidth

		// Track break opportunities
		if w.canBreakAt(i) {
			lastBreakPoint = i
		}

		// Check if we exceeded maxWidth
		if newWidth > maxWidth && i > lineStart {
			return calculateLineBreakPosition(w, i, lineStart, lastBreakPoint, mode)
		}

		width = newWidth
	}

	// Reached end of text
	return len(w.runes)
}

// calculateLineBreakPosition determines where to break when line exceeds maxWidth.
func calculateLineBreakPosition(w *wrapTextInfo, pos, lineStart, lastBreakPoint int, mode WrapMode) int {
	// Try to break at last break point
	if lastBreakPoint > lineStart {
		return lastBreakPoint
	}

	switch mode {
	case WrapWordChar, WrapChar:
		// Fall back to character break
		return pos
	case WrapWord:
		// Allow overflow until we find a break point
		for j := pos; j < len(w.runes); j++ {
			if w.canBreakAt(j) {
				return j
			}
		}
		return len(w.runes)
	default:
		return pos
	}
}

// measureRune measures the advance width of a single rune.
func measureRune(r rune, face Face) float64 {
	// Use shaping for accurate measurement
	glyphs := Shape(string(r), face)
	if len(glyphs) == 0 {
		return 0
	}

	var width float64
	for i := range glyphs {
		width += glyphs[i].XAdvance
	}
	return width
}

// MeasureText measures the total advance width of text.
// The font size is obtained from face.Size().
func MeasureText(text string, face Face) float64 {
	if text == "" || face == nil {
		return 0
	}

	glyphs := Shape(text, face)
	var width float64
	for i := range glyphs {
		width += glyphs[i].XAdvance
	}
	return width
}
