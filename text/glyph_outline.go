// Package text provides GPU text rendering infrastructure.
package text

import (
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// OutlinePoint represents a point in a glyph outline.
// All coordinates are in font units and should be scaled by size/unitsPerEm.
type OutlinePoint struct {
	X, Y float32
}

// OutlineSegment represents a segment of a glyph outline.
type OutlineSegment struct {
	// Op is the segment operation type.
	Op OutlineOp

	// Points contains the control and end points for this segment.
	// - MoveTo: Points[0] is the target point
	// - LineTo: Points[0] is the target point
	// - QuadTo: Points[0] is control, Points[1] is target
	// - CubicTo: Points[0], Points[1] are controls, Points[2] is target
	Points [3]OutlinePoint
}

// OutlineOp is the type of path operation.
type OutlineOp uint8

const (
	// OutlineOpMoveTo moves to a new point without drawing.
	OutlineOpMoveTo OutlineOp = iota

	// OutlineOpLineTo draws a line to the target point.
	OutlineOpLineTo

	// OutlineOpQuadTo draws a quadratic bezier curve.
	OutlineOpQuadTo

	// OutlineOpCubicTo draws a cubic bezier curve.
	OutlineOpCubicTo
)

// String returns a string representation of the operation.
func (op OutlineOp) String() string {
	switch op {
	case OutlineOpMoveTo:
		return "MoveTo"
	case OutlineOpLineTo:
		return "LineTo"
	case OutlineOpQuadTo:
		return "QuadTo"
	case OutlineOpCubicTo:
		return "CubicTo"
	default:
		return "Unknown"
	}
}

// GlyphOutline represents the vector outline of a glyph.
// The outline consists of one or more closed contours.
type GlyphOutline struct {
	// Segments is the list of path segments that make up the outline.
	Segments []OutlineSegment

	// Bounds is the bounding box of the outline in scaled units.
	Bounds Rect

	// Advance is the horizontal advance width of the glyph.
	Advance float32

	// LSB is the left side bearing.
	LSB float32

	// GID is the glyph ID this outline represents.
	GID GlyphID

	// Type indicates the type of glyph (outline, bitmap, COLR).
	Type GlyphType
}

// IsEmpty returns true if the outline has no segments.
func (o *GlyphOutline) IsEmpty() bool {
	return len(o.Segments) == 0
}

// SegmentCount returns the number of segments in the outline.
func (o *GlyphOutline) SegmentCount() int {
	return len(o.Segments)
}

// Clone creates a deep copy of the outline.
func (o *GlyphOutline) Clone() *GlyphOutline {
	if o == nil {
		return nil
	}

	clone := &GlyphOutline{
		Segments: make([]OutlineSegment, len(o.Segments)),
		Bounds:   o.Bounds,
		Advance:  o.Advance,
		LSB:      o.LSB,
		GID:      o.GID,
		Type:     o.Type,
	}
	copy(clone.Segments, o.Segments)
	return clone
}

// Scale returns a new outline with all coordinates scaled by the given factor.
func (o *GlyphOutline) Scale(factor float32) *GlyphOutline {
	if o == nil {
		return nil
	}

	scaled := &GlyphOutline{
		Segments: make([]OutlineSegment, len(o.Segments)),
		Bounds: Rect{
			MinX: o.Bounds.MinX * float64(factor),
			MinY: o.Bounds.MinY * float64(factor),
			MaxX: o.Bounds.MaxX * float64(factor),
			MaxY: o.Bounds.MaxY * float64(factor),
		},
		Advance: o.Advance * factor,
		LSB:     o.LSB * factor,
		GID:     o.GID,
		Type:    o.Type,
	}

	for i, seg := range o.Segments {
		scaled.Segments[i] = OutlineSegment{
			Op: seg.Op,
			Points: [3]OutlinePoint{
				{X: seg.Points[0].X * factor, Y: seg.Points[0].Y * factor},
				{X: seg.Points[1].X * factor, Y: seg.Points[1].Y * factor},
				{X: seg.Points[2].X * factor, Y: seg.Points[2].Y * factor},
			},
		}
	}

	return scaled
}

// Translate returns a new outline with all coordinates translated by (dx, dy).
func (o *GlyphOutline) Translate(dx, dy float32) *GlyphOutline {
	if o == nil {
		return nil
	}

	translated := &GlyphOutline{
		Segments: make([]OutlineSegment, len(o.Segments)),
		Bounds: Rect{
			MinX: o.Bounds.MinX + float64(dx),
			MinY: o.Bounds.MinY + float64(dy),
			MaxX: o.Bounds.MaxX + float64(dx),
			MaxY: o.Bounds.MaxY + float64(dy),
		},
		Advance: o.Advance,
		LSB:     o.LSB,
		GID:     o.GID,
		Type:    o.Type,
	}

	for i, seg := range o.Segments {
		translated.Segments[i] = OutlineSegment{
			Op: seg.Op,
			Points: [3]OutlinePoint{
				{X: seg.Points[0].X + dx, Y: seg.Points[0].Y + dy},
				{X: seg.Points[1].X + dx, Y: seg.Points[1].Y + dy},
				{X: seg.Points[2].X + dx, Y: seg.Points[2].Y + dy},
			},
		}
	}

	return translated
}

// Transform returns a new outline with all coordinates transformed.
func (o *GlyphOutline) Transform(m *AffineTransform) *GlyphOutline {
	if o == nil || m == nil {
		return o.Clone()
	}

	transformed := &GlyphOutline{
		Segments: make([]OutlineSegment, len(o.Segments)),
		Advance:  o.Advance,
		LSB:      o.LSB,
		GID:      o.GID,
		Type:     o.Type,
	}

	// Transform all segments and compute new bounds
	minX, minY := float32(1e10), float32(1e10)
	maxX, maxY := float32(-1e10), float32(-1e10)

	for i, seg := range o.Segments {
		transformed.Segments[i] = OutlineSegment{Op: seg.Op}

		pointCount := 1
		switch seg.Op {
		case OutlineOpMoveTo, OutlineOpLineTo:
			pointCount = 1
		case OutlineOpQuadTo:
			pointCount = 2
		case OutlineOpCubicTo:
			pointCount = 3
		}

		for j := 0; j < pointCount; j++ {
			x, y := m.TransformPoint(seg.Points[j].X, seg.Points[j].Y)
			transformed.Segments[i].Points[j] = OutlinePoint{X: x, Y: y}

			updateMinMax(x, y, &minX, &minY, &maxX, &maxY)
		}
	}

	if len(o.Segments) > 0 {
		transformed.Bounds = Rect{
			MinX: float64(minX),
			MinY: float64(minY),
			MaxX: float64(maxX),
			MaxY: float64(maxY),
		}
	}

	return transformed
}

// updateMinMax updates min/max bounds.
func updateMinMax(x, y float32, minX, minY, maxX, maxY *float32) {
	if x < *minX {
		*minX = x
	}
	if y < *minY {
		*minY = y
	}
	if x > *maxX {
		*maxX = x
	}
	if y > *maxY {
		*maxY = y
	}
}

// AffineTransform represents a 2D affine transformation matrix.
// The matrix is:
//
//	[A B Tx]
//	[C D Ty]
//	[0 0 1 ]
type AffineTransform struct {
	A, B, C, D float32 // Matrix coefficients
	Tx, Ty     float32 // Translation
}

// IdentityTransform returns the identity transformation.
func IdentityTransform() *AffineTransform {
	return &AffineTransform{A: 1, D: 1}
}

// ScaleTransform returns a scaling transformation.
func ScaleTransform(sx, sy float32) *AffineTransform {
	return &AffineTransform{A: sx, D: sy}
}

// TranslateTransform returns a translation transformation.
func TranslateTransform(tx, ty float32) *AffineTransform {
	return &AffineTransform{A: 1, D: 1, Tx: tx, Ty: ty}
}

// TransformPoint applies the transformation to a point.
func (m *AffineTransform) TransformPoint(x, y float32) (float32, float32) {
	return m.A*x + m.B*y + m.Tx, m.C*x + m.D*y + m.Ty
}

// Multiply returns the composition of two transformations.
func (m *AffineTransform) Multiply(other *AffineTransform) *AffineTransform {
	return &AffineTransform{
		A:  m.A*other.A + m.B*other.C,
		B:  m.A*other.B + m.B*other.D,
		C:  m.C*other.A + m.D*other.C,
		D:  m.C*other.B + m.D*other.D,
		Tx: m.A*other.Tx + m.B*other.Ty + m.Tx,
		Ty: m.C*other.Tx + m.D*other.Ty + m.Ty,
	}
}

// OutlineExtractor extracts glyph outlines from fonts.
// It uses a buffer pool internally for efficiency.
type OutlineExtractor struct {
	// buffer is reused for sfnt operations
	buffer sfnt.Buffer
}

// NewOutlineExtractor creates a new outline extractor.
func NewOutlineExtractor() *OutlineExtractor {
	return &OutlineExtractor{}
}

// ExtractOutline extracts the outline for a glyph at the given size.
// The size is in pixels (ppem - pixels per em).
// Returns nil if the glyph has no outline (e.g., space character).
func (e *OutlineExtractor) ExtractOutline(font ParsedFont, gid GlyphID, size float64) (*GlyphOutline, error) {
	// Type assert to get the underlying sfnt.Font
	xiFont, ok := font.(*ximageParsedFont)
	if !ok {
		return nil, ErrUnsupportedFontType
	}

	return e.extractFromSFNT(xiFont.font, gid, size)
}

// extractFromSFNT extracts outline from an sfnt.Font.
func (e *OutlineExtractor) extractFromSFNT(font *sfntFont, gid GlyphID, size float64) (*GlyphOutline, error) {
	ppem := fixed.Int26_6(size * 64) // Convert to 26.6 fixed point

	// Load glyph segments
	segments, err := font.LoadGlyph(&e.buffer, sfnt.GlyphIndex(gid), ppem, nil)
	if err != nil {
		// ErrNotFound means glyph doesn't exist
		// ErrColoredGlyph means it's a color glyph (COLR/sbix)
		return nil, err
	}

	// Check if glyph has no outline (like space)
	if len(segments) == 0 {
		// Still return an outline with advance info
		advance := getGlyphAdvance(font, &e.buffer, gid, size)
		return &GlyphOutline{
			Segments: nil,
			GID:      gid,
			Type:     GlyphTypeOutline,
			Advance:  float32(advance),
		}, nil
	}

	// Convert sfnt segments to our format
	outline := &GlyphOutline{
		Segments: make([]OutlineSegment, 0, len(segments)),
		GID:      gid,
		Type:     GlyphTypeOutline,
	}

	// Track bounds
	minX, minY := float64(1e10), float64(1e10)
	maxX, maxY := float64(-1e10), float64(-1e10)

	for _, seg := range segments {
		outSeg := OutlineSegment{}

		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			outSeg.Op = OutlineOpMoveTo
			outSeg.Points[0] = fixedPointToOutline(seg.Args[0])
			updateBounds(outSeg.Points[0], &minX, &minY, &maxX, &maxY)

		case sfnt.SegmentOpLineTo:
			outSeg.Op = OutlineOpLineTo
			outSeg.Points[0] = fixedPointToOutline(seg.Args[0])
			updateBounds(outSeg.Points[0], &minX, &minY, &maxX, &maxY)

		case sfnt.SegmentOpQuadTo:
			outSeg.Op = OutlineOpQuadTo
			outSeg.Points[0] = fixedPointToOutline(seg.Args[0]) // Control
			outSeg.Points[1] = fixedPointToOutline(seg.Args[1]) // Target
			updateBounds(outSeg.Points[0], &minX, &minY, &maxX, &maxY)
			updateBounds(outSeg.Points[1], &minX, &minY, &maxX, &maxY)

		case sfnt.SegmentOpCubeTo:
			outSeg.Op = OutlineOpCubicTo
			outSeg.Points[0] = fixedPointToOutline(seg.Args[0]) // Control 1
			outSeg.Points[1] = fixedPointToOutline(seg.Args[1]) // Control 2
			outSeg.Points[2] = fixedPointToOutline(seg.Args[2]) // Target
			updateBounds(outSeg.Points[0], &minX, &minY, &maxX, &maxY)
			updateBounds(outSeg.Points[1], &minX, &minY, &maxX, &maxY)
			updateBounds(outSeg.Points[2], &minX, &minY, &maxX, &maxY)
		}

		outline.Segments = append(outline.Segments, outSeg)
	}

	// Set bounds
	if len(outline.Segments) > 0 {
		outline.Bounds = Rect{
			MinX: minX,
			MinY: minY,
			MaxX: maxX,
			MaxY: maxY,
		}
	}

	// Get advance
	outline.Advance = float32(getGlyphAdvance(font, &e.buffer, gid, size))

	return outline, nil
}

