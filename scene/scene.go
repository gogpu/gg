package scene

// Scene is the main retained mode container for accumulating drawing operations.
// It builds an Encoding that can be efficiently rendered or cached.
//
// Scene provides a familiar drawing API with Fill, Stroke, layers, clips,
// and transforms. All operations are recorded into an internal Encoding
// for later playback or GPU submission.
//
// Example:
//
//	scene := NewScene()
//	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), circle)
//	scene.PushLayer(BlendMultiply, 0.5, nil)
//	scene.Stroke(DefaultStrokeStyle(), IdentityAffine(), SolidBrush(gg.Blue), rect)
//	scene.PopLayer()
//	enc := scene.Encoding()
type Scene struct {
	// encoding is the root encoding that accumulates all commands
	encoding *Encoding

	// layerStack manages the hierarchy of compositing layers
	layerStack *LayerStack

	// transformStack holds the transform state stack
	transformStack []Affine

	// clipStack holds the clip state stack
	clipStack *ClipStack

	// version is incremented on each modification for cache invalidation
	version uint64

	// bounds tracks the cumulative bounding box of all content
	bounds Rect

	// currentTransform is the current combined transform
	currentTransform Affine

	// imageRegistry maps image handles to indices
	imageRegistry []*Image
}

// NewScene creates a new empty scene.
func NewScene() *Scene {
	return &Scene{
		encoding:         NewEncoding(),
		layerStack:       NewLayerStack(),
		transformStack:   make([]Affine, 0, 8),
		clipStack:        NewClipStack(),
		version:          0,
		bounds:           EmptyRect(),
		currentTransform: IdentityAffine(),
		imageRegistry:    make([]*Image, 0, 8),
	}
}

// Reset clears the scene for reuse without deallocating memory.
func (s *Scene) Reset() {
	s.encoding.Reset()
	s.layerStack.Reset()
	s.transformStack = s.transformStack[:0]
	s.clipStack.Reset()
	s.version++
	s.bounds = EmptyRect()
	s.currentTransform = IdentityAffine()
	s.imageRegistry = s.imageRegistry[:0]
}

// Fill fills a shape with the given style, transform, and brush.
func (s *Scene) Fill(style FillStyle, transform Affine, brush Brush, shape Shape) {
	if shape == nil {
		return
	}

	// Combine with current transform
	combinedTransform := s.currentTransform.Multiply(transform)

	// Get the current layer's encoding
	enc := s.currentEncoding()

	// Encode transform if not identity
	if !combinedTransform.IsIdentity() {
		enc.EncodeTransform(combinedTransform)
	}

	// Convert shape to path and encode
	path := shape.ToPath()
	if path == nil || path.IsEmpty() {
		return
	}
	s.encodeScenePath(enc, path)

	// Encode fill command
	enc.EncodeFill(brush, style)

	// Update bounds
	shapeBounds := shape.Bounds()
	if !combinedTransform.IsIdentity() {
		shapeBounds = transformBounds(shapeBounds, combinedTransform)
	}
	s.bounds = s.bounds.Union(shapeBounds)

	// Update layer bounds
	s.layerStack.Top().UpdateBounds(shapeBounds)

	s.version++
}

// Stroke strokes a shape with the given style, transform, and brush.
func (s *Scene) Stroke(style *StrokeStyle, transform Affine, brush Brush, shape Shape) {
	if shape == nil {
		return
	}
	if style == nil {
		style = DefaultStrokeStyle()
	}

	// Combine with current transform
	combinedTransform := s.currentTransform.Multiply(transform)

	// Get the current layer's encoding
	enc := s.currentEncoding()

	// Encode transform if not identity
	if !combinedTransform.IsIdentity() {
		enc.EncodeTransform(combinedTransform)
	}

	// Convert shape to path and encode
	path := shape.ToPath()
	if path == nil || path.IsEmpty() {
		return
	}
	s.encodeScenePath(enc, path)

	// Encode stroke command
	enc.EncodeStroke(brush, style)

	// Update bounds (expand by stroke width)
	shapeBounds := shape.Bounds()
	halfWidth := style.Width / 2
	shapeBounds.MinX -= halfWidth
	shapeBounds.MinY -= halfWidth
	shapeBounds.MaxX += halfWidth
	shapeBounds.MaxY += halfWidth

	if !combinedTransform.IsIdentity() {
		shapeBounds = transformBounds(shapeBounds, combinedTransform)
	}
	s.bounds = s.bounds.Union(shapeBounds)

	// Update layer bounds
	s.layerStack.Top().UpdateBounds(shapeBounds)

	s.version++
}

