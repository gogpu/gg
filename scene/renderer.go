package scene

import (
	"context"
	"image"
	"runtime"
	"sync"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/parallel"
	"github.com/gogpu/gg/text"
)

// tilePool manages pooled resources for per-tile rendering.
// SoftwareRenderers, Pixmaps, Decoders, Paths, Paints, and clip masks are
// expensive to allocate, so we reuse them across tiles and frames via sync.Pool.
type tilePool struct {
	renderers  sync.Pool // *gg.SoftwareRenderer
	pixmaps    sync.Pool // *gg.Pixmap
	decoders   sync.Pool // *Decoder
	scenePaths sync.Pool // *Path (scene.Path for decode loop)
	ggPaths    sync.Pool // *gg.Path (for convertPath)
	fillPaints sync.Pool // *gg.Paint (for convertFillPaint)
	clipMasks  sync.Pool // *[]byte (for clip mask buffers)
}

// getRenderer returns a SoftwareRenderer from the pool, resizing if necessary.
func (p *tilePool) getRenderer(w, h int) *gg.SoftwareRenderer {
	if v := p.renderers.Get(); v != nil {
		sr := v.(*gg.SoftwareRenderer)
		sr.Resize(w, h)
		return sr
	}
	return gg.NewSoftwareRenderer(w, h)
}

// putRenderer returns a SoftwareRenderer to the pool for reuse.
func (p *tilePool) putRenderer(sr *gg.SoftwareRenderer) {
	p.renderers.Put(sr)
}

// getPixmap returns a Pixmap from the pool. If the cached pixmap has the right
// dimensions it is cleared and reused; otherwise a new one is allocated.
func (p *tilePool) getPixmap(w, h int) *gg.Pixmap {
	if v := p.pixmaps.Get(); v != nil {
		pm := v.(*gg.Pixmap)
		if pm.Width() == w && pm.Height() == h {
			pm.Clear(gg.Transparent)
			return pm
		}
	}
	return gg.NewPixmap(w, h)
}

// putPixmap returns a Pixmap to the pool for reuse.
func (p *tilePool) putPixmap(pm *gg.Pixmap) {
	p.pixmaps.Put(pm)
}

// getDecoder returns a Decoder from the pool, reset to the given encoding.
func (p *tilePool) getDecoder(enc *Encoding) *Decoder {
	if v := p.decoders.Get(); v != nil {
		dec := v.(*Decoder)
		dec.Reset(enc)
		return dec
	}
	return NewDecoder(enc)
}

// putDecoder returns a Decoder to the pool for reuse.
func (p *tilePool) putDecoder(dec *Decoder) {
	p.decoders.Put(dec)
}

// getScenePath returns a scene.Path from the pool, reset for reuse.
func (p *tilePool) getScenePath() *Path {
	if v := p.scenePaths.Get(); v != nil {
		sp := v.(*Path)
		sp.Reset()
		return sp
	}
	return NewPath()
}

// putScenePath returns a scene.Path to the pool for reuse.
func (p *tilePool) putScenePath(sp *Path) {
	p.scenePaths.Put(sp)
}

// getGGPath returns a gg.Path from the pool, cleared for reuse.
func (p *tilePool) getGGPath() *gg.Path {
	if v := p.ggPaths.Get(); v != nil {
		gp := v.(*gg.Path)
		gp.Clear()
		return gp
	}
	return gg.NewPath()
}

// putGGPath returns a gg.Path to the pool for reuse.
func (p *tilePool) putGGPath(gp *gg.Path) {
	p.ggPaths.Put(gp)
}

// getFillPaint returns a Paint from the pool, reset for fill use.
func (p *tilePool) getFillPaint() *gg.Paint {
	if v := p.fillPaints.Get(); v != nil {
		return v.(*gg.Paint)
	}
	return gg.NewPaint()
}

// putFillPaint returns a Paint to the pool for reuse.
func (p *tilePool) putFillPaint(paint *gg.Paint) {
	p.fillPaints.Put(paint)
}

// getClipMask returns a clip mask buffer from the pool (at least size bytes).
func (p *tilePool) getClipMask(size int) []byte {
	if v := p.clipMasks.Get(); v != nil {
		buf := v.(*[]byte)
		if cap(*buf) >= size {
			b := (*buf)[:size]
			clear(b)
			return b
		}
	}
	return make([]byte, size)
}

// putClipMask returns a clip mask buffer to the pool for reuse.
func (p *tilePool) putClipMask(buf []byte) {
	p.clipMasks.Put(&buf)
}

// Renderer renders Scene content to a target Pixmap using parallel tile-based processing.
// It integrates with TileGrid for spatial subdivision and WorkerPool for concurrent execution.
//
// The renderer supports:
//   - Full scene rendering (all tiles)
//   - Incremental rendering (dirty tiles only)
//   - Layer caching for static content
//   - Performance statistics collection
//
// Thread safety: Renderer methods are safe for concurrent use after initialization.
type Renderer struct {
	// Tile-based rendering infrastructure
	tileGrid   *parallel.TileGrid
	workerPool *parallel.WorkerPool
	dirty      *parallel.DirtyRegion

	// Layer caching
	cache *LayerCache

	// Per-tile resource pool (SoftwareRenderer + Pixmap reuse)
	pool tilePool

	// Dimensions
	width  int
	height int

	// Configuration
	tileSize int
	workers  int

	// Font registry for TagText resolution (set per-render from Scene).
	fontRegistry map[uint64]*text.FontSource

	// Statistics
	stats     RenderStats
	statsMu   sync.RWMutex
	lastFrame time.Time
}

// RenderStats contains performance statistics for a render operation.
type RenderStats struct {
	// Tile statistics
	TilesTotal    int
	TilesDirty    int
	TilesRendered int

	// Layer statistics
	LayersCached   int
	LayersRendered int

	// Timing (durations for the last render)
	TimeEncode    time.Duration
	TimeRaster    time.Duration
	TimeComposite time.Duration
	TimeTotal     time.Duration

	// Frame timing
	FrameTime time.Duration
	FPS       float64
}

// RendererOption configures a Renderer.
type RendererOption func(*Renderer)

// WithCacheSize sets the layer cache size in megabytes.
// Default is 64MB.
func WithCacheSize(mb int) RendererOption {
	return func(r *Renderer) {
		if r.cache != nil {
			r.cache.SetMaxSize(mb)
		}
	}
}

