//go:build !nogpu

package gpu

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/stroke"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"

	// Import Vulkan backend so it registers via init().
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

// SDFAccelerator provides GPU-accelerated rendering using wgpu/hal render
// pipelines. It implements the gg.GPUAccelerator interface.
//
// The accelerator uses a unified GPURenderSession to render all draw commands
// (SDF shapes + convex polygons + stencil-then-cover paths) in a single render
// pass. Shapes submitted via FillShape/StrokeShape and paths via FillPath are
// accumulated and rendered together on Flush().
//
// When PipelineMode is set to Compute (or Auto selects Compute based on scene
// complexity), the accelerator delegates to an internal VelloAccelerator that
// uses the 9-stage compute pipeline instead of render passes.
//
// This unified approach matches enterprise 2D engines (Skia Ganesh/Graphite,
// Flutter Impeller, Gio): one render pass with pipeline switching, shared
// MSAA + stencil textures, single submit + fence wait, single readback.
type SDFAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	// Unified render session managing shared textures and frame encoding.
	session *GPURenderSession

	// SDF render pipeline (vertex+fragment) -- owned by the accelerator,
	// shared with the session.
	sdfRenderPipeline *SDFRenderPipeline

	// Convex polygon renderer -- owned by the accelerator,
	// shared with the session.
	convexRenderer *ConvexRenderer

	// Stencil-then-cover renderer -- owned by the accelerator,
	// shared with the session.
	stencilRenderer *StencilRenderer

	// Pending SDF shapes for batch dispatch.
	pendingShapes []SDFRenderShape

	// Pending convex polygon commands for batch dispatch.
	pendingConvexCommands []ConvexDrawCommand

	// Pending stencil paths for unified dispatch.
	pendingStencilPaths []StencilPathCommand

	// Pending MSDF text batches for Tier 4 dispatch.
	pendingTextBatches []TextBatch

	// GPUTextEngine bridges text shaping with the MSDF atlas + quad pipeline.
	// Lazily created on first DrawText call.
	textEngine *GPUTextEngine

	// GlyphMaskEngine bridges text shaping with the CPU-rasterized R8 alpha
	// atlas + Tier 6 quad pipeline. Lazily created on first glyph mask call.
	glyphMaskEngine *GlyphMaskEngine

	// Pending glyph mask batches for Tier 6 dispatch.
	pendingGlyphMaskBatches []GlyphMaskBatch

	pendingTarget *gg.GPURenderTarget // nil if no pending commands

	cpuFallback    gg.SDFAccelerator
	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)

	// Pipeline mode routing: VelloAccelerator for compute, scene stats for Auto.
	velloAccel   *VelloAccelerator // Internal compute accelerator (Tier 5)
	pipelineMode gg.PipelineMode   // Current pipeline mode (from Context)
	sceneStats   gg.SceneStats     // Accumulated per-frame stats for Auto selection

	// Scissor state: tracks the current clip rect and a timeline of scissor
	// changes within a frame. Instead of flushing on every scissor change
	// (which creates multiple render passes), we record scissor boundaries
	// and replay them within a single render pass on Flush().
	clipRect        *[4]uint32       // Current scissor rect (nil = full framebuffer)
	clipRRect       *ClipParams      // Current RRect clip (nil = no RRect clip)
	scissorSegments []scissorSegment // Timeline of scissor changes within a frame
}

var _ gg.GPUAccelerator = (*SDFAccelerator)(nil)
var _ gg.SurfaceTargetAware = (*SDFAccelerator)(nil)
var _ gg.GPUTextAccelerator = (*SDFAccelerator)(nil)
var _ gg.GPUGlyphMaskAccelerator = (*SDFAccelerator)(nil)
var _ gg.PipelineModeAware = (*SDFAccelerator)(nil)
var _ gg.ComputePipelineAware = (*SDFAccelerator)(nil)
var _ gg.ForceSDFAware = (*SDFAccelerator)(nil)
var _ gg.ClipAware = (*SDFAccelerator)(nil)
var _ gg.RRectClipAware = (*SDFAccelerator)(nil)

// Name returns the accelerator identifier.
func (a *SDFAccelerator) Name() string { return "sdf-gpu" }

// SetForceSDF propagates the force-SDF flag to the CPU fallback accelerator.
// When enabled, the CPU fallback skips the minimum size check for SDF shapes.
func (a *SDFAccelerator) SetForceSDF(force bool) {
	a.cpuFallback.SetForceSDF(force)
}

// SetClipRect sets the scissor rect for subsequent GPU draw commands.
// The scissor rect is recorded in the scissor timeline so that all
// pending commands can be rendered in a single render pass on Flush(),
// with per-batch scissor state changes. This avoids creating multiple
// render passes per frame when clipping is used.
func (a *SDFAccelerator) SetClipRect(x, y, w, h uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rect := [4]uint32{x, y, w, h}
	a.clipRect = &rect
	a.recordScissorSegment(&rect)
}

// ClearClipRect removes the scissor rect, restoring full-framebuffer
// rendering for subsequent draw commands. The change is recorded in the
// scissor timeline for deferred application during Flush().
func (a *SDFAccelerator) ClearClipRect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clipRect = nil
	a.recordScissorSegment(nil)
}

