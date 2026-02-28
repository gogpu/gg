//go:build !nogpu

package gpu

import (
	"fmt"
	"hash/fnv"
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
	const glyphSize = 64
	const pxRange = 8.0

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
// into the atlas, and TextQuads are produced with user-space positions and
// atlas UV coordinates.
//
// Parameters:
//   - face: font face (provides glyph iteration and metrics)
//   - s: the string to render
//   - x, y: baseline origin in user-space coordinates
//   - color: text color as gg.RGBA
//   - viewportW, viewportH: viewport dimensions for building the ortho projection
//   - matrix: the context's current transformation matrix (CTM)
//
// The returned TextBatch contains quads in user-space coordinates. The
// Transform field is set to CTM x ortho_projection so the vertex shader
// transforms positions from user space to clip space. This ensures that
// Scale, Rotate, and Skew transforms applied to the drawing context affect
// text rendering correctly. The MSDF fragment shader's fwidth() automatically
// adapts to the composed transform for correct anti-aliasing.
func (e *GPUTextEngine) LayoutText(
	face text.Face,
	s string,
	x, y float64,
	color gg.RGBA,
	viewportW, viewportH int,
	matrix gg.Matrix,
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

	// Scale ratio: outline is extracted at msdfSize, positions are at fontSize.
	// All outline-space values are multiplied by ratio to get screen pixels.
	refSize := float64(e.msdfSize)
	ratio := fontSize / refSize

	for glyph := range face.Glyphs(s) {
		glyphCount++

		// Extract outline at the REFERENCE size (msdfSize), NOT the rendering
		// fontSize. This ensures the outline bounds EXACTLY match what the MSDF
		// generator uses, regardless of which fontSize first triggered generation.
		// The MSDF is resolution-independent; its cache key is (font, glyph, msdfSize).
		outline, err := e.extractor.ExtractOutline(fontSource.Parsed(), glyph.GID, refSize)
		if err != nil || outline == nil || outline.IsEmpty() {
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

		// Skip empty/degenerate regions (e.g. space characters).
		if region.PlaneMaxX <= region.PlaneMinX || region.PlaneMaxY <= region.PlaneMinY {
			boundsSkip++
			continue
		}

		// Position quad using pre-computed planeBounds from the atlas.
		// PlaneBounds are in refSize outline coordinates; multiply by ratio
		// to convert to screen pixels. This replaces the 15-line
		// scale/padding recomputation that previously duplicated the
		// generator's math.
		qx0 := float32(x + glyph.X + float64(region.PlaneMinX)*ratio)
		qx1 := float32(x + glyph.X + float64(region.PlaneMaxX)*ratio)
		qy0 := float32(y + float64(region.PlaneMinY)*ratio)
		qy1 := float32(y + float64(region.PlaneMaxY)*ratio)

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

	// Build the composed transform: CTM x ortho_projection.
	//
	// The ortho projection maps pixel coordinates to NDC [-1, 1]:
	//   ndc_x = x / w * 2 - 1
	//   ndc_y = 1 - y / h * 2
	// As a 2D affine matrix:
	//   A = 2/w, B = 0,    C = -1
	//   D = 0,   E = -2/h, F = 1
	//
	// The CTM (context's current transformation matrix) is applied first
	// to transform user-space positions to device pixels, then the ortho
	// projection maps to NDC. The composition is: ortho x CTM.
	//
	// This enables Scale, Rotate, and Skew to affect text rendering.
	// The fragment shader's fwidth() automatically adapts to the composed
	// transform, producing correct screenPxRange for anti-aliasing at any
	// scale/rotation.
	vw := float64(viewportW)
	vh := float64(viewportH)
	ortho := gg.Matrix{
		A: 2.0 / vw, B: 0, C: -1.0,
		D: 0, E: -2.0 / vh, F: 1.0,
	}
	// Compose: ortho * CTM (CTM applied first to vertex, then ortho).
	transform := ortho.Multiply(matrix)

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
