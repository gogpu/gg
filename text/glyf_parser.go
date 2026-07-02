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

// GlyfContours holds the raw contour data parsed from the TrueType glyf table.
// For simple glyphs this contains the direct contour points. For composite
// glyphs (numContours < 0), the components are recursively loaded, transformed,
// and merged into a single flattened contour set.
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
// Composite glyphs (numberOfContours < 0) are recursively flattened: each
// component glyph is loaded, its affine transform (offset, scale, 2x2 matrix)
// is applied, and the resulting points and contour endpoints are merged into
// a single GlyfContours. This ensures glyphs like 'i', 'j', and accented
// characters (e.g., 'é', 'ñ') that are stored as composites are correctly
// returned with their full outline data.
//
// Returns nil, nil for:
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
// for the given glyph ID using pure Go binary parsing. Composite glyphs
// (numContours < 0) are recursively flattened via extractCompositeContours.
// Returns nil, nil for empty glyphs (space, etc.) or glyphs with zero contours.
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
		return extractCompositeContours(glyfData, locaData, glyphIndex, isLong, 0)
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

// Composite glyph component flags.
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#composite-glyph-description
const (
	compositeArgWords       = 0x0001 // Arguments are words (int16) vs bytes (int8)
	compositeArgsAreXY      = 0x0002 // Arguments are XY offsets vs point indices
	compositeRoundXYToGrid  = 0x0004 // Round XY offset to grid
	compositeHaveScale      = 0x0008 // Uniform scale (F2.14)
	compositeMoreComponents = 0x0020 // More components follow
	compositeHaveXYScale    = 0x0040 // Separate X and Y scales (2x F2.14)
	compositeHave2x2        = 0x0080 // Full 2x2 affine matrix (4x F2.14)
	compositeHaveInstr      = 0x0100 // Component glyph has instructions
	compositeUseMyMetrics   = 0x0200 // Use metrics from this component
)

// compositeRecursionLimit is the maximum recursion depth for composite glyphs.
// Matches skrifa GLYF_COMPOSITE_RECURSION_LIMIT = 32.
const compositeRecursionLimit = 32

// compositeComponent holds the parsed data for a single component of a
// composite TrueType glyph.
type compositeComponent struct {
	glyphID      uint16 // Component glyph ID
	flags        uint16 // Component flags
	dx, dy       int32  // XY offset (font units, only valid if argsAreXY)
	xx, xy       int32  // F2.14 transform matrix row 1 (default: identity)
	yx, yy       int32  // F2.14 transform matrix row 2 (default: identity)
	hasTransform bool   // True if non-identity transform present
}

