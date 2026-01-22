package native

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/types"
)

// =============================================================================
// HALCommandEncoder Tests
// =============================================================================

func TestHALCommandEncoder_CreateFromBackend(t *testing.T) {
	tests := []struct {
		name          string
		backend       *NativeBackend
		label         string
		wantErr       bool
		wantErrTarget error
	}{
		{
			name:        "success with label",
			backend:     &NativeBackend{initialized: true},
			label:       "test-encoder",
			wantErr:     false,
		},
		{
			name:        "success without label",
			backend:     &NativeBackend{initialized: true},
			label:       "",
			wantErr:     false,
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
			enc, err := NewHALCommandEncoder(tt.backend, tt.label)

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

func TestHALCommandEncoder_Label(t *testing.T) {
	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		want    string
	}{
		{
			name:    "nil encoder",
			encoder: nil,
			want:    "",
		},
		{
			name:    "encoder with label",
			encoder: &HALCommandEncoder{label: "my-encoder"},
			want:    "my-encoder",
		},
		{
			name:    "encoder without label",
			encoder: &HALCommandEncoder{label: ""},
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

func TestHALCommandEncoder_Status(t *testing.T) {
	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		want    core.CommandEncoderStatus
	}{
		{
			name:    "nil encoder",
			encoder: nil,
			want:    core.CommandEncoderStatusError,
		},
		{
			name:    "recording state",
			encoder: &HALCommandEncoder{},
			want:    core.CommandEncoderStatusRecording,
		},
		{
			name: "locked state (render pass)",
			encoder: &HALCommandEncoder{
				activeRenderPass: &HALRenderPassEncoder{},
			},
			want: core.CommandEncoderStatusLocked,
		},
		{
			name: "locked state (compute pass)",
			encoder: &HALCommandEncoder{
				activeComputePass: &HALComputePassEncoder{},
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

func TestHALCommandEncoder_BeginRenderPass(t *testing.T) {
	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		desc    *HALRenderPassDescriptor
		wantErr bool
	}{
		{
			name: "success with descriptor",
			encoder: &HALCommandEncoder{
				label: "test",
			},
			desc: &HALRenderPassDescriptor{
				Label: "render-pass",
				ColorAttachments: []HALRenderPassColorAttachment{
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
			encoder: &HALCommandEncoder{
				label: "test",
			},
			desc:    nil,
			wantErr: true,
		},
		{
			name: "encoder locked",
			encoder: &HALCommandEncoder{
				label:            "test",
				activeRenderPass: &HALRenderPassEncoder{},
			},
			desc: &HALRenderPassDescriptor{
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

func TestHALCommandEncoder_BeginComputePass(t *testing.T) {
	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		desc    *HALComputePassDescriptor
		wantErr bool
	}{
		{
			name: "success with descriptor",
			encoder: &HALCommandEncoder{
				label: "test",
			},
			desc: &HALComputePassDescriptor{
				Label: "compute-pass",
			},
			wantErr: false,
		},
		{
			name: "success with nil descriptor",
			encoder: &HALCommandEncoder{
				label: "test",
			},
			desc:    nil,
			wantErr: false,
		},
		{
			name: "encoder locked",
			encoder: &HALCommandEncoder{
				label:             "test",
				activeComputePass: &HALComputePassEncoder{},
			},
			desc: &HALComputePassDescriptor{
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

func TestHALCommandEncoder_CopyBufferToBuffer(t *testing.T) {
	// Create mock buffers for testing
	srcBuffer := &core.Buffer{}
	dstBuffer := &core.Buffer{}

	tests := []struct {
		name          string
		encoder       *HALCommandEncoder
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
			encoder:   &HALCommandEncoder{},
			src:       nil,
			dst:       dstBuffer,
			srcOffset: 0,
			dstOffset: 0,
			size:      64,
			wantErr:   true,
		},
		{
			name:      "nil destination buffer",
			encoder:   &HALCommandEncoder{},
			src:       srcBuffer,
			dst:       nil,
			srcOffset: 0,
			dstOffset: 0,
			size:      64,
			wantErr:   true,
		},
		{
			name:          "unaligned source offset",
			encoder:       &HALCommandEncoder{},
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
			encoder:       &HALCommandEncoder{},
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
			encoder:       &HALCommandEncoder{},
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
			encoder: &HALCommandEncoder{
				activeRenderPass: &HALRenderPassEncoder{},
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

func TestHALCommandEncoder_CopyBufferToTexture(t *testing.T) {
	texture := &GPUTexture{}
	buffer := &core.Buffer{}

	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		source  *HALImageCopyBuffer
		dest    *HALImageCopyTexture
		size    types.Extent3D
		wantErr bool
	}{
		{
			name:    "nil source",
			encoder: &HALCommandEncoder{},
			source:  nil,
			dest: &HALImageCopyTexture{
				Texture: texture,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name:    "nil destination",
			encoder: &HALCommandEncoder{},
			source: &HALImageCopyBuffer{
				Buffer: buffer,
			},
			dest:    nil,
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name: "encoder locked",
			encoder: &HALCommandEncoder{
				activeRenderPass: &HALRenderPassEncoder{},
			},
			source: &HALImageCopyBuffer{
				Buffer: buffer,
			},
			dest: &HALImageCopyTexture{
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

func TestHALCommandEncoder_CopyTextureToBuffer(t *testing.T) {
	texture := &GPUTexture{}
	buffer := &core.Buffer{}

	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		source  *HALImageCopyTexture
		dest    *HALImageCopyBuffer
		size    types.Extent3D
		wantErr bool
	}{
		{
			name:    "nil source",
			encoder: &HALCommandEncoder{},
			source:  nil,
			dest: &HALImageCopyBuffer{
				Buffer: buffer,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name:    "nil destination",
			encoder: &HALCommandEncoder{},
			source: &HALImageCopyTexture{
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

func TestHALCommandEncoder_CopyTextureToTexture(t *testing.T) {
	texture1 := &GPUTexture{}
	texture2 := &GPUTexture{}

	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		source  *HALImageCopyTexture
		dest    *HALImageCopyTexture
		size    types.Extent3D
		wantErr bool
	}{
		{
			name:    "nil source",
			encoder: &HALCommandEncoder{},
			source:  nil,
			dest: &HALImageCopyTexture{
				Texture: texture2,
			},
			size:    types.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
			wantErr: true,
		},
		{
			name:    "nil destination",
			encoder: &HALCommandEncoder{},
			source: &HALImageCopyTexture{
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

func TestHALCommandEncoder_ClearBuffer(t *testing.T) {
	buffer := &core.Buffer{}

	tests := []struct {
		name          string
		encoder       *HALCommandEncoder
		buffer        *core.Buffer
		offset        uint64
		size          uint64
		wantErr       bool
		wantErrTarget error
	}{
		{
			name:    "nil buffer",
			encoder: &HALCommandEncoder{},
			buffer:  nil,
			offset:  0,
			size:    64,
			wantErr: true,
		},
		{
			name:          "unaligned offset",
			encoder:       &HALCommandEncoder{},
			buffer:        buffer,
			offset:        3,
			size:          64,
			wantErr:       true,
			wantErrTarget: ErrCopyOffsetNotAligned,
		},
		{
			name: "encoder locked",
			encoder: &HALCommandEncoder{
				activeComputePass: &HALComputePassEncoder{},
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

func TestHALCommandEncoder_Finish(t *testing.T) {
	tests := []struct {
		name    string
		encoder *HALCommandEncoder
		wantErr bool
	}{
		{
			name:    "success",
			encoder: &HALCommandEncoder{label: "test"},
			wantErr: false,
		},
		{
			name: "encoder locked",
			encoder: &HALCommandEncoder{
				label:            "test",
				activeRenderPass: &HALRenderPassEncoder{},
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
// HALRenderPassEncoder Tests
// =============================================================================

func TestHALRenderPassEncoder_End(t *testing.T) {
	t.Run("normal end", func(t *testing.T) {
		encoder := &HALCommandEncoder{label: "test"}
		pass := &HALRenderPassEncoder{encoder: encoder}
		encoder.activeRenderPass = pass

		err := pass.End()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !pass.ended {
			t.Error("pass.ended should be true")
		}

		if encoder.activeRenderPass != nil {
			t.Error("encoder.activeRenderPass should be nil")
		}
	})

	t.Run("double end is no-op", func(t *testing.T) {
		encoder := &HALCommandEncoder{label: "test"}
		pass := &HALRenderPassEncoder{encoder: encoder, ended: true}

		err := pass.End()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestHALRenderPassEncoder_Draw(t *testing.T) {
	t.Run("draw with ended pass", func(t *testing.T) {
		pass := &HALRenderPassEncoder{ended: true}

		// Should not panic, just silently return
		pass.Draw(3, 1, 0, 0)
	})

	t.Run("draw without core pass", func(t *testing.T) {
		pass := &HALRenderPassEncoder{ended: false}

		// Should not panic, just silently return
		pass.Draw(3, 1, 0, 0)
	})
}

func TestHALRenderPassEncoder_DrawIndexed(t *testing.T) {
	t.Run("draw indexed with ended pass", func(t *testing.T) {
		pass := &HALRenderPassEncoder{ended: true}

		// Should not panic
		pass.DrawIndexed(6, 1, 0, 0, 0)
	})
}

func TestHALRenderPassEncoder_SetViewport(t *testing.T) {
	t.Run("set viewport with ended pass", func(t *testing.T) {
		pass := &HALRenderPassEncoder{ended: true}

		// Should not panic
		pass.SetViewport(0, 0, 800, 600, 0, 1)
	})
}

func TestHALRenderPassEncoder_SetScissorRect(t *testing.T) {
	t.Run("set scissor with ended pass", func(t *testing.T) {
		pass := &HALRenderPassEncoder{ended: true}

		// Should not panic
		pass.SetScissorRect(0, 0, 800, 600)
	})
}

// =============================================================================
// HALComputePassEncoder Tests
// =============================================================================

func TestHALComputePassEncoder_End(t *testing.T) {
	t.Run("normal end", func(t *testing.T) {
		encoder := &HALCommandEncoder{label: "test"}
		pass := &HALComputePassEncoder{encoder: encoder}
		encoder.activeComputePass = pass

		err := pass.End()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !pass.ended {
			t.Error("pass.ended should be true")
		}

		if encoder.activeComputePass != nil {
			t.Error("encoder.activeComputePass should be nil")
		}
	})

	t.Run("double end is no-op", func(t *testing.T) {
		encoder := &HALCommandEncoder{label: "test"}
		pass := &HALComputePassEncoder{encoder: encoder, ended: true}

		err := pass.End()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestHALComputePassEncoder_Dispatch(t *testing.T) {
	t.Run("dispatch with ended pass", func(t *testing.T) {
		pass := &HALComputePassEncoder{ended: true}

		// Should not panic
		pass.Dispatch(8, 8, 1)
	})

	t.Run("dispatch without core pass", func(t *testing.T) {
		pass := &HALComputePassEncoder{ended: false}

		// Should not panic
		pass.Dispatch(8, 8, 1)
	})
}

// =============================================================================
// HALCommandBuffer Tests
// =============================================================================

func TestHALCommandBuffer_Label(t *testing.T) {
	tests := []struct {
		name string
		cb   *HALCommandBuffer
		want string
	}{
		{
			name: "nil buffer",
			cb:   nil,
			want: "",
		},
		{
			name: "buffer with label",
			cb:   &HALCommandBuffer{label: "test-buffer"},
			want: "test-buffer",
		},
		{
			name: "buffer without label",
			cb:   &HALCommandBuffer{label: ""},
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

func TestHALCommandBuffer_CoreBuffer(t *testing.T) {
	t.Run("nil command buffer", func(t *testing.T) {
		var cb *HALCommandBuffer
		if cb.CoreBuffer() != nil {
			t.Error("CoreBuffer() should return nil for nil buffer")
		}
	})

	t.Run("buffer without core buffer", func(t *testing.T) {
		cb := &HALCommandBuffer{label: "test"}
		if cb.CoreBuffer() != nil {
			t.Error("CoreBuffer() should return nil when coreBuffer is nil")
		}
	})
}

// =============================================================================
// HALRenderPassDescriptor Tests
// =============================================================================

func TestHALRenderPassDescriptor_toCoreDescriptor(t *testing.T) {
	t.Run("nil descriptor", func(t *testing.T) {
		var desc *HALRenderPassDescriptor
		coreDesc := desc.toCoreDescriptor()
		if coreDesc != nil {
			t.Error("expected nil for nil descriptor")
		}
	})

	t.Run("descriptor with color attachment", func(t *testing.T) {
		desc := &HALRenderPassDescriptor{
			Label: "test-pass",
			ColorAttachments: []HALRenderPassColorAttachment{
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
		desc := &HALRenderPassDescriptor{
			Label: "test-pass",
			DepthStencilAttachment: &HALRenderPassDepthStencilAttachment{
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

func TestHALCommandEncoder_RenderWorkflow(t *testing.T) {
	backend := &NativeBackend{initialized: true}

	// Create encoder
	encoder, err := NewHALCommandEncoder(backend, "render-workflow")
	if err != nil {
		t.Fatalf("NewHALCommandEncoder failed: %v", err)
	}

	// Verify initial state
	if encoder.Status() != core.CommandEncoderStatusRecording {
		t.Errorf("initial status = %v, want Recording", encoder.Status())
	}

	// Begin render pass
	pass, err := encoder.BeginRenderPass(&HALRenderPassDescriptor{
		Label: "main-pass",
		ColorAttachments: []HALRenderPassColorAttachment{
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

func TestHALCommandEncoder_ComputeWorkflow(t *testing.T) {
	backend := &NativeBackend{initialized: true}

	// Create encoder
	encoder, err := NewHALCommandEncoder(backend, "compute-workflow")
	if err != nil {
		t.Fatalf("NewHALCommandEncoder failed: %v", err)
	}

	// Begin compute pass
	pass, err := encoder.BeginComputePass(&HALComputePassDescriptor{
		Label: "compute-pass",
	})
	if err != nil {
		t.Fatalf("BeginComputePass failed: %v", err)
	}

	// Verify locked state
	if encoder.Status() != core.CommandEncoderStatusLocked {
		t.Errorf("status after begin pass = %v, want Locked", encoder.Status())
	}

	// Dispatch
	pass.Dispatch(8, 8, 1)

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
