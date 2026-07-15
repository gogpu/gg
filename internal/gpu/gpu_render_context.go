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

// drawCommandKind identifies the type of a backend-agnostic draw command.
// Shapes and paths are stored in their original gg representation at queue time
// and converted to GPU-specific or CPU-specific data at Flush dispatch time.
// This separation follows the Skia Graphite DrawList / Flutter DisplayList pattern.
type drawCommandKind uint8

const (
	drawCmdFillShape     drawCommandKind = iota // SDF shapes (circle, rect, rrect, ellipse)
	drawCmdStrokeShape                          // SDF stroked shapes
	drawCmdFillPath                             // complex paths -> convex/stencil
	drawCmdStrokePath                           // stroked paths (pre-expanded)
	drawCmdText                                 // MSDF text batch (Phase 2)
	drawCmdGlyphMaskText                        // glyph-mask text batch (Phase 2)
	drawCmdImage                                // DrawImage (Phase 2)
	drawCmdGPUTexture                           // DrawGPUTexture overlay (Phase 2)
	drawCmdBaseLayer                            // DrawGPUTextureBase, z-order: always first (Phase 2)
)

// drawCommand is a backend-agnostic draw command stored at queue time.
// It holds the original shape/path + paint so dispatch can choose between
// GPU render passes or CPU SoftwareRenderer at Flush time.
//
// For GPU path commands (drawCmdFillPath, drawCmdStrokePath), tessellation
// is performed eagerly at draw time and stored in convexPoints/stencilCmd.
// This avoids re-tessellating every frame at Flush time, which was causing
// 3 FPS regression on animated paths (ADR-051 fix).
//
// Clip state is snapshotted at queue time (Skia Graphite per-draw clip pattern).
// This ensures clip changes between draws produce correct ScissorGroups at flush
// time, regardless of whether draws route through GPU or CPU.
type drawCommand struct {
	kind    drawCommandKind
	sortKey uint64 //nolint:unused // ADR-053 Phase 0: pipeline grouping key. Zero = FIFO order.

	// Geometry — only one group populated per kind (Skia Geometry union pattern).
	shape     gg.DetectedShape // drawCmdFillShape, drawCmdStrokeShape
	path      *gg.Path         // drawCmdFillPath, drawCmdStrokePath (COPIED at queue time)
	textBatch any              // drawCmdText: TextBatch, drawCmdGlyphMaskText: GlyphMaskBatch
	imageCmd  any              // drawCmdImage: ImageDrawCommand
	gpuTexCmd any              // drawCmdGPUTexture, drawCmdBaseLayer: GPUTextureDrawCommand

	// Style.
	paint gg.Paint // fill/stroke params (value copy)

	// Pre-tessellated GPU data (computed at draw time, avoids re-tessellation
	// at flush). Exactly one of convexPoints or stencilCmd is set for path
	// commands; both nil for shape commands (SDF pipeline, no tessellation).
	convexPoints []gg.Point          // convex polygon fast-path (Tier 2a)
	stencilCmd   *StencilPathCommand // fan-tessellated vertices (Tier 2b)

	// Per-draw clip state snapshot (ADR-051 Phase 1.1).
	// Captured at queue time so flush can build ScissorGroups from per-draw
	// clip rather than relying on legacy recordScissorSegment timeline.
	clipRect  *[4]uint32  // scissor rect; nil = full framebuffer
	clipRRect *ClipParams // analytic RRect clip; nil = no RRect clip
	clipPath  *gg.Path    // arbitrary clip path for depth clipping; nil = no clip
}

