//go:build !nogpu

package gpu

import (
	"fmt"
	"hash/fnv"
	"math"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// GlyphMaskEngine manages the CPU-rasterized glyph mask atlas and produces
// GPU-ready GlyphMaskBatch data for Tier 6 rendering. It bridges the text
// shaping infrastructure (Face, Glyph) with the GPU glyph mask pipeline
// (GlyphMaskBatch, GlyphMaskQuad).
//
// Usage flow:
//  1. Call LayoutText to convert a string into a GlyphMaskBatch (shapes glyphs,
//     rasterizes missing glyphs into the R8 atlas, builds quads).
//  2. Before rendering, call SyncAtlasTextures to upload dirty atlas pages
//     to GPU textures.
//  3. Pass the resulting GlyphMaskBatch slice to RenderFrame.
//
// GlyphMaskEngine is safe for concurrent use.
type GlyphMaskEngine struct {
	mu sync.Mutex

	atlas      *text.GlyphMaskAtlas
	rasterizer *text.GlyphMaskRasterizer

	// GPU textures for atlas pages. Index matches atlas page index.
	pageTextures []hal.Texture
	pageViews    []hal.TextureView
}

// NewGlyphMaskEngine creates a new glyph mask engine with the default atlas
// configuration.
func NewGlyphMaskEngine() *GlyphMaskEngine {
	return &GlyphMaskEngine{
		atlas:      text.NewGlyphMaskAtlasDefault(),
		rasterizer: text.NewGlyphMaskRasterizer(),
	}
}

// LayoutText converts a text string with font face into a GPU-ready
// GlyphMaskBatch. The text is shaped into glyphs, each glyph is rasterized
// (or retrieved from cache) into the R8 alpha atlas, and GlyphMaskQuads are
// produced with screen-space positions and atlas UV coordinates.
//
// Parameters:
//   - face: font face (provides glyph iteration and metrics)
//   - s: the string to render
//   - x, y: baseline origin in user-space coordinates
//   - color: text color as gg.RGBA
//   - viewportW, viewportH: viewport dimensions for building the ortho projection
//   - matrix: the context's current transformation matrix (CTM)
//   - deviceScale: DPI scale factor (e.g., 2.0 on Retina)
//
// The returned GlyphMaskBatch contains quads in user-space coordinates. The
// Transform field is set to CTM x ortho_projection so the vertex shader
// transforms positions from user space to clip space.
func (e *GlyphMaskEngine) LayoutText(
	face text.Face,
	s string,
	x, y float64,
	color gg.RGBA,
	viewportW, viewportH int,
	matrix gg.Matrix,
	deviceScale float64,
) (GlyphMaskBatch, error) {
	if face == nil || s == "" {
		return GlyphMaskBatch{}, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	fontSize := face.Size() * deviceScale
	if fontSize <= 0 {
		fontSize = face.Size()
	}
	fontSource := face.Source()
	fontID := computeGlyphMaskFontID(fontSource)
	parsed := fontSource.Parsed()

	// Auto-select hinting: enable for small text (≤48px) on axis-aligned
	// matrices. Hinting grid-fits outlines to pixel boundaries, which only
	// makes sense when the pixel grid is axis-aligned (no rotation/skew).
	hinting := selectGlyphMaskHinting(fontSize, matrix)

	// Premultiply color for per-vertex embedding.
	premul := color.Premultiply()
	vertColor := [4]float32{
		float32(premul.R), float32(premul.G),
		float32(premul.B), float32(premul.A),
	}

	var quads []GlyphMaskQuad

	for glyph := range face.Glyphs(s) {
		// Compute subpixel position (fractional part of absolute position).
		absX := x + glyph.X
		absY := y + glyph.Y
		fracX := absX - math.Floor(absX)
		fracY := absY - math.Floor(absY)

		key := text.MakeGlyphMaskKey(fontID, glyph.GID, fontSize, fracX, fracY)

		// GetOrRasterize: cache hit returns immediately, miss triggers
		// CPU rasterization via AnalyticFiller.
		region, err := e.atlas.GetOrRasterize(key, func() ([]byte, int, int, float32, float32, error) {
			result, rErr := e.rasterizer.RasterizeHinted(parsed, glyph.GID, fontSize, fracX, fracY, hinting)
			if rErr != nil {
				return nil, 0, 0, 0, 0, rErr
			}
			if result == nil {
				return nil, 0, 0, 0, 0, nil // empty glyph (space)
			}
			return result.Mask, result.Width, result.Height, result.BearingX, result.BearingY, nil
		})
		if err != nil {
			slogger().Warn("glyph mask rasterize failed", "gid", glyph.GID, "err", err)
			continue
		}

		// Empty glyph (e.g., space) — no quad needed.
		if region.Width <= 0 || region.Height <= 0 {
			continue
		}

		// Position the quad in user space using glyph bearings.
		// BearingX: offset from glyph origin to left edge of mask.
		// BearingY: offset from baseline to top edge of mask (positive = above).
		//
		// The mask was rasterized at deviceScale * fontSize. We need to
		// convert mask pixel coordinates back to user-space coordinates
		// by dividing by deviceScale.
		scale := 1.0 / deviceScale
		qx0 := float32(absX + float64(region.BearingX)*scale)
		qy0 := float32(absY - float64(region.BearingY)*scale) // flip Y: bearing is up, screen is down
		qx1 := qx0 + float32(float64(region.Width)*scale)
		qy1 := qy0 + float32(float64(region.Height)*scale)

		quads = append(quads, GlyphMaskQuad{
			X0: qx0, Y0: qy0,
			X1: qx1, Y1: qy1,
			U0: region.U0, V0: region.V0,
			U1: region.U1, V1: region.V1,
			Color: vertColor,
		})
	}

	if len(quads) == 0 {
		return GlyphMaskBatch{}, nil
	}

	// Build the composed transform: CTM x ortho_projection.
	// The ortho projection maps pixel coordinates to NDC [-1, 1]:
	//   ndc_x = x / w * 2 - 1
	//   ndc_y = 1 - y / h * 2
	vw := float64(viewportW)
	vh := float64(viewportH)
	ortho := gg.Matrix{
		A: 2.0 / vw, B: 0, C: -1.0,
		D: 0, E: -2.0 / vh, F: 1.0,
	}
	transform := ortho.Multiply(matrix)

	return GlyphMaskBatch{
		Quads:          quads,
		Transform:      transform,
		AtlasPageIndex: 0, // Currently single page support (first page).
	}, nil
}

// SyncAtlasTextures uploads dirty atlas pages to the GPU as R8 textures.
// Must be called before rendering any glyph mask batches. Creates new
// textures on first use and re-uploads data when pages are modified.
func (e *GlyphMaskEngine) SyncAtlasTextures(device hal.Device, queue hal.Queue) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	dirtyPages := e.atlas.DirtyPages()
	if len(dirtyPages) == 0 {
		return nil
	}

	for _, idx := range dirtyPages {
		r8Data, pageSize, _ := e.atlas.PageR8Data(idx)
		if r8Data == nil || pageSize == 0 {
			continue
		}

		// Ensure texture/view slices are large enough.
		for len(e.pageTextures) <= idx {
			e.pageTextures = append(e.pageTextures, nil)
			e.pageViews = append(e.pageViews, nil)
		}

		size := uint32(pageSize) //nolint:gosec // atlas size always fits uint32

		// Create texture on first use.
		if e.pageTextures[idx] == nil {
			tex, err := device.CreateTexture(&hal.TextureDescriptor{
				Label:         fmt.Sprintf("glyph_mask_atlas_%d", idx),
				Size:          hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     gputypes.TextureDimension2D,
				Format:        gputypes.TextureFormatR8Unorm,
				Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
			})
			if err != nil {
				return fmt.Errorf("create glyph mask atlas texture %d: %w", idx, err)
			}
			e.pageTextures[idx] = tex

			view, err := device.CreateTextureView(tex, &hal.TextureViewDescriptor{
				Label:         fmt.Sprintf("glyph_mask_atlas_%d_view", idx),
				Format:        gputypes.TextureFormatR8Unorm,
				Dimension:     gputypes.TextureViewDimension2D,
				Aspect:        gputypes.TextureAspectAll,
				MipLevelCount: 1,
			})
			if err != nil {
				return fmt.Errorf("create glyph mask atlas view %d: %w", idx, err)
			}
			e.pageViews[idx] = view
		}

		// Upload R8 data. R8 format = 1 byte per pixel, so BytesPerRow = width.
		if err := queue.WriteTexture(
			&hal.ImageCopyTexture{
				Texture:  e.pageTextures[idx],
				MipLevel: 0,
			},
			r8Data,
			&hal.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  size,
				RowsPerImage: size,
			},
			&hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		); err != nil {
			return fmt.Errorf("upload glyph mask atlas %d: %w", idx, err)
		}

		e.atlas.MarkClean(idx)
	}

	return nil
}

