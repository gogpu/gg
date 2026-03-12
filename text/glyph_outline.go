// Package text provides GPU text rendering infrastructure.
package text

import (
	"math"

	"golang.org/x/image/font"
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
func (e *OutlineExtractor) ExtractOutline(parsedFont ParsedFont, gid GlyphID, size float64) (*GlyphOutline, error) {
	return e.ExtractOutlineHinted(parsedFont, gid, size, HintingNone)
}

// ExtractOutlineHinted extracts the outline for a glyph at the given size
// with the specified hinting mode.
//
// When hinting is enabled:
//   - Advance widths are grid-fitted to integer pixel boundaries (via sfnt)
//   - Y-coordinates of horizontal segments are snapped to pixel grid
//     (crisp baselines, x-heights, cap-heights)
//   - HintingVertical snaps only Y-coordinates (horizontal stems)
//   - HintingFull snaps both X and Y coordinates
//
// Hinting should be disabled for rotated/scaled text where grid-fitting
// doesn't apply (the pixel grid is no longer axis-aligned).
func (e *OutlineExtractor) ExtractOutlineHinted(parsedFont ParsedFont, gid GlyphID, size float64, hinting Hinting) (*GlyphOutline, error) {
	xiFont, ok := parsedFont.(*ximageParsedFont)
	if !ok {
		return nil, ErrUnsupportedFontType
	}

	outline, err := e.extractFromSFNT(xiFont.font, gid, size, hinting)
	if err != nil {
		return nil, err
	}

	if outline == nil || hinting == HintingNone {
		return outline, nil
	}

	gridFitOutline(outline, hinting)
	return outline, nil
}

// extractFromSFNT extracts outline from an sfnt.Font.
func (e *OutlineExtractor) extractFromSFNT(f *sfntFont, gid GlyphID, size float64, hinting Hinting) (*GlyphOutline, error) {
	ppem := fixed.Int26_6(size * 64) // Convert to 26.6 fixed point

	// Load glyph segments
	segments, err := f.LoadGlyph(&e.buffer, sfnt.GlyphIndex(gid), ppem, nil)
	if err != nil {
		// ErrNotFound means glyph doesn't exist
		// ErrColoredGlyph means it's a color glyph (COLR/sbix)
		return nil, err
	}

	// Convert our Hinting to font.Hinting for advance width grid-fitting.
	fontHinting := toFontHinting(hinting)

	// Check if glyph has no outline (like space)
	if len(segments) == 0 {
		// Still return an outline with advance info
		advance := getGlyphAdvance(f, &e.buffer, gid, size, fontHinting)
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

	// Get advance with hinting
	outline.Advance = float32(getGlyphAdvance(f, &e.buffer, gid, size, fontHinting))

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
// When hinting is enabled, the advance is grid-fitted by sfnt to integer pixels.
func getGlyphAdvance(f *sfntFont, buf *sfnt.Buffer, gid GlyphID, size float64, h font.Hinting) float64 {
	ppem := fixed.Int26_6(size * 64)
	advance, err := f.GlyphAdvance(buf, sfnt.GlyphIndex(gid), ppem, h)
	if err != nil {
		return 0
	}
	return float64(advance) / 64.0
}

// toFontHinting converts our Hinting enum to golang.org/x/image/font.Hinting.
func toFontHinting(h Hinting) font.Hinting {
	switch h {
	case HintingVertical:
		return font.HintingVertical
	case HintingFull:
		return font.HintingFull
	default:
		return font.HintingNone
	}
}

// gridFitOutline applies grid-fitting to outline coordinates for crisp rendering
// at small pixel sizes. This is a lightweight auto-hinter inspired by FreeType's
// approach — it snaps key coordinates to pixel boundaries without executing
// TrueType bytecode instructions.
//
// Strategy per hinting mode:
//   - HintingVertical: snap Y-coordinates of near-horizontal segments to pixel grid.
//     This aligns baselines, x-heights, and cap-heights to pixels, which is the
//     single highest-impact hinting operation for body text.
//   - HintingFull: snap both X and Y coordinates of axis-aligned segments.
//     Additionally snaps vertical stems for consistent stem widths.
//
// The grid-fitting threshold (0.3px) allows tolerance for slightly off-axis
// segments that should still be snapped (e.g., a "horizontal" line at Y=3.02
// due to floating-point rounding in font scaling).
func gridFitOutline(outline *GlyphOutline, hinting Hinting) {
	if outline == nil || len(outline.Segments) == 0 {
		return
	}

	// Build snap map: detect Y-values to grid-fit and baseline snap points.
	ySnaps := buildYSnapMap(outline)

	// Apply snapping and update bounds.
	applyGridFit(outline, ySnaps, hinting)
}

// gridFitSnapThreshold is the max deviation from a pixel boundary for a coordinate
// to be considered "aligned" and eligible for snapping. 0.3px allows tolerance for
// slightly off-axis segments due to floating-point rounding in font scaling.
const gridFitSnapThreshold = 0.3

// buildYSnapMap detects Y-values that should be snapped to pixel boundaries.
// It finds near-horizontal segments (where consecutive endpoints have similar Y)
// and baseline-proximity points (Y near 0).
func buildYSnapMap(outline *GlyphOutline) map[float32]float32 {
	ySnaps := make(map[float32]float32)

	// Detect horizontal segments: consecutive endpoints with similar Y.
	for i := range len(outline.Segments) - 1 {
		seg := &outline.Segments[i]
		next := &outline.Segments[i+1]

		if next.Op == OutlineOpMoveTo {
			continue // new contour
		}

		if next.Op == OutlineOpLineTo {
			endY := segEndY(seg)
			nextY := next.Points[0].Y
			if abs32f(endY-nextY) < gridFitSnapThreshold {
				avgY := (endY + nextY) / 2
				snapped := float32(math.Round(float64(avgY)))
				ySnaps[endY] = snapped
				ySnaps[nextY] = snapped
			}
		}
	}

	// Baseline snap: Y near 0 → exactly 0 (highest-impact single snap point).
	for i := range outline.Segments {
		seg := &outline.Segments[i]
		for j := range segPointCount(seg.Op) {
			if abs32f(seg.Points[j].Y) < gridFitSnapThreshold {
				ySnaps[seg.Points[j].Y] = 0
			}
		}
	}

	return ySnaps
}

// applyGridFit applies the snap map to outline coordinates and refreshes bounds.
func applyGridFit(outline *GlyphOutline, ySnaps map[float32]float32, hinting Hinting) {
	snapY := hinting == HintingVertical || hinting == HintingFull
	snapX := hinting == HintingFull

	minX, minY := float64(1e10), float64(1e10)
	maxX, maxY := float64(-1e10), float64(-1e10)

	for i := range outline.Segments {
		seg := &outline.Segments[i]
		for j := range segPointCount(seg.Op) {
			if snapY {
				if snapped, ok := ySnaps[seg.Points[j].Y]; ok {
					seg.Points[j].Y = snapped
				}
			}
			if snapX && (seg.Op == OutlineOpMoveTo || seg.Op == OutlineOpLineTo) {
				frac := seg.Points[j].X - float32(math.Round(float64(seg.Points[j].X)))
				if abs32f(frac) < gridFitSnapThreshold {
					seg.Points[j].X = float32(math.Round(float64(seg.Points[j].X)))
				}
			}
			updateBounds(seg.Points[j], &minX, &minY, &maxX, &maxY)
		}
	}

	outline.Bounds = Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

// segEndY returns the Y coordinate of the last on-curve point of a segment.
func segEndY(seg *OutlineSegment) float32 {
	switch seg.Op {
	case OutlineOpMoveTo, OutlineOpLineTo:
		return seg.Points[0].Y
	case OutlineOpQuadTo:
		return seg.Points[1].Y
	case OutlineOpCubicTo:
		return seg.Points[2].Y
	}
	return 0
}

// segPointCount returns the number of points used by a segment op.
func segPointCount(op OutlineOp) int {
	switch op {
	case OutlineOpMoveTo, OutlineOpLineTo:
		return 1
	case OutlineOpQuadTo:
		return 2
	case OutlineOpCubicTo:
		return 3
	}
	return 0
}

// abs32f returns the absolute value of a float32.
func abs32f(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
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
