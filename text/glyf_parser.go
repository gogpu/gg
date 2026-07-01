// Package text provides GPU text rendering infrastructure.
//
// This file implements a raw TrueType glyf table contour point parser.
// It reads raw contour points from the glyf table, providing the exact
// same data that FreeType's FT_Load_Glyph or skrifa's Outline::fill
// produces: unscaled font-unit coordinates with on-curve/off-curve flags
// and contour end indices.
//
// This is critical for the auto-hinter: FreeType works on raw contour
// points (e.g., 32 points for a glyph), not pen-derived outline segments
// (which may expand to 42+ points due to curve decomposition). The
// auto-hinter's segment detection, stem linking, and point propagation
// must operate on the raw TrueType point representation to achieve
// coordinate parity with FreeType.
//
// References:
//   - TrueType glyf table: https://learn.microsoft.com/en-us/typography/opentype/spec/glyf
//   - FreeType FT_Load_Glyph → FT_GlyphSlot.outline (raw contour points)
//   - skrifa Outline::fill (raw contour iteration)
package text

import (
	"encoding/binary"
	"fmt"
)

// ContourPoint represents a raw TrueType glyph contour point.
// Coordinates are in font units (unscaled design space).
type ContourPoint struct {
	X, Y    int16 // coordinates in font units (unscaled)
	OnCurve bool  // true = on-curve point, false = off-curve control point
}

// GlyfContours holds the raw contour data parsed from the TrueType glyf table
// for a simple (non-composite) glyph.
type GlyfContours struct {
	Points []ContourPoint // all contour points in order
	EndPts []uint16       // index of last point in each contour (increasing)
	XMin   int16          // glyph bounding box from glyf header
	YMin   int16
	XMax   int16
	YMax   int16
}

// NumContours returns the number of contours in the glyph.
func (g *GlyfContours) NumContours() int {
	return len(g.EndPts)
}

// ContourPoints returns the points belonging to contour index ci.
// Returns nil if ci is out of range.
func (g *GlyfContours) ContourPoints(ci int) []ContourPoint {
	if ci < 0 || ci >= len(g.EndPts) {
		return nil
	}
	start := 0
	if ci > 0 {
		start = int(g.EndPts[ci-1]) + 1
	}
	end := int(g.EndPts[ci]) + 1
	if start >= len(g.Points) || end > len(g.Points) {
		return nil
	}
	return g.Points[start:end]
}

// glyfOnCurveFlag is bit 0 of the TrueType glyph point flag.
// When set, the point is on-curve; when clear, it is an off-curve control point.
// See: https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#simple-glyph-description
const glyfOnCurveFlag = 0x01

// ParseGlyfContours reads raw contour points from the TrueType glyf table
// for the specified glyph. This parses the binary font data directly using
// pure Go binary parsing, producing the same point representation that
// FreeType and skrifa use internally.
//
// Returns nil, nil for:
//   - composite glyphs (numberOfContours < 0)
//   - empty glyphs with no outline data (e.g., space)
//   - glyphs with zero contours
//
// The font data must be valid TrueType (.ttf) or OpenType (.otf) with a glyf table.
// CFF-based OpenType fonts do not have a glyf table and will return an error.
func ParseGlyfContours(fontData []byte, gid GlyphID) (*GlyfContours, error) {
	tables, err := parseFontTables(fontData)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: failed to load font: %w", err)
	}

	headData, ok := tables["head"]
	if !ok || len(headData) < 54 {
		return nil, fmt.Errorf("text: glyf parser: missing or invalid head table")
	}

	maxpData, ok := tables["maxp"]
	if !ok || len(maxpData) < 6 {
		return nil, fmt.Errorf("text: glyf parser: missing or invalid maxp table")
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxpData[4:6]))
	if int(gid) >= numGlyphs {
		return nil, fmt.Errorf("text: glyf parser: glyph ID %d out of range (font has %d glyphs)", gid, numGlyphs)
	}

	locaData, ok := tables["loca"]
	if !ok {
		return nil, fmt.Errorf("text: glyf parser: missing loca table")
	}
	isLong := binary.BigEndian.Uint16(headData[50:52]) != 0

	glyfData, ok := tables["glyf"]
	if !ok {
		return nil, fmt.Errorf("text: glyf parser: missing glyf table")
	}

	return extractGlyfContourOwn(glyfData, locaData, int(gid), isLong)
}