// DrawImage draws an image at the given transform.
func (s *Scene) DrawImage(img *Image, transform Affine) {
	if img == nil {
		return
	}

	// Register image and get index
	imageIdx := s.registerImage(img)

	// Combine with current transform
	combinedTransform := s.currentTransform.Multiply(transform)

	// Get the current layer's encoding
	enc := s.currentEncoding()

	// Encode image command
	//nolint:gosec // image index is bounded by slice length
	enc.EncodeImage(uint32(imageIdx), combinedTransform)

	// Update bounds
	imgBounds := Rect{
		MinX: 0,
		MinY: 0,
		MaxX: float32(img.Width),
		MaxY: float32(img.Height),
	}
	if !combinedTransform.IsIdentity() {
		imgBounds = transformBounds(imgBounds, combinedTransform)
	}
	s.bounds = s.bounds.Union(imgBounds)

	// Update layer bounds
	s.layerStack.Top().UpdateBounds(imgBounds)

	s.version++
}

// PushLayer pushes a new compositing layer.
// All subsequent drawing operations will be rendered to this layer.
// Call PopLayer to composite the layer with the content below.
func (s *Scene) PushLayer(blend BlendMode, alpha float32, clip Shape) {
	layer := s.layerStack.AcquireLayer()
	layer.Kind = LayerRegular
	layer.BlendMode = blend
	layer.Alpha = clampAlpha(alpha)
	layer.Clip = clip
	layer.Transform = s.currentTransform
	layer.ClipStackDepth = s.clipStack.Depth()

	// Initialize layer encoding
	if layer.Encoding == nil {
		layer.Encoding = NewEncoding()
	} else {
		layer.Encoding.Reset()
	}

	// Encode push layer in parent
	parentEnc := s.currentEncoding()
	parentEnc.EncodePushLayer(blend, alpha)

	// If there's a clip, encode it
	if clip != nil {
		path := clip.ToPath()
		if path != nil && !path.IsEmpty() {
			s.encodeScenePath(parentEnc, path)
			parentEnc.EncodeBeginClip()
		}
	}

	s.layerStack.Push(layer)
	s.version++
}

// PopLayer pops the current layer and composites it with the content below.
// Returns false if there's no layer to pop (only root layer remains).
func (s *Scene) PopLayer() bool {
	if s.layerStack.IsRoot() {
		return false
	}

	layer := s.layerStack.Pop()
	if layer == nil {
		return false
	}

	// Get parent encoding
	parentEnc := s.currentEncoding()

	// If layer had content, append it to parent
	if layer.Encoding != nil && !layer.Encoding.IsEmpty() {
		parentEnc.Append(layer.Encoding)
	}

	// If layer had a clip, end it
	if layer.Clip != nil {
		parentEnc.EncodeEndClip()
	}

	// Encode pop layer
	parentEnc.EncodePopLayer()

	// Update parent bounds
	s.layerStack.Top().UpdateBounds(layer.Bounds)

	// Return layer to pool
	s.layerStack.ReleaseLayer(layer)

	s.version++
	return true
}

// PushClip pushes a clip region. All subsequent drawing operations
// will be clipped to this shape until PopClip is called.
func (s *Scene) PushClip(shape Shape) {
	if shape == nil {
		return
	}

	clipState := NewClipState(shape, s.currentTransform)
	s.clipStack.Push(clipState)

	// Encode clip in current layer
	enc := s.currentEncoding()
	path := shape.ToPath()
	if path != nil && !path.IsEmpty() {
		// Encode transform if not identity
		if !s.currentTransform.IsIdentity() {
			enc.EncodeTransform(s.currentTransform)
		}
		s.encodeScenePath(enc, path)
		enc.EncodeBeginClip()
	}

	s.version++
}

// PopClip pops the current clip region.
// Returns false if there's no clip to pop.
func (s *Scene) PopClip() bool {
	if s.clipStack.IsEmpty() {
		return false
	}

	s.clipStack.Pop()

	// Encode end clip
	enc := s.currentEncoding()
	enc.EncodeEndClip()

	s.version++
	return true
}

// PushTransform pushes a transform onto the transform stack.
// The transform is concatenated with the current transform.
func (s *Scene) PushTransform(t Affine) {
	s.transformStack = append(s.transformStack, s.currentTransform)
	s.currentTransform = s.currentTransform.Multiply(t)
	s.version++
}

