//go:build !nogpu

package gpu

import (
	"fmt"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

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
	device hal.Device
	queue  hal.Queue

	// Shared textures (MSAA 4x color + depth/stencil + 1x resolve).
	textures textureSet

	// Pipeline owners (lazily created). The session does not own these
	// pipelines -- it holds references and delegates draw recording to them.
	sdfPipeline     *SDFRenderPipeline
	convexRenderer  *ConvexRenderer
	stencilRenderer *StencilRenderer
	textPipeline    *MSDFTextPipeline

	// Surface rendering mode fields. When surfaceView is non-nil, the session
	// renders directly to the surface instead of reading back to CPU.
	surfaceView   hal.TextureView
	surfaceWidth  uint32
	surfaceHeight uint32

	// Persistent per-frame GPU buffers (survive across frames).
	// Grow-only: reallocated only when data exceeds current capacity.
	sdfVertBuf       hal.Buffer
	sdfVertBufCap    uint64
	sdfUniformBuf    hal.Buffer
	sdfBindGroup     hal.BindGroup
	convexVertBuf    hal.Buffer
	convexVertBufCap uint64
	convexUniformBuf hal.Buffer
	convexBindGroup  hal.BindGroup

	// Tier 4: MSDF text persistent buffers.
	textVertBuf    hal.Buffer
	textVertBufCap uint64
	textIdxBuf     hal.Buffer
	textIdxBufCap  uint64
	textUniformBuf hal.Buffer
	textBindGroup  hal.BindGroup
	// Atlas texture and view for current frame's text rendering.
	textAtlasTex  hal.Texture
	textAtlasView hal.TextureView

	// Stencil buffers are per-path, so we keep a pool of reusable buffer sets.
	stencilBufPool []*stencilCoverBuffers

	// Pre-allocated CPU staging slices for vertex data generation.
	sdfVertexStaging    []byte
	convexVertexStaging []byte

	// In-flight command buffer from the previous surface frame. Freed at
	// the start of the next frame, when VSync guarantees the GPU is done.
	prevCmdBuf hal.CommandBuffer
}

// NewGPURenderSession creates a new render session with the given device and
// queue. Textures and pipelines are not allocated until RenderFrame is called.
func NewGPURenderSession(device hal.Device, queue hal.Queue) *GPURenderSession {
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
func (s *GPURenderSession) SetSurfaceTarget(view hal.TextureView, width, height uint32) {
	// If switching modes or resizing, invalidate cached textures so they
	// are recreated on the next RenderFrame call.
	modeChanged := (view == nil) != (s.surfaceView == nil)
	sizeChanged := width != s.surfaceWidth || height != s.surfaceHeight
	if modeChanged || sizeChanged {
		// Drain the GPU before destroying textures — an in-flight command
		// buffer may still reference framebuffers built from these views.
		if s.prevCmdBuf != nil {
			s.drainQueue()
			s.device.FreeCommandBuffer(s.prevCmdBuf)
			s.prevCmdBuf = nil
		}
		s.textures.destroyTextures(s.device)
	}
	s.surfaceView = view
	s.surfaceWidth = width
	s.surfaceHeight = height
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
// stencil paths + MSDF text) in a single render pass. This is the main
// entry point for unified rendering.
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
) error {
	if len(sdfShapes) == 0 && len(convexCommands) == 0 && len(stencilPaths) == 0 && len(textBatches) == 0 {
		return nil
	}

	w, h := uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions always fit uint32
	if err := s.EnsureTextures(w, h); err != nil {
		return fmt.Errorf("ensure textures: %w", err)
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

	var textRes *textFrameResources
	if len(textBatches) > 0 {
		var buildErr error
		textRes, buildErr = s.buildTextResources(textBatches)
		if buildErr != nil {
			return fmt.Errorf("build text resources: %w", buildErr)
		}
	}

	if s.surfaceView != nil {
		return s.encodeSubmitSurface(w, h, sdfResources, sdfShapes, convexRes, stencilResources, stencilPaths, textRes)
	}
	return s.encodeSubmitReadback(w, h, sdfResources, sdfShapes, convexRes, stencilResources, stencilPaths, textRes, target)
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
	// Submit a no-op command buffer with a fence — the queue is FIFO,
	// so when the fence signals, all prior submissions are complete.
	if s.prevCmdBuf != nil {
		s.drainQueue()
		s.device.FreeCommandBuffer(s.prevCmdBuf)
		s.prevCmdBuf = nil
	}
	s.destroyPersistentBuffers()
	s.textures.destroyTextures(s.device)
	s.surfaceView = nil
	s.surfaceWidth = 0
	s.surfaceHeight = 0
	// Do not destroy pipelines -- they are owned by the caller.
}

// drainQueue submits a no-op command buffer with a fence and waits for it.
// Since the GPU queue is FIFO, this guarantees all prior submissions are done.
func (s *GPURenderSession) drainQueue() {
	fence, err := s.device.CreateFence()
	if err != nil {
		return
	}
	defer s.device.DestroyFence(fence)

	encoder, err := s.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "session_drain",
	})
	if err != nil {
		return
	}
	if err := encoder.BeginEncoding("drain"); err != nil {
		return
	}
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return
	}
	defer s.device.FreeCommandBuffer(cmdBuf)

	_ = s.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1)
	_, _ = s.device.Wait(fence, 1, 5*time.Second)
}

