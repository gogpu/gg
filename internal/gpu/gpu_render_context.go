//go:build !nogpu

package gpu

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/stroke"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// GPURenderContext holds per-gg.Context GPU state: pending draw commands,
// clip state, frame tracking, and its own render session. Each gg.Context
// lazily creates one GPURenderContext, ensuring isolated pending command
// queues and independent LoadOp tracking.
//
// This follows the enterprise pattern: Skia SurfaceFillContext.fOpsTask
// (per-surface), Flutter EntityPass (per-layer), Vello Scene (per-call).
//
// GPURenderContext references the shared GPUShared for device, pipelines,
// and atlas engines but never owns them.
type GPURenderContext struct {
	shared *GPUShared // reference to shared resources (NOT owned)

	// Per-context render session (owns frame textures: MSAA, depth, resolve).
	session *GPURenderSession

	// Per-context pending command queues.
	pendingShapes             []SDFRenderShape
	pendingConvexCommands     []ConvexDrawCommand
	pendingStencilPaths       []StencilPathCommand
	pendingImageCommands      []ImageDrawCommand
	pendingGPUTextureCommands []GPUTextureDrawCommand
	pendingTextBatches        []TextBatch
	pendingGlyphMaskBatches   []GlyphMaskBatch
	baseLayer                 *GPUTextureDrawCommand
	pendingTarget             gg.GPURenderTarget
	hasPendingTarget          bool

	// Per-context clip state.
	clipRect        *[4]uint32
	clipRRect       *ClipParams
	clipPath        *gg.Path // arbitrary clip path for depth clipping (GPU-CLIP-003a)
	scissorSegments []scissorSegment

	// Per-context frame tracking (fixes LoadOp corruption).
	// When frameRendered is true, subsequent render passes use LoadOpLoad.
	// Reset by BeginFrame() at the start of each frame.
	frameRendered bool
	lastView      *wgpu.TextureView

	// Per-context scene stats (for Auto pipeline mode).
	sceneStats   gg.SceneStats
	pipelineMode gg.PipelineMode

	// Shared command encoder for single-command-buffer frames (ADR-017).
	// When set, Flush records render passes into this encoder instead of
	// creating its own + submitting. The caller owns Finish + Submit.
	sharedEncoder *wgpu.CommandEncoder
}

// PendingCount returns the total number of pending commands (for testing).
func (rc *GPURenderContext) PendingCount() int {
	n := len(rc.pendingShapes) + len(rc.pendingConvexCommands) +
		len(rc.pendingStencilPaths) + len(rc.pendingImageCommands) + len(rc.pendingGPUTextureCommands) +
		len(rc.pendingTextBatches) + len(rc.pendingGlyphMaskBatches)
	if rc.baseLayer != nil {
		n++
	}
	return n
}

// SetPipelineMode sets the pipeline mode for this context's operations.
func (rc *GPURenderContext) SetPipelineMode(mode gg.PipelineMode) {
	rc.pipelineMode = mode
}

// SetClipRect records a scissor rect change for this context.
func (rc *GPURenderContext) SetClipRect(x, y, w, h uint32) {
	rect := [4]uint32{x, y, w, h}
	rc.clipRect = &rect
	rc.recordScissorSegment(&rect)
}

// ClearClipRect removes the scissor rect for this context.
func (rc *GPURenderContext) ClearClipRect() {
	rc.clipRect = nil
	rc.recordScissorSegment(nil)
}

// SetClipRRect sets the rounded rectangle clip for this context.
func (rc *GPURenderContext) SetClipRRect(x, y, w, h, radius float32) {
	rc.clipRRect = &ClipParams{
		RectX1:  x,
		RectY1:  y,
		RectX2:  x + w,
		RectY2:  y + h,
		Radius:  radius,
		Enabled: 1.0,
	}
	rc.recordScissorSegment(rc.clipRect)
}

// ClearClipRRect removes the rounded rectangle clip for this context.
func (rc *GPURenderContext) ClearClipRRect() {
	rc.clipRRect = nil
	rc.recordScissorSegment(rc.clipRect)
}

// SetClipPath sets an arbitrary clip path for depth-based clipping (GPU-CLIP-003a).
// The path must be in device-space coordinates. When set, subsequent draws are
// clipped to the path region via the depth buffer. The path is fan-tessellated
// and rendered to the depth buffer before content; content fragments test against
// the clip depth so only pixels within the clipped region pass.
func (rc *GPURenderContext) SetClipPath(path *gg.Path) {
	rc.clipPath = path
	rc.recordScissorSegment(rc.clipRect)
}