// PopTransform pops the current transform from the stack.
// Returns false if there's no transform to pop.
func (s *Scene) PopTransform() bool {
	if len(s.transformStack) == 0 {
		return false
	}
	s.currentTransform = s.transformStack[len(s.transformStack)-1]
	s.transformStack = s.transformStack[:len(s.transformStack)-1]
	s.version++
	return true
}

// SetTransform sets the current transform, replacing the existing one.
// This does not affect the transform stack.
func (s *Scene) SetTransform(t Affine) {
	s.currentTransform = t
	s.version++
}

// Transform returns the current transform.
func (s *Scene) Transform() Affine {
	return s.currentTransform
}

// Translate applies a translation to the current transform.
func (s *Scene) Translate(x, y float32) {
	s.currentTransform = s.currentTransform.Multiply(TranslateAffine(x, y))
	s.version++
}

// Scale applies a scale to the current transform.
func (s *Scene) Scale(x, y float32) {
	s.currentTransform = s.currentTransform.Multiply(ScaleAffine(x, y))
	s.version++
}

// Rotate applies a rotation to the current transform (angle in radians).
func (s *Scene) Rotate(angle float32) {
	s.currentTransform = s.currentTransform.Multiply(RotateAffine(angle))
	s.version++
}

// Encoding returns the root encoding containing all scene commands.
// This encoding can be used for rendering or caching.
func (s *Scene) Encoding() *Encoding {
	// Flatten all layers into root encoding
	s.flattenLayers()
	return s.encoding
}

// Bounds returns the cumulative bounding box of all scene content.
func (s *Scene) Bounds() Rect {
	return s.bounds
}

// Version returns the scene version number.
// This is incremented on each modification and can be used for cache invalidation.
func (s *Scene) Version() uint64 {
	return s.version
}

// IsEmpty returns true if the scene has no content.
func (s *Scene) IsEmpty() bool {
	return s.encoding.IsEmpty() && s.layerStack.Root().IsEmpty()
}

// LayerDepth returns the current layer stack depth.
func (s *Scene) LayerDepth() int {
	return s.layerStack.Depth()
}

// ClipDepth returns the current clip stack depth.
func (s *Scene) ClipDepth() int {
	return s.clipStack.Depth()
}

// TransformDepth returns the current transform stack depth.
func (s *Scene) TransformDepth() int {
	return len(s.transformStack)
}

// ClipBounds returns the intersection of all active clip regions.
func (s *Scene) ClipBounds() Rect {
	return s.clipStack.CombinedBounds()
}

// currentEncoding returns the encoding for the current layer.
func (s *Scene) currentEncoding() *Encoding {
	layer := s.layerStack.Top()
	if layer.Encoding != nil {
		return layer.Encoding
	}
	return s.encoding
}

// encodeScenePath encodes a scene Path to an Encoding.
func (s *Scene) encodeScenePath(enc *Encoding, path *Path) {
	if path == nil || path.IsEmpty() {
		return
	}

	enc.tags = append(enc.tags, TagBeginPath)
	enc.pathBounds = EmptyRect()
	enc.pathCount++

	pointIdx := 0
	for _, verb := range path.verbs {
		switch verb {
		case VerbMoveTo:
			enc.tags = append(enc.tags, TagMoveTo)
			x, y := path.points[pointIdx], path.points[pointIdx+1]
			enc.pathData = append(enc.pathData, x, y)
			enc.pathBounds = enc.pathBounds.UnionPoint(x, y)
			pointIdx += 2

		case VerbLineTo:
			enc.tags = append(enc.tags, TagLineTo)
			x, y := path.points[pointIdx], path.points[pointIdx+1]
			enc.pathData = append(enc.pathData, x, y)
			enc.pathBounds = enc.pathBounds.UnionPoint(x, y)
			pointIdx += 2

		case VerbQuadTo:
			enc.tags = append(enc.tags, TagQuadTo)
			cx, cy := path.points[pointIdx], path.points[pointIdx+1]
			x, y := path.points[pointIdx+2], path.points[pointIdx+3]
			enc.pathData = append(enc.pathData, cx, cy, x, y)
			enc.pathBounds = enc.pathBounds.UnionPoint(cx, cy)
			enc.pathBounds = enc.pathBounds.UnionPoint(x, y)
			pointIdx += 4

		case VerbCubicTo:
			enc.tags = append(enc.tags, TagCubicTo)
			c1x, c1y := path.points[pointIdx], path.points[pointIdx+1]
			c2x, c2y := path.points[pointIdx+2], path.points[pointIdx+3]
			x, y := path.points[pointIdx+4], path.points[pointIdx+5]
			enc.pathData = append(enc.pathData, c1x, c1y, c2x, c2y, x, y)
			enc.pathBounds = enc.pathBounds.UnionPoint(c1x, c1y)
			enc.pathBounds = enc.pathBounds.UnionPoint(c2x, c2y)
			enc.pathBounds = enc.pathBounds.UnionPoint(x, y)
			pointIdx += 6

		case VerbClose:
			enc.tags = append(enc.tags, TagClosePath)
		}
	}

	enc.tags = append(enc.tags, TagEndPath)
	enc.bounds = enc.bounds.Union(enc.pathBounds)
}

