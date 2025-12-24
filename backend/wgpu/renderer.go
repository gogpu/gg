package wgpu

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// Renderer-specific errors.
var (
	// ErrRendererClosed is returned when operating on a closed renderer.
	ErrRendererClosed = errors.New("wgpu: renderer closed")

	// ErrEmptyScene is returned when rendering an empty scene.
	ErrEmptyScene = errors.New("wgpu: empty scene")

	// ErrLayerStackUnderflow is returned when popping more layers than pushed.
	ErrLayerStackUnderflow = errors.New("wgpu: layer stack underflow")
)

// GPUSceneRenderer renders scenes using GPU acceleration.
// It implements the full render pipeline: scene decoding, path tessellation,
// strip rasterization, and layer compositing.
//
// GPUSceneRenderer is safe for concurrent use from multiple goroutines.
type GPUSceneRenderer struct {
	mu sync.Mutex

	// Backend reference
	backend *WGPUBackend

	// Render target dimensions
	width  int
	height int

	// Pipeline cache
	pipelines *PipelineCache

	// Memory manager for texture allocation
	memory *MemoryManager

	// Tessellator pool for path conversion
	tessellatorPool *TessellatorPool

	// Working textures
	targetTex  *GPUTexture   // Final render target
	layerStack []*GPUTexture // Layer texture stack

	// Strip buffer for current path
	stripBuffer *StripBuffer

	// Current transform for rendering
	currentTransform scene.Affine

	// Clip state
	clipStack []clipState

	// State
	closed bool
}

// clipState represents a clipping region.
type clipState struct {
	texture   *GPUTexture
	transform scene.Affine
}

// GPUSceneRendererConfig holds configuration for creating a GPUSceneRenderer.
type GPUSceneRendererConfig struct {
	// Width is the render target width in pixels.
	Width int

	// Height is the render target height in pixels.
	Height int

	// MaxLayers is the maximum layer stack depth (default: 16).
	MaxLayers int

	// MemoryBudgetMB is the texture memory budget in MB (default: 128).
	MemoryBudgetMB int
}

// NewGPUSceneRenderer creates a new GPU scene renderer.
// The renderer is configured for the specified dimensions.
//
// Returns an error if the backend is not initialized or configuration is invalid.
func NewGPUSceneRenderer(backend *WGPUBackend, config GPUSceneRendererConfig) (*GPUSceneRenderer, error) {
	if backend == nil {
		return nil, ErrNotInitialized
	}

	if !backend.IsInitialized() {
		return nil, ErrNotInitialized
	}

	if config.Width <= 0 || config.Height <= 0 {
		return nil, ErrInvalidDimensions
	}

	// Apply defaults
	if config.MaxLayers <= 0 {
		config.MaxLayers = 16
	}
	if config.MemoryBudgetMB <= 0 {
		config.MemoryBudgetMB = 128
	}

	// Create memory manager
	memory := NewMemoryManager(backend, MemoryManagerConfig{
		MaxMemoryMB: config.MemoryBudgetMB,
	})

	// Compile shaders and create pipeline cache
	shaders, err := CompileShaders(uint64(backend.Device().Raw()))
	if err != nil {
		memory.Close()
		return nil, fmt.Errorf("shader compilation failed: %w", err)
	}

	pipelines, err := NewPipelineCache(backend.Device(), shaders)
	if err != nil {
		memory.Close()
		return nil, fmt.Errorf("pipeline creation failed: %w", err)
	}

	// Warmup common blend pipelines
	pipelines.WarmupBlendPipelines()

	// Create target texture
	targetTex, err := memory.AllocTexture(TextureConfig{
		Width:  config.Width,
		Height: config.Height,
		Format: TextureFormatRGBA8,
		Label:  "render-target",
	})
	if err != nil {
		pipelines.Close()
		memory.Close()
		return nil, fmt.Errorf("target texture allocation failed: %w", err)
	}

	r := &GPUSceneRenderer{
		backend:          backend,
		width:            config.Width,
		height:           config.Height,
		pipelines:        pipelines,
		memory:           memory,
		tessellatorPool:  NewTessellatorPool(),
		targetTex:        targetTex,
		layerStack:       make([]*GPUTexture, 0, config.MaxLayers),
		stripBuffer:      NewStripBuffer(),
		currentTransform: scene.IdentityAffine(),
		clipStack:        make([]clipState, 0, 8),
	}

	return r, nil
}

// RenderScene renders a complete scene to the internal target texture.
// After rendering, use DownloadPixmap to retrieve the result.
//
// For cancellable rendering, use RenderSceneWithContext.
func (r *GPUSceneRenderer) RenderScene(s *scene.Scene) error {
	return r.RenderSceneWithContext(context.Background(), s)
}

