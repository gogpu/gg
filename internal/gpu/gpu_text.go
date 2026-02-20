//go:build !nogpu

package gpu

import (
	"fmt"
	"hash/fnv"
	"math"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gg/text/msdf"
)

// GPUTextEngine manages MSDF atlas generation and text layout for GPU
// rendering. It bridges the text shaping infrastructure (Face, Glyph) with
// the GPU text pipeline (TextBatch, TextQuad).
//
// Usage flow:
//  1. Call LayoutText to convert a string into a TextBatch (shapes glyphs,
//     generates MSDF textures into atlas, builds quads).
//  2. Before Flush, call DirtyAtlases/AtlasRGBAData/MarkClean to upload
//     modified atlas pages to GPU textures.
//  3. Pass the resulting TextBatch slice to RenderFrame.
//
// GPUTextEngine is safe for concurrent use.
type GPUTextEngine struct {
	mu sync.Mutex

	atlasManager *msdf.AtlasManager
	extractor    *text.OutlineExtractor

	// msdfSize is the MSDF texture size per glyph cell (in pixels).
	msdfSize int

	// pxRange is the MSDF distance range in pixels (typically 4.0).
	pxRange float32
}

// NewGPUTextEngine creates a new GPU text engine with default configuration.
func NewGPUTextEngine() *GPUTextEngine {
	const glyphSize = 48
	const pxRange = 6.0

	cfg := msdf.DefaultAtlasConfig()
	cfg.GlyphSize = glyphSize
	mgr, _ := msdf.NewAtlasManager(cfg)

	// Update generator range to match our pxRange.
	genCfg := mgr.Generator().Config()
	genCfg.Range = pxRange
	mgr.Generator().SetConfig(genCfg)

	return &GPUTextEngine{
		atlasManager: mgr,
		extractor:    text.NewOutlineExtractor(),
		msdfSize:     glyphSize,
		pxRange:      pxRange,
	}
}

