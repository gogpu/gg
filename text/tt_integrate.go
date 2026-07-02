// TrueType bytecode interpreter — integration with glyph rendering pipeline.
//
// This file wires the TT bytecode interpreter into the existing glyph
// outline extraction and advance width computation. When a TrueType font
// has fpgm/prep bytecode instructions, this path produces professionally
// hinted glyph outlines and correct hinted advance widths.
//
// Priority chain for hinting:
//  1. TT bytecode (this file) — if font has fpgm/prep instructions
//  2. Auto-hinter — if TT bytecode fails or is absent
//  3. Grid-fit — basic Y-snapping fallback
//
// Priority chain for advance width:
//  1. TT bytecode phantom points — hinted advance from interpreter
//  2. HVAR — if font has HVAR table (variable fonts)
//  3. Raw hmtx — unhinted advance from horizontal metrics table
//
// Reference: skrifa glyf/mod.rs (FreeTypeScaler lifecycle)
// Reference: skrifa hint/instance.rs (HintInstance::hint)
package text

import (
	"sync"
)

// ttHintCache provides cached TT hint instances per font+size combination.
// Thread-safe via sync.Map for concurrent access from multiple goroutines.
//
// Cache lifecycle:
//   - Key: ppem (int32) — one instance per font size
//   - Value: *ttHintCacheEntry — contains hint instance + glyph loader
//   - Eviction: none (ppem cardinality is small, typically 5-20 sizes)
//
// Each font gets its own ttHintCache (stored in ximageParsedFont).
type ttHintCache struct {
	font    *ttFontProgram
	loader  *ttGlyphLoader
	entries sync.Map // key: int32 (ppem) → value: *ttHintCacheEntry
}

// ttHintCacheEntry holds the cached hint instance for a specific ppem.
type ttHintCacheEntry struct {
	instance *ttHintInstance
	err      error // non-nil if instance creation failed
}

// newTTHintCache creates a hint cache for the given font data.
// Returns nil if the font has no TrueType instructions.
func newTTHintCache(fontData []byte) *ttHintCache {
	font, err := loadTTFontProgram(fontData)
	if err != nil || font == nil {
		return nil
	}

	loader, err := newTTGlyphLoader(fontData, font)
	if err != nil || loader == nil {
		return nil
	}

	return &ttHintCache{
		font:   font,
		loader: loader,
	}
}

// getInstance returns the hint instance for the given ppem, creating it
// if needed. The instance is cached for reuse across glyphs at the same size.
func (c *ttHintCache) getInstance(ppem int32) (*ttHintInstance, error) {
	if ppem <= 0 {
		return nil, nil //nolint:nilnil // invalid ppem
	}

	// Check cache first.
	if v, ok := c.entries.Load(ppem); ok {
		entry := v.(*ttHintCacheEntry)
		return entry.instance, entry.err
	}

	// Create new instance (fpgm + prep execution).
	instance, err := newTTHintInstance(c.font, ppem, ttTargetSmooth)
	entry := &ttHintCacheEntry{instance: instance, err: err}

	// Store in cache (LoadOrStore handles races).
	actual, _ := c.entries.LoadOrStore(ppem, entry)
	stored := actual.(*ttHintCacheEntry)
	return stored.instance, stored.err
}

// hintGlyphOutline loads, hints, and returns the glyph outline with hinted
// points and phantom-point advance. This is the main entry point for TT
// bytecode hinting of a single glyph.
//
// Both simple and composite glyphs are supported. Composite glyphs are
// recursively loaded and merged by loadCompositeGlyphOutline before hinting.
//
// For empty glyphs (space, etc.), returns a phantom-only outline with rounded
// phantom points but no contour points. The advance from such an outline is
// integer-pixel, matching FreeType/skrifa behavior.
//
// Returns nil, nil for glyphs that cannot be hinted (e.g., hinting disabled).
//
//nolint:nilnil // nil result = "no hintable outline"
func (c *ttHintCache) hintGlyphOutline(glyphID uint16, ppem int32) (*ttGlyphOutline, error) {
	instance, err := c.getInstance(ppem)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, nil
	}

	// Check if hinting was disabled by the prep program.
	if !instance.isEnabled() {
		return nil, nil
	}

	// Compute scale: ppem * 64 / upem in 16.16 fixed-point.
	scale := instance.scale

	// Load glyph outline with phantom points.
	outline, err := c.loader.loadGlyphOutline(glyphID, scale)
	if err != nil {
		return nil, err
	}
	if outline == nil {
		return nil, nil
	}

	// Empty glyphs (space, etc.) have phantom-only outlines with pre-rounded
	// phantom points. No bytecode to run — just return the outline.
	// Reference: FreeType ttgload.c:1555-1608 — does NOT call TT_Hint_Glyph
	// for empty glyphs. skrifa load_empty does NOT call hinter.hint().
	if len(outline.contours) == 0 && len(outline.bytecode) == 0 {
		return outline, nil
	}

	// Run the bytecode interpreter.
	if err := instance.hintGlyph(outline); err != nil {
		// Non-pedantic: return unhinted outline on error.
		return outline, nil //nolint:nilerr // intentional: use unhinted outline on hinting failure
	}

	return outline, nil
}