// SetClipRRect sets the rounded rectangle clip for subsequent GPU draw
// commands. The RRect is specified in device pixels. The clip parameters are
// recorded in the scissor timeline so that all pending commands can be
// rendered in a single render pass on Flush(), with per-group clip state.
func (a *SDFAccelerator) SetClipRRect(x, y, w, h, radius float32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clipRRect = &ClipParams{
		RectX1:  x,
		RectY1:  y,
		RectX2:  x + w,
		RectY2:  y + h,
		Radius:  radius,
		Enabled: 1.0,
	}
	a.recordScissorSegment(a.clipRect)
}

// ClearClipRRect removes the rounded rectangle clip, restoring full rendering
// for subsequent draw commands. The change is recorded in the scissor timeline.
func (a *SDFAccelerator) ClearClipRRect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clipRRect = nil
	a.recordScissorSegment(a.clipRect)
}

// recordScissorSegment records a scissor state change in the timeline.
// Each segment marks the current pending counts at the time of the change.
// During Flush(), the timeline is used to slice pending arrays into groups,
// each rendered with the correct scissor rect within a single render pass.
// Must be called with a.mu held.
func (a *SDFAccelerator) recordScissorSegment(rect *[4]uint32) {
	// Copy the rect so it's immutable (caller may reuse the value).
	var copied *[4]uint32
	if rect != nil {
		c := *rect
		copied = &c
	}
	// Copy the RRect clip so it's immutable.
	var copiedRRect *ClipParams
	if a.clipRRect != nil {
		c := *a.clipRRect
		copiedRRect = &c
	}
	a.scissorSegments = append(a.scissorSegments, scissorSegment{
		rect:         copied,
		clipRRect:    copiedRRect,
		sdfCount:     len(a.pendingShapes),
		convexCount:  len(a.pendingConvexCommands),
		stencilCount: len(a.pendingStencilPaths),
		textCount:    len(a.pendingTextBatches),
		glyphCount:   len(a.pendingGlyphMaskBatches),
	})
}

// scissorSegment records a scissor state change along with the cumulative
// pending counts at the time of the change. Used to slice pending arrays
// into per-scissor groups during Flush().
type scissorSegment struct {
	rect         *[4]uint32  // nil = full framebuffer
	clipRRect    *ClipParams // nil = no RRect clip
	sdfCount     int         // len(pendingShapes) at time of change
	convexCount  int         // len(pendingConvexCommands) at time of change
	stencilCount int         // len(pendingStencilPaths) at time of change
	textCount    int         // len(pendingTextBatches) at time of change
	glyphCount   int         // len(pendingGlyphMaskBatches) at time of change
}

// CanAccelerate reports whether this accelerator supports the given operation.
func (a *SDFAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelCircleSDF|gg.AccelRRectSDF|gg.AccelFill|gg.AccelStroke|gg.AccelText) != 0
}

// SetPipelineMode sets the pipeline mode for subsequent operations.
// When set to Compute, FillPath/StrokePath/FillShape/StrokeShape are delegated
// to the internal VelloAccelerator. When set to Auto, the accelerator uses
// scene statistics to choose the best pipeline on Flush.
func (a *SDFAccelerator) SetPipelineMode(mode gg.PipelineMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pipelineMode = mode
}

// CanCompute reports whether the compute pipeline is available and ready.
// Returns true when the internal VelloAccelerator is initialized and its
// compute dispatcher is ready.
func (a *SDFAccelerator) CanCompute() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.velloAccel != nil && a.velloAccel.CanCompute()
}

// SceneStats returns the accumulated scene statistics for the current frame.
func (a *SDFAccelerator) SceneStats() gg.SceneStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sceneStats
}

// Init registers the accelerator. GPU device initialization is deferred
// until the first render to avoid creating a standalone Vulkan device that
// may interfere with an external DX12/Metal device provided later via
// SetDeviceProvider. This lazy approach prevents Intel iGPU driver issues
// where destroying a Vulkan device kills a coexisting DX12 device.
func (a *SDFAccelerator) Init() error {
	return nil
}

// Close releases all GPU resources held by the accelerator.
func (a *SDFAccelerator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingShapes = nil
	a.pendingConvexCommands = nil
	a.pendingStencilPaths = nil
	a.pendingTextBatches = nil
	a.pendingGlyphMaskBatches = nil
	a.textEngine = nil
	if a.glyphMaskEngine != nil && a.device != nil {
		a.glyphMaskEngine.Destroy(a.device)
		a.glyphMaskEngine = nil
	}
	a.pendingTarget = nil
	a.clipRect = nil
	a.clipRRect = nil
	a.scissorSegments = nil
	a.sceneStats = gg.SceneStats{}
	if a.velloAccel != nil {
		a.velloAccel.Close()
		a.velloAccel = nil
	}
	if a.session != nil {
		// Destroy the text pipeline before the session. The session does not
		// own pipelines (Destroy says "owned by the caller"), but the text
		// pipeline is lazily created inside the session and the accelerator
		// has no direct reference to it. Without this, ShaderModule,
		// PipelineLayout, Pipelines, DescriptorSetLayout, and Sampler leak.
		if tp := a.session.TextPipelineRef(); tp != nil {
			tp.Destroy()
		}
		a.session.Destroy()
		a.session = nil
	}
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	if a.convexRenderer != nil {
		a.convexRenderer.Destroy()
		a.convexRenderer = nil
	}
	if a.stencilRenderer != nil {
		a.stencilRenderer.Destroy()
		a.stencilRenderer = nil
	}
	if !a.externalDevice {
		if a.device != nil {
			a.device.Destroy()
			a.device = nil
		}
		if a.instance != nil {
			a.instance.Destroy()
			a.instance = nil
		}
	} else {
		// Don't destroy shared resources -- we don't own them.
		a.device = nil
		a.instance = nil
	}
	a.queue = nil
	a.gpuReady = false
	a.externalDevice = false
}

