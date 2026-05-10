package text

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// GlyphMaskKey uniquely identifies a rasterized glyph mask in the atlas.
// The key captures all parameters that affect the rendered appearance:
// font identity, glyph index, pixel size (quantized to 1/16 px), and
// subpixel position (quantized to 1/4 px).
//
// This follows the Skia/Chrome pattern where each (font, glyph, size, subpixel)
// combination produces a distinct alpha mask.
type GlyphMaskKey struct {
	// FontID identifies the font (hash of font data or name).
	FontID uint64

	// GlyphID is the glyph index within the font.
	GlyphID uint16

	// SizeQ4 is the font size multiplied by 16, giving 1/16 pixel precision.
	// For example, 13px = 208, 14.5px = 232.
	// Range: 1..32767 covers sizes 0.0625..2048 px.
	SizeQ4 int16

	// SubpixelXQ2 is the fractional X position multiplied by 4 (0..3).
	// This gives 4 horizontal subpixel variants per glyph (1/4 pixel positioning).
	SubpixelXQ2 uint8

	// SubpixelYQ2 is the fractional Y position multiplied by 4 (0..3).
	// Typically 0 for horizontal text (no vertical subpixel positioning).
	SubpixelYQ2 uint8
}

// MakeGlyphMaskKey creates a GlyphMaskKey from rendering parameters.
// The size is in pixels (ppem). The subpixelX/Y are fractional pixel offsets [0, 1).
func MakeGlyphMaskKey(fontID uint64, glyphID GlyphID, size float64, subpixelX, subpixelY float64) GlyphMaskKey {
	// Quantize size to 1/16 pixel (Q4 fixed-point).
	sizeQ4 := int16(size * 16) //nolint:gosec // intentional truncation for quantization
	sizeQ4 = max(sizeQ4, 1)

	// Quantize subpixel position to 1/4 pixel (Q2 fixed-point).
	// Clamp to [0, 0.75] range (4 variants: 0, 0.25, 0.5, 0.75).
	spxQ2 := uint8(subpixelX * 4) //nolint:gosec // fractional [0,1) * 4 fits uint8
	spyQ2 := uint8(subpixelY * 4) //nolint:gosec // fractional [0,1) * 4 fits uint8
	if spxQ2 > 3 {
		spxQ2 = 3
	}
	if spyQ2 > 3 {
		spyQ2 = 3
	}

	return GlyphMaskKey{
		FontID:      fontID,
		GlyphID:     uint16(glyphID), //nolint:gosec // GlyphID is uint16
		SizeQ4:      sizeQ4,
		SubpixelXQ2: spxQ2,
		SubpixelYQ2: spyQ2,
	}
}

// sizeBuckets defines discrete rasterization sizes for zoom resilience (Skia pattern).
// During zoom, fractional sizes snap to the nearest bucket instead of creating
// a new atlas entry per 1/16px increment. Without buckets: 14px→15px = 16 sizes.
// With buckets: 14px→48px = 4 sizes total. Eliminates atlas overflow under zoom.
//
// Reference: Skia SubRunControl.cpp:28-35 (kSmallDFFontLimit/kMediumDFFontLimit/kLargeDFFontLimit)
var sizeBuckets = [...]float64{16, 24, 32, 48}

// MakeGlyphMaskKeyBucketed creates a GlyphMaskKey with size snapped to discrete
// buckets. Use this when atlas pressure is detected (zoom scenarios with many
// unique sizes). The GPU scales the rasterized glyph from bucket size to actual
// size — quality loss is negligible (Skia uses this for all SDF text in Chrome).
func MakeGlyphMaskKeyBucketed(fontID uint64, glyphID GlyphID, size float64, subpixelX, subpixelY float64) GlyphMaskKey {
	bucketSize := sizeBuckets[len(sizeBuckets)-1]
	for _, b := range sizeBuckets {
		if size <= b {
			bucketSize = b
			break
		}
	}
	return MakeGlyphMaskKey(fontID, glyphID, bucketSize, subpixelX, subpixelY)
}

