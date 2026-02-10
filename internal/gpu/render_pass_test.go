//go:build !nogpu

// Package gpu provides a GPU-accelerated rendering backend using gogpu/wgpu.
package gpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// =============================================================================
// RenderPassEncoder Tests
// =============================================================================

func TestRenderPassEncoder_State(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *RenderPassEncoder
		expected RenderPassState
	}{
		{
			name: "recording state on creation",
			setup: func() *RenderPassEncoder {
				return &RenderPassEncoder{
					state: RenderPassStateRecording,
				}
			},
			expected: RenderPassStateRecording,
		},
		{
			name: "ended state after End",
			setup: func() *RenderPassEncoder {
				p := &RenderPassEncoder{
					state: RenderPassStateRecording,
				}
				_ = p.End()
				return p
			},
			expected: RenderPassStateEnded,
		},
		{
			name: "nil pass returns ended",
			setup: func() *RenderPassEncoder {
				return nil
			},
			expected: RenderPassStateEnded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup()
			got := p.State()
			if got != tt.expected {
				t.Errorf("State() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRenderPassEncoder_IsEnded(t *testing.T) {
	tests := []struct {
		name     string
		state    RenderPassState
		expected bool
	}{
		{"recording", RenderPassStateRecording, false},
		{"ended", RenderPassStateEnded, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RenderPassEncoder{state: tt.state}
			if got := p.IsEnded(); got != tt.expected {
				t.Errorf("IsEnded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRenderPassEncoder_SetPipeline(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		pipeline := &RenderPipeline{id: 1}
		err := p.SetPipeline(pipeline)
		if err != nil {
			t.Errorf("SetPipeline() error = %v, want nil", err)
		}
		if p.currentPipeline != pipeline {
			t.Error("pipeline not set")
		}
	})

	t.Run("nil pipeline error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.SetPipeline(nil)
		if err == nil {
			t.Error("SetPipeline(nil) should return error")
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		pipeline := &RenderPipeline{id: 1}
		err := p.SetPipeline(pipeline)
		if err == nil {
			t.Error("SetPipeline() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetBindGroup(t *testing.T) {
	tests := []struct {
		name      string
		index     uint32
		bindGroup *BindGroup
		offsets   []uint32
		wantErr   bool
	}{
		{
			name:      "valid bind group index 0",
			index:     0,
			bindGroup: &BindGroup{id: 1},
			offsets:   nil,
			wantErr:   false,
		},
		{
			name:      "valid bind group index 3",
			index:     3,
			bindGroup: &BindGroup{id: 2},
			offsets:   []uint32{256, 512},
			wantErr:   false,
		},
		{
			name:      "index out of range",
			index:     4,
			bindGroup: &BindGroup{id: 1},
			offsets:   nil,
			wantErr:   true,
		},
		{
			name:      "nil bind group",
			index:     0,
			bindGroup: nil,
			offsets:   nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RenderPassEncoder{state: RenderPassStateRecording}
			err := p.SetBindGroup(tt.index, tt.bindGroup, tt.offsets)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetBindGroup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetBindGroup(0, &BindGroup{id: 1}, nil)
		if err == nil {
			t.Error("SetBindGroup() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetVertexBuffer(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.SetVertexBuffer(0, buffer, 0, 0)
		if err != nil {
			t.Errorf("SetVertexBuffer() error = %v, want nil", err)
		}
		if p.vertexBufferCount != 1 {
			t.Errorf("vertexBufferCount = %d, want 1", p.vertexBufferCount)
		}
	})

	t.Run("nil buffer error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.SetVertexBuffer(0, nil, 0, 0)
		if err == nil {
			t.Error("SetVertexBuffer(nil) should return error")
		}
	})

	t.Run("multiple slots", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}

		_ = p.SetVertexBuffer(0, buffer, 0, 0)
		_ = p.SetVertexBuffer(5, buffer, 0, 0)

		if p.vertexBufferCount != 6 {
			t.Errorf("vertexBufferCount = %d, want 6", p.vertexBufferCount)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetVertexBuffer(0, &Buffer{}, 0, 0)
		if err == nil {
			t.Error("SetVertexBuffer() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetIndexBuffer(t *testing.T) {
	t.Run("success uint16", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.SetIndexBuffer(buffer, IndexFormatUint16, 0, 0)
		if err != nil {
			t.Errorf("SetIndexBuffer() error = %v, want nil", err)
		}
		if !p.hasIndexBuffer {
			t.Error("hasIndexBuffer should be true")
		}
	})

	t.Run("success uint32", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.SetIndexBuffer(buffer, IndexFormatUint32, 0, 0)
		if err != nil {
			t.Errorf("SetIndexBuffer() error = %v, want nil", err)
		}
	})

	t.Run("nil buffer error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.SetIndexBuffer(nil, IndexFormatUint16, 0, 0)
		if err == nil {
			t.Error("SetIndexBuffer(nil) should return error")
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetIndexBuffer(&Buffer{}, IndexFormatUint16, 0, 0)
		if err == nil {
			t.Error("SetIndexBuffer() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetViewport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.SetViewport(0, 0, 1920, 1080, 0, 1)
		if err != nil {
			t.Errorf("SetViewport() error = %v, want nil", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetViewport(0, 0, 100, 100, 0, 1)
		if err == nil {
			t.Error("SetViewport() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetScissorRect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.SetScissorRect(0, 0, 1920, 1080)
		if err != nil {
			t.Errorf("SetScissorRect() error = %v, want nil", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetScissorRect(0, 0, 100, 100)
		if err == nil {
			t.Error("SetScissorRect() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetBlendConstant(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		color := gputypes.Color{R: 1, G: 0, B: 0, A: 1}
		err := p.SetBlendConstant(color)
		if err != nil {
			t.Errorf("SetBlendConstant() error = %v, want nil", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetBlendConstant(gputypes.Color{})
		if err == nil {
			t.Error("SetBlendConstant() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_SetStencilReference(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.SetStencilReference(128)
		if err != nil {
			t.Errorf("SetStencilReference() error = %v, want nil", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.SetStencilReference(0)
		if err == nil {
			t.Error("SetStencilReference() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_Draw(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.Draw(6, 1, 0, 0)
		if err != nil {
			t.Errorf("Draw() error = %v, want nil", err)
		}
	})

	t.Run("instanced", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.Draw(36, 100, 0, 0)
		if err != nil {
			t.Errorf("Draw() error = %v, want nil", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.Draw(3, 1, 0, 0)
		if err == nil {
			t.Error("Draw() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_DrawIndexed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.DrawIndexed(36, 1, 0, 0, 0)
		if err != nil {
			t.Errorf("DrawIndexed() error = %v, want nil", err)
		}
	})

	t.Run("with base vertex", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.DrawIndexed(36, 1, 0, -10, 0)
		if err != nil {
			t.Errorf("DrawIndexed() error = %v, want nil", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.DrawIndexed(6, 1, 0, 0, 0)
		if err == nil {
			t.Error("DrawIndexed() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_DrawIndirect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.DrawIndirect(buffer, 0)
		if err != nil {
			t.Errorf("DrawIndirect() error = %v, want nil", err)
		}
	})

	t.Run("nil buffer error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.DrawIndirect(nil, 0)
		if err == nil {
			t.Error("DrawIndirect(nil) should return error")
		}
	})

	t.Run("unaligned offset error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.DrawIndirect(buffer, 3)
		if err == nil {
			t.Error("DrawIndirect() with unaligned offset should return error")
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.DrawIndirect(&Buffer{}, 0)
		if err == nil {
			t.Error("DrawIndirect() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_DrawIndexedIndirect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.DrawIndexedIndirect(buffer, 0)
		if err != nil {
			t.Errorf("DrawIndexedIndirect() error = %v, want nil", err)
		}
	})

	t.Run("nil buffer error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.DrawIndexedIndirect(nil, 0)
		if err == nil {
			t.Error("DrawIndexedIndirect(nil) should return error")
		}
	})

	t.Run("unaligned offset error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		buffer := &Buffer{}
		err := p.DrawIndexedIndirect(buffer, 5)
		if err == nil {
			t.Error("DrawIndexedIndirect() with unaligned offset should return error")
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateEnded}
		err := p.DrawIndexedIndirect(&Buffer{}, 0)
		if err == nil {
			t.Error("DrawIndexedIndirect() on ended pass should return error")
		}
	})
}

func TestRenderPassEncoder_End(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		err := p.End()
		if err != nil {
			t.Errorf("End() error = %v, want nil", err)
		}
		if p.state != RenderPassStateEnded {
			t.Errorf("state = %v, want %v", p.state, RenderPassStateEnded)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		p := &RenderPassEncoder{state: RenderPassStateRecording}
		_ = p.End()
		err := p.End()
		if err != nil {
			t.Errorf("End() second call error = %v, want nil", err)
		}
	})
}

// =============================================================================
// RenderPassState Tests
// =============================================================================

func TestRenderPassState_String(t *testing.T) {
	tests := []struct {
		state    RenderPassState
		expected string
	}{
		{RenderPassStateRecording, "Recording"},
		{RenderPassStateEnded, "Ended"},
		{RenderPassState(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// RenderPipeline Tests
// =============================================================================

func TestRenderPipeline_Methods(t *testing.T) {
	p := &RenderPipeline{
		id:    123,
		label: "test-pipeline",
	}

	t.Run("ID", func(t *testing.T) {
		if got := p.ID(); got != 123 {
			t.Errorf("ID() = %v, want 123", got)
		}
	})

	t.Run("Label", func(t *testing.T) {
		if got := p.Label(); got != "test-pipeline" {
			t.Errorf("Label() = %v, want test-pipeline", got)
		}
	})

	t.Run("IsDestroyed before destroy", func(t *testing.T) {
		if p.IsDestroyed() {
			t.Error("IsDestroyed() should be false before Destroy()")
		}
	})

	t.Run("Destroy", func(t *testing.T) {
		p.Destroy()
		if !p.IsDestroyed() {
			t.Error("IsDestroyed() should be true after Destroy()")
		}
	})
}

// =============================================================================
// BindGroup Tests
// =============================================================================

func TestBindGroup_Methods(t *testing.T) {
	bg := &BindGroup{
		id:    456,
		label: "test-bindgroup",
	}

	t.Run("ID", func(t *testing.T) {
		if got := bg.ID(); got != 456 {
			t.Errorf("ID() = %v, want 456", got)
		}
	})

	t.Run("Label", func(t *testing.T) {
		if got := bg.Label(); got != "test-bindgroup" {
			t.Errorf("Label() = %v, want test-bindgroup", got)
		}
	})

	t.Run("IsDestroyed before destroy", func(t *testing.T) {
		if bg.IsDestroyed() {
			t.Error("IsDestroyed() should be false before Destroy()")
		}
	})

	t.Run("Destroy", func(t *testing.T) {
		bg.Destroy()
		if !bg.IsDestroyed() {
			t.Error("IsDestroyed() should be true after Destroy()")
		}
	})
}

// =============================================================================
// IndexFormat Tests (IndexFormat is defined in commands.go)
// =============================================================================

func TestIndexFormat_Values(t *testing.T) {
	// Verify the constant values match expected
	if IndexFormatUint16 != 0 {
		t.Errorf("IndexFormatUint16 = %d, want 0", IndexFormatUint16)
	}
	if IndexFormatUint32 != 1 {
		t.Errorf("IndexFormatUint32 = %d, want 1", IndexFormatUint32)
	}
}