// flattenLayers collapses all layer content into the root encoding.
// This is called when Encoding() is requested.
func (s *Scene) flattenLayers() {
	// If we're already at root level and root has content, nothing to do
	if s.layerStack.IsRoot() {
		rootLayer := s.layerStack.Root()
		if rootLayer.Encoding != nil && !rootLayer.Encoding.IsEmpty() {
			s.encoding.Append(rootLayer.Encoding)
			rootLayer.Encoding.Reset()
		}
		return
	}

	// Pop all layers and merge into root
	for !s.layerStack.IsRoot() {
		_ = s.PopLayer()
	}

	// Merge root layer content
	rootLayer := s.layerStack.Root()
	if rootLayer.Encoding != nil && !rootLayer.Encoding.IsEmpty() {
		s.encoding.Append(rootLayer.Encoding)
		rootLayer.Encoding.Reset()
	}
}

// registerImage adds an image to the registry and returns its index.
func (s *Scene) registerImage(img *Image) int {
	// Check if already registered
	for i, registered := range s.imageRegistry {
		if registered == img {
			return i
		}
	}
	// Add new image
	idx := len(s.imageRegistry)
	s.imageRegistry = append(s.imageRegistry, img)
	return idx
}

// Images returns all registered images.
func (s *Scene) Images() []*Image {
	return s.imageRegistry
}

// transformBounds transforms a bounding rectangle by an affine transform.
func transformBounds(bounds Rect, transform Affine) Rect {
	if bounds.IsEmpty() {
		return bounds
	}

	// Transform all four corners
	corners := [][2]float32{
		{bounds.MinX, bounds.MinY},
		{bounds.MaxX, bounds.MinY},
		{bounds.MaxX, bounds.MaxY},
		{bounds.MinX, bounds.MaxY},
	}

	result := EmptyRect()
	for _, c := range corners {
		x, y := transform.TransformPoint(c[0], c[1])
		result = result.UnionPoint(x, y)
	}

	return result
}

// Image represents an image resource for drawing.
// This is a placeholder that will be expanded during integration.
type Image struct {
	// Width is the image width in pixels
	Width int

	// Height is the image height in pixels
	Height int

	// Data holds the pixel data (RGBA format)
	// This will be populated during integration phase
	Data []byte
}

// NewImage creates a new image with the given dimensions.
func NewImage(width, height int) *Image {
	return &Image{
		Width:  width,
		Height: height,
	}
}

// Bounds returns the image bounds as a Rect.
func (img *Image) Bounds() Rect {
	return Rect{
		MinX: 0,
		MinY: 0,
		MaxX: float32(img.Width),
		MaxY: float32(img.Height),
	}
}

// IsEmpty returns true if the image has no dimensions.
func (img *Image) IsEmpty() bool {
	return img.Width <= 0 || img.Height <= 0
}

// ScenePool manages a pool of reusable Scene objects.
type ScenePool struct {
	scenes []*Scene
}

// NewScenePool creates a new scene pool.
func NewScenePool() *ScenePool {
	return &ScenePool{
		scenes: make([]*Scene, 0, 4),
	}
}

// Get retrieves a scene from the pool or creates a new one.
func (sp *ScenePool) Get() *Scene {
	if len(sp.scenes) > 0 {
		scene := sp.scenes[len(sp.scenes)-1]
		sp.scenes = sp.scenes[:len(sp.scenes)-1]
		scene.Reset()
		return scene
	}
	return NewScene()
}

// Put returns a scene to the pool for reuse.
func (sp *ScenePool) Put(scene *Scene) {
	if scene == nil {
		return
	}
	sp.scenes = append(sp.scenes, scene)
}

// Warmup pre-allocates scenes to avoid allocation during critical paths.
func (sp *ScenePool) Warmup(count int) {
	scenes := make([]*Scene, count)
	for i := 0; i < count; i++ {
		scenes[i] = sp.Get()
	}
	for i := 0; i < count; i++ {
		sp.Put(scenes[i])
	}
}