// ClearClipPath removes the arbitrary clip path, restoring full rendering.
func (rc *GPURenderContext) ClearClipPath() {
	rc.clipPath = nil
	rc.recordScissorSegment(rc.clipRect)
}

// BeginFrame resets per-frame state so the first render pass clears the surface.
func (rc *GPURenderContext) BeginFrame() {
	rc.clipRect = nil
	rc.clipPath = nil
	rc.frameRendered = false
	rc.lastView = nil
}

// SetSharedEncoder sets a shared command encoder for single-command-buffer
// frames (ADR-017). When set, Flush() records render passes into this encoder
// instead of creating its own and submitting. The caller is responsible for
// encoder.Finish() + queue.Submit() after all contexts have flushed.
// Pass a zero-value CommandEncoder (IsNil() == true) to restore normal
// per-context submit behavior.
func (rc *GPURenderContext) SetSharedEncoder(encoder gpucontext.CommandEncoder) {
	if encoder.IsNil() {
		rc.sharedEncoder = nil
		return
	}
	rc.sharedEncoder = (*wgpu.CommandEncoder)(encoder.Pointer())
}

// CreateEncoder creates a new command encoder for shared use across contexts.
// Returns a zero-value CommandEncoder (IsNil() == true) if the session is not
// initialized or encoder creation fails.
func (rc *GPURenderContext) CreateEncoder() gpucontext.CommandEncoder {
	if rc.session == nil {
		return gpucontext.CommandEncoder{}
	}
	enc, err := rc.session.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "shared_frame_encoder",
	})
	if err != nil {
		return gpucontext.CommandEncoder{}
	}
	return gpucontext.NewCommandEncoder(unsafe.Pointer(enc)) //nolint:gosec // Go spec Rule 1 (ADR-018)
}

// SubmitEncoder finishes the shared encoder and submits the command buffer.
func (rc *GPURenderContext) SubmitEncoder(encoder gpucontext.CommandEncoder) error {
	if rc.session == nil {
		return fmt.Errorf("GPU session not initialized")
	}
	if encoder.IsNil() {
		return fmt.Errorf("nil command encoder")
	}
	enc := (*wgpu.CommandEncoder)(encoder.Pointer())
	cmdBuf, err := enc.Finish()
	if err != nil {
		return fmt.Errorf("finish shared encoder: %w", err)
	}
	if _, err := rc.session.queue.Submit(cmdBuf); err != nil {
		rc.session.device.FreeCommandBuffer(cmdBuf)
		return fmt.Errorf("submit shared encoder: %w", err)
	}
	return nil
}

// SceneStats returns the accumulated scene statistics for this context.
func (rc *GPURenderContext) SceneStats() gg.SceneStats {
	return rc.sceneStats
}

// QueueShape accumulates an SDF shape for batch dispatch.
func (rc *GPURenderContext) QueueShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint, stroked bool) error {
	// If target changed, flush previous batch first.
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if err := rc.Flush(rc.pendingTarget); err != nil {
			return err
		}
	}

	rs, ok := DetectedShapeToRenderShape(shape, paint, stroked)
	if !ok {
		return gg.ErrFallbackToCPU
	}
	rc.pendingShapes = append(rc.pendingShapes, rs)

	rc.pendingTarget = target
	rc.hasPendingTarget = true
	return nil
}

// QueueConvex accumulates a convex polygon for batch dispatch.
func (rc *GPURenderContext) QueueConvex(target gg.GPURenderTarget, cmd ConvexDrawCommand) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingConvexCommands = append(rc.pendingConvexCommands, cmd)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueStencil accumulates a stencil path for batch dispatch.
func (rc *GPURenderContext) QueueStencil(target gg.GPURenderTarget, cmd StencilPathCommand) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingStencilPaths = append(rc.pendingStencilPaths, cmd)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueText accumulates an MSDF text batch for dispatch.
func (rc *GPURenderContext) QueueText(target gg.GPURenderTarget, batch TextBatch) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingTextBatches = append(rc.pendingTextBatches, batch)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueImageDraw accumulates an image draw command for Tier 3 dispatch.
// Parameters are kept primitive to avoid import cycles (gg root -> internal/gpu).
func (rc *GPURenderContext) QueueImageDraw(target gg.GPURenderTarget, pixelData []byte, genID uint64, imgWidth, imgHeight, imgStride int,
	dstX, dstY, dstW, dstH, opacity float32, viewportW, viewportH uint32,
	u0, v0, u1, v1 float32,
) {
	cmd := ImageDrawCommand{
		PixelData:      pixelData,
		GenerationID:   genID,
		ImgWidth:       imgWidth,
		ImgHeight:      imgHeight,
		ImgStride:      imgStride,
		DstX:           dstX,
		DstY:           dstY,
		DstW:           dstW,
		DstH:           dstH,
		Opacity:        opacity,
		ViewportWidth:  viewportW,
		ViewportHeight: viewportH,
		U0:             u0,
		V0:             v0,
		U1:             u1,
		V1:             v1,
	}
	rc.queueImageCmd(target, cmd)
}