// copyClipRect returns a deep copy of a scissor rect pointer.
// Returns nil if the source is nil (no scissor).
func copyClipRect(r *[4]uint32) *[4]uint32 {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

// copyClipRRect returns a deep copy of a ClipParams pointer.
// Returns nil if the source is nil (no RRect clip).
func copyClipRRect(p *ClipParams) *ClipParams {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// drawClipEqual reports whether two draw commands have the same clip state.
// Used to group consecutive same-clip draws into a single ScissorGroup.
// clipPath uses pointer equality — same path object means same clip region
// (paths are immutable after being set as clip).
func drawClipEqual(a, b *drawCommand) bool {
	// Compare clipRect.
	if (a.clipRect == nil) != (b.clipRect == nil) {
		return false
	}
	if a.clipRect != nil && *a.clipRect != *b.clipRect {
		return false
	}
	// Compare clipRRect.
	if (a.clipRRect == nil) != (b.clipRRect == nil) {
		return false
	}
	if a.clipRRect != nil && *a.clipRRect != *b.clipRRect {
		return false
	}
	// Compare clipPath (pointer equality — same path object = same clip).
	return a.clipPath == b.clipPath
}

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

	// Backend-agnostic draw command queue (ADR-051 Phase 1).
	// Shapes and paths are stored here at queue time in their original gg
	// representation. At Flush time, commands are dispatched to either GPU
	// render passes or CPU SoftwareRenderer depending on strategy.
	pendingDraws []drawCommand

	pendingTarget    gg.GPURenderTarget
	hasPendingTarget bool

	// Per-context clip state.
	clipRect  *[4]uint32
	clipRRect *ClipParams
	clipPath  *gg.Path // arbitrary clip path for depth clipping (GPU-CLIP-003a)

	// Per-context frame tracking (fixes LoadOp corruption).
	// When frameRendered is true, subsequent render passes use LoadOpLoad.
	// Reset by BeginFrame() at the start of each frame.
	frameRendered bool
	lastView      *wgpu.TextureView

	// Per-context scene stats (for Auto pipeline mode).
	sceneStats   gg.SceneStats
	pipelineMode gg.PipelineMode

	// Anti-aliasing state for GPU rendering (propagated from Context).
	antiAlias bool

	// Shared command encoder for single-command-buffer frames (ADR-017).
	// When set, Flush records render passes into this encoder instead of
	// creating its own + submitting. The caller owns Finish + Submit.
	sharedEncoder *wgpu.CommandEncoder

	// --- Cached resources for per-frame allocation elimination ---

	// P0: Pooled BGRA swizzle buffer — avoids 8 MB/frame allocation at 1080p.
	// Grow-only: reused across frames, only reallocated when frame size increases.
	bgraBuffer []byte

	// P0: Cached tmpPixmap for flushCPUToView — avoids 1.9 MB/frame at 800x600.
	// Recreated only when dimensions change (which is <1% of frames).
	cachedPixmap       *gg.Pixmap
	cachedPixmapWidth  int
	cachedPixmapHeight int

	// P1: Cached SoftwareRenderer — avoids 13 allocs + 12-17 KB per flush.
	// Resize() is called only when dimensions change; steady state = 0 allocs.
	cachedSR       *gg.SoftwareRenderer
	cachedSRWidth  int
	cachedSRHeight int

	// P3: Scratch stroke path — avoids 1 KB allocation per strokeResultToPath.
	// Reset and reused across strokes within a frame; same pattern as
	// SoftwareRenderer.scratchStrokePath (Skia fOuter.reset()).
	scratchStrokePath *gg.Path
}

// DrawRecording is an immutable snapshot of draw commands (Skia Recording pattern).
// Created by Snap(), designed for transfer to a dispatch goroutine (ADR-053 Phase 1+).
// Once created, the commands slice is never modified.
type DrawRecording struct {
	commands []drawCommand
}

// Len returns the number of commands in the recording.
func (r DrawRecording) Len() int { return len(r.commands) }

// Snap produces an immutable DrawRecording and resets the queue.
// Follows Skia Graphite Recorder::snap() (Recorder.cpp:196) — ownership of the
// backing array transfers to the recording; the queue starts fresh.
func (rc *GPURenderContext) Snap() DrawRecording {
	rec := DrawRecording{commands: rc.pendingDraws}
	rc.pendingDraws = nil
	return rec
}

// PendingCount returns the total number of pending draw commands (for testing).
func (rc *GPURenderContext) PendingCount() int {
	return len(rc.pendingDraws)
}

// SetPipelineMode sets the pipeline mode for this context's operations.
func (rc *GPURenderContext) SetPipelineMode(mode gg.PipelineMode) {
	rc.pipelineMode = mode
}

// SetAntiAlias sets the anti-aliasing state for GPU rendering.
// When false, SDF shapes use binary step coverage instead of smoothstep.
func (rc *GPURenderContext) SetAntiAlias(enabled bool) {
	rc.antiAlias = enabled
}

// SetClipRect sets the scissor rect for this context. Per-draw clip state is
// snapshotted at queue time, so this only sets the context-level state.
func (rc *GPURenderContext) SetClipRect(x, y, w, h uint32) {
	rect := [4]uint32{x, y, w, h}
	rc.clipRect = &rect
}

// ClearClipRect removes the scissor rect for this context.
func (rc *GPURenderContext) ClearClipRect() {
	rc.clipRect = nil
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
}

// ClearClipRRect removes the rounded rectangle clip for this context.
func (rc *GPURenderContext) ClearClipRRect() {
	rc.clipRRect = nil
}

// SetClipPath sets an arbitrary clip path for depth-based clipping (GPU-CLIP-003a).
// The path must be in device-space coordinates. When set, subsequent draws are
// clipped to the path region via the depth buffer. The path is fan-tessellated
// and rendered to the depth buffer before content; content fragments test against
// the clip depth so only pixels within the clipped region pass.
func (rc *GPURenderContext) SetClipPath(path *gg.Path) {
	rc.clipPath = path
}

// ClearClipPath removes the arbitrary clip path, restoring full rendering.
func (rc *GPURenderContext) ClearClipPath() {
	rc.clipPath = nil
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

// QueueText accumulates an MSDF text batch for dispatch via the unified draw
// queue (ADR-051 Phase 2). Adjacent batches with identical clip AND visual
// properties (transform, color, atlas, MSDF parameters) are coalesced into a
// single batch to minimize GPU draw calls (ADR-031).
func (rc *GPURenderContext) QueueText(target gg.GPURenderTarget, batch TextBatch) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}

	cmd := drawCommand{
		kind:      drawCmdText,
		textBatch: batch,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	}

	// Coalesce with last pending draw if same clip AND same visual properties
	// (ADR-031). Per-draw clip replaces the legacy textBatchSealed flag —
	// clip change = different drawCommand = no merge, naturally.
	if n := len(rc.pendingDraws); n > 0 {
		last := &rc.pendingDraws[n-1]
		if last.kind == drawCmdText && drawClipEqual(last, &cmd) {
			lastBatch := last.textBatch.(TextBatch)
			if lastBatch.CanMerge(batch) {
				lastBatch.Quads = append(lastBatch.Quads, batch.Quads...)
				last.textBatch = lastBatch
				rc.pendingTarget = target
				rc.hasPendingTarget = true
				return
			}
		}
	}

	rc.pendingDraws = append(rc.pendingDraws, cmd)
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

// queueImageCmd accumulates an image draw command for Tier 3 dispatch via the
// unified draw queue (ADR-051 Phase 2 Step 4). Per-draw clip is snapshotted at
// queue time (same pattern as QueueText from Step 3).
func (rc *GPURenderContext) queueImageCmd(target gg.GPURenderTarget, cmd ImageDrawCommand) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind:      drawCmdImage,
		imageCmd:  cmd,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	})
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueBaseLayer sets the compositor base layer — a textured quad drawn BEFORE
// all tiers in the render pass. Last call wins: subsequent calls overwrite the
// previous base layer command. Used for CPU pixmap compositing in zero-readback
// rendering (ADR-015, Flutter OffsetLayer pattern).
//
// ADR-051 Phase 2 Step 5: routed through pendingDraws with drawCmdBaseLayer
// kind. At flush time, the base layer is extracted from pendingDraws and passed
// as a separate parameter to RenderFrameGrouped (preserving existing API).
func (rc *GPURenderContext) QueueBaseLayer(target gg.GPURenderTarget, view gpucontext.TextureView,
	dstX, dstY, dstW, dstH, opacity float32, vpW, vpH uint32,
) {
	cmd := drawCommand{
		kind: drawCmdBaseLayer,
		gpuTexCmd: GPUTextureDrawCommand{
			View: view, DstX: dstX, DstY: dstY, DstW: dstW, DstH: dstH,
			Opacity: opacity, ViewportWidth: vpW, ViewportHeight: vpH,
		},
	}

	// Last call wins: replace any existing drawCmdBaseLayer in pendingDraws.
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdBaseLayer {
			rc.pendingDraws[i] = cmd
			rc.pendingTarget = target
			rc.hasPendingTarget = true
			return
		}
	}

	rc.pendingDraws = append(rc.pendingDraws, cmd)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueGPUTextureDraw queues a GPU-to-GPU texture compositing command via
