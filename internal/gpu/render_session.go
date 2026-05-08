//go:build !nogpu

package gpu

import (
	"context"
	"fmt"
	"image"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// ScissorGroup holds a subset of draw commands that share the same scissor
// rect. During rendering, the scissor rect is applied before recording the
// group's draws, allowing multiple scissor states within a single render pass.
// This eliminates the need for multiple render pass submissions when clipping
// is used (e.g., scroll views, panels), reducing GPU utilization from ~45%
// to ~3% on Intel iGPUs.
type ScissorGroup struct {
	// Rect is the scissor rect in device pixels. nil means full framebuffer.
	Rect *[4]uint32

	// ClipRRect is the analytic RRect clip parameters for this group.
	// nil means no RRect clip (full rendering within scissor rect).
	ClipRRect *ClipParams

	// ClipPath is an arbitrary path for depth-based clipping (GPU-CLIP-003a).
	// When set, the path is fan-tessellated and rendered to the depth buffer
	// before any content draws. Content pipelines then test against the clip
	// depth so fragments only pass within the clip region.
	// nil means no depth clip (default). Independent of ClipRRect.
	ClipPath *gg.Path

	// ClipDepthLevel is the 1-based nesting level for depth clipping.
	// Level 0 means no depth clip. Level 1..255 maps to depth values
	// via clipDepthValue(). Nested clips use increasing levels.
	ClipDepthLevel uint32

	// Per-tier draw command subsets for this scissor state.
	SDFShapes          []SDFRenderShape
	ConvexCommands     []ConvexDrawCommand
	StencilPaths       []StencilPathCommand
	ImageCommands      []ImageDrawCommand
	GPUTextureCommands []GPUTextureDrawCommand
	TextBatches        []TextBatch
	GlyphMaskBatches   []GlyphMaskBatch
}

// StencilPathCommand holds a path and paint for stencil-then-cover rendering
// within a unified render session. The vertices are pre-tessellated fan
// triangles from FanTessellator.
type StencilPathCommand struct {
	// Vertices holds fan-tessellated triangle vertices as x,y float32 pairs.
	// Every 6 consecutive floats form one triangle (3 vertices x 2 coords).
	Vertices []float32

	// CoverQuad holds 6 vertices (12 floats) forming the path's bounding
	// rectangle for the cover pass.
	CoverQuad [12]float32

	// Color is the premultiplied RGBA fill color.
	Color [4]float32

	// FillRule determines inside/outside testing (NonZero or EvenOdd).
	FillRule gg.FillRule
}

// RenderMode controls how the GPURenderSession outputs rendering results.
type RenderMode int

const (
	// RenderModeOffscreen renders to offscreen textures and reads back to CPU.
	// Use this for standalone gg without a window. The session creates its own
	// resolve texture and copies pixels to a staging buffer for CPU readback.
	RenderModeOffscreen RenderMode = iota

	// RenderModeSurface renders directly to a provided surface texture view.
	// Use this when gg runs inside gogpu via ggcanvas. No readback occurs --
	// the MSAA color attachment resolves directly to the surface view.
	// This eliminates the GPU->CPU->GPU round-trip for windowed rendering.
	RenderModeSurface
)

// GPURenderSession manages a single frame's GPU rendering across all tiers.
// It owns shared MSAA color, stencil, and resolve textures, and executes
// all draw commands in a single render pass with pipeline switching.
//
// This is the central abstraction that unifies SDF shape rendering (Tier 1),
// convex polygon fast-path rendering (Tier 2a), stencil-then-cover path
// rendering (Tier 2b), and MSDF text rendering (Tier 4) into one GPU
// submission. Enterprise 2D engines (Skia Ganesh/Graphite, Flutter Impeller,
// Gio) use the same pattern: one render pass, multiple pipeline switches.
//
// The session supports two render modes:
//   - Offscreen (default): renders to an internal resolve texture, then reads
//     back pixels to CPU via staging buffer. Used for standalone gg.
//   - Surface: renders directly to a caller-provided surface texture view.
//     No readback occurs. Used when gg runs inside gogpu (ggcanvas).
//
// Architecture:
//
//	GPURenderSession
//	  +-- Manages shared MSAA + stencil + resolve textures (via textureSet)
//	  +-- Holds references to SDFRenderPipeline, ConvexRenderer, StencilRenderer, MSDFTextPipeline
//	  +-- Encodes single render pass with pipeline switching
//	  +-- Single submit + fence wait
//	  +-- Single readback (offscreen) or resolve to surface (direct)
type GPURenderSession struct {
	device *wgpu.Device
	queue  *wgpu.Queue

	// Shared textures (MSAA 4x color + depth/stencil + 1x resolve).
	textures textureSet

	// Pipeline owners (lazily created). The session does not own these
	// pipelines -- it holds references and delegates draw recording to them.
	sdfPipeline     *SDFRenderPipeline
	convexRenderer  *ConvexRenderer
	stencilRenderer *StencilRenderer
	imagePipeline   *TexturedQuadPipeline
	imageCache      *ImageCache
	textPipeline    *MSDFTextPipeline

	// Surface rendering mode fields. When surfaceView is non-nil, the session
	// renders directly to the surface instead of reading back to CPU.
	surfaceView   *wgpu.TextureView
	surfaceWidth  uint32
	surfaceHeight uint32

	// Persistent per-frame GPU buffers (survive across frames).
	// Grow-only: reallocated only when data exceeds current capacity.
	sdfVertBuf       *wgpu.Buffer
	sdfVertBufCap    uint64
	sdfUniformBuf    *wgpu.Buffer
	sdfBindGroup     *wgpu.BindGroup
	convexVertBuf    *wgpu.Buffer
	convexVertBufCap uint64
	convexUniformBuf *wgpu.Buffer
	convexBindGroup  *wgpu.BindGroup

	// Tier 3: Image textured quad persistent buffers.
	imageVertBuf    *wgpu.Buffer
	imageVertBufCap uint64
	// Per-draw uniform buffers and bind groups (pool, grows as needed).
	imageUniformBufs []*wgpu.Buffer
	imageBindGroups  []*wgpu.BindGroup

	// Tier 4: MSDF text persistent buffers.
	textVertBuf    *wgpu.Buffer
	textVertBufCap uint64
	textIdxBuf     *wgpu.Buffer
	textIdxBufCap  uint64
	// Per-batch uniform buffers and bind groups (pool, grows as needed).
	textUniformBufs []*wgpu.Buffer
	textBindGroups  []*wgpu.BindGroup
	// Atlas texture and view for current frame's text rendering.
	textAtlasTex  *wgpu.Texture
	textAtlasView *wgpu.TextureView

	// Tier 6: Glyph mask text persistent buffers.
	glyphMaskPipeline    *GlyphMaskPipeline
	glyphMaskVertBuf     *wgpu.Buffer
	glyphMaskVertBufCap  uint64
	glyphMaskIdxBuf      *wgpu.Buffer
	glyphMaskIdxBufCap   uint64
	glyphMaskUniformBufs []*wgpu.Buffer
	glyphMaskBindGroups  []*wgpu.BindGroup

	// Tier 3b: GPU texture compositing persistent buffers.
	// Overlay and base layer use SEPARATE vertex buffers to prevent
	// base layer (full-screen quad) from overwriting overlay vertices.
	// Both use the same uniform/bind-group pools (indexed independently).
	gpuTexVertBuf        *wgpu.Buffer // overlay GPU textures
	gpuTexVertBufCap     uint64
	gpuTexBaseVertBuf    *wgpu.Buffer // base layer only (1 quad, never shares with overlays)
	gpuTexBaseVertBufCap uint64
	gpuTexUniformBufs    []*wgpu.Buffer
	gpuTexBindGroups     []*wgpu.BindGroup

	// Bind groups pending release — deferred until after command buffer submit.
	// WebGPU requires bind groups to be alive at submit time (wgpu-core track/mod.rs:631).
	// Skia Graphite pattern: batch-release after GPU completion.
	pendingBindGroupRelease []*wgpu.BindGroup

	// Depth clip pipeline (GPU-CLIP-003a): fan-tessellated clip path rendered
	// to depth buffer before content draws. Lazily created on first use.
	depthClipPipeline *DepthClipPipeline

	// Stencil buffers are per-path, so we keep a pool of reusable buffer sets.
	stencilBufPool []*stencilCoverBuffers

	// Pre-allocated CPU staging slices for vertex data generation.
	sdfVertexStaging    []byte
	convexVertexStaging []byte

	// In-flight command buffers from the previous frame. Freed at the
	// start of the next frame, when VSync guarantees the GPU is done.
	// A slice is used instead of a single pointer because multiple
	// FlushGPUWithView calls per frame (e.g., render to offscreen then
	// composite to swapchain) each produce a separate command buffer.
	// Freeing a command buffer while the GPU is still executing it causes
	// vkResetCommandPool on an in-flight pool — undefined behavior that
	// manifests as trail artifacts (stale MSAA resolve content).
	prevCmdBufs []*wgpu.CommandBuffer

	// frameRendered tracks whether at least one render pass has been
	// submitted to the surface in the current frame. When true, subsequent
	// render passes use LoadOpLoad instead of LoadOpClear to preserve
	// previously drawn content. This handles mid-frame flushes caused by
	// CPU fallback operations (e.g., DrawImage between GPU draws).
	//
	// Reset by BeginFrame() at the start of each frame, or when the
	// active view changes (different Context targets different view).
	// Only relevant in surface mode — offscreen mode composites via
	// Porter-Duff "over" during readback, so LoadOpClear is always safe.
	frameRendered bool

	// lastView tracks the most recent per-pass view used for rendering.
	// When the view changes between Flush calls (e.g., two gg.Context
	// instances rendering to different targets), frameRendered is reset
	// so the new view gets a LoadOpClear on its first render pass.
	lastView *wgpu.TextureView

	// scissorRect holds the current scissor rect in device pixels.
	// When non-nil, all draw commands are clipped to this rectangle.
	// nil means full framebuffer (default, no clipping).
	scissorRect *[4]uint32

	// RRect clip bind group infrastructure. All 5 pipelines share the same
	// clip bind group layout at @group(1) @binding(0). A no-clip bind group
	// (clip_enabled=0.0) is created once and reused for groups without RRect clip.
	clipBindLayout   *wgpu.BindGroupLayout
	noClipUniformBuf *wgpu.Buffer
	noClipBindGroup  *wgpu.BindGroup
	// Pool of per-group clip uniform buffers and bind groups.
	clipUniformPool []*wgpu.Buffer
	clipBindPool    []*wgpu.BindGroup
	clipPoolUsed    int // number of pool entries used in current frame
}

// NewGPURenderSession creates a new render session with the given device and
// queue. Textures and pipelines are not allocated until RenderFrame is called.
func NewGPURenderSession(device *wgpu.Device, queue *wgpu.Queue) *GPURenderSession {
	return &GPURenderSession{
		device: device,
		queue:  queue,
	}
}

// SetSurfaceTarget configures the session to render directly to the given
// texture view instead of creating an offscreen resolve texture. This
// eliminates the GPU->CPU readback for windowed rendering.
//
// When a surface target is set, RenderFrame ignores GPURenderTarget.Data
// and writes directly to the surface. The MSAA color attachment resolves
// to the surface view. The MSAA color texture and stencil texture are still
// created and managed by the session.
//
// Call with nil view to return to offscreen mode. The caller retains
// ownership of the surface view -- the session will not destroy it.
func (s *GPURenderSession) SetSurfaceTarget(view *wgpu.TextureView, width, height uint32) {
	// If switching modes or resizing, invalidate cached textures so they
	// are recreated on the next RenderFrame call.
	modeChanged := (view == nil) != (s.surfaceView == nil)
	sizeChanged := width != s.surfaceWidth || height != s.surfaceHeight
	if modeChanged || sizeChanged {
		// Drain the GPU before destroying textures — an in-flight command
		// buffer may still reference framebuffers built from these views.
		if len(s.prevCmdBufs) > 0 {
			s.drainQueue()
			for _, cb := range s.prevCmdBufs {
				if cb != nil {
					s.device.FreeCommandBuffer(cb)
				}
			}
			s.prevCmdBufs = s.prevCmdBufs[:0]
		}
		s.textures.destroyTextures()
	}

	// Detect new frame: swapchain creates a new TextureView each frame
	// (gogpu renderer.BeginFrame → CreateTextureView), so a different view
	// pointer means a new frame has started. Reset per-frame state so the
	// first render pass clears the surface while subsequent mid-frame
	// flushes preserve content via LoadOpLoad.
	//
	// When the same view is passed again (e.g., ggcanvas.RenderDirect calls
	// SetSurfaceTarget a second time within the same frame), this is a no-op
	// for frame state — preserving frameRendered so mid-frame content survives.
	if view != s.surfaceView {
		s.BeginFrame()
	}

	s.surfaceView = view
	s.surfaceWidth = width
	s.surfaceHeight = height
	if modeChanged || sizeChanged {
		slogger().Debug("GPURenderSession.SetSurfaceTarget changed",
			"surface", view != nil,
			"width", width, "height", height,
			"modeChanged", modeChanged, "sizeChanged", sizeChanged,
		)
	}
}

// BeginFrame resets per-frame state. Call this at the start of each frame
// before any drawing operations. In surface mode, this ensures the first
// render pass clears the surface while subsequent mid-frame flushes
// preserve previously drawn content (LoadOpLoad instead of LoadOpClear).
//
// For offscreen mode this is a no-op — offscreen readback composites via
// Porter-Duff "over", so LoadOpClear is always safe there.
func (s *GPURenderSession) BeginFrame() {
	// Free all command buffers from the previous frame. By now, VSync (or
	// the equivalent present barrier) guarantees the GPU is done with them.
	// This MUST happen at frame boundaries — not mid-frame — because
	// multiple FlushGPUWithView calls within a single frame produce
	// separate command buffers that may still be in-flight when the next
	// flush begins. Freeing them mid-frame would vkResetCommandPool on an
	// in-flight pool, causing undefined behavior (trail artifacts from
	// incomplete MSAA resolve).
	for _, cb := range s.prevCmdBufs {
		if cb != nil {
			s.device.FreeCommandBuffer(cb)
		}
	}
	s.prevCmdBufs = s.prevCmdBufs[:0]

	s.frameRendered = false
	s.lastView = nil
}

// SetFrameState sets the per-context frame tracking state before a render pass.
// GPURenderContext transfers its frameRendered/lastView into the session so the
// session can compute the correct LoadOp (Clear vs Load) for this context.
func (s *GPURenderSession) SetFrameState(frameRendered bool, lastView *wgpu.TextureView) {
	s.frameRendered = frameRendered
	s.lastView = lastView
}

// FrameState returns the current frame tracking state after a render pass.
// GPURenderContext reads this back to maintain per-context LoadOp tracking.
func (s *GPURenderSession) FrameState() (frameRendered bool, lastView *wgpu.TextureView) {
	return s.frameRendered, s.lastView
}

// resolveActiveView returns the per-pass texture view to use for surface
// rendering. The per-pass target.View takes priority over the session-level
// surfaceView (which is retained for backward compatibility with callers
// that still use SetSurfaceTarget).
//
// Returns nil when rendering should use the CPU readback path.
func (s *GPURenderSession) resolveActiveView(target gg.GPURenderTarget) *wgpu.TextureView {
	if !target.View.IsNil() {
		if v := extractTextureView(target.View); v != nil {
			return v
		}
	}
	// No View → readback path. The caller is an offscreen context or
	// a context that wants CPU pixel access. Per WebGPU spec, the render
	// target is determined by what the caller passes, not by session state.
	return nil
}

// extractTextureView converts a gpucontext.TextureView opaque handle to
// the concrete *wgpu.TextureView via unsafe.Pointer (Go spec Rule 1).
func extractTextureView(view gpucontext.TextureView) *wgpu.TextureView {
	if view.IsNil() {
		return nil
	}
	return (*wgpu.TextureView)(view.Pointer())
}

// effectiveDimensions returns the width and height to use for MSAA textures
// and viewport. When rendering to a view, uses the view dimensions; otherwise
// uses the CPU readback target dimensions.
func (s *GPURenderSession) effectiveDimensions(target gg.GPURenderTarget, activeView *wgpu.TextureView) (uint32, uint32) {
	if activeView != nil {
		// Per-pass view dimensions from target take priority.
		if !target.View.IsNil() && target.ViewWidth > 0 && target.ViewHeight > 0 {
			return target.ViewWidth, target.ViewHeight
		}
		// Fall back to session-level surface dimensions (backward compat).
		if s.surfaceWidth > 0 && s.surfaceHeight > 0 {
			return s.surfaceWidth, s.surfaceHeight
		}
	}
	return uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions always fit uint32
}

// ensureTexturesForView creates or ensures MSAA/stencil textures sized for
// the active render target. In surface/view mode only MSAA and stencil are
// created (the view itself is the resolve target). In offscreen mode a
// resolve texture is also created for CPU readback.
//
// When the dimensions change (e.g., switching from a 100x100 offscreen target
// to a 700x500 swapchain within the same frame), old textures must be
// destroyed. If there are in-flight command buffers from earlier flushes in
// the same frame, we must drain the GPU first — otherwise destroying MSAA
// textures referenced by in-flight render passes is undefined behavior.
func (s *GPURenderSession) ensureTexturesForView(activeView *wgpu.TextureView, w, h uint32) error {
	// If dimensions are changing and there are in-flight command buffers
	// from earlier flushes in this frame, drain the GPU before destroying
	// the old textures. Without this, the earlier flush's render pass
	// would reference destroyed MSAA textures.
	if s.textures.msaaTex != nil && (s.textures.width != w || s.textures.height != h) {
		if len(s.prevCmdBufs) > 0 {
			s.drainQueue()
			for _, cb := range s.prevCmdBufs {
				if cb != nil {
					s.device.FreeCommandBuffer(cb)
				}
			}
			s.prevCmdBufs = s.prevCmdBufs[:0]
		}
	}

	if activeView != nil {
		return s.textures.ensureSurfaceTextures(s.device, w, h, "session")
	}
	return s.textures.ensureTextures(s.device, w, h, "session")
}

// SetScissorRect sets the scissor rect for subsequent GPU draw commands.
// Coordinates are in device pixels. The scissor rect clips all rendering
// to the rectangle (x, y, w, h).
func (s *GPURenderSession) SetScissorRect(x, y, w, h uint32) {
	s.scissorRect = &[4]uint32{x, y, w, h}
}

// ClearScissorRect removes the scissor rect, restoring full-framebuffer
// rendering for subsequent draw commands.
func (s *GPURenderSession) ClearScissorRect() {
	s.scissorRect = nil
}

// applyScissorRect applies the current scissor rect (if any) to the given
// render pass encoder. Call this after BeginRenderPass and before draw calls.
func (s *GPURenderSession) applyScissorRect(rp *wgpu.RenderPassEncoder) {
	if s.scissorRect != nil {
		rp.SetScissorRect(s.scissorRect[0], s.scissorRect[1], s.scissorRect[2], s.scissorRect[3])
	}
}

// RenderMode returns the current render mode based on whether a surface
// target has been set.
func (s *GPURenderSession) RenderMode() RenderMode {
	if s.surfaceView != nil {
		return RenderModeSurface
	}
	return RenderModeOffscreen
}

// EnsureTextures creates or recreates the shared MSAA color, depth/stencil,
// and resolve textures if the requested dimensions differ from the current
// size. If dimensions match and textures exist, this is a no-op.
//
// In surface mode, only MSAA and stencil textures are created -- the resolve
// texture is skipped because the surface view serves as the resolve target.
func (s *GPURenderSession) EnsureTextures(w, h uint32) error {
	if s.surfaceView != nil {
		return s.textures.ensureSurfaceTextures(s.device, w, h, "session")
	}
	return s.textures.ensureTextures(s.device, w, h, "session")
}

// RenderFrame renders all draw commands (SDF shapes + convex polygons +
// stencil paths + MSDF text + glyph mask text) in a single render pass.
// This is the main entry point for unified rendering.
//
// The render pass uses the shared textures with:
//   - MSAA color cleared to transparent black
//   - Stencil cleared to 0
//   - MSAA resolve to single-sample target
//   - Copy resolve to staging buffer for CPU readback (offscreen mode)
//
// Returns nil if all command slices are empty. Pipelines are lazily
// created on first use.
func (s *GPURenderSession) RenderFrame(
	target gg.GPURenderTarget,
	sdfShapes []SDFRenderShape,
	convexCommands []ConvexDrawCommand,
	stencilPaths []StencilPathCommand,
	textBatches []TextBatch,
	glyphMaskBatches ...GlyphMaskBatch,
) error {
	if len(sdfShapes) == 0 && len(convexCommands) == 0 && len(stencilPaths) == 0 && len(textBatches) == 0 && len(glyphMaskBatches) == 0 {
		return nil
	}

	// Determine render target view: per-pass target.View takes priority
	// over session-level surfaceView (backward compat).
	activeView := s.resolveActiveView(target)

	slogger().Debug("RenderFrame",
		"sdf", len(sdfShapes), "convex", len(convexCommands),
		"stencil", len(stencilPaths), "text", len(textBatches),
		"glyphMask", len(glyphMaskBatches),
		"surface", activeView != nil)

	w, h := s.effectiveDimensions(target, activeView)
	slogger().Debug("RenderFrame dimensions",
		"target_w", target.Width, "target_h", target.Height,
		"effective_w", w, "effective_h", h,
		"surface", activeView != nil,
	)
	if err := s.ensureTexturesForView(activeView, w, h); err != nil {
		return fmt.Errorf("ensure textures: %w", err)
	}

	// Clip bind layout must be created BEFORE pipelines, because pipeline
	// layout creation includes the clip layout at @group(1).
	if err := s.ensureClipBindLayout(); err != nil {
		return fmt.Errorf("ensure clip bind layout: %w", err)
	}
	if err := s.ensurePipelines(); err != nil {
		return fmt.Errorf("ensure pipelines: %w", err)
	}

	// Build per-frame GPU resources using persistent buffers.
	var sdfResources *sdfFrameResources
	if len(sdfShapes) > 0 {
		var err error
		sdfResources, err = s.buildSDFResources(sdfShapes, w, h)
		if err != nil {
			return fmt.Errorf("build SDF resources: %w", err)
		}
	}

	var convexRes *convexFrameResources
	if len(convexCommands) > 0 {
		var err error
		convexRes, err = s.buildConvexResources(convexCommands, w, h)
		if err != nil {
			return fmt.Errorf("build convex resources: %w", err)
		}
	}

	stencilResources, err := s.buildStencilResourcesBatch(stencilPaths, w, h)
	if err != nil {
		return err
	}

	textRes, err := s.prepareTextResources(textBatches)
	if err != nil {
		return err
	}

	glyphMaskRes, err := s.prepareGlyphMaskResources(glyphMaskBatches)
	if err != nil {
		return err
	}

	if activeView != nil {
		return s.encodeSubmitSurface(activeView, w, h, sdfResources, sdfShapes, convexRes, stencilResources, stencilPaths, textRes, glyphMaskRes)
	}
	return s.encodeSubmitReadback(w, h, sdfResources, sdfShapes, convexRes, stencilResources, stencilPaths, textRes, glyphMaskRes, target)
}

// groupResources holds pre-built GPU resources for a single ScissorGroup.
type groupResources struct {
	scissorRect   *[4]uint32
	clipBindGroup *wgpu.BindGroup // @group(1) bind group for RRect clip (or no-clip)
	depthClipRes  *DepthClipResources
	hasDepthClip  bool
	sdfRes        *sdfFrameResources
	sdfShapes     []SDFRenderShape
	convexRes     *convexFrameResources
	stencilRes    []*stencilCoverBuffers
	stencilPaths  []StencilPathCommand
	imageRes      *imageFrameResources
	gpuTexRes     *imageFrameResources // GPU-to-GPU texture compositing (same pipeline)
	textRes       *textFrameResources
	glyphMaskRes  *glyphMaskFrameResources
}

// RenderFrameGrouped renders multiple scissor groups in a single render pass.
// Each group's draw commands are rendered with the group's scissor rect applied,
// then the scissor is changed for the next group. This eliminates multiple
// render pass submissions when clipping is used.
//
// For frames with no scissor changes (single group with nil rect), this
// behaves identically to the original RenderFrame.
func (s *GPURenderSession) RenderFrameGrouped(target gg.GPURenderTarget, groups []ScissorGroup, baseLayer *GPUTextureDrawCommand, sharedEncoder *wgpu.CommandEncoder) error { //nolint:gocognit,gocyclo,cyclop,funlen,maintidx // sequential resource setup + group dispatch
	if len(groups) == 0 && baseLayer == nil {
		return nil
	}

	// Check if all groups are empty.
	totalItems := 0
	for i := range groups {
		totalItems += len(groups[i].SDFShapes) + len(groups[i].ConvexCommands) + len(groups[i].StencilPaths) +
			len(groups[i].ImageCommands) + len(groups[i].TextBatches) + len(groups[i].GlyphMaskBatches)
	}
	if totalItems == 0 && baseLayer == nil {
		return nil
	}

	// Determine render target view: per-pass target.View takes priority
	// over session-level surfaceView (backward compat).
	activeView := s.resolveActiveView(target)

	w, h := s.effectiveDimensions(target, activeView)
	if err := s.ensureTexturesForView(activeView, w, h); err != nil {
		return fmt.Errorf("ensure textures: %w", err)
	}
	// Clip bind layout must be created BEFORE pipelines, because pipeline
	// layout creation includes the clip layout at @group(1).
	if err := s.ensureClipBindLayout(); err != nil {
		return fmt.Errorf("ensure clip bind layout: %w", err)
	}
	if err := s.ensurePipelines(); err != nil {
		return fmt.Errorf("ensure pipelines: %w", err)
	}
	// Reset clip pool usage for this frame.
	s.clipPoolUsed = 0

	// Concatenate all items across groups into combined arrays, tracking
	// per-group offsets. Resources are built ONCE from combined data to avoid
	// overwriting shared GPU buffers (vertex, index, uniform) — all build*
	// methods write to session-level shared buffers, so calling them per-group
	// would overwrite previous groups' data.
	var allSDF []SDFRenderShape
	var allConvex []ConvexDrawCommand
	var allStencil []StencilPathCommand
	var allImage []ImageDrawCommand
	var allGPUTex []GPUTextureDrawCommand
	var allText []TextBatch
	var allGlyph []GlyphMaskBatch

	type groupOffset struct {
		sdfStart, sdfCount         int
		convexStart, convexCount   int
		stencilStart, stencilCount int
		imageStart, imageCount     int
		gpuTexStart, gpuTexCount   int
		textStart, textCount       int
		glyphStart, glyphCount     int
	}
	gOff := make([]groupOffset, len(groups))

	for i := range groups {
		g := &groups[i]
		gOff[i] = groupOffset{
			sdfStart: len(allSDF), sdfCount: len(g.SDFShapes),
			convexStart: len(allConvex), convexCount: len(g.ConvexCommands),
			stencilStart: len(allStencil), stencilCount: len(g.StencilPaths),
			imageStart: len(allImage), imageCount: len(g.ImageCommands),
			gpuTexStart: len(allGPUTex), gpuTexCount: len(g.GPUTextureCommands),
			textStart: len(allText), textCount: len(g.TextBatches),
			glyphStart: len(allGlyph), glyphCount: len(g.GlyphMaskBatches),
		}
		allSDF = append(allSDF, g.SDFShapes...)
		allConvex = append(allConvex, g.ConvexCommands...)
		allStencil = append(allStencil, g.StencilPaths...)
		allImage = append(allImage, g.ImageCommands...)
		allGPUTex = append(allGPUTex, g.GPUTextureCommands...)
		allText = append(allText, g.TextBatches...)
		allGlyph = append(allGlyph, g.GlyphMaskBatches...)
	}

	// Build combined resources (single buffer write per tier).
	var combinedSdfRes *sdfFrameResources
	if len(allSDF) > 0 {
		res, err := s.buildSDFResources(allSDF, w, h)
		if err != nil {
			return fmt.Errorf("build SDF resources: %w", err)
		}
		combinedSdfRes = res
	}

	var combinedConvexRes *convexFrameResources
	if len(allConvex) > 0 {
		res, err := s.buildConvexResources(allConvex, w, h)
		if err != nil {
			return fmt.Errorf("build convex resources: %w", err)
		}
		combinedConvexRes = res
	}

	var combinedStencilRes []*stencilCoverBuffers
	if len(allStencil) > 0 {
		res, err := s.buildStencilResourcesBatch(allStencil, w, h)
		if err != nil {
			return fmt.Errorf("build stencil resources: %w", err)
		}
		combinedStencilRes = res
	}

	var combinedImageRes *imageFrameResources
	if len(allImage) > 0 {
		res, err := s.buildImageResources(allImage, w, h)
		if err != nil {
			return fmt.Errorf("build image resources: %w", err)
		}
		combinedImageRes = res
	}

	var combinedGPUTexRes *imageFrameResources
	if len(allGPUTex) > 0 {
		res, err := s.buildGPUTextureResources(allGPUTex, w, h, false)
		if err != nil {
			return fmt.Errorf("build gpu texture resources: %w", err)
		}
		combinedGPUTexRes = res
	}

	var combinedTextRes *textFrameResources
	if len(allText) > 0 {
		res, err := s.prepareTextResources(allText)
		if err != nil {
			return err
		}
		combinedTextRes = res
	}

	var combinedGlyphRes *glyphMaskFrameResources
	if len(allGlyph) > 0 {
		res, err := s.prepareGlyphMaskResources(allGlyph)
		if err != nil {
			return err
		}
		combinedGlyphRes = res
	}

	// Create per-group resources as sub-ranges of the combined data.
	grpRes := make([]groupResources, len(groups))
	for i := range groups {
		o := &gOff[i]
		grpRes[i].scissorRect = groups[i].Rect
		grpRes[i].stencilPaths = allStencil[o.stencilStart : o.stencilStart+o.stencilCount]

		// Clip bind group: active RRect clip or no-clip.
		clipBG, err := s.getClipBindGroup(groups[i].ClipRRect)
		if err != nil {
			return fmt.Errorf("get clip bind group for group %d: %w", i, err)
		}
		grpRes[i].clipBindGroup = clipBG

		// GPU-CLIP-003a: depth clip resources for arbitrary path clipping.
		// Each call creates per-group owned buffers to avoid cross-group overwrites.
		if groups[i].ClipPath != nil && s.depthClipPipeline != nil {
			res, dcErr := s.depthClipPipeline.BuildClipResources(groups[i].ClipPath, w, h)
			if dcErr == nil && res != nil {
				grpRes[i].depthClipRes = res
				grpRes[i].hasDepthClip = true
			}
		}

		// SDF: shared buffer, per-group firstVertex + vertCount.
		if o.sdfCount > 0 && combinedSdfRes != nil {
			grpRes[i].sdfRes = &sdfFrameResources{
				vertBuf:     combinedSdfRes.vertBuf,
				uniformBuf:  combinedSdfRes.uniformBuf,
				bindGroup:   combinedSdfRes.bindGroup,
				firstVertex: uint32(o.sdfStart * 6), //nolint:gosec // bounded by shape count
				vertCount:   uint32(o.sdfCount * 6), //nolint:gosec // bounded by shape count
			}
			grpRes[i].sdfShapes = allSDF[o.sdfStart : o.sdfStart+o.sdfCount]
		}

		// Convex: shared buffer, per-group firstVertex + vertCount.
		if o.convexCount > 0 && combinedConvexRes != nil {
			grpRes[i].convexRes = s.sliceConvexResources(
				combinedConvexRes, allConvex, o.convexStart, o.convexCount)
		}

		// Stencil: per-path independent buffers, slice the combined array.
		if o.stencilCount > 0 && len(combinedStencilRes) > 0 {
			grpRes[i].stencilRes = combinedStencilRes[o.stencilStart : o.stencilStart+o.stencilCount]
		}

		// Image (Tier 3): per-draw independent bind groups, slice drawCalls.
		if o.imageCount > 0 && combinedImageRes != nil {
			grpRes[i].imageRes = s.sliceImageResources(
				combinedImageRes, o.imageStart, o.imageCount)
		}

		// GPU Texture (Tier 3b): per-draw independent bind groups.
		if o.gpuTexCount > 0 && combinedGPUTexRes != nil {
			grpRes[i].gpuTexRes = s.sliceImageResources(
				combinedGPUTexRes, o.gpuTexStart, o.gpuTexCount)
		}

		// Text (MSDF): shared vertex/index buffers, slice drawCalls.
		if o.textCount > 0 && combinedTextRes != nil {
			grpRes[i].textRes = s.sliceTextResources(
				combinedTextRes, allText, o.textStart, o.textCount)
		}

		// Glyph mask: shared vertex/index buffers, slice drawCalls.
		if o.glyphCount > 0 && combinedGlyphRes != nil {
			grpRes[i].glyphMaskRes = s.sliceGlyphMaskResources(
				combinedGlyphRes, allGlyph, o.glyphStart, o.glyphCount)
		}
	}

	// GPU-CLIP-003a: ensure depth-clipped pipeline variants exist if any group
	// uses depth clipping. Created lazily on first use to avoid unnecessary
	// pipeline compilation when no depth clip is active.
	if s.hasAnyDepthClip(grpRes) {
		if err := s.ensureDepthClipPipelineVariants(); err != nil {
			slogger().Warn("depth clip pipeline variant creation failed", "err", err)
		}
	}

	var baseLayerRes *imageFrameResources
	if baseLayer != nil {
		res, err := s.buildGPUTextureResources([]GPUTextureDrawCommand{*baseLayer}, w, h, true)
		if err != nil {
			slogger().Warn("base layer resource build failed", "err", err)
		} else {
			baseLayerRes = res
		}
	}

	// GPU-CLIP-003a: release per-group depth clip buffers after frame encoding.
	defer s.releaseDepthClipResources(grpRes)
	defer s.releasePendingBindGroups()

	if activeView == nil {
		return s.encodeSubmitReadbackGrouped(w, h, grpRes, target, baseLayerRes)
	}

	blitOnly := s.isBlitOnly(grpRes, baseLayerRes)

	// ADR-017: shared encoder → record render pass without submit.
	if sharedEncoder != nil {
		if blitOnly {
			return s.encodeBlitToEncoder(sharedEncoder, activeView, w, h, grpRes, baseLayerRes, target.DamageRect)
		}
		return s.encodeToEncoder(sharedEncoder, activeView, w, h, grpRes, baseLayerRes)
	}

	if blitOnly {
		return s.encodeBlitOnlyPass(activeView, w, h, grpRes, baseLayerRes, target.DamageRect)
	}
	return s.encodeSubmitSurfaceGrouped(activeView, w, h, grpRes, baseLayerRes)
}

// releaseDepthClipResources frees per-group owned depth clip buffers.
// releasePendingBindGroups frees bind groups that were deferred during
// multi-pass rendering. Called after command buffer submit or at frame end.
func (s *GPURenderSession) releasePendingBindGroups() {
	for _, bg := range s.pendingBindGroupRelease {
		if bg != nil {
			bg.Release()
		}
	}
	s.pendingBindGroupRelease = s.pendingBindGroupRelease[:0]
}

func (s *GPURenderSession) releaseDepthClipResources(grpRes []groupResources) {
	for i := range grpRes {
		if grpRes[i].depthClipRes != nil {
			grpRes[i].depthClipRes.Release()
			grpRes[i].depthClipRes = nil
		}
	}
}

// prepareTextResources builds text GPU resources if there are text batches.
// Text pipeline failure is non-fatal: logs and skips text rendering.
func (s *GPURenderSession) prepareTextResources(textBatches []TextBatch) (*textFrameResources, error) {
	if len(textBatches) == 0 {
		return nil, nil //nolint:nilnil // no text to render
	}
	if err := s.ensureTextPipeline(); err != nil {
		slogger().Warn("text pipeline init failed", "err", err)
		return nil, nil //nolint:nilnil // non-fatal, skip text
	}
	res, err := s.buildTextResources(textBatches)
	if err != nil {
		return nil, fmt.Errorf("build text resources: %w", err)
	}
	return res, nil
}

// Size returns the current shared texture dimensions.
func (s *GPURenderSession) Size() (uint32, uint32) {
	return s.textures.width, s.textures.height
}

// Destroy releases all GPU resources held by the session. Safe to call
// multiple times or on a session with no allocated resources.
// The surface view is not destroyed -- it is owned by the caller.
func (s *GPURenderSession) Destroy() {
	// Drain the GPU queue before freeing any in-flight resources.
	// WaitIdle guarantees all prior submissions are complete (FIFO queue).
	if len(s.prevCmdBufs) > 0 {
		s.drainQueue()
		for _, cb := range s.prevCmdBufs {
			if cb != nil {
				s.device.FreeCommandBuffer(cb)
			}
		}
		s.prevCmdBufs = s.prevCmdBufs[:0]
	}
	s.destroyPersistentBuffers()
	s.textures.destroyTextures()
	s.surfaceView = nil
	s.surfaceWidth = 0
	s.surfaceHeight = 0
	s.lastView = nil
	// Do not destroy pipelines -- they are owned by the caller.
}

// drainQueue waits for all prior GPU submissions to complete.
// Since the GPU queue is FIFO, WaitIdle guarantees all prior submissions are done.
func (s *GPURenderSession) drainQueue() {
	if err := s.device.WaitIdle(); err != nil {
		slogger().Warn("WaitIdle failed during queue drain", "err", err)
	}
}

func (s *GPURenderSession) destroyPersistentBuffers() { //nolint:gocyclo,cyclop,funlen,gocognit // sequential resource cleanup across 6 tiers
	if s.sdfBindGroup != nil {
		s.sdfBindGroup.Release()
		s.sdfBindGroup = nil
	}
	if s.sdfUniformBuf != nil {
		s.sdfUniformBuf.Release()
		s.sdfUniformBuf = nil
	}
	if s.sdfVertBuf != nil {
		s.sdfVertBuf.Release()
		s.sdfVertBuf = nil
		s.sdfVertBufCap = 0
	}
	if s.convexBindGroup != nil {
		s.convexBindGroup.Release()
		s.convexBindGroup = nil
	}
	if s.convexUniformBuf != nil {
		s.convexUniformBuf.Release()
		s.convexUniformBuf = nil
	}
	if s.convexVertBuf != nil {
		s.convexVertBuf.Release()
		s.convexVertBuf = nil
		s.convexVertBufCap = 0
	}
	// Tier 3: Image per-draw pools.
	for i, bg := range s.imageBindGroups {
		if bg != nil {
			bg.Release()
			s.imageBindGroups[i] = nil
		}
	}
	s.imageBindGroups = nil
	for _, buf := range s.imageUniformBufs {
		if buf != nil {
			buf.Release()
		}
	}
	s.imageUniformBufs = nil
	if s.imageVertBuf != nil {
		s.imageVertBuf.Release()
		s.imageVertBuf = nil
		s.imageVertBufCap = 0
	}
	if s.imageCache != nil {
		s.imageCache.Destroy()
		s.imageCache = nil
	}
	if s.imagePipeline != nil {
		s.imagePipeline.Destroy()
		s.imagePipeline = nil
	}
	// Tier 3b: GPU texture compositing per-draw pools.
	for i, bg := range s.gpuTexBindGroups {
		if bg != nil {
			bg.Release()
			s.gpuTexBindGroups[i] = nil
		}
	}
	s.gpuTexBindGroups = nil
	s.releasePendingBindGroups()
	s.pendingBindGroupRelease = nil
	for _, buf := range s.gpuTexUniformBufs {
		if buf != nil {
			buf.Release()
		}
	}
	s.gpuTexUniformBufs = nil
	if s.gpuTexVertBuf != nil {
		s.gpuTexVertBuf.Release()
		s.gpuTexVertBuf = nil
		s.gpuTexVertBufCap = 0
	}
	if s.gpuTexBaseVertBuf != nil {
		s.gpuTexBaseVertBuf.Release()
		s.gpuTexBaseVertBuf = nil
		s.gpuTexBaseVertBufCap = 0
	}
	// Tier 4: Text per-batch pools.
	for i, bg := range s.textBindGroups {
		if bg != nil {
			bg.Release()
			s.textBindGroups[i] = nil
		}
	}
	s.textBindGroups = nil
	for _, buf := range s.textUniformBufs {
		if buf != nil {
			buf.Release()
		}
	}
	s.textUniformBufs = nil
	if s.textIdxBuf != nil {
		s.textIdxBuf.Release()
		s.textIdxBuf = nil
		s.textIdxBufCap = 0
	}
	if s.textVertBuf != nil {
		s.textVertBuf.Release()
		s.textVertBuf = nil
		s.textVertBufCap = 0
	}
	// Atlas textures are owned by GPUShared (shared across sessions).
	// Just clear refs, don't Release.
	s.textAtlasView = nil
	s.textAtlasTex = nil
	// Tier 6: Glyph mask text per-batch pools.
	for i, bg := range s.glyphMaskBindGroups {
		if bg != nil {
			bg.Release()
			s.glyphMaskBindGroups[i] = nil
		}
	}
	s.glyphMaskBindGroups = nil
	for _, buf := range s.glyphMaskUniformBufs {
		if buf != nil {
			buf.Release()
		}
	}
	s.glyphMaskUniformBufs = nil
	if s.glyphMaskIdxBuf != nil {
		s.glyphMaskIdxBuf.Release()
		s.glyphMaskIdxBuf = nil
		s.glyphMaskIdxBufCap = 0
	}
	if s.glyphMaskVertBuf != nil {
		s.glyphMaskVertBuf.Release()
		s.glyphMaskVertBuf = nil
		s.glyphMaskVertBufCap = 0
	}
	if s.glyphMaskPipeline != nil {
		s.glyphMaskPipeline.Destroy()
		s.glyphMaskPipeline = nil
	}
	if s.depthClipPipeline != nil {
		s.depthClipPipeline.Destroy()
		s.depthClipPipeline = nil
	}
	for _, b := range s.stencilBufPool {
		b.destroy()
	}
	s.stencilBufPool = s.stencilBufPool[:0]

	// Clip bind group pool.
	for i, bg := range s.clipBindPool {
		if bg != nil {
			bg.Release()
			s.clipBindPool[i] = nil
		}
	}
	s.clipBindPool = nil
	for _, buf := range s.clipUniformPool {
		if buf != nil {
			buf.Release()
		}
	}
	s.clipUniformPool = nil
	s.clipPoolUsed = 0
	if s.noClipBindGroup != nil {
		s.noClipBindGroup.Release()
		s.noClipBindGroup = nil
	}
	if s.noClipUniformBuf != nil {
		s.noClipUniformBuf.Release()
		s.noClipUniformBuf = nil
	}
	if s.clipBindLayout != nil {
		s.clipBindLayout.Release()
		s.clipBindLayout = nil
	}
}

// sdfFrameResources holds per-frame GPU resources for SDF rendering.
type sdfFrameResources struct {
	vertBuf     *wgpu.Buffer
	uniformBuf  *wgpu.Buffer
	bindGroup   *wgpu.BindGroup
	vertCount   uint32
	firstVertex uint32 // offset into shared vertex buffer (for scissor group sub-ranges)
}

// ensurePipelines creates SDF, convex, and stencil pipelines if they don't
// exist yet. Pipelines are lazily created on first use. The text pipeline
// is NOT created here — it is created on demand when text batches are present
// (see ensureTextPipeline).
func (s *GPURenderSession) ensurePipelines() error {
	if s.sdfPipeline == nil {
		s.sdfPipeline = NewSDFRenderPipeline(s.device, s.queue)
	}
	s.sdfPipeline.SetClipBindLayout(s.clipBindLayout)
	if err := s.sdfPipeline.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("SDF pipeline: %w", err)
	}

	if s.convexRenderer == nil {
		s.convexRenderer = NewConvexRenderer(s.device, s.queue)
	}
	s.convexRenderer.SetClipBindLayout(s.clipBindLayout)
	if err := s.convexRenderer.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("convex pipeline: %w", err)
	}

	if s.stencilRenderer == nil {
		s.stencilRenderer = NewStencilRenderer(s.device, s.queue)
	}
	s.stencilRenderer.SetClipBindLayout(s.clipBindLayout)
	// Recreate stencil pipelines if cover layout was created without clip.
	if s.stencilRenderer.nonZeroStencilPipeline == nil ||
		(s.clipBindLayout != nil && !s.stencilRenderer.coverPipeLayoutHasClip) {
		s.stencilRenderer.destroyPipelines()
		if err := s.stencilRenderer.createPipelines(); err != nil {
			return fmt.Errorf("stencil pipelines: %w", err)
		}
	}

	// Image pipeline (Tier 3) — lazily created alongside other pipelines.
	if s.imagePipeline == nil {
		s.imagePipeline = NewTexturedQuadPipeline(s.device, s.queue)
	}
	s.imagePipeline.SetClipBindLayout(s.clipBindLayout)
	if err := s.imagePipeline.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("image pipeline: %w", err)
	}
	if s.imageCache == nil {
		s.imageCache = NewImageCache(s.device, s.queue)
	}

	// Depth clip pipeline (GPU-CLIP-003a) — lazily created alongside others.
	if s.depthClipPipeline == nil {
		s.depthClipPipeline = NewDepthClipPipeline(s.device, s.queue)
	}
	if err := s.depthClipPipeline.ensurePipeline(); err != nil {
		return fmt.Errorf("depth clip pipeline: %w", err)
	}

	return nil
}