// queueImageCmd accumulates an image draw command for Tier 3 dispatch.
func (rc *GPURenderContext) queueImageCmd(target gg.GPURenderTarget, cmd ImageDrawCommand) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingImageCommands = append(rc.pendingImageCommands, cmd)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueBaseLayer sets the compositor base layer — a textured quad drawn BEFORE
// all tiers in the render pass. Last call wins. Used for CPU pixmap compositing
// in zero-readback rendering (ADR-015, Flutter OffsetLayer pattern).
func (rc *GPURenderContext) QueueBaseLayer(target gg.GPURenderTarget, view gpucontext.TextureView,
	dstX, dstY, dstW, dstH, opacity float32, vpW, vpH uint32,
) {
	rc.baseLayer = &GPUTextureDrawCommand{
		View: view, DstX: dstX, DstY: dstY, DstW: dstW, DstH: dstH,
		Opacity: opacity, ViewportWidth: vpW, ViewportHeight: vpH,
	}
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueGPUTextureDraw queues a GPU-to-GPU texture compositing command.
// The texture view is sampled directly — zero CPU readback, zero upload.
func (rc *GPURenderContext) QueueGPUTextureDraw(target gg.GPURenderTarget, view gpucontext.TextureView,
	dstX, dstY, dstW, dstH, opacity float32, vpW, vpH uint32,
) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingGPUTextureCommands = append(rc.pendingGPUTextureCommands, GPUTextureDrawCommand{
		View: view, DstX: dstX, DstY: dstY, DstW: dstW, DstH: dstH,
		Opacity: opacity, ViewportWidth: vpW, ViewportHeight: vpH,
	})
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueGlyphMask accumulates a glyph mask batch for dispatch.
func (rc *GPURenderContext) QueueGlyphMask(target gg.GPURenderTarget, batch GlyphMaskBatch) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingGlyphMaskBatches = append(rc.pendingGlyphMaskBatches, batch)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// DrawText shapes and queues text for MSDF rendering (Tier 4).
func (rc *GPURenderContext) DrawText(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	textFace, ok := face.(text.Face)
	if !ok || textFace == nil {
		return gg.ErrFallbackToCPU
	}

	rc.sceneStats.TextCount++

	if !rc.shared.gpuReady {
		rc.shared.mu.Lock()
		err := rc.shared.ensureGPU()
		rc.shared.mu.Unlock()
		if err != nil || !rc.shared.gpuReady {
			return gg.ErrFallbackToCPU
		}
	}

	rc.shared.mu.Lock()
	rc.shared.ensureTextEngine()
	engine := rc.shared.textEngine
	rc.shared.mu.Unlock()

	batch, err := engine.LayoutText(textFace, s, x, y, color, matrix, deviceScale)
	if err != nil {
		slogger().Debug("DrawText: LayoutText failed", "err", err, "text", s)
		return gg.ErrFallbackToCPU
	}
	if len(batch.Quads) == 0 {
		return nil
	}

	rc.QueueText(target, batch)
	return nil
}

// DrawGlyphMaskText shapes and queues text for glyph mask rendering (Tier 6).
func (rc *GPURenderContext) DrawGlyphMaskText(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	textFace, ok := face.(text.Face)
	if !ok || textFace == nil {
		return gg.ErrFallbackToCPU
	}

	rc.sceneStats.TextCount++

	if !rc.shared.gpuReady {
		rc.shared.mu.Lock()
		err := rc.shared.ensureGPU()
		rc.shared.mu.Unlock()
		if err != nil || !rc.shared.gpuReady {
			return gg.ErrFallbackToCPU
		}
	}

	rc.shared.mu.Lock()
	rc.shared.ensureGlyphMaskEngine()
	engine := rc.shared.glyphMaskEngine
	rc.shared.mu.Unlock()

	batch, err := engine.LayoutText(textFace, s, x, y, color, matrix, deviceScale)
	if err != nil {
		slogger().Debug("DrawGlyphMaskText: LayoutText failed", "err", err, "text", s, "w", target.Width, "h", target.Height)
		return gg.ErrFallbackToCPU
	}
	if len(batch.Quads) == 0 {
		return nil
	}

	rc.QueueGlyphMask(target, batch)
	return nil
}

// DrawShapedGlyphMaskText renders pre-shaped glyphs through the glyph mask pipeline.
// Same as DrawGlyphMaskText but skips shaping — uses stored glyph positions directly.
func (rc *GPURenderContext) DrawShapedGlyphMaskText(target gg.GPURenderTarget, face any, glyphs []text.ShapedGlyph, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	textFace, ok := face.(text.Face)
	if !ok || textFace == nil {
		return gg.ErrFallbackToCPU
	}

	rc.sceneStats.TextCount++

	if !rc.shared.gpuReady {
		rc.shared.mu.Lock()
		err := rc.shared.ensureGPU()
		rc.shared.mu.Unlock()
		if err != nil || !rc.shared.gpuReady {
			return gg.ErrFallbackToCPU
		}
	}

	rc.shared.mu.Lock()
	rc.shared.ensureGlyphMaskEngine()
	engine := rc.shared.glyphMaskEngine
	rc.shared.mu.Unlock()

	batch, err := engine.LayoutShapedGlyphs(textFace, glyphs, x, y, color, matrix, deviceScale)
	if err != nil {
		return gg.ErrFallbackToCPU
	}
	if len(batch.Quads) == 0 {
		return nil
	}

	rc.QueueGlyphMask(target, batch)
	return nil
}

// FillPath queues a filled path for GPU rendering.
func (rc *GPURenderContext) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	if !rc.shared.gpuReady {
		return gg.ErrFallbackToCPU
	}

	rc.sceneStats.PathCount++
	rc.sceneStats.ShapeCount++

	// If in Compute mode, delegate to VelloAccelerator.
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			return va.FillPath(target, path, paint)
		}
	}

	// If target changed, flush previous batch first.
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if err := rc.Flush(rc.pendingTarget); err != nil {
			return err
		}
	}

	color := getColorFromPaint(paint)
	premulR := float32(color.R * color.A)
	premulG := float32(color.G * color.A)
	premulB := float32(color.B * color.A)
	premulA := float32(color.A)

	// Try convex fast-path.
	if points, ok := extractConvexPolygon(path); ok {
		cmd := ConvexDrawCommand{
			Points: points,
			Color:  [4]float32{premulR, premulG, premulB, premulA},
		}
		rc.QueueConvex(target, cmd)
		return nil
	}

	// Fall back to stencil-then-cover.
	tess := NewFanTessellator()
	tess.TessellatePath(path)
	fanVerts := tess.Vertices()
	if len(fanVerts) == 0 {
		return nil
	}

	cmd := StencilPathCommand{
		Vertices:  make([]float32, len(fanVerts)),
		CoverQuad: tess.CoverQuad(),
		Color:     [4]float32{premulR, premulG, premulB, premulA},
		FillRule:  paint.FillRule,
	}
	copy(cmd.Vertices, fanVerts)
	rc.QueueStencil(target, cmd)
	return nil
}