// the unified draw queue (ADR-051 Phase 2 Step 5). Per-draw clip is
// snapshotted at queue time (same pattern as QueueText from Step 3).
// The texture view is sampled directly — zero CPU readback, zero upload.
func (rc *GPURenderContext) QueueGPUTextureDraw(target gg.GPURenderTarget, view gpucontext.TextureView,
	dstX, dstY, dstW, dstH, opacity float32, vpW, vpH uint32,
) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}
	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind: drawCmdGPUTexture,
		gpuTexCmd: GPUTextureDrawCommand{
			View: view, DstX: dstX, DstY: dstY, DstW: dstW, DstH: dstH,
			Opacity: opacity, ViewportWidth: vpW, ViewportHeight: vpH,
		},
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	})
	rc.pendingTarget = target
	rc.hasPendingTarget = true
}

// QueueGlyphMask accumulates a glyph mask batch for dispatch via the unified
// draw queue (ADR-051 Phase 2). Adjacent batches with identical clip AND visual
// properties (transform, color, LCD mode, atlas page) are coalesced into a
// single batch to minimize GPU draw calls (ADR-031).
func (rc *GPURenderContext) QueueGlyphMask(target gg.GPURenderTarget, batch GlyphMaskBatch) {
	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if fErr := rc.Flush(rc.pendingTarget); fErr != nil {
			slogger().Warn("auto-flush failed", "err", fErr)
		}
	}

	cmd := drawCommand{
		kind:      drawCmdGlyphMaskText,
		textBatch: batch,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	}

	// Coalesce with last pending draw if same clip AND same visual properties
	// (ADR-031). Per-draw clip replaces the legacy glyphBatchSealed flag —
	// clip change = different drawCommand = no merge, naturally.
	if n := len(rc.pendingDraws); n > 0 {
		last := &rc.pendingDraws[n-1]
		if last.kind == drawCmdGlyphMaskText && drawClipEqual(last, &cmd) {
			lastBatch := last.textBatch.(GlyphMaskBatch)
			if lastBatch.CanMerge(batch) {
				lastBatch.Quads = append(lastBatch.Quads, batch.Quads...)
				last.textBatch = lastBatch
				rc.pendingTarget = target
				rc.hasPendingTarget = true
				return
			}
		}
	}

	rc.pendingDraws = append(rc.pendingDraws, cmd)
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

// DrawGlyphMaskTextAliased shapes and queues text for aliased (binary coverage)
// glyph mask rendering. Same pipeline as DrawGlyphMaskText but rasterizes with
// NoAAFiller (0/255 only) instead of AnalyticFiller (256-level AA).
func (rc *GPURenderContext) DrawGlyphMaskTextAliased(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
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

	batch, err := engine.LayoutTextAliased(textFace, s, x, y, color, matrix, deviceScale)
	if err != nil {
		slogger().Debug("DrawGlyphMaskTextAliased: LayoutTextAliased failed", "err", err, "text", s, "w", target.Width, "h", target.Height)
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

	isCJK := len(glyphs) > 0 && glyphs[0].IsCJK
	batch, err := engine.LayoutShapedGlyphs(textFace, glyphs, x, y, color, matrix, deviceScale, isCJK)
	if err != nil {
		return gg.ErrFallbackToCPU
	}
	if len(batch.Quads) == 0 {
		return nil
	}

	rc.QueueGlyphMask(target, batch)
	return nil
}

// FillPath queues a filled path as a backend-agnostic draw command (ADR-051).
// The path is cloned at queue time so the caller can reuse or mutate it.
// At Flush time, the path is dispatched to either GPU (convex/stencil) or CPU.
func (rc *GPURenderContext) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	rc.sceneStats.PathCount++
	rc.sceneStats.ShapeCount++

	// Compute mode delegates directly to VelloAccelerator (separate pipeline).
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			va.SetAntiAlias(rc.antiAlias)
			return va.FillPath(target, path, paint)
		}
	}

	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if err := rc.Flush(rc.pendingTarget); err != nil {
			return err
		}
	}

	cmd := drawCommand{
		kind:      drawCmdFillPath,
		path:      path.Clone(),
		paint:     *paint,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	}
	cmd.paint.ClipCoverage = nil //nolint:staticcheck // M-1: intentional clear of deprecated stale closure

	// Pre-tessellate at draw time to avoid re-tessellation every frame at
	// Flush (ADR-051 fix: 3 FPS → 60 FPS on animated paths). CPU dispatch
	// path uses cmd.path directly via SoftwareRenderer — tessellated data
	// is only consumed by the GPU scissor-group builder.
	rc.preTessellateFill(&cmd)

	rc.pendingDraws = append(rc.pendingDraws, cmd)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
	return nil
}

// StrokePath queues a stroked path as a backend-agnostic draw command (ADR-051).
// Dashed strokes fall back to CPU. The path is cloned at queue time.
func (rc *GPURenderContext) StrokePath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	if paint.IsDashed() {
		return gg.ErrFallbackToCPU
	}

	rc.sceneStats.PathCount++
	rc.sceneStats.ShapeCount++

	// Compute mode delegates directly to VelloAccelerator (separate pipeline).
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			va.SetAntiAlias(rc.antiAlias)
			return va.StrokePath(target, path, paint)
		}
	}

	if path.NumVerbs() == 0 {
		return nil
	}

	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if err := rc.Flush(rc.pendingTarget); err != nil {
			return err
		}
	}

	cmd := drawCommand{
		kind:      drawCmdStrokePath,
		path:      path.Clone(),
		paint:     *paint,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	}
	cmd.paint.ClipCoverage = nil //nolint:staticcheck // M-1: intentional clear of deprecated stale closure

	// Pre-tessellate at draw time: expand stroke geometry, then tessellate
	// the expanded fill path (ADR-051 fix). The expanded path replaces
	// cmd.path so CPU dispatch can use it directly via SoftwareRenderer.
	rc.preTessellateStroke(&cmd)

	rc.pendingDraws = append(rc.pendingDraws, cmd)
	rc.pendingTarget = target
	rc.hasPendingTarget = true
	return nil
}