// hasAnyDepthClip returns true if any group in the slice has depth clipping active.
func (s *GPURenderSession) hasAnyDepthClip(grpRes []groupResources) bool {
	for i := range grpRes {
		if grpRes[i].hasDepthClip {
			return true
		}
	}
	return false
}

// ensureDepthClipPipelineVariants creates the depth-clipped pipeline variant
// for each renderer that participates in the render pass. These are created
// lazily (only when at least one group uses depth clipping) to avoid
// unnecessary GPU pipeline compilation for the common no-clip case.
func (s *GPURenderSession) ensureDepthClipPipelineVariants() error {
	if err := s.sdfPipeline.ensureDepthClipPipeline(); err != nil {
		return fmt.Errorf("SDF depth clip pipeline: %w", err)
	}
	if err := s.convexRenderer.ensureDepthClipPipeline(); err != nil {
		return fmt.Errorf("convex depth clip pipeline: %w", err)
	}
	if err := s.stencilRenderer.ensureDepthClipPipelines(); err != nil {
		return fmt.Errorf("stencil depth clip pipelines: %w", err)
	}
	if err := s.imagePipeline.ensureDepthClipPipeline(); err != nil {
		return fmt.Errorf("image depth clip pipeline: %w", err)
	}
	if s.textPipeline != nil {
		if err := s.textPipeline.ensureDepthClipPipeline(); err != nil {
			return fmt.Errorf("MSDF text depth clip pipeline: %w", err)
		}
	}
	if s.glyphMaskPipeline != nil {
		if err := s.glyphMaskPipeline.ensureDepthClipPipeline(); err != nil {
			return fmt.Errorf("glyph mask depth clip pipeline: %w", err)
		}
	}
	return nil
}