// RenderSceneWithContext renders a complete scene to the internal target texture
// with cancellation support.
//
// The context can be used to cancel long-running renders. When canceled,
// the function returns ctx.Err() and the texture may contain partial results.
func (r *GPUSceneRenderer) RenderSceneWithContext(ctx context.Context, s *scene.Scene) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if r.closed {
		return ErrRendererClosed
	}

	if s == nil || s.IsEmpty() {
		return ErrEmptyScene
	}

	// Get flattened encoding
	enc := s.Encoding()
	if enc.IsEmpty() {
		return ErrEmptyScene
	}

	// Check for cancellation after encoding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Clear target texture
	r.clearTexture(r.targetTex)

	// Reset state
	r.currentTransform = scene.IdentityAffine()
	r.layerStack = r.layerStack[:0]
	r.clipStack = r.clipStack[:0]

	// Create decoder and process commands with context
	decoder := scene.NewDecoder(enc)

	return r.processCommandsWithContext(ctx, decoder)
}

// RenderToPixmap renders a scene directly to a pixmap.
// This is a convenience method that renders and downloads in one call.
//
// For cancellable rendering, use RenderToPixmapWithContext.
func (r *GPUSceneRenderer) RenderToPixmap(target *gg.Pixmap, s *scene.Scene) error {
	return r.RenderToPixmapWithContext(context.Background(), target, s)
}

// RenderToPixmapWithContext renders a scene directly to a pixmap with cancellation support.
// This is a convenience method that renders and downloads in one call.
//
// The context can be used to cancel long-running renders. When canceled,
// the function returns ctx.Err() and the target may contain partial results.
func (r *GPUSceneRenderer) RenderToPixmapWithContext(ctx context.Context, target *gg.Pixmap, s *scene.Scene) error {
	if target == nil {
		return ErrNilTarget
	}

	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check dimensions match
	if target.Width() != r.width || target.Height() != r.height {
		return fmt.Errorf("%w: expected %dx%d, got %dx%d",
			ErrTextureSizeMismatch, r.width, r.height, target.Width(), target.Height())
	}

	// Render scene with context
	if err := r.RenderSceneWithContext(ctx, s); err != nil {
		return err
	}

	// Check for cancellation before download
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Download result
	return r.downloadToPixmap(target)
}

// commandProcessor tracks state during command processing.
type commandProcessor struct {
	r           *GPUSceneRenderer
	currentPath *scene.Path
}

// processPathCommand handles path-building commands.
func (cp *commandProcessor) processPathCommand(tag scene.Tag, dec *scene.Decoder) {
	switch tag {
	case scene.TagBeginPath:
		cp.currentPath = scene.NewPath()
	case scene.TagMoveTo:
		if cp.currentPath != nil {
			x, y := dec.MoveTo()
			cp.currentPath.MoveTo(x, y)
		}
	case scene.TagLineTo:
		if cp.currentPath != nil {
			x, y := dec.LineTo()
			cp.currentPath.LineTo(x, y)
		}
	case scene.TagQuadTo:
		if cp.currentPath != nil {
			cx, cy, x, y := dec.QuadTo()
			cp.currentPath.QuadTo(cx, cy, x, y)
		}
	case scene.TagCubicTo:
		if cp.currentPath != nil {
			c1x, c1y, c2x, c2y, x, y := dec.CubicTo()
			cp.currentPath.CubicTo(c1x, c1y, c2x, c2y, x, y)
		}
	case scene.TagClosePath:
		if cp.currentPath != nil {
			cp.currentPath.Close()
		}
	}
}

// processDrawCommand handles fill and stroke commands.
func (cp *commandProcessor) processDrawCommand(tag scene.Tag, dec *scene.Decoder) {
	switch tag {
	case scene.TagFill:
		brush, style := dec.Fill()
		if cp.currentPath != nil && !cp.currentPath.IsEmpty() {
			cp.r.renderFill(cp.currentPath, brush, style)
		}
		cp.currentPath = nil
	case scene.TagStroke:
		brush, style := dec.Stroke()
		if cp.currentPath != nil && !cp.currentPath.IsEmpty() {
			cp.r.renderStroke(cp.currentPath, brush, style)
		}
		cp.currentPath = nil
	}
}

// processLayerCommand handles layer push/pop commands.
func (cp *commandProcessor) processLayerCommand(tag scene.Tag, dec *scene.Decoder) error {
	switch tag {
	case scene.TagPushLayer:
		blend, alpha := dec.PushLayer()
		return cp.r.pushLayer(blend, alpha)
	case scene.TagPopLayer:
		return cp.r.popLayer()
	}
	return nil
}

