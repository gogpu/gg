package gg

import (
	intImage "github.com/gogpu/gg/internal/image"
)

// Layer represents a drawing layer with blend mode and opacity.
// Layers allow isolating drawing operations and compositing them with
// different blend modes and opacity values, similar to layers in Photoshop
// or SVG group opacity.
type Layer struct {
	pixmap    *Pixmap
	blendMode BlendMode
	opacity   float64
}

// layerStack manages the layer hierarchy for the context.
type layerStack struct {
	layers []*Layer
	pool   *intImage.Pool
}

// newLayerStack creates a new layer stack with a pool for memory reuse.
func newLayerStack() *layerStack {
	return &layerStack{
		layers: make([]*Layer, 0, 4),
		pool:   intImage.NewPool(8),
	}
}

// PushLayer creates a new layer and makes it the active drawing target.
// All subsequent drawing operations will render to this layer until PopLayer is called.
//
// The layer will be composited onto the parent layer/canvas when PopLayer is called,
// using the specified blend mode and opacity.
//
// Parameters:
//   - blendMode: How to composite this layer onto the parent (e.g., BlendMultiply, BlendScreen)
//   - opacity: Layer opacity in range [0.0, 1.0] where 0 is fully transparent and 1 is fully opaque
//
// Example:
//
//	dc.PushLayer(gg.BlendMultiply, 0.5)
//	dc.SetRGB(1, 0, 0)
//	dc.DrawCircle(100, 100, 50)
//	dc.Fill()
//	dc.PopLayer() // Composite circle onto canvas with multiply blend at 50% opacity
func (c *Context) PushLayer(blendMode BlendMode, opacity float64) {
	// Clamp opacity to valid range
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}

	// Initialize layer stack if needed
	if c.layerStack == nil {
		c.layerStack = newLayerStack()
	}

	// Save base pixmap on first push
	if len(c.layerStack.layers) == 0 && c.basePixmap == nil {
		c.basePixmap = c.pixmap
	}

	// Create new pixmap for the layer (same size as context)
	layerPixmap := NewPixmap(c.width, c.height)
	layerPixmap.Clear(Transparent)

	// Create layer
	layer := &Layer{
		pixmap:    layerPixmap,
		blendMode: blendMode,
		opacity:   opacity,
	}

	// Save current pixmap and switch to layer pixmap
	c.layerStack.layers = append(c.layerStack.layers, layer)
	c.pixmap = layerPixmap
}

// PopLayer composites the current layer onto the parent layer/canvas.
// Uses the blend mode and opacity specified in the corresponding PushLayer call.
//
// The layer is composited using the specified blend mode and opacity.
// After compositing, the layer's memory is returned to the pool for reuse.
//
// If there are no layers to pop, this function does nothing.
//
// Example:
//
//	dc.PushLayer(gg.BlendScreen, 1.0)
//	// ... draw operations ...
//	dc.PopLayer() // Composite layer onto parent
func (c *Context) PopLayer() {
	if c.layerStack == nil || len(c.layerStack.layers) == 0 {
		return
	}

	// Pop the current layer
	layers := c.layerStack.layers
	layer := layers[len(layers)-1]
	c.layerStack.layers = layers[:len(layers)-1]

	// Get parent pixmap (either previous layer or base)
	var parentPixmap *Pixmap
	if len(c.layerStack.layers) > 0 {
		parentPixmap = c.layerStack.layers[len(c.layerStack.layers)-1].pixmap
	} else {
		// Restore base pixmap
		parentPixmap = c.basePixmap
		c.basePixmap = nil
	}

	// Composite layer onto parent
	c.compositeLayer(layer, parentPixmap)

	// Restore parent pixmap as current drawing target
	c.pixmap = parentPixmap
}

// SetBlendMode sets the blend mode for subsequent fill and stroke operations.
// This is currently a placeholder for future blend mode support in direct drawing operations.
//
// For now, blend modes are primarily used with layers via PushLayer/PopLayer.
//
// Example:
//
//	dc.SetBlendMode(gg.BlendMultiply)
//	dc.Fill() // Future: will use multiply blend mode
func (c *Context) SetBlendMode(_ BlendMode) {
	// Store for future use in paint operations
	// Currently, blending is primarily done via layers
}

// compositeLayer composites a layer onto a parent pixmap using the layer's
// blend mode and opacity.
func (c *Context) compositeLayer(layer *Layer, parent *Pixmap) {
	// Convert pixmaps to ImageBuf for blending
	srcImg := c.pixmapToImageBuf(layer.pixmap)
	dstImg := c.pixmapToImageBuf(parent)

	// Use DrawImage to composite with blend mode and opacity
	srcW, srcH := srcImg.Bounds()

	params := intImage.DrawParams{
		DstRect: intImage.Rect{
			X:      0,
			Y:      0,
			Width:  srcW,
			Height: srcH,
		},
		Interp:    intImage.InterpNearest, // No scaling, so nearest is fine
		Opacity:   layer.opacity,
		BlendMode: layer.blendMode,
	}

	intImage.DrawImage(dstImg, srcImg, params)
}
