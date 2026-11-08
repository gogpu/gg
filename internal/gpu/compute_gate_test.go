//go:build !nogpu

package gpu

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// capsProvider is a DeviceProvider over a test device that reports the given downlevel capabilities, so the compute
// gate can be driven without a GLES 3.0 or WebGL2 adapter.
type capsProvider struct {
	device *wgpu.Device
	caps   gputypes.DownlevelCapabilities
}

func (p *capsProvider) Device() gpucontext.Device {
	return gpucontext.NewDevice(unsafe.Pointer(p.device)) //nolint:gosec // wgpu handle for the test
}

func (p *capsProvider) Queue() gpucontext.Queue {
	return gpucontext.NewQueue(unsafe.Pointer(p.device.Queue())) //nolint:gosec // wgpu handle for the test
}

func (p *capsProvider) SurfaceFormat() gputypes.TextureFormat {
	return gputypes.TextureFormatBGRA8Unorm
}
func (p *capsProvider) Adapter() gpucontext.Adapter { return gpucontext.Adapter{} }
func (p *capsProvider) AdapterInfo() gpucontext.AdapterInfo {
	return gpucontext.AdapterInfo{Name: "caps-test"}
}
func (p *capsProvider) Features() gputypes.Features { return 0 }
func (p *capsProvider) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	return p.caps
}

// TestSetDeviceProvider_ComputeGate checks the ADR-071 capability gate on the shared-device path: the Vello compute
// accelerator is created only when the provider reports DownlevelFlagsComputeShaders (Skia Graphite
// caps->computeSupport(), Flutter SupportsCompute()). Without the flag (GLES 3.0, WebGL2) gg must not try to build
// compute pipelines, and CanCompute reports false so paths go to the CPU coverage filler.
func TestSetDeviceProvider_ComputeGate(t *testing.T) {
	tests := []struct {
		name        string
		flags       gputypes.DownlevelFlags
		wantCompute bool
	}{
		{name: "no_downlevel_flags", flags: 0, wantCompute: false},
		{name: "other_flags_without_compute", flags: gputypes.DownlevelFlagsIndirectExecution, wantCompute: false},
		{name: "compute_shaders", flags: gputypes.DownlevelFlagsComputeShaders, wantCompute: true},
		{
			name:        "compute_among_other_flags",
			flags:       gputypes.DownlevelFlagsComputeShaders | gputypes.DownlevelFlagsIndirectExecution,
			wantCompute: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, _, cleanup := createNoopDevice(t)
			defer cleanup()

			s := NewGPUShared()
			defer s.Close()
			p := &capsProvider{device: device, caps: gputypes.DownlevelCapabilities{Flags: tt.flags}}
			if err := s.SetDeviceProvider(p); err != nil {
				t.Fatalf("SetDeviceProvider: %v", err)
			}

			s.mu.Lock()
			created := s.velloAccel != nil
			s.mu.Unlock()
			if created != tt.wantCompute {
				t.Errorf("flags %v: Vello accelerator created = %v, want %v", tt.flags, created, tt.wantCompute)
			}
			if !tt.wantCompute && s.CanCompute() {
				t.Error("CanCompute() = true without DownlevelFlagsComputeShaders")
			}
		})
	}
}

// TestInitGPU_ComputeGate checks the same gate on the standalone path, where initGPU reads the adapter's own
// downlevel capabilities: the Vello accelerator exists exactly when the adapter reports compute shaders. It needs a real
// adapter (ggtest_<backend> build tag) and skips without one, unless GG_REQUIRE is set.
func TestInitGPU_ComputeGate(t *testing.T) {
	s := NewGPUShared()
	defer s.Close()
	s.mu.Lock()
	err := s.initGPU()
	s.mu.Unlock()
	if err != nil {
		skipUnlessRequiredf(t, "GPU adapter not available: %v", err)
	}

	adapter, err := s.instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceHighPerformance,
	})
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	wantCompute := adapter.DownlevelCapabilities().Flags.Contains(gputypes.DownlevelFlagsComputeShaders)

	s.mu.Lock()
	created := s.velloAccel != nil
	s.mu.Unlock()
	if created != wantCompute {
		t.Errorf("adapter %q reports compute shaders = %v, but Vello accelerator created = %v",
			adapter.Info().Name, wantCompute, created)
	}
}

func TestCheckGPUComputeSupport(t *testing.T) {
	if CheckGPUComputeSupport(nil) {
		t.Error("CheckGPUComputeSupport(nil) = true, want false")
	}
	for _, tt := range []struct {
		flags gputypes.DownlevelFlags
		want  bool
	}{
		{0, false},
		{gputypes.DownlevelFlagsIndirectExecution, false},
		{gputypes.DownlevelFlagsComputeShaders, true},
	} {
		p := &capsProvider{caps: gputypes.DownlevelCapabilities{Flags: tt.flags}}
		if got := CheckGPUComputeSupport(p); got != tt.want {
			t.Errorf("CheckGPUComputeSupport(flags %v) = %v, want %v", tt.flags, got, tt.want)
		}
	}
}
