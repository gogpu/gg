//go:build rust

package rust

import (
	"fmt"
	"log"
	"sync"

	"github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/backend"
	"github.com/gogpu/gg/scene"
)

// init registers the rust backend on package import.
func init() {
	backend.Register(backend.BackendRust, func() backend.RenderBackend {
		return &RustBackend{}
	})
}

// RustBackend is a GPU-accelerated rendering backend using go-webgpu/webgpu.
// It implements the backend.RenderBackend interface.
//
// The backend manages GPU resources including instance, adapter, device,
// and queue via wgpu-native FFI bindings.
type RustBackend struct {
	mu sync.RWMutex

	// GPU resources (go-webgpu/webgpu)
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue

	// GPU information
	gpuInfo *GPUInfo

	// State
	initialized bool
}

// GPUInfo contains information about the selected GPU.
type GPUInfo struct {
	Vendor       string
	Architecture string
	Device       string
	Description  string
	BackendType  string
	AdapterType  string
	VendorID     uint32
	DeviceID     uint32
}

// NewRustBackend creates a new Rust GPU rendering backend.
// The backend must be initialized with Init() before use.
func NewRustBackend() *RustBackend {
	return &RustBackend{}
}

// Name returns the backend identifier.
func (b *RustBackend) Name() string {
	return backend.BackendRust
}

// Init initializes the backend by creating GPU resources.
// This includes initializing wgpu-native, creating an instance,
// requesting an adapter, creating a device, and getting the command queue.
//
// Returns an error if GPU initialization fails.
func (b *RustBackend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized {
		return nil
	}

	// Step 1: Initialize wgpu-native library
	if err := wgpu.Init(); err != nil {
		return fmt.Errorf("%w: %w", ErrLibraryNotFound, err)
	}

	// Step 2: Create Instance
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("instance creation failed: %w", err)
	}
	b.instance = instance

	// Step 3: Request Adapter (prefer high performance GPU)
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceHighPerformance,
	})
	if err != nil {
		b.instance.Release()
		b.instance = nil
		return fmt.Errorf("%w: %w", ErrNoGPU, err)
	}
	b.adapter = adapter

	// Log and store GPU information
	b.gpuInfo = b.getGPUInfo()
	b.logGPUInfo()

	// Step 4: Create Device
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		b.adapter.Release()
		b.adapter = nil
		b.instance.Release()
		b.instance = nil
		return fmt.Errorf("device creation failed: %w", err)
	}
	b.device = device

	// Step 5: Get Queue
	queue := device.GetQueue()
	if queue == nil {
		b.device.Release()
		b.device = nil
		b.adapter.Release()
		b.adapter = nil
		b.instance.Release()
		b.instance = nil
		return fmt.Errorf("queue retrieval failed")
	}
	b.queue = queue

	b.initialized = true
	log.Println("rust: backend initialized successfully")

	return nil
}

// Close releases all backend resources.
// The backend should not be used after Close is called.
func (b *RustBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return
	}

	// Release resources in reverse order of creation
	if b.queue != nil {
		b.queue.Release()
		b.queue = nil
	}

	if b.device != nil {
		b.device.Release()
		b.device = nil
	}

	if b.adapter != nil {
		b.adapter.Release()
		b.adapter = nil
	}

	if b.instance != nil {
		b.instance.Release()
		b.instance = nil
	}

	b.gpuInfo = nil
	b.initialized = false

	log.Println("rust: backend closed")
}

// NewRenderer creates a renderer for immediate mode rendering.
// The renderer is sized for the given dimensions.
//
// Note: This is a stub implementation. Full GPU rendering
// will be implemented in future versions.
func (b *RustBackend) NewRenderer(width, height int) gg.Renderer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.initialized {
		log.Println("rust: warning - creating renderer on uninitialized backend")
		return nil
	}

	if width <= 0 || height <= 0 {
		log.Printf("rust: warning - invalid dimensions: %dx%d", width, height)
		return nil
	}

	return newGPURenderer(b, width, height)
}

// RenderScene renders a scene to the target pixmap using retained mode.
// This method is optimized for complex scenes with many draw operations.
//
// Note: This is a stub implementation. Full GPU rendering
// will be implemented in future versions.
func (b *RustBackend) RenderScene(target *gg.Pixmap, s *scene.Scene) error {
	b.mu.RLock()
	initialized := b.initialized
	b.mu.RUnlock()

	if !initialized {
		return ErrNotInitialized
	}

	if target == nil {
		return ErrNilTarget
	}

	if s == nil {
		return ErrNilScene
	}

	// TODO: Implement GPU scene rendering
	// For now, this is a stub that does nothing
	log.Printf("rust: RenderScene called (stub, %dx%d)", target.Width(), target.Height())

	return nil
}

