package scene

// LayerKind identifies the type of compositing layer.
type LayerKind uint8

// Layer kind constants.
const (
	// LayerRegular is a normal blend layer with alpha and blend mode.
	LayerRegular LayerKind = iota

	// LayerFiltered is a layer that will have filter effects applied.
	// This layer's contents are rendered to an offscreen buffer for processing.
	LayerFiltered

	// LayerClip is a clip-only layer that masks subsequent content.
	// No rendering is done to this layer; it only defines a clip region.
	LayerClip
)

// String returns a human-readable name for the layer kind.
func (k LayerKind) String() string {
	switch k {
	case LayerRegular:
		return "Regular"
	case LayerFiltered:
		return "Filtered"
	case LayerClip:
		return "Clip"
	default:
		return unknownStr
	}
}

// NeedsOffscreen returns true if this layer kind requires an offscreen buffer.
func (k LayerKind) NeedsOffscreen() bool {
	return k == LayerFiltered
}

// IsClipOnly returns true if this layer is only for clipping (no rendering).
func (k LayerKind) IsClipOnly() bool {
	return k == LayerClip
}

// LayerState represents the state of an active compositing layer.
// Each layer has its own encoding for isolated rendering.
type LayerState struct {
	// Kind identifies the type of layer
	Kind LayerKind

	// BlendMode specifies how this layer composites with layers below
	BlendMode BlendMode

	// Alpha is the layer opacity (0.0 to 1.0)
	Alpha float32

	// Clip is the optional clip shape for this layer.
	// If nil, the layer has no clip (infinite bounds).
	Clip Shape

	// Encoding holds the layer's drawing commands.
	// This is populated as drawing commands are added to the scene
	// while this layer is active.
	Encoding *Encoding

	// Bounds tracks the cumulative bounding box of layer content.
	// This is updated as content is added.
	Bounds Rect

	// Transform is the transform active when the layer was pushed.
	// Used to restore transform state on pop.
	Transform Affine

	// ClipStackDepth records the clip stack depth when layer was pushed.
	// Used to restore clip state on pop.
	ClipStackDepth int
}

// NewLayerState creates a new layer state with default values.
func NewLayerState(kind LayerKind, blend BlendMode, alpha float32) *LayerState {
	return &LayerState{
		Kind:      kind,
		BlendMode: blend,
		Alpha:     clampAlpha(alpha),
		Encoding:  NewEncoding(),
		Bounds:    EmptyRect(),
		Transform: IdentityAffine(),
	}
}

// NewClipLayer creates a new clip-only layer.
func NewClipLayer(clip Shape) *LayerState {
	return &LayerState{
		Kind:      LayerClip,
		BlendMode: BlendNormal,
		Alpha:     1.0,
		Clip:      clip,
		Encoding:  nil, // Clip layers don't need their own encoding
		Bounds:    EmptyRect(),
		Transform: IdentityAffine(),
	}
}

// NewFilteredLayer creates a new layer for filter effects.
func NewFilteredLayer(blend BlendMode, alpha float32) *LayerState {
	return &LayerState{
		Kind:      LayerFiltered,
		BlendMode: blend,
		Alpha:     clampAlpha(alpha),
		Encoding:  NewEncoding(),
		Bounds:    EmptyRect(),
		Transform: IdentityAffine(),
	}
}

// Reset clears the layer state for reuse.
func (ls *LayerState) Reset() {
	ls.Kind = LayerRegular
	ls.BlendMode = BlendNormal
	ls.Alpha = 1.0
	ls.Clip = nil
	if ls.Encoding != nil {
		ls.Encoding.Reset()
	}
	ls.Bounds = EmptyRect()
	ls.Transform = IdentityAffine()
	ls.ClipStackDepth = 0
}

// UpdateBounds expands the layer bounds to include the given rectangle.
func (ls *LayerState) UpdateBounds(r Rect) {
	ls.Bounds = ls.Bounds.Union(r)
}

// IsEmpty returns true if the layer has no content.
func (ls *LayerState) IsEmpty() bool {
	if ls.Kind == LayerClip {
		return ls.Clip == nil
	}
	return ls.Encoding == nil || ls.Encoding.IsEmpty()
}