// GlyphMaskRegion describes a glyph mask's location and metrics in the atlas.
type GlyphMaskRegion struct {
	// AtlasIndex indicates which atlas page this glyph is in.
	AtlasIndex int

	// X, Y are the pixel coordinates of the mask in the atlas.
	X, Y int

	// Width, Height are the dimensions of the mask in pixels.
	Width, Height int

	// BearingX is the horizontal offset from the glyph origin to the left edge
	// of the mask, in pixels. Used to position the quad correctly.
	BearingX float32

	// BearingY is the vertical offset from the baseline to the top edge of the
	// mask, in pixels. Positive = above baseline (standard glyph rendering).
	BearingY float32

	// UV coordinates [0, 1] for texture sampling.
	// Inset by 0.5 texels to prevent bilinear bleed.
	U0, V0, U1, V1 float32

	// IsLCD indicates that this region contains LCD subpixel data stored
	// at 3x horizontal width in the R8 atlas. Each logical pixel occupies
	// 3 consecutive R8 texels (R, G, B coverage). The Width field stores
	// the atlas width (3 * logical width), not the logical pixel width.
	IsLCD bool
}

// GlyphMaskAtlasConfig holds configuration for the glyph mask atlas.
type GlyphMaskAtlasConfig struct {
	// Size is the atlas texture size (width = height).
	// Must be power of 2. Default: 1024.
	Size int

	// Padding between glyphs to prevent texture bleeding.
	// Default: 1.
	Padding int

	// MaxAtlases limits the number of atlas pages.
	// Default: 4.
	MaxAtlases int

	// MaxEntries is the maximum number of cached glyph masks.
	// When exceeded, LRU eviction removes the least recently used entries.
	// Default: 8192.
	MaxEntries int
}

// DefaultGlyphMaskAtlasConfig returns the default configuration.
func DefaultGlyphMaskAtlasConfig() GlyphMaskAtlasConfig {
	return GlyphMaskAtlasConfig{
		Size:       1024,
		Padding:    1,
		MaxAtlases: 4,
		MaxEntries: 16384, // ADR-027: increased for CJK (20K+ glyphs × subpixel variants)
	}
}

// Validate checks if the configuration is valid.
func (c *GlyphMaskAtlasConfig) Validate() error {
	if c.Size < 64 {
		return errors.New("text: glyph mask atlas size must be at least 64")
	}
	if c.Size > 8192 {
		return errors.New("text: glyph mask atlas size must be at most 8192")
	}
	if c.Size&(c.Size-1) != 0 {
		return errors.New("text: glyph mask atlas size must be power of 2")
	}
	if c.Padding < 0 {
		return errors.New("text: glyph mask atlas padding must be non-negative")
	}
	if c.MaxAtlases < 1 {
		return errors.New("text: glyph mask atlas must have at least 1 page")
	}
	if c.MaxEntries < 1 {
		return errors.New("text: glyph mask atlas must allow at least 1 entry")
	}
	return nil
}

// glyphMaskPage is a single atlas texture page containing R8 alpha masks.
type glyphMaskPage struct {
	// Data is the R8 pixel data (1 byte per pixel, alpha only).
	Data []byte

	// Size is width = height of the atlas page.
	Size int

	// allocator packs variable-sized glyph masks using shelf algorithm.
	allocator *glyphMaskShelfAllocator

	// dirty marks if the page needs GPU re-upload.
	dirty bool

	// index is the page index in the manager.
	index int

	// lastUsedFrame is the frame when this page was last written to.
	// Used by Compact() for frame-based page eviction (Skia pattern).
	lastUsedFrame uint64

	// entryCount tracks how many live entries reference this page.
	entryCount int
}

// newGlyphMaskPage creates a new atlas page.
func newGlyphMaskPage(index, size, padding int) *glyphMaskPage {
	return &glyphMaskPage{
		Data:      make([]byte, size*size),
		Size:      size,
		allocator: newGlyphMaskShelfAllocator(size, size, padding),
		dirty:     false,
		index:     index,
	}
}