// IsInitialized returns true if the backend has been initialized.
func (b *RustBackend) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized
}

// GPUInfoData returns information about the selected GPU.
// Returns nil if the backend is not initialized.
func (b *RustBackend) GPUInfoData() *GPUInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.gpuInfo
}

// Device returns the GPU device.
// Returns nil if the backend is not initialized.
func (b *RustBackend) Device() *wgpu.Device {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.device
}

// Queue returns the GPU queue.
// Returns nil if the backend is not initialized.
func (b *RustBackend) Queue() *wgpu.Queue {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.queue
}

// getGPUInfo retrieves information about the adapter.
func (b *RustBackend) getGPUInfo() *GPUInfo {
	if b.adapter == nil {
		return nil
	}

	info, err := b.adapter.GetInfo()
	if err != nil {
		return nil
	}

	return &GPUInfo{
		Vendor:       info.Vendor,
		Architecture: info.Architecture,
		Device:       info.Device,
		Description:  info.Description,
		BackendType:  backendTypeToString(info.BackendType),
		AdapterType:  adapterTypeToString(info.AdapterType),
		VendorID:     info.VendorID,
		DeviceID:     info.DeviceID,
	}
}

// logGPUInfo logs information about the selected GPU.
func (b *RustBackend) logGPUInfo() {
	if b.gpuInfo == nil {
		return
	}

	log.Printf("rust: GPU: %s (%s)", b.gpuInfo.Device, b.gpuInfo.Description)
	log.Printf("rust: Backend: %s, Type: %s", b.gpuInfo.BackendType, b.gpuInfo.AdapterType)
	log.Printf("rust: Vendor: %s (ID: 0x%04X), Device ID: 0x%04X",
		b.gpuInfo.Vendor, b.gpuInfo.VendorID, b.gpuInfo.DeviceID)
}

// backendTypeToString converts wgpu backend type to string.
func backendTypeToString(bt wgpu.BackendType) string {
	switch bt {
	case wgpu.BackendTypeNull:
		return "Null"
	case wgpu.BackendTypeWebGPU:
		return "WebGPU"
	case wgpu.BackendTypeD3D11:
		return "D3D11"
	case wgpu.BackendTypeD3D12:
		return "D3D12"
	case wgpu.BackendTypeMetal:
		return "Metal"
	case wgpu.BackendTypeVulkan:
		return "Vulkan"
	case wgpu.BackendTypeOpenGL:
		return "OpenGL"
	case wgpu.BackendTypeOpenGLES:
		return "OpenGLES"
	default:
		return "Unknown"
	}
}

// adapterTypeToString converts wgpu adapter type to string.
func adapterTypeToString(at wgpu.AdapterType) string {
	switch at {
	case wgpu.AdapterTypeDiscreteGPU:
		return "DiscreteGPU"
	case wgpu.AdapterTypeIntegratedGPU:
		return "IntegratedGPU"
	case wgpu.AdapterTypeCPU:
		return "CPU"
	default:
		return "Unknown"
	}
}

// GPURenderer is a GPU-backed renderer for immediate mode drawing.
// It implements the gg.Renderer interface.
//
// Note: This is a stub implementation. Full GPU rendering
// will be implemented in future versions.
type GPURenderer struct {
	backend          *RustBackend
	width            int
	height           int
	softwareRenderer *gg.SoftwareRenderer
}

// newGPURenderer creates a new GPU renderer.
func newGPURenderer(b *RustBackend, width, height int) *GPURenderer {
	return &GPURenderer{
		backend:          b,
		width:            width,
		height:           height,
		softwareRenderer: gg.NewSoftwareRenderer(width, height),
	}
}

// Fill fills a path with the given paint.
//
// Current Implementation:
// Uses software rasterization via SoftwareRenderer. Future versions will add
// GPU texture upload and native GPU path rendering.
func (r *GPURenderer) Fill(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Delegate to software renderer (stub implementation)
	if err := r.softwareRenderer.Fill(pixmap, path, paint); err != nil {
		return fmt.Errorf("fill: %w", err)
	}

	return nil
}

// Stroke strokes a path with the given paint.
//
// Current Implementation:
// Uses software rasterization via SoftwareRenderer. Future versions will add
// GPU texture upload and native GPU stroke expansion.
func (r *GPURenderer) Stroke(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Delegate to software renderer (stub implementation)
	if err := r.softwareRenderer.Stroke(pixmap, path, paint); err != nil {
		return fmt.Errorf("stroke: %w", err)
	}

	return nil
}

// Width returns the renderer width.
func (r *GPURenderer) Width() int {
	return r.width
}

// Height returns the renderer height.
func (r *GPURenderer) Height() int {
	return r.height
}

// Close releases renderer resources.
func (r *GPURenderer) Close() {
	// TODO: Release GPU resources when implemented
}