// SetLogger sets the logger for the GPU accelerator and its internal packages.
// Called by gg.SetLogger to propagate logging configuration.
func (a *SDFAccelerator) SetLogger(l *slog.Logger) {
	setLogger(l)
}

// SetDeviceProvider switches the accelerator to use a shared GPU device
// from an external provider (e.g., gogpu). The provider's Device() must
// return a *wgpu.Device (as gpucontext.Device) so we can access HalDevice()
// and HalQueue() for direct HAL-level GPU operations.
func (a *SDFAccelerator) SetDeviceProvider(provider gpucontext.DeviceProvider) error {
	dev := provider.Device()
	if dev == nil {
		return fmt.Errorf("gpu-sdf: provider Device is nil")
	}

	type halAccessor interface {
		HalDevice() hal.Device
		HalQueue() hal.Queue
	}
	ha, ok := dev.(halAccessor)
	if !ok {
		return fmt.Errorf("gpu-sdf: provider Device does not expose HAL types (got %T)", dev)
	}
	device := ha.HalDevice()
	if device == nil {
		return fmt.Errorf("gpu-sdf: provider HalDevice is nil")
	}
	queue := ha.HalQueue()
	if queue == nil {
		return fmt.Errorf("gpu-sdf: provider HalQueue is nil")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Destroy own resources if we created them.
	if a.session != nil {
		a.session.Destroy()
		a.session = nil
	}
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	if a.convexRenderer != nil {
		a.convexRenderer.Destroy()
		a.convexRenderer = nil
	}
	if a.stencilRenderer != nil {
		a.stencilRenderer.Destroy()
		a.stencilRenderer = nil
	}
	if !a.externalDevice && a.device != nil {
		a.device.Destroy()
	}
	if a.instance != nil {
		a.instance.Destroy()
		a.instance = nil
	}

	// Use provided resources.
	a.device = device
	a.queue = queue
	a.externalDevice = true

	// Create pipelines and session with shared device.
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	a.convexRenderer = NewConvexRenderer(a.device, a.queue)
	a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
	a.session = NewGPURenderSession(a.device, a.queue)
	a.session.SetSDFPipeline(a.sdfRenderPipeline)
	a.session.SetConvexRenderer(a.convexRenderer)
	a.session.SetStencilRenderer(a.stencilRenderer)

	a.gpuReady = true

	// Initialize internal VelloAccelerator with the shared device for compute routing.
	a.initVelloAccelerator(device, queue)

	slogger().Info("switched to shared GPU device",
		"adapter", fmt.Sprintf("%T", device),
	)
	return nil
}

// SetSurfaceTarget configures the accelerator for direct surface rendering.
// When view is non-nil, Flush() renders directly to the surface texture view
// instead of reading back to GPURenderTarget.Data. This eliminates the
// GPU->CPU readback for windowed rendering.
//
// When the view pointer changes (new swapchain frame), per-frame state is
// automatically reset so the first render pass clears the surface while
// subsequent mid-frame flushes preserve content (LoadOpLoad).
//
// Call with nil to return to offscreen mode. The caller retains ownership
// of the view.
func (a *SDFAccelerator) SetSurfaceTarget(view any, width, height uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session == nil {
		return
	}
	if view == nil {
		a.session.SetSurfaceTarget(nil, 0, 0)
		return
	}
	halView, ok := view.(hal.TextureView)
	if !ok {
		slogger().Warn("SetSurfaceTarget: view is not hal.TextureView")
		return
	}
	a.session.SetSurfaceTarget(halView, width, height)
	slogger().Debug("SetSurfaceTarget configured", "width", width, "height", height)
}

// BeginFrame resets per-frame state so that the first render pass of each
// frame clears the surface (LoadOpClear). Without this call, subsequent
// frames use LoadOpLoad and composite on top of previous content.
func (a *SDFAccelerator) BeginFrame() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clipRect = nil
	// NOTE: Do NOT clear scissorSegments here. They are accumulated during the
	// draw phase (before BeginFrame+Flush) and consumed by flushLocked(), which
	// clears them after use. Clearing here would destroy clip data recorded
	// between draws and the flush call in RenderDirect().
	if a.session != nil {
		a.session.BeginFrame()
	}
}

