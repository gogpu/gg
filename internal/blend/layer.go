// Package blend implements layer system for isolated drawing surfaces with compositing.
package blend

import (
	"github.com/gogpu/gg/internal/image"
)

// Layer represents an isolated drawing surface with blend mode and opacity.
//
// Layer provides a buffer for drawing operations that can be composited onto
// a parent layer using a specific blend mode and opacity. Layers form a stack,
// allowing for nested compositing operations.
//
// Thread safety: Layer is not safe for concurrent access. External synchronization
// is required if multiple goroutines access the same layer.
type Layer struct {
	buffer    *image.ImageBuf
	blendMode BlendMode
	opacity   float64
	bounds    Bounds
}

// Bounds represents a rectangular region in pixel coordinates.
type Bounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

// NewLayer creates a new layer with the specified blend mode, opacity, and bounds.
// The buffer is allocated from the pool for efficient memory reuse.
// Opacity must be in range [0.0, 1.0] where 0 is fully transparent and 1 is fully opaque.
func NewLayer(blendMode BlendMode, opacity float64, bounds Bounds, pool *image.Pool) (*Layer, error) {
	// Clamp opacity to valid range
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}

	// Allocate buffer from pool
	buf := pool.Get(bounds.Width, bounds.Height, image.FormatRGBA8)
	if buf == nil {
		return nil, image.ErrInvalidDimensions
	}

	return &Layer{
		buffer:    buf,
		blendMode: blendMode,
		opacity:   opacity,
		bounds:    bounds,
	}, nil
}

// Buffer returns the layer's image buffer.
func (l *Layer) Buffer() *image.ImageBuf {
	return l.buffer
}

// BlendMode returns the layer's blend mode.
func (l *Layer) BlendMode() BlendMode {
	return l.blendMode
}

// Opacity returns the layer's opacity (0.0 to 1.0).
func (l *Layer) Opacity() float64 {
	return l.opacity
}

// Bounds returns the layer's bounds.
func (l *Layer) Bounds() Bounds {
	return l.bounds
}

// SetOpacity sets the layer's opacity, clamped to [0.0, 1.0].
func (l *Layer) SetOpacity(opacity float64) {
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}
	l.opacity = opacity
}

// LayerStack manages a stack of layers for nested compositing operations.
//
// LayerStack provides push/pop semantics for creating temporary drawing surfaces
// that can be composited back onto the parent layer. The base layer represents
// the final output surface.
//
// Thread safety: LayerStack is not safe for concurrent access. External synchronization
// is required if multiple goroutines access the same stack.
type LayerStack struct {
	layers []*Layer
	base   *image.ImageBuf
	pool   *image.Pool
}

// NewLayerStack creates a new layer stack with the given base image buffer.
// The base buffer represents the final output surface.
// If pool is nil, the default pool is used for layer allocation.
func NewLayerStack(base *image.ImageBuf, pool *image.Pool) *LayerStack {
	if pool == nil {
		pool = image.NewPool(8)
	}

	return &LayerStack{
		layers: make([]*Layer, 0, 4),
		base:   base,
		pool:   pool,
	}
}

// Push creates a new layer with the given blend mode, opacity, and bounds,
// and pushes it onto the stack. Returns the new layer.
// If bounds has zero or negative dimensions, uses the base image dimensions.
func (s *LayerStack) Push(blendMode BlendMode, opacity float64, bounds Bounds) (*Layer, error) {
	// Use base dimensions if bounds are invalid
	if bounds.Width <= 0 || bounds.Height <= 0 {
		baseW, baseH := s.base.Bounds()
		bounds = Bounds{
			X:      0,
			Y:      0,
			Width:  baseW,
			Height: baseH,
		}
	}

	layer, err := NewLayer(blendMode, opacity, bounds, s.pool)
	if err != nil {
		return nil, err
	}

	s.layers = append(s.layers, layer)
	return layer, nil
}

