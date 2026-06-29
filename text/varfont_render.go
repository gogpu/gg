package text

import (
	"bytes"
	"math"
	"sync"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// goTextFontCache caches parsed go-text *font.Font objects by FontSource.
// font.Font is read-only and safe for concurrent use.
var (
	goTextFontCacheMu sync.RWMutex
	goTextFontCache   = make(map[*FontSource]*font.Font)
)

// GetGoTextFont returns a cached go-text font.Font for the given FontSource.
func GetGoTextFont(source *FontSource) (*font.Font, error) {
	goTextFontCacheMu.RLock()
	if f, ok := goTextFontCache[source]; ok {
		goTextFontCacheMu.RUnlock()
		return f, nil
	}
	goTextFontCacheMu.RUnlock()

	goTextFontCacheMu.Lock()
	defer goTextFontCacheMu.Unlock()

	if f, ok := goTextFontCache[source]; ok {
		return f, nil
	}

	reader := bytes.NewReader(source.data)
	goTextFace, err := font.ParseTTF(reader)
	if err != nil {
		return nil, err
	}

	goTextFontCache[source] = goTextFace.Font
	return goTextFace.Font, nil
}

// ExtractOutlineGoText extracts a glyph outline using go-text with variations applied.
// Coordinates are returned in pixel units (scaled by ppem/upem), matching sfnt output.
//
// When hinting is not HintingNone, gridFitOutline is applied after scaling
// (Skia/FreeType pattern: hinting AFTER variation interpolation). This snaps
// horizontal stems to pixel boundaries for crisp rendering at small sizes.
func ExtractOutlineGoText(
	goTextFont *font.Font,
	gid GlyphID,
	ppem float64,
	variations []FontVariation,
	hinting ...Hinting,
) *GlyphOutline {
	face := font.NewFace(goTextFont)
	if len(variations) > 0 {
		face.SetVariations(convertVariationsForFont(variations))
	}

	glyphData := face.GlyphData(font.GID(gid))
	gtOutline, ok := glyphData.(font.GlyphOutline)
	if !ok || len(gtOutline.Segments) == 0 {
		advance := float64(face.HorizontalAdvance(font.GID(gid)))
		upem := float64(goTextFont.Upem())
		scale := ppem / upem
		return &GlyphOutline{
			GID:     gid,
			Type:    GlyphTypeOutline,
			Advance: float32(advance * scale),
		}
	}

	upem := float64(goTextFont.Upem())
	scale := ppem / upem

	outline := &GlyphOutline{
		Segments: make([]OutlineSegment, 0, len(gtOutline.Segments)),
		GID:      gid,
		Type:     GlyphTypeOutline,
	}

	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	for _, seg := range gtOutline.Segments {
		outSeg := OutlineSegment{}

		switch seg.Op {
		case ot.SegmentOpMoveTo:
			outSeg.Op = OutlineOpMoveTo
			outSeg.Points[0] = scaleGoTextPoint(seg.Args[0], scale)
			updateBoundsF(outSeg.Points[0], &minX, &minY, &maxX, &maxY)

		case ot.SegmentOpLineTo:
			outSeg.Op = OutlineOpLineTo
			outSeg.Points[0] = scaleGoTextPoint(seg.Args[0], scale)
			updateBoundsF(outSeg.Points[0], &minX, &minY, &maxX, &maxY)

		case ot.SegmentOpQuadTo:
			outSeg.Op = OutlineOpQuadTo
			outSeg.Points[0] = scaleGoTextPoint(seg.Args[0], scale)
			outSeg.Points[1] = scaleGoTextPoint(seg.Args[1], scale)
			updateBoundsF(outSeg.Points[0], &minX, &minY, &maxX, &maxY)
			updateBoundsF(outSeg.Points[1], &minX, &minY, &maxX, &maxY)

		case ot.SegmentOpCubeTo:
			outSeg.Op = OutlineOpCubicTo
			outSeg.Points[0] = scaleGoTextPoint(seg.Args[0], scale)
			outSeg.Points[1] = scaleGoTextPoint(seg.Args[1], scale)
			outSeg.Points[2] = scaleGoTextPoint(seg.Args[2], scale)
			updateBoundsF(outSeg.Points[0], &minX, &minY, &maxX, &maxY)
			updateBoundsF(outSeg.Points[1], &minX, &minY, &maxX, &maxY)
			updateBoundsF(outSeg.Points[2], &minX, &minY, &maxX, &maxY)
		}

		outline.Segments = append(outline.Segments, outSeg)
	}

	if len(outline.Segments) > 0 {
		outline.Bounds = Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
	}

	advance := float64(face.HorizontalAdvance(font.GID(gid)))
	outline.Advance = float32(advance * scale)

	// Apply grid-fitting after scaling (Skia/FreeType pattern: hint AFTER variation
	// interpolation). go-text returns unhinted outlines; our auto-hinter snaps
	// horizontal stems to pixel boundaries for crisp rendering at small sizes.
	if len(hinting) > 0 && hinting[0] != HintingNone {
		gridFitOutline(outline, hinting[0])
	}

	return outline
}

// goTextGlyphAdvance returns the variation-aware advance for a glyph in pixel units.
func goTextGlyphAdvance(
	goTextFont *font.Font,
	gid GlyphID,
	ppem float64,
	variations []FontVariation,
) float64 {
	face := font.NewFace(goTextFont)
	if len(variations) > 0 {
		face.SetVariations(convertVariationsForFont(variations))
	}
	advance := float64(face.HorizontalAdvance(font.GID(gid)))
	upem := float64(goTextFont.Upem())
	return advance * ppem / upem
}

func scaleGoTextPoint(p ot.SegmentPoint, scale float64) OutlinePoint {
	// go-text uses Y-up (font convention), our pipeline uses Y-down (screen).
	return OutlinePoint{
		X: float32(float64(p.X) * scale),
		Y: float32(float64(-p.Y) * scale),
	}
}

func updateBoundsF(p OutlinePoint, minX, minY, maxX, maxY *float64) {
	px, py := float64(p.X), float64(p.Y)
	if px < *minX {
		*minX = px
	}
	if py < *minY {
		*minY = py
	}
	if px > *maxX {
		*maxX = px
	}
	if py > *maxY {
		*maxY = py
	}
}

func convertVariationsForFont(variations []FontVariation) []font.Variation {
	if len(variations) == 0 {
		return nil
	}
	out := make([]font.Variation, len(variations))
	for i, v := range variations {
		out[i] = font.Variation{
			Tag:   ot.NewTag(v.Tag[0], v.Tag[1], v.Tag[2], v.Tag[3]),
			Value: v.Value,
		}
	}
	return out
}