// DrawText queues text for GPU MSDF rendering. The face parameter must be a
// text.Face; it is typed as any in the GPUTextAccelerator interface to avoid
// a circular dependency between gg and the text package.
//
// The text is shaped into glyphs, each glyph's MSDF is generated and packed
// into the atlas, and TextQuads are produced for the unified render pass.
// Actual rendering is deferred to Flush().
//
// The matrix parameter is the context's current transformation matrix (CTM).
// It is composed with the pixel-to-NDC ortho projection in the uniform
// buffer so that Scale, Rotate, and Skew transforms affect text rendering.
// Quad positions are computed in user space; the CTM is applied by the
// vertex shader.
//
// Returns ErrFallbackToCPU if the GPU is not ready or the face type is wrong.
//
//nolint:dupl // Intentional: mirrors DrawGlyphMaskText structure; each method owns its engine/batch type.
func (a *SDFAccelerator) DrawText(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	textFace, ok := face.(text.Face)
	if !ok || textFace == nil {
		return gg.ErrFallbackToCPU
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Track text scene stats.
	a.sceneStats.TextCount++

	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// Lazily create the text engine.
	if a.textEngine == nil {
		a.textEngine = NewGPUTextEngine()
	}

	// If target changed, flush previous batch first.
	if a.pendingTarget != nil && !sameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	batch, err := a.textEngine.LayoutText(textFace, s, x, y, color, target.Width, target.Height, matrix, deviceScale)
	if err != nil {
		return gg.ErrFallbackToCPU
	}
	if len(batch.Quads) == 0 {
		return nil // Empty text (e.g., all spaces), nothing to render.
	}

	a.pendingTextBatches = append(a.pendingTextBatches, batch)
	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

// FillPath queues a filled path for GPU rendering. The path is analyzed to
// determine the optimal rendering tier:
//
//   - If the path is a single closed convex polygon with only line segments,
//     it is queued as a ConvexDrawCommand (Tier 2a: single draw call, no stencil).
//   - Otherwise, it is tessellated into fan triangles and queued as a
//     StencilPathCommand (Tier 2b: stencil-then-cover).
//
// When PipelineMode is Compute and the compute pipeline is available, the path
// is delegated to the internal VelloAccelerator instead.
//
// Rendering is deferred until Flush() so all commands can be combined in a
// single render pass. Returns ErrFallbackToCPU if the GPU is not ready.
func (a *SDFAccelerator) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// Track scene stats for Auto pipeline selection.
	a.sceneStats.PathCount++
	a.sceneStats.ShapeCount++

	// If in Compute mode and compute is available, delegate to VelloAccelerator.
	if a.pipelineMode == gg.PipelineModeCompute && a.velloAccel != nil && a.velloAccel.CanCompute() {
		return a.velloAccel.FillPath(target, path, paint)
	}

	// If target changed, flush previous batch first.
	if a.pendingTarget != nil && !sameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	color := getColorFromPaint(paint)
	premulR := float32(color.R * color.A)
	premulG := float32(color.G * color.A)
	premulB := float32(color.B * color.A)
	premulA := float32(color.A)

	// Try convex fast-path: extract points from path elements and check convexity.
	if points, ok := extractConvexPolygon(path); ok {
		cmd := ConvexDrawCommand{
			Points: points,
			Color:  [4]float32{premulR, premulG, premulB, premulA},
		}
		a.pendingConvexCommands = append(a.pendingConvexCommands, cmd)

		targetCopy := target
		a.pendingTarget = &targetCopy
		return nil
	}

	// Fall back to stencil-then-cover for non-convex or complex paths.
	tess := NewFanTessellator()
	tess.TessellatePath(path.Elements())
	fanVerts := tess.Vertices()
	if len(fanVerts) == 0 {
		return nil // empty path, nothing to render
	}

	cmd := StencilPathCommand{
		Vertices:  make([]float32, len(fanVerts)),
		CoverQuad: tess.CoverQuad(),
		Color:     [4]float32{premulR, premulG, premulB, premulA},
		FillRule:  paint.FillRule,
	}
	copy(cmd.Vertices, fanVerts)
	a.pendingStencilPaths = append(a.pendingStencilPaths, cmd)

	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

// extractConvexPolygon checks if a path is a single closed contour made entirely
// of line segments that form a convex polygon. If so, it returns the polygon
// points. If the path contains curves, multiple subpaths, or is not convex,
// it returns nil, false.
//
// This enables Tier 2a (convex fast-path) for paths like triangles, pentagons,
// and other convex shapes that don't need stencil-then-cover.
func extractConvexPolygon(path *gg.Path) ([]gg.Point, bool) {
	elements := path.Elements()
	if len(elements) < 3 {
		return nil, false
	}

	var points []gg.Point
	moveCount := 0
	closed := false

	for _, elem := range elements {
		switch e := elem.(type) {
		case gg.MoveTo:
			moveCount++
			if moveCount > 1 {
				// Multiple subpaths: not a single polygon.
				return nil, false
			}
			points = append(points, e.Point)
		case gg.LineTo:
			points = append(points, e.Point)
		case gg.QuadTo, gg.CubicTo:
			// Paths with curves need flattening, which changes point positions.
			// Use stencil-then-cover for these (fan tessellator handles curves).
			return nil, false
		case gg.Close:
			closed = true
		}
	}

	// Must be a single closed subpath with no curves.
	if !closed || moveCount != 1 {
		return nil, false
	}

	// Need at least 3 points for a polygon.
	if len(points) < 3 {
		return nil, false
	}

	// Check convexity.
	if !IsConvex(points) {
		return nil, false
	}

	return points, true
}

// StrokePath renders a stroked path on the GPU by expanding the stroke into a
// filled outline and routing it through the fill pipeline (convex fast-path or
// stencil-then-cover). Dashed strokes fall back to CPU rendering.
//
// When PipelineMode is Compute and the compute pipeline is available, the
// stroke is delegated to the internal VelloAccelerator.
func (a *SDFAccelerator) StrokePath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	if paint.IsDashed() {
		return gg.ErrFallbackToCPU
	}

	// Track scene stats for Auto pipeline selection.
	a.mu.Lock()
	a.sceneStats.PathCount++
	a.sceneStats.ShapeCount++
	computeMode := a.pipelineMode == gg.PipelineModeCompute && a.velloAccel != nil && a.velloAccel.CanCompute()
	a.mu.Unlock()

	// If in Compute mode and compute is available, delegate to VelloAccelerator.
	if computeMode {
		return a.velloAccel.StrokePath(target, path, paint)
	}

	ggElems := path.Elements()
	if len(ggElems) == 0 {
		return nil
	}

	// Convert gg path elements to stroke package elements.
	strokeElems := convertPathToStrokeElements(ggElems)

	// Expand stroke to filled outline.
	style := stroke.Stroke{
		Width:      paint.EffectiveLineWidth(),
		Cap:        stroke.LineCap(paint.EffectiveLineCap()),
		Join:       stroke.LineJoin(paint.EffectiveLineJoin()),
		MiterLimit: paint.EffectiveMiterLimit(),
	}
	expander := stroke.NewStrokeExpander(style)
	expanded := expander.Expand(strokeElems)
	if len(expanded) == 0 {
		return nil
	}

	// Build a gg.Path from the expanded outline.
	fillPath := gg.NewPath()
	for _, e := range expanded {
		switch el := e.(type) {
		case stroke.MoveTo:
			fillPath.MoveTo(el.Point.X, el.Point.Y)
		case stroke.LineTo:
			fillPath.LineTo(el.Point.X, el.Point.Y)
		case stroke.QuadTo:
			fillPath.QuadraticTo(el.Control.X, el.Control.Y, el.Point.X, el.Point.Y)
		case stroke.CubicTo:
			fillPath.CubicTo(el.Control1.X, el.Control1.Y, el.Control2.X, el.Control2.Y, el.Point.X, el.Point.Y)
		case stroke.Close:
			fillPath.Close()
		}
	}

	// Route through the fill pipeline (convex fast-path or stencil-then-cover).
	return a.FillPath(target, fillPath, paint)
}

// FillShape accumulates a filled shape for batch dispatch.
// The actual GPU work happens on Flush().
//
// When PipelineMode is Compute and the compute pipeline is available, the
// shape is delegated to the internal VelloAccelerator.
func (a *SDFAccelerator) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Track scene stats.
	a.sceneStats.ShapeCount++

	if !a.gpuReady {
		return a.cpuFallback.FillShape(target, shape, paint)
	}

	// If in Compute mode, delegate to VelloAccelerator.
	if a.pipelineMode == gg.PipelineModeCompute && a.velloAccel != nil && a.velloAccel.CanCompute() {
		return a.velloAccel.FillShape(target, shape, paint)
	}

	return a.queueShape(target, shape, paint, false)
}