// WithWorkers sets the number of worker goroutines for parallel rendering.
// If n <= 0, GOMAXPROCS is used.
func WithWorkers(n int) RendererOption {
	return func(r *Renderer) {
		r.workers = n
	}
}

// WithTileSize sets the tile size for rendering.
// This is informational only; actual tile size is fixed at 64x64.
func WithTileSize(size int) RendererOption {
	return func(r *Renderer) {
		r.tileSize = size
	}
}

// WithCache sets a custom layer cache.
// If nil, a default cache is created.
func WithCache(cache *LayerCache) RendererOption {
	return func(r *Renderer) {
		r.cache = cache
	}
}

// NewRenderer creates a new scene renderer for the given dimensions.
// Options can be used to configure caching, parallelism, and other settings.
func NewRenderer(width, height int, opts ...RendererOption) *Renderer {
	if width <= 0 || height <= 0 {
		return nil
	}

	workers := runtime.GOMAXPROCS(0)

	r := &Renderer{
		width:    width,
		height:   height,
		tileSize: parallel.TileWidth,
		workers:  workers,
		cache:    DefaultLayerCache(),
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// Initialize parallel infrastructure
	r.tileGrid = parallel.NewTileGrid(width, height)
	r.workerPool = parallel.NewWorkerPool(r.workers)

	// Initialize dirty region tracking
	tilesX := (width + parallel.TileWidth - 1) / parallel.TileWidth
	tilesY := (height + parallel.TileHeight - 1) / parallel.TileHeight
	r.dirty = parallel.NewDirtyRegion(tilesX, tilesY)
	r.dirty.MarkAll() // Initially all tiles are dirty

	return r
}

// renderGPU renders the scene through the GPU accelerator via GPUSceneRenderer.
// Creates a temporary gg.Context backed by the target pixmap, renders the scene
// through it (GPU shapes → FlushGPU → readback to pixmap), returns nil on success.
func (r *Renderer) renderGPU(target *gg.Pixmap, scene *Scene) error {
	dc := gg.NewContextForPixmap(target)
	if dc == nil {
		return gg.ErrFallbackToCPU
	}
	defer func() { _ = dc.Close() }()

	gpuR := NewGPUSceneRenderer(dc)
	if err := gpuR.RenderScene(scene); err != nil {
		return err
	}
	return dc.FlushGPU()
}

// Render renders the entire scene to the target pixmap.
// This processes all tiles regardless of dirty state.
//
// For cancellable rendering, use RenderWithContext.
func (r *Renderer) Render(target *gg.Pixmap, scene *Scene) error {
	return r.RenderWithContext(context.Background(), target, scene)
}

// RenderWithContext renders the entire scene to the target pixmap with cancellation support.
// This processes all tiles regardless of dirty state.
//
// The context can be used to cancel long-running renders. When canceled,
// the function returns ctx.Err() and the target may contain partial results.
func (r *Renderer) RenderWithContext(ctx context.Context, target *gg.Pixmap, scene *Scene) error {
	if target == nil || scene == nil {
		return nil
	}

	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// GPU fast path: if a GPU accelerator is registered, render through
	// GPUSceneRenderer which decodes scene commands into gg.Context GPU calls.
	// The gg.Context handles GPU→CPU fallback automatically per-shape.
	if gg.Accelerator() != nil {
		if err := r.renderGPU(target, scene); err == nil {
			return nil
		}
	}

	startTotal := time.Now()
	r.statsMu.Lock()
	r.stats = RenderStats{} // Reset stats
	r.statsMu.Unlock()

	// Mark all tiles dirty for full render
	r.dirty.MarkAll()

	// Get the flattened encoding, image registry, and font registry.
	startEncode := time.Now()
	enc := scene.Encoding()
	images := scene.Images()
	r.fontRegistry = scene.FontRegistry()
	encodeTime := time.Since(startEncode)

	// Check for cancellation after encoding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get all tiles for rendering
	tiles := r.tileGrid.AllTiles()
	tilesTotal := len(tiles)

	// Render tiles in parallel with context
	startRaster := time.Now()
	if err := r.renderTilesWithContext(ctx, tiles, enc, target, images); err != nil {
		return err
	}
	rasterTime := time.Since(startRaster)

	// Check for cancellation before compositing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Composite tiles to target
	startComposite := time.Now()
	r.compositeTiles(tiles, target)
	compositeTime := time.Since(startComposite)

	// Clear dirty flags
	r.dirty.Clear()
	r.tileGrid.ClearDirty()

	// Update statistics
	totalTime := time.Since(startTotal)
	r.updateStats(tilesTotal, tilesTotal, tilesTotal, encodeTime, rasterTime, compositeTime, totalTime)

	return nil
}

// RenderDirty renders only the dirty regions of the scene.
// This is more efficient when only parts of the scene have changed.
// The dirty parameter specifies which tiles need re-rendering.
//
// For cancellable rendering, use RenderDirtyWithContext.
func (r *Renderer) RenderDirty(target *gg.Pixmap, scene *Scene, dirty *parallel.DirtyRegion) error {
	return r.RenderDirtyWithContext(context.Background(), target, scene, dirty)
}

// RenderWithDamage uses a DamageTracker to compute the minimal dirty region
// from frame-to-frame object changes, then renders only affected tiles.
// This is Level 1-2 of the four-level damage pipeline (ADR-021).
//
// Returns the damage rect (in pixels) for downstream use by ggcanvas/gogpu
// (Level 3-4: GPU scissor + OS present). Returns image.Rectangle{} if
// nothing changed (caller can skip GPU upload + present entirely).
//
// On first frame, renders everything (full scene).
func (r *Renderer) RenderWithDamage(target *gg.Pixmap, scene *Scene, tracker *DamageTracker) (
	damageRect image.Rectangle, err error,
) {
	if target == nil || scene == nil {
		return image.Rectangle{}, nil
	}

	objects := scene.TaggedBounds()
	damage := tracker.ComputeDamage(objects)

	if damage.Empty() && !tracker.IsFirstRender() {
		return image.Rectangle{}, nil
	}

	// First render or actual damage — determine what to redraw
	if tracker.IsFirstRender() {
		tracker.MarkRendered()
		r.dirty.MarkAll()
		err = r.RenderDirty(target, scene, nil)
		return scene.encoding.Bounds().ImageRect(), err
	}

	// Convert pixel damage rect to tile coordinates and mark dirty
	tileW := r.tileSize
	tileH := r.tileSize
	tx0 := damage.Min.X / tileW
	ty0 := damage.Min.Y / tileH
	tx1 := (damage.Max.X + tileW - 1) / tileW
	ty1 := (damage.Max.Y + tileH - 1) / tileH

	for ty := ty0; ty < ty1; ty++ {
		for tx := tx0; tx < tx1; tx++ {
			r.dirty.Mark(tx, ty)
		}
	}

	err = r.RenderDirty(target, scene, nil)
	return damage, err
}

// RenderDirtyWithContext renders only the dirty regions of the scene with cancellation support.
// This is more efficient when only parts of the scene have changed.
// The dirty parameter specifies which tiles need re-rendering.
//
// The context can be used to cancel long-running renders. When canceled,
// the function returns ctx.Err() and the target may contain partial results.
func (r *Renderer) RenderDirtyWithContext(ctx context.Context, target *gg.Pixmap, scene *Scene, dirty *parallel.DirtyRegion) error {
	if target == nil || scene == nil {
		return nil
	}

	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	startTotal := time.Now()
	r.statsMu.Lock()
	r.stats = RenderStats{}
	r.statsMu.Unlock()

	// Use provided dirty region or fall back to internal tracking
	dirtyRegion := dirty
	if dirtyRegion == nil {
		dirtyRegion = r.dirty
	}

	// Get the flattened encoding, image registry, and font registry.
	startEncode := time.Now()
	enc := scene.Encoding()
	images := scene.Images()
	r.fontRegistry = scene.FontRegistry()
	encodeTime := time.Since(startEncode)

	// Check for cancellation after encoding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get dirty tile coordinates
	dirtyCoords := dirtyRegion.GetAndClear()
	tilesDirty := len(dirtyCoords)

	if tilesDirty == 0 {
		return nil // Nothing to render
	}

	// Collect dirty tiles
	tiles := make([]*parallel.Tile, 0, tilesDirty)
	for _, coord := range dirtyCoords {
		if tile := r.tileGrid.TileAt(coord[0], coord[1]); tile != nil {
			tiles = append(tiles, tile)
		}
	}

	// Render dirty tiles in parallel with context
	startRaster := time.Now()
	if err := r.renderTilesWithContext(ctx, tiles, enc, target, images); err != nil {
		return err
	}
	rasterTime := time.Since(startRaster)

	// Check for cancellation before compositing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Composite dirty tiles to target
	startComposite := time.Now()
	r.compositeTiles(tiles, target)
	compositeTime := time.Since(startComposite)

	// Update statistics
	totalTime := time.Since(startTotal)
	tilesTotal := r.tileGrid.TileCount()
	r.updateStats(tilesTotal, tilesDirty, len(tiles), encodeTime, rasterTime, compositeTime, totalTime)

	return nil
}

// renderTilesWithContext renders the scene encoding to the specified tiles in parallel
// with cancellation support.
func (r *Renderer) renderTilesWithContext(ctx context.Context, tiles []*parallel.Tile, enc *Encoding, target *gg.Pixmap, images []*Image) error {
	if len(tiles) == 0 || enc == nil || enc.IsEmpty() {
		return nil
	}

	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// For small tile counts, check context less frequently
	checkInterval := 1
	if len(tiles) > 16 {
		checkInterval = len(tiles) / 16
	}

	r.workerPool.ExecuteIndexed(len(tiles), func(i int) {
		// Check for cancellation periodically
		if i%checkInterval == 0 {
			select {
			case <-ctx.Done():
				return // Stop processing on cancellation
			default:
			}
		}
		r.renderTile(tiles[i], enc, target, images)
	})

	// Check if context was canceled during execution
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// renderTile renders the scene encoding to a single tile.
func (r *Renderer) renderTile(tile *parallel.Tile, enc *Encoding, _ *gg.Pixmap, images []*Image) {
	if tile == nil || enc == nil {
		return
	}

	// Calculate tile bounds in canvas space
	tileX, tileY, tileW, tileH := tile.Bounds()
	tileBounds := Rect{
		MinX: float32(tileX),
		MinY: float32(tileY),
		MaxX: float32(tileX + tileW),
		MaxY: float32(tileY + tileH),
	}

	// Check if encoding bounds intersect tile
	encBounds := enc.Bounds()
	if !rectIntersects(encBounds, tileBounds) {
		// Clear tile if nothing to render
		clear(tile.Data)
		return
	}

	// Clear tile before rendering
	clear(tile.Data)

	// Acquire pooled resources for this tile
	pm := r.pool.getPixmap(tileW, tileH)
	sr := r.pool.getRenderer(tileW, tileH)
	dec := r.pool.getDecoder(enc)

	// Render commands using gg.SoftwareRenderer
	r.executeEncodingOnTile(dec, tile, pm, sr, images)

	// Copy rendered pixmap data into the tile buffer.
	// Pixmap stores premultiplied RGBA, same as tile.Data.
	pmData := pm.Data()
	copy(tile.Data, pmData)

	// Return resources to pool
	r.pool.putDecoder(dec)
	r.pool.putPixmap(pm)
	r.pool.putRenderer(sr)

	tile.Dirty = true
}

// tileClipState holds the state for one level of clip nesting during tile rendering.
// Each BeginClip pushes a new state onto the per-tile clip stack; EndClip pops it
// and composites the clipped content back onto the parent pixmap.
type tileClipState struct {
	mask    []byte     // R8 alpha mask (tileW * tileH)
	savedPM *gg.Pixmap // the pixmap we were drawing to before this clip
}

// executeEncodingOnTile executes encoding commands on a single tile, delegating
// rasterization to gg.SoftwareRenderer for analytic anti-aliased output.
//
//nolint:gocyclo,cyclop,gocognit,funlen // Command interpreter with multiple cases is inherently complex
func (r *Renderer) executeEncodingOnTile(dec *Decoder, tile *parallel.Tile, pm *gg.Pixmap, sr *gg.SoftwareRenderer, images []*Image) { //nolint:maintidx // tag dispatch across all scene command types
	// Reusable scene.Path for the decode loop — reset per TagBeginPath instead of allocating.
	currentPath := r.pool.getScenePath()
	defer r.pool.putScenePath(currentPath)
	pathActive := false // true between TagBeginPath and fill/stroke/clip consume

	currentTransform := IdentityAffine()

	tileX, tileY, _, _ := tile.Bounds()

	// activePM is the pixmap we currently render into. It starts as the tile
	// pixmap but may be swapped to a temporary pixmap inside clip regions.
	activePM := pm

	// clipStack tracks nested clip states for BeginClip/EndClip pairs.
	var clipStack []tileClipState

	// Reusable gg.Path for convertPath — avoids per-fill/stroke allocation.
	ggPath := r.pool.getGGPath()
	defer r.pool.putGGPath(ggPath)

	// Reusable Paint for fill operations.
	fillPaint := r.pool.getFillPaint()
	defer r.pool.putFillPaint(fillPaint)

	for dec.Next() {
		switch dec.Tag() {
		case TagTransform:
			currentTransform = dec.Transform()

		case TagBeginPath:
			currentPath.Reset()
			pathActive = true

		case TagMoveTo:
			x, y := dec.MoveTo()
			if pathActive {
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.MoveTo(tx, ty)
			}

		case TagLineTo:
			x, y := dec.LineTo()
			if pathActive {
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.LineTo(tx, ty)
			}

		case TagQuadTo:
			cx, cy, x, y := dec.QuadTo()
			if pathActive {
				tcx, tcy := currentTransform.TransformPoint(cx, cy)
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.QuadTo(tcx, tcy, tx, ty)
			}

		case TagCubicTo:
			c1x, c1y, c2x, c2y, x, y := dec.CubicTo()
			if pathActive {
				tc1x, tc1y := currentTransform.TransformPoint(c1x, c1y)
				tc2x, tc2y := currentTransform.TransformPoint(c2x, c2y)
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.CubicTo(tc1x, tc1y, tc2x, tc2y, tx, ty)
			}

		case TagClosePath:
			if pathActive {
				currentPath.Close()
			}

		case TagEndPath:
			// Path is complete, ready for fill/stroke

		case TagFill:
			brush, style := dec.Fill()
			if pathActive && !currentPath.IsEmpty() {
				convertPathInto(currentPath, tileX, tileY, ggPath)
				resetFillPaint(fillPaint, brush, style)
				_ = sr.Fill(activePM, ggPath, fillPaint)
			}
			pathActive = false

		case TagFillRoundRect:
			brush, _, rect, rx, ry := dec.FillRoundRect()
			renderFillRoundRect(pm, currentTransform, brush, rect, rx, ry, tileX, tileY)

		case TagStroke:
			brush, style := dec.Stroke()
			if pathActive && !currentPath.IsEmpty() {
				convertPathInto(currentPath, tileX, tileY, ggPath)
				paint := convertStrokePaint(brush, style)
				_ = sr.Stroke(activePM, ggPath, paint)
			}
			pathActive = false

		case TagPushLayer:
			// Layer management - skip for now
			_, _ = dec.PushLayer()

		case TagPopLayer:
			// Layer pop - skip for now

		case TagBeginClip:
			tileW := activePM.Width()
			tileH := activePM.Height()

			if pathActive && !currentPath.IsEmpty() {
				// 1. Render clip path as white on a temporary pixmap to get coverage.
				maskPM := r.pool.getPixmap(tileW, tileH)
				convertPathInto(currentPath, tileX, tileY, ggPath)
				resetFillPaint(fillPaint, Brush{Kind: BrushSolid, Color: gg.RGBA{R: 1, G: 1, B: 1, A: 1}}, FillNonZero)
				_ = sr.Fill(maskPM, ggPath, fillPaint)

				// 2. Extract alpha channel as R8 mask (reuse pooled buffer).
				clipMask := r.pool.getClipMask(tileW * tileH)
				extractAlphaMaskInto(maskPM, clipMask)
				r.pool.putPixmap(maskPM)

				// 3. Save current pixmap and allocate a fresh one for clipped content.
				savedPM := activePM
				activePM = r.pool.getPixmap(tileW, tileH)

				// 4. Push clip state.
				clipStack = append(clipStack, tileClipState{mask: clipMask, savedPM: savedPM})
			} else {
				// Empty clip path clips everything (fully transparent mask).
				clipMask := r.pool.getClipMask(tileW * tileH)
				savedPM := activePM
				activePM = r.pool.getPixmap(tileW, tileH)
				clipStack = append(clipStack, tileClipState{mask: clipMask, savedPM: savedPM})
			}
			pathActive = false // consumed by clip

		case TagEndClip:
			if len(clipStack) > 0 {
				state := clipStack[len(clipStack)-1]
				clipStack = clipStack[:len(clipStack)-1]

				// Apply alpha mask to clipped content.
				applyAlphaMask(activePM, state.mask)

				// Composite masked content onto the saved pixmap (source-over).
				compositePixmaps(state.savedPM, activePM)

				// Return the temporary pixmap and clip mask, restore the saved pixmap.
				r.pool.putPixmap(activePM)
				r.pool.putClipMask(state.mask)
				activePM = state.savedPM
			}

		case TagImage:
			imageIndex, imgTransform := dec.Image()
			if int(imageIndex) < len(images) {
				img := images[imageIndex]
				if img != nil && len(img.Data) >= img.Width*img.Height*4 {
					blitImageToTile(img, imgTransform, tileX, tileY, activePM)
				}
			}

		case TagText:
			run, _, str, brush := dec.Text()
			r.renderTextOnTile(run, str, brush, currentTransform, tileX, tileY, activePM, sr, fillPaint)

		case TagBrush:
			// Brush definition - skip for now
			_, _, _, _ = dec.Brush()
		}
	}

	// Safety: if clip stack is not empty (unbalanced clips), composite remaining.
	for len(clipStack) > 0 {
		state := clipStack[len(clipStack)-1]
		clipStack = clipStack[:len(clipStack)-1]
		applyAlphaMask(activePM, state.mask)
		compositePixmaps(state.savedPM, activePM)
		r.pool.putPixmap(activePM)
		r.pool.putClipMask(state.mask)
		activePM = state.savedPM
	}

	// If activePM differs from pm (shouldn't happen with balanced clips, but be safe),
	// copy data back to the original tile pixmap.
	if activePM != pm {
		copy(pm.Data(), activePM.Data())
		r.pool.putPixmap(activePM)
	}
}

// ---------------------------------------------------------------------------
// Clip Helpers (alpha mask compositing)
// ---------------------------------------------------------------------------

// extractAlphaMaskInto extracts the alpha channel from a pixmap into a pre-allocated
// buffer. The buffer must have at least Width*Height bytes. This avoids per-clip
// allocation of the mask buffer.
func extractAlphaMaskInto(pm *gg.Pixmap, mask []byte) {
	data := pm.Data()
	pixelCount := pm.Width() * pm.Height()
	for i := 0; i < pixelCount && i*4+3 < len(data); i++ {
		mask[i] = data[i*4+3] // alpha channel (offset 3 in RGBA)
	}
}

// applyAlphaMask multiplies each pixel's channels by the corresponding mask value.
// Both the pixel data and mask use premultiplied alpha, so all four channels
// (R, G, B, A) are scaled by mask/255.
//
//nolint:gosec // G115: Integer overflow is not possible - math is bounded to [0,255]
func applyAlphaMask(pm *gg.Pixmap, mask []byte) {
	data := pm.Data()
	for i := 0; i < len(mask) && i*4+3 < len(data); i++ {
		m := uint32(mask[i])
		if m == 0 {
			data[i*4] = 0
			data[i*4+1] = 0
			data[i*4+2] = 0
			data[i*4+3] = 0
		} else if m < 255 {
			// Premultiplied alpha: multiply all channels by mask/255.
			// Uses +127 for correct rounding (matches Skia/Cairo convention).
			data[i*4] = uint8((uint32(data[i*4])*m + 127) / 255)
			data[i*4+1] = uint8((uint32(data[i*4+1])*m + 127) / 255)
			data[i*4+2] = uint8((uint32(data[i*4+2])*m + 127) / 255)
			data[i*4+3] = uint8((uint32(data[i*4+3])*m + 127) / 255)
		}
		// m == 255: no change needed
	}
}

// compositePixmaps composites src onto dst using premultiplied source-over.
// Formula: dst' = src + dst * (1 - srcAlpha)
//
//nolint:gosec // G115: Integer overflow is not possible - math is bounded to [0,255]
func compositePixmaps(dst, src *gg.Pixmap) {
	dstData := dst.Data()
	srcData := src.Data()
	n := min(len(dstData), len(srcData))
	for i := 0; i < n; i += 4 {
		sa := srcData[i+3]
		if sa == 0 {
			continue // Fully transparent source, keep destination
		}
		if sa == 255 {
			// Fully opaque source, overwrite destination (fast path)
			dstData[i] = srcData[i]
			dstData[i+1] = srcData[i+1]
			dstData[i+2] = srcData[i+2]
			dstData[i+3] = srcData[i+3]
			continue
		}
		// Premultiplied source-over: dst' = src + dst * (1 - srcAlpha)
		invAlpha := 255 - uint32(sa)
		dstData[i] = srcData[i] + uint8((uint32(dstData[i])*invAlpha+127)/255)
		dstData[i+1] = srcData[i+1] + uint8((uint32(dstData[i+1])*invAlpha+127)/255)
		dstData[i+2] = srcData[i+2] + uint8((uint32(dstData[i+2])*invAlpha+127)/255)
		dstData[i+3] = srcData[i+3] + uint8((uint32(dstData[i+3])*invAlpha+127)/255)
	}
}

// ---------------------------------------------------------------------------
// Path and Paint Conversion (scene types -> gg types)
// ---------------------------------------------------------------------------

// renderTextOnTile renders a TagText glyph run on a CPU tile by extracting
// glyph outlines and rendering each as a filled path. This is the CPU fallback
// for text rendering (software backend, headless). GPU path uses dc.DrawString.
func (r *Renderer) renderTextOnTile(
	run GlyphRunData, str string, brush Brush,
	transform Affine, tileX, tileY int,
	pm *gg.Pixmap, sr *gg.SoftwareRenderer, fillPaint *gg.Paint,
) {
	if str == "" || run.GlyphCount == 0 {
		return
	}

	source := r.fontRegistry[run.FontSourceID]
	if source == nil {
		return
	}

	face := source.Face(float64(run.FontSize))
	shaped := text.Shape(str, face)
	if len(shaped) == 0 {
		return
	}

	textRenderer := NewTextRenderer()
	rendered, err := textRenderer.RenderGlyphs(shaped, face)
	if err != nil || len(rendered) == 0 {
		return
	}

	ggPath := r.pool.getGGPath()
	defer r.pool.putGGPath(ggPath)

	for _, rg := range rendered {
		if rg.Path == nil || rg.Path.IsEmpty() {
			continue
		}

		// Apply text origin offset: glyph positions are relative to (0,0),
		// the run origin (OriginX, OriginY) shifts the entire text block.
		offsetPath := rg.Path.Transform(TranslateAffine(run.OriginX, run.OriginY))

		// Apply scene transform.
		if !transform.IsIdentity() {
			transformedPath := NewPath()
			for _, verb := range offsetPath.Verbs() {
				_ = verb // path iteration handled below
			}
			// Transform each point through the scene transform.
			pointIdx := 0
			pts := offsetPath.Points()
			verbs := offsetPath.Verbs()
			for _, verb := range verbs {
				switch verb {
				case MoveTo:
					tx, ty := transform.TransformPoint(pts[pointIdx], pts[pointIdx+1])
					transformedPath.MoveTo(tx, ty)
					pointIdx += 2
				case LineTo:
					tx, ty := transform.TransformPoint(pts[pointIdx], pts[pointIdx+1])
					transformedPath.LineTo(tx, ty)
					pointIdx += 2
				case QuadTo:
					tcx, tcy := transform.TransformPoint(pts[pointIdx], pts[pointIdx+1])
					tx, ty := transform.TransformPoint(pts[pointIdx+2], pts[pointIdx+3])
					transformedPath.QuadTo(tcx, tcy, tx, ty)
					pointIdx += 4
				case CubicTo:
					tc1x, tc1y := transform.TransformPoint(pts[pointIdx], pts[pointIdx+1])
					tc2x, tc2y := transform.TransformPoint(pts[pointIdx+2], pts[pointIdx+3])
					tx, ty := transform.TransformPoint(pts[pointIdx+4], pts[pointIdx+5])
					transformedPath.CubicTo(tc1x, tc1y, tc2x, tc2y, tx, ty)
					pointIdx += 6
				case Close:
					transformedPath.Close()
				}
			}
			offsetPath = transformedPath
		}

		convertPathInto(offsetPath, tileX, tileY, ggPath)
		resetFillPaint(fillPaint, brush, FillNonZero)
		_ = sr.Fill(pm, ggPath, fillPaint)
	}
}

// renderFillRoundRect renders a filled rounded rectangle using SDF per-pixel
// evaluation directly onto the pixmap, bypassing the path pipeline entirely.
// This is the key performance optimization: no path construction, no edge building,
// no scanline rasterization — just per-pixel SDF coverage with smoothstep AA.
//
//nolint:gosec // G115: Integer conversions are bounded by tile/pixmap dimensions
func renderFillRoundRect(pm *gg.Pixmap, transform Affine, brush Brush, rect Rect, rx, ry float32, tileX, tileY int) {
	if brush.Kind != BrushSolid {
		return // Only solid brushes supported for SDF path
	}

	// Transform the rect corners
	minX, minY := transform.TransformPoint(rect.MinX, rect.MinY)
	maxX, maxY := transform.TransformPoint(rect.MaxX, rect.MaxY)

	// Ensure min < max after transform (handles negative scale)
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	// SDF parameters
	cx := (minX + maxX) / 2
	cy := (minY + maxY) / 2
	halfW := (maxX - minX) / 2
	halfH := (maxY - minY) / 2
	radius := min32(min32(rx, ry), min32(halfW, halfH))

	// Compute pixel bounds within the tile (with 1px margin for AA)
	tileW := pm.Width()
	tileH := pm.Height()
	startX := max(int(minX)-tileX-1, 0)
	startY := max(int(minY)-tileY-1, 0)
	endX := min(int(maxX)-tileX+2, tileW)
	endY := min(int(maxY)-tileY+2, tileH)

	// Brush color components
	br := float32(brush.Color.R)
	bg := float32(brush.Color.G)
	bb := float32(brush.Color.B)
	ba := float32(brush.Color.A)

	pmData := pm.Data()
	stride := tileW * 4

	for py := startY; py < endY; py++ {
		canvasY := float32(py+tileY) + 0.5
		rowOff := py * stride
		for px := startX; px < endX; px++ {
			canvasX := float32(px+tileX) + 0.5
			coverage := sdfRoundRectCoverage(canvasX, canvasY, cx, cy, halfW, halfH, radius)
			if coverage <= 0 {
				continue
			}
			off := rowOff + px*4
			if off+3 >= len(pmData) {
				continue
			}
			blendSDF(pmData, off, br, bg, bb, ba, coverage)
		}
	}
}

// blendSDF blends a source color with coverage onto a destination pixel
// using premultiplied source-over compositing.
func blendSDF(dst []byte, off int, sr, sg, sb, sa, coverage float32) {
	alpha := coverage * sa
	invAlpha := 1.0 - alpha

	dr := float32(dst[off]) / 255.0
	dg := float32(dst[off+1]) / 255.0
	db := float32(dst[off+2]) / 255.0
	da := float32(dst[off+3]) / 255.0

	dst[off] = clampByte((sr*alpha + dr*invAlpha) * 255.0)
	dst[off+1] = clampByte((sg*alpha + dg*invAlpha) * 255.0)
	dst[off+2] = clampByte((sb*alpha + db*invAlpha) * 255.0)
	dst[off+3] = clampByte((alpha + da*invAlpha) * 255.0)
}

// clampByte clamps a float32 to [0, 255] and converts to byte.
func clampByte(v float32) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(v + 0.5)
}

// blitImageToTile composites a scene image onto a tile pixmap using the given
// affine transform. The transform maps image-space coordinates to canvas-space.
// For each destination pixel in the tile, the inverse transform computes the
// corresponding source pixel in the image. This supports translation, scale,
// and rotation in a single codepath (standard inverse-mapping approach used
// by Cairo and Skia).
//
// Image.Data is treated as straight-alpha RGBA; the pixmap uses premultiplied
// alpha. Source pixels are premultiplied during compositing.
//
//nolint:gosec // G115: Integer conversions are bounded by image/tile dimensions
func blitImageToTile(img *Image, transform Affine, tileX, tileY int, pm *gg.Pixmap) {
	tileW := pm.Width()
	tileH := pm.Height()
	pmData := pm.Data()
	stride := tileW * 4

	imgW := img.Width
	imgH := img.Height
	imgData := img.Data
	imgStride := imgW * 4

	// Compute inverse transform: canvas-space -> image-space.
	// For affine [A B C; D E F; 0 0 1], det = A*E - B*D.
	det := transform.A*transform.E - transform.B*transform.D
	if det == 0 {
		return // Degenerate transform
	}
	invDet := 1.0 / det
	inv := Affine{
		A: transform.E * invDet,
		B: -transform.B * invDet,
		C: (transform.B*transform.F - transform.E*transform.C) * invDet,
		D: -transform.D * invDet,
		E: transform.A * invDet,
		F: (transform.D*transform.C - transform.A*transform.F) * invDet,
	}

	// Compute the image bounding box in canvas space to limit iteration.
	corners := [4][2]float32{
		{0, 0},
		{float32(imgW), 0},
		{float32(imgW), float32(imgH)},
		{0, float32(imgH)},
	}
	bboxMinX, bboxMinY := corners[0][0], corners[0][1]
	bboxMaxX, bboxMaxY := bboxMinX, bboxMinY
	for _, c := range corners {
		cx, cy := transform.TransformPoint(c[0], c[1])
		bboxMinX = min32(bboxMinX, cx)
		bboxMinY = min32(bboxMinY, cy)
		bboxMaxX = max32(bboxMaxX, cx)
		bboxMaxY = max32(bboxMaxY, cy)
	}

	// Clip to tile bounds (tile-local coordinates).
	startX := max(int(bboxMinX)-tileX, 0)
	startY := max(int(bboxMinY)-tileY, 0)
	endX := min(int(bboxMaxX)-tileX+1, tileW)
	endY := min(int(bboxMaxY)-tileY+1, tileH)

	for py := startY; py < endY; py++ {
		canvasY := float32(py+tileY) + 0.5
		rowOff := py * stride
		for px := startX; px < endX; px++ {
			canvasX := float32(px+tileX) + 0.5

			// Map canvas pixel center back to image space.
			srcX, srcY := inv.TransformPoint(canvasX, canvasY)
			ix := int(srcX)
			iy := int(srcY)
			if ix < 0 || iy < 0 || ix >= imgW || iy >= imgH {
				continue
			}

			srcOff := iy*imgStride + ix*4
			sa := imgData[srcOff+3]
			if sa == 0 {
				continue
			}

			dstOff := rowOff + px*4
			if dstOff+3 >= len(pmData) {
				continue
			}

			// Source is straight alpha — premultiply for compositing.
			if sa == 255 {
				// Fully opaque: overwrite (premultiplied == straight when A=255).
				pmData[dstOff] = imgData[srcOff]
				pmData[dstOff+1] = imgData[srcOff+1]
				pmData[dstOff+2] = imgData[srcOff+2]
				pmData[dstOff+3] = 255
			} else {
				// Premultiply source: pR = R * A / 255
				srcA := uint32(sa)
				pR := uint8((uint32(imgData[srcOff])*srcA + 127) / 255)
				pG := uint8((uint32(imgData[srcOff+1])*srcA + 127) / 255)
				pB := uint8((uint32(imgData[srcOff+2])*srcA + 127) / 255)

				// Source-over: dst' = src + dst * (1 - srcAlpha)
				invAlpha := 255 - srcA
				pmData[dstOff] = pR + uint8((uint32(pmData[dstOff])*invAlpha+127)/255)
				pmData[dstOff+1] = pG + uint8((uint32(pmData[dstOff+1])*invAlpha+127)/255)
				pmData[dstOff+2] = pB + uint8((uint32(pmData[dstOff+2])*invAlpha+127)/255)
				pmData[dstOff+3] = sa + uint8((uint32(pmData[dstOff+3])*invAlpha+127)/255)
			}
		}
	}
}

// convertPathInto converts a scene.Path (float32, canvas space) into an existing
// gg.Path (float64, tile-local space), avoiding allocation. The gg.Path is cleared
// first, then populated with the scene path data offset by the tile origin.
func convertPathInto(scenePath *Path, tileOffsetX, tileOffsetY int, p *gg.Path) {
	p.Clear()
	ox := float64(tileOffsetX)
	oy := float64(tileOffsetY)

	pointIdx := 0
	pts := scenePath.Points()
	for _, verb := range scenePath.Verbs() {
		switch verb {
		case MoveTo:
			p.MoveTo(float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy)
			pointIdx += 2
		case LineTo:
			p.LineTo(float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy)
			pointIdx += 2
		case QuadTo:
			p.QuadraticTo(
				float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy,
				float64(pts[pointIdx+2])-ox, float64(pts[pointIdx+3])-oy,
			)
			pointIdx += 4
		case CubicTo:
			p.CubicTo(
				float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy,
				float64(pts[pointIdx+2])-ox, float64(pts[pointIdx+3])-oy,
				float64(pts[pointIdx+4])-ox, float64(pts[pointIdx+5])-oy,
			)
			pointIdx += 6
		case Close:
			p.Close()
		}
	}
}

// resetFillPaint resets a Paint for fill use with the given brush and style.
// This avoids allocating a new Paint per fill command.
func resetFillPaint(paint *gg.Paint, brush Brush, style FillStyle) {
	if brush.Kind == BrushSolid {
		paint.SetBrush(gg.Solid(brush.Color))
	}
	if style == FillEvenOdd {
		paint.FillRule = gg.FillRuleEvenOdd
	} else {
		paint.FillRule = gg.FillRuleNonZero
	}
	paint.Stroke = nil
}

// convertPath converts a scene.Path to a new gg.Path. This allocates a new gg.Path
// and is kept for compatibility. The renderer hot path uses convertPathInto instead.
func convertPath(scenePath *Path, tileOffsetX, tileOffsetY int) *gg.Path {
	p := gg.NewPath()
	convertPathInto(scenePath, tileOffsetX, tileOffsetY, p)
	return p
}

// convertFillPaint converts a scene.Brush and FillStyle to a new gg.Paint.
// This allocates a new Paint and is kept for compatibility. The renderer hot path
// uses resetFillPaint instead.
func convertFillPaint(brush Brush, style FillStyle) *gg.Paint {
	paint := gg.NewPaint()
	resetFillPaint(paint, brush, style)
	return paint
}

// extractAlphaMask extracts the alpha channel from a pixmap as a new byte slice.
// This allocates a new buffer and is kept for compatibility. The renderer hot path
// uses extractAlphaMaskInto with pooled buffers instead.
func extractAlphaMask(pm *gg.Pixmap) []byte {
	mask := make([]byte, pm.Width()*pm.Height())
	extractAlphaMaskInto(pm, mask)
	return mask
}

// convertStrokePaint converts a scene.Brush and StrokeStyle to a gg.Paint
// suitable for gg.SoftwareRenderer.Stroke.
func convertStrokePaint(brush Brush, style *StrokeStyle) *gg.Paint {
	if style == nil {
		style = DefaultStrokeStyle()
	}

	paint := gg.NewPaint()
	if brush.Kind == BrushSolid {
		paint.SetBrush(gg.Solid(brush.Color))
	}

	// Build a gg.Stroke using the non-deprecated API
	s := gg.Stroke{
		Width:      float64(style.Width),
		MiterLimit: float64(style.MiterLimit),
	}

	switch style.Cap {
	case LineCapButt:
		s.Cap = gg.LineCapButt
	case LineCapRound:
		s.Cap = gg.LineCapRound
	case LineCapSquare:
		s.Cap = gg.LineCapSquare
	}

	switch style.Join {
	case LineJoinMiter:
		s.Join = gg.LineJoinMiter
	case LineJoinRound:
		s.Join = gg.LineJoinRound
	case LineJoinBevel:
		s.Join = gg.LineJoinBevel
	}

	paint.SetStroke(s)
	return paint
}

// compositeTiles copies rendered tiles to the target pixmap.
func (r *Renderer) compositeTiles(tiles []*parallel.Tile, target *gg.Pixmap) {
	if len(tiles) == 0 || target == nil {
		return
	}

	targetData := target.Data()
	stride := target.Width() * 4

	r.workerPool.ExecuteIndexed(len(tiles), func(i int) {
		r.compositeTile(tiles[i], targetData, stride)
	})
}

// compositeTile blends a single tile's data onto the target buffer using
// premultiplied source-over alpha compositing. This preserves the user's
// pre-existing background (e.g. a Clear(white) call) instead of overwriting it.
//
//nolint:gosec // G115: Integer overflow is not possible - math is bounded to [0,255]
func (r *Renderer) compositeTile(tile *parallel.Tile, dst []byte, dstStride int) {
	tileX, tileY, _, _ := tile.Bounds()

	srcStride := tile.Width * 4

	for row := 0; row < tile.Height; row++ {
		canvasY := tileY + row
		if canvasY >= r.height {
			break
		}

		dstRowStart := canvasY*dstStride + tileX*4
		srcRowStart := row * srcStride

		// Determine number of pixels to process in this row
		pixelsInRow := tile.Width
		if tileX+pixelsInRow > r.width {
			pixelsInRow = r.width - tileX
		}
		if pixelsInRow <= 0 {
			continue
		}

		for col := 0; col < pixelsInRow; col++ {
			srcOff := srcRowStart + col*4
			dstOff := dstRowStart + col*4

			if srcOff+3 >= len(tile.Data) || dstOff+3 >= len(dst) {
				continue
			}

			sa := tile.Data[srcOff+3]
			if sa == 0 {
				continue // Fully transparent source pixel, keep destination
			}
			if sa == 255 {
				// Fully opaque source, overwrite destination (fast path)
				dst[dstOff] = tile.Data[srcOff]
				dst[dstOff+1] = tile.Data[srcOff+1]
				dst[dstOff+2] = tile.Data[srcOff+2]
				dst[dstOff+3] = tile.Data[srcOff+3]
				continue
			}

			// Premultiplied source-over: dst' = src + dst * (1 - srcAlpha)
			invAlpha := 255 - uint32(sa)
			dst[dstOff] = tile.Data[srcOff] + uint8((uint32(dst[dstOff])*invAlpha+127)/255)
			dst[dstOff+1] = tile.Data[srcOff+1] + uint8((uint32(dst[dstOff+1])*invAlpha+127)/255)
			dst[dstOff+2] = tile.Data[srcOff+2] + uint8((uint32(dst[dstOff+2])*invAlpha+127)/255)
			dst[dstOff+3] = tile.Data[srcOff+3] + uint8((uint32(dst[dstOff+3])*invAlpha+127)/255)
		}
	}
}

// updateStats updates the render statistics.
func (r *Renderer) updateStats(total, dirty, rendered int, encode, raster, composite, totalTime time.Duration) {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TilesTotal = total
	r.stats.TilesDirty = dirty
	r.stats.TilesRendered = rendered
	r.stats.TimeEncode = encode
	r.stats.TimeRaster = raster
	r.stats.TimeComposite = composite
	r.stats.TimeTotal = totalTime

	// Calculate frame timing
	if !r.lastFrame.IsZero() {
		r.stats.FrameTime = time.Since(r.lastFrame)
		if r.stats.FrameTime > 0 {
			r.stats.FPS = float64(time.Second) / float64(r.stats.FrameTime)
		}
	}
	r.lastFrame = time.Now()
}

// Resize updates the renderer dimensions.
// All tiles will be marked dirty after resize.
func (r *Renderer) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}

	if r.width == width && r.height == height {
		return
	}

	r.width = width
	r.height = height

	// Resize tile grid
	r.tileGrid.Resize(width, height)

	// Resize dirty region
	tilesX := (width + parallel.TileWidth - 1) / parallel.TileWidth
	tilesY := (height + parallel.TileHeight - 1) / parallel.TileHeight
	r.dirty = parallel.NewDirtyRegion(tilesX, tilesY)
	r.dirty.MarkAll()
}