// LayoutText converts a text string with font face into a GPU-ready TextBatch.
// The text is shaped into glyphs, each glyph's MSDF is generated and packed
// into the atlas, and TextQuads are produced with pixel-space positions and
// atlas UV coordinates.
//
// Parameters:
//   - face: font face (provides glyph iteration and metrics)
//   - s: the string to render
//   - x, y: baseline origin in pixel coordinates
//   - color: text color as gg.RGBA
//   - viewportW, viewportH: viewport dimensions for building the pixel-to-NDC transform
//
// The returned TextBatch contains quads in pixel coordinates. The Transform
// field is set to a pixel-to-NDC matrix that the shader uses to convert
// positions to clip space.
func (e *GPUTextEngine) LayoutText(
	face text.Face,
	s string,
	x, y float64,
	color gg.RGBA,
	viewportW, viewportH int,
) (TextBatch, error) {
	if face == nil || s == "" {
		return TextBatch{}, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	fontSize := face.Size()
	fontSource := face.Source()
	fontID := computeFontID(fontSource)
	atlasConfig := e.atlasManager.Config()

	var quads []TextQuad
	var glyphCount, outlineSkip, atlasSkip, boundsSkip int

	for glyph := range face.Glyphs(s) {
		glyphCount++
		// Extract outline for this glyph.
		outline, err := e.extractor.ExtractOutline(fontSource.Parsed(), glyph.GID, fontSize)
		if err != nil || outline == nil || outline.IsEmpty() {
			// Space or unsupported glyph -- skip (advance is handled by position).
			outlineSkip++
			continue
		}

		// Get or generate MSDF in atlas.
		key := msdf.GlyphKey{
			FontID:  fontID,
			GlyphID: uint16(glyph.GID), //nolint:gosec // GlyphID is uint16
			Size:    int16(e.msdfSize), //nolint:gosec // msdfSize fits int16
		}
		region, err := e.atlasManager.Get(key, outline)
		if err != nil {
			slogger().Warn("MSDF atlas get failed", "gid", glyph.GID, "err", err)
			atlasSkip++
			continue
		}

		// Calculate screen-space quad position.
		// glyph.Bounds gives the bounding box in font units at the given size.
		// X,Y in glyph are relative to text origin (0,0).
		// We position the quad so the MSDF cell covers the glyph bounds.
		bounds := glyph.Bounds
		glyphW := bounds.Width()
		glyphH := bounds.Height()

		// If bounds are empty (degenerate glyph), skip.
		if glyphW <= 0 || glyphH <= 0 {
			boundsSkip++
			continue
		}

		// Glyph screen position: baseline origin + glyph offset + bounds offset.
		// bounds.MinX/MinY are relative to the glyph origin.
		//
		// The MSDF generator fits the glyph into a square cell (msdfSize x msdfSize)
		// using UNIFORM scaling: scale = min(scaleX, scaleY). This preserves the
		// aspect ratio but means one axis may not fill the cell. We must replicate
		// that same scale here to compute correct padding for BOTH axes.
		//
		// Generator formula (see generator.go:calculateScale):
		//   available = msdfSize - 2*pxRange
		//   expandedDim = glyphDim + 2*pxRange  (bounds expanded by pxRange)
		//   scale = min(available/expandedW, available/expandedH)
		//
		// Full cell in screen units = msdfSize / scale (square).
		// Padding per axis = (cellScreen - glyphDim) / 2.
		avail := float64(e.msdfSize) - 2*float64(e.pxRange)
		expandedW := glyphW + 2*float64(e.pxRange)
		expandedH := glyphH + 2*float64(e.pxRange)
		scaleX := avail / expandedW
		scaleY := avail / expandedH
		scale := min(scaleX, scaleY)

		// Full MSDF cell in screen (font) units — square, uniform scale.
		cellScreen := float64(e.msdfSize) / scale
		padX := (cellScreen - glyphW) / 2
		padY := (cellScreen - glyphH) / 2

		// Pixel-snap quad corners to the pixel grid. Without snapping,
		// sub-pixel offsets cause the MSDF to be sampled between texels,
		// producing blurry edges on narrow characters like 'i' and 'l'.
		qx0 := float32(math.Floor(x + glyph.X + bounds.MinX - padX))
		qx1 := float32(math.Ceil(x + glyph.X + bounds.MaxX + padX))

		// Y coordinate: Go sfnt returns bounds in Y-down convention
		// (matching Go image coords). MinY is negative (above baseline),
		// MaxY is zero or positive (at/below baseline).
		// Screen Y is also Y-down, so: screenY = baseline + fontY
		qy0 := float32(math.Floor(y + bounds.MinY - padY)) // top of glyph on screen
		qy1 := float32(math.Ceil(y + bounds.MaxY + padY))  // bottom of glyph on screen

		quads = append(quads, TextQuad{
			X0: qx0, Y0: qy0,
			X1: qx1, Y1: qy1,
			U0: region.U0, V0: region.V0,
			U1: region.U1, V1: region.V1,
		})
	}

	slogger().Info("LayoutText result",
		"text", s, "glyphs", glyphCount,
		"quads", len(quads),
		"outlineSkip", outlineSkip, "atlasSkip", atlasSkip, "boundsSkip", boundsSkip,
		"viewport", fmt.Sprintf("%dx%d", viewportW, viewportH))

	if len(quads) == 0 {
		return TextBatch{}, nil
	}

	// Build pixel-to-NDC transform matrix.
	// The MSDF text shader applies: clip_pos = transform * vec4(position, 0, 1)
	// We need: ndc_x = x / w * 2 - 1, ndc_y = 1 - y / h * 2
	// As a mat4x4 stored in the affine layout expected by makeTextUniform:
	//   A = 2/w, B = 0,    C = -1
	//   D = 0,   E = -2/h, F = 1
	vw := float64(viewportW)
	vh := float64(viewportH)
	transform := gg.Matrix{
		A: 2.0 / vw, B: 0, C: -1.0,
		D: 0, E: -2.0 / vh, F: 1.0,
	}

	return TextBatch{
		Quads:      quads,
		Color:      color,
		Transform:  transform,
		AtlasIndex: 0, // Currently single atlas support.
		PxRange:    e.pxRange,
		AtlasSize:  float32(atlasConfig.Size),
	}, nil
}

// DirtyAtlases returns indices of atlases that have been modified since
// the last MarkClean call and need GPU upload.
func (e *GPUTextEngine) DirtyAtlases() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.atlasManager.DirtyAtlases()
}

// AtlasRGBAData returns the atlas pixel data converted from RGB (3 bytes/pixel)
// to RGBA (4 bytes/pixel) suitable for GPU texture upload. Also returns the
// atlas dimensions.
func (e *GPUTextEngine) AtlasRGBAData(index int) (data []byte, width, height int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	atlas := e.atlasManager.GetAtlas(index)
	if atlas == nil {
		return nil, 0, 0
	}
	rgba := rgbToRGBA(atlas.Data, atlas.Size, atlas.Size)
	return rgba, atlas.Size, atlas.Size
}

// MarkClean marks an atlas as uploaded to GPU.
func (e *GPUTextEngine) MarkClean(index int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.atlasManager.MarkClean(index)
}

// AtlasSize returns the atlas texture size (width = height).
func (e *GPUTextEngine) AtlasSize() int {
	return e.atlasManager.Config().Size
}

// PxRange returns the MSDF pixel range.
func (e *GPUTextEngine) PxRange() float32 {
	return e.pxRange
}

// GlyphCount returns the total number of cached glyphs across all atlases.
func (e *GPUTextEngine) GlyphCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.atlasManager.GlyphCount()
}

// computeFontID generates a stable hash identifier for a font source.
// Uses the font name and number of glyphs as a lightweight fingerprint.
func computeFontID(source *text.FontSource) uint64 {
	if source == nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d", source.Name(), source.Parsed().NumGlyphs())
	return h.Sum64()
}