// StrokeShape accumulates a stroked shape for batch dispatch.
// The actual GPU work happens on Flush().
//
// When PipelineMode is Compute and the compute pipeline is available, the
// shape is delegated to the internal VelloAccelerator.
func (a *SDFAccelerator) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Track scene stats.
	a.sceneStats.ShapeCount++

	if !a.gpuReady {
		return a.cpuFallback.StrokeShape(target, shape, paint)
	}

	// If in Compute mode, delegate to VelloAccelerator.
	if a.pipelineMode == gg.PipelineModeCompute && a.velloAccel != nil && a.velloAccel.CanCompute() {
		return a.velloAccel.StrokeShape(target, shape, paint)
	}

	return a.queueShape(target, shape, paint, true)
}

// Flush dispatches all pending commands (SDF shapes + convex polygons +
// stencil paths) via the unified render session. All commands are rendered
// in a single render pass. Returns nil if there are no pending commands.
//
// When PipelineMode is Compute (or Auto selects Compute), pending operations
// that were accumulated in the internal VelloAccelerator are flushed through
// the 9-stage compute pipeline instead.
func (a *SDFAccelerator) Flush(target gg.GPURenderTarget) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Determine effective mode for this flush.
	effectiveMode := a.effectivePipelineMode()

	slogger().Debug("Flush",
		"sdf", len(a.pendingShapes),
		"convex", len(a.pendingConvexCommands),
		"stencil", len(a.pendingStencilPaths),
		"text", len(a.pendingTextBatches),
		"glyphMask", len(a.pendingGlyphMaskBatches),
		"mode", effectiveMode,
	)

	// Reset scene stats for the next frame.
	a.sceneStats = gg.SceneStats{}

	// If VelloAccelerator has pending paths, flush them.
	// This happens when mode is Compute or Auto selected Compute.
	if a.velloAccel != nil && a.velloAccel.PendingCount() > 0 {
		if effectiveMode == gg.PipelineModeCompute {
			// Flush VelloAccelerator first (compute pipeline).
			if err := a.velloAccel.Flush(target); err != nil {
				slogger().Debug("vello compute flush failed, render-pass fallback", "err", err)
				// Compute failed — don't also try render pass (data was consumed by Vello).
			}
		}
	}

	return a.flushLocked(target)
}

// PendingCount returns the number of commands waiting for dispatch (for testing).
func (a *SDFAccelerator) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pendingShapes) + len(a.pendingConvexCommands) + len(a.pendingStencilPaths) + len(a.pendingTextBatches) + len(a.pendingGlyphMaskBatches)
}