// copyMask copies an alpha mask into the page at the given position.
func (p *glyphMaskPage) copyMask(mask []byte, maskW, maskH, dstX, dstY int) {
	for row := range maskH {
		srcOffset := row * maskW
		dstOffset := (dstY+row)*p.Size + dstX
		if srcOffset+maskW > len(mask) || dstOffset+maskW > len(p.Data) {
			continue
		}
		copy(p.Data[dstOffset:dstOffset+maskW], mask[srcOffset:srcOffset+maskW])
	}
	p.dirty = true
}

// glyphMaskShelfAllocator is a shelf-based allocator for variable-sized glyph masks.
// Unlike the MSDF GridAllocator (fixed-size cells), glyph masks vary in size
// depending on the glyph and font size, so we use shelf packing.
type glyphMaskShelfAllocator struct {
	width   int
	height  int
	padding int
	shelves []glyphMaskShelf
}

// glyphMaskShelf represents a horizontal strip in the atlas page.
type glyphMaskShelf struct {
	y      int // Y position of shelf top
	height int // Height of the shelf (tallest item)
	x      int // Current X position (next free slot)
}

// newGlyphMaskShelfAllocator creates a new shelf allocator.
func newGlyphMaskShelfAllocator(width, height, padding int) *glyphMaskShelfAllocator {
	return &glyphMaskShelfAllocator{
		width:   width,
		height:  height,
		padding: padding,
		shelves: make([]glyphMaskShelf, 0, 32),
	}
}

// Allocate finds space for a rectangle of size (w, h).
// Returns (x, y, true) on success, or (-1, -1, false) if the page is full.
func (a *glyphMaskShelfAllocator) Allocate(w, h int) (x, y int, ok bool) {
	paddedW := w + a.padding
	paddedH := h + a.padding

	// Try existing shelves
	for i := range a.shelves {
		s := &a.shelves[i]

		// Must fit horizontally
		if s.x+paddedW > a.width {
			continue
		}

		// Must fit in shelf height (or be extendable if last shelf)
		if h > s.height {
			if i == len(a.shelves)-1 {
				newBottom := s.y + paddedH
				if newBottom <= a.height {
					s.height = h
					x, y = s.x, s.y
					s.x += paddedW
					return x, y, true
				}
			}
			continue
		}

		x, y = s.x, s.y
		s.x += paddedW
		return x, y, true
	}

	// Create new shelf
	newY := 0
	if len(a.shelves) > 0 {
		last := a.shelves[len(a.shelves)-1]
		newY = last.y + last.height + a.padding
	}

	if newY+paddedH > a.height {
		return -1, -1, false
	}

	newShelf := glyphMaskShelf{
		y:      newY,
		height: h,
		x:      paddedW,
	}
	a.shelves = append(a.shelves, newShelf)
	return 0, newY, true
}

// CanFit returns true if a rectangle of the given size could fit.
func (a *glyphMaskShelfAllocator) CanFit(w, h int) bool {
	paddedW := w + a.padding
	paddedH := h + a.padding

	if paddedW > a.width || paddedH > a.height {
		return false
	}

	for i := range a.shelves {
		s := &a.shelves[i]
		if s.x+paddedW <= a.width && h <= s.height {
			return true
		}
		if i == len(a.shelves)-1 && s.x+paddedW <= a.width && s.y+paddedH <= a.height {
			return true
		}
	}

	newY := 0
	if len(a.shelves) > 0 {
		last := a.shelves[len(a.shelves)-1]
		newY = last.y + last.height + a.padding
	}
	return newY+paddedH <= a.height
}

// Reset clears all allocations.
func (a *glyphMaskShelfAllocator) Reset() {
	a.shelves = a.shelves[:0]
}

// glyphMaskEntry is an LRU cache entry for a glyph mask.
type glyphMaskEntry struct {
	key    GlyphMaskKey
	region GlyphMaskRegion

	// LRU doubly-linked list pointers
	prev *glyphMaskEntry
	next *glyphMaskEntry

	// lastAccessFrame for frame-based eviction
	lastAccessFrame uint64
}

