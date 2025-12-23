package text

import (
	"unicode"

	"golang.org/x/text/unicode/bidi"
)

// Segment represents a contiguous run of text with the same direction and script.
type Segment struct {
	Text      string
	Start     int
	End       int
	Direction Direction
	Script    Script
	Level     int
}

func (s Segment) RuneCount() int {
	count := 0
	for range s.Text {
		count++
	}
	return count
}

type Segmenter interface {
	Segment(text string) []Segment
}

type BuiltinSegmenter struct {
	BaseDirection Direction
}

func NewBuiltinSegmenter() *BuiltinSegmenter {
	return &BuiltinSegmenter{BaseDirection: DirectionLTR}
}

func NewBuiltinSegmenterWithDirection(dir Direction) *BuiltinSegmenter {
	return &BuiltinSegmenter{BaseDirection: dir}
}

func (s *BuiltinSegmenter) Segment(text string) []Segment {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	levels := s.computeBidiLevels(text)
	scripts := s.detectScripts(runes)
	scripts = s.resolveInheritedScripts(scripts)
	return s.buildSegments(text, runes, levels, scripts)
}

func (s *BuiltinSegmenter) computeBidiLevels(text string) []int {
	runes := []rune(text)
	levels := make([]int, len(runes))

	var defaultDir bidi.Direction
	if s.BaseDirection == DirectionRTL {
		defaultDir = bidi.RightToLeft
	} else {
		defaultDir = bidi.Neutral
	}

	p := bidi.Paragraph{}
	_, _ = p.SetString(text, bidi.DefaultDirection(defaultDir))

	ordering, err := p.Order()
	if err != nil {
		return levels
	}

	// run.Pos() returns RUNE indices (start, end inclusive)
	for i := 0; i < ordering.NumRuns(); i++ {
		run := ordering.Run(i)
		startRune, endRune := run.Pos()
		runLevel := 0
		if run.Direction() == bidi.RightToLeft {
			runLevel = 1
		}
		for j := startRune; j <= endRune && j < len(levels); j++ {
			levels[j] = runLevel
		}
	}

	return levels
}

func (s *BuiltinSegmenter) detectScripts(runes []rune) []Script {
	scripts := make([]Script, len(runes))
	for i, r := range runes {
		scripts[i] = DetectScript(r)
	}
	return scripts
}

func (s *BuiltinSegmenter) resolveInheritedScripts(scripts []Script) []Script {
	resolved := make([]Script, len(scripts))
	copy(resolved, scripts)

	lastConcreteScript := ScriptCommon
	for i := range resolved {
		if resolved[i] == ScriptInherited {
			resolved[i] = lastConcreteScript
		} else if resolved[i] != ScriptCommon {
			lastConcreteScript = resolved[i]
		}
	}

	lastConcreteScript = ScriptCommon
	for i := range resolved {
		if resolved[i] != ScriptCommon {
			if resolved[i] != ScriptInherited {
				lastConcreteScript = resolved[i]
			}
			continue
		}
		// Resolve Common script from context
		nextConcrete := findNextConcreteScript(resolved, i+1)
		resolved[i] = resolveCommonScript(lastConcreteScript, nextConcrete)
	}

	return resolved
}

// findNextConcreteScript finds the next non-Common, non-Inherited script starting at index start.
func findNextConcreteScript(scripts []Script, start int) Script {
	for j := start; j < len(scripts); j++ {
		if scripts[j] != ScriptCommon && scripts[j] != ScriptInherited {
			return scripts[j]
		}
	}
	return ScriptCommon
}

// resolveCommonScript determines what script a Common character should inherit.
func resolveCommonScript(prev, next Script) Script {
	switch {
	case prev != ScriptCommon && prev == next:
		return prev
	case prev != ScriptCommon && next == ScriptCommon:
		return prev
	case prev == ScriptCommon && next != ScriptCommon:
		return next
	default:
		return ScriptCommon
	}
}

func (s *BuiltinSegmenter) buildSegments(text string, runes []rune, levels []int, scripts []Script) []Segment {
	if len(runes) == 0 {
		return nil
	}

	segments := make([]Segment, 0, 4)
	byteOffsets := computeByteOffsets(text, runes)

	currentLevel := levels[0]
	currentScript := scripts[0]
	segmentStartRune := 0

	for i := 1; i < len(runes); i++ {
		if levels[i] == currentLevel && scripts[i] == currentScript {
			continue
		}

		seg := makeSegment(text, byteOffsets, segmentStartRune, i, currentLevel, currentScript)
		segments = append(segments, seg)

		segmentStartRune = i
		currentLevel = levels[i]
		currentScript = scripts[i]
	}

	seg := makeSegment(text, byteOffsets, segmentStartRune, len(runes), currentLevel, currentScript)
	segments = append(segments, seg)

	return segments
}

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

func makeSegment(text string, byteOffsets []int, startRune, endRune int, level int, script Script) Segment {
	startByte := byteOffsets[startRune]
	endByte := byteOffsets[endRune]

	dir := DirectionLTR
	if level%2 == 1 {
		dir = DirectionRTL
	}

	return Segment{
		Text:      text[startByte:endByte],
		Start:     startByte,
		End:       endByte,
		Direction: dir,
		Script:    script,
		Level:     level,
	}
}

func SegmentText(text string) []Segment {
	return NewBuiltinSegmenter().Segment(text)
}

func SegmentTextRTL(text string) []Segment {
	return NewBuiltinSegmenterWithDirection(DirectionRTL).Segment(text)
}

func IsWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}

func IsPunctuation(r rune) bool {
	return unicode.IsPunct(r)
}