// parseCompositeComponents parses the component list from a composite glyph's
// binary data starting at offset pos. Returns the parsed components, the
// byte offset after the last component (for instructions), and any error.
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#composite-glyph-description
//
//nolint:gocognit,nestif,gocritic // TrueType composite glyph binary parsing is inherently complex — direct port of spec.
func parseCompositeComponents(data []byte, startPos int) ([]compositeComponent, int, error) {
	var components []compositeComponent
	pos := startPos

	for {
		if pos+4 > len(data) {
			return nil, pos, fmt.Errorf("truncated component at offset %d", pos)
		}

		flags := binary.BigEndian.Uint16(data[pos : pos+2])
		gid := binary.BigEndian.Uint16(data[pos+2 : pos+4])
		pos += 4

		comp := compositeComponent{
			glyphID: gid,
			flags:   flags,
			xx:      0x4000, // F2.14 identity
			yy:      0x4000,
		}

		// Read offset/anchor arguments.
		if flags&compositeArgWords != 0 {
			if pos+4 > len(data) {
				return nil, pos, fmt.Errorf("truncated word args")
			}
			if flags&compositeArgsAreXY != 0 {
				comp.dx = int32(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
				comp.dy = int32(int16(binary.BigEndian.Uint16(data[pos+2 : pos+4])))
			}
			pos += 4
		} else {
			if pos+2 > len(data) {
				return nil, pos, fmt.Errorf("truncated byte args")
			}
			if flags&compositeArgsAreXY != 0 {
				comp.dx = int32(int8(data[pos]))
				comp.dy = int32(int8(data[pos+1]))
			}
			pos += 2
		}

		// Read optional transform (F2.14 fixed-point, 1.0 = 0x4000).
		if flags&compositeHaveScale != 0 {
			if pos+2 > len(data) {
				return nil, pos, fmt.Errorf("truncated scale")
			}
			s := int32(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			comp.xx = s
			comp.yy = s
			comp.hasTransform = true
			pos += 2
		} else if flags&compositeHaveXYScale != 0 {
			if pos+4 > len(data) {
				return nil, pos, fmt.Errorf("truncated xy scale")
			}
			comp.xx = int32(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			comp.yy = int32(int16(binary.BigEndian.Uint16(data[pos+2 : pos+4])))
			comp.hasTransform = true
			pos += 4
		} else if flags&compositeHave2x2 != 0 {
			if pos+8 > len(data) {
				return nil, pos, fmt.Errorf("truncated 2x2 matrix")
			}
			comp.xx = int32(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			comp.xy = int32(int16(binary.BigEndian.Uint16(data[pos+2 : pos+4])))
			comp.yx = int32(int16(binary.BigEndian.Uint16(data[pos+4 : pos+6])))
			comp.yy = int32(int16(binary.BigEndian.Uint16(data[pos+6 : pos+8])))
			comp.hasTransform = true
			pos += 8
		}

		components = append(components, comp)

		if flags&compositeMoreComponents == 0 {
			break
		}
	}

	return components, pos, nil
}

// extractCompositeContours recursively loads and merges component glyphs
// for a composite glyph (numContours < 0) in the glyf table.
//
// For each component, the function:
//  1. Reads flags, component GID, and offsets from the composite glyph data
//  2. Recursively loads the component's contour points
//  3. Applies the component's affine transform (offset, scale, 2x2 matrix)
//  4. Merges all component points and contour endpoints into a single GlyfContours
//
// Reference: skrifa glyf/mod.rs:784-960 (load_composite)
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#composite-glyph-description
//
//nolint:nilnil // Intentional nil-nil for empty/missing glyphs.
func extractCompositeContours(glyfData, locaData []byte, glyphIndex int, isLong bool, depth int) (*GlyfContours, error) {
	if depth > compositeRecursionLimit {
		return nil, fmt.Errorf("text: glyf parser: composite recursion limit exceeded for glyph %d", glyphIndex)
	}

	off, length := locateGlyph(locaData, glyphIndex, isLong)
	if length == 0 {
		return nil, nil // empty glyph
	}
	end := off + length
	if end > len(glyfData) {
		return nil, fmt.Errorf("text: glyf parser: composite glyph %d offset out of range", glyphIndex)
	}

	data := glyfData[off:end]
	if len(data) < 10 {
		return nil, nil
	}

	// Parse glyph header.
	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	if numContours >= 0 {
		// This is actually a simple glyph, not composite — delegate.
		return extractGlyfContourOwn(glyfData, locaData, glyphIndex, isLong)
	}

	xMin := int16(binary.BigEndian.Uint16(data[2:4]))
	yMin := int16(binary.BigEndian.Uint16(data[4:6]))
	xMax := int16(binary.BigEndian.Uint16(data[6:8]))
	yMax := int16(binary.BigEndian.Uint16(data[8:10]))

	// Parse component list starting after the 10-byte glyph header.
	components, _, err := parseCompositeComponents(data, 10)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: composite glyph %d: %w", glyphIndex, err)
	}

	var allPoints []ContourPoint
	var allEndPts []uint16

	for _, comp := range components {
		// Recursively load the component glyph.
		componentContours, compErr := extractCompositeContours(glyfData, locaData, int(comp.glyphID), isLong, depth+1)
		if compErr != nil {
			return nil, fmt.Errorf("text: glyf parser: composite glyph %d component %d: %w", glyphIndex, comp.glyphID, compErr)
		}

		if componentContours == nil || len(componentContours.Points) == 0 {
			continue
		}

		// Apply transform to component points and merge.
		baseIdx := uint16(len(allPoints))
		for _, pt := range componentContours.Points {
			x := int32(pt.X)
			y := int32(pt.Y)

			if comp.hasTransform {
				// F2.14 multiply: result = (a * b + rounding) >> 14
				newX := (x*comp.xx + y*comp.xy + (1 << 13)) >> 14
				newY := (x*comp.yx + y*comp.yy + (1 << 13)) >> 14
				x = newX
				y = newY
			}

			// Apply offset.
			x += comp.dx
			y += comp.dy

			allPoints = append(allPoints, ContourPoint{
				X:       int16(x),
				Y:       int16(y),
				OnCurve: pt.OnCurve,
			})
		}

		// Shift contour endpoints by the current point base.
		for _, endPt := range componentContours.EndPts {
			allEndPts = append(allEndPts, endPt+baseIdx)
		}
	}

	if len(allPoints) == 0 {
		return nil, nil
	}

	return &GlyfContours{
		Points: allPoints,
		EndPts: allEndPts,
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
// Composite glyphs are recursively flattened (same as ParseGlyfContours).
// Returns nil, nil for empty glyphs (space, etc.).
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
// Composite glyphs are recursively flattened (same as ParseGlyfContours).
// Returns nil, nil for empty glyphs (space, etc.).
func (p *cachedGlyfParser) Contours(gid GlyphID) (*GlyfContours, error) {
	return extractGlyfContourOwn(p.glyfData, p.locaData, int(gid), p.isLong)
}

// NumGlyphs returns the number of glyphs in the font.
func (p *cachedGlyfParser) NumGlyphs() int {
	return p.numGlyphs
}
