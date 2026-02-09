package gpu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/gg"
)

// Atlas-related errors.
var (
	// ErrAtlasFull is returned when the atlas cannot fit the requested region.
	ErrAtlasFull = errors.New("wgpu: texture atlas is full")

	// ErrAtlasClosed is returned when operating on a closed atlas.
	ErrAtlasClosed = errors.New("wgpu: texture atlas is closed")

	// ErrRegionOutOfBounds is returned when a region is outside atlas bounds.
	ErrRegionOutOfBounds = errors.New("wgpu: region is outside atlas bounds")
)

// Default atlas settings.
const (
	// DefaultAtlasSize is the default atlas dimension (2048x2048).
	DefaultAtlasSize = 2048

	// MinAtlasSize is the minimum atlas dimension (256x256).
	MinAtlasSize = 256

	// DefaultShelfPadding is the padding between shelves.
	DefaultShelfPadding = 1
)

// AtlasRegion represents a rectangular region in a texture atlas.
type AtlasRegion struct {
	// X is the left edge of the region.
	X int
	// Y is the top edge of the region.
	Y int
	// Width is the region width.
	Width int
	// Height is the region height.
	Height int
}

// IsValid returns true if the region has valid dimensions.
func (r AtlasRegion) IsValid() bool {
	return r.Width > 0 && r.Height > 0
}

// Contains returns true if the point (x, y) is inside the region.
func (r AtlasRegion) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

// String returns a string representation of the region.
func (r AtlasRegion) String() string {
	return fmt.Sprintf("Region(%d,%d %dx%d)", r.X, r.Y, r.Width, r.Height)
}

// shelf represents a horizontal shelf in the shelf-packing algorithm.
type shelf struct {
	y       int // Top Y coordinate of this shelf
	height  int // Height of this shelf (tallest item so far)
	nextX   int // Next available X position on this shelf
	padding int // Padding between items
}

// RectAllocator implements a simple shelf-packing algorithm for
// allocating rectangular regions within a fixed-size area.
//
// The shelf-packing algorithm works by dividing the atlas into
// horizontal "shelves". Each new rectangle is placed on the current
// shelf if it fits, or a new shelf is created below.
type RectAllocator struct {
	mu sync.Mutex

	// Atlas dimensions
	width  int
	height int

	// Shelves (sorted by Y position)
	shelves []*shelf

	// Current shelf for allocation
	currentShelf int

	// Padding between items and shelves
	padding int

	// Statistics
	allocCount int
	usedArea   int
}

// NewRectAllocator creates a new rectangular region allocator.
func NewRectAllocator(width, height, padding int) *RectAllocator {
	if width < MinAtlasSize {
		width = MinAtlasSize
	}
	if height < MinAtlasSize {
		height = MinAtlasSize
	}
	if padding < 0 {
		padding = 0
	}

	return &RectAllocator{
		width:   width,
		height:  height,
		shelves: make([]*shelf, 0, 16),
		padding: padding,
	}
}

// Allocate finds space for a rectangle of the given size.
// Returns an invalid region if the rectangle cannot be allocated.
func (a *RectAllocator) Allocate(width, height int) AtlasRegion {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Validate size
	if width <= 0 || height <= 0 {
		return AtlasRegion{}
	}

	// Add padding to requested size
	paddedWidth := width + a.padding
	paddedHeight := height + a.padding

	// Check if it fits in the atlas at all
	if paddedWidth > a.width || paddedHeight > a.height {
		return AtlasRegion{}
	}

	// Try to fit on existing shelves
	for i, s := range a.shelves {
		if a.fitsOnShelf(s, paddedWidth, paddedHeight) {
			return a.allocateOnShelf(i, width, height, paddedWidth)
		}
	}

	// Create a new shelf
	return a.allocateNewShelf(width, height, paddedWidth, paddedHeight)
}