// ensureClipBindLayout creates the shared bind group layout for the RRect
// clip uniform at @group(1) @binding(0), plus the no-clip bind group that
// is used when no RRect clip is active.
func (s *GPURenderSession) ensureClipBindLayout() error {
	if s.clipBindLayout != nil {
		return nil
	}

	layout, err := s.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "clip_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: clipParamsSize},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create clip bind layout: %w", err)
	}
	s.clipBindLayout = layout

	// Create the no-clip uniform buffer (clip_enabled=0.0).
	noClip := NoClipParams()
	buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "clip_no_clip_uniform",
		Size:  clipParamsSize,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create no-clip uniform buffer: %w", err)
	}
	if err := s.queue.WriteBuffer(buf, 0, noClip.Bytes()); err != nil {
		buf.Release()
		return fmt.Errorf("write no-clip uniform buffer: %w", err)
	}
	s.noClipUniformBuf = buf

	bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "clip_no_clip_bind",
		Layout: s.clipBindLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: clipParamsSize},
		},
	})
	if err != nil {
		return fmt.Errorf("create no-clip bind group: %w", err)
	}
	s.noClipBindGroup = bg
	return nil
}

// getClipBindGroup returns a bind group for the given ClipParams. If params
// is nil, returns the shared no-clip bind group. Otherwise, allocates from
// a pool of per-frame clip uniform buffers.
func (s *GPURenderSession) getClipBindGroup(params *ClipParams) (*wgpu.BindGroup, error) {
	if params == nil {
		return s.noClipBindGroup, nil
	}

	// Reuse from pool if available.
	idx := s.clipPoolUsed
	if idx < len(s.clipUniformPool) {
		// Reuse existing buffer, just update its contents.
		if err := s.queue.WriteBuffer(s.clipUniformPool[idx], 0, params.Bytes()); err != nil {
			return nil, fmt.Errorf("write clip uniform: %w", err)
		}
		s.clipPoolUsed++
		return s.clipBindPool[idx], nil
	}

	// Grow pool: create new buffer and bind group.
	buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: fmt.Sprintf("clip_uniform_%d", idx),
		Size:  clipParamsSize,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create clip uniform buffer: %w", err)
	}
	if err := s.queue.WriteBuffer(buf, 0, params.Bytes()); err != nil {
		buf.Release()
		return nil, fmt.Errorf("write clip uniform: %w", err)
	}
	bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  fmt.Sprintf("clip_bind_%d", idx),
		Layout: s.clipBindLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: clipParamsSize},
		},
	})
	if err != nil {
		buf.Release()
		return nil, fmt.Errorf("create clip bind group: %w", err)
	}
	s.clipUniformPool = append(s.clipUniformPool, buf)
	s.clipBindPool = append(s.clipBindPool, bg)
	s.clipPoolUsed++
	return bg, nil
}

