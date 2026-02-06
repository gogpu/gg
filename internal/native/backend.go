package native

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// BackendNative is the identifier for the native GPU backend.
const BackendNative = "native"

// NativeBackend is a GPU-accelerated rendering backend using gogpu/wgpu.
//
// The backend manages GPU resources including instance, adapter, device,
// and queue. It supports both immediate mode rendering (via NewRenderer)
// and retained mode rendering (via RenderScene).
type NativeBackend struct {
	mu sync.RWMutex

	// GPU resources
	instance *core.Instance
	adapter  core.AdapterID
	device   core.DeviceID
	queue    core.QueueID

	// GPU information
	gpuInfo *GPUInfo

	// State
	initialized bool
}

// NewNativeBackend creates a new Pure Go GPU rendering backend.
// The backend must be initialized with Init() before use.
func NewNativeBackend() *NativeBackend {
	return &NativeBackend{}
}

// Name returns the backend identifier.
func (b *NativeBackend) Name() string {
	return BackendNative
}

// Init initializes the backend by creating GPU resources.
// This includes creating an instance, requesting an adapter,
// creating a device, and getting the command queue.
//
// Returns an error if GPU initialization fails.
func (b *NativeBackend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized {
		return nil
	}

	// Step 1: Create Instance
	desc := &gputypes.InstanceDescriptor{
		Backends: gputypes.BackendsPrimary,
		Flags:    0,
	}
	b.instance = core.NewInstance(desc)

	// Step 2: Request Adapter (prefer high performance GPU)
	adapterID, err := b.instance.RequestAdapter(&gputypes.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGPU, err)
	}
	b.adapter = adapterID

	// Log GPU information
	logGPUInfo(adapterID)

	// Get GPU info for later use
	b.gpuInfo, _ = getGPUInfo(adapterID)

	// Step 3: Create Device
	deviceID, err := createDevice(adapterID, "gg-wgpu-device")
	if err != nil {
		return fmt.Errorf("device creation failed: %w", err)
	}
	b.device = deviceID

	// Step 4: Get Queue
	queueID, err := getDeviceQueue(deviceID)
	if err != nil {
		// Cleanup on failure
		_ = releaseDevice(deviceID)
		return fmt.Errorf("queue retrieval failed: %w", err)
	}
	b.queue = queueID

	b.initialized = true
	log.Println("native: backend initialized successfully")

	return nil
}

// Close releases all backend resources.
// The backend should not be used after Close is called.
func (b *NativeBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return
	}

	// Release resources in reverse order of creation
	// Note: Queue is released when device is dropped

	if !b.device.IsZero() {
		if err := releaseDevice(b.device); err != nil {
			log.Printf("native: error releasing device: %v", err)
		}
		b.device = core.DeviceID{}
	}

	if !b.adapter.IsZero() {
		if err := releaseAdapter(b.adapter); err != nil {
			log.Printf("native: error releasing adapter: %v", err)
		}
		b.adapter = core.AdapterID{}
	}

	// Instance doesn't need explicit cleanup in the current implementation
	b.instance = nil
	b.queue = core.QueueID{}
	b.gpuInfo = nil
	b.initialized = false

	log.Println("native: backend closed")
}

// NewRenderer creates a renderer for immediate mode rendering.
// The renderer is sized for the given dimensions.
//
// Note: This is a stub implementation that returns a GPURenderer.
// The actual GPU rendering will be implemented in TASK-110.
func (b *NativeBackend) NewRenderer(width, height int) gg.Renderer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.initialized {
		log.Println("native: warning - creating renderer on uninitialized backend")
		return nil
	}

	if width <= 0 || height <= 0 {
		log.Printf("native: warning - invalid dimensions: %dx%d", width, height)
		return nil
	}

	return newGPURenderer(b, width, height)
}

// RenderScene renders a scene to the target pixmap using retained mode.
// This method is optimized for complex scenes with many draw operations.
//
// The implementation uses GPUSceneRenderer for tessellation, strip
// rasterization, and layer compositing on the GPU. When wgpu texture
// readback is fully implemented, results will be downloaded to the target
// pixmap. Currently, data flows through the GPU pipeline as stubs.
func (b *NativeBackend) RenderScene(target *gg.Pixmap, s *scene.Scene) error {
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

	// Create GPU scene renderer for this frame
	renderer, err := NewGPUSceneRenderer(b, GPUSceneRendererConfig{
		Width:  target.Width(),
		Height: target.Height(),
	})
	if err != nil {
		return fmt.Errorf("failed to create GPU renderer: %w", err)
	}
	defer renderer.Close()

	// Render the scene to GPU
	if err := renderer.RenderToPixmap(target, s); err != nil {
		// ErrTextureReadbackNotSupported is expected until wgpu implements readback
		// In this case, the GPU pipeline was executed but we can't retrieve results
		if errors.Is(err, ErrTextureReadbackNotSupported) {
			// Log for debugging but don't fail - GPU ops were executed
			log.Printf("native: RenderScene completed on GPU (readback pending wgpu support)")
			return nil
		}
		return fmt.Errorf("GPU render failed: %w", err)
	}

	return nil
}

// IsInitialized returns true if the backend has been initialized.
func (b *NativeBackend) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized
}

// GPUInfo returns information about the selected GPU.
// Returns nil if the backend is not initialized.
func (b *NativeBackend) GPUInfo() *GPUInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.gpuInfo
}

// Device returns the GPU device ID.
// Returns a zero ID if the backend is not initialized.
func (b *NativeBackend) Device() core.DeviceID {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.device
}

// Queue returns the GPU queue ID.
// Returns a zero ID if the backend is not initialized.
func (b *NativeBackend) Queue() core.QueueID {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.queue
}

// GPURenderer is a GPU-backed renderer for immediate mode drawing.
// It implements the gg.Renderer interface.
//
// Note: This is a stub implementation. The actual GPU rendering
// will be implemented in TASK-110.
type GPURenderer struct {
	backend          *NativeBackend
	width            int
	height           int
	softwareRenderer *gg.SoftwareRenderer
}

// newGPURenderer creates a new GPU renderer.
func newGPURenderer(b *NativeBackend, width, height int) *GPURenderer {
	return &GPURenderer{
		backend:          b,
		width:            width,
		height:           height,
		softwareRenderer: gg.NewSoftwareRenderer(width, height),
	}
}

// Fill fills a path with the given paint.
//
// Phase 1 Implementation:
// Uses software rasterization via SoftwareRenderer. Future phases will add
// GPU texture upload and native GPU path rendering.
func (r *GPURenderer) Fill(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Phase 1: Delegate to software renderer
	if err := r.softwareRenderer.Fill(pixmap, path, paint); err != nil {
		return fmt.Errorf("fill: %w", err)
	}

	// TODO Phase 2: Upload pixmap to GPU texture for compositing
	// TODO Phase 3: Native GPU path tessellation

	return nil
}

// Stroke strokes a path with the given paint.
//
// Phase 1 Implementation:
// Uses software rasterization via SoftwareRenderer. Future phases will add
// GPU texture upload and native GPU stroke expansion.
func (r *GPURenderer) Stroke(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Phase 1: Delegate to software renderer
	if err := r.softwareRenderer.Stroke(pixmap, path, paint); err != nil {
		return fmt.Errorf("stroke: %w", err)
	}

	// TODO Phase 2: Upload pixmap to GPU texture for compositing
	// TODO Phase 3: Native GPU stroke expansion and tessellation

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
// Note: This is a stub implementation.
func (r *GPURenderer) Close() {
	// TODO: Release GPU resources in TASK-110
}