// FillShape queues a filled shape as a backend-agnostic draw command (ADR-051).
// The shape is dispatched at Flush time to either GPU render passes (SDF/convex)
// or CPU SoftwareRenderer depending on strategy. This ensures offscreen targets
// get isolated rendering regardless of backend.
func (rc *GPURenderContext) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	rc.sceneStats.ShapeCount++

	// Compute mode delegates directly to VelloAccelerator (separate pipeline).
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			va.SetAntiAlias(rc.antiAlias)
			return va.FillShape(target, shape, paint)
		}
	}

	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if err := rc.Flush(rc.pendingTarget); err != nil {
			return err
		}
	}

	cmd := drawCommand{
		kind:      drawCmdFillShape,
		shape:     shape,
		paint:     *paint,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	}
	cmd.paint.ClipCoverage = nil //nolint:staticcheck // M-1: intentional clear of deprecated stale closure
	rc.pendingDraws = append(rc.pendingDraws, cmd)

	rc.pendingTarget = target
	rc.hasPendingTarget = true
	return nil
}

// StrokeShape queues a stroked shape as a backend-agnostic draw command (ADR-051).
// Thin strokes (< 2px) fall back to CPU geometric expansion because SDF annular
// ring is thinner than the smoothstep AA zone (ADR-040).
func (rc *GPURenderContext) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	rc.sceneStats.ShapeCount++

	// Thin strokes fall back to geometric expansion regardless of strategy.
	if paint.EffectiveLineWidth() < 2.0 {
		return gg.ErrFallbackToCPU
	}

	// Compute mode delegates directly to VelloAccelerator (separate pipeline).
	if rc.pipelineMode == gg.PipelineModeCompute {
		rc.shared.mu.Lock()
		va := rc.shared.velloAccel
		rc.shared.mu.Unlock()
		if va != nil && va.CanCompute() {
			va.SetAntiAlias(rc.antiAlias)
			return va.StrokeShape(target, shape, paint)
		}
	}

	if rc.hasPendingTarget && !sameTarget(&rc.pendingTarget, &target) {
		if err := rc.Flush(rc.pendingTarget); err != nil {
			return err
		}
	}

	cmd := drawCommand{
		kind:      drawCmdStrokeShape,
		shape:     shape,
		paint:     *paint,
		clipRect:  copyClipRect(rc.clipRect),
		clipRRect: copyClipRRect(rc.clipRRect),
		clipPath:  rc.clipPath,
	}
	cmd.paint.ClipCoverage = nil //nolint:staticcheck // M-1: intentional clear of deprecated stale closure
	rc.pendingDraws = append(rc.pendingDraws, cmd)

	rc.pendingTarget = target
	rc.hasPendingTarget = true
	return nil
}

// Flush dispatches all pending commands for this context via the render session.
func (rc *GPURenderContext) Flush(target gg.GPURenderTarget) error { //nolint:cyclop,gocognit,gocyclo,funlen // sequential resource setup + group dispatch
	// Dispatch backend-agnostic draw commands (ADR-051).
	// rasterAtlas: mixed CPU/GPU dispatch — shape commands are CPU-dispatched
	// immediately; non-shape commands (images, GPU textures, base layers)
	// are retained in pendingDraws for the standard GPU flush path below.
	// GPU path: all draw types stay in pendingDraws for buildScissorGroupsFromDraws.
	rasterAtlasDispatched := false
	if len(rc.pendingDraws) > 0 && rc.shared.strategy == strategyRasterAtlas {
		rasterAtlasDispatched = true
		if err := rc.dispatchRasterAtlasDraws(target); err != nil {
			return err
		}
	}

	// Check if we have anything to render via the draw queue or Vello.
	if len(rc.pendingDraws) == 0 {
		// rasterAtlas: CPU shapes already in pixmap, upload to offscreen texture.
		// Skip if dispatchRasterAtlasDraws already called flushCPUToView —
		// re-uploading would overwrite the offscreen content with c.pixmap data.
		if !target.View.IsNil() && rc.shared.strategy == strategyRasterAtlas && !rasterAtlasDispatched {
			return rc.uploadPixmapToView(target)
		}
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
		rc.session = NewGPURenderSession(device, queue, rc.shared.SampleCount())
		rc.session.SetSDFPipeline(sdfPipeline)
		rc.session.SetConvexRenderer(convexRend)
		rc.session.SetStencilRenderer(stencilRend)
	}

	// Propagate per-frame anti-aliasing state to session.
	rc.session.antiAlias = rc.antiAlias

	// Transfer per-context frame tracking to session before rendering.
	rc.session.SetFrameState(rc.frameRendered, rc.lastView)

	// Extract baseLayer from pendingDraws before building ScissorGroups.
	// BaseLayer is passed as a separate parameter to RenderFrameGrouped
	// (renders BEFORE all tiers) — not part of ScissorGroups.
	var baseLayer *GPUTextureDrawCommand
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdBaseLayer {
			bl := rc.pendingDraws[i].gpuTexCmd.(GPUTextureDrawCommand)
			baseLayer = &bl
			break
		}
	}

	// Build ScissorGroups from per-draw clip state. All command types flow
	// through pendingDraws: shapes, paths, text, images, GPU textures.
	ownedGroups := rc.buildScissorGroupsFromDraws()

	// Clear pending state (P2 GC retention prevention).
	rc.clearPendingDraws()
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