// GlyphMaskAtlas manages R8 alpha mask atlases for CPU-rasterized glyphs.
//
// Architecture (Skia/Chrome pattern):
//  1. CPU rasterizes glyph at exact device pixel size via AnalyticFiller (256-level AA)
//  2. Alpha mask is packed into R8 atlas page using shelf allocator
//  3. GPU composites as textured quad in render pass (Tier 6)
//
// The atlas uses LRU eviction: when MaxEntries is reached, the least recently
// used glyphs are evicted. Page-level eviction happens when all entries on a
// page are evicted.
//
// GlyphMaskAtlas is safe for concurrent use.
type GlyphMaskAtlas struct {
	mu     sync.Mutex
	config GlyphMaskAtlasConfig

	// Atlas pages (R8 textures)
	pages []*glyphMaskPage

	// Cache: key -> entry
	lookup map[GlyphMaskKey]*glyphMaskEntry

	// LRU list: head = most recently used, tail = least recently used
	head *glyphMaskEntry
	tail *glyphMaskEntry

	// Current frame counter for frame-based access tracking
	currentFrame atomic.Uint64

	// bucketedMode is a sticky flag for size bucket quantization.
	// Enter at 50% capacity, exit at 25% — hysteresis prevents oscillation
	// between bucketed and fine-grained modes during smooth zoom.
	bucketedMode bool

	// Statistics
	hits   atomic.Uint64
	misses atomic.Uint64
}

// NewGlyphMaskAtlas creates a new glyph mask atlas with the given configuration.
func NewGlyphMaskAtlas(config GlyphMaskAtlasConfig) (*GlyphMaskAtlas, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &GlyphMaskAtlas{
		config: config,
		pages:  make([]*glyphMaskPage, 0, config.MaxAtlases),
		lookup: make(map[GlyphMaskKey]*glyphMaskEntry, 256),
	}, nil
}

// NewGlyphMaskAtlasDefault creates a new glyph mask atlas with default configuration.
func NewGlyphMaskAtlasDefault() *GlyphMaskAtlas {
	atlas, _ := NewGlyphMaskAtlas(DefaultGlyphMaskAtlasConfig())
	return atlas
}

// Get retrieves a cached glyph mask region.
// Returns the region and true if found, or zero region and false if not cached.
func (a *GlyphMaskAtlas) Get(key GlyphMaskKey) (GlyphMaskRegion, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	entry, ok := a.lookup[key]
	if !ok {
		a.misses.Add(1)
		return GlyphMaskRegion{}, false
	}

	// Update LRU position
	entry.lastAccessFrame = a.currentFrame.Load()
	a.moveToFront(entry)

	a.hits.Add(1)
	return entry.region, true
}