// ClipBindLayout returns the shared clip bind group layout for @group(1).
// Pipeline creation methods use this to include the clip layout in their
// pipeline layouts. Must call ensureClipBindLayout() first.
func (s *GPURenderSession) ClipBindLayout() *wgpu.BindGroupLayout {
	return s.clipBindLayout
}

// ensureTextPipeline creates the MSDF text pipeline on demand. Called only
// when text batches are present. Returns error if shader compilation fails.
func (s *GPURenderSession) ensureTextPipeline() error {
	if s.textPipeline == nil {
		s.textPipeline = NewMSDFTextPipeline(s.device, s.queue)
	}
	s.textPipeline.SetClipBindLayout(s.clipBindLayout)
	if err := s.textPipeline.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("text pipeline: %w", err)
	}
	return nil
}

// buildSDFResources updates persistent vertex buffer, uniform buffer, and bind
// group for rendering SDF shapes. Buffers are only reallocated when the data
// exceeds current capacity.
func (s *GPURenderSession) buildSDFResources(shapes []SDFRenderShape, w, h uint32) (*sdfFrameResources, error) {
	var vertexData []byte
	s.sdfVertexStaging, vertexData = buildSDFRenderVerticesReuse(shapes, w, h, s.sdfVertexStaging)
	vertexCount := uint32(len(shapes) * 6) //nolint:gosec // shape count fits uint32
	vertSize := uint64(len(vertexData))

	if s.sdfVertBuf == nil || s.sdfVertBufCap < vertSize {
		if s.sdfBindGroup != nil {
			s.sdfBindGroup.Release()
			s.sdfBindGroup = nil
		}
		if s.sdfVertBuf != nil {
			s.sdfVertBuf.Release()
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_sdf_verts",
			Size:  allocSize,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			s.sdfVertBuf = nil
			s.sdfVertBufCap = 0
			return nil, fmt.Errorf("create vertex buffer: %w", err)
		}
		s.sdfVertBuf = buf
		s.sdfVertBufCap = allocSize
	}
	if err := s.queue.WriteBuffer(s.sdfVertBuf, 0, vertexData); err != nil {
		return nil, fmt.Errorf("write vertex buffer: %w", err)
	}

	uniformData := makeSDFRenderUniform(w, h)
	if s.sdfUniformBuf == nil {
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_sdf_uniform",
			Size:  sdfRenderUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create uniform buffer: %w", err)
		}
		s.sdfUniformBuf = buf
		// Bind group must be recreated with the new uniform buffer.
		if s.sdfBindGroup != nil {
			s.sdfBindGroup.Release()
			s.sdfBindGroup = nil
		}
	}
	if err := s.queue.WriteBuffer(s.sdfUniformBuf, 0, uniformData); err != nil {
		return nil, fmt.Errorf("write uniform buffer: %w", err)
	}

	if s.sdfBindGroup == nil {
		bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  "session_sdf_bind",
			Layout: s.sdfPipeline.uniformLayout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: s.sdfUniformBuf, Offset: 0, Size: sdfRenderUniformSize},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create bind group: %w", err)
		}
		s.sdfBindGroup = bg
	}

	return &sdfFrameResources{
		vertBuf:    s.sdfVertBuf,
		uniformBuf: s.sdfUniformBuf,
		bindGroup:  s.sdfBindGroup,
		vertCount:  vertexCount,
	}, nil
}

// buildConvexResources updates persistent vertex buffer, uniform buffer, and
// bind group for rendering convex polygons. Buffers are only reallocated when
// the data exceeds current capacity.
func (s *GPURenderSession) buildConvexResources(commands []ConvexDrawCommand, w, h uint32) (*convexFrameResources, error) {
	var vertexData []byte
	s.convexVertexStaging, vertexData = buildConvexVerticesReuse(commands, s.convexVertexStaging)
	if len(vertexData) == 0 {
		return nil, nil //nolint:nilnil // empty vertex data is a valid no-op, not an error
	}
	vertexCount := convexVertexCount(commands)
	vertSize := uint64(len(vertexData))

	if s.convexVertBuf == nil || s.convexVertBufCap < vertSize {
		if s.convexBindGroup != nil {
			s.convexBindGroup.Release()
			s.convexBindGroup = nil
		}
		if s.convexVertBuf != nil {
			s.convexVertBuf.Release()
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_convex_verts",
			Size:  allocSize,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			s.convexVertBuf = nil
			s.convexVertBufCap = 0
			return nil, fmt.Errorf("create convex vertex buffer: %w", err)
		}
		s.convexVertBuf = buf
		s.convexVertBufCap = allocSize
	}
	if err := s.queue.WriteBuffer(s.convexVertBuf, 0, vertexData); err != nil {
		return nil, fmt.Errorf("write convex vertex buffer: %w", err)
	}

	uniformData := makeSDFRenderUniform(w, h) // Same 16-byte viewport layout.
	if s.convexUniformBuf == nil {
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_convex_uniform",
			Size:  sdfRenderUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create convex uniform buffer: %w", err)
		}
		s.convexUniformBuf = buf
		if s.convexBindGroup != nil {
			s.convexBindGroup.Release()
			s.convexBindGroup = nil
		}
	}
	if err := s.queue.WriteBuffer(s.convexUniformBuf, 0, uniformData); err != nil {
		return nil, fmt.Errorf("write convex uniform buffer: %w", err)
	}

	if s.convexBindGroup == nil {
		bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  "session_convex_bind",
			Layout: s.convexRenderer.uniformLayout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: s.convexUniformBuf, Offset: 0, Size: sdfRenderUniformSize},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create convex bind group: %w", err)
		}
		s.convexBindGroup = bg
	}

	return &convexFrameResources{
		vertBuf:    s.convexVertBuf,
		uniformBuf: s.convexUniformBuf,
		bindGroup:  s.convexBindGroup,
		vertCount:  vertexCount,
	}, nil
}

// buildStencilResourcesBatch builds stencil GPU buffers for all paths,
// reusing pooled buffer sets where possible. Unused pool entries beyond the
// current path count are kept alive for future frames.
func (s *GPURenderSession) buildStencilResourcesBatch(paths []StencilPathCommand, w, h uint32) ([]*stencilCoverBuffers, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	// Grow pool if needed.
	for len(s.stencilBufPool) < len(paths) {
		s.stencilBufPool = append(s.stencilBufPool, nil)
	}

	result := make([]*stencilCoverBuffers, len(paths))
	for i := range paths {
		cmd := &paths[i]
		color := gg.RGBA{
			R: float64(cmd.Color[0]),
			G: float64(cmd.Color[1]),
			B: float64(cmd.Color[2]),
			A: float64(cmd.Color[3]),
		}

		// Destroy old pooled entry and create fresh buffers.
		// Stencil paths vary wildly per frame (different vertex counts, colors),
		// so recreating is simpler than capacity tracking for 6 sub-buffers.
		if s.stencilBufPool[i] != nil {
			s.stencilBufPool[i].destroy()
			s.stencilBufPool[i] = nil
		}
		bufs, err := s.stencilRenderer.createRenderBuffers(w, h, cmd.Vertices, cmd.CoverQuad, color)
		if err != nil {
			// Clean up buffers created in this batch.
			for j := 0; j < i; j++ {
				if s.stencilBufPool[j] != nil {
					s.stencilBufPool[j].destroy()
					s.stencilBufPool[j] = nil
				}
			}
			return nil, fmt.Errorf("build stencil resources for path %d: %w", i, err)
		}
		s.stencilBufPool[i] = bufs
		result[i] = bufs
	}

	return result, nil
}