// buildScissorGroupsFromDraws converts backend-agnostic drawCommands into
// ScissorGroups using per-draw clip state (ADR-051 Phase 1.1 + Phase 2).
//
// Consecutive draws with the same clip state are grouped together. Each group
// is a ScissorGroup with the clip rect/rrect/path from the draws. This replaces
// the legacy recordScissorSegment timeline for all draw types.
//
// All command types flow through pendingDraws: shapes, paths, text batches
// (MSDF + glyph mask), images, and GPU textures. BaseLayer commands are
// skipped here and extracted separately at flush time.
func (rc *GPURenderContext) buildScissorGroupsFromDraws() []ScissorGroup {
	if len(rc.pendingDraws) == 0 {
		return nil
	}

	var groups []ScissorGroup
	groupStart := 0

	for i := 1; i <= len(rc.pendingDraws); i++ {
		// Detect clip boundary: either end-of-slice or clip changed.
		clipChanged := i < len(rc.pendingDraws) &&
			!drawClipEqual(&rc.pendingDraws[i], &rc.pendingDraws[groupStart])
		atEnd := i == len(rc.pendingDraws)

		if clipChanged || atEnd {
			end := i
			if clipChanged {
				end = i // commands [groupStart, i) share the same clip
			}

			// Build one ScissorGroup from draws [groupStart, end).
			g := rc.drawsToScissorGroup(rc.pendingDraws[groupStart:end])
			groups = append(groups, g)

			groupStart = i
		}
	}

	return groups
}

// drawsToScissorGroup converts a slice of same-clip drawCommands into a single
// ScissorGroup populated with GPU-specific command data. The clip state is taken
// from the first draw (all draws in the slice share the same clip).
func (rc *GPURenderContext) drawsToScissorGroup(draws []drawCommand) ScissorGroup {
	g := ScissorGroup{
		Rect:      copyClipRect(draws[0].clipRect),
		ClipRRect: copyClipRRect(draws[0].clipRRect),
		ClipPath:  draws[0].clipPath,
	}

	for i := range draws {
		cmd := &draws[i]
		switch cmd.kind {
		case drawCmdFillShape:
			rs, ok := DetectedShapeToRenderShape(cmd.shape, &cmd.paint, false)
			if !ok || rs.ColorA == 0 {
				continue
			}
			g.SDFShapes = append(g.SDFShapes, rs)

		case drawCmdStrokeShape:
			rs, ok := DetectedShapeToRenderShape(cmd.shape, &cmd.paint, true)
			if !ok || rs.ColorA == 0 {
				continue
			}
			g.SDFShapes = append(g.SDFShapes, rs)

		case drawCmdFillPath, drawCmdStrokePath:
			// Use pre-tessellated data from draw time (ADR-051 fix).
			// No re-tessellation at flush — convexPoints or stencilCmd was
			// computed once in FillPath()/StrokePath().
			if cmd.convexPoints != nil {
				color := premulColorFromPaint(&cmd.paint)
				g.ConvexCommands = append(g.ConvexCommands, ConvexDrawCommand{
					Points: cmd.convexPoints,
					Color:  color,
				})
			} else if cmd.stencilCmd != nil {
				g.StencilPaths = append(g.StencilPaths, *cmd.stencilCmd)
			}

		case drawCmdText:
			g.TextBatches = append(g.TextBatches, cmd.textBatch.(TextBatch))

		case drawCmdGlyphMaskText:
			g.GlyphMaskBatches = append(g.GlyphMaskBatches, cmd.textBatch.(GlyphMaskBatch))

		case drawCmdImage:
			g.ImageCommands = append(g.ImageCommands, cmd.imageCmd.(ImageDrawCommand))

		case drawCmdGPUTexture:
			g.GPUTextureCommands = append(g.GPUTextureCommands, cmd.gpuTexCmd.(GPUTextureDrawCommand))

		case drawCmdBaseLayer:
			// BaseLayer is extracted separately at flush time — skip here.
		}
	}

	return g
}

// preTessellateFill tessellates a fill path command at draw time.
// Stores the result in cmd.convexPoints (Tier 2a) or cmd.stencilCmd (Tier 2b).
// This is called once at queue time so flush never re-tessellates.
func (rc *GPURenderContext) preTessellateFill(cmd *drawCommand) {
	if cmd.path == nil || cmd.path.NumVerbs() == 0 {
		return
	}

	color := premulColorFromPaint(&cmd.paint)

	// Convex fast-path (NonZero fill rule only).
	if cmd.paint.FillRule != gg.FillRuleEvenOdd {
		if points, ok := extractConvexPolygon(cmd.path); ok {
			cmd.convexPoints = points
			// Store premul color in paint for later retrieval at flush time.
			return
		}
	}

	// Stencil-then-cover.
	tess := NewFanTessellator()
	tess.TessellatePath(cmd.path)
	fanVerts := tess.Vertices()
	if len(fanVerts) == 0 {
		return
	}

	sc := StencilPathCommand{
		Vertices:  make([]float32, len(fanVerts)),
		CoverQuad: tess.CoverQuad(),
		Color:     color,
		FillRule:  cmd.paint.FillRule,
	}
	copy(sc.Vertices, fanVerts)
	cmd.stencilCmd = &sc
}

// preTessellateStroke expands stroke geometry and tessellates the result at draw
// time. The expanded path replaces cmd.path (so CPU dispatch can use it via
// SoftwareRenderer.Fill with EvenOdd). GPU data is stored in convexPoints or
// stencilCmd just like fills.
func (rc *GPURenderContext) preTessellateStroke(cmd *drawCommand) {
	if cmd.path == nil || cmd.path.NumVerbs() == 0 {
		return
	}

	// Expand stroke to fill geometry.
	strokeVerbs := convertPathVerbsToStroke(cmd.path.Verbs())
	style := stroke.Stroke{
		Width:      cmd.paint.EffectiveLineWidth(),
		Cap:        stroke.LineCap(cmd.paint.EffectiveLineCap()),
		Join:       stroke.LineJoin(cmd.paint.EffectiveLineJoin()),
		MiterLimit: cmd.paint.EffectiveMiterLimit(),
	}
	expander := stroke.NewStrokeExpander(style)
	outVerbs, outCoords := expander.Expand(strokeVerbs, cmd.path.Coords())
	if len(outVerbs) == 0 {
		return
	}

	// Expand stroke verbs/coords into a scratch path, then clone into
	// cmd.path. The scratch path is reused across strokes within a frame
	// (P3 optimization — avoids 1 KB alloc per strokeResultToPath).
	if rc.scratchStrokePath == nil {
		rc.scratchStrokePath = gg.NewPath()
	}
	strokeResultToPath(rc.scratchStrokePath, outVerbs, outCoords)
	cmd.path = rc.scratchStrokePath.Clone()
	cmd.paint.FillRule = gg.FillRuleEvenOdd // stroke topology

	// Tessellate the expanded fill path for GPU.
	rc.preTessellateFill(cmd)
}

