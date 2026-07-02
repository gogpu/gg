// Package text provides GPU text rendering infrastructure.
package text

import (
	"encoding/binary"
	"math"
	"sort"
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
type OutlineExtractor struct{}

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
//   - Y-coordinates of horizontal segments are snapped to pixel grid
//     (crisp baselines, x-heights, cap-heights)
//   - HintingVertical snaps only Y-coordinates (horizontal stems)
//   - HintingFull snaps both X and Y coordinates
//
// Hinting should be disabled for rotated/scaled text where grid-fitting
// doesn't apply (the pixel grid is no longer axis-aligned).
func (e *OutlineExtractor) ExtractOutlineHinted(parsedFont ParsedFont, gid GlyphID, size float64, hinting Hinting) (*GlyphOutline, error) {
	var outline *GlyphOutline
	var err error

	switch f := parsedFont.(type) {
	case *ownParsedFont:
		outline, err = e.extractFromOwn(f, gid, size)
	default:
		return nil, ErrUnsupportedFontType
	}
	if err != nil {
		return nil, err
	}

	if outline == nil || hinting == HintingNone {
		return outline, nil
	}

	// Priority 1: TT bytecode hinting (for fonts with fpgm/prep instructions).
	// This produces professionally hinted outlines matching the font designer's
	// intent. Fonts like Arial, Times New Roman, Segoe UI rely on TT instructions
	// for quality rendering at screen sizes.
	if ttOutline := tryTTBytecodeHintingGeneric(parsedFont, gid, size); ttOutline != nil && len(ttOutline.Segments) > 0 {
		return ttOutline, nil
	}

	// Priority 2: Auto-hinter (contour-based, Y-UP convention).
	// Falls back to simple grid-fitting if contour data is unavailable
	// (TTC fonts, CFF fonts). Composite glyphs are handled transparently
	// by ParseGlyfContours (recursive flattening). The legacy outline-based
	// auto-hinter is not used because sfnt outlines are Y-DOWN while the
	// hinting pipeline operates in Y-UP — a convention mismatch that
	// collapses all Y coordinates to the baseline.
	if !autoHintOutline(outline, parsedFont, size, hinting) {
		gridFitOutline(outline, hinting)
	}
	return outline, nil
}

// ExtractOutlineHintedVar extracts a glyph outline with font variations AND
// hinting applied in a single unified path. This matches skrifa's load_simple
// architecture where gvar deltas are applied to unscaled points BEFORE
// scaling and TT bytecode hinting.
//
// This fixes the variable font rendering bug where variable fonts skipped
// TT bytecode hinting and auto-hinting, causing them to render bolder
// than static fonts at the same weight.
//
// The hinting priority chain (same as static fonts):
//  1. TT bytecode hinting with gvar-varied unscaled points
//  2. Auto-hinter on the gvar-varied outline
//  3. Grid-fit fallback
//
// When variations is nil or empty, this produces identical output to
// ExtractOutlineHinted (delegates to the static path).
//
// Reference: skrifa glyf/mod.rs:584-782 (load_simple — one path for both)
func (e *OutlineExtractor) ExtractOutlineHintedVar(
	parsedFont ParsedFont,
	gid GlyphID,
	size float64,
	hinting Hinting,
	variations []FontVariation,
) (*GlyphOutline, error) {
	// If no variations, delegate to the static path.
	if len(variations) == 0 {
		return e.ExtractOutlineHinted(parsedFont, gid, size, hinting)
	}

	ownFont, ok := parsedFont.(*ownParsedFont)
	if !ok {
		return nil, ErrUnsupportedFontType
	}

	// Extract the gvar-varied outline AND the varied contour points.
	// Both are needed: the outline for the final result, and the contours
	// for the auto-hinter fallback (which must operate on gvar-varied points,
	// not re-read the original unvaried glyf data).
	outline, variedContours, err := e.extractFromOwnVariableWithContours(ownFont, gid, size, variations)
	if err != nil {
		return nil, err
	}

	if outline == nil || hinting == HintingNone {
		return outline, nil
	}

	// Priority 1: TT bytecode hinting with gvar-varied unscaled points.
	// This runs gvar deltas on unscaled points, then scales to 26.6,
	// then runs the TT interpreter — exactly matching skrifa load_simple.
	if ttOutline := tryTTBytecodeHintingVar(parsedFont, gid, size, variations); ttOutline != nil && len(ttOutline.Segments) > 0 {
		return ttOutline, nil
	}

	// Priority 2: Auto-hinter on the gvar-varied outline.
	// Pass the pre-varied contour points so the auto-hinter operates on
	// gvar-modified coordinates, not the original unvaried glyf data.
	// This fixes the bug where gvar deltas were computed correctly but
	// lost when the auto-hinter re-read contours from the raw font.
	if !autoHintOutlineVar(outline, variedContours, parsedFont, size, hinting) {
		gridFitOutline(outline, hinting)
	}
	return outline, nil
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

	// Stem-width preservation (FreeType pattern): detect snapped Y-values
	// that collapsed to the same pixel row and enforce minimum 1px separation.
	// Without this, thin horizontal features (T crossbar, E/F bars) at 10-16px
	// collapse to 0px height and become invisible.
	enforceMinStemWidth(ySnaps)

	return ySnaps
}

// enforceMinStemWidth detects pairs of original Y-coordinates that mapped to
// the same snapped value and pushes them apart to maintain at least 1px stem.
// This matches FreeType's af_latin_hints_compute_edges pattern.
func enforceMinStemWidth(ySnaps map[float32]float32) {
	if len(ySnaps) < 2 {
		return
	}

	type snapEntry struct {
		orig    float32
		snapped float32
	}
	entries := make([]snapEntry, 0, len(ySnaps))
	for orig, snapped := range ySnaps {
		entries = append(entries, snapEntry{orig, snapped})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].orig < entries[j].orig })

	for i := 0; i < len(entries)-1; i++ {
		curr := entries[i]
		next := entries[i+1]

		origGap := next.orig - curr.orig
		if origGap < 0.1 {
			continue // duplicate or near-duplicate points, not a stem
		}

		if curr.snapped == next.snapped && origGap > 0.3 {
			// Two distinct edges collapsed to same pixel row → stem invisible.
			// Push the lower (smaller Y = higher on screen in Y-down) edge up by 1px.
			newSnapped := next.snapped - 1.0
			ySnaps[curr.orig] = newSnapped
			entries[i] = snapEntry{curr.orig, newSnapped}
		}
	}
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