// buildScissorGroups slices the pending arrays into ScissorGroup entries
// based on the recorded scissor timeline. Each group contains the subset
// of shapes/commands/batches that were submitted under the same scissor
// state, and the scissor rect to apply when rendering that group.
//
// If no scissor changes were recorded (common case: no clipping), returns
// a single group covering all pending items with nil scissor (full framebuffer).
func (a *SDFAccelerator) buildScissorGroups() []ScissorGroup {
	// No scissor changes: single group with all items and no scissor.
	if len(a.scissorSegments) == 0 {
		return []ScissorGroup{{
			Rect:             nil,
			SDFShapes:        a.pendingShapes,
			ConvexCommands:   a.pendingConvexCommands,
			StencilPaths:     a.pendingStencilPaths,
			TextBatches:      a.pendingTextBatches,
			GlyphMaskBatches: a.pendingGlyphMaskBatches,
		}}
	}

	var groups []ScissorGroup

	// Items before the first scissor change use nil scissor (full framebuffer).
	firstSeg := a.scissorSegments[0]
	if firstSeg.sdfCount > 0 || firstSeg.convexCount > 0 || firstSeg.stencilCount > 0 ||
		firstSeg.textCount > 0 || firstSeg.glyphCount > 0 {
		groups = append(groups, ScissorGroup{
			Rect:             nil,
			SDFShapes:        a.pendingShapes[:firstSeg.sdfCount],
			ConvexCommands:   a.pendingConvexCommands[:firstSeg.convexCount],
			StencilPaths:     a.pendingStencilPaths[:firstSeg.stencilCount],
			TextBatches:      a.pendingTextBatches[:firstSeg.textCount],
			GlyphMaskBatches: a.pendingGlyphMaskBatches[:firstSeg.glyphCount],
		})
	}

	// Each segment defines a scissor state and the start of its range.
	for i, seg := range a.scissorSegments {
		// End index is the start of the next segment, or len(pending).
		var endSDF, endConvex, endStencil, endText, endGlyph int
		if i+1 < len(a.scissorSegments) {
			next := a.scissorSegments[i+1]
			endSDF = next.sdfCount
			endConvex = next.convexCount
			endStencil = next.stencilCount
			endText = next.textCount
			endGlyph = next.glyphCount
		} else {
			endSDF = len(a.pendingShapes)
			endConvex = len(a.pendingConvexCommands)
			endStencil = len(a.pendingStencilPaths)
			endText = len(a.pendingTextBatches)
			endGlyph = len(a.pendingGlyphMaskBatches)
		}

		// Skip empty groups.
		if seg.sdfCount == endSDF && seg.convexCount == endConvex &&
			seg.stencilCount == endStencil && seg.textCount == endText &&
			seg.glyphCount == endGlyph {
			continue
		}

		groups = append(groups, ScissorGroup{
			Rect:             seg.rect,
			ClipRRect:        seg.clipRRect,
			SDFShapes:        a.pendingShapes[seg.sdfCount:endSDF],
			ConvexCommands:   a.pendingConvexCommands[seg.convexCount:endConvex],
			StencilPaths:     a.pendingStencilPaths[seg.stencilCount:endStencil],
			TextBatches:      a.pendingTextBatches[seg.textCount:endText],
			GlyphMaskBatches: a.pendingGlyphMaskBatches[seg.glyphCount:endGlyph],
		})
	}

	return groups
}

func (a *SDFAccelerator) flushLocked(target gg.GPURenderTarget) error { //nolint:cyclop,gocognit,gocyclo,funlen // sequential resource setup + group dispatch
	if len(a.pendingShapes) == 0 && len(a.pendingConvexCommands) == 0 &&
		len(a.pendingStencilPaths) == 0 && len(a.pendingTextBatches) == 0 &&
		len(a.pendingGlyphMaskBatches) == 0 {
		return nil
	}

	// Lazy GPU initialization: only create a standalone device if no shared
	// device was provided via SetDeviceProvider. This avoids creating a
	// Vulkan device at import time that can interfere with an external DX12
	// device on the same physical GPU (Intel iGPU driver issue).
	if a.device == nil {
		if err := a.initGPU(); err != nil {
			slogger().Warn("GPU init failed, using CPU fallback", "err", err)
			return gg.ErrFallbackToCPU
		}
	}

	// Ensure session exists with all renderers.
	if a.session == nil {
		a.session = NewGPURenderSession(a.device, a.queue)
		if a.sdfRenderPipeline == nil {
			a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
		}
		if a.convexRenderer == nil {
			a.convexRenderer = NewConvexRenderer(a.device, a.queue)
		}
		if a.stencilRenderer == nil {
			a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
		}
		a.session.SetSDFPipeline(a.sdfRenderPipeline)
		a.session.SetConvexRenderer(a.convexRenderer)
		a.session.SetStencilRenderer(a.stencilRenderer)
	}

	// Build scissor groups from the timeline BEFORE taking ownership of data.
	// The groups reference slices of the pending arrays (shallow copy).
	groups := a.buildScissorGroups()

	// Deep-copy each group's slices so we own the data, then clear pending.
	ownedGroups := make([]ScissorGroup, len(groups))
	for i := range groups {
		g := &groups[i]
		ownedGroups[i] = ScissorGroup{Rect: g.Rect, ClipRRect: g.ClipRRect}
		if len(g.SDFShapes) > 0 {
			ownedGroups[i].SDFShapes = make([]SDFRenderShape, len(g.SDFShapes))
			copy(ownedGroups[i].SDFShapes, g.SDFShapes)
		}
		if len(g.ConvexCommands) > 0 {
			ownedGroups[i].ConvexCommands = make([]ConvexDrawCommand, len(g.ConvexCommands))
			copy(ownedGroups[i].ConvexCommands, g.ConvexCommands)
		}
		if len(g.StencilPaths) > 0 {
			ownedGroups[i].StencilPaths = make([]StencilPathCommand, len(g.StencilPaths))
			copy(ownedGroups[i].StencilPaths, g.StencilPaths)
		}
		if len(g.TextBatches) > 0 {
			ownedGroups[i].TextBatches = make([]TextBatch, len(g.TextBatches))
			copy(ownedGroups[i].TextBatches, g.TextBatches)
		}
		if len(g.GlyphMaskBatches) > 0 {
			ownedGroups[i].GlyphMaskBatches = make([]GlyphMaskBatch, len(g.GlyphMaskBatches))
			copy(ownedGroups[i].GlyphMaskBatches, g.GlyphMaskBatches)
		}
	}

	// Clear pending state.
	a.pendingShapes = a.pendingShapes[:0]
	a.pendingConvexCommands = a.pendingConvexCommands[:0]
	a.pendingStencilPaths = a.pendingStencilPaths[:0]
	a.pendingTextBatches = a.pendingTextBatches[:0]
	a.pendingGlyphMaskBatches = a.pendingGlyphMaskBatches[:0]
	a.scissorSegments = a.scissorSegments[:0]
	a.pendingTarget = nil

	// Collect all text and glyph mask batches across groups for atlas sync.
	var allTextBatches []TextBatch
	var allGlyphMaskBatches []GlyphMaskBatch
	for i := range ownedGroups {
		allTextBatches = append(allTextBatches, ownedGroups[i].TextBatches...)
		allGlyphMaskBatches = append(allGlyphMaskBatches, ownedGroups[i].GlyphMaskBatches...)
	}

	// Upload dirty MSDF atlases to the GPU before rendering text.
	if len(allTextBatches) > 0 && a.textEngine != nil {
		if err := a.syncTextAtlases(); err != nil {
			slogger().Warn("atlas sync failed", "err", err)
			// Clear text batches from all groups on atlas failure.
			for i := range ownedGroups {
				ownedGroups[i].TextBatches = nil
			}
		}
	}

	// Upload dirty glyph mask atlas pages to GPU before rendering.
	if len(allGlyphMaskBatches) > 0 && a.glyphMaskEngine != nil {
		if err := a.syncGlyphMaskAtlases(allGlyphMaskBatches); err != nil {
			slogger().Warn("glyph mask atlas sync failed", "err", err)
			for i := range ownedGroups {
				ownedGroups[i].GlyphMaskBatches = nil
			}
		}
	}

	err := a.session.RenderFrameGrouped(target, ownedGroups)
	if err != nil {
		total := 0
		for i := range ownedGroups {
			total += len(ownedGroups[i].SDFShapes) + len(ownedGroups[i].ConvexCommands) + len(ownedGroups[i].StencilPaths) +
				len(ownedGroups[i].TextBatches) + len(ownedGroups[i].GlyphMaskBatches)
		}
		slogger().Warn("render session error",
			"groups", len(ownedGroups), "totalItems", total, "err", err)
	}
	return err
}