// HasClip returns true if the layer has a clip shape.
func (ls *LayerState) HasClip() bool {
	return ls.Clip != nil
}

// clampAlpha clamps alpha to [0, 1] range.
func clampAlpha(alpha float32) float32 {
	if alpha < 0 {
		return 0
	}
	if alpha > 1 {
		return 1
	}
	return alpha
}

// LayerStack manages a stack of active layers.
// The stack always has at least one layer (the root layer).
type LayerStack struct {
	layers []*LayerState
	pool   *layerPool
}

// NewLayerStack creates a new layer stack with a root layer.
func NewLayerStack() *LayerStack {
	stack := &LayerStack{
		layers: make([]*LayerState, 0, 8),
		pool:   newLayerPool(),
	}
	// Create root layer
	root := stack.pool.get()
	root.Kind = LayerRegular
	root.BlendMode = BlendNormal
	root.Alpha = 1.0
	stack.layers = append(stack.layers, root)
	return stack
}

// Push adds a new layer to the stack.
func (s *LayerStack) Push(layer *LayerState) {
	s.layers = append(s.layers, layer)
}

// Pop removes and returns the top layer.
// Returns nil if only the root layer remains.
func (s *LayerStack) Pop() *LayerState {
	if len(s.layers) <= 1 {
		return nil // Cannot pop root layer
	}
	layer := s.layers[len(s.layers)-1]
	s.layers = s.layers[:len(s.layers)-1]
	return layer
}

// Top returns the current (topmost) layer without removing it.
func (s *LayerStack) Top() *LayerState {
	if len(s.layers) == 0 {
		return nil
	}
	return s.layers[len(s.layers)-1]
}

// Root returns the root (bottom) layer.
func (s *LayerStack) Root() *LayerState {
	if len(s.layers) == 0 {
		return nil
	}
	return s.layers[0]
}

// Depth returns the current stack depth (1 = only root).
func (s *LayerStack) Depth() int {
	return len(s.layers)
}

// IsRoot returns true if only the root layer is on the stack.
func (s *LayerStack) IsRoot() bool {
	return len(s.layers) == 1
}

// Reset clears the stack, returning to just the root layer.
func (s *LayerStack) Reset() {
	// Return all non-root layers to pool
	for i := len(s.layers) - 1; i > 0; i-- {
		s.pool.put(s.layers[i])
	}
	s.layers = s.layers[:1]

	// Reset root layer
	if len(s.layers) > 0 {
		s.layers[0].Reset()
	}
}

// All returns all layers in the stack (bottom to top).
func (s *LayerStack) All() []*LayerState {
	return s.layers
}

// AcquireLayer gets a layer from the pool.
func (s *LayerStack) AcquireLayer() *LayerState {
	return s.pool.get()
}

// ReleaseLayer returns a layer to the pool.
func (s *LayerStack) ReleaseLayer(layer *LayerState) {
	s.pool.put(layer)
}

// layerPool manages a pool of reusable LayerState objects.
type layerPool struct {
	layers []*LayerState
}

// newLayerPool creates a new layer pool.
func newLayerPool() *layerPool {
	return &layerPool{
		layers: make([]*LayerState, 0, 8),
	}
}

// get retrieves a layer from the pool or creates a new one.
func (p *layerPool) get() *LayerState {
	if len(p.layers) > 0 {
		layer := p.layers[len(p.layers)-1]
		p.layers = p.layers[:len(p.layers)-1]
		layer.Reset()
		return layer
	}
	return &LayerState{
		Encoding:  NewEncoding(),
		Bounds:    EmptyRect(),
		Transform: IdentityAffine(),
	}
}

// put returns a layer to the pool.
func (p *layerPool) put(layer *LayerState) {
	if layer == nil {
		return
	}
	layer.Reset()
	p.layers = append(p.layers, layer)
}

// ClipState represents a clip region on the clip stack.
type ClipState struct {
	// Shape is the clip shape
	Shape Shape

	// Bounds is the clip bounds (for quick rejection)
	Bounds Rect

	// Transform is the transform that was active when clip was pushed
	Transform Affine
}