// StrokePath renders a stroked path by expanding to filled outline.
func (rc *GPURenderContext) StrokePath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	if paint.IsDashed() {
		return gg.ErrFallbackToCPU
	}

	rc.sceneStats.PathCount++
	rc.sceneStats.ShapeCount++

	// If in Compute mode, delegate to VelloAccelerator.
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			return va.StrokePath(target, path, paint)
		}
	}

	if path.NumVerbs() == 0 {
		return nil
	}

	strokeVerbs := convertPathVerbsToStroke(path.Verbs())
	style := stroke.Stroke{
		Width:      paint.EffectiveLineWidth(),
		Cap:        stroke.LineCap(paint.EffectiveLineCap()),
		Join:       stroke.LineJoin(paint.EffectiveLineJoin()),
		MiterLimit: paint.EffectiveMiterLimit(),
	}
	expander := stroke.NewStrokeExpander(style)
	outVerbs, outCoords := expander.Expand(strokeVerbs, path.Coords())
	if len(outVerbs) == 0 {
		return nil
	}

	fillPath := strokeResultToPath(outVerbs, outCoords)
	return rc.FillPath(target, fillPath, paint)
}

// FillShape accumulates a filled shape for batch dispatch.
func (rc *GPURenderContext) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	rc.sceneStats.ShapeCount++

	if !rc.shared.gpuReady {
		return rc.shared.cpuFallback.FillShape(target, shape, paint)
	}

	// If in Compute mode, delegate to VelloAccelerator.
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			return va.FillShape(target, shape, paint)
		}
	}

	return rc.QueueShape(target, shape, paint, false)
}