// PageTextureView returns the GPU texture view for the given atlas page.
// Returns nil if the page has not been uploaded.
func (e *GlyphMaskEngine) PageTextureView(index int) hal.TextureView {
	e.mu.Lock()
	defer e.mu.Unlock()
	if index < 0 || index >= len(e.pageViews) {
		return nil
	}
	return e.pageViews[index]
}

// Destroy releases all GPU textures held by the engine.
func (e *GlyphMaskEngine) Destroy(device hal.Device) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, v := range e.pageViews {
		if v != nil {
			device.DestroyTextureView(v)
		}
	}
	e.pageViews = nil

	for _, t := range e.pageTextures {
		if t != nil {
			device.DestroyTexture(t)
		}
	}
	e.pageTextures = nil

	e.atlas.Clear()
}

// Atlas returns the underlying glyph mask atlas (for testing/introspection).
func (e *GlyphMaskEngine) Atlas() *text.GlyphMaskAtlas {
	return e.atlas
}

// glyphMaskHintingMaxSize is the maximum font size in device pixels for which
// hinting is auto-enabled. Above this size, outlines are smooth enough that
// grid-fitting provides no visual benefit and can introduce distortion.
const glyphMaskHintingMaxSize = 48.0

// selectGlyphMaskHinting returns the hinting mode for glyph mask rendering.
// Hinting is enabled for small text (≤48px) when the CTM is axis-aligned
// (no rotation or skew), since grid-fitting requires an aligned pixel grid.
func selectGlyphMaskHinting(fontSize float64, matrix gg.Matrix) text.Hinting {
	// Rotated/skewed text: pixel grid is not axis-aligned, hinting would distort.
	if matrix.B != 0 || matrix.D != 0 {
		return text.HintingNone
	}

	// Large text: smooth enough without hinting.
	if fontSize > glyphMaskHintingMaxSize {
		return text.HintingNone
	}

	// Small axis-aligned text: full hinting for crisp stems and baselines.
	return text.HintingFull
}

// computeGlyphMaskFontID generates a stable hash identifier for a font source.
// Uses the same approach as computeFontID in gpu_text.go.
func computeGlyphMaskFontID(source *text.FontSource) uint64 {
	if source == nil {
		return 0
	}
	h := fnv.New64a()
	parsed := source.Parsed()
	fullName := parsed.FullName()
	if fullName == "" {
		fullName = source.Name()
	}
	_, _ = fmt.Fprintf(h, "%s:%d", fullName, parsed.NumGlyphs())
	return h.Sum64()
}
