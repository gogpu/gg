package recording

import (
	"image"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// Note: PathRef, BrushRef, ImageRef, and InvalidRef are defined in command.go

// FontRef is a reference to a pooled font face.
// The zero value is a valid reference to the first font (if any).
type FontRef uint32

// IsValid returns true if the reference is valid (not InvalidRef).
func (r FontRef) IsValid() bool {
	return uint32(r) != InvalidRef
}

// ResourcePool stores resources referenced by recording commands.
// Resources are stored in slices indexed by their reference types.
// Each Add operation clones mutable resources to ensure immutability.
//
// ResourcePool is not safe for concurrent use. If concurrent access is needed,
// external synchronization must be provided.
type ResourcePool struct {
	paths   []*gg.Path
	brushes []Brush
	images  []image.Image
	fonts   []text.Face
}

// NewResourcePool creates an empty resource pool with pre-allocated capacity.
func NewResourcePool() *ResourcePool {
	return &ResourcePool{
		paths:   make([]*gg.Path, 0, 64),
		brushes: make([]Brush, 0, 32),
		images:  make([]image.Image, 0, 8),
		fonts:   make([]text.Face, 0, 4),
	}
}

// AddPath adds a path to the pool and returns its reference.
// The path is cloned to ensure immutability of the recording.
func (p *ResourcePool) AddPath(path *gg.Path) PathRef {
	if path == nil {
		// Add nil as-is to preserve the reference
		p.paths = append(p.paths, nil)
		// #nosec G115 -- pool size is bounded by available memory, well under uint32 max
		return PathRef(uint32(len(p.paths) - 1))
	}
	// Clone path to ensure immutability
	cloned := path.Clone()
	p.paths = append(p.paths, cloned)
	// #nosec G115 -- pool size is bounded by available memory, well under uint32 max
	return PathRef(uint32(len(p.paths) - 1))
}

// GetPath returns the path for the given reference.
// Returns nil if the reference is invalid.
func (p *ResourcePool) GetPath(ref PathRef) *gg.Path {
	if int(ref) >= len(p.paths) {
		return nil
	}
	return p.paths[ref]
}

// PathCount returns the number of paths in the pool.
func (p *ResourcePool) PathCount() int {
	return len(p.paths)
}

// AddBrush adds a brush to the pool and returns its reference.
// Brushes are stored directly as they are typically immutable value types.
func (p *ResourcePool) AddBrush(brush Brush) BrushRef {
	p.brushes = append(p.brushes, brush)
	// #nosec G115 -- pool size is bounded by available memory, well under uint32 max
	return BrushRef(uint32(len(p.brushes) - 1))
}

// GetBrush returns the brush for the given reference.
// Returns nil if the reference is invalid.
func (p *ResourcePool) GetBrush(ref BrushRef) Brush {
	if int(ref) >= len(p.brushes) {
		return nil
	}
	return p.brushes[ref]
}

// BrushCount returns the number of brushes in the pool.
func (p *ResourcePool) BrushCount() int {
	return len(p.brushes)
}

// AddImage adds an image to the pool and returns its reference.
// Images are stored directly as Go's image.Image is already immutable.
func (p *ResourcePool) AddImage(img image.Image) ImageRef {
	p.images = append(p.images, img)
	// #nosec G115 -- pool size is bounded by available memory, well under uint32 max
	return ImageRef(uint32(len(p.images) - 1))
}

// GetImage returns the image for the given reference.
// Returns nil if the reference is invalid.
func (p *ResourcePool) GetImage(ref ImageRef) image.Image {
	if int(ref) >= len(p.images) {
		return nil
	}
	return p.images[ref]
}

// ImageCount returns the number of images in the pool.
func (p *ResourcePool) ImageCount() int {
	return len(p.images)
}

// AddFont adds a font face to the pool and returns its reference.
// Font faces are stored directly as they are already immutable and safe to share.
func (p *ResourcePool) AddFont(face text.Face) FontRef {
	p.fonts = append(p.fonts, face)
	// #nosec G115 -- pool size is bounded by available memory, well under uint32 max
	return FontRef(uint32(len(p.fonts) - 1))
}

// GetFont returns the font face for the given reference.
// Returns nil if the reference is invalid.
func (p *ResourcePool) GetFont(ref FontRef) text.Face {
	if int(ref) >= len(p.fonts) {
		return nil
	}
	return p.fonts[ref]
}

// FontCount returns the number of font faces in the pool.
func (p *ResourcePool) FontCount() int {
	return len(p.fonts)
}

// Clear removes all resources from the pool.
// This does not release the underlying memory; use NewResourcePool for that.
func (p *ResourcePool) Clear() {
	p.paths = p.paths[:0]
	p.brushes = p.brushes[:0]
	p.images = p.images[:0]
	p.fonts = p.fonts[:0]
}

// Clone creates a deep copy of the resource pool.
// Paths are cloned; other resources are copied by reference (they are immutable).
func (p *ResourcePool) Clone() *ResourcePool {
	clone := &ResourcePool{
		paths:   make([]*gg.Path, len(p.paths)),
		brushes: make([]Brush, len(p.brushes)),
		images:  make([]image.Image, len(p.images)),
		fonts:   make([]text.Face, len(p.fonts)),
	}

	// Clone paths (mutable)
	for i, path := range p.paths {
		if path != nil {
			clone.paths[i] = path.Clone()
		}
	}

	// Copy brushes directly (immutable structs)
	copy(clone.brushes, p.brushes)

	// Copy images and fonts by reference (already immutable)
	copy(clone.images, p.images)
	copy(clone.fonts, p.fonts)

	return clone
}