// premulColorFromPaint extracts a premultiplied RGBA color from a Paint.
func premulColorFromPaint(paint *gg.Paint) [4]float32 {
	color := getColorFromPaint(paint)
	return [4]float32{
		float32(color.R * color.A),
		float32(color.G * color.A),
		float32(color.B * color.A),
		float32(color.A),
	}
}

// dispatchRasterAtlasDraws performs mixed CPU/GPU dispatch on the rasterAtlas
// strategy (software adapter). Shape commands (fill/stroke shape/path) are
// CPU-dispatched via SoftwareRenderer; non-shape commands (images, GPU textures,
// base layers) are retained in pendingDraws for the standard GPU flush path.
//
// The device is alive on rasterAtlas (deviceReady=true), so image/texture
// draws can proceed through GPU render passes even though shape pipelines
// are unavailable (gpuReady=false). Without this split, clearPendingDraws
// would drop texture commands and make UI boundary content invisible.
func (rc *GPURenderContext) dispatchRasterAtlasDraws(target gg.GPURenderTarget) error {
	if !target.View.IsNil() {
		if err := rc.flushCPUToView(target); err != nil {
			rc.clearPendingDraws()
			return err
		}
	} else {
		rc.flushCPUToPixmap(target)
	}

	// Retain non-shape commands for GPU flush (images, textures, base layer).
	// dispatchDrawsToSoftware already skipped them (no matching switch case).
	n := 0
	for i := range rc.pendingDraws {
		switch rc.pendingDraws[i].kind {
		case drawCmdFillShape, drawCmdStrokeShape, drawCmdFillPath, drawCmdStrokePath:
			// Already CPU-dispatched — drop.
		default:
			rc.pendingDraws[n] = rc.pendingDraws[i]
			n++
		}
	}
	// Zero removed tail slots to release GC references (P2 pattern).
	for i := n; i < len(rc.pendingDraws); i++ {
		rc.pendingDraws[i] = drawCommand{}
	}
	rc.pendingDraws = rc.pendingDraws[:n]
	return nil
}

// dispatchDrawsToSoftware renders pendingDraws into a pixmap using
// SoftwareRenderer. Shared dispatch loop for both surface (flushCPUToPixmap)
// and offscreen (flushCPUToView) paths. Skia Graphite pattern: one dispatch,
// target provided by caller.
func (rc *GPURenderContext) dispatchDrawsToSoftware(pm *gg.Pixmap, sr *gg.SoftwareRenderer) {
	w, h := pm.Width(), pm.Height()
	sdfTarget := gg.GPURenderTarget{Data: pm.Data(), Width: w, Height: h, Stride: w * 4}

	for i := range rc.pendingDraws {
		cmd := &rc.pendingDraws[i]

		// Apply per-draw clip from value copies (ADR-052 three-tier clip).
		rc.applyDrawClipToSoftware(cmd, sr)

		hasClip := len(cmd.paint.ClipMask) > 0

		switch cmd.kind {
		case drawCmdFillShape:
			rc.dispatchFillShape(hasClip, sdfTarget, cmd, pm, sr)

		case drawCmdStrokeShape:
			rc.dispatchStrokeShape(hasClip, sdfTarget, cmd, pm, sr)

		case drawCmdFillPath:
			if err := sr.Fill(pm, cmd.path, &cmd.paint); err != nil {
				slogger().Debug("dispatchDrawsToSoftware: Fill failed", "err", err)
			}

		case drawCmdStrokePath:
			// preTessellateStroke already expanded stroke → fill path with EvenOdd.
			// Use Fill (not Stroke) to avoid double expansion.
			//
			// Force RasterizerAnalytic: stroke expansion produces multi-contour
			// fill paths (inner + outer outline) that require per-scanline winding
			// tracking. SparseStripsFiller cannot handle this correctly and
			// produces visible artifacts inside the stroke ring. This matches
			// SoftwareRenderer.Stroke() which forces Analytic for the same reason.
			sr.SetRasterizerMode(gg.RasterizerAnalytic)
			if err := sr.Fill(pm, cmd.path, &cmd.paint); err != nil {
				slogger().Debug("dispatchDrawsToSoftware: StrokePath Fill failed", "err", err)
			}
			sr.SetRasterizerMode(gg.RasterizerAuto)
		}
	}
}

// applyDrawClipToSoftware sets clip state on SoftwareRenderer and paint from
// per-draw value copies (ADR-052). Layer A: rect bounds for scanline skip.
// Layer B: pre-rasterized mask for RRect/path clips.
func (rc *GPURenderContext) applyDrawClipToSoftware(cmd *drawCommand, sr *gg.SoftwareRenderer) {
	// Layer A: rect bounds → scanline/tile skip (zero cost).
	if cmd.clipRect != nil {
		r := cmd.clipRect
		sr.SetClipBounds(int(r[0]), int(r[1]), int(r[0]+r[2]), int(r[1]+r[3]))
	} else {
		sr.ClearClipBounds()
	}

	// Layer B: paint.ClipMask is already set by Context.applyClipToPaint()
	// at queue time — pre-rasterized []uint8 snapshot, safe for deferred use.
	// ClipCoverage closure is stale (captures mutable clipStack), clear it.
	cmd.paint.ClipCoverage = nil //nolint:staticcheck // intentional clear of deprecated stale closure
}