// extractFromOwn extracts outline from an ownParsedFont using raw glyf
// contour points. This is the Pure Go path that does not depend on
// sfnt.Font.LoadGlyph.
//
// The approach:
//  1. Parse raw contour points via ParseGlyfContours (Y-UP font units)
//  2. Scale to ppem and convert Y-UP → Y-DOWN (Go rendering convention)
//  3. Convert TrueType on/off-curve points to MoveTo/LineTo/QuadTo segments
//
// This matches the sfnt.LoadGlyph output format so the rest of the
// pipeline (hinting, rendering) works identically.
func (e *OutlineExtractor) extractFromOwn(f *ownParsedFont, gid GlyphID, size float64) (*GlyphOutline, error) {
	rawData := f.RawFontData()
	if rawData == nil {
		return nil, &FontError{Reason: "own parser: no raw font data"}
	}

	upem := f.UnitsPerEm()
	if upem == 0 {
		return nil, &FontError{Reason: "own parser: zero unitsPerEm"}
	}

	// Get advance width.
	advance := f.GlyphAdvance(uint16(gid), size)

	// Parse raw contour points from glyf table.
	contours, err := ParseGlyfContours(rawData, gid)
	if err != nil {
		return nil, err
	}
	if contours == nil || len(contours.Points) == 0 {
		// Empty glyph (space, etc.) — return outline with advance only.
		return &GlyphOutline{
			GID:     gid,
			Type:    GlyphTypeOutline,
			Advance: float32(advance),
		}, nil
	}

	// Scale factor: ppem / unitsPerEm.
	scale := size / float64(upem)

	// Convert raw contour points to outline segments.
	// sfnt.LoadGlyph returns coordinates in Y-DOWN at ppem scale.
	// Our raw contours are in Y-UP font units.
	// To match sfnt: scaleX = scale, scaleY = -scale (Y-UP → Y-DOWN).
	segments := contourPointsToSegments(contours, scale)

	if len(segments) == 0 {
		return &GlyphOutline{
			GID:     gid,
			Type:    GlyphTypeOutline,
			Advance: float32(advance),
		}, nil
	}

	// Compute bounds from segments.
	outline := &GlyphOutline{
		Segments: segments,
		GID:      gid,
		Type:     GlyphTypeOutline,
		Advance:  float32(advance),
	}

	minX, minY := float64(1e10), float64(1e10)
	maxX, maxY := float64(-1e10), float64(-1e10)
	for _, seg := range segments {
		for j := range segPointCount(seg.Op) {
			updateBounds(seg.Points[j], &minX, &minY, &maxX, &maxY)
		}
	}
	outline.Bounds = Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}

	return outline, nil
}