// StrokeShape accumulates a stroked shape for batch dispatch.
func (rc *GPURenderContext) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	rc.sceneStats.ShapeCount++

	if !rc.shared.gpuReady {
		return rc.shared.cpuFallback.StrokeShape(target, shape, paint)
	}

	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			return va.StrokeShape(target, shape, paint)
		}
	}

	return rc.QueueShape(target, shape, paint, true)
}

// Flush dispatches all pending commands for this context via the render session.
func (rc *GPURenderContext) Flush(target gg.GPURenderTarget) error { //nolint:cyclop,gocognit,gocyclo,funlen // sequential resource setup + group dispatch
	pending := rc.PendingCount()
	if pending == 0 {
		return rc.flushVello(target)
	}

	rc.shared.mu.Lock()
	// Lazy GPU initialization.
	if rc.shared.device == nil {
		if err := rc.shared.ensureGPU(); err != nil {
			rc.shared.mu.Unlock()
			slogger().Warn("GPU init failed, using CPU fallback", "err", err)
			return gg.ErrFallbackToCPU
		}
	}
	rc.shared.ensurePipelines()

	device := rc.shared.device
	queue := rc.shared.queue
	sdfPipeline := rc.shared.sdfRenderPipeline
	convexRend := rc.shared.convexRenderer
	stencilRend := rc.shared.stencilRenderer
	textEng := rc.shared.textEngine
	glyphEng := rc.shared.glyphMaskEngine
	rc.shared.mu.Unlock()

	// Ensure session exists with all renderers.
	if rc.session == nil {
		rc.session = NewGPURenderSession(device, queue)
		rc.session.SetSDFPipeline(sdfPipeline)
		rc.session.SetConvexRenderer(convexRend)
		rc.session.SetStencilRenderer(stencilRend)
	}

	// Transfer per-context frame tracking to session before rendering.
	rc.session.SetFrameState(rc.frameRendered, rc.lastView)

	// Build scissor groups from the timeline.
	groups := rc.buildScissorGroups()

	// Deep-copy each group's slices so we own the data, then clear pending.
	ownedGroups := make([]ScissorGroup, len(groups))
	for i := range groups {
		g := &groups[i]
		ownedGroups[i] = ScissorGroup{Rect: g.Rect, ClipRRect: g.ClipRRect, ClipPath: g.ClipPath, ClipDepthLevel: g.ClipDepthLevel}
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
		if len(g.ImageCommands) > 0 {
			ownedGroups[i].ImageCommands = make([]ImageDrawCommand, len(g.ImageCommands))
			copy(ownedGroups[i].ImageCommands, g.ImageCommands)
		}
		if len(g.GPUTextureCommands) > 0 {
			ownedGroups[i].GPUTextureCommands = make([]GPUTextureDrawCommand, len(g.GPUTextureCommands))
			copy(ownedGroups[i].GPUTextureCommands, g.GPUTextureCommands)
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
	rc.pendingShapes = rc.pendingShapes[:0]
	rc.pendingConvexCommands = rc.pendingConvexCommands[:0]
	rc.pendingStencilPaths = rc.pendingStencilPaths[:0]
	rc.pendingImageCommands = rc.pendingImageCommands[:0]
	rc.pendingGPUTextureCommands = rc.pendingGPUTextureCommands[:0]
	rc.pendingTextBatches = rc.pendingTextBatches[:0]
	rc.pendingGlyphMaskBatches = rc.pendingGlyphMaskBatches[:0]
	rc.scissorSegments = rc.scissorSegments[:0]
	rc.hasPendingTarget = false
	rc.sceneStats = gg.SceneStats{}

	// Collect all text and glyph mask batches for atlas sync.
	var allTextBatches []TextBatch
	var allGlyphMaskBatches []GlyphMaskBatch
	for i := range ownedGroups {
		allTextBatches = append(allTextBatches, ownedGroups[i].TextBatches...)
		allGlyphMaskBatches = append(allGlyphMaskBatches, ownedGroups[i].GlyphMaskBatches...)
	}

	// Upload dirty MSDF atlases to the GPU before rendering text.
	if len(allTextBatches) > 0 && textEng != nil {
		rc.shared.mu.Lock()
		err := rc.syncTextAtlases()
		rc.shared.mu.Unlock()
		if err != nil {
			slogger().Warn("atlas sync failed", "err", err)
			for i := range ownedGroups {
				ownedGroups[i].TextBatches = nil
			}
		}
	}

	// Upload dirty glyph mask atlas pages.
	if len(allGlyphMaskBatches) > 0 && glyphEng != nil {
		rc.shared.mu.Lock()
		err := rc.syncGlyphMaskAtlases(allGlyphMaskBatches)
		rc.shared.mu.Unlock()
		if err != nil {
			slogger().Warn("glyph mask atlas sync failed", "err", err)
			for i := range ownedGroups {
				ownedGroups[i].GlyphMaskBatches = nil
			}
		}
	}

	// Propagate shared atlas texture to this session (may differ from session's own).
	// This ensures offscreen sessions see the atlas even if they didn't sync it.
	if rc.shared.sharedAtlasView != nil {
		rc.session.SetTextAtlasRef(rc.shared.sharedAtlasTex, rc.shared.sharedAtlasView)
	}

	// Propagate glyph mask atlas page views for offscreen sessions.
	// Same pattern as MSDF atlas — engine is shared, views must reach each session.
	if len(allGlyphMaskBatches) > 0 && glyphEng != nil {
		for i, batch := range allGlyphMaskBatches {
			view := glyphEng.PageTextureView(batch.AtlasPageIndex)
			if view != nil {
				rc.session.SetGlyphMaskAtlasView(i, view, batch.IsLCD)
			}
		}
	}

	baseLayer := rc.baseLayer
	rc.baseLayer = nil

	err := rc.session.RenderFrameGrouped(target, ownedGroups, baseLayer, rc.sharedEncoder)
	if err != nil {
		total := 0
		for i := range ownedGroups {
			total += len(ownedGroups[i].SDFShapes) + len(ownedGroups[i].ConvexCommands) + len(ownedGroups[i].StencilPaths) +
				len(ownedGroups[i].ImageCommands) + len(ownedGroups[i].TextBatches) + len(ownedGroups[i].GlyphMaskBatches)
		}
		slogger().Warn("render session error",
			"groups", len(ownedGroups), "totalItems", total, "err", err)
	}

	// Read back frame tracking from session.
	rc.frameRendered, rc.lastView = rc.session.FrameState()

	return err
}

// flushVello flushes Vello compute if it has pending paths.
func (rc *GPURenderContext) flushVello(target gg.GPURenderTarget) error {
	effectiveMode := rc.effectivePipelineMode()
	rc.shared.mu.Lock()
	va := rc.shared.velloAccel
	rc.shared.mu.Unlock()
	if va != nil && va.PendingCount() > 0 && effectiveMode == gg.PipelineModeCompute {
		if err := va.Flush(target); err != nil {
			slogger().Debug("vello compute flush failed", "err", err)
		}
	}
	rc.sceneStats = gg.SceneStats{}
	return nil
}

// effectivePipelineMode determines the actual mode for this flush.
func (rc *GPURenderContext) effectivePipelineMode() gg.PipelineMode {
	mode := rc.pipelineMode
	if mode == gg.PipelineModeAuto {
		rc.shared.mu.Lock()
		hasCompute := rc.shared.velloAccel != nil && rc.shared.velloAccel.CanCompute()
		rc.shared.mu.Unlock()
		mode = gg.SelectPipeline(rc.sceneStats, hasCompute)
	}
	return mode
}

// CreateOffscreenTexture allocates a GPU texture for offscreen rendering.
// The texture has usage flags suitable for both FlushGPUWithView (render to)
// and DrawGPUTexture (sample from). Returns view + release function.
func (rc *GPURenderContext) CreateOffscreenTexture(w, h int) (gpucontext.TextureView, func()) {
	if rc.shared == nil {
		return gpucontext.TextureView{}, nil
	}
	if !rc.shared.gpuReady {
		rc.shared.mu.Lock()
		err := rc.shared.ensureGPU()
		rc.shared.mu.Unlock()
		if err != nil || !rc.shared.gpuReady {
			return gpucontext.TextureView{}, nil
		}
	}
	device := rc.shared.Device()
	if device == nil {
		return gpucontext.TextureView{}, nil
	}

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "offscreen_cache",
		Size:          wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}, //nolint:gosec // bounded
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		return gpucontext.TextureView{}, nil
	}

	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label:         "offscreen_cache_view",
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Dimension:     gputypes.TextureViewDimension2D,
		Aspect:        gputypes.TextureAspectAll,
		MipLevelCount: 1,
	})
	if err != nil {
		tex.Release()
		return gpucontext.TextureView{}, nil
	}

	release := func() {
		view.Release()
		tex.Release()
	}
	return gpucontext.NewTextureView(unsafe.Pointer(view)), release //nolint:gosec // Go spec Rule 1 (ADR-018)
}

