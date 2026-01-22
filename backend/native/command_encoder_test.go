package native

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/types"
)

// =============================================================================
// CoreCommandEncoder Tests
// =============================================================================

func TestCoreCommandEncoder_CreateFromBackend(t *testing.T) {
	tests := []struct {
		name          string
		backend       *NativeBackend
		label         string
		wantErr       bool
		wantErrTarget error
	}{
		{
			name:    "success with label",
			backend: &NativeBackend{initialized: true},
			label:   "test-encoder",
			wantErr: false,
		},
		{
			name:    "success without label",
			backend: &NativeBackend{initialized: true},
			label:   "",
			wantErr: false,
		},
		{
			name:          "fail when not initialized",
			backend:       &NativeBackend{initialized: false},
			label:         "test",
			wantErr:       true,
			wantErrTarget: ErrNotInitialized,
		},
		{
			name:          "fail with nil backend",
			backend:       nil,
			label:         "test",
			wantErr:       true,
			wantErrTarget: ErrNilDevice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewCoreCommandEncoder(tt.backend, tt.label)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrTarget != nil && !errors.Is(err, tt.wantErrTarget) {
					t.Errorf("expected error %v, got %v", tt.wantErrTarget, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if enc == nil {
				t.Fatal("expected encoder, got nil")
			}

			if enc.Label() != tt.label {
				t.Errorf("label = %q, want %q", enc.Label(), tt.label)
			}
		})
	}
}

func TestCoreCommandEncoder_Label(t *testing.T) {
	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		want    string
	}{
		{
			name:    "nil encoder",
			encoder: nil,
			want:    "",
		},
		{
			name:    "encoder with label",
			encoder: &CoreCommandEncoder{label: "my-encoder"},
			want:    "my-encoder",
		},
		{
			name:    "encoder without label",
			encoder: &CoreCommandEncoder{label: ""},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.encoder.Label()
			if got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCoreCommandEncoder_Status(t *testing.T) {
	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		want    core.CommandEncoderStatus
	}{
		{
			name:    "nil encoder",
			encoder: nil,
			want:    core.CommandEncoderStatusError,
		},
		{
			name:    "recording state",
			encoder: &CoreCommandEncoder{},
			want:    core.CommandEncoderStatusRecording,
		},
		{
			name: "locked state (render pass)",
			encoder: &CoreCommandEncoder{
				activeRenderPass: &RenderPassEncoder{},
			},
			want: core.CommandEncoderStatusLocked,
		},
		{
			name: "locked state (compute pass)",
			encoder: &CoreCommandEncoder{
				activeComputePass: &ComputePassEncoder{},
			},
			want: core.CommandEncoderStatusLocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.encoder.Status()
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoreCommandEncoder_BeginRenderPass(t *testing.T) {
	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		desc    *RenderPassDescriptor
		wantErr bool
	}{
		{
			name: "success with descriptor",
			encoder: &CoreCommandEncoder{
				label: "test",
			},
			desc: &RenderPassDescriptor{
				Label: "render-pass",
				ColorAttachments: []RenderPassColorAttachment{
					{
						LoadOp:     types.LoadOpClear,
						StoreOp:    types.StoreOpStore,
						ClearValue: types.Color{R: 0, G: 0, B: 0, A: 1},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "nil descriptor",
			encoder: &CoreCommandEncoder{
				label: "test",
			},
			desc:    nil,
			wantErr: true,
		},
		{
			name: "encoder locked",
			encoder: &CoreCommandEncoder{
				label:            "test",
				activeRenderPass: &RenderPassEncoder{},
			},
			desc: &RenderPassDescriptor{
				Label: "render-pass",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass, err := tt.encoder.BeginRenderPass(tt.desc)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if pass == nil {
				t.Fatal("expected pass encoder, got nil")
			}

			// Verify encoder is now locked
			if tt.encoder.Status() != core.CommandEncoderStatusLocked {
				t.Errorf("encoder status = %v, want Locked", tt.encoder.Status())
			}

			// End the pass and verify encoder returns to recording
			if err := pass.End(); err != nil {
				t.Errorf("End() error: %v", err)
			}

			if tt.encoder.Status() != core.CommandEncoderStatusRecording {
				t.Errorf("encoder status after End() = %v, want Recording", tt.encoder.Status())
			}
		})
	}
}

func TestCoreCommandEncoder_BeginComputePass(t *testing.T) {
	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		desc    *ComputePassDescriptor
		wantErr bool
	}{
		{
			name: "success with descriptor",
			encoder: &CoreCommandEncoder{
				label: "test",
			},
			desc: &ComputePassDescriptor{
				Label: "compute-pass",
			},
			wantErr: false,
		},
		{
			name: "success with nil descriptor",
			encoder: &CoreCommandEncoder{
				label: "test",
			},
			desc:    nil,
			wantErr: false,
		},
		{
			name: "encoder locked",
			encoder: &CoreCommandEncoder{
				label:             "test",
				activeComputePass: &ComputePassEncoder{},
			},
			desc: &ComputePassDescriptor{
				Label: "compute-pass",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass, err := tt.encoder.BeginComputePass(tt.desc)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if pass == nil {
				t.Fatal("expected pass encoder, got nil")
			}

			// Verify encoder is now locked
			if tt.encoder.Status() != core.CommandEncoderStatusLocked {
				t.Errorf("encoder status = %v, want Locked", tt.encoder.Status())
			}

			// End the pass
			if err := pass.End(); err != nil {
				t.Errorf("End() error: %v", err)
			}

			if tt.encoder.Status() != core.CommandEncoderStatusRecording {
				t.Errorf("encoder status after End() = %v, want Recording", tt.encoder.Status())
			}
		})
	}
}

func TestCoreCommandEncoder_CopyBufferToBuffer(t *testing.T) {
	// Create mock buffers for testing
	srcBuffer := &core.Buffer{}
	dstBuffer := &core.Buffer{}

	tests := []struct {
		name          string
		encoder       *CoreCommandEncoder
		src           *core.Buffer
		dst           *core.Buffer
		srcOffset     uint64
		dstOffset     uint64
		size          uint64
		wantErr       bool
		wantErrTarget error
	}{
		{
			name:      "nil source buffer",
			encoder:   &CoreCommandEncoder{},
			src:       nil,
			dst:       dstBuffer,
			srcOffset: 0,
			dstOffset: 0,
			size:      64,
			wantErr:   true,
		},
		{
			name:      "nil destination buffer",
			encoder:   &CoreCommandEncoder{},
			src:       srcBuffer,
			dst:       nil,
			srcOffset: 0,
			dstOffset: 0,
			size:      64,
			wantErr:   true,
		},
		{
			name:          "unaligned source offset",
			encoder:       &CoreCommandEncoder{},
			src:           srcBuffer,
			dst:           dstBuffer,
			srcOffset:     3,
			dstOffset:     0,
			size:          64,
			wantErr:       true,
			wantErrTarget: ErrCopyOffsetNotAligned,
		},
		{
			name:          "unaligned destination offset",
			encoder:       &CoreCommandEncoder{},
			src:           srcBuffer,
			dst:           dstBuffer,
			srcOffset:     0,
			dstOffset:     1,
			size:          64,
			wantErr:       true,
			wantErrTarget: ErrCopyOffsetNotAligned,
		},
		{
			name:          "unaligned size",
			encoder:       &CoreCommandEncoder{},
			src:           srcBuffer,
			dst:           dstBuffer,
			srcOffset:     0,
			dstOffset:     0,
			size:          63,
			wantErr:       true,
			wantErrTarget: ErrCopySizeNotAligned,
		},
		{
			name: "encoder locked",
			encoder: &CoreCommandEncoder{
				activeRenderPass: &RenderPassEncoder{},
			},
			src:       srcBuffer,
			dst:       dstBuffer,
			srcOffset: 0,
			dstOffset: 0,
			size:      64,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.encoder.CopyBufferToBuffer(tt.src, tt.dst, tt.srcOffset, tt.dstOffset, tt.size)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrTarget != nil && !errors.Is(err, tt.wantErrTarget) {
					t.Errorf("expected error containing %v, got %v", tt.wantErrTarget, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCoreCommandEncoder_CopyBufferToTexture(t *testing.T) {
	texture := &GPUTexture{}
	buffer := &core.Buffer{}

	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		source  *ImageCopyBuffer
		dest    *ImageCopyTexture
		size    types.Extent3D
		wantErr bool
	}{
		{
			name:    "nil source",
			encoder: &CoreCommandEncoder{},
			source:  nil,
			dest: &ImageCopyTexture{
				Texture: texture,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name:    "nil destination",
			encoder: &CoreCommandEncoder{},
			source: &ImageCopyBuffer{
				Buffer: buffer,
			},
			dest:    nil,
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name: "encoder locked",
			encoder: &CoreCommandEncoder{
				activeRenderPass: &RenderPassEncoder{},
			},
			source: &ImageCopyBuffer{
				Buffer: buffer,
			},
			dest: &ImageCopyTexture{
				Texture: texture,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.encoder.CopyBufferToTexture(tt.source, tt.dest, tt.size)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCoreCommandEncoder_CopyTextureToBuffer(t *testing.T) {
	texture := &GPUTexture{}
	buffer := &core.Buffer{}

	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		source  *ImageCopyTexture
		dest    *ImageCopyBuffer
		size    types.Extent3D
		wantErr bool
	}{
		{
			name:    "nil source",
			encoder: &CoreCommandEncoder{},
			source:  nil,
			dest: &ImageCopyBuffer{
				Buffer: buffer,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name:    "nil destination",
			encoder: &CoreCommandEncoder{},
			source: &ImageCopyTexture{
				Texture: texture,
			},
			dest:    nil,
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.encoder.CopyTextureToBuffer(tt.source, tt.dest, tt.size)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCoreCommandEncoder_CopyTextureToTexture(t *testing.T) {
	texture1 := &GPUTexture{}
	texture2 := &GPUTexture{}

	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		source  *ImageCopyTexture
		dest    *ImageCopyTexture
		size    types.Extent3D
		wantErr bool
	}{
		{
			name:    "nil source",
			encoder: &CoreCommandEncoder{},
			source:  nil,
			dest: &ImageCopyTexture{
				Texture: texture2,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name:    "nil destination",
			encoder: &CoreCommandEncoder{},
			source: &ImageCopyTexture{
				Texture: texture1,
			},
			dest:    nil,
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.encoder.CopyTextureToTexture(tt.source, tt.dest, tt.size)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCoreCommandEncoder_ClearBuffer(t *testing.T) {
	buffer := &core.Buffer{}

	tests := []struct {
		name          string
		encoder       *CoreCommandEncoder
		buffer        *core.Buffer
		offset        uint64
		size          uint64
		wantErr       bool
		wantErrTarget error
	}{
		{
			name:    "nil buffer",
			encoder: &CoreCommandEncoder{},
			buffer:  nil,
			offset:  0,
			size:    64,
			wantErr: true,
		},
		{
			name:          "unaligned offset",
			encoder:       &CoreCommandEncoder{},
			buffer:        buffer,
			offset:        3,
			size:          64,
			wantErr:       true,
			wantErrTarget: ErrCopyOffsetNotAligned,
		},
		{
			name: "encoder locked",
			encoder: &CoreCommandEncoder{
				activeComputePass: &ComputePassEncoder{},
			},
			buffer:  buffer,
			offset:  0,
			size:    64,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.encoder.ClearBuffer(tt.buffer, tt.offset, tt.size)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrTarget != nil && !errors.Is(err, tt.wantErrTarget) {
					t.Errorf("expected error containing %v, got %v", tt.wantErrTarget, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCoreCommandEncoder_Finish(t *testing.T) {
	tests := []struct {
		name    string
		encoder *CoreCommandEncoder
		wantErr bool
	}{
		{
			name:    "success",
			encoder: &CoreCommandEncoder{label: "test"},
			wantErr: false,
		},
		{
			name: "encoder locked",
			encoder: &CoreCommandEncoder{
				label:            "test",
				activeRenderPass: &RenderPassEncoder{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdBuffer, err := tt.encoder.Finish()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cmdBuffer == nil {
				t.Fatal("expected command buffer, got nil")
			}

			if cmdBuffer.Label() != tt.encoder.label {
				t.Errorf("command buffer label = %q, want %q", cmdBuffer.Label(), tt.encoder.label)
			}
		})
	}
}

// =============================================================================
// RenderPassEncoder Basic Tests
// Note: Comprehensive tests are in hal_render_pass_test.go
// =============================================================================

func TestRenderPassEncoder_IntegrationWithEncoder(t *testing.T) {
	t.Run("normal end clears active pass", func(t *testing.T) {
		encoder := &CoreCommandEncoder{label: "test"}
		pass := &RenderPassEncoder{encoder: encoder, state: RenderPassStateRecording}
		encoder.activeRenderPass = pass

		err := pass.End()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pass.state != RenderPassStateEnded {
			t.Error("pass.state should be Ended")
		}

		if encoder.activeRenderPass != nil {
			t.Error("encoder.activeRenderPass should be nil")
		}
	})
}

// =============================================================================
// ComputePassEncoder Basic Tests
// Note: Comprehensive tests are in hal_compute_pass_test.go
// =============================================================================

func TestComputePassEncoder_IntegrationWithEncoder(t *testing.T) {
	t.Run("normal end clears active pass", func(t *testing.T) {
		encoder := &CoreCommandEncoder{label: "test"}
		pass := &ComputePassEncoder{encoder: encoder, state: ComputePassStateRecording}
		encoder.activeComputePass = pass

		err := pass.End()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pass.state != ComputePassStateEnded {
			t.Error("pass.state should be Ended")
		}

		if encoder.activeComputePass != nil {
			t.Error("encoder.activeComputePass should be nil")
		}
	})
}

// =============================================================================
// CoreCommandBuffer Tests
// =============================================================================

func TestCoreCommandBuffer_Label(t *testing.T) {
	tests := []struct {
		name string
		cb   *CoreCommandBuffer
		want string
	}{
		{
			name: "nil buffer",
			cb:   nil,
			want: "",
		},
		{
			name: "buffer with label",
			cb:   &CoreCommandBuffer{label: "test-buffer"},
			want: "test-buffer",
		},
		{
			name: "buffer without label",
			cb:   &CoreCommandBuffer{label: ""},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cb.Label()
			if got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCoreCommandBuffer_CoreBuffer(t *testing.T) {
	t.Run("nil command buffer", func(t *testing.T) {
		var cb *CoreCommandBuffer
		if cb.CoreBuffer() != nil {
			t.Error("CoreBuffer() should return nil for nil buffer")
		}
	})

	t.Run("buffer without core buffer", func(t *testing.T) {
		cb := &CoreCommandBuffer{label: "test"}
		if cb.CoreBuffer() != nil {
			t.Error("CoreBuffer() should return nil when coreBuffer is nil")
		}
	})
}

// =============================================================================
// RenderPassDescriptor Tests
// =============================================================================

func TestRenderPassDescriptor_toCoreDescriptor(t *testing.T) {
	t.Run("nil descriptor", func(t *testing.T) {
		var desc *RenderPassDescriptor
		coreDesc := desc.toCoreDescriptor()
		if coreDesc != nil {
			t.Error("expected nil for nil descriptor")
		}
	})

	t.Run("descriptor with color attachment", func(t *testing.T) {
		desc := &RenderPassDescriptor{
			Label: "test-pass",
			ColorAttachments: []RenderPassColorAttachment{
				{
					LoadOp:     types.LoadOpClear,
					StoreOp:    types.StoreOpStore,
					ClearValue: types.Color{R: 1, G: 0, B: 0, A: 1},
				},
			},
		}

		coreDesc := desc.toCoreDescriptor()
		if coreDesc == nil {
			t.Fatal("expected non-nil core descriptor")
		}

		if coreDesc.Label != "test-pass" {
			t.Errorf("label = %q, want %q", coreDesc.Label, "test-pass")
		}

		if len(coreDesc.ColorAttachments) != 1 {
			t.Errorf("color attachments = %d, want 1", len(coreDesc.ColorAttachments))
		}

		if coreDesc.ColorAttachments[0].LoadOp != types.LoadOpClear {
			t.Errorf("load op = %v, want Clear", coreDesc.ColorAttachments[0].LoadOp)
		}
	})

	t.Run("descriptor with depth stencil", func(t *testing.T) {
		desc := &RenderPassDescriptor{
			Label: "test-pass",
			DepthStencilAttachment: &RenderPassDepthStencilAttachment{
				DepthLoadOp:       types.LoadOpClear,
				DepthStoreOp:      types.StoreOpStore,
				DepthClearValue:   1.0,
				DepthReadOnly:     false,
				StencilLoadOp:     types.LoadOpClear,
				StencilStoreOp:    types.StoreOpDiscard,
				StencilClearValue: 0,
				StencilReadOnly:   true,
			},
		}

		coreDesc := desc.toCoreDescriptor()
		if coreDesc == nil {
			t.Fatal("expected non-nil core descriptor")
		}

		if coreDesc.DepthStencilAttachment == nil {
			t.Fatal("expected non-nil depth stencil attachment")
		}

		ds := coreDesc.DepthStencilAttachment
		if ds.DepthClearValue != 1.0 {
			t.Errorf("depth clear value = %f, want 1.0", ds.DepthClearValue)
		}
		if !ds.StencilReadOnly {
			t.Error("stencil read only should be true")
		}
	})
}

// =============================================================================
// Error Tests
// =============================================================================

func TestCommandEncoderErrors(t *testing.T) {
	t.Run("error constants are distinct", func(t *testing.T) {
		errList := []error{
			ErrEncoderNotRecording,
			ErrEncoderLocked,
			ErrEncoderFinished,
			ErrEncoderConsumed,
			ErrNilDevice,
			ErrNilEncoder,
			ErrNilCoreBuffer,
			ErrCopyRangeOutOfBounds,
			ErrCopyOverlap,
			ErrCopyOffsetNotAligned,
			ErrCopySizeNotAligned,
		}

		seen := make(map[string]bool)
		for _, err := range errList {
			msg := err.Error()
			if seen[msg] {
				t.Errorf("duplicate error message: %q", msg)
			}
			seen[msg] = true
		}
	})
}

// =============================================================================
// Integration Tests (Workflow)
// =============================================================================

func TestCoreCommandEncoder_RenderWorkflow(t *testing.T) {
	backend := &NativeBackend{initialized: true}

	// Create encoder
	encoder, err := NewCoreCommandEncoder(backend, "render-workflow")
	if err != nil {
		t.Fatalf("NewCoreCommandEncoder failed: %v", err)
	}

	// Verify initial state
	if encoder.Status() != core.CommandEncoderStatusRecording {
		t.Errorf("initial status = %v, want Recording", encoder.Status())
	}

	// Begin render pass
	pass, err := encoder.BeginRenderPass(&RenderPassDescriptor{
		Label: "main-pass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				LoadOp:     types.LoadOpClear,
				StoreOp:    types.StoreOpStore,
				ClearValue: types.Color{R: 0, G: 0, B: 0, A: 1},
			},
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}

	// Verify locked state
	if encoder.Status() != core.CommandEncoderStatusLocked {
		t.Errorf("status after begin pass = %v, want Locked", encoder.Status())
	}

	// Record some commands
	pass.SetViewport(0, 0, 800, 600, 0, 1)
	pass.SetScissorRect(0, 0, 800, 600)
	pass.Draw(3, 1, 0, 0)

	// End render pass
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify recording state
	if encoder.Status() != core.CommandEncoderStatusRecording {
		t.Errorf("status after end pass = %v, want Recording", encoder.Status())
	}

	// Finish
	cmdBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	if cmdBuffer == nil {
		t.Fatal("expected command buffer, got nil")
	}

	if cmdBuffer.Label() != "render-workflow" {
		t.Errorf("command buffer label = %q, want %q", cmdBuffer.Label(), "render-workflow")
	}
}

func TestCoreCommandEncoder_ComputeWorkflow(t *testing.T) {
	backend := &NativeBackend{initialized: true}

	// Create encoder
	encoder, err := NewCoreCommandEncoder(backend, "compute-workflow")
	if err != nil {
		t.Fatalf("NewCoreCommandEncoder failed: %v", err)
	}

	// Begin compute pass
	pass, err := encoder.BeginComputePass(&ComputePassDescriptor{
		Label: "compute-pass",
	})
	if err != nil {
		t.Fatalf("BeginComputePass failed: %v", err)
	}

	// Verify locked state
	if encoder.Status() != core.CommandEncoderStatusLocked {
		t.Errorf("status after begin pass = %v, want Locked", encoder.Status())
	}

	// Dispatch workgroups
	if err := pass.DispatchWorkgroups(8, 8, 1); err != nil {
		t.Fatalf("DispatchWorkgroups failed: %v", err)
	}

	// End compute pass
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Finish
	cmdBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	if cmdBuffer.Label() != "compute-workflow" {
		t.Errorf("command buffer label = %q, want %q", cmdBuffer.Label(), "compute-workflow")
	}
}