// Put stores a rasterized glyph mask in the atlas.
// The mask is an R8 alpha buffer of dimensions (maskW x maskH).
// BearingX/BearingY are the glyph positioning offsets from the origin.
//
// Returns the region where the mask was stored, or an error if the atlas is full.
func (a *GlyphMaskAtlas) Put(key GlyphMaskKey, mask []byte, maskW, maskH int, bearingX, bearingY float32) (GlyphMaskRegion, error) {
	if maskW <= 0 || maskH <= 0 || len(mask) < maskW*maskH {
		return GlyphMaskRegion{}, errors.New("text: invalid glyph mask dimensions")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if already cached (race with concurrent Put)
	if entry, ok := a.lookup[key]; ok {
		entry.lastAccessFrame = a.currentFrame.Load()
		a.moveToFront(entry)
		return entry.region, nil
	}

	// Evict LRU entries if at capacity
	for len(a.lookup) >= a.config.MaxEntries {
		a.evictTail()
	}

	// Find or create a page with space
	page, err := a.findOrCreatePage(maskW, maskH)
	if err != nil {
		return GlyphMaskRegion{}, err
	}

	// Allocate space in the page
	x, y, ok := page.allocator.Allocate(maskW, maskH)
	if !ok {
		return GlyphMaskRegion{}, fmt.Errorf("text: failed to allocate %dx%d glyph mask in atlas page %d", maskW, maskH, page.index)
	}

	// Copy mask data into the page
	page.copyMask(mask, maskW, maskH, x, y)

	frame := a.currentFrame.Load()
	page.lastUsedFrame = frame
	page.entryCount++

	// Compute UV coordinates with half-texel inset
	atlasSize := float32(a.config.Size)
	halfTexel := float32(0.5) / atlasSize

	region := GlyphMaskRegion{
		AtlasIndex: page.index,
		X:          x,
		Y:          y,
		Width:      maskW,
		Height:     maskH,
		BearingX:   bearingX,
		BearingY:   bearingY,
		U0:         float32(x)/atlasSize + halfTexel,
		V0:         float32(y)/atlasSize + halfTexel,
		U1:         float32(x+maskW)/atlasSize - halfTexel,
		V1:         float32(y+maskH)/atlasSize - halfTexel,
	}

	// Create cache entry and add to LRU
	entry := &glyphMaskEntry{
		key:             key,
		region:          region,
		lastAccessFrame: frame,
	}
	a.lookup[key] = entry
	a.addToFront(entry)

	return region, nil
}

// PutLCD stores an LCD (ClearType) glyph mask in the atlas. The mask contains
// RGB coverage data (3 bytes per pixel, logicalW pixels wide), which is packed
// into the R8 atlas at 3x width (3 * logicalW R8 texels per row). The region's
// Width is set to 3 * logicalW (atlas texels), and IsLCD is set to true.
//
// The caller must convert the RGB triplets to row-major R8 data before calling:
// for each row, the 3*logicalW bytes are stored sequentially in the atlas.
func (a *GlyphMaskAtlas) PutLCD(key GlyphMaskKey, rgbMask []byte, logicalW, maskH int, bearingX, bearingY float32) (GlyphMaskRegion, error) {
	atlasW := logicalW * 3 // width in R8 texels
	if logicalW <= 0 || maskH <= 0 || len(rgbMask) < atlasW*maskH {
		return GlyphMaskRegion{}, errors.New("text: invalid LCD glyph mask dimensions")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if already cached.
	if entry, ok := a.lookup[key]; ok {
		entry.lastAccessFrame = a.currentFrame.Load()
		a.moveToFront(entry)
		return entry.region, nil
	}

	// Evict LRU entries if at capacity.
	for len(a.lookup) >= a.config.MaxEntries {
		a.evictTail()
	}

	// Find or create a page with space for the 3x-wide data.
	page, err := a.findOrCreatePage(atlasW, maskH)
	if err != nil {
		return GlyphMaskRegion{}, err
	}

	x, y, ok := page.allocator.Allocate(atlasW, maskH)
	if !ok {
		return GlyphMaskRegion{}, fmt.Errorf("text: failed to allocate %dx%d LCD glyph mask in atlas page %d", atlasW, maskH, page.index)
	}

	// Copy the RGB data row by row into the R8 atlas at 3x width.
	page.copyMask(rgbMask, atlasW, maskH, x, y)

	frame := a.currentFrame.Load()
	page.lastUsedFrame = frame
	page.entryCount++

	atlasSize := float32(a.config.Size)
	halfTexel := float32(0.5) / atlasSize

	region := GlyphMaskRegion{
		AtlasIndex: page.index,
		X:          x,
		Y:          y,
		Width:      atlasW, // 3x logical width in R8 texels
		Height:     maskH,
		BearingX:   bearingX,
		BearingY:   bearingY,
		U0:         float32(x)/atlasSize + halfTexel,
		V0:         float32(y)/atlasSize + halfTexel,
		U1:         float32(x+atlasW)/atlasSize - halfTexel,
		V1:         float32(y+maskH)/atlasSize - halfTexel,
		IsLCD:      true,
	}

	entry := &glyphMaskEntry{
		key:             key,
		region:          region,
		lastAccessFrame: frame,
	}
	a.lookup[key] = entry
	a.addToFront(entry)

	return region, nil
}

// GetOrRasterize retrieves a cached glyph mask or rasterizes it using the
// provided function. This is the primary API for the glyph mask pipeline.
//
// The rasterize function is called on cache miss and should return:
//   - mask: R8 alpha buffer
//   - maskW, maskH: mask dimensions
//   - bearingX, bearingY: glyph positioning offsets
//   - err: any error during rasterization
func (a *GlyphMaskAtlas) GetOrRasterize(
	key GlyphMaskKey,
	rasterize func() (mask []byte, maskW, maskH int, bearingX, bearingY float32, err error),
) (GlyphMaskRegion, error) {
	// Fast path: check cache
	if region, ok := a.Get(key); ok {
		return region, nil
	}

	// Slow path: rasterize and store
	mask, maskW, maskH, bearingX, bearingY, err := rasterize()
	if err != nil {
		return GlyphMaskRegion{}, fmt.Errorf("text: glyph mask rasterization failed: %w", err)
	}

	// Empty glyph (e.g., space) — return zero region without storing
	if maskW <= 0 || maskH <= 0 {
		return GlyphMaskRegion{}, nil
	}

	return a.Put(key, mask, maskW, maskH, bearingX, bearingY)
}

// findOrCreatePage finds a page with space for the given dimensions, or creates a new one.
// Must be called with a.mu held.
func (a *GlyphMaskAtlas) findOrCreatePage(w, h int) (*glyphMaskPage, error) {
	// Try existing pages
	for _, page := range a.pages {
		if page.allocator.CanFit(w, h) {
			return page, nil
		}
	}

	// Create new page
	if len(a.pages) >= a.config.MaxAtlases {
		return nil, fmt.Errorf("text: all %d glyph mask atlas pages are full", a.config.MaxAtlases)
	}

	page := newGlyphMaskPage(len(a.pages), a.config.Size, a.config.Padding)
	a.pages = append(a.pages, page)
	return page, nil
}

// evictTail removes the least recently used entry.
// When a page loses all its entries, the page is reset (shelf allocator cleared,
// pixel data zeroed) — reclaiming atlas space for new allocations.
// Must be called with a.mu held.
func (a *GlyphMaskAtlas) evictTail() {
	if a.tail == nil {
		return
	}
	entry := a.tail
	pageIdx := entry.region.AtlasIndex
	a.removeFromList(entry)
	delete(a.lookup, entry.key)

	if pageIdx >= 0 && pageIdx < len(a.pages) {
		page := a.pages[pageIdx]
		page.entryCount--
		if page.entryCount <= 0 {
			a.resetPage(page)
		}
	}
}

// resetPage clears a page's allocator and pixel data, making it available
// for new allocations. Entries referencing this page must already be removed.
// Must be called with a.mu held.
func (a *GlyphMaskAtlas) resetPage(page *glyphMaskPage) {
	page.allocator.Reset()
	clear(page.Data)
	page.dirty = true
	page.entryCount = 0
}

// LRU list operations. Must be called with a.mu held.

func (a *GlyphMaskAtlas) addToFront(entry *glyphMaskEntry) {
	entry.prev = nil
	entry.next = a.head
	if a.head != nil {
		a.head.prev = entry
	}
	a.head = entry
	if a.tail == nil {
		a.tail = entry
	}
}

func (a *GlyphMaskAtlas) moveToFront(entry *glyphMaskEntry) {
	if entry == a.head {
		return
	}
	a.removeFromList(entry)
	a.addToFront(entry)
}

func (a *GlyphMaskAtlas) removeFromList(entry *glyphMaskEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		a.head = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		a.tail = entry.prev
	}
	entry.prev = nil
	entry.next = nil
}

// compactStaleFrames is the number of frames a page must be unused before
// Compact() resets it. Matches Skia's kPlotRecentlyUsedCount = 32.
const compactStaleFrames = 32

// AdvanceFrame increments the frame counter and runs compaction.
// Call once per frame (e.g., from GPU flush). This is the primary
// self-healing mechanism: pages unused for 32+ frames are reset,
// reclaiming atlas space after zoom or font size changes.
//
// Reference: Skia GrAtlasManager::postFlush() calls compact() every flush.
func (a *GlyphMaskAtlas) AdvanceFrame() {
	frame := a.currentFrame.Add(1)
	a.compact(frame)
}

// compact resets atlas pages that have not been used for compactStaleFrames.
// Entries on stale pages are removed from the lookup map, and the page's
// shelf allocator is cleared — making the space available for new glyphs.
// Must NOT be called with a.mu held (acquires lock internally).
func (a *GlyphMaskAtlas) compact(currentFrame uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if currentFrame < compactStaleFrames {
		return
	}
	threshold := currentFrame - compactStaleFrames

	for _, page := range a.pages {
		if page.entryCount == 0 || page.lastUsedFrame > threshold {
			continue
		}

		// Page is stale — remove all entries referencing it.
		for key, entry := range a.lookup {
			if entry.region.AtlasIndex == page.index {
				a.removeFromList(entry)
				delete(a.lookup, key)
			}
		}
		a.resetPage(page)
	}
}

// DirtyPages returns indices of pages that have been modified since
// the last MarkClean call and need GPU upload.
func (a *GlyphMaskAtlas) DirtyPages() []int {
	a.mu.Lock()
	defer a.mu.Unlock()

	var dirty []int
	for i, page := range a.pages {
		if page.dirty {
			dirty = append(dirty, i)
		}
	}
	return dirty
}

// PageR8Data returns the R8 pixel data for a page.
// Returns nil if the index is out of range.
func (a *GlyphMaskAtlas) PageR8Data(index int) (data []byte, width, height int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if index < 0 || index >= len(a.pages) {
		return nil, 0, 0
	}
	page := a.pages[index]
	return page.Data, page.Size, page.Size
}

// MarkClean marks a page as uploaded to GPU.
func (a *GlyphMaskAtlas) MarkClean(index int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if index >= 0 && index < len(a.pages) {
		a.pages[index].dirty = false
	}
}

// Clear removes all cached glyphs and resets all pages.
func (a *GlyphMaskAtlas) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.pages = a.pages[:0]
	a.lookup = make(map[GlyphMaskKey]*glyphMaskEntry, 256)
	a.head = nil
	a.tail = nil
	a.hits.Store(0)
	a.misses.Store(0)
}