// hintedAdvanceWidth returns the hinted advance width for a glyph in pixels.
// This runs the full TT interpreter to get phantom point positions, then
// computes the advance from phantom[1].x - phantom[0].x.
//
// Returns 0, false if TT hinting is unavailable or the glyph cannot be hinted.
func (c *ttHintCache) hintedAdvanceWidth(glyphID uint16, ppem int32) (float64, bool) {
	outline, err := c.hintGlyphOutline(glyphID, ppem)
	if err != nil || outline == nil {
		return 0, false
	}

	// Convert 26.6 advance to pixels.
	advance26dot6 := outline.hintedAdvance()
	return float64(advance26dot6) / 64.0, true
}

// hintGlyphOutlineVar loads, applies gvar deltas, hints, and returns the
// glyph outline for variable fonts. This is the variable-font counterpart
// of hintGlyphOutline — it applies gvar deltas to unscaled points BEFORE
// scaling and hinting, matching skrifa load_simple (lines 647-773).
//
// Returns nil, nil for glyphs that cannot be hinted.
//
//nolint:nilnil // nil result = "no hintable outline"
func (c *ttHintCache) hintGlyphOutlineVar(
	glyphID uint16,
	ppem int32,
	font *ownParsedFont,
	variations []FontVariation,
) (*ttGlyphOutline, error) {
	instance, err := c.getInstance(ppem)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, nil
	}

	// Check if hinting was disabled by the prep program.
	if !instance.isEnabled() {
		return nil, nil
	}

	scale := instance.scale

	// Load glyph outline with gvar deltas applied to unscaled points.
	outline, err := c.loader.loadGlyphOutlineVar(glyphID, scale, font, variations)
	if err != nil {
		return nil, err
	}
	if outline == nil {
		return nil, nil
	}

	// Empty glyphs (space, etc.) have phantom-only outlines with pre-rounded
	// phantom points. No bytecode to run — just return the outline.
	if len(outline.contours) == 0 && len(outline.bytecode) == 0 {
		return outline, nil
	}

	// Run the bytecode interpreter on the varied+scaled points.
	if err := instance.hintGlyph(outline); err != nil {
		// Non-pedantic: return unhinted outline on error.
		return outline, nil //nolint:nilerr // intentional: use unhinted outline on hinting failure
	}

	return outline, nil
}

// tryTTBytecodeHintingVar attempts TT bytecode hinting for a variable font
// glyph. This applies gvar deltas to unscaled points before scaling and
// hinting — matching skrifa's unified load_simple path.
//
// This is used by ExtractOutlineHintedVar to provide the same hinting
// quality for variable fonts as for static fonts.
func tryTTBytecodeHintingVar(
	parsedFont ParsedFont,
	gid GlyphID,
	size float64,
	variations []FontVariation,
) *GlyphOutline {
	ownFont, ok := parsedFont.(*ownParsedFont)
	if !ok {
		return nil
	}

	cache := ownFont.loadTTHintCache()
	if cache == nil {
		return nil
	}

	ppem := int32(size)
	if ppem <= 0 {
		return nil
	}

	hinted, err := cache.hintGlyphOutlineVar(uint16(gid), ppem, ownFont, variations)
	if err != nil || hinted == nil {
		return nil
	}

	return ttHintedOutlineToGlyphOutline(hinted, gid)
}