// buildTextResources updates persistent vertex, index, and per-batch uniform
// buffers for MSDF text rendering. Each batch gets its own uniform buffer and
// bind group so that different text colors/transforms render correctly.
// Vertex and index buffers are shared across all batches. Buffers are grow-only.
func (s *GPURenderSession) buildTextResources(batches []TextBatch) (*textFrameResources, error) { //nolint:gocyclo,cyclop,funlen // sequential buffer management
	if len(batches) == 0 {
		return nil, nil //nolint:nilnil // empty batch list is a valid no-op, not an error
	}

	// The bind group requires an atlas texture view.
	if s.textAtlasView == nil {
		slogger().Warn("text atlas view is nil, skipping text rendering")
		return nil, nil //nolint:nilnil // no atlas uploaded yet
	}

	// Aggregate all quads into shared vertex/index buffers.
	totalQuads := 0
	for i := range batches {
		totalQuads += len(batches[i].Quads)
	}
	if totalQuads == 0 {
		return nil, nil //nolint:nilnil // no quads to render
	}

	allQuads := make([]TextQuad, 0, totalQuads)
	for i := range batches {
		allQuads = append(allQuads, batches[i].Quads...)
	}

	// Build and upload shared vertex data (4 vertices per quad, 16 bytes per vertex).
	vertexData := buildTextVertexData(allQuads)
	vertSize := uint64(len(vertexData))

	if s.textVertBuf == nil || s.textVertBufCap < vertSize {
		s.invalidateTextBindGroups()
		if s.textVertBuf != nil {
			s.textVertBuf.Release()
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_text_verts",
			Size:  allocSize,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			s.textVertBuf = nil
			s.textVertBufCap = 0
			return nil, fmt.Errorf("create text vertex buffer: %w", err)
		}
		s.textVertBuf = buf
		s.textVertBufCap = allocSize
	}
	if err := s.queue.WriteBuffer(s.textVertBuf, 0, vertexData); err != nil {
		return nil, fmt.Errorf("write text vertex buffer: %w", err)
	}

	// Build and upload shared index data (6 indices per quad, 2 bytes per index).
	indexData := buildTextIndexData(totalQuads)
	idxSize := uint64(len(indexData))

	if s.textIdxBuf == nil || s.textIdxBufCap < idxSize {
		s.invalidateTextBindGroups()
		if s.textIdxBuf != nil {
			s.textIdxBuf.Release()
		}
		allocSize := idxSize * 2
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_text_indices",
			Size:  allocSize,
			Usage: gputypes.BufferUsageIndex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			s.textIdxBuf = nil
			s.textIdxBufCap = 0
			return nil, fmt.Errorf("create text index buffer: %w", err)
		}
		s.textIdxBuf = buf
		s.textIdxBufCap = allocSize
	}
	if err := s.queue.WriteBuffer(s.textIdxBuf, 0, indexData); err != nil {
		return nil, fmt.Errorf("write text index buffer: %w", err)
	}

	// Grow uniform buffer and bind group pools to match batch count.
	s.ensureTextBatchPools(len(batches))

	// Build per-batch draw calls with individual uniform buffers.
	drawCalls := make([]textDrawCall, 0, len(batches))
	quadOffset := 0

	for i, batch := range batches {
		nQuads := len(batch.Quads)
		if nQuads == 0 {
			continue
		}

		// Write uniform data for this batch.
		uniformData := makeTextUniform(batch.Color, batch.Transform, batch.PxRange, batch.AtlasSize)
		if err := s.queue.WriteBuffer(s.textUniformBufs[i], 0, uniformData); err != nil {
			return nil, fmt.Errorf("write text uniform[%d]: %w", i, err)
		}

		// Ensure bind group exists for this batch.
		if s.textBindGroups[i] == nil {
			bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
				Label:  fmt.Sprintf("session_text_bind_%d", i),
				Layout: s.textPipeline.uniformLayout,
				Entries: []wgpu.BindGroupEntry{
					{Binding: 0, Buffer: s.textUniformBufs[i], Offset: 0, Size: textUniformSize},
					{Binding: 1, TextureView: s.textAtlasView},
					{Binding: 2, Sampler: s.textPipeline.sampler},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("create text bind group[%d]: %w", i, err)
			}
			s.textBindGroups[i] = bg
		}

		indexOffset := uint32(quadOffset * 6) //nolint:gosec // bounded by MaxQuadCapacity
		indexCount := uint32(nQuads * 6)      //nolint:gosec // bounded by MaxQuadCapacity

		drawCalls = append(drawCalls, textDrawCall{
			indexOffset: indexOffset,
			indexCount:  indexCount,
			bindGroup:   s.textBindGroups[i],
		})

		quadOffset += nQuads
	}

	return &textFrameResources{
		vertBuf:   s.textVertBuf,
		idxBuf:    s.textIdxBuf,
		drawCalls: drawCalls,
	}, nil
}

// ensureTextBatchPools grows the uniform buffer and bind group pools to at
// least n entries. Existing entries are preserved.
func (s *GPURenderSession) ensureTextBatchPools(n int) {
	for len(s.textUniformBufs) < n {
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("session_text_uniform_%d", len(s.textUniformBufs)),
			Size:  textUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			slogger().Warn("failed to create text uniform buffer", "err", err)
			return
		}
		s.textUniformBufs = append(s.textUniformBufs, buf)
		s.textBindGroups = append(s.textBindGroups, nil) // bind group created lazily
	}
}

// invalidateTextBindGroups destroys all cached text bind groups so they are
// recreated with updated buffer references.
func (s *GPURenderSession) invalidateTextBindGroups() {
	for i, bg := range s.textBindGroups {
		if bg != nil {
			bg.Release()
			s.textBindGroups[i] = nil
		}
	}
}

// SetTextAtlas sets the atlas texture and view for MSDF text rendering.
// The session takes ownership of both the texture and view and will destroy
// them when the session is destroyed or when a new atlas is set.
//
// Call this after uploading atlas data to the GPU (e.g., from
// TextRenderer.SyncAtlases). The atlas view is used in the text bind group.
func (s *GPURenderSession) SetTextAtlas(tex *wgpu.Texture, view *wgpu.TextureView) {
	// Atlas textures now owned by GPUShared — don't Release old refs here.
	s.textAtlasTex = tex
	s.textAtlasView = view
	s.invalidateTextBindGroups()
}

// SetTextAtlasRef sets the atlas texture view as a non-owning reference.
// Unlike SetTextAtlas, this does NOT take ownership — the texture is owned
// by GPUShared and shared across all sessions.
func (s *GPURenderSession) SetTextAtlasRef(tex *wgpu.Texture, view *wgpu.TextureView) {
	if s.textAtlasView == view {
		return // already set
	}
	// Don't release old — it may be owned by GPUShared too.
	// Only release if this session created its own (non-shared) atlas.
	s.textAtlasTex = tex
	s.textAtlasView = view
	s.invalidateTextBindGroups()
}

// TextPipelineRef returns the MSDF text pipeline. It is lazily created by
// ensurePipelines, so may be nil before RenderFrame is called.
func (s *GPURenderSession) TextPipelineRef() *MSDFTextPipeline {
	return s.textPipeline
}

// TextAtlasView returns the current text atlas texture view, or nil if no
// atlas has been uploaded yet.
func (s *GPURenderSession) TextAtlasView() *wgpu.TextureView {
	return s.textAtlasView
}

// SetTextPipeline sets an external MSDF text pipeline for the session to use.
// The session does not own the pipeline and will not destroy it.
func (s *GPURenderSession) SetTextPipeline(p *MSDFTextPipeline) {
	s.textPipeline = p
}

// ---- Tier 6: Glyph mask text rendering ----

// prepareGlyphMaskResources builds glyph mask GPU resources if there are batches.
// Pipeline failure is non-fatal: logs and skips glyph mask rendering.
func (s *GPURenderSession) prepareGlyphMaskResources(batches []GlyphMaskBatch) (*glyphMaskFrameResources, error) {
	if len(batches) == 0 {
		return nil, nil //nolint:nilnil // no glyph mask text to render
	}

	// Check if any batch uses LCD mode.
	hasLCD := false
	for i := range batches {
		if batches[i].IsLCD {
			hasLCD = true
			break
		}
	}

	if err := s.ensureGlyphMaskPipeline(hasLCD); err != nil {
		slogger().Warn("glyph mask pipeline init failed", "err", err)
		return nil, nil //nolint:nilnil // non-fatal, skip glyph mask text
	}
	res, err := s.buildGlyphMaskResources(batches)
	if err != nil {
		return nil, fmt.Errorf("build glyph mask resources: %w", err)
	}
	return res, nil
}

// buildImageResources creates GPU resources for all image draw commands in the
// current frame. Each command gets its own uniform buffer + bind group (with
// texture and sampler), but all share a single vertex buffer.
func (s *GPURenderSession) buildImageResources(cmds []ImageDrawCommand, w, h uint32) (*imageFrameResources, error) {
	if len(cmds) == 0 {
		return nil, nil //nolint:nilnil // no images
	}

	// Build combined vertex data (6 verts per quad).
	totalVertBytes := len(cmds) * 6 * imageVertexStride //nolint:mnd // 6 verts per quad
	var allVertData []byte
	for i := range cmds {
		allVertData = append(allVertData, buildImageVertices(&cmds[i])...)
	}

	// Ensure persistent vertex buffer is large enough.
	needed := uint64(totalVertBytes) //nolint:gosec // bounded by command count
	if s.imageVertBuf == nil || s.imageVertBufCap < needed {
		if s.imageVertBuf != nil {
			s.imageVertBuf.Release()
		}
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "image_vert_buf",
			Size:  needed,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create image vertex buffer: %w", err)
		}
		s.imageVertBuf = buf
		s.imageVertBufCap = needed
	}
	if err := s.queue.WriteBuffer(s.imageVertBuf, 0, allVertData); err != nil {
		return nil, fmt.Errorf("upload image vertices: %w", err)
	}

	// Grow the uniform buffer and bind group pools as needed.
	for len(s.imageUniformBufs) < len(cmds) {
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("image_uniform_%d", len(s.imageUniformBufs)),
			Size:  imageUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create image uniform buffer: %w", err)
		}
		s.imageUniformBufs = append(s.imageUniformBufs, buf)
	}

	// Release stale bind groups from previous frame.
	for i := range s.imageBindGroups {
		if s.imageBindGroups[i] != nil {
			s.imageBindGroups[i].Release()
			s.imageBindGroups[i] = nil
		}
	}

	drawCalls := make([]imageDrawCall, 0, len(cmds))
	for i := range cmds {
		cmd := &cmds[i]

		// Upload uniform data.
		uniformData := makeImageUniform(w, h, cmd.Opacity)
		if err := s.queue.WriteBuffer(s.imageUniformBufs[i], 0, uniformData); err != nil {
			return nil, fmt.Errorf("upload image uniform %d: %w", i, err)
		}

		// Get or upload image texture.
		texView, err := s.imageCache.GetOrUpload(cmd)
		if err != nil {
			slogger().Warn("image cache upload failed, skipping", "err", err)
			continue
		}

		// Create bind group: uniform + texture + sampler.
		bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  fmt.Sprintf("image_bind_%d", i),
			Layout: s.imagePipeline.uniformLayout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: s.imageUniformBufs[i], Offset: 0, Size: imageUniformSize},
				{Binding: 1, TextureView: texView},
				{Binding: 2, Sampler: s.imagePipeline.sampler},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create image bind group %d: %w", i, err)
		}

		// Grow the bind group pool if needed.
		for len(s.imageBindGroups) <= i {
			s.imageBindGroups = append(s.imageBindGroups, nil)
		}
		s.imageBindGroups[i] = bg

		drawCalls = append(drawCalls, imageDrawCall{
			bindGroup:   bg,
			firstVertex: uint32(i * 6), //nolint:gosec // bounded by command count, 6 verts per quad
		})
	}

	return &imageFrameResources{
		vertBuf:   s.imageVertBuf,
		drawCalls: drawCalls,
	}, nil
}

// buildGPUTextureResources builds render resources for GPU-to-GPU texture compositing.
// Same pipeline as CPU images, but texture view comes directly — no ImageCache upload.
// Uses session-level persistent buffers (grow-only) to avoid per-frame GPU allocation.
//
// isBaseLayer selects a separate vertex buffer for the base layer to prevent
// base layer vertices (full-screen quad) from overwriting overlay vertices
// (BUG-GG-GPU-TEXTURE-OVERLAY-SIZE).
func (s *GPURenderSession) buildGPUTextureResources(cmds []GPUTextureDrawCommand, w, h uint32, isBaseLayer bool) (*imageFrameResources, error) {
	if len(cmds) == 0 {
		return nil, nil //nolint:nilnil // no GPU texture commands
	}
	if err := s.ensurePipelines(); err != nil {
		return nil, fmt.Errorf("ensure pipelines: %w", err)
	}

	totalVertBytes := len(cmds) * 6 * imageVertexStride //nolint:mnd // 6 verts per quad
	var allVertData []byte
	for i := range cmds {
		cmd := &cmds[i]
		imgCmd := ImageDrawCommand{
			DstX: cmd.DstX, DstY: cmd.DstY, DstW: cmd.DstW, DstH: cmd.DstH,
			Opacity: cmd.Opacity, ViewportWidth: cmd.ViewportWidth, ViewportHeight: cmd.ViewportHeight,
			U0: 0, V0: 0, U1: 1, V1: 1,
		}
		allVertData = append(allVertData, buildImageVertices(&imgCmd)...)
	}

	// Select vertex buffer: base layer and overlays use SEPARATE buffers
	// to prevent overwrite (BUG-GG-GPU-TEXTURE-OVERLAY-SIZE).
	vertBufPtr := &s.gpuTexVertBuf
	vertCapPtr := &s.gpuTexVertBufCap
	label := "gpu_tex_vert_buf"
	if isBaseLayer {
		vertBufPtr = &s.gpuTexBaseVertBuf
		vertCapPtr = &s.gpuTexBaseVertBufCap
		label = "gpu_tex_base_vert_buf"
	}

	needed := uint64(totalVertBytes) //nolint:gosec // bounded
	if *vertBufPtr == nil || *vertCapPtr < needed {
		if *vertBufPtr != nil {
			(*vertBufPtr).Release()
		}
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: label,
			Size:  needed,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create gpu texture vertex buffer: %w", err)
		}
		*vertBufPtr = buf
		*vertCapPtr = needed
	}
	if err := s.queue.WriteBuffer(*vertBufPtr, 0, allVertData); err != nil {
		return nil, fmt.Errorf("upload gpu texture vertices: %w", err)
	}

	drawCalls := make([]imageDrawCall, 0, len(cmds))
	for i := range cmds {
		cmd := &cmds[i]
		if cmd.View.IsNil() {
			continue
		}
		texView := (*wgpu.TextureView)(cmd.View.Pointer())

		// Grow uniform buffer pool.
		for len(s.gpuTexUniformBufs) <= i {
			buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
				Label: fmt.Sprintf("gpu_tex_uniform_%d", len(s.gpuTexUniformBufs)),
				Size:  imageUniformSize,
				Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
			})
			if err != nil {
				return nil, fmt.Errorf("create gpu texture uniform buffer: %w", err)
			}
			s.gpuTexUniformBufs = append(s.gpuTexUniformBufs, buf)
			s.gpuTexBindGroups = append(s.gpuTexBindGroups, nil)
		}

		uniformData := makeImageUniform(w, h, cmd.Opacity)
		if err := s.queue.WriteBuffer(s.gpuTexUniformBufs[i], 0, uniformData); err != nil {
			continue
		}

		// Bind group must be recreated each frame because the texture view changes.
		// DON'T Release() here — shared command encoder may still reference it.
		// Defer release until after submit (Skia Graphite/Rust wgpu pattern).
		if s.gpuTexBindGroups[i] != nil {
			s.pendingBindGroupRelease = append(s.pendingBindGroupRelease, s.gpuTexBindGroups[i])
		}
		bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  fmt.Sprintf("gpu_tex_bind_%d", i),
			Layout: s.imagePipeline.uniformLayout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: s.gpuTexUniformBufs[i], Offset: 0, Size: imageUniformSize},
				{Binding: 1, TextureView: texView},
				{Binding: 2, Sampler: s.imagePipeline.sampler},
			},
		})
		if err != nil {
			continue
		}
		s.gpuTexBindGroups[i] = bg

		drawCalls = append(drawCalls, imageDrawCall{
			bindGroup:   bg,
			firstVertex: uint32(i * 6), //nolint:gosec // bounded
		})
	}

	return &imageFrameResources{
		vertBuf:   *vertBufPtr,
		drawCalls: drawCalls,
	}, nil
}