// syncTextAtlases uploads any dirty MSDF atlas pages to the GPU as textures.
// The session's SetTextAtlas method is called with the newly created (or
// recreated) texture and view. Currently supports a single atlas (index 0).
func (a *SDFAccelerator) syncTextAtlases() error {
	dirtyIndices := a.textEngine.DirtyAtlases()
	if len(dirtyIndices) == 0 {
		return nil
	}

	for _, idx := range dirtyIndices {
		rgbaData, size, _ := a.textEngine.AtlasRGBAData(idx)
		if rgbaData == nil || size == 0 {
			continue
		}

		atlasSize := uint32(size) //nolint:gosec // atlas size always fits uint32

		// Create the GPU texture for this atlas page.
		tex, err := a.device.CreateTexture(&hal.TextureDescriptor{
			Label:         fmt.Sprintf("msdf_atlas_%d", idx),
			Size:          hal.Extent3D{Width: atlasSize, Height: atlasSize, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
		})
		if err != nil {
			return fmt.Errorf("create atlas texture %d: %w", idx, err)
		}

		view, err := a.device.CreateTextureView(tex, &hal.TextureViewDescriptor{
			Label:         fmt.Sprintf("msdf_atlas_%d_view", idx),
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Dimension:     gputypes.TextureViewDimension2D,
			Aspect:        gputypes.TextureAspectAll,
			MipLevelCount: 1,
		})
		if err != nil {
			a.device.DestroyTexture(tex)
			return fmt.Errorf("create atlas texture view %d: %w", idx, err)
		}

		// Upload RGBA data to the GPU texture.
		if err := a.queue.WriteTexture(
			&hal.ImageCopyTexture{Texture: tex, MipLevel: 0},
			rgbaData,
			&hal.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  atlasSize * 4,
				RowsPerImage: atlasSize,
			},
			&hal.Extent3D{Width: atlasSize, Height: atlasSize, DepthOrArrayLayers: 1},
		); err != nil {
			a.device.DestroyTexture(tex)
			return fmt.Errorf("upload atlas texture %d: %w", idx, err)
		}

		// Pass texture ownership to the session (it destroys the old one).
		a.session.SetTextAtlas(tex, view)
		a.textEngine.MarkClean(idx)
	}

	return nil
}

