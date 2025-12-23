package text

import (
	"strings"
	"unicode"
)

// Alignment specifies text horizontal alignment within the layout width.
type Alignment int

const (
	// AlignLeft aligns text to the left edge (default).
	AlignLeft Alignment = iota
	// AlignCenter centers text horizontally.
	AlignCenter
	// AlignRight aligns text to the right edge.
	AlignRight
	// AlignJustify distributes text evenly (future implementation).
	AlignJustify
)

// String returns the string representation of the alignment.
func (a Alignment) String() string {
	switch a {
	case AlignLeft:
		return "Left"
	case AlignCenter:
		return "Center"
	case AlignRight:
		return "Right"
	case AlignJustify:
		return "Justify"
	default:
		return unknownStr
	}
}

// LayoutOptions configures text layout behavior.
type LayoutOptions struct {
	// MaxWidth is the maximum line width in pixels.
	// If 0, no line wrapping is performed (single-line paragraphs).
	MaxWidth float64

	// LineSpacing is a multiplier for line height.
	// 1.0 uses the font's natural line height; 1.5 adds 50% extra space.
	LineSpacing float64

	// Alignment specifies horizontal text alignment.
	Alignment Alignment

	// Direction is the base text direction (LTR or RTL).
	// Used for paragraph-level direction when no strong directional text is present.
	Direction Direction
}

// DefaultLayoutOptions returns sensible default layout options.
func DefaultLayoutOptions() LayoutOptions {
	return LayoutOptions{
		MaxWidth:    0, // No wrapping
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
		Direction:   DirectionLTR,
	}
}

// Line represents a positioned line of text ready for rendering.
type Line struct {
	// Runs contains the shaped runs that make up this line.
	// Multiple runs occur with mixed scripts or directions.
	Runs []ShapedRun

	// Glyphs contains all glyphs from Runs, positioned for rendering.
	// Glyph X positions are absolute within the layout.
	Glyphs []ShapedGlyph

	// Width is the total advance width of all glyphs in this line.
	Width float64

	// Ascent is the maximum ascent of all runs (distance above baseline).
	Ascent float64

	// Descent is the maximum descent of all runs (distance below baseline).
	Descent float64

	// Y is the baseline Y position of this line within the layout.
	Y float64
}

// Height returns the total height of the line (ascent + descent).
func (l *Line) Height() float64 {
	return l.Ascent + l.Descent
}

// Layout represents the result of text layout.
type Layout struct {
	// Lines contains all lines of laid out text.
	Lines []Line

	// Width is the maximum width among all lines.
	Width float64

	// Height is the total height of all lines.
	Height float64
}

// LayoutText performs text layout with the given options.
// It segments text by direction/script, shapes each segment,
// wraps lines if MaxWidth > 0, and positions lines with alignment.
func LayoutText(text string, face Face, size float64, opts LayoutOptions) *Layout {
	if text == "" {
		return &Layout{}
	}
	if face == nil {
		return &Layout{}
	}

	// Apply defaults for zero values
	if opts.LineSpacing <= 0 {
		opts.LineSpacing = 1.0
	}

	// Get font metrics for line height calculation
	metrics := face.Metrics()

	// Split by hard line breaks
	paragraphs := splitParagraphs(text)

	layout := &Layout{
		Lines: make([]Line, 0, len(paragraphs)),
	}

	var y float64

	for _, para := range paragraphs {
		paraLines := layoutParagraph(para, face, size, opts, metrics)

		for i := range paraLines {
			line := &paraLines[i]

			// Calculate baseline position: previous Y + line ascent
			if len(layout.Lines) == 0 {
				y = line.Ascent
			} else {
				// Add spacing between lines
				prevLine := &layout.Lines[len(layout.Lines)-1]
				lineGap := metrics.LineGap * opts.LineSpacing
				y = prevLine.Y + prevLine.Descent + lineGap + line.Ascent
			}

			line.Y = y

			// Apply horizontal alignment
			applyAlignment(line, opts.Alignment, opts.MaxWidth)

			// Track layout bounds
			if line.Width > layout.Width {
				layout.Width = line.Width
			}
		}

		layout.Lines = append(layout.Lines, paraLines...)
	}

	// Calculate total height
	if len(layout.Lines) > 0 {
		lastLine := &layout.Lines[len(layout.Lines)-1]
		layout.Height = lastLine.Y + lastLine.Descent
	}

	return layout
}

