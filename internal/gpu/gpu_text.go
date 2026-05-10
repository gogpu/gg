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

	atlasManager    *msdf.AtlasManager
	cjkAtlasManager *msdf.AtlasManager // ADR-027: separate atlas for CJK (128px reference)
	extractor       *text.OutlineExtractor

	// msdfSize is the MSDF texture size per glyph cell (in pixels).
	msdfSize    int
	msdfSizeCJK int // ADR-027: 128px for CJK display text

	// pxRange is the MSDF distance range in pixels (typically 4.0).
	pxRange float32
}

// NewGPUTextEngine creates a new GPU text engine with default configuration.
func NewGPUTextEngine() *GPUTextEngine {
	const glyphSize = 64
	const glyphSizeCJK = 128 // ADR-027: 2x for dense CJK strokes (MapLibre pattern)
	const pxRange = 4.0

	cfg := msdf.DefaultAtlasConfig()
	cfg.GlyphSize = glyphSize
	mgr, _ := msdf.NewAtlasManager(cfg)
	genCfg := mgr.Generator().Config()
	genCfg.Range = pxRange
	mgr.Generator().SetConfig(genCfg)

	// ADR-027: CJK display text uses 128px reference with 2048 texture.
	// Single atlas manager with larger reference — CJK glyphs that reach MSDF
	// (>64px display text) are rare and benefit from 2x resolution.
	// Body text CJK (≤64px) routes to Tier 6 bitmap, never reaches here.
	cjkCfg := msdf.DefaultAtlasConfig()
	cjkCfg.GlyphSize = glyphSizeCJK
	cjkCfg.Size = 2048
	cjkMgr, _ := msdf.NewAtlasManager(cjkCfg)
	cjkGenCfg := cjkMgr.Generator().Config()
	cjkGenCfg.Range = pxRange
	cjkMgr.Generator().SetConfig(cjkGenCfg)

	return &GPUTextEngine{
		atlasManager:    mgr,
		cjkAtlasManager: cjkMgr,
		extractor:       text.NewOutlineExtractor(),
		msdfSize:        glyphSize,
		msdfSizeCJK:     glyphSizeCJK,
		pxRange:         pxRange,
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
//
// The deviceScale parameter scales the logical font size to physical pixels
// (e.g., 2.0 on a Retina display). This produces a higher screenPxRange in
// the MSDF shader, yielding crisper text on HiDPI displays. Quad positions
// remain in logical coordinates because the CTM already handles device scaling.
// Pass 1.0 for standard (non-HiDPI) rendering.
func (e *GPUTextEngine) LayoutText(
	face text.Face,
	s string,
	x, y float64,
	color gg.RGBA,
	matrix gg.Matrix,
	deviceScale float64,
) (TextBatch, error) {
	if face == nil || s == "" {
		return TextBatch{}, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	logicalSize := face.Size()
	if logicalSize <= 0 {
		logicalSize = 16 // fallback: never zero
	}
	fontSource := face.Source()
	fontID := computeFontID(fontSource)

	// ADR-027: detect CJK from first rune → select appropriate atlas.
	isCJK := false
	for _, r := range s {
		isCJK = text.IsCJKRune(r)
		break
	}
	activeAtlas := e.atlasManager
	atlasIndex := 0
	if isCJK {
		activeAtlas = e.cjkAtlasManager
		atlasIndex = cjkAtlasOffset
	}
	atlasConfig := activeAtlas.Config()

	var quads []TextQuad
	var glyphCount, outlineSkip, atlasSkip, boundsSkip int

	// Scale ratio: outline is extracted at msdfSize, quad positions are in user space.
	// All outline-space values are multiplied by ratio to get user-space coordinates.
	// Use logicalSize (not fontSize which includes deviceScale) because the CTM
	// already handles device scaling — quads must be in logical user-space coords.
	// (BUG-MSDF-RETINA-001: was fontSize/refSize which doubled positions on Retina)
	refSize := float64(e.msdfSize)
	refSizeCJK := float64(e.msdfSizeCJK)
	var ratio float64

	for glyph := range face.Glyphs(s) {
		glyphCount++

		// ADR-027: CJK display text uses 128px reference for dense strokes.
		glyphRefSize := refSize
		glyphAtlas := activeAtlas
		glyphMsdfSize := e.msdfSize
		if text.IsCJKRune(glyph.Rune) {
			glyphRefSize = refSizeCJK
			glyphAtlas = e.cjkAtlasManager
			glyphMsdfSize = e.msdfSizeCJK
			ratio = logicalSize / refSizeCJK
		} else {
			ratio = logicalSize / refSize
		}

		outline, err := e.extractor.ExtractOutline(fontSource.Parsed(), glyph.GID, glyphRefSize)
		if err != nil || outline == nil || outline.IsEmpty() {
			outlineSkip++
			continue
		}

		key := msdf.GlyphKey{
			FontID:  fontID,
			GlyphID: uint16(glyph.GID),    //nolint:gosec // GlyphID is uint16
			Size:    int16(glyphMsdfSize), //nolint:gosec // msdfSize fits int16
		}
		region, err := glyphAtlas.Get(key, outline)
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
		"outlineSkip", outlineSkip, "atlasSkip", atlasSkip, "boundsSkip", boundsSkip)

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
	// Store device-space CTM only — ortho projection deferred to flush time
	// when actual render target dimensions are known (ADR-025).

	return TextBatch{
		Quads:      quads,
		Color:      color,
		Transform:  matrix,
		AtlasIndex: atlasIndex,
		PxRange:    e.pxRange,
		AtlasSize:  float32(atlasConfig.Size),
	}, nil
}

// cjkAtlasOffset is the index offset for CJK atlas pages.
// Latin atlas pages: 0..N-1, CJK atlas pages: cjkAtlasOffset..cjkAtlasOffset+M-1.
const cjkAtlasOffset = 100

// DirtyAtlases returns indices of atlases that have been modified since
// the last MarkClean call and need GPU upload. Includes both Latin and CJK atlases.
func (e *GPUTextEngine) DirtyAtlases() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	dirty := e.atlasManager.DirtyAtlases()
	for _, idx := range e.cjkAtlasManager.DirtyAtlases() {
		dirty = append(dirty, idx+cjkAtlasOffset)
	}
	return dirty
}

// AtlasRGBAData returns the atlas pixel data converted from RGB (3 bytes/pixel)
// to RGBA (4 bytes/pixel) suitable for GPU texture upload. Also returns the
// atlas dimensions. Indices ≥ cjkAtlasOffset refer to CJK atlas pages.
func (e *GPUTextEngine) AtlasRGBAData(index int) (data []byte, width, height int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var atlas *msdf.Atlas
	if index >= cjkAtlasOffset {
		atlas = e.cjkAtlasManager.GetAtlas(index - cjkAtlasOffset)
	} else {
		atlas = e.atlasManager.GetAtlas(index)
	}
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
	if index >= cjkAtlasOffset {
		e.cjkAtlasManager.MarkClean(index - cjkAtlasOffset)
	} else {
		e.atlasManager.MarkClean(index)
	}
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
// Uses the full font name (includes subfamily like "Regular"/"Bold") and
// number of glyphs as a lightweight fingerprint. The full name is critical
// to distinguish fonts within the same family (e.g., "Go Regular" vs "Go Bold")
// that share the same family name and glyph count.
func computeFontID(source *text.FontSource) uint64 {
	if source == nil {
		return 0
	}
	h := fnv.New64a()
	parsed := source.Parsed()
	fullName := parsed.FullName()
	if fullName == "" {
		fullName = source.Name() // fallback to family name
	}
	_, _ = fmt.Fprintf(h, "%s:%d", fullName, parsed.NumGlyphs())
	return h.Sum64()
}