// Stats returns cache statistics.
func (a *GlyphMaskAtlas) Stats() (hits, misses uint64, entryCount, pageCount int) {
	a.mu.Lock()
	entryCount = len(a.lookup)
	pageCount = len(a.pages)
	a.mu.Unlock()

	hits = a.hits.Load()
	misses = a.misses.Load()
	return
}

// UnderPressure returns true when callers should use MakeGlyphMaskKeyBucketed
// to reduce unique entries. Uses hysteresis to prevent oscillation:
// enters bucketed mode at 50% capacity, exits at 25%.
func (a *GlyphMaskAtlas) UnderPressure() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	entries := len(a.lookup)
	if a.bucketedMode {
		if entries < a.config.MaxEntries/4 {
			a.bucketedMode = false
		}
	} else {
		if entries >= a.config.MaxEntries/2 {
			a.bucketedMode = true
		}
	}
	return a.bucketedMode
}

// Config returns the atlas configuration.
func (a *GlyphMaskAtlas) Config() GlyphMaskAtlasConfig {
	return a.config
}

// PageCount returns the number of atlas pages currently in use.
func (a *GlyphMaskAtlas) PageCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pages)
}

// EntryCount returns the number of cached glyph masks.
func (a *GlyphMaskAtlas) EntryCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.lookup)
}

// MemoryUsage returns the total memory used by all atlas pages in bytes.
func (a *GlyphMaskAtlas) MemoryUsage() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	var total int64
	for _, page := range a.pages {
		total += int64(len(page.Data))
	}
	return total
}