// DrawGlyphMaskText queues text for GPU glyph mask rendering (Tier 6).
// The face parameter must be a text.Face. Glyphs are CPU-rasterized into
// the R8 alpha atlas at the exact device pixel size, then rendered as
// textured quads in the unified render pass.
//
// Returns ErrFallbackToCPU if the GPU is not ready or the face type is wrong.
//
//nolint:dupl // Intentional: mirrors DrawText structure for MSDF; each method owns its engine/batch type.
func (a *SDFAccelerator) DrawGlyphMaskText(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	textFace, ok := face.(text.Face)
	if !ok || textFace == nil {
		return gg.ErrFallbackToCPU
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Track text scene stats.
	a.sceneStats.TextCount++

	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// Lazily create the glyph mask engine.
	if a.glyphMaskEngine == nil {
		a.glyphMaskEngine = NewGlyphMaskEngine()
	}

	// If target changed, flush previous batch first.
	if a.pendingTarget != nil && !sameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	batch, err := a.glyphMaskEngine.LayoutText(textFace, s, x, y, color, target.Width, target.Height, matrix, deviceScale)
	if err != nil {
		return gg.ErrFallbackToCPU
	}
	if len(batch.Quads) == 0 {
		return nil // Empty text (e.g., all spaces), nothing to render.
	}

	a.pendingGlyphMaskBatches = append(a.pendingGlyphMaskBatches, batch)
	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

// syncGlyphMaskAtlases uploads dirty R8 atlas pages to GPU textures and
// sets atlas views on the render session for each batch. Must be called
// after accumulating glyph mask batches and before RenderFrame.
func (a *SDFAccelerator) syncGlyphMaskAtlases(batches []GlyphMaskBatch) error {
	if err := a.glyphMaskEngine.SyncAtlasTextures(a.device, a.queue); err != nil {
		return err
	}

	// Ensure the glyph mask pipeline is initialized before setting atlas views.
	// SetGlyphMaskAtlasView requires the pipeline's uniformLayout and sampler.
	if err := a.session.ensureGlyphMaskPipeline(); err != nil {
		return err
	}

	// Bind the atlas texture view for each batch.
	for i, batch := range batches {
		view := a.glyphMaskEngine.PageTextureView(batch.AtlasPageIndex)
		if view == nil {
			continue
		}
		a.session.SetGlyphMaskAtlasView(i, view)
	}
	return nil
}

func (a *SDFAccelerator) queueShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint, stroked bool) error {
	// If target changed, flush previous batch first.
	if a.pendingTarget != nil && !sameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	rs, ok := DetectedShapeToRenderShape(shape, paint, stroked)
	if !ok {
		return gg.ErrFallbackToCPU
	}
	a.pendingShapes = append(a.pendingShapes, rs)

	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

func sameTarget(a *gg.GPURenderTarget, b *gg.GPURenderTarget) bool {
	return a.Width == b.Width && a.Height == b.Height &&
		len(a.Data) == len(b.Data) && len(a.Data) > 0 && &a.Data[0] == &b.Data[0]
}

func (a *SDFAccelerator) initGPU() error {
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		return fmt.Errorf("vulkan backend not available")
	}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Flags: 0})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	a.instance = instance
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		return fmt.Errorf("no GPU adapters found")
	}
	var selected *hal.ExposedAdapter
	for i := range adapters {
		if adapters[i].Info.DeviceType == gputypes.DeviceTypeDiscreteGPU ||
			adapters[i].Info.DeviceType == gputypes.DeviceTypeIntegratedGPU {
			selected = &adapters[i]
			break
		}
	}
	if selected == nil {
		selected = &adapters[0]
	}
	openDev, err := selected.Adapter.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	a.device = openDev.Device
	a.queue = openDev.Queue

	// Create pipelines and session.
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	a.convexRenderer = NewConvexRenderer(a.device, a.queue)
	a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
	a.session = NewGPURenderSession(a.device, a.queue)
	a.session.SetSDFPipeline(a.sdfRenderPipeline)
	a.session.SetConvexRenderer(a.convexRenderer)
	a.session.SetStencilRenderer(a.stencilRenderer)

	a.gpuReady = true

	// Initialize internal VelloAccelerator for compute routing.
	a.initVelloAccelerator(a.device, a.queue)

	slogger().Info("GPU accelerator initialized", "adapter", selected.Info.Name)
	return nil
}

// initVelloAccelerator creates the internal VelloAccelerator and sets its
// device from the provided HAL device/queue. This is called lazily from
// SetDeviceProvider or initGPU. Failures are non-fatal — compute just
// won't be available.
func (a *SDFAccelerator) initVelloAccelerator(device hal.Device, queue hal.Queue) {
	va := &VelloAccelerator{}
	va.device = device
	va.queue = queue
	va.externalDevice = true
	va.gpuReady = true

	// Create dispatcher with the provided device/queue.
	dispatcher := NewVelloComputeDispatcher(device, queue)
	if err := dispatcher.Init(); err != nil {
		slogger().Debug("VelloAccelerator init: compute pipeline unavailable", "error", err)
		// Still keep the VelloAccelerator for accumulation — CanCompute() returns false.
		a.velloAccel = va
		return
	}
	va.dispatcher = dispatcher
	a.velloAccel = va
	slogger().Debug("VelloAccelerator initialized for compute routing")
}

// effectivePipelineMode determines the actual pipeline mode for the current
// frame based on the configured mode, scene statistics, and GPU capabilities.
// Must be called with a.mu held.
func (a *SDFAccelerator) effectivePipelineMode() gg.PipelineMode {
	mode := a.pipelineMode
	if mode == gg.PipelineModeAuto {
		hasCompute := a.velloAccel != nil && a.velloAccel.CanCompute()
		mode = gg.SelectPipeline(a.sceneStats, hasCompute)
	}
	return mode
}

func getColorFromPaint(paint *gg.Paint) gg.RGBA {
	if paint.Brush != nil {
		if sb, isSolid := paint.Brush.(gg.SolidBrush); isSolid {
			return sb.Color
		}
		return paint.Brush.ColorAt(0, 0)
	}
	return gg.Black
}
