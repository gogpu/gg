// Package native provides a GPU-accelerated rendering backend using gogpu/wgpu.
package native

import (
	"testing"
)

// =============================================================================
// HALComputePassEncoder Tests
// =============================================================================

func TestHALComputePassEncoder_State(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *HALComputePassEncoder
		expected HALComputePassState
	}{
		{
			name: "recording state on creation",
			setup: func() *HALComputePassEncoder {
				return &HALComputePassEncoder{
					state: HALComputePassStateRecording,
				}
			},
			expected: HALComputePassStateRecording,
		},
		{
			name: "ended state after End",
			setup: func() *HALComputePassEncoder {
				p := &HALComputePassEncoder{
					state: HALComputePassStateRecording,
				}
				_ = p.End()
				return p
			},
			expected: HALComputePassStateEnded,
		},
		{
			name: "nil pass returns ended",
			setup: func() *HALComputePassEncoder {
				return nil
			},
			expected: HALComputePassStateEnded,
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

func TestHALComputePassEncoder_IsEnded(t *testing.T) {
	tests := []struct {
		name     string
		state    HALComputePassState
		expected bool
	}{
		{"recording", HALComputePassStateRecording, false},
		{"ended", HALComputePassStateEnded, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &HALComputePassEncoder{state: tt.state}
			if got := p.IsEnded(); got != tt.expected {
				t.Errorf("IsEnded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHALComputePassEncoder_SetPipeline(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		pipeline := &HALComputePipeline{id: 1}
		err := p.SetPipeline(pipeline)
		if err != nil {
			t.Errorf("SetPipeline() error = %v, want nil", err)
		}
		if p.currentPipeline != pipeline {
			t.Error("pipeline not set")
		}
	})

	t.Run("nil pipeline error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		err := p.SetPipeline(nil)
		if err == nil {
			t.Error("SetPipeline(nil) should return error")
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateEnded}
		pipeline := &HALComputePipeline{id: 1}
		err := p.SetPipeline(pipeline)
		if err == nil {
			t.Error("SetPipeline() on ended pass should return error")
		}
	})
}

func TestHALComputePassEncoder_SetBindGroup(t *testing.T) {
	tests := []struct {
		name      string
		index     uint32
		bindGroup *HALBindGroup
		offsets   []uint32
		wantErr   bool
	}{
		{
			name:      "valid bind group index 0",
			index:     0,
			bindGroup: &HALBindGroup{id: 1},
			offsets:   nil,
			wantErr:   false,
		},
		{
			name:      "valid bind group index 3",
			index:     3,
			bindGroup: &HALBindGroup{id: 2},
			offsets:   []uint32{256, 512},
			wantErr:   false,
		},
		{
			name:      "index out of range",
			index:     4,
			bindGroup: &HALBindGroup{id: 1},
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
			p := &HALComputePassEncoder{state: HALComputePassStateRecording}
			err := p.SetBindGroup(tt.index, tt.bindGroup, tt.offsets)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetBindGroup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("ended pass error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateEnded}
		err := p.SetBindGroup(0, &HALBindGroup{id: 1}, nil)
		if err == nil {
			t.Error("SetBindGroup() on ended pass should return error")
		}
	})
}

func TestHALComputePassEncoder_DispatchWorkgroups(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		err := p.DispatchWorkgroups(64, 64, 1)
		if err != nil {
			t.Errorf("DispatchWorkgroups() error = %v, want nil", err)
		}
		if p.dispatchCount != 1 {
			t.Errorf("dispatchCount = %d, want 1", p.dispatchCount)
		}
	})

	t.Run("multiple dispatches", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		_ = p.DispatchWorkgroups(32, 32, 1)
		_ = p.DispatchWorkgroups(16, 16, 1)
		_ = p.DispatchWorkgroups(8, 8, 8)

		if p.dispatchCount != 3 {
			t.Errorf("dispatchCount = %d, want 3", p.dispatchCount)
		}
	})

	t.Run("zero workgroups allowed", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		err := p.DispatchWorkgroups(0, 0, 0)
		if err != nil {
			t.Errorf("DispatchWorkgroups(0,0,0) error = %v, want nil (spec allows zero)", err)
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateEnded}
		err := p.DispatchWorkgroups(1, 1, 1)
		if err == nil {
			t.Error("DispatchWorkgroups() on ended pass should return error")
		}
	})
}

func TestHALComputePassEncoder_DispatchWorkgroupsIndirect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		buffer := &HALBuffer{}
		err := p.DispatchWorkgroupsIndirect(buffer, 0)
		if err != nil {
			t.Errorf("DispatchWorkgroupsIndirect() error = %v, want nil", err)
		}
		if p.dispatchCount != 1 {
			t.Errorf("dispatchCount = %d, want 1", p.dispatchCount)
		}
	})

	t.Run("aligned offset", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		buffer := &HALBuffer{}
		err := p.DispatchWorkgroupsIndirect(buffer, 256)
		if err != nil {
			t.Errorf("DispatchWorkgroupsIndirect() error = %v, want nil", err)
		}
	})

	t.Run("nil buffer error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		err := p.DispatchWorkgroupsIndirect(nil, 0)
		if err == nil {
			t.Error("DispatchWorkgroupsIndirect(nil) should return error")
		}
	})

	t.Run("unaligned offset error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		buffer := &HALBuffer{}
		err := p.DispatchWorkgroupsIndirect(buffer, 3)
		if err == nil {
			t.Error("DispatchWorkgroupsIndirect() with unaligned offset should return error")
		}
	})

	t.Run("ended pass error", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateEnded}
		err := p.DispatchWorkgroupsIndirect(&HALBuffer{}, 0)
		if err == nil {
			t.Error("DispatchWorkgroupsIndirect() on ended pass should return error")
		}
	})
}