// Close releases this context's GPU resources. Shared resources are NOT
// released — they are owned by GPUShared.
func (rc *GPURenderContext) Close() {
	if rc.session != nil {
		if tp := rc.session.TextPipelineRef(); tp != nil {
			tp.Destroy()
		}
		rc.session.Destroy()
		rc.session = nil
	}
	rc.pendingShapes = nil
	rc.pendingConvexCommands = nil
	rc.pendingStencilPaths = nil
	rc.pendingImageCommands = nil
	rc.pendingGPUTextureCommands = nil
	rc.baseLayer = nil
	rc.pendingTextBatches = nil
	rc.pendingGlyphMaskBatches = nil
	rc.hasPendingTarget = false
	rc.clipRect = nil
	rc.clipRRect = nil
	rc.clipPath = nil
	rc.scissorSegments = nil
	rc.sceneStats = gg.SceneStats{}
}

// recordScissorSegment records a scissor state change in the timeline.
func (rc *GPURenderContext) recordScissorSegment(rect *[4]uint32) {
	seg := scissorSegment{
		sdfCount:     len(rc.pendingShapes),
		convexCount:  len(rc.pendingConvexCommands),
		stencilCount: len(rc.pendingStencilPaths),
		imageCount:   len(rc.pendingImageCommands),
		gpuTexCount:  len(rc.pendingGPUTextureCommands),
		textCount:    len(rc.pendingTextBatches),
		glyphCount:   len(rc.pendingGlyphMaskBatches),
	}
	if rect != nil {
		seg.rect = *rect
		seg.hasRect = true
	}
	if rc.clipRRect != nil {
		seg.clipRRect = *rc.clipRRect
		seg.hasClipRRect = true
	}
	seg.clipPath = rc.clipPath
	rc.scissorSegments = append(rc.scissorSegments, seg)
}