// processClipCommand handles clip begin/end commands.
func (cp *commandProcessor) processClipCommand(tag scene.Tag) {
	switch tag {
	case scene.TagBeginClip:
		if cp.currentPath != nil {
			cp.r.pushClip(cp.currentPath)
		}
	case scene.TagEndClip:
		cp.r.popClip()
	}
}

// processCommandsWithContext processes all commands from the decoder with cancellation support.
func (r *GPUSceneRenderer) processCommandsWithContext(ctx context.Context, dec *scene.Decoder) error {
	cp := &commandProcessor{r: r}

	// Check context less frequently for performance (every 16 commands)
	cmdCount := 0

	for dec.Next() {
		// Check for cancellation periodically
		cmdCount++
		if cmdCount%16 == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		tag := dec.Tag()

		switch {
		case tag == scene.TagTransform:
			r.currentTransform = dec.Transform()
		case tag.IsPathCommand():
			cp.processPathCommand(tag, dec)
		case tag.IsDrawCommand():
			cp.processDrawCommand(tag, dec)
		case tag.IsLayerCommand():
			if err := cp.processLayerCommand(tag, dec); err != nil {
				return err
			}
		case tag.IsClipCommand():
			cp.processClipCommand(tag)
		case tag == scene.TagImage:
			imageIdx, transform := dec.Image()
			r.renderImage(imageIdx, transform)
		}
	}

	// Check for cancellation before final composite
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Composite final result
	return r.compositeToTarget()
}

// renderFill fills a path with the given brush and style.
func (r *GPUSceneRenderer) renderFill(path *scene.Path, brush scene.Brush, style scene.FillStyle) {
	// Get tessellator from pool
	tess := r.tessellatorPool.Get()
	defer r.tessellatorPool.Put(tess)

	// Configure fill rule
	tess.SetFillRule(style)

	// Tessellate path to strips
	strips := tess.TessellatePath(path, r.currentTransform)
	if strips.IsEmpty() {
		return
	}

	// Merge adjacent strips for efficiency
	strips.MergeAdjacent()

	// Get current render target
	target := r.getCurrentTarget()

	// Rasterize strips to target
	r.rasterizeStrips(strips, brush, target)
}

// renderStroke strokes a path with the given brush and style.
func (r *GPUSceneRenderer) renderStroke(path *scene.Path, brush scene.Brush, style *scene.StrokeStyle) {
	// For stroke, we need to expand the path to a filled region
	// This is typically done by creating an offset path or using
	// stroke-specific tessellation

	// For now, use a simplified approach: tessellate the path outline
	// A proper implementation would expand the stroke to a fill region

	tess := r.tessellatorPool.Get()
	defer r.tessellatorPool.Put(tess)

	// Expand stroke to fill (simplified - proper implementation would
	// create offset curves based on stroke width)
	expandedPath := r.expandStroke(path, style)
	if expandedPath == nil || expandedPath.IsEmpty() {
		return
	}

	tess.SetFillRule(scene.FillNonZero)

	strips := tess.TessellatePath(expandedPath, r.currentTransform)
	if strips.IsEmpty() {
		return
	}

	strips.MergeAdjacent()

	target := r.getCurrentTarget()
	r.rasterizeStrips(strips, brush, target)
}

// expandStroke creates a filled path representing the stroke.
// This is a simplified implementation; a full implementation would
// handle line caps, joins, and proper offset curves.
func (r *GPUSceneRenderer) expandStroke(path *scene.Path, style *scene.StrokeStyle) *scene.Path {
	if style == nil {
		style = scene.DefaultStrokeStyle()
	}
	_ = style // TODO: use style when stroke expansion is implemented

	// For now, return the original path as a stub
	// A proper implementation would:
	// 1. Create parallel offset curves at +/- width/2
	// 2. Add line caps at endpoints
	// 3. Handle line joins at corners
	// 4. Return the outline as a closed path

	// TODO: Implement proper stroke expansion
	return path
}

