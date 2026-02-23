package scene

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/parallel"
)

// tilePool manages pooled resources for per-tile rendering.
// SoftwareRenderers and Pixmaps are expensive to allocate, so we reuse them
// across tiles and frames via sync.Pool.
type tilePool struct {
	renderers sync.Pool // *gg.SoftwareRenderer
	pixmaps   sync.Pool // *gg.Pixmap
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

	startTotal := time.Now()
	r.statsMu.Lock()
	r.stats = RenderStats{} // Reset stats
	r.statsMu.Unlock()

	// Mark all tiles dirty for full render
	r.dirty.MarkAll()

	// Get the flattened encoding
	startEncode := time.Now()
	enc := scene.Encoding()
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
	if err := r.renderTilesWithContext(ctx, tiles, enc, target); err != nil {
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

	// Get the flattened encoding
	startEncode := time.Now()
	enc := scene.Encoding()
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
	if err := r.renderTilesWithContext(ctx, tiles, enc, target); err != nil {
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
func (r *Renderer) renderTilesWithContext(ctx context.Context, tiles []*parallel.Tile, enc *Encoding, target *gg.Pixmap) error {
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

	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		idx := i
		work[i] = func() {
			// Check for cancellation periodically
			if idx%checkInterval == 0 {
				select {
				case <-ctx.Done():
					return // Stop processing on cancellation
				default:
				}
			}
			r.renderTile(t, enc, target)
		}
	}

	r.workerPool.ExecuteAll(work)

	// Check if context was canceled during execution
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// renderTile renders the scene encoding to a single tile.
func (r *Renderer) renderTile(tile *parallel.Tile, enc *Encoding, _ *gg.Pixmap) {
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

	// Create decoder and render commands using gg.SoftwareRenderer
	dec := NewDecoder(enc)
	r.executeEncodingOnTile(dec, tile, pm, sr)

	// Copy rendered pixmap data into the tile buffer.
	// Pixmap stores premultiplied RGBA, same as tile.Data.
	pmData := pm.Data()
	copy(tile.Data, pmData)

	// Return resources to pool
	r.pool.putPixmap(pm)
	r.pool.putRenderer(sr)

	tile.Dirty = true
}

// executeEncodingOnTile executes encoding commands on a single tile, delegating
// rasterization to gg.SoftwareRenderer for analytic anti-aliased output.
//
//nolint:gocyclo,cyclop // Command interpreter with multiple cases is inherently complex
func (r *Renderer) executeEncodingOnTile(dec *Decoder, tile *parallel.Tile, pm *gg.Pixmap, sr *gg.SoftwareRenderer) {
	var currentPath *Path
	currentTransform := IdentityAffine()

	tileX, tileY, _, _ := tile.Bounds()

	for dec.Next() {
		switch dec.Tag() {
		case TagTransform:
			currentTransform = dec.Transform()

		case TagBeginPath:
			currentPath = NewPath()

		case TagMoveTo:
			x, y := dec.MoveTo()
			if currentPath != nil {
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.MoveTo(tx, ty)
			}

		case TagLineTo:
			x, y := dec.LineTo()
			if currentPath != nil {
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.LineTo(tx, ty)
			}

		case TagQuadTo:
			cx, cy, x, y := dec.QuadTo()
			if currentPath != nil {
				tcx, tcy := currentTransform.TransformPoint(cx, cy)
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.QuadTo(tcx, tcy, tx, ty)
			}

		case TagCubicTo:
			c1x, c1y, c2x, c2y, x, y := dec.CubicTo()
			if currentPath != nil {
				tc1x, tc1y := currentTransform.TransformPoint(c1x, c1y)
				tc2x, tc2y := currentTransform.TransformPoint(c2x, c2y)
				tx, ty := currentTransform.TransformPoint(x, y)
				currentPath.CubicTo(tc1x, tc1y, tc2x, tc2y, tx, ty)
			}

		case TagClosePath:
			if currentPath != nil {
				currentPath.Close()
			}

		case TagEndPath:
			// Path is complete, ready for fill/stroke

		case TagFill:
			brush, style := dec.Fill()
			if currentPath != nil && !currentPath.IsEmpty() {
				ggPath := convertPath(currentPath, tileX, tileY)
				paint := convertFillPaint(brush, style)
				_ = sr.Fill(pm, ggPath, paint)
			}

		case TagStroke:
			brush, style := dec.Stroke()
			if currentPath != nil && !currentPath.IsEmpty() {
				ggPath := convertPath(currentPath, tileX, tileY)
				paint := convertStrokePaint(brush, style)
				_ = sr.Stroke(pm, ggPath, paint)
			}

		case TagPushLayer:
			// Layer management - skip for now
			_, _ = dec.PushLayer()

		case TagPopLayer:
			// Layer pop - skip for now

		case TagBeginClip:
			// Clip management - skip for now

		case TagEndClip:
			// Clip end - skip for now

		case TagImage:
			// Image rendering - skip for now
			_, _ = dec.Image()

		case TagBrush:
			// Brush definition - skip for now
			_, _, _, _ = dec.Brush()
		}
	}
}

// ---------------------------------------------------------------------------
// Path and Paint Conversion (scene types -> gg types)
// ---------------------------------------------------------------------------

// convertPath converts a scene.Path (float32, canvas space) to a gg.Path
// (float64, tile-local space) by subtracting the tile origin offset.
// The scene.Path is already in transformed canvas coordinates.
func convertPath(scenePath *Path, tileOffsetX, tileOffsetY int) *gg.Path {
	p := gg.NewPath()
	ox := float64(tileOffsetX)
	oy := float64(tileOffsetY)

	pointIdx := 0
	pts := scenePath.Points()
	for _, verb := range scenePath.Verbs() {
		switch verb {
		case VerbMoveTo:
			p.MoveTo(float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy)
			pointIdx += 2
		case VerbLineTo:
			p.LineTo(float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy)
			pointIdx += 2
		case VerbQuadTo:
			p.QuadraticTo(
				float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy,
				float64(pts[pointIdx+2])-ox, float64(pts[pointIdx+3])-oy,
			)
			pointIdx += 4
		case VerbCubicTo:
			p.CubicTo(
				float64(pts[pointIdx])-ox, float64(pts[pointIdx+1])-oy,
				float64(pts[pointIdx+2])-ox, float64(pts[pointIdx+3])-oy,
				float64(pts[pointIdx+4])-ox, float64(pts[pointIdx+5])-oy,
			)
			pointIdx += 6
		case VerbClose:
			p.Close()
		}
	}
	return p
}

// convertFillPaint converts a scene.Brush and FillStyle to a gg.Paint
// suitable for gg.SoftwareRenderer.Fill.
func convertFillPaint(brush Brush, style FillStyle) *gg.Paint {
	paint := gg.NewPaint()
	if brush.Kind == BrushSolid {
		paint.SetBrush(gg.Solid(brush.Color))
	}
	if style == FillEvenOdd {
		paint.FillRule = gg.FillRuleEvenOdd
	}
	return paint
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

	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			r.compositeTile(t, targetData, stride)
		}
	}

	r.workerPool.ExecuteAll(work)
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