// fixedPointToOutline converts a fixed.Point26_6 to OutlinePoint.
func fixedPointToOutline(p fixed.Point26_6) OutlinePoint {
	return OutlinePoint{
		X: float32(p.X) / 64.0,
		Y: float32(p.Y) / 64.0,
	}
}

// updateBounds updates the min/max bounds.
func updateBounds(p OutlinePoint, minX, minY, maxX, maxY *float64) {
	if float64(p.X) < *minX {
		*minX = float64(p.X)
	}
	if float64(p.Y) < *minY {
		*minY = float64(p.Y)
	}
	if float64(p.X) > *maxX {
		*maxX = float64(p.X)
	}
	if float64(p.Y) > *maxY {
		*maxY = float64(p.Y)
	}
}

// getGlyphAdvance returns the advance width for a glyph.
func getGlyphAdvance(font *sfntFont, buf *sfnt.Buffer, gid GlyphID, size float64) float64 {
	ppem := fixed.Int26_6(size * 64)
	advance, err := font.GlyphAdvance(buf, sfnt.GlyphIndex(gid), ppem, 0) // No hinting for outline extraction
	if err != nil {
		return 0
	}
	return float64(advance) / 64.0
}

// sfntFont is a type alias for easier access.
type sfntFont = sfnt.Font

// ErrUnsupportedFontType is returned when the font type is not supported.
var ErrUnsupportedFontType = &FontError{Reason: "unsupported font type for outline extraction"}

// FontError represents a font-related error.
type FontError struct {
	Reason string
}

func (e *FontError) Error() string {
	return "text: " + e.Reason
}
