// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !nogpu

package gpu

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"

	// Import Vulkan backend so it registers via init().
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

// VelloAccelerator provides GPU-accelerated scene rendering using the Vello-style
// compute pipeline. It implements gg.GPUAccelerator and gg.ComputePipelineAware.
//
// Unlike SDFAccelerator which uses render passes (vertex/fragment shaders),
// VelloAccelerator uses 8 compute shader stages to rasterize entire scenes:
// pathtag_reduce -> pathtag_scan -> draw_reduce -> draw_leaf ->
// path_count -> backdrop -> coarse -> fine
//
// The compute pipeline excels at complex scenes with many overlapping paths,
// deep clip stacks, and high shape counts (>50 shapes per frame).
type VelloAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	dispatcher *VelloComputeDispatcher

	// TODO: scene encoding accumulation
	// pendingScene *velloport.SceneEncoding

	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)
}

// Interface compliance checks.
var _ gg.GPUAccelerator = (*VelloAccelerator)(nil)
var _ gg.DeviceProviderAware = (*VelloAccelerator)(nil)
var _ gg.ComputePipelineAware = (*VelloAccelerator)(nil)

// Name returns the accelerator identifier.
func (a *VelloAccelerator) Name() string { return "vello-compute" }

// Init registers the accelerator. GPU device initialization is deferred
// until the first use or until SetDeviceProvider is called, to avoid
// creating a standalone Vulkan device that may interfere with an external
// DX12/Metal device provided later. This lazy approach prevents Intel iGPU
// driver issues where destroying a Vulkan device kills a coexisting DX12 device.
func (a *VelloAccelerator) Init() error {
	return nil
}

// Close releases all GPU resources held by the accelerator.
func (a *VelloAccelerator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.dispatcher != nil {
		a.dispatcher.Close()
		a.dispatcher = nil
	}

	if !a.externalDevice {
		if a.device != nil {
			a.device.Destroy()
			a.device = nil
		}
		if a.instance != nil {
			a.instance.Destroy()
			a.instance = nil
		}
	} else {
		// Don't destroy shared resources -- we don't own them.
		a.device = nil
		a.instance = nil
	}
	a.queue = nil
	a.gpuReady = false
	a.externalDevice = false
}

// SetLogger sets the logger for the GPU accelerator and its internal packages.
// Called by gg.SetLogger to propagate logging configuration.
func (a *VelloAccelerator) SetLogger(l *slog.Logger) {
	setLogger(l)
}

// CanAccelerate reports whether this accelerator supports the given operation.
// The Vello compute pipeline operates at scene level, not per-shape.
func (a *VelloAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&gg.AccelScene != 0
}

// CanCompute reports whether the compute pipeline is available and ready.
func (a *VelloAccelerator) CanCompute() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gpuReady && a.dispatcher != nil && a.dispatcher.initialized
}

// SetDeviceProvider switches the accelerator to use a shared GPU device
// from an external provider (e.g., gogpu). The provider must implement
// HalDevice() any and HalQueue() any returning hal.Device and hal.Queue.
func (a *VelloAccelerator) SetDeviceProvider(provider any) error {
	type halProvider interface {
		HalDevice() any
		HalQueue() any
	}
	hp, ok := provider.(halProvider)
	if !ok {
		return fmt.Errorf("vello-compute: provider does not expose HAL types")
	}
	device, ok := hp.HalDevice().(hal.Device)
	if !ok || device == nil {
		return fmt.Errorf("vello-compute: provider HalDevice is not hal.Device")
	}
	queue, ok := hp.HalQueue().(hal.Queue)
	if !ok || queue == nil {
		return fmt.Errorf("vello-compute: provider HalQueue is not hal.Queue")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Destroy own resources if we created them.
	if a.dispatcher != nil {
		a.dispatcher.Close()
		a.dispatcher = nil
	}
	if !a.externalDevice && a.device != nil {
		a.device.Destroy()
	}
	if a.instance != nil {
		a.instance.Destroy()
		a.instance = nil
	}

	// Use provided resources.
	a.device = device
	a.queue = queue
	a.externalDevice = true

	// Create dispatcher with the provided device/queue.
	dispatcher := NewVelloComputeDispatcher(device, queue)
	if err := dispatcher.Init(); err != nil {
		slogger().Warn("vello-compute: pipeline init failed, compute unavailable", "error", err)
		// Still mark gpuReady — device is valid, just compute isn't available.
		a.gpuReady = true
		return nil
	}
	a.dispatcher = dispatcher

	a.gpuReady = true
	slogger().Debug("vello-compute: switched to shared GPU device")
	return nil
}

// FillPath returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual path operations are not supported in Phase 1.
// In Phase 2, paths will be accumulated into a scene encoding.
func (a *VelloAccelerator) FillPath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// StrokePath returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual path operations are not supported in Phase 1.
// In Phase 2, paths will be accumulated into a scene encoding.
func (a *VelloAccelerator) StrokePath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// FillShape returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual shape operations are not supported in Phase 1.
// In Phase 2, shapes will be accumulated into a scene encoding.
func (a *VelloAccelerator) FillShape(_ gg.GPURenderTarget, _ gg.DetectedShape, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// StrokeShape returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual shape operations are not supported in Phase 1.
// In Phase 2, shapes will be accumulated into a scene encoding.
func (a *VelloAccelerator) StrokeShape(_ gg.GPURenderTarget, _ gg.DetectedShape, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// Flush dispatches any pending GPU operations to the target pixel buffer.
// In Phase 1, there is no scene accumulation, so Flush is a no-op.
// In Phase 2, this will pack the accumulated scene encoding and dispatch
// the 8-stage compute pipeline via VelloComputeDispatcher.
func (a *VelloAccelerator) Flush(_ gg.GPURenderTarget) error {
	return nil
}

// initGPU creates a standalone Vulkan device for compute-only use.
// This is the fallback path when no external device is provided via
// SetDeviceProvider (e.g., when gg is used without gogpu).
func (a *VelloAccelerator) initGPU() error {
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		return fmt.Errorf("vulkan backend not available")
	}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Flags: 0})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	a.instance = instance

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		return fmt.Errorf("no GPU adapters found")
	}

	var selected *hal.ExposedAdapter
	for i := range adapters {
		if adapters[i].Info.DeviceType == gputypes.DeviceTypeDiscreteGPU ||
			adapters[i].Info.DeviceType == gputypes.DeviceTypeIntegratedGPU {
			selected = &adapters[i]
			break
		}
	}
	if selected == nil {
		selected = &adapters[0]
	}

	openDev, err := selected.Adapter.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	a.device = openDev.Device
	a.queue = openDev.Queue

	// Create dispatcher with the standalone device/queue.
	dispatcher := NewVelloComputeDispatcher(a.device, a.queue)
	if err := dispatcher.Init(); err != nil {
		slogger().Warn("vello-compute: pipeline init failed, compute unavailable", "error", err)
		a.gpuReady = true
		return nil
	}
	a.dispatcher = dispatcher

	a.gpuReady = true
	slogger().Info("vello-compute: GPU initialized (standalone)", "adapter", selected.Info.Name)
	return nil
}