// fitsOnShelf checks if a rectangle fits on the given shelf.
func (a *RectAllocator) fitsOnShelf(s *shelf, paddedWidth, paddedHeight int) bool {
	// Check horizontal space
	if s.nextX+paddedWidth > a.width {
		return false
	}
	// Check if height is compatible (can be taller if shelf is tall enough,
	// but we can't make the shelf taller if items are already on it)
	if paddedHeight > s.height && s.nextX > 0 {
		return false
	}
	return true
}

// allocateOnShelf allocates space on an existing shelf.
func (a *RectAllocator) allocateOnShelf(shelfIndex, width, height, paddedWidth int) AtlasRegion {
	s := a.shelves[shelfIndex]

	region := AtlasRegion{
		X:      s.nextX,
		Y:      s.y,
		Width:  width,
		Height: height,
	}

	// Update shelf state
	s.nextX += paddedWidth
	if height+a.padding > s.height {
		s.height = height + a.padding
	}

	a.allocCount++
	a.usedArea += width * height

	return region
}

// allocateNewShelf creates a new shelf and allocates the rectangle on it.
func (a *RectAllocator) allocateNewShelf(width, height, paddedWidth, paddedHeight int) AtlasRegion {
	// Calculate Y position for new shelf
	newY := 0
	if len(a.shelves) > 0 {
		lastShelf := a.shelves[len(a.shelves)-1]
		newY = lastShelf.y + lastShelf.height
	}

	// Check if there's vertical space
	if newY+paddedHeight > a.height {
		return AtlasRegion{}
	}

	// Create new shelf
	newShelf := &shelf{
		y:       newY,
		height:  paddedHeight,
		nextX:   paddedWidth,
		padding: a.padding,
	}
	a.shelves = append(a.shelves, newShelf)

	region := AtlasRegion{
		X:      0,
		Y:      newY,
		Width:  width,
		Height: height,
	}

	a.allocCount++
	a.usedArea += width * height

	return region
}

// Reset clears all allocations, making the entire area available again.
func (a *RectAllocator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.shelves = a.shelves[:0]
	a.currentShelf = 0
	a.allocCount = 0
	a.usedArea = 0
}

// UsedArea returns the total area of allocated rectangles.
func (a *RectAllocator) UsedArea() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.usedArea
}

// Utilization returns the fraction of area used (0.0 to 1.0).
func (a *RectAllocator) Utilization() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	totalArea := a.width * a.height
	if totalArea == 0 {
		return 0
	}
	return float64(a.usedArea) / float64(totalArea)
}

// AllocCount returns the number of successful allocations.
func (a *RectAllocator) AllocCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.allocCount
}

// TextureAtlas manages a texture atlas for efficient batching of small images.
// It combines multiple small textures into a single large GPU texture to
// reduce draw calls and texture binding changes.
//
// TextureAtlas is safe for concurrent use.
type TextureAtlas struct {
	mu sync.RWMutex

	// The underlying GPU texture
	texture *GPUTexture

	// Region allocator
	allocator *RectAllocator

	// Atlas dimensions
	width  int
	height int

	// Configuration
	padding int

	// State
	closed bool
	dirty  bool // True if texture needs re-upload
}

// TextureAtlasConfig holds configuration for creating a TextureAtlas.
type TextureAtlasConfig struct {
	// Width is the atlas width in pixels. Defaults to DefaultAtlasSize.
	Width int

	// Height is the atlas height in pixels. Defaults to DefaultAtlasSize.
	Height int

	// Padding is the spacing between regions. Defaults to DefaultShelfPadding.
	Padding int

	// Label is an optional debug label.
	Label string
}

