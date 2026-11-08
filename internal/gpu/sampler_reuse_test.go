//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// newTestClipLayout builds the same clip bind group layout the render session uses.
func newTestClipLayout(t *testing.T, device *wgpu.Device) *wgpu.BindGroupLayout {
	t.Helper()
	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "test_clip_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: gputypes.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: clipParamsSize},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	return layout
}

// The text pipelines are first created without the clip layout and recreated once it arrives. The recreate must
// keep the sampler: replacing it dropped the only reference to the first one, which then outlived the device
// (VUID-vkDestroyDevice-device-05137 on close, P2-GPU-039).

func TestMSDFTextPipelineKeepsSamplerWhenClipLayoutArrives(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewMSDFTextPipeline(device, queue, 4)
	defer p.Destroy()

	if err := p.ensurePipelineWithStencil(); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	first := p.sampler
	if first == nil {
		t.Fatal("no sampler after the first ensure")
	}

	p.SetClipBindLayout(newTestClipLayout(t, device))
	if err := p.ensurePipelineWithStencil(); err != nil {
		t.Fatalf("ensure after the clip layout: %v", err)
	}
	if !p.pipeLayoutHasClip {
		t.Fatal("pipeline layout was not recreated with the clip layout")
	}
	if p.sampler != first {
		t.Fatal("recreate replaced the sampler; the first one is never released")
	}
}

func TestGlyphMaskPipelineKeepsSamplerWhenClipLayoutArrives(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewGlyphMaskPipeline(device, queue, 4)
	defer p.Destroy()

	if err := p.ensurePipelineWithStencil(); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	first := p.sampler
	if first == nil {
		t.Fatal("no sampler after the first ensure")
	}

	p.SetClipBindLayout(newTestClipLayout(t, device))
	if err := p.ensurePipelineWithStencil(); err != nil {
		t.Fatalf("ensure after the clip layout: %v", err)
	}
	if !p.pipeLayoutHasClip {
		t.Fatal("pipeline layout was not recreated with the clip layout")
	}
	if p.sampler != first {
		t.Fatal("recreate replaced the sampler; the first one is never released")
	}
}