// LayoutTextSimple is a convenience wrapper with default options.
func LayoutTextSimple(text string, face Face, size float64) *Layout {
	return LayoutText(text, face, size, DefaultLayoutOptions())
}

// splitParagraphs splits text by hard line breaks.
func splitParagraphs(text string) []string {
	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	return strings.Split(text, "\n")
}

// layoutParagraph lays out a single paragraph (no hard breaks).
func layoutParagraph(para string, face Face, size float64, opts LayoutOptions, metrics Metrics) []Line {
	if para == "" {
		// Empty paragraph still produces a line (for line height)
		return []Line{{
			Runs:    nil,
			Glyphs:  nil,
			Width:   0,
			Ascent:  metrics.Ascent,
			Descent: metrics.Descent,
		}}
	}

	// Segment the paragraph by direction and script
	segmenter := NewBuiltinSegmenterWithDirection(opts.Direction)
	segments := segmenter.Segment(para)

	if len(segments) == 0 {
		return nil
	}

	// Shape each segment
	runs := shapeSegments(segments, face, size, metrics)

	// If no wrapping needed, return as single line
	if opts.MaxWidth <= 0 {
		return []Line{createLine(runs)}
	}

	// Wrap lines at MaxWidth
	return wrapLines(runs, opts.MaxWidth, metrics)
}

// shapeSegments shapes each segment and returns ShapedRuns.
func shapeSegments(segments []Segment, face Face, size float64, metrics Metrics) []ShapedRun {
	runs := make([]ShapedRun, 0, len(segments))
	var xOffset float64

	for _, seg := range segments {
		glyphs := Shape(seg.Text, face, size)
		if len(glyphs) == 0 {
			continue
		}

		// Adjust glyph positions by xOffset
		for i := range glyphs {
			glyphs[i].X += xOffset
			// Adjust cluster indices to be relative to paragraph start
			glyphs[i].Cluster += seg.Start
		}

		// Calculate run advance
		var advance float64
		if len(glyphs) > 0 {
			lastGlyph := &glyphs[len(glyphs)-1]
			advance = lastGlyph.X - xOffset + lastGlyph.XAdvance
		}

		run := ShapedRun{
			Glyphs:    glyphs,
			Advance:   advance,
			Ascent:    metrics.Ascent,
			Descent:   metrics.Descent,
			Direction: seg.Direction,
			Face:      face,
			Size:      size,
		}
		runs = append(runs, run)

		xOffset += advance
	}

	return runs
}

// createLine creates a Line from runs.
func createLine(runs []ShapedRun) Line {
	line := Line{
		Runs:   runs,
		Glyphs: make([]ShapedGlyph, 0),
	}

	for i := range runs {
		run := &runs[i]
		line.Glyphs = append(line.Glyphs, run.Glyphs...)
		line.Width += run.Advance

		if run.Ascent > line.Ascent {
			line.Ascent = run.Ascent
		}
		if run.Descent > line.Descent {
			line.Descent = run.Descent
		}
	}

	return line
}

// glyphInfo is used internally for line wrapping.
type glyphInfo struct {
	glyph   ShapedGlyph
	runIdx  int
	isBreak bool
}

// wrapLines wraps runs into lines at maxWidth.
func wrapLines(runs []ShapedRun, maxWidth float64, metrics Metrics) []Line {
	if len(runs) == 0 {
		return nil
	}

	// Flatten all glyphs with their run info for easier processing
	infos := make([]glyphInfo, 0)
	for runIdx, run := range runs {
		for _, g := range run.Glyphs {
			infos = append(infos, glyphInfo{
				glyph:   g,
				runIdx:  runIdx,
				isBreak: false,
			})
		}
	}

	if len(infos) == 0 {
		return []Line{{
			Ascent:  metrics.Ascent,
			Descent: metrics.Descent,
		}}
	}

	// Mark break opportunities (after spaces)
	markBreakOpportunities(infos)

	// Greedy line breaking
	lines := make([]Line, 0)
	lineStart := 0
	lastBreak := -1
	startX := infos[0].glyph.X

	for i := range infos {
		info := &infos[i]
		glyphEnd := info.glyph.X - startX + info.glyph.XAdvance

		if info.isBreak {
			lastBreak = i
		}

		if glyphEnd > maxWidth && lineStart < i {
			// Line exceeds max width, need to break
			breakAt := i
			if lastBreak > lineStart {
				// Break at last break opportunity
				breakAt = lastBreak + 1
			}

			// Create line from lineStart to breakAt
			line := createLineFromGlyphs(infos[lineStart:breakAt], runs, startX, metrics)
			lines = append(lines, line)

			// Start new line
			lineStart = breakAt
			lastBreak = -1
			if lineStart < len(infos) {
				startX = infos[lineStart].glyph.X
			}
		}
	}

	// Add remaining glyphs as final line
	if lineStart < len(infos) {
		line := createLineFromGlyphs(infos[lineStart:], runs, startX, metrics)
		lines = append(lines, line)
	}

	return lines
}