// rasterizeStrips rasterizes coverage strips to a texture.
func (r *GPUSceneRenderer) rasterizeStrips(strips *StripBuffer, brush scene.Brush, target *GPUTexture) {
	if strips.IsEmpty() {
		return
	}

	// Pack strips for GPU
	headers, coverage := strips.PackForGPU()
	if len(headers) == 0 {
		return
	}

	// Extract color from brush
	color := [4]float32{
		float32(brush.Color.R),
		float32(brush.Color.G),
		float32(brush.Color.B),
		float32(brush.Color.A),
	}

	// Create strip params
	//nolint:gosec // texture dimensions and strip count are bounded by reasonable limits
	params := StripParams{
		Color:        color,
		TargetWidth:  int32(target.Width()),
		TargetHeight: int32(target.Height()),
		StripCount:   int32(len(headers)),
	}

	// TODO: When wgpu is ready:
	// 1. Upload headers to GPU buffer
	// 2. Upload coverage to GPU buffer
	// 3. Create command encoder
	// 4. Begin compute pass
	// 5. Set strip pipeline
	// 6. Set bind group with buffers and target texture
	// 7. Dispatch workgroups
	// 8. End pass and submit

	// For now, this is a stub that prepares data but doesn't execute GPU commands
	_ = headers
	_ = coverage
	_ = params
}

// pushLayer pushes a new compositing layer.
//
//nolint:unparam // blend and alpha prepared for when layer compositing is implemented
func (r *GPUSceneRenderer) pushLayer(blend scene.BlendMode, alpha float32) error {
	// Allocate layer texture
	layerTex, err := r.memory.AllocTexture(TextureConfig{
		Width:  r.width,
		Height: r.height,
		Format: TextureFormatRGBA8,
		Label:  fmt.Sprintf("layer-%d", len(r.layerStack)),
	})
	if err != nil {
		return fmt.Errorf("layer allocation failed: %w", err)
	}

	// Clear the layer
	r.clearTexture(layerTex)

	// Push to stack (store blend mode and alpha for pop)
	// For simplicity, we store them in a separate tracking structure
	r.layerStack = append(r.layerStack, layerTex)

	return nil
}

// popLayer pops the current layer and composites it.
func (r *GPUSceneRenderer) popLayer() error {
	if len(r.layerStack) == 0 {
		return ErrLayerStackUnderflow
	}

	// Get top layer
	layerIdx := len(r.layerStack) - 1
	layerTex := r.layerStack[layerIdx]
	r.layerStack = r.layerStack[:layerIdx]

	// Get destination (previous layer or target)
	dstTex := r.getCurrentTarget()

	// Blend layer onto destination
	// TODO: Get actual blend mode and alpha from tracking
	r.blendTextures(dstTex, layerTex, scene.BlendSourceOver, 1.0)

	// Return texture to pool
	if err := r.memory.FreeTexture(layerTex); err != nil {
		return fmt.Errorf("layer free failed: %w", err)
	}

	return nil
}

// getCurrentTarget returns the current render target.
// If there are layers on the stack, returns the top layer.
// Otherwise returns the main target texture.
func (r *GPUSceneRenderer) getCurrentTarget() *GPUTexture {
	if len(r.layerStack) > 0 {
		return r.layerStack[len(r.layerStack)-1]
	}
	return r.targetTex
}

// blendTextures blends src onto dst using the specified blend mode.
//
//nolint:unparam // dst prepared for when blending is implemented
func (r *GPUSceneRenderer) blendTextures(dst, src *GPUTexture, mode scene.BlendMode, alpha float32) {
	// Create blend params
	params := BlendParams{
		Mode:  BlendModeToShader(mode),
		Alpha: alpha,
	}

	// Get blend pipeline
	pipeline := r.pipelines.GetBlendPipeline(mode)

	// TODO: When wgpu is ready:
	// 1. Create command encoder
	// 2. Begin render pass with dst as target (LoadOp: Load)
	// 3. Set blend pipeline
	// 4. Create bind group with src texture and params
	// 5. Set bind group
	// 6. Draw full-screen triangle (3 vertices)
	// 7. End pass and submit

	_ = params
	_ = pipeline
}

// pushClip pushes a clipping region.
func (r *GPUSceneRenderer) pushClip(path *scene.Path) {
	// Create clip mask texture
	clipTex, err := r.memory.AllocTexture(TextureConfig{
		Width:  r.width,
		Height: r.height,
		Format: TextureFormatR8, // Single-channel mask
		Label:  fmt.Sprintf("clip-%d", len(r.clipStack)),
	})
	if err != nil {
		// Log error but continue - clipping will be a no-op
		return
	}

	// Render clip path to mask
	r.renderClipMask(path, clipTex)

	// Push to clip stack
	r.clipStack = append(r.clipStack, clipState{
		texture:   clipTex,
		transform: r.currentTransform,
	})
}

// popClip pops the current clipping region.
func (r *GPUSceneRenderer) popClip() {
	if len(r.clipStack) == 0 {
		return
	}

	clipIdx := len(r.clipStack) - 1
	clip := r.clipStack[clipIdx]
	r.clipStack = r.clipStack[:clipIdx]

	// Free clip texture
	_ = r.memory.FreeTexture(clip.texture)
}

