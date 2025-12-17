package scene

// SceneBuilder provides a fluent API for constructing scenes ergonomically.
// It wraps a Scene and provides chainable methods for drawing operations,
// transform management, and layer composition.
//
// The builder maintains its own transform state that accumulates with each
// transform operation. Use ResetTransform to clear the accumulated transform.
//
// Example:
//
//	scene := NewSceneBuilder().
//	    Fill(NewRectShape(0, 0, 800, 600), SolidBrush(White)).
//	    Translate(100, 100).
//	    Layer(BlendMultiply, 0.8, nil, func(b *SceneBuilder) {
//	        b.Fill(NewCircleShape(0, 0, 50), SolidBrush(Red)).
//	          Stroke(NewCircleShape(0, 0, 50), SolidBrush(Black), 2)
//	    }).
//	    Build()
type SceneBuilder struct {
	scene     *Scene
	transform Affine
}

// NewSceneBuilder creates a new scene builder with an empty scene.
func NewSceneBuilder() *SceneBuilder {
	return &SceneBuilder{
		scene:     NewScene(),
		transform: IdentityAffine(),
	}
}

// NewSceneBuilderFrom creates a scene builder wrapping an existing scene.
// This is useful for adding to a scene that already has content.
func NewSceneBuilderFrom(scene *Scene) *SceneBuilder {
	if scene == nil {
		scene = NewScene()
	}
	return &SceneBuilder{
		scene:     scene,
		transform: scene.Transform(),
	}
}

// ---------------------------------------------------------------------------
// Drawing Operations
// ---------------------------------------------------------------------------

// Fill fills a shape with the given brush using the non-zero winding rule.
func (b *SceneBuilder) Fill(shape Shape, brush Brush) *SceneBuilder {
	b.scene.Fill(FillNonZero, b.transform, brush, shape)
	return b
}

// FillWith fills a shape with the given brush and fill style.
func (b *SceneBuilder) FillWith(shape Shape, brush Brush, style FillStyle) *SceneBuilder {
	b.scene.Fill(style, b.transform, brush, shape)
	return b
}

// Stroke strokes a shape with the given brush and line width.
func (b *SceneBuilder) Stroke(shape Shape, brush Brush, width float32) *SceneBuilder {
	style := &StrokeStyle{
		Width:      width,
		MiterLimit: 10.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
	}
	b.scene.Stroke(style, b.transform, brush, shape)
	return b
}

// StrokeWith strokes a shape with the given brush and stroke style.
func (b *SceneBuilder) StrokeWith(shape Shape, brush Brush, style *StrokeStyle) *SceneBuilder {
	b.scene.Stroke(style, b.transform, brush, shape)
	return b
}

// Image draws an image at the given rectangle.
// The image is scaled to fit the rectangle bounds.
func (b *SceneBuilder) Image(img *Image, rect Rect) *SceneBuilder {
	if img == nil || img.IsEmpty() || rect.IsEmpty() {
		return b
	}

	// Calculate scale to fit image in rect
	scaleX := rect.Width() / float32(img.Width)
	scaleY := rect.Height() / float32(img.Height)

	// Create transform: translate to rect position, then scale
	imgTransform := b.transform.
		Multiply(TranslateAffine(rect.MinX, rect.MinY)).
		Multiply(ScaleAffine(scaleX, scaleY))

	b.scene.DrawImage(img, imgTransform)
	return b
}

// ---------------------------------------------------------------------------
// Transform Operations
// ---------------------------------------------------------------------------

// Transform sets the current transform, replacing any existing transform.
func (b *SceneBuilder) Transform(t Affine) *SceneBuilder {
	b.transform = t
	return b
}

// Translate applies a translation to the current transform.
// Translations accumulate - use ResetTransform to clear.
func (b *SceneBuilder) Translate(x, y float32) *SceneBuilder {
	b.transform = b.transform.Multiply(TranslateAffine(x, y))
	return b
}

// Scale applies a scale to the current transform.
// Scales accumulate - use ResetTransform to clear.
func (b *SceneBuilder) Scale(x, y float32) *SceneBuilder {
	b.transform = b.transform.Multiply(ScaleAffine(x, y))
	return b
}

// Rotate applies a rotation to the current transform (angle in radians).
// Rotations accumulate - use ResetTransform to clear.
func (b *SceneBuilder) Rotate(angle float32) *SceneBuilder {
	b.transform = b.transform.Multiply(RotateAffine(angle))
	return b
}

// ResetTransform resets the current transform to identity.
func (b *SceneBuilder) ResetTransform() *SceneBuilder {
	b.transform = IdentityAffine()
	return b
}

// ---------------------------------------------------------------------------
// Layer Operations
// ---------------------------------------------------------------------------

// Layer creates a compositing layer with the given blend mode and alpha.
// The callback receives the same builder to add content to the layer.
// If clip is not nil, the layer content is clipped to that shape.
//
// Example:
//
//	builder.Layer(BlendMultiply, 0.8, nil, func(b *SceneBuilder) {
//	    b.Fill(circle, SolidBrush(Red))
//	})
func (b *SceneBuilder) Layer(blend BlendMode, alpha float32, clip Shape, fn func(*SceneBuilder)) *SceneBuilder {
	if fn == nil {
		return b
	}

	// Save current transform
	savedTransform := b.transform

	// Push layer on scene
	b.scene.PushLayer(blend, alpha, clip)

	// Call the callback with this builder
	fn(b)

	// Pop the layer
	b.scene.PopLayer()

	// Restore transform
	b.transform = savedTransform

	return b
}

