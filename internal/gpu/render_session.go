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
// convex polygon fast-path rendering (Tier 2a), and stencil-then-cover path
// rendering (Tier 2b) into one GPU submission.
// Enterprise 2D engines (Skia Ganesh/Graphite, Flutter Impeller, Gio) use
// the same pattern: one render pass, multiple pipeline switches.
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
//	  +-- Holds references to SDFRenderPipeline, ConvexRenderer, StencilRenderer
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

	// Surface rendering mode fields. When surfaceView is non-nil, the session
	// renders directly to the surface instead of reading back to CPU.
	surfaceView   hal.TextureView
	surfaceWidth  uint32
	surfaceHeight uint32
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
// stencil paths) in a single render pass. This is the main entry point
// for unified rendering.
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
) error {
	if len(sdfShapes) == 0 && len(convexCommands) == 0 && len(stencilPaths) == 0 {
		return nil
	}

	w, h := uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions always fit uint32
	if err := s.EnsureTextures(w, h); err != nil {
		return fmt.Errorf("ensure textures: %w", err)
	}

	// Ensure pipelines exist.
	if err := s.ensurePipelines(); err != nil {
		return fmt.Errorf("ensure pipelines: %w", err)
	}

	// Build per-frame GPU resources for SDF shapes.
	var sdfResources *sdfFrameResources
	if len(sdfShapes) > 0 {
		var err error
		sdfResources, err = s.buildSDFResources(sdfShapes, w, h)
		if err != nil {
			return fmt.Errorf("build SDF resources: %w", err)
		}
		defer sdfResources.destroy(s.device)
	}

	// Build per-frame GPU resources for convex polygons.
	var convexRes *convexFrameResources
	if len(convexCommands) > 0 {
		var err error
		convexRes, err = s.buildConvexResources(convexCommands, w, h)
		if err != nil {
			return fmt.Errorf("build convex resources: %w", err)
		}
		defer convexRes.destroy(s.device)
	}

	// Build per-frame GPU resources for stencil paths.
	var stencilResources []*stencilCoverBuffers
	if len(stencilPaths) > 0 {
		for i := range stencilPaths {
			bufs, err := s.buildStencilResources(&stencilPaths[i], w, h)
			if err != nil {
				// Clean up already-created buffers.
				for _, b := range stencilResources {
					b.destroy(s.device)
				}
				return fmt.Errorf("build stencil resources for path %d: %w", i, err)
			}
			stencilResources = append(stencilResources, bufs)
		}
		defer func() {
			for _, b := range stencilResources {
				b.destroy(s.device)
			}
		}()
	}

	// Encode, submit, and optionally read back.
	if s.surfaceView != nil {
		return s.encodeSubmitSurface(w, h, sdfResources, sdfShapes, convexRes, stencilResources, stencilPaths)
	}
	return s.encodeSubmitReadback(w, h, sdfResources, sdfShapes, convexRes, stencilResources, stencilPaths, target)
}

// Size returns the current shared texture dimensions.
func (s *GPURenderSession) Size() (uint32, uint32) {
	return s.textures.width, s.textures.height
}

// Destroy releases all GPU resources held by the session. Safe to call
// multiple times or on a session with no allocated resources.
// The surface view is not destroyed -- it is owned by the caller.
func (s *GPURenderSession) Destroy() {
	s.textures.destroyTextures(s.device)
	s.surfaceView = nil
	s.surfaceWidth = 0
	s.surfaceHeight = 0
	// Do not destroy pipelines -- they are owned by the caller.
}

// sdfFrameResources holds per-frame GPU resources for SDF rendering.
type sdfFrameResources struct {
	vertBuf    hal.Buffer
	uniformBuf hal.Buffer
	bindGroup  hal.BindGroup
	vertCount  uint32
}

func (r *sdfFrameResources) destroy(device hal.Device) {
	if r.bindGroup != nil {
		device.DestroyBindGroup(r.bindGroup)
	}
	if r.uniformBuf != nil {
		device.DestroyBuffer(r.uniformBuf)
	}
	if r.vertBuf != nil {
		device.DestroyBuffer(r.vertBuf)
	}
}

// ensurePipelines creates SDF, convex, and stencil pipelines if they don't exist yet.
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

	return nil
}

// buildSDFResources creates per-frame vertex buffer, uniform buffer, and bind
// group for rendering SDF shapes.
func (s *GPURenderSession) buildSDFResources(shapes []SDFRenderShape, w, h uint32) (*sdfFrameResources, error) {
	vertexData := buildSDFRenderVertices(shapes, w, h)
	vertexCount := uint32(len(shapes) * 6) //nolint:gosec // shape count fits uint32

	vertBuf, err := s.createAndUploadBuffer("session_sdf_verts", vertexData,
		gputypes.BufferUsageVertex|gputypes.BufferUsageCopyDst)
	if err != nil {
		return nil, fmt.Errorf("create vertex buffer: %w", err)
	}

	uniformData := makeSDFRenderUniform(w, h)
	uniformBuf, err := s.createAndUploadBuffer("session_sdf_uniform", uniformData,
		gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)
	if err != nil {
		s.device.DestroyBuffer(vertBuf)
		return nil, fmt.Errorf("create uniform buffer: %w", err)
	}

	bindGroup, err := s.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "session_sdf_bind",
		Layout: s.sdfPipeline.uniformLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: uniformBuf.NativeHandle(), Offset: 0, Size: sdfRenderUniformSize,
			}},
		},
	})
	if err != nil {
		s.device.DestroyBuffer(uniformBuf)
		s.device.DestroyBuffer(vertBuf)
		return nil, fmt.Errorf("create bind group: %w", err)
	}

	return &sdfFrameResources{
		vertBuf:    vertBuf,
		uniformBuf: uniformBuf,
		bindGroup:  bindGroup,
		vertCount:  vertexCount,
	}, nil
}