// renderClipMask renders a path to a single-channel mask texture.
func (r *GPUSceneRenderer) renderClipMask(path *scene.Path, mask *GPUTexture) {
	// Tessellate path
	tess := r.tessellatorPool.Get()
	defer r.tessellatorPool.Put(tess)

	strips := tess.TessellatePath(path, r.currentTransform)
	if strips.IsEmpty() {
		return
	}

	// TODO: Rasterize to single-channel mask texture
	// This is similar to rasterizeStrips but outputs coverage to a mask
	_ = mask
}

// renderImage renders an image at the given transform.
func (r *GPUSceneRenderer) renderImage(imageIdx uint32, transform scene.Affine) {
	// Combine with current transform
	combinedTransform := r.currentTransform.Multiply(transform)

	// TODO: When image support is implemented:
	// 1. Get image texture from image registry
	// 2. Create bind group with image texture
	// 3. Render textured quad with combined transform
	_ = imageIdx
	_ = combinedTransform
}

// clearTexture clears a texture to transparent.
func (r *GPUSceneRenderer) clearTexture(tex *GPUTexture) {
	// TODO: When wgpu is ready:
	// 1. Create command encoder
	// 2. Begin render pass with LoadOp: Clear, ClearValue: transparent
	// 3. End pass immediately
	// 4. Submit
	_ = tex
}

// compositeToTarget performs final compositing to the target texture.
func (r *GPUSceneRenderer) compositeToTarget() error {
	// If there are remaining layers, pop them all
	for len(r.layerStack) > 0 {
		if err := r.popLayer(); err != nil {
			return err
		}
	}

	// Target texture now contains the final result
	return nil
}

// downloadToPixmap downloads the target texture to a pixmap.
func (r *GPUSceneRenderer) downloadToPixmap(target *gg.Pixmap) error {
	// TODO: When wgpu texture readback is implemented:
	// 1. Create staging buffer
	// 2. Copy texture to buffer
	// 3. Map buffer for reading
	// 4. Copy data to pixmap
	// 5. Unmap and destroy buffer

	// For now, return stub error
	return ErrTextureReadbackNotSupported
}

// Resize resizes the renderer to new dimensions.
// All layer textures are released and the target texture is reallocated.
func (r *GPUSceneRenderer) Resize(width, height int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRendererClosed
	}

	if width <= 0 || height <= 0 {
		return ErrInvalidDimensions
	}

	// Release all layers
	for _, tex := range r.layerStack {
		_ = r.memory.FreeTexture(tex)
	}
	r.layerStack = r.layerStack[:0]

	// Release all clips
	for _, clip := range r.clipStack {
		_ = r.memory.FreeTexture(clip.texture)
	}
	r.clipStack = r.clipStack[:0]

	// Release old target
	if err := r.memory.FreeTexture(r.targetTex); err != nil {
		return fmt.Errorf("target free failed: %w", err)
	}

	// Allocate new target
	targetTex, err := r.memory.AllocTexture(TextureConfig{
		Width:  width,
		Height: height,
		Format: TextureFormatRGBA8,
		Label:  "render-target",
	})
	if err != nil {
		return fmt.Errorf("target allocation failed: %w", err)
	}

	r.targetTex = targetTex
	r.width = width
	r.height = height

	return nil
}

// Width returns the renderer width.
func (r *GPUSceneRenderer) Width() int {
	return r.width
}

// Height returns the renderer height.
func (r *GPUSceneRenderer) Height() int {
	return r.height
}

// MemoryStats returns GPU memory usage statistics.
func (r *GPUSceneRenderer) MemoryStats() MemoryStats {
	return r.memory.Stats()
}

// LayerDepth returns the current layer stack depth.
func (r *GPUSceneRenderer) LayerDepth() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.layerStack)
}

// Close releases all renderer resources.
func (r *GPUSceneRenderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	// Release all layers
	for _, tex := range r.layerStack {
		_ = r.memory.FreeTexture(tex)
	}
	r.layerStack = nil

	// Release all clips
	for _, clip := range r.clipStack {
		_ = r.memory.FreeTexture(clip.texture)
	}
	r.clipStack = nil

	// Release target
	if r.targetTex != nil {
		_ = r.memory.FreeTexture(r.targetTex)
		r.targetTex = nil
	}

	// Close pipeline cache
	if r.pipelines != nil {
		r.pipelines.Close()
		r.pipelines = nil
	}

	// Close memory manager
	if r.memory != nil {
		r.memory.Close()
		r.memory = nil
	}

	r.closed = true
}
