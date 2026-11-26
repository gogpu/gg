//go:build darwin && !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

type adapterProvider struct {
	capsProvider
	adapter *wgpu.Adapter
}

func (p *adapterProvider) Adapter() gpucontext.Adapter {
	return wgpu.AdapterToHandle(p.adapter)
}

func openAdapterProvider(t *testing.T, backend gputypes.Backends) *adapterProvider {
	t.Helper()
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: backend})
	if err != nil {
		t.Skipf("create instance for %v: %v", backend, err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Skipf("request adapter for %v: %v", backend, err)
	}
	t.Cleanup(adapter.Release)
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("request device for %v: %v", backend, err)
	}
	t.Cleanup(device.Release)
	return &adapterProvider{capsProvider: capsProvider{device: device}, adapter: adapter}
}

func TestSetDeviceProviderSwitchesFromSoftwareToMetal(t *testing.T) {
	software := openAdapterProvider(t, gputypes.BackendsSoftware)
	metal := openAdapterProvider(t, wgpu.BackendsMetal)
	t.Logf("switching from %s to %s", software.adapter.Info().Name, metal.adapter.Info().Name)
	if software.adapter.Info().DeviceType != gputypes.DeviceTypeCPU ||
		metal.adapter.Info().Backend != gputypes.BackendMetal {
		t.Skip("requires both a software adapter and a Metal adapter")
	}

	shared := NewGPUShared()
	defer shared.Close()
	if err := shared.SetDeviceProvider(software); err != nil {
		t.Fatal(err)
	}
	if shared.strategy != strategyRasterAtlas || shared.IsReady() {
		t.Fatalf("software adapter: strategy = %v, GPU rendering ready = %t", shared.strategy, shared.IsReady())
	}
	if err := shared.SetDeviceProvider(metal); err != nil {
		t.Fatal(err)
	}
	if shared.strategy == strategyRasterAtlas || !shared.IsReady() {
		t.Fatalf("Metal adapter: strategy = %v, GPU rendering ready = %t", shared.strategy, shared.IsReady())
	}
}