// Clip creates a clipping region from the given shape.
// The callback receives the same builder to add clipped content.
//
// Example:
//
//	builder.Clip(circle, func(b *SceneBuilder) {
//	    b.Fill(largeRect, SolidBrush(Blue)) // clipped to circle
//	})
func (b *SceneBuilder) Clip(shape Shape, fn func(*SceneBuilder)) *SceneBuilder {
	if fn == nil || shape == nil {
		return b
	}

	// Save current transform
	savedTransform := b.transform

	// Push clip on scene
	b.scene.PushClip(shape)

	// Call the callback with this builder
	fn(b)

	// Pop the clip
	b.scene.PopClip()

	// Restore transform
	b.transform = savedTransform

	return b
}

// Group creates a logical grouping of drawing operations.
// This is primarily for organizational purposes; no blending or clipping is applied.
// The callback receives the same builder.
//
// Example:
//
//	builder.Group(func(b *SceneBuilder) {
//	    b.Fill(rect1, SolidBrush(Red))
//	    b.Fill(rect2, SolidBrush(Blue))
//	})
func (b *SceneBuilder) Group(fn func(*SceneBuilder)) *SceneBuilder {
	if fn == nil {
		return b
	}

	// Save current transform
	savedTransform := b.transform

	// Call the callback directly (no layer push, just grouping)
	fn(b)

	// Restore transform
	b.transform = savedTransform

	return b
}

// WithTransform executes the callback with a temporary transform applied.
// The transform is reset after the callback completes.
//
// Example:
//
//	builder.WithTransform(TranslateAffine(100, 100), func(b *SceneBuilder) {
//	    b.Fill(shape, brush) // drawn at (100, 100)
//	})
func (b *SceneBuilder) WithTransform(t Affine, fn func(*SceneBuilder)) *SceneBuilder {
	if fn == nil {
		return b
	}

	// Save current transform
	savedTransform := b.transform

	// Apply temporary transform
	b.transform = b.transform.Multiply(t)

	// Call the callback
	fn(b)

	// Restore transform
	b.transform = savedTransform

	return b
}

// ---------------------------------------------------------------------------
// Building
// ---------------------------------------------------------------------------

// Build returns the constructed scene and resets the builder for reuse.
// The builder is left in a clean state with a new empty scene.
func (b *SceneBuilder) Build() *Scene {
	result := b.scene
	b.scene = NewScene()
	b.transform = IdentityAffine()
	return result
}

// Reset clears the builder's scene and transform for reuse.
// Unlike Build(), this does not return the scene.
func (b *SceneBuilder) Reset() *SceneBuilder {
	b.scene.Reset()
	b.transform = IdentityAffine()
	return b
}

// Scene returns the current scene without resetting the builder.
// Use this to inspect the scene during construction.
func (b *SceneBuilder) Scene() *Scene {
	return b.scene
}

// CurrentTransform returns the builder's current transform.
func (b *SceneBuilder) CurrentTransform() Affine {
	return b.transform
}

// ---------------------------------------------------------------------------
// Convenience Methods
// ---------------------------------------------------------------------------

// FillRect is a convenience method to fill a rectangle.
func (b *SceneBuilder) FillRect(x, y, width, height float32, brush Brush) *SceneBuilder {
	return b.Fill(NewRectShape(x, y, width, height), brush)
}

// StrokeRect is a convenience method to stroke a rectangle.
func (b *SceneBuilder) StrokeRect(x, y, width, height float32, brush Brush, lineWidth float32) *SceneBuilder {
	return b.Stroke(NewRectShape(x, y, width, height), brush, lineWidth)
}

// FillCircle is a convenience method to fill a circle.
func (b *SceneBuilder) FillCircle(cx, cy, r float32, brush Brush) *SceneBuilder {
	return b.Fill(NewCircleShape(cx, cy, r), brush)
}

// StrokeCircle is a convenience method to stroke a circle.
func (b *SceneBuilder) StrokeCircle(cx, cy, r float32, brush Brush, lineWidth float32) *SceneBuilder {
	return b.Stroke(NewCircleShape(cx, cy, r), brush, lineWidth)
}

// DrawLine is a convenience method to draw a line.
func (b *SceneBuilder) DrawLine(x1, y1, x2, y2 float32, brush Brush, lineWidth float32) *SceneBuilder {
	return b.Stroke(NewLineShape(x1, y1, x2, y2), brush, lineWidth)
}

// FillPath is a convenience method to fill a path wrapped as a shape.
func (b *SceneBuilder) FillPath(path *Path, brush Brush) *SceneBuilder {
	return b.Fill(NewPathShape(path), brush)
}

// StrokePath is a convenience method to stroke a path wrapped as a shape.
func (b *SceneBuilder) StrokePath(path *Path, brush Brush, lineWidth float32) *SceneBuilder {
	return b.Stroke(NewPathShape(path), brush, lineWidth)
}
