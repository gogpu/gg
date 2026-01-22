package text

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"

	"github.com/gogpu/gg/text/emoji"
)

// DrawWithEmoji renders text with full color emoji support.
// This function detects glyph types and routes to appropriate renderers:
//   - Outline glyphs: Standard font.Drawer rendering
//   - Bitmap glyphs: PNG bitmap scaling and compositing
//   - COLR glyphs: Layer-based color glyph rendering (future)
//
// Position (x, y) is the baseline origin.
// For fonts without color tables, this behaves identically to Draw().
func DrawWithEmoji(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	if text == "" || face == nil {
		return
	}

	// Check if the font has color tables.
	parsed := face.Source().Parsed()
	colorFont, hasColor := parsed.(ColorFont)

	if !hasColor || !colorFont.HasColorTables() {
		// No color support, fall back to standard rendering.
		Draw(dst, text, face, x, y, col)
		return
	}

	// Color-aware rendering.
	drawWithColorSupport(dst, text, face, x, y, col, colorFont)
}

// drawWithColorSupport renders text with glyph type detection.
func drawWithColorSupport(dst draw.Image, text string, face Face, x, y float64, col color.Color, colorFont ColorFont) {
	currentX := x
	ppem := uint16(face.Size())
	parsed := face.Source().Parsed()

	for _, r := range text {
		glyphID := parsed.GlyphIndex(r)
		advance := parsed.GlyphAdvance(glyphID, face.Size())
		glyphType := colorFont.GlyphType(glyphID)

		switch glyphType {
		case GlyphTypeBitmap:
			drawBitmapGlyph(dst, colorFont, glyphID, ppem, currentX, y)

		case GlyphTypeCOLR:
			// TODO: Implement COLR rendering in Phase 2.
			// For now, fall back to outline.
			drawSingleOutlineGlyph(dst, text, face, glyphID, currentX, y, col)

		default:
			// GlyphTypeOutline, GlyphTypeSVG (not yet supported), and unknown types.
			drawSingleOutlineGlyph(dst, text, face, glyphID, currentX, y, col)
		}

		currentX += advance
	}
}

// drawBitmapGlyph renders a bitmap glyph (CBDT or sbix).
func drawBitmapGlyph(dst draw.Image, colorFont ColorFont, glyphID uint16, ppem uint16, x, y float64) {
	bitmap, err := colorFont.BitmapGlyph(glyphID, ppem)
	if err != nil {
		// Silently skip - could render tofu glyph instead.
		return
	}

	// Decode PNG data.
	img, err := bitmap.Decode()
	if err != nil {
		return
	}

	// Calculate scaling factor.
	scale := float64(ppem) / float64(bitmap.PPEM)
	if scale == 0 {
		scale = 1.0
	}

	// Calculate scaled dimensions.
	scaledW := int(float64(bitmap.Width) * scale)
	scaledH := int(float64(bitmap.Height) * scale)

	if scaledW <= 0 || scaledH <= 0 {
		return
	}

	// Calculate destination bounds.
	// OriginX is horizontal bearing (offset from glyph origin).
	// OriginY is vertical bearing (distance from baseline to top of glyph).
	destX := int(x + float64(bitmap.OriginX)*scale)
	destY := int(y - float64(bitmap.OriginY)*scale)

	destRect := image.Rect(
		destX,
		destY,
		destX+scaledW,
		destY+scaledH,
	)

	// Draw with high-quality scaling.
	xdraw.CatmullRom.Scale(dst, destRect, img, img.Bounds(), xdraw.Over, nil)
}

// drawSingleOutlineGlyph renders a single outline glyph.
// This is a placeholder - ideally we'd render just the glyph without creating a full drawer.
func drawSingleOutlineGlyph(dst draw.Image, text string, face Face, glyphID uint16, x, y float64, col color.Color) {
	// For now, delegate to the standard Draw function with a single-rune string.
	// This is inefficient but correct. A better implementation would:
	// 1. Get the glyph outline path
	// 2. Rasterize directly to the destination
	//
	// TODO: Optimize in Phase 3 with direct glyph rendering.

	// Find the rune for this glyph ID.
	parsed := face.Source().Parsed()
	for _, r := range text {
		if parsed.GlyphIndex(r) == glyphID {
			Draw(dst, string(r), face, x, y, col)
			return
		}
	}
}

// BitmapGlyphCache caches decoded bitmap glyphs to avoid repeated PNG decoding.
// This is important for performance when rendering the same emoji multiple times.
type BitmapGlyphCache struct {
	entries map[bitmapCacheKey]*CachedBitmap
	maxSize int
}

// bitmapCacheKey uniquely identifies a cached bitmap.
type bitmapCacheKey struct {
	fontID  uintptr // Pointer to font for identity.
	glyphID uint16
	ppem    uint16
}

// CachedBitmap holds a decoded and optionally scaled bitmap.
type CachedBitmap struct {
	Img     image.Image
	OriginX float32
	OriginY float32
	Width   int
	Height  int
}

// NewBitmapGlyphCache creates a new bitmap glyph cache.
// maxSize is the maximum number of cached entries.
func NewBitmapGlyphCache(maxSize int) *BitmapGlyphCache {
	return &BitmapGlyphCache{
		entries: make(map[bitmapCacheKey]*CachedBitmap),
		maxSize: maxSize,
	}
}

// Get retrieves a cached bitmap, or nil if not cached.
func (c *BitmapGlyphCache) Get(fontID uintptr, glyphID, ppem uint16) *CachedBitmap {
	key := bitmapCacheKey{fontID: fontID, glyphID: glyphID, ppem: ppem}
	return c.entries[key]
}

// Put stores a bitmap in the cache.
func (c *BitmapGlyphCache) Put(fontID uintptr, glyphID, ppem uint16, bitmap *emoji.BitmapGlyph, img image.Image) {
	// Simple eviction: clear when full.
	if len(c.entries) >= c.maxSize {
		c.Clear()
	}

	key := bitmapCacheKey{fontID: fontID, glyphID: glyphID, ppem: ppem}
	c.entries[key] = &CachedBitmap{
		Img:     img,
		OriginX: bitmap.OriginX,
		OriginY: bitmap.OriginY,
		Width:   bitmap.Width,
		Height:  bitmap.Height,
	}
}

// Clear removes all entries from the cache.
func (c *BitmapGlyphCache) Clear() {
	c.entries = make(map[bitmapCacheKey]*CachedBitmap)
}

// Size returns the number of cached entries.
func (c *BitmapGlyphCache) Size() int {
	return len(c.entries)
}