func (s *GPURenderSession) destroyPersistentBuffers() {
	if s.sdfBindGroup != nil {
		s.device.DestroyBindGroup(s.sdfBindGroup)
		s.sdfBindGroup = nil
	}
	if s.sdfUniformBuf != nil {
		s.device.DestroyBuffer(s.sdfUniformBuf)
		s.sdfUniformBuf = nil
	}
	if s.sdfVertBuf != nil {
		s.device.DestroyBuffer(s.sdfVertBuf)
		s.sdfVertBuf = nil
		s.sdfVertBufCap = 0
	}
	if s.convexBindGroup != nil {
		s.device.DestroyBindGroup(s.convexBindGroup)
		s.convexBindGroup = nil
	}
	if s.convexUniformBuf != nil {
		s.device.DestroyBuffer(s.convexUniformBuf)
		s.convexUniformBuf = nil
	}
	if s.convexVertBuf != nil {
		s.device.DestroyBuffer(s.convexVertBuf)
		s.convexVertBuf = nil
		s.convexVertBufCap = 0
	}
	// Tier 4: Text buffers.
	if s.textBindGroup != nil {
		s.device.DestroyBindGroup(s.textBindGroup)
		s.textBindGroup = nil
	}
	if s.textUniformBuf != nil {
		s.device.DestroyBuffer(s.textUniformBuf)
		s.textUniformBuf = nil
	}
	if s.textIdxBuf != nil {
		s.device.DestroyBuffer(s.textIdxBuf)
		s.textIdxBuf = nil
		s.textIdxBufCap = 0
	}
	if s.textVertBuf != nil {
		s.device.DestroyBuffer(s.textVertBuf)
		s.textVertBuf = nil
		s.textVertBufCap = 0
	}
	if s.textAtlasView != nil {
		s.device.DestroyTextureView(s.textAtlasView)
		s.textAtlasView = nil
	}
	if s.textAtlasTex != nil {
		s.device.DestroyTexture(s.textAtlasTex)
		s.textAtlasTex = nil
	}
	for _, b := range s.stencilBufPool {
		b.destroy(s.device)
	}
	s.stencilBufPool = s.stencilBufPool[:0]
}

// sdfFrameResources holds per-frame GPU resources for SDF rendering.
type sdfFrameResources struct {
	vertBuf    hal.Buffer
	uniformBuf hal.Buffer
	bindGroup  hal.BindGroup
	vertCount  uint32
}