// Pop removes and composites the top layer onto the parent layer or base.
// Returns the composited buffer (either parent layer or base).
// Returns nil if the stack is empty.
func (s *LayerStack) Pop() *image.ImageBuf {
	if len(s.layers) == 0 {
		return nil
	}

	// Pop top layer
	layer := s.layers[len(s.layers)-1]
	s.layers = s.layers[:len(s.layers)-1]

	// Get destination (parent layer or base)
	var dst *image.ImageBuf
	if len(s.layers) > 0 {
		dst = s.layers[len(s.layers)-1].buffer
	} else {
		dst = s.base
	}

	// Composite layer onto destination
	compositeLayer(layer, dst)

	// Return buffer to pool
	s.pool.Put(layer.buffer)

	return dst
}

// Current returns the current drawing target (top layer or base).
// Returns the base image if the stack is empty.
func (s *LayerStack) Current() *image.ImageBuf {
	if len(s.layers) == 0 {
		return s.base
	}
	return s.layers[len(s.layers)-1].buffer
}

// CurrentBlendMode returns the blend mode of the current layer.
// Returns BlendSourceOver if the stack is empty.
func (s *LayerStack) CurrentBlendMode() BlendMode {
	if len(s.layers) == 0 {
		return BlendSourceOver
	}
	return s.layers[len(s.layers)-1].blendMode
}

// Depth returns the number of layers in the stack (not including base).
func (s *LayerStack) Depth() int {
	return len(s.layers)
}

// Clear pops all layers and returns their buffers to the pool.
func (s *LayerStack) Clear() {
	for len(s.layers) > 0 {
		layer := s.layers[len(s.layers)-1]
		s.layers = s.layers[:len(s.layers)-1]
		s.pool.Put(layer.buffer)
	}
}

// compositeLayer composites the source layer onto the destination buffer.
// Applies the layer's blend mode and opacity during composition.
func compositeLayer(src *Layer, dst *image.ImageBuf) {
	srcBuf := src.buffer
	srcW, srcH := srcBuf.Bounds()
	dstW, dstH := dst.Bounds()

	// Get blend function
	blendFunc := GetBlendFunc(src.blendMode)
	opacity := src.opacity

	// Calculate destination region
	bounds := src.bounds
	x0 := bounds.X
	y0 := bounds.Y
	x1 := x0 + srcW
	y1 := y0 + srcH

	// Clip to destination bounds
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > dstW {
		x1 = dstW
	}
	if y1 > dstH {
		y1 = dstH
	}

	// Composite each pixel
	for dy := y0; dy < y1; dy++ {
		sy := dy - bounds.Y
		if sy < 0 || sy >= srcH {
			continue
		}

		for dx := x0; dx < x1; dx++ {
			sx := dx - bounds.X
			if sx < 0 || sx >= srcW {
				continue
			}

			// Get source pixel (premultiplied)
			srcData := srcBuf.PremultipliedData()
			srcOffset := srcBuf.PixelOffset(sx, sy)
			if srcOffset < 0 {
				continue
			}
			sr := srcData[srcOffset]
			sg := srcData[srcOffset+1]
			sb := srcData[srcOffset+2]
			sa := srcData[srcOffset+3]

			// Apply opacity to source alpha
			if opacity < 1.0 {
				sa = byte(float64(sa) * opacity)
				// For premultiplied alpha, also scale RGB
				sr = byte(float64(sr) * opacity)
				sg = byte(float64(sg) * opacity)
				sb = byte(float64(sb) * opacity)
			}

			// Get destination pixel (premultiplied)
			dstData := dst.PremultipliedData()
			dstOffset := dst.PixelOffset(dx, dy)
			if dstOffset < 0 {
				continue
			}
			dr := dstData[dstOffset]
			dg := dstData[dstOffset+1]
			db := dstData[dstOffset+2]
			da := dstData[dstOffset+3]

			// Blend
			r, g, b, a := blendFunc(sr, sg, sb, sa, dr, dg, db, da)

			// Write result (using direct data access, not premultiplied)
			// Note: dst.PremultipliedData() may return the same buffer as Data()
			// for formats that are already premultiplied or have no alpha.
			// We need to write to the actual buffer and invalidate cache.
			dstRawData := dst.Data()
			dstRawData[dstOffset] = r
			dstRawData[dstOffset+1] = g
			dstRawData[dstOffset+2] = b
			dstRawData[dstOffset+3] = a
		}
	}

	// Invalidate destination's premul cache after modification
	dst.InvalidatePremulCache()
}