// buildScissorGroups builds scissor groups from the pending commands and timeline.
func (rc *GPURenderContext) buildScissorGroups() []ScissorGroup {
	if len(rc.scissorSegments) == 0 {
		return []ScissorGroup{{
			Rect:               nil,
			SDFShapes:          rc.pendingShapes,
			ConvexCommands:     rc.pendingConvexCommands,
			StencilPaths:       rc.pendingStencilPaths,
			ImageCommands:      rc.pendingImageCommands,
			GPUTextureCommands: rc.pendingGPUTextureCommands,
			TextBatches:        rc.pendingTextBatches,
			GlyphMaskBatches:   rc.pendingGlyphMaskBatches,
		}}
	}

	var groups []ScissorGroup

	firstSeg := rc.scissorSegments[0]
	if firstSeg.sdfCount > 0 || firstSeg.convexCount > 0 || firstSeg.stencilCount > 0 ||
		firstSeg.imageCount > 0 || firstSeg.gpuTexCount > 0 || firstSeg.textCount > 0 || firstSeg.glyphCount > 0 {
		groups = append(groups, ScissorGroup{
			Rect:               nil,
			SDFShapes:          rc.pendingShapes[:firstSeg.sdfCount],
			ConvexCommands:     rc.pendingConvexCommands[:firstSeg.convexCount],
			StencilPaths:       rc.pendingStencilPaths[:firstSeg.stencilCount],
			ImageCommands:      rc.pendingImageCommands[:firstSeg.imageCount],
			GPUTextureCommands: rc.pendingGPUTextureCommands[:firstSeg.gpuTexCount],
			TextBatches:        rc.pendingTextBatches[:firstSeg.textCount],
			GlyphMaskBatches:   rc.pendingGlyphMaskBatches[:firstSeg.glyphCount],
		})
	}

	for i, seg := range rc.scissorSegments {
		var endSDF, endConvex, endStencil, endImage, endGPUTex, endText, endGlyph int
		if i+1 < len(rc.scissorSegments) {
			next := rc.scissorSegments[i+1]
			endSDF = next.sdfCount
			endConvex = next.convexCount
			endStencil = next.stencilCount
			endImage = next.imageCount
			endGPUTex = next.gpuTexCount
			endText = next.textCount
			endGlyph = next.glyphCount
		} else {
			endSDF = len(rc.pendingShapes)
			endConvex = len(rc.pendingConvexCommands)
			endStencil = len(rc.pendingStencilPaths)
			endImage = len(rc.pendingImageCommands)
			endGPUTex = len(rc.pendingGPUTextureCommands)
			endText = len(rc.pendingTextBatches)
			endGlyph = len(rc.pendingGlyphMaskBatches)
		}

		if seg.sdfCount == endSDF && seg.convexCount == endConvex &&
			seg.stencilCount == endStencil && seg.imageCount == endImage &&
			seg.gpuTexCount == endGPUTex && seg.textCount == endText && seg.glyphCount == endGlyph {
			continue
		}

		var groupRect *[4]uint32
		if seg.hasRect {
			r := seg.rect
			groupRect = &r
		}
		var groupClip *ClipParams
		if seg.hasClipRRect {
			c := seg.clipRRect
			groupClip = &c
		}
		groups = append(groups, ScissorGroup{
			Rect:               groupRect,
			ClipRRect:          groupClip,
			ClipPath:           seg.clipPath,
			SDFShapes:          rc.pendingShapes[seg.sdfCount:endSDF],
			ConvexCommands:     rc.pendingConvexCommands[seg.convexCount:endConvex],
			StencilPaths:       rc.pendingStencilPaths[seg.stencilCount:endStencil],
			ImageCommands:      rc.pendingImageCommands[seg.imageCount:endImage],
			GPUTextureCommands: rc.pendingGPUTextureCommands[seg.gpuTexCount:endGPUTex],
			TextBatches:        rc.pendingTextBatches[seg.textCount:endText],
			GlyphMaskBatches:   rc.pendingGlyphMaskBatches[seg.glyphCount:endGlyph],
		})
	}

	return groups
}