// NewTextureAtlas creates a new texture atlas with the given configuration.
func NewTextureAtlas(backend *Backend, config TextureAtlasConfig) (*TextureAtlas, error) {
	width := config.Width
	if width < MinAtlasSize {
		width = DefaultAtlasSize
	}

	height := config.Height
	if height < MinAtlasSize {
		height = DefaultAtlasSize
	}

	padding := config.Padding
	if padding < 0 {
		padding = DefaultShelfPadding
	}

	// Create the GPU texture
	tex, err := CreateTexture(backend, TextureConfig{
		Width:  width,
		Height: height,
		Format: TextureFormatRGBA8,
		Label:  config.Label,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create atlas texture: %w", err)
	}

	return &TextureAtlas{
		texture:   tex,
		allocator: NewRectAllocator(width, height, padding),
		width:     width,
		height:    height,
		padding:   padding,
	}, nil
}

// Allocate finds space for a rectangle of the given size.
// Returns an invalid region (Width/Height == 0) if the atlas is full.
func (a *TextureAtlas) Allocate(width, height int) (AtlasRegion, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return AtlasRegion{}, ErrAtlasClosed
	}

	region := a.allocator.Allocate(width, height)
	if !region.IsValid() {
		return AtlasRegion{}, ErrAtlasFull
	}

	return region, nil
}

// Upload copies pixel data from a pixmap to a region of the atlas.
// The pixmap dimensions must match the region dimensions.
func (a *TextureAtlas) Upload(region AtlasRegion, pixmap *gg.Pixmap) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return ErrAtlasClosed
	}

	if pixmap == nil {
		return ErrNilPixmap
	}

	// Validate region bounds
	if region.X < 0 || region.Y < 0 ||
		region.X+region.Width > a.width ||
		region.Y+region.Height > a.height {
		return ErrRegionOutOfBounds
	}

	// Validate pixmap size matches region
	if pixmap.Width() != region.Width || pixmap.Height() != region.Height {
		return fmt.Errorf("%w: region is %dx%d but pixmap is %dx%d",
			ErrTextureSizeMismatch,
			region.Width, region.Height,
			pixmap.Width(), pixmap.Height())
	}

	// Upload to the region
	if err := a.texture.UploadRegion(region.X, region.Y, pixmap); err != nil {
		return fmt.Errorf("failed to upload to atlas region: %w", err)
	}

	a.dirty = true
	return nil
}

// AllocateAndUpload combines Allocate and Upload into a single operation.
// This is a convenience method for adding new images to the atlas.
func (a *TextureAtlas) AllocateAndUpload(pixmap *gg.Pixmap) (AtlasRegion, error) {
	if pixmap == nil {
		return AtlasRegion{}, ErrNilPixmap
	}

	region, err := a.Allocate(pixmap.Width(), pixmap.Height())
	if err != nil {
		return AtlasRegion{}, err
	}

	if err := a.Upload(region, pixmap); err != nil {
		return AtlasRegion{}, err
	}

	return region, nil
}

// Texture returns the underlying GPU texture.
func (a *TextureAtlas) Texture() *GPUTexture {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.texture
}

// Width returns the atlas width in pixels.
func (a *TextureAtlas) Width() int {
	return a.width
}

// Height returns the atlas height in pixels.
func (a *TextureAtlas) Height() int {
	return a.height
}

// Utilization returns the fraction of atlas area used (0.0 to 1.0).
func (a *TextureAtlas) Utilization() float64 {
	return a.allocator.Utilization()
}

// AllocCount returns the number of allocated regions.
func (a *TextureAtlas) AllocCount() int {
	return a.allocator.AllocCount()
}

// Reset clears all allocations, making the entire atlas available again.
// Note: This does not clear the texture data, just the allocation tracking.
func (a *TextureAtlas) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}

	a.allocator.Reset()
	a.dirty = false
}

// Close releases the atlas resources.
// The atlas should not be used after Close is called.
func (a *TextureAtlas) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}

	if a.texture != nil {
		a.texture.Close()
		a.texture = nil
	}

	a.allocator = nil
	a.closed = true
}

// IsClosed returns true if the atlas has been closed.
func (a *TextureAtlas) IsClosed() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.closed
}