// sliceImageResources creates an imageFrameResources referencing a sub-range of
// the combined drawCalls. One drawCall per ImageDrawCommand.
func (s *GPURenderSession) sliceImageResources(
	combined *imageFrameResources,
	cmdStart, cmdCount int,
) *imageFrameResources {
	if cmdCount == 0 || combined == nil || cmdStart+cmdCount > len(combined.drawCalls) {
		return nil
	}
	return &imageFrameResources{
		vertBuf:   combined.vertBuf,
		drawCalls: combined.drawCalls[cmdStart : cmdStart+cmdCount],
	}
}

// ensureGlyphMaskPipeline creates the glyph mask pipeline on demand. Called
// only when glyph mask batches are present. If hasLCD is true, also creates
// the LCD pipeline variant for ClearType rendering.
func (s *GPURenderSession) ensureGlyphMaskPipeline(hasLCD bool) error {
	if s.glyphMaskPipeline == nil {
		s.glyphMaskPipeline = NewGlyphMaskPipeline(s.device, s.queue)
	}
	s.glyphMaskPipeline.SetClipBindLayout(s.clipBindLayout)
	if err := s.glyphMaskPipeline.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("glyph mask pipeline: %w", err)
	}
	if hasLCD {
		if err := s.glyphMaskPipeline.ensureLCDPipelineWithStencil(); err != nil {
			slogger().Warn("glyph mask LCD pipeline init failed, falling back to grayscale", "err", err)
			// Non-fatal: LCD batches will render with the grayscale pipeline.
		}
	}
	return nil
}

// buildGlyphMaskResources updates persistent vertex, index, and per-batch
// uniform buffers for glyph mask text rendering. Each batch gets its own
// uniform buffer and bind group. Vertex and index buffers are shared across
// all batches. Buffers are grow-only.
func (s *GPURenderSession) buildGlyphMaskResources(batches []GlyphMaskBatch) (*glyphMaskFrameResources, error) {
	if len(batches) == 0 {
		return nil, nil //nolint:nilnil // empty batch list is a valid no-op
	}

	// Aggregate all quads into shared vertex/index buffers.
	totalQuads := 0
	for i := range batches {
		totalQuads += len(batches[i].Quads)
	}
	if totalQuads == 0 {
		return nil, nil //nolint:nilnil // no quads to render
	}

	allQuads := make([]GlyphMaskQuad, 0, totalQuads)
	for i := range batches {
		allQuads = append(allQuads, batches[i].Quads...)
	}

	// Build and upload shared vertex data (4 vertices per quad, 32 bytes per vertex).
	vertexData := buildGlyphMaskVertexData(allQuads)
	vertSize := uint64(len(vertexData))

	if s.glyphMaskVertBuf == nil || s.glyphMaskVertBufCap < vertSize {
		// Note: no bind group invalidation needed here — glyph mask bind groups
		// reference (uniform, atlas texture, sampler), not vertex/index buffers.
		if s.glyphMaskVertBuf != nil {
			s.glyphMaskVertBuf.Release()
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_glyph_mask_verts",
			Size:  allocSize,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			s.glyphMaskVertBuf = nil
			s.glyphMaskVertBufCap = 0
			return nil, fmt.Errorf("create glyph mask vertex buffer: %w", err)
		}
		s.glyphMaskVertBuf = buf
		s.glyphMaskVertBufCap = allocSize
	}
	if err := s.queue.WriteBuffer(s.glyphMaskVertBuf, 0, vertexData); err != nil {
		return nil, fmt.Errorf("write glyph mask vertex buffer: %w", err)
	}

	// Build and upload shared index data (6 indices per quad, 2 bytes per index).
	indexData := buildGlyphMaskIndexData(totalQuads)
	idxSize := uint64(len(indexData))

	if s.glyphMaskIdxBuf == nil || s.glyphMaskIdxBufCap < idxSize {
		if s.glyphMaskIdxBuf != nil {
			s.glyphMaskIdxBuf.Release()
		}
		allocSize := idxSize * 2
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "session_glyph_mask_indices",
			Size:  allocSize,
			Usage: gputypes.BufferUsageIndex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			s.glyphMaskIdxBuf = nil
			s.glyphMaskIdxBufCap = 0
			return nil, fmt.Errorf("create glyph mask index buffer: %w", err)
		}
		s.glyphMaskIdxBuf = buf
		s.glyphMaskIdxBufCap = allocSize
	}
	if err := s.queue.WriteBuffer(s.glyphMaskIdxBuf, 0, indexData); err != nil {
		return nil, fmt.Errorf("write glyph mask index buffer: %w", err)
	}

	// Determine if this frame uses LCD mode (all batches share the same mode
	// because the GlyphMaskEngine uses a single LCD layout per frame).
	frameIsLCD := hasLCDBatches(batches)

	// Use the max uniform size (96 bytes) for the pool so we never need to
	// recreate buffers when switching between grayscale and LCD modes.
	s.ensureGlyphMaskBatchPools(len(batches), glyphMaskLCDUniformSize)

	// Build per-batch draw calls.
	drawCalls, err := s.buildGlyphMaskDrawCalls(batches)
	if err != nil {
		return nil, err
	}

	return &glyphMaskFrameResources{
		vertBuf:   s.glyphMaskVertBuf,
		idxBuf:    s.glyphMaskIdxBuf,
		drawCalls: drawCalls,
		isLCD:     frameIsLCD,
	}, nil
}

// hasLCDBatches returns true if any batch uses LCD subpixel rendering.
func hasLCDBatches(batches []GlyphMaskBatch) bool {
	for i := range batches {
		if batches[i].IsLCD {
			return true
		}
	}
	return false
}

// buildGlyphMaskDrawCalls creates per-batch draw calls with uniform data upload.
func (s *GPURenderSession) buildGlyphMaskDrawCalls(batches []GlyphMaskBatch) ([]glyphMaskDrawCall, error) {
	drawCalls := make([]glyphMaskDrawCall, 0, len(batches))
	quadOffset := 0

	for i, batch := range batches {
		nQuads := len(batch.Quads)
		if nQuads == 0 {
			continue
		}

		// Write uniform data for this batch.
		var uniformData []byte
		if batch.IsLCD {
			uniformData = makeGlyphMaskLCDUniform(batch.Transform, batch.Color, batch.AtlasWidth, batch.AtlasHeight)
		} else {
			uniformData = makeGlyphMaskUniform(batch.Transform, batch.Color)
		}
		if err := s.queue.WriteBuffer(s.glyphMaskUniformBufs[i], 0, uniformData); err != nil {
			return nil, fmt.Errorf("write glyph mask uniform[%d]: %w", i, err)
		}

		// Ensure bind group exists for this batch.
		if s.glyphMaskBindGroups[i] == nil {
			slogger().Warn("glyph mask bind group nil, skipping batch",
				"index", i, "nQuads", nQuads, "totalBatches", len(batches),
				"poolLen", len(s.glyphMaskBindGroups))
			continue
		}

		indexOffset := uint32(quadOffset * 6) //nolint:gosec // bounded by quad capacity
		indexCount := uint32(nQuads * 6)      //nolint:gosec // bounded by quad capacity

		drawCalls = append(drawCalls, glyphMaskDrawCall{
			indexOffset: indexOffset,
			indexCount:  indexCount,
			bindGroup:   s.glyphMaskBindGroups[i],
		})

		quadOffset += nQuads
	}
	return drawCalls, nil
}

// ensureGlyphMaskBatchPools grows the uniform buffer and bind group pools to
// at least n entries. Existing entries are preserved. The uniformSize parameter
// is the byte size of each uniform buffer (80 for grayscale, 96 for LCD).
func (s *GPURenderSession) ensureGlyphMaskBatchPools(n int, uniformSize uint64) {
	for len(s.glyphMaskUniformBufs) < n {
		buf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("session_glyph_mask_uniform_%d", len(s.glyphMaskUniformBufs)),
			Size:  uniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			slogger().Warn("failed to create glyph mask uniform buffer", "err", err)
			return
		}
		s.glyphMaskUniformBufs = append(s.glyphMaskUniformBufs, buf)
		s.glyphMaskBindGroups = append(s.glyphMaskBindGroups, nil) // bind group created lazily
	}
}

// SetGlyphMaskAtlasView creates or updates the bind group for a glyph mask
// batch at the given index, binding the provided atlas texture view. This must
// be called after syncing atlas textures and before RenderFrame.
//
// When isLCD is true and the LCD pipeline is available, the bind group uses
// the LCD uniform layout (96 bytes with atlas_size) for per-channel alpha
// compositing. Otherwise, the grayscale layout (80 bytes) is used.
func (s *GPURenderSession) SetGlyphMaskAtlasView(batchIndex int, atlasView *wgpu.TextureView, isLCD bool) {
	if s.glyphMaskPipeline == nil {
		slogger().Warn("SetGlyphMaskAtlasView: pipeline not initialized", "batchIndex", batchIndex)
		return
	}
	if atlasView == nil {
		slogger().Warn("SetGlyphMaskAtlasView: nil atlas view", "batchIndex", batchIndex)
		return
	}
	s.ensureGlyphMaskBatchPools(batchIndex+1, glyphMaskLCDUniformSize)
	if batchIndex >= len(s.glyphMaskBindGroups) {
		return
	}
	// Destroy old bind group if it exists.
	if s.glyphMaskBindGroups[batchIndex] != nil {
		s.glyphMaskBindGroups[batchIndex].Release()
		s.glyphMaskBindGroups[batchIndex] = nil
	}

	// Select layout and uniform size based on LCD mode.
	layout := s.glyphMaskPipeline.uniformLayout
	uniformSize := uint64(glyphMaskUniformSize)
	if isLCD && s.glyphMaskPipeline.lcdUniformLayout != nil {
		layout = s.glyphMaskPipeline.lcdUniformLayout
		uniformSize = glyphMaskLCDUniformSize
	}

	bg, err := s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  fmt.Sprintf("session_glyph_mask_bind_%d", batchIndex),
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: s.glyphMaskUniformBufs[batchIndex], Offset: 0, Size: uniformSize},
			{Binding: 1, TextureView: atlasView},
			{Binding: 2, Sampler: s.glyphMaskPipeline.sampler},
		},
	})
	if err != nil {
		slogger().Warn("failed to create glyph mask bind group", "index", batchIndex, "err", err)
		return
	}
	s.glyphMaskBindGroups[batchIndex] = bg
}

// encodeSubmitReadback encodes the unified render pass, copies the resolve
// texture to a staging buffer, submits, waits, and reads back pixels.
func (s *GPURenderSession) encodeSubmitReadback(
	w, h uint32,
	sdfRes *sdfFrameResources,
	sdfShapes []SDFRenderShape,
	convexRes *convexFrameResources,
	stencilRes []*stencilCoverBuffers,
	stencilPaths []StencilPathCommand,
	textRes *textFrameResources,
	glyphMaskRes *glyphMaskFrameResources,
	target gg.GPURenderTarget,
) error {
	encoder, err := s.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "session_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	// BUG-GG-ENCODER-LIFECYCLE-001: defer-based safety net ensures the encoder
	// is always finalized even if a panic or unexpected error path is hit.
	// DiscardEncoding is idempotent (no-op if already released by Finish).
	encoderConsumed := false
	defer func() {
		if !encoderConsumed {
			encoder.DiscardEncoding()
		}
	}()

	// Unified render pass descriptor with MSAA color + stencil + resolve.
	rpDesc := &wgpu.RenderPassDescriptor{
		Label: "session_unified_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: s.textures.resolveView,
			LoadOp:        gputypes.LoadOpClear,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       gputypes.LoadOpClear,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     gputypes.LoadOpClear,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	}

	rp, rpErr := encoder.BeginRenderPass(rpDesc)
	if rpErr != nil {
		return fmt.Errorf("begin render pass: %w", rpErr)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)
	s.applyScissorRect(rp)

	// Bind no-clip at @group(1) for non-grouped path (no RRect clip).
	// Clip bind group is passed to each RecordDraws (must be bound AFTER
	// SetPipeline due to Vulkan pipeline layout requirement).
	clipBG := s.noClipBindGroup

	// Tier 1: SDF shapes (no stencil interaction).
	if sdfRes != nil && len(sdfShapes) > 0 {
		s.sdfPipeline.RecordDraws(rp, sdfRes, clipBG)
	}

	// Tier 2a: Convex polygon fast-path (no stencil interaction).
	if convexRes != nil {
		s.convexRenderer.RecordDraws(rp, convexRes, clipBG)
	}

	// Tier 2b: Stencil-then-cover paths.
	for i, bufs := range stencilRes {
		s.stencilRenderer.RecordPath(rp, bufs, stencilPaths[i].FillRule, clipBG)
	}

	// Tier 4: MSDF text (rendered after shapes).
	if textRes != nil && len(textRes.drawCalls) > 0 {
		s.textPipeline.RecordDraws(rp, textRes, clipBG)
	}

	// Tier 6: Glyph mask text (rendered last, on top of all other geometry).
	if glyphMaskRes != nil && len(glyphMaskRes.drawCalls) > 0 {
		s.glyphMaskPipeline.RecordDraws(rp, glyphMaskRes, clipBG)
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("render pass End failed", "err", endErr)
	}

	// VK-LAYOUT-001: After MSAA resolve the texture is in
	// COLOR_ATTACHMENT_OPTIMAL layout. CopyTextureToBuffer requires
	// TRANSFER_SRC_OPTIMAL. Insert an explicit barrier to transition.
	// This is a no-op on Metal, GLES, software, and noop backends.
	encoder.TransitionTextures([]wgpu.TextureBarrier{{
		Texture: s.textures.resolveTex,
		Usage: wgpu.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

	// Mark encoder as consumed before handing to copySubmitAndReadback,
	// which calls Finish() internally.
	encoderConsumed = true

	// Encode copy and submit, then read back pixels to the target.
	return s.copySubmitAndReadback(encoder, w, h, target)
}

// copySubmitAndReadback creates a staging buffer, encodes the texture-to-buffer
// copy, submits the command buffer, waits for the GPU, and reads back pixels
// into the render target. This is the second half of encodeSubmitReadback,
// extracted for readability.
func (s *GPURenderSession) copySubmitAndReadback(
	encoder *wgpu.CommandEncoder, w, h uint32, target gg.GPURenderTarget,
) error {
	// BUG-GG-ENCODER-LIFECYCLE-001: this method takes ownership of encoder.
	// Defer ensures DiscardEncoding on any error or panic before Finish.
	// DiscardEncoding is idempotent (no-op if already released by Finish).
	encoderConsumed := false
	defer func() {
		if !encoderConsumed {
			encoder.DiscardEncoding()
		}
	}()

	// Copy resolve texture to staging buffer for CPU readback.
	// WebGPU (and DX12) requires BytesPerRow aligned to 256 bytes.
	bytesPerRow := w * 4
	const copyPitchAlignment = 256
	alignedBytesPerRow := (bytesPerRow + copyPitchAlignment - 1) &^ (copyPitchAlignment - 1)
	stagingBufSize := uint64(alignedBytesPerRow) * uint64(h)

	stagingBuf, err := s.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "session_staging",
		Size:  stagingBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer stagingBuf.Release()

	encoder.CopyTextureToBuffer(s.textures.resolveTex, stagingBuf, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{Offset: 0, BytesPerRow: alignedBytesPerRow, RowsPerImage: h},
		TextureBase:  wgpu.ImageCopyTexture{Texture: s.textures.resolveTex, MipLevel: 0},
		Size:         wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	}})

	// Transition resolve texture back to RenderAttachment so the next frame's
	// render pass End() can transition from RENDER_TARGET → RESOLVE_DEST.
	// Without this, the texture remains in COPY_SOURCE and the next resolve
	// barrier (which expects RENDER_TARGET) would be invalid on DX12.
	encoder.TransitionTextures([]wgpu.TextureBarrier{{
		Texture: s.textures.resolveTex,
		Usage: wgpu.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageCopySrc,
			NewUsage: gputypes.TextureUsageRenderAttachment,
		},
	}})

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	encoderConsumed = true

	// Submit (auto-polls pending maps at tail).
	if _, err := s.queue.Submit(cmdBuf); err != nil {
		return fmt.Errorf("submit: %w", err)
	}

	// Map the staging buffer. Map blocks until the GPU finishes the copy
	// via Device.Poll-driven submission tracking (no manual WaitIdle needed).
	if err := stagingBuf.Map(context.Background(), wgpu.MapModeRead, 0, stagingBufSize); err != nil {
		return fmt.Errorf("map staging: %w", err)
	}
	rng, err := stagingBuf.MappedRange(0, stagingBufSize)
	if err != nil {
		if err := stagingBuf.Unmap(); err != nil {
			slogger().Warn("unmap failed", "err", err)
		}
		return fmt.Errorf("mapped range: %w", err)
	}
	readback := make([]byte, stagingBufSize)
	copy(readback, rng.Bytes())
	if err := stagingBuf.Unmap(); err != nil {
		slogger().Warn("unmap failed", "err", err)
	}

	// Strip row padding (if any) and composite BGRA over existing RGBA pixmap.
	// Compositing (Porter-Duff "over") preserves existing pixmap content where
	// the GPU rendered transparent pixels. This is essential when Flush() is
	// called multiple times per frame (e.g., before each CPU text draw).
	if alignedBytesPerRow == bytesPerRow {
		// No padding — fast path.
		compositeBGRAOverRGBA(readback, target.Data, target.Width*target.Height)
	} else {
		// Strip per-row padding from aligned readback data, then composite.
		tight := make([]byte, uint64(bytesPerRow)*uint64(h))
		for row := uint32(0); row < h; row++ {
			srcOff := int(row) * int(alignedBytesPerRow)
			dstOff := int(row) * int(bytesPerRow)
			copy(tight[dstOff:dstOff+int(bytesPerRow)], readback[srcOff:srcOff+int(bytesPerRow)])
		}
		compositeBGRAOverRGBA(tight, target.Data, target.Width*target.Height)
	}
	return nil
}

