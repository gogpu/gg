package wgpu

import (
	"fmt"
	"log"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/types"
)

// GPUInfo contains information about the selected GPU.
type GPUInfo struct {
	// Name is the GPU name (e.g., "NVIDIA GeForce RTX 3080").
	Name string
	// Vendor is the GPU vendor.
	Vendor string
	// DeviceType is the type of GPU (discrete, integrated, etc.).
	DeviceType types.DeviceType
	// Backend is the graphics API in use (Vulkan, Metal, DX12).
	Backend types.Backend
	// Driver is the driver version string.
	Driver string
}

// String returns a human-readable description of the GPU.
func (g *GPUInfo) String() string {
	return fmt.Sprintf("%s (%s, %s)", g.Name, g.DeviceType, g.Backend)
}

// getGPUInfo retrieves information about the GPU adapter.
func getGPUInfo(adapterID core.AdapterID) (*GPUInfo, error) {
	info, err := core.GetAdapterInfo(adapterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get adapter info: %w", err)
	}

	return &GPUInfo{
		Name:       info.Name,
		Vendor:     info.Vendor,
		DeviceType: info.DeviceType,
		Backend:    info.Backend,
		Driver:     info.Driver,
	}, nil
}

// logGPUInfo logs information about the selected GPU.
func logGPUInfo(adapterID core.AdapterID) {
	info, err := getGPUInfo(adapterID)
	if err != nil {
		log.Printf("wgpu: failed to get GPU info: %v", err)
		return
	}

	log.Printf("wgpu: GPU: %s", info.String())
	if info.Driver != "" {
		log.Printf("wgpu: Driver: %s", info.Driver)
	}
}

// createDevice creates a logical device from an adapter.
// This is a helper function that encapsulates device creation logic.
func createDevice(adapterID core.AdapterID, label string) (core.DeviceID, error) {
	desc := &types.DeviceDescriptor{
		Label: label,
		// Use default limits and no special features for now
		RequiredFeatures: nil,
		RequiredLimits:   types.DefaultLimits(),
	}

	deviceID, err := core.RequestDevice(adapterID, desc)
	if err != nil {
		return core.DeviceID{}, fmt.Errorf("failed to create device: %w", err)
	}

	return deviceID, nil
}

// getDeviceQueue retrieves the queue associated with a device.
func getDeviceQueue(deviceID core.DeviceID) (core.QueueID, error) {
	queueID, err := core.GetDeviceQueue(deviceID)
	if err != nil {
		return core.QueueID{}, fmt.Errorf("failed to get device queue: %w", err)
	}
	return queueID, nil
}

// releaseDevice releases a device and its associated resources.
func releaseDevice(deviceID core.DeviceID) error {
	if deviceID.IsZero() {
		return nil
	}

	err := core.DeviceDrop(deviceID)
	if err != nil {
		return fmt.Errorf("failed to release device: %w", err)
	}
	return nil
}

// releaseAdapter releases an adapter.
func releaseAdapter(adapterID core.AdapterID) error {
	if adapterID.IsZero() {
		return nil
	}

	err := core.AdapterDrop(adapterID)
	if err != nil {
		return fmt.Errorf("failed to release adapter: %w", err)
	}
	return nil
}

// CheckDeviceLimits verifies that the device meets minimum requirements.
// This can be used to validate GPU capabilities before rendering.
func CheckDeviceLimits(deviceID core.DeviceID) error {
	limits, err := core.GetDeviceLimits(deviceID)
	if err != nil {
		return fmt.Errorf("failed to get device limits: %w", err)
	}

	// For now, just log some basic limits
	// In the future, we can validate minimum requirements
	log.Printf("wgpu: Max texture dimension 2D: %d", limits.MaxTextureDimension2D)
	log.Printf("wgpu: Max buffer size: %d", limits.MaxBufferSize)

	return nil
}