// extractFromOwnVariableWithContours extracts a glyph outline with font
// variations applied and also returns the gvar-varied GlyfContours.
// The contours are needed by the auto-hinter fallback, which must operate
// on gvar-modified coordinates rather than re-reading unvaried glyf data.
//
// Returns (outline, variedContours, err). variedContours is nil for empty
// glyphs or on error.
func (e *OutlineExtractor) extractFromOwnVariableWithContours(
	f *ownParsedFont,
	gid GlyphID,
	size float64,
	variations []FontVariation,
) (*GlyphOutline, *GlyfContours, error) {
	outline, contours, err := e.extractFromOwnVariableImpl(f, gid, size, variations)
	return outline, contours, err
}

// extractFromOwnVariable extracts a glyph outline with font variations
// applied (gvar/HVAR). This replaces the deleted ExtractOutlineGoText
// which used an external library for outline extraction.
//
// The approach:
//  1. Parse raw glyf contour points (Y-UP font units, with phantom points)
//  2. Apply gvar deltas via ownParsedFont.applyVariations
//  3. Convert modified points to OutlineSegments (Y-UP → Y-DOWN)
//  4. Compute variation-aware advance from HVAR
func (e *OutlineExtractor) extractFromOwnVariable(
	f *ownParsedFont,
	gid GlyphID,
	size float64,
	variations []FontVariation,
) (*GlyphOutline, error) {
	outline, _, err := e.extractFromOwnVariableImpl(f, gid, size, variations)
	return outline, err
}

// extractFromOwnVariableImpl is the shared implementation for
// extractFromOwnVariable and extractFromOwnVariableWithContours.
// Returns (outline, variedContours, err).
func (e *OutlineExtractor) extractFromOwnVariableImpl(
	f *ownParsedFont,
	gid GlyphID,
	size float64,
	variations []FontVariation,
) (*GlyphOutline, *GlyfContours, error) {
	upem := f.UnitsPerEm()
	if upem == 0 {
		return nil, nil, &FontError{Reason: "own parser: zero unitsPerEm"}
	}

	// Get variation-aware advance.
	var advance float64
	if vap, ok := ParsedFont(f).(VariableAdvanceProvider); ok {
		advance = vap.GlyphAdvanceVar(uint16(gid), size, variations)
	} else {
		advance = f.GlyphAdvance(uint16(gid), size)
	}

	// Parse raw contour points with phantom points for gvar.
	glyfData, ok := f.tables["glyf"]
	if !ok {
		return nil, nil, &FontError{Reason: "own parser: missing glyf table"}
	}
	locaData, ok := f.tables["loca"]
	if !ok {
		return nil, nil, &FontError{Reason: "own parser: missing loca table"}
	}
	headData, ok := f.tables["head"]
	if !ok || len(headData) < 54 {
		return nil, nil, &FontError{Reason: "own parser: missing head table"}
	}
	isLongLoca := binary.BigEndian.Uint16(headData[50:52]) != 0

	contours, err := extractGlyfContourOwn(glyfData, locaData, int(gid), isLongLoca)
	if err != nil {
		return nil, nil, err
	}
	if contours == nil || len(contours.Points) == 0 {
		return &GlyphOutline{
			GID:     gid,
			Type:    GlyphTypeOutline,
			Advance: float32(advance),
		}, nil, nil
	}

	// Build points array for applyVariations: [x, y] pairs + 4 phantom points.
	nPts := len(contours.Points)
	points := make([][2]int32, nPts+4)
	for i, pt := range contours.Points {
		points[i] = [2]int32{int32(pt.X), int32(pt.Y)}
	}

	// Compute hmtx advance for phantom points.
	var hmtxAdv int32
	f.ensureHmtx()
	if f.hmtxParsed && f.hmtxAdv != nil {
		hmtxAdv = int32(hmtxAdvance(f.hmtxAdv, f.numHMetrics, uint16(gid)))
	}

	// Phantom points: [nPts+0]=origin, [nPts+1]=advance, [nPts+2/3]=vertical.
	points[nPts] = [2]int32{0, 0}
	points[nPts+1] = [2]int32{hmtxAdv, 0}
	points[nPts+2] = [2]int32{0, 0}
	points[nPts+3] = [2]int32{0, 0}

	// Apply gvar deltas.
	f.applyVariations(uint16(gid), points, contours.EndPts, variations)

	// Write modified points back to contours.
	for i := range contours.Points {
		contours.Points[i].X = int16(points[i][0])
		contours.Points[i].Y = int16(points[i][1])
	}

	// Scale and convert to segments.
	scale := size / float64(upem)
	segments := contourPointsToSegments(contours, scale)
	if len(segments) == 0 {
		return &GlyphOutline{
			GID:     gid,
			Type:    GlyphTypeOutline,
			Advance: float32(advance),
		}, contours, nil
	}

	outline := &GlyphOutline{
		Segments: segments,
		GID:      gid,
		Type:     GlyphTypeOutline,
		Advance:  float32(advance),
	}

	minX, minY := float64(1e10), float64(1e10)
	maxX, maxY := float64(-1e10), float64(-1e10)
	for _, seg := range segments {
		for j := range segPointCount(seg.Op) {
			updateBounds(seg.Points[j], &minX, &minY, &maxX, &maxY)
		}
	}
	outline.Bounds = Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}

	return outline, contours, nil
}