// NewClipState creates a new clip state.
func NewClipState(shape Shape, transform Affine) *ClipState {
	var bounds Rect
	if shape != nil {
		bounds = shape.Bounds()
		// Transform bounds if needed
		if !transform.IsIdentity() {
			// Transform all four corners and compute new bounds
			corners := [][2]float32{
				{bounds.MinX, bounds.MinY},
				{bounds.MaxX, bounds.MinY},
				{bounds.MaxX, bounds.MaxY},
				{bounds.MinX, bounds.MaxY},
			}
			bounds = EmptyRect()
			for _, c := range corners {
				x, y := transform.TransformPoint(c[0], c[1])
				bounds = bounds.UnionPoint(x, y)
			}
		}
	} else {
		bounds = EmptyRect()
	}

	return &ClipState{
		Shape:     shape,
		Bounds:    bounds,
		Transform: transform,
	}
}

// Contains returns true if the point is inside the clip region.
// This is a conservative test using the bounding box.
func (cs *ClipState) Contains(x, y float32) bool {
	if cs.Bounds.IsEmpty() {
		return false
	}
	return x >= cs.Bounds.MinX && x <= cs.Bounds.MaxX &&
		y >= cs.Bounds.MinY && y <= cs.Bounds.MaxY
}

// Intersects returns true if the rectangle intersects the clip region.
// This is a conservative test using bounding boxes.
func (cs *ClipState) Intersects(r Rect) bool {
	if cs.Bounds.IsEmpty() || r.IsEmpty() {
		return false
	}
	return !(r.MaxX < cs.Bounds.MinX || r.MinX > cs.Bounds.MaxX ||
		r.MaxY < cs.Bounds.MinY || r.MinY > cs.Bounds.MaxY)
}

// ClipStack manages a stack of clip regions.
type ClipStack struct {
	clips []*ClipState
}

// NewClipStack creates a new empty clip stack.
func NewClipStack() *ClipStack {
	return &ClipStack{
		clips: make([]*ClipState, 0, 4),
	}
}

// Push adds a clip region to the stack.
func (s *ClipStack) Push(clip *ClipState) {
	s.clips = append(s.clips, clip)
}

// Pop removes and returns the top clip region.
func (s *ClipStack) Pop() *ClipState {
	if len(s.clips) == 0 {
		return nil
	}
	clip := s.clips[len(s.clips)-1]
	s.clips = s.clips[:len(s.clips)-1]
	return clip
}

// Top returns the current clip region without removing it.
func (s *ClipStack) Top() *ClipState {
	if len(s.clips) == 0 {
		return nil
	}
	return s.clips[len(s.clips)-1]
}

// Depth returns the current stack depth.
func (s *ClipStack) Depth() int {
	return len(s.clips)
}

// IsEmpty returns true if there are no clips on the stack.
func (s *ClipStack) IsEmpty() bool {
	return len(s.clips) == 0
}

// Reset clears the clip stack.
func (s *ClipStack) Reset() {
	s.clips = s.clips[:0]
}

// CombinedBounds returns the intersection of all clip bounds.
// Returns an empty rect if any clip is empty.
func (s *ClipStack) CombinedBounds() Rect {
	if len(s.clips) == 0 {
		return Rect{
			MinX: -1e30,
			MinY: -1e30,
			MaxX: 1e30,
			MaxY: 1e30,
		} // No clip = infinite bounds
	}

	result := s.clips[0].Bounds
	for i := 1; i < len(s.clips); i++ {
		// Intersect bounds
		result = Rect{
			MinX: max32(result.MinX, s.clips[i].Bounds.MinX),
			MinY: max32(result.MinY, s.clips[i].Bounds.MinY),
			MaxX: min32(result.MaxX, s.clips[i].Bounds.MaxX),
			MaxY: min32(result.MaxY, s.clips[i].Bounds.MaxY),
		}
		if result.IsEmpty() {
			return EmptyRect()
		}
	}

	return result
}

// Contains returns true if the point is inside all clip regions.
func (s *ClipStack) Contains(x, y float32) bool {
	for _, clip := range s.clips {
		if !clip.Contains(x, y) {
			return false
		}
	}
	return true
}

// Intersects returns true if the rectangle intersects all clip regions.
func (s *ClipStack) Intersects(r Rect) bool {
	for _, clip := range s.clips {
		if !clip.Intersects(r) {
			return false
		}
	}
	return true
}