// markBreakOpportunities marks positions where line breaks can occur.
// Currently uses a simple heuristic - every glyph is a potential break point.
// Future: Use Unicode line breaking algorithm (UAX #14).
func markBreakOpportunities(infos []glyphInfo) {
	// Simple heuristic: allow break at every glyph boundary.
	// This is a placeholder for proper Unicode line breaking.
	for i := range infos {
		infos[i].isBreak = true
	}
}

// createLineFromGlyphs creates a Line from a slice of glyph infos.
func createLineFromGlyphs(infos []glyphInfo, runs []ShapedRun, startX float64, metrics Metrics) Line {
	line := Line{
		Runs:    make([]ShapedRun, 0),
		Glyphs:  make([]ShapedGlyph, 0, len(infos)),
		Ascent:  metrics.Ascent,
		Descent: metrics.Descent,
	}

	if len(infos) == 0 {
		return line
	}

	// Collect glyphs and adjust X positions
	currentRunIdx := -1
	var currentRun *ShapedRun

	for _, info := range infos {
		// Clone glyph and adjust X position to start from 0
		g := info.glyph
		g.X -= startX

		line.Glyphs = append(line.Glyphs, g)

		// Track runs
		if info.runIdx != currentRunIdx && info.runIdx < len(runs) {
			currentRunIdx = info.runIdx
			srcRun := &runs[info.runIdx]
			// Create a new run for this line
			currentRun = &ShapedRun{
				Glyphs:    make([]ShapedGlyph, 0),
				Direction: srcRun.Direction,
				Face:      srcRun.Face,
				Size:      srcRun.Size,
				Ascent:    srcRun.Ascent,
				Descent:   srcRun.Descent,
			}
			line.Runs = append(line.Runs, *currentRun)
		}

		// Add glyph to current run
		if len(line.Runs) > 0 {
			lastRunIdx := len(line.Runs) - 1
			line.Runs[lastRunIdx].Glyphs = append(line.Runs[lastRunIdx].Glyphs, g)
		}
	}

	// Calculate line width and run advances
	if len(line.Glyphs) > 0 {
		lastGlyph := &line.Glyphs[len(line.Glyphs)-1]
		line.Width = lastGlyph.X + lastGlyph.XAdvance
	}

	// Update run advances
	for i := range line.Runs {
		run := &line.Runs[i]
		if len(run.Glyphs) > 0 {
			firstG := &run.Glyphs[0]
			lastG := &run.Glyphs[len(run.Glyphs)-1]
			run.Advance = lastG.X - firstG.X + lastG.XAdvance
		}
	}

	return line
}

// applyAlignment adjusts glyph X positions based on alignment.
func applyAlignment(line *Line, alignment Alignment, maxWidth float64) {
	if len(line.Glyphs) == 0 {
		return
	}

	var offset float64

	switch alignment {
	case AlignLeft:
		// No adjustment needed
		return
	case AlignCenter:
		containerWidth := maxWidth
		if containerWidth <= 0 {
			containerWidth = line.Width
		}
		offset = (containerWidth - line.Width) / 2
	case AlignRight:
		containerWidth := maxWidth
		if containerWidth <= 0 {
			containerWidth = line.Width
		}
		offset = containerWidth - line.Width
	case AlignJustify:
		// Justify: distribute extra space between words
		// For now, treat as left-aligned (future implementation)
		return
	}

	if offset <= 0 {
		return
	}

	// Adjust all glyph positions
	for i := range line.Glyphs {
		line.Glyphs[i].X += offset
	}

	// Adjust run glyphs as well
	for i := range line.Runs {
		for j := range line.Runs[i].Glyphs {
			line.Runs[i].Glyphs[j].X += offset
		}
	}
}

// isWordBreakRune returns true if the rune is a word break opportunity.
func isWordBreakRune(r rune) bool {
	return unicode.IsSpace(r) || isCJK(r)
}

// isCJK returns true if the rune is a CJK character.
// CJK characters can break anywhere (no word boundaries).
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul Syllables
}
