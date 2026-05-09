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
	"github.com/gogpu/wgpu"
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

	// LCD subpixel rendering configuration.
	lcdLayout text.LCDLayout
	lcdFilter text.LCDFilter

	// GPU textures for atlas pages. Index matches atlas page index.
	pageTextures []*wgpu.Texture
	pageViews    []*wgpu.TextureView
}

// NewGlyphMaskEngine creates a new glyph mask engine with the default atlas
// configuration. LCD subpixel rendering is disabled by default (LCDLayoutNone).
func NewGlyphMaskEngine() *GlyphMaskEngine {
	return &GlyphMaskEngine{
		atlas:      text.NewGlyphMaskAtlasDefault(),
		rasterizer: text.NewGlyphMaskRasterizer(),
		lcdLayout:  text.LCDLayoutNone,
		lcdFilter:  text.DefaultLCDFilter(),
	}
}

// SetLCDLayout sets the LCD subpixel layout for ClearType rendering.
// Use LCDLayoutRGB for most monitors, LCDLayoutBGR for rare BGR panels,
// or LCDLayoutNone to disable subpixel rendering (grayscale).
func (e *GlyphMaskEngine) SetLCDLayout(layout text.LCDLayout) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lcdLayout != layout {
		e.lcdLayout = layout
		// Clear atlas: existing masks were rasterized for different layout.
		e.atlas.Clear()
	}
}

// SetLCDFilter sets the LCD FIR filter for ClearType fringe reduction.
func (e *GlyphMaskEngine) SetLCDFilter(filter text.LCDFilter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lcdFilter = filter
}