// syncTextAtlases uploads dirty MSDF atlas pages. Must be called with shared.mu held.
func (rc *GPURenderContext) syncTextAtlases() error {
	s := rc.shared
	dirtyIndices := s.textEngine.DirtyAtlases()
	if len(dirtyIndices) == 0 {
		return nil
	}

	for _, idx := range dirtyIndices {
		rgbaData, size, _ := s.textEngine.AtlasRGBAData(idx)
		if rgbaData == nil || size == 0 {
			continue
		}

		atlasSize := uint32(size) //nolint:gosec // atlas size always fits uint32

		tex, err := s.device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         fmt.Sprintf("msdf_atlas_%d", idx),
			Size:          wgpu.Extent3D{Width: atlasSize, Height: atlasSize, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
		})
		if err != nil {
			return fmt.Errorf("create atlas texture %d: %w", idx, err)
		}

		view, err := s.device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
			Label:         fmt.Sprintf("msdf_atlas_%d_view", idx),
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Dimension:     gputypes.TextureViewDimension2D,
			Aspect:        gputypes.TextureAspectAll,
			MipLevelCount: 1,
		})
		if err != nil {
			tex.Release()
			return fmt.Errorf("create atlas texture view %d: %w", idx, err)
		}

		if err := s.queue.WriteTexture(
			&wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0},
			rgbaData,
			&wgpu.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  atlasSize * 4,
				RowsPerImage: atlasSize,
			},
			&wgpu.Extent3D{Width: atlasSize, Height: atlasSize, DepthOrArrayLayers: 1},
		); err != nil {
			tex.Release()
			return fmt.Errorf("upload atlas texture %d: %w", idx, err)
		}

		// Store atlas in GPUShared (shared across all contexts).
		if s.sharedAtlasView != nil {
			s.sharedAtlasView.Release()
		}
		if s.sharedAtlasTex != nil {
			s.sharedAtlasTex.Release()
		}
		s.sharedAtlasTex = tex
		s.sharedAtlasView = view
		s.textEngine.MarkClean(idx)
	}
	return nil
}

// syncGlyphMaskAtlases uploads dirty R8 atlas pages. Must be called with shared.mu held.
func (rc *GPURenderContext) syncGlyphMaskAtlases(batches []GlyphMaskBatch) error {
	s := rc.shared
	if err := s.glyphMaskEngine.SyncAtlasTextures(s.device, s.queue); err != nil {
		return err
	}

	hasLCD := false
	for i := range batches {
		if batches[i].IsLCD {
			hasLCD = true
			break
		}
	}

	if err := rc.session.ensureGlyphMaskPipeline(hasLCD); err != nil {
		return err
	}

	for i, batch := range batches {
		view := s.glyphMaskEngine.PageTextureView(batch.AtlasPageIndex)
		if view == nil {
			slogger().Warn("glyph mask atlas page not synced — text skipped",
				"pageIndex", batch.AtlasPageIndex, "batchIndex", i, "quads", len(batch.Quads))
			continue
		}
		rc.session.SetGlyphMaskAtlasView(i, view, batch.IsLCD)
	}
	return nil
}
