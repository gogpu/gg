package scene

import (
	"runtime"
	"sync"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/parallel"
)

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
func (r *Renderer) Render(target *gg.Pixmap, scene *Scene) error {
	if target == nil || scene == nil {
		return nil
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

	// Get all tiles for rendering
	tiles := r.tileGrid.AllTiles()
	tilesTotal := len(tiles)

	// Render tiles in parallel
	startRaster := time.Now()
	r.renderTiles(tiles, enc, target)
	rasterTime := time.Since(startRaster)

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
func (r *Renderer) RenderDirty(target *gg.Pixmap, scene *Scene, dirty *parallel.DirtyRegion) error {
	if target == nil || scene == nil {
		return nil
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

	// Render dirty tiles in parallel
	startRaster := time.Now()
	r.renderTiles(tiles, enc, target)
	rasterTime := time.Since(startRaster)

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

// renderTiles renders the scene encoding to the specified tiles in parallel.
func (r *Renderer) renderTiles(tiles []*parallel.Tile, enc *Encoding, target *gg.Pixmap) {
	if len(tiles) == 0 || enc == nil || enc.IsEmpty() {
		return
	}

	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			r.renderTile(t, enc, target)
		}
	}

	r.workerPool.ExecuteAll(work)
}

// renderTile renders the scene encoding to a single tile.
func (r *Renderer) renderTile(tile *parallel.Tile, enc *Encoding, target *gg.Pixmap) {
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

	// Create decoder and render commands
	dec := NewDecoder(enc)
	r.executeEncodingOnTile(dec, tile, tileBounds, target)

	tile.Dirty = true
}

// executeEncodingOnTile executes encoding commands on a single tile.
//
//nolint:gocyclo,cyclop // Command interpreter with multiple cases is inherently complex
func (r *Renderer) executeEncodingOnTile(dec *Decoder, tile *parallel.Tile, tileBounds Rect, _ *gg.Pixmap) {
	var currentPath *Path
	currentTransform := IdentityAffine()

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
				r.fillPathOnTile(tile, tileBounds, currentPath, brush, style)
			}

		case TagStroke:
			brush, style := dec.Stroke()
			if currentPath != nil && !currentPath.IsEmpty() {
				r.strokePathOnTile(tile, tileBounds, currentPath, brush, style)
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

// fillPathOnTile fills a path on a tile using simple scanline rasterization.
func (r *Renderer) fillPathOnTile(tile *parallel.Tile, tileBounds Rect, path *Path, brush Brush, _ FillStyle) {
	if path.IsEmpty() || brush.Kind != BrushSolid {
		return
	}

	// Get path bounds
	pathBounds := path.Bounds()
	if !rectIntersects(pathBounds, tileBounds) {
		return
	}

	// Convert color to bytes
	color := brush.Color
	r8 := uint8(clamp01(color.R) * 255)
	g8 := uint8(clamp01(color.G) * 255)
	b8 := uint8(clamp01(color.B) * 255)
	a8 := uint8(clamp01(color.A) * 255)

	// Simple bounding box fill for demonstration
	// Full implementation would use scanline rasterization
	tileX, tileY, _, _ := tile.Bounds()

	// Calculate intersection of path bounds with tile
	minX := int(pathBounds.MinX) - tileX
	minY := int(pathBounds.MinY) - tileY
	maxX := int(pathBounds.MaxX) - tileX
	maxY := int(pathBounds.MaxY) - tileY

	// Clamp to tile bounds
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > tile.Width {
		maxX = tile.Width
	}
	if maxY > tile.Height {
		maxY = tile.Height
	}

	// Fill pixels (simplified - should use actual path containment)
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			// Check if point is inside path using winding number
			px := float32(tileX+x) + 0.5
			py := float32(tileY+y) + 0.5

			if path.Contains(px, py) {
				offset := tile.PixelOffset(x, y)
				blendPixel(tile.Data, offset, r8, g8, b8, a8)
			}
		}
	}
}

// strokePathOnTile strokes a path on a tile.
func (r *Renderer) strokePathOnTile(tile *parallel.Tile, tileBounds Rect, path *Path, brush Brush, style *StrokeStyle) {
	if path.IsEmpty() || brush.Kind != BrushSolid {
		return
	}

	// Get expanded path bounds for stroke width
	pathBounds := path.Bounds()
	halfWidth := style.Width / 2
	pathBounds.MinX -= halfWidth
	pathBounds.MinY -= halfWidth
	pathBounds.MaxX += halfWidth
	pathBounds.MaxY += halfWidth

	if !rectIntersects(pathBounds, tileBounds) {
		return
	}

	// Convert color to bytes
	color := brush.Color
	r8 := uint8(clamp01(color.R) * 255)
	g8 := uint8(clamp01(color.G) * 255)
	b8 := uint8(clamp01(color.B) * 255)
	a8 := uint8(clamp01(color.A) * 255)

	tileX, tileY, _, _ := tile.Bounds()

	// Iterate through path segments and draw lines
	var lastX, lastY float32
	var lastValid bool

	for i := 0; i < len(path.verbs); i++ {
		verb := path.verbs[i]

		switch verb {
		case VerbMoveTo:
			if i*2+1 < len(path.points) {
				lastX = path.points[i*2]
				lastY = path.points[i*2+1]
				lastValid = true
			}

		case VerbLineTo:
			if lastValid && i*2+1 < len(path.points) {
				x := path.points[i*2]
				y := path.points[i*2+1]
				r.drawLineOnTile(tile, tileX, tileY, lastX, lastY, x, y, style.Width, r8, g8, b8, a8)
				lastX, lastY = x, y
			}

		case VerbClose:
			// Draw line back to subpath start if needed
		}
	}
}

// drawLineOnTile draws a line segment on a tile with the given stroke width.
func (r *Renderer) drawLineOnTile(tile *parallel.Tile, tileX, tileY int, x1, y1, x2, y2, width float32, red, green, blue, alpha uint8) {
	// Simple Bresenham-style line drawing for demonstration
	// Full implementation would use proper thick line rasterization

	dx := x2 - x1
	dy := y2 - y1

	// Number of steps based on the longer dimension
	steps := int(abs32(dx))
	if int(abs32(dy)) > steps {
		steps = int(abs32(dy))
	}
	if steps == 0 {
		steps = 1
	}

	xInc := dx / float32(steps)
	yInc := dy / float32(steps)

	x := x1
	y := y1
	halfWidth := width / 2

	for i := 0; i <= steps; i++ {
		// Draw a small area around the point for thick lines
		for offsetY := -int(halfWidth); offsetY <= int(halfWidth); offsetY++ {
			for offsetX := -int(halfWidth); offsetX <= int(halfWidth); offsetX++ {
				px := int(x+0.5) + offsetX - tileX
				py := int(y+0.5) + offsetY - tileY

				if px >= 0 && px < tile.Width && py >= 0 && py < tile.Height {
					offset := tile.PixelOffset(px, py)
					blendPixel(tile.Data, offset, red, green, blue, alpha)
				}
			}
		}

		x += xInc
		y += yInc
	}
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

// compositeTile copies a single tile's data to the target buffer.
func (r *Renderer) compositeTile(tile *parallel.Tile, dst []byte, dstStride int) {
	tileX, tileY, _, _ := tile.Bounds()

	srcStride := tile.Width * 4

	for row := 0; row < tile.Height; row++ {
		canvasY := tileY + row
		if canvasY >= r.height {
			break
		}

		dstOffset := canvasY*dstStride + tileX*4
		srcOffset := row * srcStride

		copyLen := srcStride
		if tileX*4+copyLen > dstStride {
			copyLen = dstStride - tileX*4
		}
		if copyLen <= 0 {
			continue
		}

		if dstOffset+copyLen > len(dst) || srcOffset+copyLen > len(tile.Data) {
			continue
		}

		copy(dst[dstOffset:dstOffset+copyLen], tile.Data[srcOffset:srcOffset+copyLen])
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

// clamp01 clamps a value to the range [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// abs32 returns the absolute value of a float32.
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// blendPixel blends source color onto destination using source-over blending.
// The offset is the byte offset into the tile data, and src is RGBA bytes.
//
//nolint:gosec // G115: Integer overflow is not possible here - math is bounded to [0,255]
func blendPixel(data []byte, offset int, sr, sg, sb, sa uint8) {
	if offset < 0 || offset+3 >= len(data) {
		return
	}
	if sa == 255 {
		data[offset] = sr
		data[offset+1] = sg
		data[offset+2] = sb
		data[offset+3] = sa
	} else if sa > 0 {
		invAlpha := 255 - sa
		data[offset] = sr + uint8((uint32(data[offset])*uint32(invAlpha)+127)/255)
		data[offset+1] = sg + uint8((uint32(data[offset+1])*uint32(invAlpha)+127)/255)
		data[offset+2] = sb + uint8((uint32(data[offset+2])*uint32(invAlpha)+127)/255)
		data[offset+3] = sa + uint8((uint32(data[offset+3])*uint32(invAlpha)+127)/255)
	}
}