// encodeSubmitSurface encodes the unified render pass with the given view
// as the resolve target, then submits without readback. The MSAA color
// attachment resolves directly to the provided texture view.
//
// This is the zero-copy path for windowed rendering: no staging buffer, no
// CopyTextureToBuffer, no ReadBuffer, no fence wait (presentation handles
// synchronization).
func (s *GPURenderSession) encodeSubmitSurface(
	view *wgpu.TextureView,
	w, h uint32,
	sdfRes *sdfFrameResources,
	sdfShapes []SDFRenderShape,
	convexRes *convexFrameResources,
	stencilRes []*stencilCoverBuffers,
	stencilPaths []StencilPathCommand,
	textRes *textFrameResources,
	glyphMaskRes *glyphMaskFrameResources,
) error {
	encoder, err := s.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "session_surface_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	// BUG-GG-ENCODER-LIFECYCLE-001: defer-based safety net ensures the encoder
	// is always finalized even if a panic or unexpected error path is hit.
	// DiscardEncoding is idempotent (no-op if already released by Finish).
	encoderConsumed := false
	defer func() {
		if !encoderConsumed {
			encoder.DiscardEncoding()
		}
	}()

	// Per-view frame tracking: when the view changes between flushes
	// (e.g., two gg.Context instances rendering to different targets),
	// reset frameRendered so the new view gets LoadOpClear on its first
	// pass. This prevents shapes from one Context leaking into another.
	if view != s.lastView {
		s.frameRendered = false
		s.lastView = view
	}

	// Surface render pass LoadOp: first pass clears, subsequent passes
	// preserve existing content. This handles mid-frame flushes caused by
	// CPU fallback operations (e.g., DrawImage between GPU draw calls).
	// Without this, each flush would wipe previously rendered shapes.
	colorLoadOp := gputypes.LoadOpClear
	stencilLoadOp := gputypes.LoadOpClear
	if s.frameRendered {
		colorLoadOp = gputypes.LoadOpLoad
		stencilLoadOp = gputypes.LoadOpLoad
	}
	// Depth is ALWAYS cleared — never loaded. DepthStoreOp=Discard means
	// depth content is undefined after each render pass, so LoadOpLoad would
	// read garbage on subsequent passes. The depth clip pipeline (GPU-CLIP-003a)
	// writes Z=0.0 inside clip regions fresh each pass, so clearing to 1.0 is
	// always the correct initial state.
	depthLoadOp := gputypes.LoadOpClear

	rpDesc := &wgpu.RenderPassDescriptor{
		Label: "session_surface_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: view,
			LoadOp:        colorLoadOp,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       depthLoadOp,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     stencilLoadOp,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	}

	rp, rpErr := encoder.BeginRenderPass(rpDesc)
	if rpErr != nil {
		return fmt.Errorf("begin render pass: %w", rpErr)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)
	s.applyScissorRect(rp)

	// Clip bind group is passed to each RecordDraws (must be bound AFTER
	// SetPipeline due to Vulkan pipeline layout requirement).
	clipBG := s.noClipBindGroup

	// Tier 1: SDF shapes (no stencil interaction).
	if sdfRes != nil && len(sdfShapes) > 0 {
		s.sdfPipeline.RecordDraws(rp, sdfRes, clipBG)
	}

	// Tier 2a: Convex polygon fast-path (no stencil interaction).
	if convexRes != nil {
		s.convexRenderer.RecordDraws(rp, convexRes, clipBG)
	}

	// Tier 2b: Stencil-then-cover paths.
	for i, bufs := range stencilRes {
		s.stencilRenderer.RecordPath(rp, bufs, stencilPaths[i].FillRule, clipBG)
	}

	// Tier 4: MSDF text (rendered after shapes).
	if textRes != nil && len(textRes.drawCalls) > 0 {
		s.textPipeline.RecordDraws(rp, textRes, clipBG)
	}

	// Tier 6: Glyph mask text (rendered last, on top of all other geometry).
	if glyphMaskRes != nil && len(glyphMaskRes.drawCalls) > 0 {
		s.glyphMaskPipeline.RecordDraws(rp, glyphMaskRes, clipBG)
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("render pass End failed", "err", endErr)
	}

	// No CopyTextureToBuffer -- the surface is the resolve target.
	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	encoderConsumed = true

	// Submit the command buffer. Do NOT free any previous command buffers
	// here — multiple FlushGPUWithView calls per frame each produce a
	// command buffer. Freeing one mid-frame would vkResetCommandPool on a
	// pool whose command buffer is still in-flight (undefined behavior,
	// manifests as trail artifacts from incomplete MSAA resolve).
	// All command buffers are freed at the start of the NEXT frame
	// (BeginFrame) when VSync guarantees the GPU is done.
	if _, err := s.queue.Submit(cmdBuf); err != nil {
		// BUG-GG-ENCODER-LIFECYCLE-001: free the command buffer that was not
		// submitted. Without this, the Vulkan command pool entry leaks.
		s.device.FreeCommandBuffer(cmdBuf)
		return fmt.Errorf("submit: %w", err)
	}

	// Keep reference so next frame can free it after GPU is done.
	s.prevCmdBufs = append(s.prevCmdBufs, cmdBuf)

	// Mark that at least one render pass has been submitted this frame.
	// Subsequent mid-frame flushes will use LoadOpLoad to preserve content.
	s.frameRendered = true

	return nil
}

// recordGroupDraws records all tier draw commands for a single group into
// the given render pass encoder. This is the inner loop of the grouped
// encode methods — called once per scissor group within a single render pass.
func (s *GPURenderSession) recordGroupDraws(rp *wgpu.RenderPassEncoder, gr *groupResources) {
	// Clip bind group is passed to each RecordDraws so it is bound at @group(1)
	// AFTER SetPipeline and BEFORE Draw. Vulkan requires a valid pipeline
	// layout when calling vkCmdBindDescriptorSets.
	clipBG := gr.clipBindGroup

	// GPU-CLIP-003a: record depth clip draw BEFORE all content.
	// This populates the depth buffer with Z=0.0 where the clip geometry
	// exists. Content pipelines then use DepthCompare=GreaterEqual to
	// restrict rendering to the clipped region.
	if gr.hasDepthClip && gr.depthClipRes != nil {
		s.depthClipPipeline.RecordDraw(rp, gr.depthClipRes)
	}

	// Tier 1: SDF shapes (no stencil interaction).
	if gr.sdfRes != nil && len(gr.sdfShapes) > 0 {
		s.sdfPipeline.RecordDraws(rp, gr.sdfRes, clipBG, gr.hasDepthClip)
	}

	// Tier 2a: Convex polygon fast-path (no stencil interaction).
	if gr.convexRes != nil {
		s.convexRenderer.RecordDraws(rp, gr.convexRes, clipBG, gr.hasDepthClip)
	}

	// Tier 2b: Stencil-then-cover paths.
	for i, bufs := range gr.stencilRes {
		s.stencilRenderer.RecordPath(rp, bufs, gr.stencilPaths[i].FillRule, clipBG, gr.hasDepthClip)
	}

	// Tier 3: Textured quad images (CPU-uploaded).
	if gr.imageRes != nil && len(gr.imageRes.drawCalls) > 0 {
		s.imagePipeline.RecordDraws(rp, gr.imageRes, clipBG, gr.hasDepthClip)
	}

	// Tier 3b: GPU texture compositing (pre-existing GPU texture, zero upload).
	if gr.gpuTexRes != nil && len(gr.gpuTexRes.drawCalls) > 0 {
		s.imagePipeline.RecordDraws(rp, gr.gpuTexRes, clipBG, gr.hasDepthClip)
	}

	// Tier 4: MSDF text (rendered after shapes).
	if gr.textRes != nil && len(gr.textRes.drawCalls) > 0 {
		s.textPipeline.RecordDraws(rp, gr.textRes, clipBG, gr.hasDepthClip)
	}

	// Tier 6: Glyph mask text (rendered last, on top of all other geometry).
	if gr.glyphMaskRes != nil && len(gr.glyphMaskRes.drawCalls) > 0 {
		s.glyphMaskPipeline.RecordDraws(rp, gr.glyphMaskRes, clipBG, gr.hasDepthClip)
	}
}

// applyGroupScissor sets or clears the scissor rect on the render pass for
// a given group. When rect is nil, the scissor is reset to the full
// framebuffer (w x h). When non-nil, the scissor clips to the given rect.
func (s *GPURenderSession) applyGroupScissor(rp *wgpu.RenderPassEncoder, rect *[4]uint32, w, h uint32) {
	if rect != nil {
		rp.SetScissorRect(rect[0], rect[1], rect[2], rect[3])
	} else {
		rp.SetScissorRect(0, 0, w, h)
	}
}

// sliceConvexResources creates a convexFrameResources referencing a sub-range
// of the combined vertex buffer. Convex commands have variable vertex counts,
// so firstVertex is computed by summing vertices of all commands before cmdStart.
func (s *GPURenderSession) sliceConvexResources(
	combined *convexFrameResources,
	allCommands []ConvexDrawCommand,
	cmdStart, cmdCount int,
) *convexFrameResources {
	firstVertex := convexVertexCount(allCommands[:cmdStart])
	vertCount := convexVertexCount(allCommands[cmdStart : cmdStart+cmdCount])
	if vertCount == 0 {
		return nil
	}
	return &convexFrameResources{
		vertBuf:     combined.vertBuf,
		uniformBuf:  combined.uniformBuf,
		bindGroup:   combined.bindGroup,
		firstVertex: firstVertex,
		vertCount:   vertCount,
	}
}

// sliceTextResources creates a textFrameResources referencing a sub-range of
// the combined drawCalls. Empty batches produce no drawCall, so the slice
// boundaries are computed by counting non-empty batches.
func (s *GPURenderSession) sliceTextResources(
	combined *textFrameResources,
	allBatches []TextBatch,
	batchStart, batchCount int,
) *textFrameResources {
	dcStart := 0
	for _, b := range allBatches[:batchStart] {
		if len(b.Quads) > 0 {
			dcStart++
		}
	}
	dcCount := 0
	for _, b := range allBatches[batchStart : batchStart+batchCount] {
		if len(b.Quads) > 0 {
			dcCount++
		}
	}
	if dcCount == 0 {
		return nil
	}
	return &textFrameResources{
		vertBuf:   combined.vertBuf,
		idxBuf:    combined.idxBuf,
		drawCalls: combined.drawCalls[dcStart : dcStart+dcCount],
	}
}

// sliceGlyphMaskResources creates a glyphMaskFrameResources referencing a
// sub-range of the combined drawCalls. Same counting logic as text slicing.
func (s *GPURenderSession) sliceGlyphMaskResources(
	combined *glyphMaskFrameResources,
	allBatches []GlyphMaskBatch,
	batchStart, batchCount int,
) *glyphMaskFrameResources {
	dcStart := 0
	for _, b := range allBatches[:batchStart] {
		if len(b.Quads) > 0 {
			dcStart++
		}
	}
	dcCount := 0
	for _, b := range allBatches[batchStart : batchStart+batchCount] {
		if len(b.Quads) > 0 {
			dcCount++
		}
	}
	if dcCount == 0 {
		return nil
	}
	return &glyphMaskFrameResources{
		vertBuf:   combined.vertBuf,
		idxBuf:    combined.idxBuf,
		drawCalls: combined.drawCalls[dcStart : dcStart+dcCount],
		isLCD:     combined.isLCD,
	}
}