func TestHALComputePassEncoder_End(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		err := p.End()
		if err != nil {
			t.Errorf("End() error = %v, want nil", err)
		}
		if p.state != HALComputePassStateEnded {
			t.Errorf("state = %v, want %v", p.state, HALComputePassStateEnded)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		_ = p.End()
		err := p.End()
		if err != nil {
			t.Errorf("End() second call error = %v, want nil", err)
		}
	})
}

func TestHALComputePassEncoder_DispatchCount(t *testing.T) {
	t.Run("zero initially", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		if got := p.DispatchCount(); got != 0 {
			t.Errorf("DispatchCount() = %d, want 0", got)
		}
	})

	t.Run("tracks dispatches", func(t *testing.T) {
		p := &HALComputePassEncoder{state: HALComputePassStateRecording}
		_ = p.DispatchWorkgroups(1, 1, 1)
		_ = p.DispatchWorkgroupsIndirect(&HALBuffer{}, 0)

		if got := p.DispatchCount(); got != 2 {
			t.Errorf("DispatchCount() = %d, want 2", got)
		}
	})
}

// =============================================================================
// HALComputePassState Tests
// =============================================================================

func TestHALComputePassState_String(t *testing.T) {
	tests := []struct {
		state    HALComputePassState
		expected string
	}{
		{HALComputePassStateRecording, "Recording"},
		{HALComputePassStateEnded, "Ended"},
		{HALComputePassState(99), "Unknown(99)"},
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
// HALComputePipeline Tests
// =============================================================================

func TestHALComputePipeline_Methods(t *testing.T) {
	p := &HALComputePipeline{
		id:            123,
		label:         "test-compute-pipeline",
		workgroupSize: [3]uint32{64, 1, 1},
	}

	t.Run("ID", func(t *testing.T) {
		if got := p.ID(); got != 123 {
			t.Errorf("ID() = %v, want 123", got)
		}
	})

	t.Run("Label", func(t *testing.T) {
		if got := p.Label(); got != "test-compute-pipeline" {
			t.Errorf("Label() = %v, want test-compute-pipeline", got)
		}
	})

	t.Run("WorkgroupSize", func(t *testing.T) {
		expected := [3]uint32{64, 1, 1}
		if got := p.WorkgroupSize(); got != expected {
			t.Errorf("WorkgroupSize() = %v, want %v", got, expected)
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
// Indirect Arguments Tests
// =============================================================================

func TestDispatchIndirectArgs_Size(t *testing.T) {
	args := DispatchIndirectArgs{}
	expected := uint64(12) // 3 * sizeof(uint32)
	if got := args.Size(); got != expected {
		t.Errorf("Size() = %d, want %d", got, expected)
	}
}

func TestDrawIndirectArgs_Size(t *testing.T) {
	args := DrawIndirectArgs{}
	expected := uint64(16) // 4 * sizeof(uint32)
	if got := args.Size(); got != expected {
		t.Errorf("Size() = %d, want %d", got, expected)
	}
}

func TestDrawIndexedIndirectArgs_Size(t *testing.T) {
	args := DrawIndexedIndirectArgs{}
	expected := uint64(20) // 5 * sizeof(uint32)
	if got := args.Size(); got != expected {
		t.Errorf("Size() = %d, want %d", got, expected)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestComputePass_TypicalWorkflow(t *testing.T) {
	// Simulate a typical compute pass workflow
	p := &HALComputePassEncoder{state: HALComputePassStateRecording}

	// Set pipeline
	pipeline := &HALComputePipeline{
		id:            1,
		workgroupSize: [3]uint32{256, 1, 1},
	}
	if err := p.SetPipeline(pipeline); err != nil {
		t.Fatalf("SetPipeline failed: %v", err)
	}

	// Set bind groups
	bindGroup := &HALBindGroup{id: 1}
	if err := p.SetBindGroup(0, bindGroup, nil); err != nil {
		t.Fatalf("SetBindGroup failed: %v", err)
	}

	// Dispatch compute work
	if err := p.DispatchWorkgroups(1024/256, 1, 1); err != nil {
		t.Fatalf("DispatchWorkgroups failed: %v", err)
	}

	// End pass
	if err := p.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify final state
	if !p.IsEnded() {
		t.Error("pass should be ended")
	}
	if p.DispatchCount() != 1 {
		t.Errorf("expected 1 dispatch, got %d", p.DispatchCount())
	}
}

func TestComputePass_MultipleDispatches(t *testing.T) {
	p := &HALComputePassEncoder{state: HALComputePassStateRecording}

	// Multiple passes for different data
	for i := 0; i < 10; i++ {
		if err := p.DispatchWorkgroups(64, 64, 1); err != nil {
			t.Fatalf("dispatch %d failed: %v", i, err)
		}
	}

	if p.DispatchCount() != 10 {
		t.Errorf("expected 10 dispatches, got %d", p.DispatchCount())
	}

	if err := p.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
}