// dispatchFillShape handles a single FillShape draw command in CPU dispatch.
// Clipped shapes go through SoftwareRenderer (supports ClipMask); unclipped
// shapes use SDFAccelerator (faster SDF per-pixel coverage).
func (rc *GPURenderContext) dispatchFillShape(hasClip bool, sdfTarget gg.GPURenderTarget, cmd *drawCommand, pm *gg.Pixmap, sr *gg.SoftwareRenderer) {
	if hasClip {
		if shapePath := shapeToPath(cmd.shape); shapePath != nil {
			if err := sr.Fill(pm, shapePath, &cmd.paint); err != nil {
				slogger().Debug("dispatchDrawsToSoftware: clipped shape Fill failed", "err", err, "shape", cmd.shape.Kind)
			}
		}
		return
	}
	if err := rc.shared.cpuFallback.FillShape(sdfTarget, cmd.shape, &cmd.paint); err != nil {
		if shapePath := shapeToPath(cmd.shape); shapePath != nil {
			if err := sr.Fill(pm, shapePath, &cmd.paint); err != nil {
				slogger().Debug("dispatchDrawsToSoftware: shape→path Fill failed", "err", err, "shape", cmd.shape.Kind)
			}
		}
	}
}

// dispatchStrokeShape handles a single StrokeShape draw command in CPU dispatch.
func (rc *GPURenderContext) dispatchStrokeShape(hasClip bool, sdfTarget gg.GPURenderTarget, cmd *drawCommand, pm *gg.Pixmap, sr *gg.SoftwareRenderer) {
	if hasClip {
		if shapePath := shapeToPath(cmd.shape); shapePath != nil {
			if err := sr.Stroke(pm, shapePath, &cmd.paint); err != nil {
				slogger().Debug("dispatchDrawsToSoftware: clipped shape Stroke failed", "err", err, "shape", cmd.shape.Kind)
			}
		}
		return
	}
	if err := rc.shared.cpuFallback.StrokeShape(sdfTarget, cmd.shape, &cmd.paint); err != nil {
		if shapePath := shapeToPath(cmd.shape); shapePath != nil {
			if err := sr.Stroke(pm, shapePath, &cmd.paint); err != nil {
				slogger().Debug("dispatchDrawsToSoftware: shape→path Stroke failed", "err", err, "shape", cmd.shape.Kind)
			}
		}
	}
}

// flushCPUToPixmap renders pending draw commands into the shared window pixmap.
func (rc *GPURenderContext) flushCPUToPixmap(target gg.GPURenderTarget) {
	if len(target.Data) == 0 {
		return
	}
	w, h := target.Width, target.Height
	pm := gg.NewPixmapFromBuffer(target.Data, w, h)
	sr := rc.getSoftwareRenderer(w, h) // P1: cached, 0 allocs steady-state
	rc.dispatchDrawsToSoftware(pm, sr)
	rc.sceneStats = gg.SceneStats{}
}

// flushCPUToView renders pending draw commands into a temporary pixmap,
// then uploads to the offscreen GPU texture via WriteTexture.
func (rc *GPURenderContext) flushCPUToView(target gg.GPURenderTarget) error {
	w, h := int(target.ViewWidth), int(target.ViewHeight)
	if w <= 0 || h <= 0 {
		return nil
	}

	queue := rc.shared.Queue()
	if queue == nil {
		return fmt.Errorf("flushCPUToView: GPU queue not available")
	}

	wgpuView := (*wgpu.TextureView)(target.View.Pointer())
	if wgpuView == nil {
		return fmt.Errorf("flushCPUToView: nil texture view")
	}
	tex := wgpuView.Texture()
	if tex == nil {
		return fmt.Errorf("flushCPUToView: texture view has no backing texture")
	}

	tmpPixmap := rc.getPixmap(w, h)    // P0: cached, 0 allocs steady-state
	sr := rc.getSoftwareRenderer(w, h) // P1: cached, 0 allocs steady-state
	rc.dispatchDrawsToSoftware(tmpPixmap, sr)

	pixels := tmpPixmap.Data()
	bgra := rc.ensureBGRABuffer(len(pixels)) // P0: pooled, 0 allocs steady-state
	for i := 0; i < len(pixels); i += 4 {
		bgra[i+0] = pixels[i+2]
		bgra[i+1] = pixels[i+1]
		bgra[i+2] = pixels[i+0]
		bgra[i+3] = pixels[i+3]
	}

	return queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0},
		bgra,
		&wgpu.ImageDataLayout{BytesPerRow: uint32(w) * 4, RowsPerImage: uint32(h)}, //nolint:gosec // bounded by pixmap
		&wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}, //nolint:gosec // bounded by pixmap
	)
}

// shapeToPath converts a DetectedShape to a Path for CPU rendering fallback.
// Returns nil for unknown shape kinds.
func shapeToPath(shape gg.DetectedShape) *gg.Path {
	p := gg.NewPath()
	switch shape.Kind {
	case gg.ShapeCircle, gg.ShapeEllipse:
		p.Ellipse(shape.CenterX, shape.CenterY, shape.RadiusX, shape.RadiusY)
		p.Close()
	case gg.ShapeRect:
		x := shape.CenterX - shape.Width/2
		y := shape.CenterY - shape.Height/2
		p.Rectangle(x, y, shape.Width, shape.Height)
		p.Close()
	case gg.ShapeRRect:
		x := shape.CenterX - shape.Width/2
		y := shape.CenterY - shape.Height/2
		p.RoundedRectangle(x, y, shape.Width, shape.Height, shape.CornerRadius)
		p.Close()
	default:
		return nil
	}
	return p
}

// flushVello flushes Vello compute if it has pending paths and the effective
// pipeline mode is Compute. In Auto mode with few shapes, SelectPipeline
// returns RenderPass — Vello paths would be lost. Guard ensures Vello is
// flushed only when the mode actually routes paths there.
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