// extractGlyfContourOwn extracts raw contour points from the glyf table
// for the given glyph ID using pure Go binary parsing.
// Returns nil, nil for composite or empty glyphs.
//
//nolint:nilnil,gocognit,cyclop,gocyclo,nestif,funlen // Intentional nil-nil; TrueType glyf binary parsing is inherently complex.
func extractGlyfContourOwn(glyfData, locaData []byte, glyphIndex int, isLong bool) (*GlyfContours, error) {
	off, length := locateGlyph(locaData, glyphIndex, isLong)
	if length == 0 {
		return nil, nil // empty glyph (space, etc.)
	}
	end := off + length
	if end > len(glyfData) {
		return nil, fmt.Errorf("text: glyf parser: glyph %d offset out of range", glyphIndex)
	}

	data := glyfData[off:end]
	if len(data) < 10 {
		return nil, nil
	}

	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	xMin := int16(binary.BigEndian.Uint16(data[2:4]))
	yMin := int16(binary.BigEndian.Uint16(data[4:6]))
	xMax := int16(binary.BigEndian.Uint16(data[6:8]))
	yMax := int16(binary.BigEndian.Uint16(data[8:10]))

	if numContours < 0 {
		return nil, nil // composite glyph
	}
	if numContours == 0 {
		return nil, nil
	}

	nc := int(numContours)
	pos := 10

	// Read endPtsOfContours.
	if pos+nc*2 > len(data) {
		return nil, fmt.Errorf("text: glyf parser: glyph %d: endPts overflow", glyphIndex)
	}
	endPts := make([]uint16, nc)
	for i := range nc {
		endPts[i] = binary.BigEndian.Uint16(data[pos : pos+2])
		pos += 2
	}

	numPoints := int(endPts[nc-1]) + 1

	// Skip instructions.
	if pos+2 > len(data) {
		return nil, fmt.Errorf("text: glyf parser: glyph %d: instruction length overflow", glyphIndex)
	}
	instructionLength := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + instructionLength

	if pos > len(data) {
		return nil, fmt.Errorf("text: glyf parser: glyph %d: instructions overflow", glyphIndex)
	}

	// Parse flags.
	flags := make([]byte, numPoints)
	for i := 0; i < numPoints; {
		if pos >= len(data) {
			return nil, fmt.Errorf("text: glyf parser: glyph %d: flags overflow", glyphIndex)
		}
		flag := data[pos]
		pos++
		flags[i] = flag
		i++

		// Repeat flag?
		if flag&0x08 != 0 {
			if pos >= len(data) {
				return nil, fmt.Errorf("text: glyf parser: glyph %d: repeat count overflow", glyphIndex)
			}
			repeat := int(data[pos])
			pos++
			for j := 0; j < repeat && i < numPoints; j++ {
				flags[i] = flag
				i++
			}
		}
	}

	// Parse X coordinates.
	xs := make([]int16, numPoints)
	var prevX int16
	for i := range numPoints {
		f := flags[i]
		xShort := f&0x02 != 0
		xSame := f&0x10 != 0
		if xShort {
			if pos >= len(data) {
				return nil, fmt.Errorf("text: glyf parser: glyph %d: X coord overflow", glyphIndex)
			}
			val := int16(data[pos])
			pos++
			if !xSame {
				val = -val
			}
			prevX += val
		} else if !xSame {
			if pos+2 > len(data) {
				return nil, fmt.Errorf("text: glyf parser: glyph %d: X coord overflow", glyphIndex)
			}
			prevX += int16(binary.BigEndian.Uint16(data[pos : pos+2]))
			pos += 2
		}
		// else: xSame && !xShort → same as previous
		xs[i] = prevX
	}

	// Parse Y coordinates.
	ys := make([]int16, numPoints)
	var prevY int16
	for i := range numPoints {
		f := flags[i]
		yShort := f&0x04 != 0
		ySame := f&0x20 != 0
		if yShort {
			if pos >= len(data) {
				return nil, fmt.Errorf("text: glyf parser: glyph %d: Y coord overflow", glyphIndex)
			}
			val := int16(data[pos])
			pos++
			if !ySame {
				val = -val
			}
			prevY += val
		} else if !ySame {
			if pos+2 > len(data) {
				return nil, fmt.Errorf("text: glyf parser: glyph %d: Y coord overflow", glyphIndex)
			}
			prevY += int16(binary.BigEndian.Uint16(data[pos : pos+2]))
			pos += 2
		}
		ys[i] = prevY
	}

	// Build result.
	points := make([]ContourPoint, numPoints)
	for i := range numPoints {
		points[i] = ContourPoint{
			X:       xs[i],
			Y:       ys[i],
			OnCurve: flags[i]&glyfOnCurveFlag != 0,
		}
	}

	return &GlyfContours{
		Points: points,
		EndPts: endPts,
		XMin:   xMin,
		YMin:   yMin,
		XMax:   xMax,
		YMax:   yMax,
	}, nil
}