// LCDLayout returns the current LCD subpixel layout.
func (e *GlyphMaskEngine) LCDLayout() text.LCDLayout {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lcdLayout
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
func (e *GlyphMaskEngine) LayoutText( //nolint:funlen // text layout with atlas pressure detection
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

	// Determine LCD mode: use ClearType for small axis-aligned text
	// (same conditions as hinting).
	useLCD := e.lcdLayout != text.LCDLayoutNone && selectGlyphMaskLCD(fontSize, matrix)
	lcdLayout := e.lcdLayout
	lcdFilter := e.lcdFilter

	// Premultiply color for batch-level uniform.
	premul := color.Premultiply()
	batchColor := [4]float32{
		float32(premul.R), float32(premul.G),
		float32(premul.B), float32(premul.A),
	}

	var quads []GlyphMaskQuad
	var batchIsLCD bool

	for glyph := range face.Glyphs(s) {
		// Compute subpixel position (fractional part of absolute position).
		absX := x + glyph.X
		absY := y + glyph.Y
		fracX := absX - math.Floor(absX)
		fracY := absY - math.Floor(absY)

		// Size bucket quantization (Skia pattern): under atlas pressure,
		// rasterize at a coarse bucket size and scale quads to actual size.
		// This reduces unique atlas entries from ~57K to ~416 during zoom.
		rasterSize := fontSize
		bucketScale := 1.0
		var key text.GlyphMaskKey
		if e.atlas.UnderPressure() {
			key = text.MakeGlyphMaskKeyBucketed(fontID, glyph.GID, fontSize, fracX, fracY)
			rasterSize = float64(key.SizeQ4) / 16.0
			if rasterSize > 0 {
				bucketScale = fontSize / rasterSize
			}
		} else {
			key = text.MakeGlyphMaskKey(fontID, glyph.GID, fontSize, fracX, fracY)
		}

		var region text.GlyphMaskRegion
		var rErr error

		if useLCD {
			region, rErr = e.rasterizeLCDGlyph(key, parsed, glyph.GID, rasterSize, fracX, fracY, hinting, lcdFilter, lcdLayout)
		} else {
			region, rErr = e.atlas.GetOrRasterize(key, func() ([]byte, int, int, float32, float32, error) {
				result, err2 := e.rasterizer.RasterizeHinted(parsed, glyph.GID, rasterSize, fracX, fracY, hinting)
				if err2 != nil {
					return nil, 0, 0, 0, 0, err2
				}
				if result == nil {
					return nil, 0, 0, 0, 0, nil // empty glyph (space)
				}
				return result.Mask, result.Width, result.Height, result.BearingX, result.BearingY, nil
			})
		}
		if rErr != nil {
			slogger().Warn("glyph mask rasterize failed", "gid", glyph.GID, "err", rErr)
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
		// The mask was rasterized at deviceScale * rasterSize. We convert
		// mask pixel coordinates to user space by dividing by deviceScale,
		// then scale by bucketScale to match the actual display size.
		// In normal mode bucketScale=1.0 (no-op). In bucketed mode
		// bucketScale = actualSize/bucketSize (Skia strikeToSourceScale).
		scale := bucketScale / deviceScale

		// For LCD glyphs, the atlas region.Width is 3x the logical pixel width.
		// The screen quad width must use the logical width (region.Width / 3).
		regionLogicalW := region.Width
		if region.IsLCD {
			regionLogicalW = region.Width / 3
		}

		qx0 := float32(absX + float64(region.BearingX)*scale)
		qy0 := float32(absY - float64(region.BearingY)*scale) // flip Y: bearing is up, screen is down
		qx1 := qx0 + float32(float64(regionLogicalW)*scale)
		qy1 := qy0 + float32(float64(region.Height)*scale)

		if region.IsLCD {
			batchIsLCD = true
		}

		quads = append(quads, GlyphMaskQuad{
			X0: qx0, Y0: qy0,
			X1: qx1, Y1: qy1,
			U0: region.U0, V0: region.V0,
			U1: region.U1, V1: region.V1,
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

	// Atlas dimensions for the LCD shader's texel stepping.
	atlasConfig := e.atlas.Config()
	atlasSize := float32(atlasConfig.Size)

	return GlyphMaskBatch{
		Quads:          quads,
		Transform:      transform,
		Color:          batchColor,
		IsLCD:          batchIsLCD,
		AtlasWidth:     atlasSize,
		AtlasHeight:    atlasSize,
		AtlasPageIndex: 0, // Currently single page support (first page).
	}, nil
}

// SyncAtlasTextures uploads dirty atlas pages to the GPU as R8 textures.
// Must be called before rendering any glyph mask batches. Creates new
// textures on first use and re-uploads data when pages are modified.
func (e *GlyphMaskEngine) SyncAtlasTextures(device *wgpu.Device, queue *wgpu.Queue) error {
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
			tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
				Label:         fmt.Sprintf("glyph_mask_atlas_%d", idx),
				Size:          wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
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

			view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
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
			&wgpu.ImageCopyTexture{
				Texture:  e.pageTextures[idx],
				MipLevel: 0,
			},
			r8Data,
			&wgpu.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  size,
				RowsPerImage: size,
			},
			&wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		); err != nil {
			return fmt.Errorf("upload glyph mask atlas %d: %w", idx, err)
		}

		e.atlas.MarkClean(idx)
	}

	return nil
}

// PageTextureView returns the GPU texture view for the given atlas page.
// Returns nil if the page has not been uploaded.
func (e *GlyphMaskEngine) PageTextureView(index int) *wgpu.TextureView {
	e.mu.Lock()
	defer e.mu.Unlock()
	if index < 0 || index >= len(e.pageViews) {
		return nil
	}
	return e.pageViews[index]
}

// Destroy releases all GPU textures held by the engine.
func (e *GlyphMaskEngine) Destroy(device *wgpu.Device) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, v := range e.pageViews {
		if v != nil {
			v.Release()
		}
	}
	e.pageViews = nil

	for _, t := range e.pageTextures {
		if t != nil {
			t.Release()
		}
	}
	e.pageTextures = nil

	e.atlas.Clear()
}

// Atlas returns the underlying glyph mask atlas (for testing/introspection).
func (e *GlyphMaskEngine) Atlas() *text.GlyphMaskAtlas {
	return e.atlas
}

// rasterizeLCDGlyph rasterizes a glyph with LCD subpixel rendering and stores
// the RGB coverage data in the R8 atlas at 3x width. Returns a cached region
// if already present.
func (e *GlyphMaskEngine) rasterizeLCDGlyph(
	key text.GlyphMaskKey,
	parsed text.ParsedFont,
	gid text.GlyphID,
	fontSize float64,
	fracX, fracY float64,
	hinting text.Hinting,
	filter text.LCDFilter,
	layout text.LCDLayout,
) (text.GlyphMaskRegion, error) {
	// Fast path: check cache.
	if region, ok := e.atlas.Get(key); ok {
		return region, nil
	}

	// Slow path: rasterize with LCD.
	result, err := e.rasterizer.RasterizeLCD(parsed, gid, fontSize, fracX, fracY, hinting, filter, layout)
	if err != nil {
		return text.GlyphMaskRegion{}, fmt.Errorf("lcd glyph rasterize: %w", err)
	}
	if result == nil {
		return text.GlyphMaskRegion{}, nil // empty glyph (space)
	}

	return e.atlas.PutLCD(key, result.Mask, result.Width, result.Height, result.BearingX, result.BearingY)
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

// glyphMaskLCDMaxSize is the maximum font size in device pixels for which
// LCD subpixel rendering is auto-enabled. Above this size, individual subpixels
// are large enough that per-channel alpha provides no visual benefit and the
// color fringing becomes more noticeable.
const glyphMaskLCDMaxSize = 48.0

// selectGlyphMaskLCD returns true if LCD subpixel rendering should be used.
// LCD rendering requires an axis-aligned matrix (no rotation/skew) and small
// font size (same conditions as hinting, since ClearType depends on the
// subpixel grid being axis-aligned).
func selectGlyphMaskLCD(fontSize float64, matrix gg.Matrix) bool {
	// Rotated/skewed text: subpixel grid is not axis-aligned.
	if matrix.B != 0 || matrix.D != 0 {
		return false
	}
	// Large text: subpixels are big enough that per-channel alpha isn't needed.
	return fontSize <= glyphMaskLCDMaxSize
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