// ttHintedOutlineToGlyphOutline converts a TT-hinted outline to the public
// GlyphOutline format used by the rendering pipeline. The hinted points
// replace the sfnt-loaded outline for professional quality rendering.
//
// For phantom-only outlines (empty glyphs like space), returns a GlyphOutline
// with the hinted advance but no segments. This is intentional — space has no
// visible outline but needs an accurate advance for text layout.
//
// All coordinates are converted from 26.6 fixed-point to float32 pixels.
func ttHintedOutlineToGlyphOutline(hinted *ttGlyphOutline, gid GlyphID) *GlyphOutline {
	if hinted == nil {
		return nil
	}

	// Count actual outline points (excluding phantom points).
	numPoints := len(hinted.points) - ttPhantomPointCount
	if numPoints <= 0 || len(hinted.contours) == 0 {
		// Phantom-only outline (empty glyph like space): return advance-only
		// GlyphOutline. The advance is hinted (integer-pixel) from phantom points.
		if len(hinted.points) >= ttPhantomPointCount {
			return &GlyphOutline{
				GID:     gid,
				Type:    GlyphTypeOutline,
				Advance: float32(hinted.hintedAdvance()) / 64.0,
			}
		}
		return nil
	}

	outline := &GlyphOutline{
		GID:  gid,
		Type: GlyphTypeOutline,
	}

	// Convert hinted advance from 26.6.
	outline.Advance = float32(hinted.hintedAdvance()) / 64.0

	// Note: skrifa does NOT translate outline by -phantom[0].x.
	// Instead it returns adjusted_lsb as separate metadata, and the
	// caller (rasterizer) uses it. Our rendering pipeline handles this
	// via the advance + bearing from the outline segments themselves.

	// Build segments from hinted contour points.
	// TrueType glyphs use on-curve/off-curve point representation.
	// We convert to MoveTo/LineTo/QuadTo segments.
	segments := make([]OutlineSegment, 0, numPoints*2)

	contourStart := 0
	for _, endIdx := range hinted.contours {
		end := int(endIdx) + 1
		if end > numPoints {
			end = numPoints
		}
		contourPts := hinted.points[contourStart:end]
		contourFlags := hinted.flags[contourStart:end]
		n := len(contourPts)
		if n < 2 {
			contourStart = end
			continue
		}

		segs := convertContourToSegments(contourPts, contourFlags, n)
		segments = append(segments, segs...)
		contourStart = end
	}

	outline.Segments = segments

	// Compute bounds from segments.
	if len(segments) > 0 {
		minX, minY := float64(1e10), float64(1e10)
		maxX, maxY := float64(-1e10), float64(-1e10)
		for _, seg := range segments {
			for j := range segPointCount(seg.Op) {
				updateBounds(seg.Points[j], &minX, &minY, &maxX, &maxY)
			}
		}
		outline.Bounds = Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
	}

	return outline
}

// convertContourToSegments converts TrueType contour points with on/off-curve
// flags into OutlineSegment slices (MoveTo/LineTo/QuadTo).
//
// TrueType representation:
//   - On-curve points are endpoints
//   - Off-curve points are quadratic Bezier control points
//   - Two consecutive off-curve points imply an intermediate on-curve point
//     at their midpoint
func convertContourToSegments(points [][2]int32, flags []ttPointFlags, n int) []OutlineSegment {
	if n < 2 {
		return nil
	}

	var segments []OutlineSegment

	// Find the first on-curve point to start.
	firstOnCurve := -1
	for i := range n {
		if flags[i]&ttPointFlagOnCurve != 0 {
			firstOnCurve = i
			break
		}
	}

	// If no on-curve point, compute the implicit first point from the
	// midpoint of the first two off-curve points.
	var startX, startY float32
	startIdx := 0
	if firstOnCurve >= 0 {
		startX = f26dot6ToPixelsX(points[firstOnCurve][0])
		startY = f26dot6ToPixelsY(points[firstOnCurve][1])
		startIdx = firstOnCurve
	} else {
		// All off-curve: start from midpoint of first and last.
		x0 := f26dot6ToPixelsX(points[0][0])
		y0 := f26dot6ToPixelsY(points[0][1])
		x1 := f26dot6ToPixelsX(points[n-1][0])
		y1 := f26dot6ToPixelsY(points[n-1][1])
		startX = (x0 + x1) / 2
		startY = (y0 + y1) / 2
	}

	segments = append(segments, OutlineSegment{
		Op:     OutlineOpMoveTo,
		Points: [3]OutlinePoint{{X: startX, Y: startY}},
	})

	// Walk points starting from after the first on-curve point.
	for count := 0; count < n; count++ {
		i := (startIdx + 1 + count) % n
		px := f26dot6ToPixelsX(points[i][0])
		py := f26dot6ToPixelsY(points[i][1])

		if flags[i]&ttPointFlagOnCurve != 0 {
			// On-curve: line to.
			segments = append(segments, OutlineSegment{
				Op:     OutlineOpLineTo,
				Points: [3]OutlinePoint{{X: px, Y: py}},
			})
		} else {
			// Off-curve: quadratic Bezier.
			// Look ahead for the endpoint.
			next := (i + 1) % n
			var endX, endY float32
			if flags[next]&ttPointFlagOnCurve != 0 {
				endX = f26dot6ToPixelsX(points[next][0])
				endY = f26dot6ToPixelsY(points[next][1])
				// Skip the next point since we consumed it.
				count++
			} else {
				// Two consecutive off-curve: implicit on-curve at midpoint.
				nx := f26dot6ToPixelsX(points[next][0])
				ny := f26dot6ToPixelsY(points[next][1])
				endX = (px + nx) / 2
				endY = (py + ny) / 2
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

	return segments
}

// f26dot6ToPixelsX converts a 26.6 fixed-point X value to float32 pixels.
// X is NOT negated (same direction in TrueType and rendering).
func f26dot6ToPixelsX(v int32) float32 {
	return float32(v) / 64.0
}

// f26dot6ToPixelsY converts a 26.6 fixed-point Y value from Y-UP (TrueType
// native) to Y-DOWN (Go rendering convention) by negating.
// Matches sfnt.LoadGlyph (a[j].Y = -scale(...)) and auto-hinter contourPtY.
func f26dot6ToPixelsY(v int32) float32 {
	return -float32(v) / 64.0
}
