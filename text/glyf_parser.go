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
	"bytes"
	"fmt"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
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
// the go-text/typesetting table parsers, producing the same point representation
// that FreeType and skrifa use internally.
//
// Returns nil, nil for:
//   - composite glyphs (numberOfContours < 0)
//   - empty glyphs with no outline data (e.g., space)
//   - glyphs with zero contours
//
// The font data must be valid TrueType (.ttf) or OpenType (.otf) with a glyf table.
// CFF-based OpenType fonts do not have a glyf table and will return an error.
func ParseGlyfContours(fontData []byte, gid GlyphID) (*GlyfContours, error) {
	loader, err := ot.NewLoader(bytes.NewReader(fontData))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: failed to load font: %w", err)
	}

	// Parse head table to get indexToLocFormat (short vs long loca offsets).
	headRaw, err := loader.RawTable(ot.MustNewTag("head"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing head table: %w", err)
	}
	head, _, err := tables.ParseHead(headRaw)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid head table: %w", err)
	}

	// Parse maxp table to get numGlyphs.
	maxpRaw, err := loader.RawTable(ot.MustNewTag("maxp"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing maxp table: %w", err)
	}
	maxp, _, err := tables.ParseMaxp(maxpRaw)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid maxp table: %w", err)
	}

	numGlyphs := int(maxp.NumGlyphs)
	if int(gid) >= numGlyphs {
		return nil, fmt.Errorf("text: glyf parser: glyph ID %d out of range (font has %d glyphs)", gid, numGlyphs)
	}

	// Parse loca table to get glyph offsets within glyf.
	locaRaw, err := loader.RawTable(ot.MustNewTag("loca"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing loca table: %w", err)
	}
	// head.IndexToLocFormat: 0 = short (uint16 offsets / 2), 1 = long (uint32 offsets).
	isLong := head.IndexToLocFormat != 0
	locaOffsets, err := tables.ParseLoca(locaRaw, numGlyphs, isLong)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid loca table: %w", err)
	}

	// Parse glyf table.
	glyfRaw, err := loader.RawTable(ot.MustNewTag("glyf"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing glyf table: %w", err)
	}
	glyf, err := tables.ParseGlyf(glyfRaw, locaOffsets)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid glyf table: %w", err)
	}

	return extractSimpleGlyph(glyf, gid)
}

// extractSimpleGlyph extracts raw contour points from a parsed Glyf table
// for the given glyph ID. Returns nil, nil for composite or empty glyphs.
// The nil-nil return is intentional API design: nil result = "no simple outline"
// (empty glyph like space, or composite glyph) without an error condition.
//
//nolint:nilnil // Intentional: nil result means "no simple outline data", not an error.
func extractSimpleGlyph(glyf tables.Glyf, gid GlyphID) (*GlyfContours, error) {
	if int(gid) >= len(glyf) {
		return nil, fmt.Errorf("text: glyf parser: glyph ID %d out of range", gid)
	}

	g := glyf[gid]

	// Empty glyph (no outline data, e.g., space).
	if g.Data == nil {
		return nil, nil
	}

	// Check if this is a simple glyph (not composite).
	simple, ok := g.Data.(tables.SimpleGlyph)
	if !ok {
		// Composite glyph — return nil without error.
		return nil, nil
	}

	if len(simple.EndPtsOfContours) == 0 || len(simple.Points) == 0 {
		return nil, nil
	}

	// Build the result.
	result := &GlyfContours{
		Points: make([]ContourPoint, len(simple.Points)),
		EndPts: make([]uint16, len(simple.EndPtsOfContours)),
		XMin:   g.XMin,
		YMin:   g.YMin,
		XMax:   g.XMax,
		YMax:   g.YMax,
	}

	copy(result.EndPts, simple.EndPtsOfContours)

	for i, pt := range simple.Points {
		result.Points[i] = ContourPoint{
			X:       pt.X,
			Y:       pt.Y,
			OnCurve: (pt.Flag & glyfOnCurveFlag) != 0,
		}
	}

	return result, nil
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
	glyf tables.Glyf
}

// newCachedGlyfParser creates a parser that caches the parsed glyf table
// for efficient repeated glyph lookups.
func newCachedGlyfParser(fontData []byte) (*cachedGlyfParser, error) {
	loader, err := ot.NewLoader(bytes.NewReader(fontData))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: failed to load font: %w", err)
	}

	headRaw, err := loader.RawTable(ot.MustNewTag("head"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing head table: %w", err)
	}
	head, _, err := tables.ParseHead(headRaw)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid head table: %w", err)
	}

	maxpRaw, err := loader.RawTable(ot.MustNewTag("maxp"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing maxp table: %w", err)
	}
	maxp, _, err := tables.ParseMaxp(maxpRaw)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid maxp table: %w", err)
	}

	locaRaw, err := loader.RawTable(ot.MustNewTag("loca"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing loca table: %w", err)
	}
	isLong := head.IndexToLocFormat != 0
	locaOffsets, err := tables.ParseLoca(locaRaw, int(maxp.NumGlyphs), isLong)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid loca table: %w", err)
	}

	glyfRaw, err := loader.RawTable(ot.MustNewTag("glyf"))
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: missing glyf table: %w", err)
	}
	glyf, err := tables.ParseGlyf(glyfRaw, locaOffsets)
	if err != nil {
		return nil, fmt.Errorf("text: glyf parser: invalid glyf table: %w", err)
	}

	return &cachedGlyfParser{glyf: glyf}, nil
}

// Contours extracts raw contour points for the given glyph ID.
// Returns nil, nil for composite or empty glyphs.
func (p *cachedGlyfParser) Contours(gid GlyphID) (*GlyfContours, error) {
	return extractSimpleGlyph(p.glyf, gid)
}

// NumGlyphs returns the number of glyphs in the font.
func (p *cachedGlyfParser) NumGlyphs() int {
	return len(p.glyf)
}