// buildConvexResources creates per-frame vertex buffer, uniform buffer, and
// bind group for rendering convex polygons.
func (s *GPURenderSession) buildConvexResources(commands []ConvexDrawCommand, w, h uint32) (*convexFrameResources, error) {
	vertexData := BuildConvexVertices(commands)
	if len(vertexData) == 0 {
		return nil, nil //nolint:nilnil // empty vertex data is a valid no-op, not an error
	}
	vertexCount := convexVertexCount(commands)

	vertBuf, err := s.createAndUploadBuffer("session_convex_verts", vertexData,
		gputypes.BufferUsageVertex|gputypes.BufferUsageCopyDst)
	if err != nil {
		return nil, fmt.Errorf("create convex vertex buffer: %w", err)
	}

	uniformData := makeSDFRenderUniform(w, h) // Same 16-byte viewport layout.
	uniformBuf, err := s.createAndUploadBuffer("session_convex_uniform", uniformData,
		gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)
	if err != nil {
		s.device.DestroyBuffer(vertBuf)
		return nil, fmt.Errorf("create convex uniform buffer: %w", err)
	}

	bindGroup, err := s.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "session_convex_bind",
		Layout: s.convexRenderer.uniformLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: uniformBuf.NativeHandle(), Offset: 0, Size: sdfRenderUniformSize,
			}},
		},
	})
	if err != nil {
		s.device.DestroyBuffer(uniformBuf)
		s.device.DestroyBuffer(vertBuf)
		return nil, fmt.Errorf("create convex bind group: %w", err)
	}

	return &convexFrameResources{
		vertBuf:    vertBuf,
		uniformBuf: uniformBuf,
		bindGroup:  bindGroup,
		vertCount:  vertexCount,
	}, nil
}

// buildStencilResources creates per-frame GPU buffers for a single stencil
// path command.
func (s *GPURenderSession) buildStencilResources(cmd *StencilPathCommand, w, h uint32) (*stencilCoverBuffers, error) {
	color := gg.RGBA{
		R: float64(cmd.Color[0]),
		G: float64(cmd.Color[1]),
		B: float64(cmd.Color[2]),
		A: float64(cmd.Color[3]),
	}
	return s.stencilRenderer.createRenderBuffers(w, h, cmd.Vertices, cmd.CoverQuad, color)
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

	// Strip row padding (if any) and convert BGRA → RGBA.
	if alignedBytesPerRow == bytesPerRow {
		// No padding — fast path.
		convertBGRAToRGBA(readback, target.Data, target.Width*target.Height)
	} else {
		// Strip per-row padding from aligned readback data, then convert.
		tight := make([]byte, uint64(bytesPerRow)*uint64(h))
		for row := uint32(0); row < h; row++ {
			srcOff := int(row) * int(alignedBytesPerRow)
			dstOff := int(row) * int(bytesPerRow)
			copy(tight[dstOff:dstOff+int(bytesPerRow)], readback[srcOff:srcOff+int(bytesPerRow)])
		}
		convertBGRAToRGBA(tight, target.Data, target.Width*target.Height)
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

	rp.End()

	// No CopyTextureToBuffer -- the surface is the resolve target.
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	defer s.device.FreeCommandBuffer(cmdBuf)

	// Submit without fence wait. For surface rendering, presentation
	// handles GPU synchronization. The caller (gogpu) will Present()
	// the surface after this returns.
	fence, err := s.device.CreateFence()
	if err != nil {
		return fmt.Errorf("create fence: %w", err)
	}
	defer s.device.DestroyFence(fence)

	if err := s.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("submit: %w", err)
	}

	// Wait for GPU to finish rendering before the surface is presented.
	// This ensures the render pass completes before Present() is called.
	fenceOK, err := s.device.Wait(fence, 1, 5*time.Second)
	if err != nil || !fenceOK {
		return fmt.Errorf("wait for GPU: ok=%v err=%w", fenceOK, err)
	}

	return nil
}

// createAndUploadBuffer creates a GPU buffer and uploads data.
func (s *GPURenderSession) createAndUploadBuffer(label string, data []byte, usage gputypes.BufferUsage) (hal.Buffer, error) {
	buf, err := s.device.CreateBuffer(&hal.BufferDescriptor{
		Label: label,
		Size:  uint64(len(data)),
		Usage: usage,
	})
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", label, err)
	}
	s.queue.WriteBuffer(buf, 0, data)
	return buf, nil
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