// ensurePipelines creates SDF, convex, stencil, and text pipelines if they
// don't exist yet. Pipelines are lazily created on first use.
func (s *GPURenderSession) ensurePipelines() error {
	if s.sdfPipeline == nil {
		s.sdfPipeline = NewSDFRenderPipeline(s.device, s.queue)
	}
	if err := s.sdfPipeline.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("SDF pipeline: %w", err)
	}

	if s.convexRenderer == nil {
		s.convexRenderer = NewConvexRenderer(s.device, s.queue)
	}
	if err := s.convexRenderer.ensurePipelineWithStencil(); err != nil {
		return fmt.Errorf("convex pipeline: %w", err)
	}

	if s.stencilRenderer == nil {
		s.stencilRenderer = NewStencilRenderer(s.device, s.queue)
	}
	if s.stencilRenderer.nonZeroStencilPipeline == nil {
		if err := s.stencilRenderer.createPipelines(); err != nil {
			return fmt.Errorf("stencil pipelines: %w", err)
		}
	}

	if s.textPipeline == nil {
		s.textPipeline = NewMSDFTextPipeline(s.device, s.queue)
	}
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
			s.device.DestroyBindGroup(s.sdfBindGroup)
			s.sdfBindGroup = nil
		}
		if s.sdfVertBuf != nil {
			s.device.DestroyBuffer(s.sdfVertBuf)
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
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
	s.queue.WriteBuffer(s.sdfVertBuf, 0, vertexData)

	uniformData := makeSDFRenderUniform(w, h)
	if s.sdfUniformBuf == nil {
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
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
			s.device.DestroyBindGroup(s.sdfBindGroup)
			s.sdfBindGroup = nil
		}
	}
	s.queue.WriteBuffer(s.sdfUniformBuf, 0, uniformData)

	if s.sdfBindGroup == nil {
		bg, err := s.device.CreateBindGroup(&hal.BindGroupDescriptor{
			Label:  "session_sdf_bind",
			Layout: s.sdfPipeline.uniformLayout,
			Entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{
					Buffer: s.sdfUniformBuf.NativeHandle(), Offset: 0, Size: sdfRenderUniformSize,
				}},
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
			s.device.DestroyBindGroup(s.convexBindGroup)
			s.convexBindGroup = nil
		}
		if s.convexVertBuf != nil {
			s.device.DestroyBuffer(s.convexVertBuf)
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
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
	s.queue.WriteBuffer(s.convexVertBuf, 0, vertexData)

	uniformData := makeSDFRenderUniform(w, h) // Same 16-byte viewport layout.
	if s.convexUniformBuf == nil {
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
			Label: "session_convex_uniform",
			Size:  sdfRenderUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create convex uniform buffer: %w", err)
		}
		s.convexUniformBuf = buf
		if s.convexBindGroup != nil {
			s.device.DestroyBindGroup(s.convexBindGroup)
			s.convexBindGroup = nil
		}
	}
	s.queue.WriteBuffer(s.convexUniformBuf, 0, uniformData)

	if s.convexBindGroup == nil {
		bg, err := s.device.CreateBindGroup(&hal.BindGroupDescriptor{
			Label:  "session_convex_bind",
			Layout: s.convexRenderer.uniformLayout,
			Entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{
					Buffer: s.convexUniformBuf.NativeHandle(), Offset: 0, Size: sdfRenderUniformSize,
				}},
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
			s.stencilBufPool[i].destroy(s.device)
			s.stencilBufPool[i] = nil
		}
		bufs, err := s.stencilRenderer.createRenderBuffers(w, h, cmd.Vertices, cmd.CoverQuad, color)
		if err != nil {
			// Clean up buffers created in this batch.
			for j := 0; j < i; j++ {
				if s.stencilBufPool[j] != nil {
					s.stencilBufPool[j].destroy(s.device)
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

// buildTextResources updates persistent vertex, index, and uniform buffers
// for MSDF text rendering. Currently handles a single atlas (first batch's
// atlas). Buffers are grow-only: reallocated only when data exceeds current
// capacity.
//
// The bind group is recreated every frame because the atlas texture or
// uniform content may change. In a future optimization, bind groups could
// be cached per atlas texture identity.
func (s *GPURenderSession) buildTextResources(batches []TextBatch) (*textFrameResources, error) {
	if len(batches) == 0 {
		return nil, nil //nolint:nilnil // empty batch list is a valid no-op, not an error
	}

	// Aggregate all quads from all batches into one draw call.
	// For simplicity, use the first batch's color/transform/atlas params.
	// Multi-batch rendering with different colors will be added later.
	var allQuads []TextQuad
	for i := range batches {
		allQuads = append(allQuads, batches[i].Quads...)
	}
	if len(allQuads) == 0 {
		return nil, nil //nolint:nilnil // no quads to render
	}

	batch := batches[0]

	// Build vertex data (4 vertices per quad, 16 bytes per vertex).
	vertexData := buildTextVertexData(allQuads)
	vertSize := uint64(len(vertexData))

	if s.textVertBuf == nil || s.textVertBufCap < vertSize {
		if s.textBindGroup != nil {
			s.device.DestroyBindGroup(s.textBindGroup)
			s.textBindGroup = nil
		}
		if s.textVertBuf != nil {
			s.device.DestroyBuffer(s.textVertBuf)
		}
		allocSize := vertSize * 2
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
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
	s.queue.WriteBuffer(s.textVertBuf, 0, vertexData)

	// Build index data (6 indices per quad, 2 bytes per index).
	indexData := buildTextIndexData(len(allQuads))
	idxSize := uint64(len(indexData))

	if s.textIdxBuf == nil || s.textIdxBufCap < idxSize {
		if s.textBindGroup != nil {
			s.device.DestroyBindGroup(s.textBindGroup)
			s.textBindGroup = nil
		}
		if s.textIdxBuf != nil {
			s.device.DestroyBuffer(s.textIdxBuf)
		}
		allocSize := idxSize * 2
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
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
	s.queue.WriteBuffer(s.textIdxBuf, 0, indexData)

	// Build uniform data (96 bytes).
	uniformData := makeTextUniform(batch.Color, batch.Transform, batch.PxRange, batch.AtlasSize)
	if s.textUniformBuf == nil {
		buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
			Label: "session_text_uniform",
			Size:  textUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create text uniform buffer: %w", err)
		}
		s.textUniformBuf = buf
	}
	s.queue.WriteBuffer(s.textUniformBuf, 0, uniformData)

	// Recreate bind group every frame (atlas texture or uniform content may
	// change between frames). The bind group layout is owned by textPipeline.
	if s.textBindGroup != nil {
		s.device.DestroyBindGroup(s.textBindGroup)
		s.textBindGroup = nil
	}

	// The bind group requires an atlas texture view. If no atlas view is
	// available yet, skip text rendering this frame.
	if s.textAtlasView == nil {
		return nil, nil //nolint:nilnil // no atlas uploaded yet
	}

	bg, err := s.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "session_text_bind",
		Layout: s.textPipeline.uniformLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: s.textUniformBuf.NativeHandle(), Offset: 0, Size: textUniformSize,
			}},
			{Binding: 1, Resource: gputypes.TextureViewBinding{
				TextureView: s.textAtlasView.NativeHandle(),
			}},
			{Binding: 2, Resource: gputypes.SamplerBinding{
				Sampler: s.textPipeline.sampler.NativeHandle(),
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create text bind group: %w", err)
	}
	s.textBindGroup = bg

	indexCount := uint32(len(allQuads) * 6) //nolint:gosec // quad count bounded by MaxQuadCapacity
	return &textFrameResources{
		vertBuf:    s.textVertBuf,
		idxBuf:     s.textIdxBuf,
		uniformBuf: s.textUniformBuf,
		bindGroup:  s.textBindGroup,
		indexCount: indexCount,
	}, nil
}

// SetTextAtlas sets the atlas texture and view for MSDF text rendering.
// The session takes ownership of both the texture and view and will destroy
// them when the session is destroyed or when a new atlas is set.
//
// Call this after uploading atlas data to the GPU (e.g., from
// TextRenderer.SyncAtlases). The atlas view is used in the text bind group.
func (s *GPURenderSession) SetTextAtlas(tex hal.Texture, view hal.TextureView) {
	if s.textAtlasView != nil {
		s.device.DestroyTextureView(s.textAtlasView)
	}
	if s.textAtlasTex != nil {
		s.device.DestroyTexture(s.textAtlasTex)
	}
	s.textAtlasTex = tex
	s.textAtlasView = view
	// Invalidate bind group -- it references the old texture view.
	if s.textBindGroup != nil {
		s.device.DestroyBindGroup(s.textBindGroup)
		s.textBindGroup = nil
	}
}

// TextPipelineRef returns the MSDF text pipeline. It is lazily created by
// ensurePipelines, so may be nil before RenderFrame is called.
func (s *GPURenderSession) TextPipelineRef() *MSDFTextPipeline {
	return s.textPipeline
}

// SetTextPipeline sets an external MSDF text pipeline for the session to use.
// The session does not own the pipeline and will not destroy it.
func (s *GPURenderSession) SetTextPipeline(p *MSDFTextPipeline) {
	s.textPipeline = p
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
	target gg.GPURenderTarget,
) error {
	encoder, err := s.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "session_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	if err := encoder.BeginEncoding("session_frame"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	// Unified render pass descriptor with MSAA color + stencil + resolve.
	rpDesc := &hal.RenderPassDescriptor{
		Label: "session_unified_pass",
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: s.textures.resolveView,
			LoadOp:        gputypes.LoadOpClear,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       gputypes.LoadOpClear,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     gputypes.LoadOpClear,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	}

	rp := encoder.BeginRenderPass(rpDesc)

	// Tier 1: SDF shapes (no stencil interaction).
	if sdfRes != nil && len(sdfShapes) > 0 {
		s.sdfPipeline.RecordDraws(rp, sdfRes)
	}

	// Tier 2a: Convex polygon fast-path (no stencil interaction).
	if convexRes != nil {
		s.convexRenderer.RecordDraws(rp, convexRes)
	}

	// Tier 2b: Stencil-then-cover paths.
	for i, bufs := range stencilRes {
		s.stencilRenderer.RecordPath(rp, bufs, stencilPaths[i].FillRule)
	}

	// Tier 4: MSDF text (rendered last, on top of all other geometry).
	if textRes != nil && textRes.indexCount > 0 {
		s.textPipeline.RecordDraws(rp, textRes)
	}

	rp.End()

	// VK-LAYOUT-001: After MSAA resolve the texture is in
	// COLOR_ATTACHMENT_OPTIMAL layout. CopyTextureToBuffer requires
	// TRANSFER_SRC_OPTIMAL. Insert an explicit barrier to transition.
	// This is a no-op on Metal, GLES, software, and noop backends.
	encoder.TransitionTextures([]hal.TextureBarrier{{
		Texture: s.textures.resolveTex,
		Usage: hal.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

	// Encode copy and submit, then read back pixels to the target.
	return s.copySubmitAndReadback(encoder, w, h, target)
}

// copySubmitAndReadback creates a staging buffer, encodes the texture-to-buffer
// copy, submits the command buffer, waits for the GPU, and reads back pixels
// into the render target. This is the second half of encodeSubmitReadback,
// extracted for readability.
func (s *GPURenderSession) copySubmitAndReadback(
	encoder hal.CommandEncoder, w, h uint32, target gg.GPURenderTarget,
) error {
	// Copy resolve texture to staging buffer for CPU readback.
	// WebGPU (and DX12) requires BytesPerRow aligned to 256 bytes.
	bytesPerRow := w * 4
	const copyPitchAlignment = 256
	alignedBytesPerRow := (bytesPerRow + copyPitchAlignment - 1) &^ (copyPitchAlignment - 1)
	stagingBufSize := uint64(alignedBytesPerRow) * uint64(h)

	stagingBuf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "session_staging",
		Size:  stagingBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		encoder.DiscardEncoding()
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer s.device.DestroyBuffer(stagingBuf)

	encoder.CopyTextureToBuffer(s.textures.resolveTex, stagingBuf, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{Offset: 0, BytesPerRow: alignedBytesPerRow, RowsPerImage: h},
		TextureBase:  hal.ImageCopyTexture{Texture: s.textures.resolveTex, MipLevel: 0},
		Size:         hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	}})

	// Transition resolve texture back to RenderAttachment so the next frame's
	// render pass End() can transition from RENDER_TARGET → RESOLVE_DEST.
	// Without this, the texture remains in COPY_SOURCE and the next resolve
	// barrier (which expects RENDER_TARGET) would be invalid on DX12.
	encoder.TransitionTextures([]hal.TextureBarrier{{
		Texture: s.textures.resolveTex,
		Usage: hal.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageCopySrc,
			NewUsage: gputypes.TextureUsageRenderAttachment,
		},
	}})

	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	defer s.device.FreeCommandBuffer(cmdBuf)

	// Submit and wait.
	fence, err := s.device.CreateFence()
	if err != nil {
		return fmt.Errorf("create fence: %w", err)
	}
	defer s.device.DestroyFence(fence)

	if err := s.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fenceOK, err := s.device.Wait(fence, 1, 5*time.Second)
	if err != nil || !fenceOK {
		return fmt.Errorf("wait for GPU: ok=%v err=%w", fenceOK, err)
	}

	readback := make([]byte, stagingBufSize)
	if err := s.queue.ReadBuffer(stagingBuf, 0, readback); err != nil {
		return fmt.Errorf("readback: %w", err)
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

// encodeSubmitSurface encodes the unified render pass with the surface view
// as the resolve target, then submits without readback. The MSAA color
// attachment resolves directly to the caller-provided surface texture view.
//
// This is the zero-copy path for windowed rendering: no staging buffer, no
// CopyTextureToBuffer, no ReadBuffer, no fence wait (presentation handles
// synchronization).
func (s *GPURenderSession) encodeSubmitSurface(
	_, _ uint32,
	sdfRes *sdfFrameResources,
	sdfShapes []SDFRenderShape,
	convexRes *convexFrameResources,
	stencilRes []*stencilCoverBuffers,
	stencilPaths []StencilPathCommand,
	textRes *textFrameResources,
) error {
	encoder, err := s.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "session_surface_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	if err := encoder.BeginEncoding("session_surface_frame"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	// Unified render pass: MSAA color resolves to surface view (zero-copy).
	rpDesc := &hal.RenderPassDescriptor{
		Label: "session_surface_pass",
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View:          s.textures.msaaView,
			ResolveTarget: s.surfaceView,
			LoadOp:        gputypes.LoadOpClear,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
		}},
		DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
			View:              s.textures.stencilView,
			DepthLoadOp:       gputypes.LoadOpClear,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     gputypes.LoadOpClear,
			StencilStoreOp:    gputypes.StoreOpStore,
			StencilClearValue: 0,
		},
	}

	rp := encoder.BeginRenderPass(rpDesc)

	// Tier 1: SDF shapes (no stencil interaction).
	if sdfRes != nil && len(sdfShapes) > 0 {
		s.sdfPipeline.RecordDraws(rp, sdfRes)
	}

	// Tier 2a: Convex polygon fast-path (no stencil interaction).
	if convexRes != nil {
		s.convexRenderer.RecordDraws(rp, convexRes)
	}

	// Tier 2b: Stencil-then-cover paths.
	for i, bufs := range stencilRes {
		s.stencilRenderer.RecordPath(rp, bufs, stencilPaths[i].FillRule)
	}

	// Tier 4: MSDF text (rendered last, on top of all other geometry).
	if textRes != nil && textRes.indexCount > 0 {
		s.textPipeline.RecordDraws(rp, textRes)
	}

	rp.End()

	// No CopyTextureToBuffer -- the surface is the resolve target.
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}

	// Free the previous frame's command buffer. By now VSync has
	// guaranteed the GPU finished with it.
	if s.prevCmdBuf != nil {
		s.device.FreeCommandBuffer(s.prevCmdBuf)
	}

	// Submit without fence -- presentation handles GPU synchronization.
	if err := s.queue.Submit([]hal.CommandBuffer{cmdBuf}, nil, 0); err != nil {
		s.prevCmdBuf = nil
		return fmt.Errorf("submit: %w", err)
	}

	// Keep reference so next frame can free it after GPU is done.
	s.prevCmdBuf = cmdBuf

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