// ParseGlyfContoursFromSource reads raw contour points for a glyph from a
// FontSource. This is a convenience method that extracts the raw font data
// from the FontSource and delegates to ParseGlyfContours.
//
// Returns nil, nil for composite or empty glyphs.
func ParseGlyfContoursFromSource(source *FontSource, gid GlyphID) (*GlyfContours, error) {
	if source == nil {
		return nil, fmt.Errorf("text: glyf parser: nil FontSource")
	}

	source.mu.RLock()
	data := source.data
	source.mu.RUnlock()

	if len(data) == 0 {
		return nil, fmt.Errorf("text: glyf parser: FontSource has no data (closed?)")
	}

	return ParseGlyfContours(data, gid)
}

// cachedGlyfParser caches parsed table data for repeated glyph lookups
// from the same font. This avoids re-parsing the head, maxp, loca, and
// glyf tables on every call when iterating over multiple glyphs.
type cachedGlyfParser struct {
	glyfData  []byte
	locaData  []byte
	isLong    bool
	numGlyphs int
}

// newCachedGlyfParser creates a parser that caches the parsed table data
// for efficient repeated glyph lookups.
func newCachedGlyfParser(fontData []byte) (*cachedGlyfParser, error) {
	tables, err := parseFontTables(fontData)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: failed to load font: %w", err)
	}

	headData, ok := tables["head"]
	if !ok || len(headData) < 54 {
		return nil, fmt.Errorf("text: glyf parser: missing or invalid head table")
	}

	maxpData, ok := tables["maxp"]
	if !ok || len(maxpData) < 6 {
		return nil, fmt.Errorf("text: glyf parser: missing or invalid maxp table")
	}

	locaData, ok := tables["loca"]
	if !ok {
		return nil, fmt.Errorf("text: glyf parser: missing loca table")
	}

	glyfData, ok := tables["glyf"]
	if !ok {
		return nil, fmt.Errorf("text: glyf parser: missing glyf table")
	}

	isLong := binary.BigEndian.Uint16(headData[50:52]) != 0
	numGlyphs := int(binary.BigEndian.Uint16(maxpData[4:6]))

	return &cachedGlyfParser{
		glyfData:  glyfData,
		locaData:  locaData,
		isLong:    isLong,
		numGlyphs: numGlyphs,
	}, nil
}

// Contours extracts raw contour points for the given glyph ID.
// Returns nil, nil for composite or empty glyphs.
func (p *cachedGlyfParser) Contours(gid GlyphID) (*GlyfContours, error) {
	return extractGlyfContourOwn(p.glyfData, p.locaData, int(gid), p.isLong)
}

// NumGlyphs returns the number of glyphs in the font.
func (p *cachedGlyfParser) NumGlyphs() int {
	return p.numGlyphs
}