// Stats returns the current render statistics.
func (r *Renderer) Stats() RenderStats {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()
	return r.stats
}

// CacheStats returns the layer cache statistics.
func (r *Renderer) CacheStats() CacheStats {
	if r.cache == nil {
		return CacheStats{}
	}
	return r.cache.Stats()
}

// MarkDirty marks the specified rectangle as needing redraw.
// Coordinates are in pixel space.
func (r *Renderer) MarkDirty(x, y, w, h int) {
	if r.dirty != nil {
		r.dirty.MarkRect(x, y, w, h)
	}
	r.tileGrid.MarkRectDirty(x, y, w, h)
}

// MarkAllDirty marks all tiles as needing redraw.
func (r *Renderer) MarkAllDirty() {
	if r.dirty != nil {
		r.dirty.MarkAll()
	}
	r.tileGrid.MarkAllDirty()
}

// DirtyTileCount returns the number of dirty tiles.
func (r *Renderer) DirtyTileCount() int {
	if r.dirty != nil {
		return r.dirty.Count()
	}
	return len(r.tileGrid.DirtyTiles())
}

// Width returns the renderer width in pixels.
func (r *Renderer) Width() int {
	return r.width
}

// Height returns the renderer height in pixels.
func (r *Renderer) Height() int {
	return r.height
}

// TileCount returns the total number of tiles.
func (r *Renderer) TileCount() int {
	return r.tileGrid.TileCount()
}

// Cache returns the layer cache.
func (r *Renderer) Cache() *LayerCache {
	return r.cache
}

// Workers returns the number of worker goroutines used for parallel rendering.
func (r *Renderer) Workers() int {
	return r.workers
}

// Close releases all resources used by the renderer.
// The renderer should not be used after Close is called.
func (r *Renderer) Close() {
	if r.workerPool != nil {
		r.workerPool.Close()
	}
	if r.tileGrid != nil {
		r.tileGrid.Close()
	}
}

// ---------------------------------------------------------------------------
// Utility Functions
// ---------------------------------------------------------------------------

// rectIntersects returns true if two rectangles intersect.
func rectIntersects(a, b Rect) bool {
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}
	return !(a.MaxX < b.MinX || a.MinX > b.MaxX ||
		a.MaxY < b.MinY || a.MinY > b.MaxY)
}