// uploadPixmapToView uploads CPU-rasterized pixmap content to an offscreen
// GPU texture on rasterAtlas strategy. Skia Graphite pattern: shapes are
// CPU-rasterized, then uploaded via WriteTexture (no render pass needed).
func (rc *GPURenderContext) uploadPixmapToView(target gg.GPURenderTarget) error {
	rc.sceneStats = gg.SceneStats{}

	queue := rc.shared.Queue()
	if queue == nil || len(target.Data) == 0 {
		return nil
	}

	wgpuView := (*wgpu.TextureView)(target.View.Pointer())
	if wgpuView == nil {
		return nil
	}
	tex := wgpuView.Texture()
	if tex == nil {
		return nil
	}

	w, h := uint32(target.Width), uint32(target.Height) //nolint:gosec // bounded by pixmap

	// Pixmap is RGBA, offscreen texture is BGRA8Unorm — swizzle R↔B.
	bgra := rc.ensureBGRABuffer(len(target.Data)) // P0: pooled, 0 allocs steady-state
	for i := 0; i < len(target.Data); i += 4 {
		bgra[i+0] = target.Data[i+2]
		bgra[i+1] = target.Data[i+1]
		bgra[i+2] = target.Data[i+0]
		bgra[i+3] = target.Data[i+3]
	}

	return queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0},
		bgra,
		&wgpu.ImageDataLayout{BytesPerRow: w * 4, RowsPerImage: h},
		&wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	)
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
// Checks deviceReady (not gpuReady): texture allocation needs a live device,
// not shape pipelines. On rasterAtlas, gpuReady is false but device is alive.
// Skia Graphite: TextureProxy::Make() works under kRasterAtlas.
func (rc *GPURenderContext) CreateOffscreenTexture(w, h int) (gpucontext.TextureView, func()) {
	if rc.shared == nil {
		slogger().Warn("CreateOffscreenTexture: shared is nil")
		return gpucontext.TextureView{}, nil
	}
	if !rc.shared.deviceReady {
		rc.shared.mu.Lock()
		err := rc.shared.ensureGPU()
		rc.shared.mu.Unlock()
		if err != nil {
			slogger().Warn("CreateOffscreenTexture: ensureGPU failed", "error", err)
			return gpucontext.TextureView{}, nil
		}
		if !rc.shared.deviceReady {
			slogger().Warn("CreateOffscreenTexture: device not ready after ensureGPU")
			return gpucontext.TextureView{}, nil
		}
	}
	device := rc.shared.Device()
	if device == nil {
		slogger().Warn("CreateOffscreenTexture: device is nil")
		return gpucontext.TextureView{}, nil
	}

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "offscreen_cache",
		Size:          wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}, //nolint:gosec // bounded
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		slogger().Warn("CreateOffscreenTexture: CreateTexture failed",
			"error", err, "width", w, "height", h,
			"format", "BGRA8Unorm", "usage", "RenderAttachment|CopySrc|TextureBinding")
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
		slogger().Warn("CreateOffscreenTexture: CreateTextureView failed",
			"error", err, "width", w, "height", h)
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
	rc.pendingDraws = nil
	rc.hasPendingTarget = false
	rc.clipRect = nil
	rc.clipRRect = nil
	rc.clipPath = nil
	rc.sceneStats = gg.SceneStats{}

	// Release cached resources (P0/P1/P3 optimization pools).
	rc.bgraBuffer = nil
	rc.cachedPixmap = nil
	rc.cachedSR = nil
	rc.scratchStrokePath = nil
}

// getSoftwareRenderer returns a cached SoftwareRenderer for the given dimensions.
// On first call or dimension change, a new renderer is created; otherwise the
// existing one is reused (0 allocs steady-state). This eliminates 13 allocs +
// 12-17 KB per flush (P1 optimization, profiling report M-3).
func (rc *GPURenderContext) getSoftwareRenderer(w, h int) *gg.SoftwareRenderer {
	if rc.cachedSR != nil && rc.cachedSRWidth == w && rc.cachedSRHeight == h {
		return rc.cachedSR
	}
	if rc.cachedSR == nil {
		rc.cachedSR = gg.NewSoftwareRenderer(w, h)
	} else {
		rc.cachedSR.Resize(w, h)
	}
	rc.cachedSRWidth = w
	rc.cachedSRHeight = h
	return rc.cachedSR
}

// getPixmap returns a cached Pixmap for the given dimensions. On first call or
// dimension change, a new pixmap is created; otherwise the existing one is
// reused and zeroed. This eliminates 1.9 MB/frame allocation for offscreen
// targets (P0 optimization, profiling report P4 stretch).
func (rc *GPURenderContext) getPixmap(w, h int) *gg.Pixmap {
	if rc.cachedPixmap != nil && rc.cachedPixmapWidth == w && rc.cachedPixmapHeight == h {
		// Clear pixel data for the new frame (Pixmap is reused).
		data := rc.cachedPixmap.Data()
		clear(data)
		return rc.cachedPixmap
	}
	rc.cachedPixmap = gg.NewPixmap(w, h)
	rc.cachedPixmapWidth = w
	rc.cachedPixmapHeight = h
	return rc.cachedPixmap
}

// ensureBGRABuffer returns a BGRA swizzle buffer with at least 'needed' bytes.
// Grow-only: the buffer is reused across frames and only reallocated when the
// frame size increases. This eliminates 8 MB/frame allocation at 1080p
// (P0 optimization, profiling report M-2).
func (rc *GPURenderContext) ensureBGRABuffer(needed int) []byte {
	if cap(rc.bgraBuffer) >= needed {
		return rc.bgraBuffer[:needed]
	}
	rc.bgraBuffer = make([]byte, needed)
	return rc.bgraBuffer
}

// clearPendingDraws zeroes pointer fields in pendingDraws before truncating
// to prevent GC reference retention (P2 optimization, Skia OpChain::reset
// pattern). Without this, stale *Path, *StencilPathCommand, and []Point
// references remain in the backing array, retaining ~125 KB per 50 draws.
func (rc *GPURenderContext) clearPendingDraws() {
	for i := range rc.pendingDraws {
		rc.pendingDraws[i] = drawCommand{}
	}
	rc.pendingDraws = rc.pendingDraws[:0]
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