// contourPointsToSegments converts raw TrueType glyf contour points to
// OutlineSegment slices. Coordinates are scaled from font units to ppem
// and Y is negated (Y-UP → Y-DOWN) to match sfnt.LoadGlyph output.
//
// TrueType contour point representation:
//   - On-curve points are line/curve endpoints
//   - Off-curve points are quadratic Bezier control points
//   - Two consecutive off-curve points imply an on-curve midpoint
func contourPointsToSegments(contours *GlyfContours, scale float64) []OutlineSegment {
	var segments []OutlineSegment

	for ci := range contours.NumContours() {
		pts := contours.ContourPoints(ci)
		if len(pts) < 2 {
			continue
		}
		n := len(pts)

		// Find the first on-curve point.
		firstOnCurve := -1
		for i := range n {
			if pts[i].OnCurve {
				firstOnCurve = i
				break
			}
		}

		// Compute start point.
		var startX, startY float32
		startIdx := 0
		if firstOnCurve >= 0 {
			startX = float32(float64(pts[firstOnCurve].X) * scale)
			startY = float32(-float64(pts[firstOnCurve].Y) * scale) // Y-UP → Y-DOWN
			startIdx = firstOnCurve
		} else {
			// All off-curve: start from midpoint of first and last.
			x0 := float64(pts[0].X) * scale
			y0 := -float64(pts[0].Y) * scale
			x1 := float64(pts[n-1].X) * scale
			y1 := -float64(pts[n-1].Y) * scale
			startX = float32((x0 + x1) / 2)
			startY = float32((y0 + y1) / 2)
		}

		segments = append(segments, OutlineSegment{
			Op:     OutlineOpMoveTo,
			Points: [3]OutlinePoint{{X: startX, Y: startY}},
		})

		// Walk points starting from after the first on-curve point.
		for count := 0; count < n; count++ {
			i := (startIdx + 1 + count) % n
			px := float32(float64(pts[i].X) * scale)
			py := float32(-float64(pts[i].Y) * scale) // Y-UP → Y-DOWN

			if pts[i].OnCurve {
				segments = append(segments, OutlineSegment{
					Op:     OutlineOpLineTo,
					Points: [3]OutlinePoint{{X: px, Y: py}},
				})
			} else {
				// Off-curve: quadratic Bezier control point.
				next := (i + 1) % n
				var endX, endY float32
				if pts[next].OnCurve {
					endX = float32(float64(pts[next].X) * scale)
					endY = float32(-float64(pts[next].Y) * scale)
					count++ // Skip the next point since we consumed it.
				} else {
					// Two consecutive off-curve: implicit on-curve at midpoint.
					nx := float64(pts[next].X) * scale
					ny := -float64(pts[next].Y) * scale
					endX = float32((float64(px) + nx) / 2)
					endY = float32((float64(py) + ny) / 2)
				}
				segments = append(segments, OutlineSegment{
					Op: OutlineOpQuadTo,
					Points: [3]OutlinePoint{
						{X: px, Y: py},     // control
						{X: endX, Y: endY}, // endpoint
					},
				})
			}
		}
	}

	return segments
}

// tryTTBytecodeHintingGeneric attempts TT bytecode hinting for an ownParsedFont.
//
// The TT bytecode hinting path requires:
//  1. Raw font data (for loading fpgm/prep programs and glyph bytecode)
//  2. A ttHintCache (lazily initialized from the raw data)
func tryTTBytecodeHintingGeneric(parsedFont ParsedFont, gid GlyphID, size float64) *GlyphOutline {
	// Get the TT hint cache based on font type.
	var cache *ttHintCache
	if f, ok := parsedFont.(*ownParsedFont); ok {
		cache = f.loadTTHintCache()
	} else {
		return nil
	}
	if cache == nil {
		return nil
	}

	ppem := int32(size)
	if ppem <= 0 {
		return nil
	}

	hinted, err := cache.hintGlyphOutline(uint16(gid), ppem)
	if err != nil || hinted == nil {
		return nil
	}

	return ttHintedOutlineToGlyphOutline(hinted, gid)
}

// ErrUnsupportedFontType is returned when the font type is not supported.
var ErrUnsupportedFontType = &FontError{Reason: "unsupported font type for outline extraction"}

// FontError represents a font-related error.
type FontError struct {
	Reason string
}

func (e *FontError) Error() string {
	return "text: " + e.Reason
}