// encodeSubmitReadbackGrouped encodes a single render pass with per-group
// scissor state changes, then copies the resolve texture to a staging buffer
// for CPU readback. This is the grouped version of encodeSubmitReadback.
func (s *GPURenderSession) encodeSubmitReadbackGrouped(
	w, h uint32,
	grpRes []groupResources,
	target gg.GPURenderTarget,
	baseLayerRes *imageFrameResources,
) error {
	encoder, err := s.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "session_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	// BUG-GG-ENCODER-LIFECYCLE-001: defer-based safety net ensures the encoder
	// is always finalized even if a panic or unexpected error path is hit.
	// DiscardEncoding is idempotent (no-op if already released by Finish).
	encoderConsumed := false
	defer func() {
		if !encoderConsumed {
			encoder.DiscardEncoding()
		}
	}()

	// Unified render pass descriptor with MSAA color + stencil + resolve.
	rpDesc := &wgpu.RenderPassDescriptor{
		Label: "session_unified_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: s.textures.resolveView,
			LoadOp:        gputypes.LoadOpClear,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       gputypes.LoadOpClear,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     gputypes.LoadOpClear,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	}

	rp, rpErr := encoder.BeginRenderPass(rpDesc)
	if rpErr != nil {
		return fmt.Errorf("begin render pass: %w", rpErr)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)

	// Base layer: pixmap textured quad drawn FIRST, before all tiers (ADR-015).
	if baseLayerRes != nil && len(baseLayerRes.drawCalls) > 0 {
		rp.SetScissorRect(0, 0, w, h)
		s.imagePipeline.RecordDraws(rp, baseLayerRes, s.noClipBindGroup)
	}

	// Render each group with its scissor rect applied.
	for i := range grpRes {
		s.applyGroupScissor(rp, grpRes[i].scissorRect, w, h)
		s.recordGroupDraws(rp, &grpRes[i])
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("render pass End failed", "err", endErr)
	}

	// VK-LAYOUT-001: After MSAA resolve the texture is in
	// COLOR_ATTACHMENT_OPTIMAL layout. CopyTextureToBuffer requires
	// TRANSFER_SRC_OPTIMAL. Insert an explicit barrier to transition.
	encoder.TransitionTextures([]wgpu.TextureBarrier{{
		Texture: s.textures.resolveTex,
		Usage: wgpu.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

	// Mark encoder as consumed before handing to copySubmitAndReadback,
	// which calls Finish() internally.
	encoderConsumed = true

	// Encode copy and submit, then read back pixels to the target.
	return s.copySubmitAndReadback(encoder, w, h, target)
}

// isBlitOnly returns true when the frame contains only textured quads (base
// layer + overlay GPU textures) with zero vector shapes that need MSAA.
func (s *GPURenderSession) isBlitOnly(grpRes []groupResources, baseLayerRes *imageFrameResources) bool {
	if baseLayerRes == nil || len(baseLayerRes.drawCalls) == 0 {
		return false
	}
	for i := range grpRes {
		gr := &grpRes[i]
		if (gr.sdfRes != nil && gr.sdfRes.vertCount > 0) ||
			(gr.convexRes != nil && gr.convexRes.vertCount > 0) ||
			len(gr.stencilRes) > 0 ||
			(gr.imageRes != nil && len(gr.imageRes.drawCalls) > 0) ||
			(gr.textRes != nil && len(gr.textRes.drawCalls) > 0) ||
			(gr.glyphMaskRes != nil && len(gr.glyphMaskRes.drawCalls) > 0) {
			return false
		}
		// gpuTexRes (GPU texture overlays from RepaintBoundary) are textured
		// quads — same shader as base layer, no MSAA needed. Allow them in
		// the blit-only fast path.
	}
	return true
}

// encodeBlitOnlyPass renders textured quads directly to the swapchain surface
// in a non-MSAA (1x) render pass. No MSAA texture, no depth/stencil, no resolve.
// This is the compositor fast path (ADR-016) — 93% bandwidth reduction vs 4x MSAA.
func (s *GPURenderSession) encodeBlitOnlyPass(
	view *wgpu.TextureView, w, h uint32,
	grpRes []groupResources,
	baseLayerRes *imageFrameResources,
	damageRect image.Rectangle,
) error {
	if err := s.imagePipeline.ensureBlitPipeline(); err != nil {
		return fmt.Errorf("ensure blit pipeline: %w", err)
	}

	encoder, err := s.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "session_blit_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	encoderConsumed := false
	defer func() {
		if !encoderConsumed {
			encoder.DiscardEncoding()
		}
	}()

	loadOp := gputypes.LoadOpClear
	if !damageRect.Empty() {
		loadOp = gputypes.LoadOpLoad
	}

	rp, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "session_blit_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     loadOp,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		return fmt.Errorf("begin blit render pass: %w", err)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)

	if !damageRect.Empty() {
		dx := uint32(max(0, damageRect.Min.X)) //nolint:gosec // clamped
		dy := uint32(max(0, damageRect.Min.Y)) //nolint:gosec // clamped
		dw := uint32(damageRect.Dx())          //nolint:gosec // positive by Empty check
		dh := uint32(damageRect.Dy())          //nolint:gosec // positive by Empty check
		rp.SetScissorRect(dx, dy, dw, dh)
	}

	s.imagePipeline.RecordBlitDraws(rp, baseLayerRes)

	// Draw GPU texture overlays (e.g., RepaintBoundary cached textures).
	// These are textured quads using the same blit pipeline — no MSAA needed.
	for i := range grpRes {
		gr := &grpRes[i]
		if gr.gpuTexRes != nil && len(gr.gpuTexRes.drawCalls) > 0 {
			s.imagePipeline.RecordBlitDraws(rp, gr.gpuTexRes)
		}
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("blit render pass End failed", "err", endErr)
	}

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("end blit encoding: %w", err)
	}
	encoderConsumed = true

	// Do NOT free previous command buffers mid-frame — see encodeSubmitSurface.
	if _, submitErr := s.queue.Submit(cmdBuf); submitErr != nil {
		s.device.FreeCommandBuffer(cmdBuf)
		return fmt.Errorf("submit blit: %w", submitErr)
	}
	s.prevCmdBufs = append(s.prevCmdBufs, cmdBuf)
	s.frameRendered = true
	s.lastView = view

	return nil
}

// encodeSubmitSurfaceGrouped encodes a single render pass with per-group
// scissor state changes, resolving directly to the given view. No readback
// occurs. This is the grouped version of encodeSubmitSurface.
func (s *GPURenderSession) encodeSubmitSurfaceGrouped(
	view *wgpu.TextureView,
	w, h uint32,
	grpRes []groupResources,
	baseLayerRes *imageFrameResources,
) error {
	encoder, err := s.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "session_surface_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	// BUG-GG-ENCODER-LIFECYCLE-001: defer-based safety net ensures the encoder
	// is always finalized even if a panic or unexpected error path is hit.
	// DiscardEncoding is idempotent (no-op if already released by Finish).
	encoderConsumed := false
	defer func() {
		if !encoderConsumed {
			encoder.DiscardEncoding()
		}
	}()

	// Per-view frame tracking (same as encodeSubmitSurface).
	if view != s.lastView {
		s.frameRendered = false
		s.lastView = view
	}

	// Surface render pass LoadOp: first pass clears, subsequent passes
	// preserve existing content. This handles mid-frame flushes caused by
	// CPU fallback operations (e.g., DrawImage between GPU draw calls).
	colorLoadOp := gputypes.LoadOpClear
	stencilLoadOp := gputypes.LoadOpClear
	if s.frameRendered {
		colorLoadOp = gputypes.LoadOpLoad
		stencilLoadOp = gputypes.LoadOpLoad
	}
	// Depth is ALWAYS cleared — never loaded. DepthStoreOp=Discard means
	// depth content is undefined after each render pass, so LoadOpLoad would
	// read garbage on subsequent passes. The depth clip pipeline (GPU-CLIP-003a)
	// writes Z=0.0 inside clip regions fresh each pass, so clearing to 1.0 is
	// always the correct initial state.
	depthLoadOp := gputypes.LoadOpClear

	rpDesc := &wgpu.RenderPassDescriptor{
		Label: "session_surface_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: view,
			LoadOp:        colorLoadOp,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       depthLoadOp,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     stencilLoadOp,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	}

	rp, rpErr := encoder.BeginRenderPass(rpDesc)
	if rpErr != nil {
		return fmt.Errorf("begin render pass: %w", rpErr)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)

	// Base layer: pixmap textured quad drawn FIRST, before all tiers (ADR-015).
	if baseLayerRes != nil && len(baseLayerRes.drawCalls) > 0 {
		rp.SetScissorRect(0, 0, w, h)
		s.imagePipeline.RecordDraws(rp, baseLayerRes, s.noClipBindGroup)
	}

	// Render each group with its scissor rect applied.
	for i := range grpRes {
		s.applyGroupScissor(rp, grpRes[i].scissorRect, w, h)
		s.recordGroupDraws(rp, &grpRes[i])
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("render pass End failed", "err", endErr)
	}

	// No CopyTextureToBuffer -- the surface is the resolve target.
	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	encoderConsumed = true

	// Do NOT free previous command buffers mid-frame — see encodeSubmitSurface.
	if _, err := s.queue.Submit(cmdBuf); err != nil {
		// BUG-GG-ENCODER-LIFECYCLE-001: free the command buffer that was not
		// submitted. Without this, the Vulkan command pool entry leaks.
		s.device.FreeCommandBuffer(cmdBuf)
		return fmt.Errorf("submit: %w", err)
	}

	// Keep reference so next frame can free it after GPU is done.
	s.prevCmdBufs = append(s.prevCmdBufs, cmdBuf)

	// Mark that at least one render pass has been submitted this frame.
	s.frameRendered = true

	return nil
}

// encodeToEncoder records a surface render pass into an external command encoder
// WITHOUT creating the encoder or submitting. The caller is responsible for
// encoder.Finish() + queue.Submit(). Used for single-command-buffer frames
// where multiple contexts share one encoder (ADR-017, Flutter Impeller pattern).
func (s *GPURenderSession) encodeToEncoder(
	encoder *wgpu.CommandEncoder,
	view *wgpu.TextureView,
	w, h uint32,
	grpRes []groupResources,
	baseLayerRes *imageFrameResources,
) error {
	if view != s.lastView {
		s.frameRendered = false
		s.lastView = view
	}

	colorLoadOp := gputypes.LoadOpClear
	stencilLoadOp := gputypes.LoadOpClear
	if s.frameRendered {
		colorLoadOp = gputypes.LoadOpLoad
		stencilLoadOp = gputypes.LoadOpLoad
	}
	// Depth always cleared (see encodeSubmitSurfaceGrouped comment).
	depthLoadOp := gputypes.LoadOpClear

	rp, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "session_shared_surface_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: view,
			LoadOp:        colorLoadOp,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       depthLoadOp,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     stencilLoadOp,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	})
	if err != nil {
		return fmt.Errorf("begin shared render pass: %w", err)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)

	if baseLayerRes != nil && len(baseLayerRes.drawCalls) > 0 {
		rp.SetScissorRect(0, 0, w, h)
		s.imagePipeline.RecordDraws(rp, baseLayerRes, s.noClipBindGroup)
	}

	for i := range grpRes {
		s.applyGroupScissor(rp, grpRes[i].scissorRect, w, h)
		s.recordGroupDraws(rp, &grpRes[i])
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("shared render pass End failed", "err", endErr)
	}

	s.frameRendered = true
	return nil
}

// encodeBlitToEncoder records a non-MSAA blit pass into an external encoder.
// Same as encodeBlitOnlyPass but without encoder creation or submit.
func (s *GPURenderSession) encodeBlitToEncoder(
	encoder *wgpu.CommandEncoder,
	view *wgpu.TextureView,
	w, h uint32,
	grpRes []groupResources,
	baseLayerRes *imageFrameResources,
	damageRect image.Rectangle,
) error {
	if err := s.imagePipeline.ensureBlitPipeline(); err != nil {
		return fmt.Errorf("ensure blit pipeline: %w", err)
	}

	loadOp := gputypes.LoadOpClear
	if !damageRect.Empty() {
		loadOp = gputypes.LoadOpLoad
	}

	rp, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "session_shared_blit_pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     loadOp,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		return fmt.Errorf("begin shared blit pass: %w", err)
	}
	rp.SetViewport(0, 0, float32(w), float32(h), 0, 1)

	if !damageRect.Empty() {
		dx := uint32(max(0, damageRect.Min.X)) //nolint:gosec // clamped
		dy := uint32(max(0, damageRect.Min.Y)) //nolint:gosec // clamped
		dw := uint32(damageRect.Dx())          //nolint:gosec // positive
		dh := uint32(damageRect.Dy())          //nolint:gosec // positive
		rp.SetScissorRect(dx, dy, dw, dh)
	}

	s.imagePipeline.RecordBlitDraws(rp, baseLayerRes)

	for i := range grpRes {
		gr := &grpRes[i]
		if gr.gpuTexRes != nil && len(gr.gpuTexRes.drawCalls) > 0 {
			s.imagePipeline.RecordBlitDraws(rp, gr.gpuTexRes)
		}
	}

	if endErr := rp.End(); endErr != nil {
		slogger().Warn("shared blit pass End failed", "err", endErr)
	}

	s.frameRendered = true
	return nil
}

// SDFPipeline returns the SDF render pipeline.
func (s *GPURenderSession) SDFPipeline() *SDFRenderPipeline {
	return s.sdfPipeline
}

// StencilRendererRef returns the stencil renderer.
func (s *GPURenderSession) StencilRendererRef() *StencilRenderer {
	return s.stencilRenderer
}

// SetSDFPipeline sets an external SDF pipeline for the session to use.
// The session does not own the pipeline and will not destroy it.
func (s *GPURenderSession) SetSDFPipeline(p *SDFRenderPipeline) {
	s.sdfPipeline = p
}

// ConvexRendererRef returns the convex renderer.
func (s *GPURenderSession) ConvexRendererRef() *ConvexRenderer {
	return s.convexRenderer
}

// SetConvexRenderer sets an external convex renderer for the session to use.
// The session does not own the renderer and will not destroy it.
func (s *GPURenderSession) SetConvexRenderer(r *ConvexRenderer) {
	s.convexRenderer = r
}

// SetStencilRenderer sets an external stencil renderer for the session to use.
// The session does not own the renderer and will not destroy it.
func (s *GPURenderSession) SetStencilRenderer(r *StencilRenderer) {
	s.stencilRenderer = r
}
