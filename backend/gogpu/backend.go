package gogpu

import (
	"fmt"
	"log"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
	"github.com/gogpu/gogpu/gpu"
	"github.com/gogpu/gogpu/gpu/types"
)

// BackendGoGPU is the name of the gogpu backend.
const BackendGoGPU = "gogpu"

// Backend is a GPU-accelerated rendering backend using gogpu/gogpu.
// It implements the backend.RenderBackend interface.
//
// This backend uses gogpu's gpu.Backend interface, which supports both
// Rust (wgpu-native) and Pure Go (gogpu/wgpu) implementations. The active
// backend is selected based on build tags or explicit registration.
//
// Backend is safe for concurrent use from multiple goroutines.
type Backend struct {
	mu sync.RWMutex

	// GPU resources via gogpu
	gpuBackend gpu.Backend
	instance   types.Instance
	adapter    types.Adapter
	device     types.Device
	queue      types.Queue

	// Scene rendering (Phase 1: software fallback)
	sceneRenderer *scene.Renderer
	sceneWidth    int
	sceneHeight   int

	// State
	initialized bool
}

// NewBackend creates a new gogpu rendering backend.
// The backend must be initialized with Init() before use.
func NewBackend() *Backend {
	return &Backend{}
}

// Name returns the backend identifier.
func (b *Backend) Name() string {
	return BackendGoGPU
}

// Init initializes the backend by creating GPU resources.
// This includes:
//   - Getting the active gogpu backend (Rust or Pure Go)
//   - Creating a WebGPU instance
//   - Requesting a GPU adapter
//   - Creating a logical device
//   - Getting the command queue
//
// Returns an error if GPU initialization fails.
func (b *Backend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized {
		return nil
	}

	// Step 1: Get gogpu backend
	gpuBackend := gpu.GetBackend()
	if gpuBackend == nil {
		// Try to initialize default backend
		if err := gpu.InitDefaultBackend(); err != nil {
			return fmt.Errorf("%w: %w", ErrNoGPUBackend, err)
		}
		gpuBackend = gpu.GetBackend()
	}
	if gpuBackend == nil {
		return ErrNoGPUBackend
	}
	b.gpuBackend = gpuBackend

	log.Printf("gogpu: using GPU backend: %s", gpuBackend.Name())

	// Step 2: Create Instance
	instance, err := gpuBackend.CreateInstance()
	if err != nil {
		return fmt.Errorf("instance creation failed: %w", err)
	}
	b.instance = instance

	// Step 3: Request Adapter (prefer high performance GPU)
	adapter, err := gpuBackend.RequestAdapter(instance, &types.AdapterOptions{
		PowerPreference: types.PowerPreferenceHighPerformance,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGPUBackend, err)
	}
	b.adapter = adapter

	// Step 4: Create Device
	device, err := gpuBackend.RequestDevice(adapter, &types.DeviceOptions{
		Label: "gg-gogpu-device",
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDeviceCreationFailed, err)
	}
	b.device = device

	// Step 5: Get Queue
	b.queue = gpuBackend.GetQueue(device)

	b.initialized = true
	log.Printf("gogpu: backend initialized successfully")

	return nil
}

// Close releases all backend resources.
// The backend should not be used after Close is called.
func (b *Backend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return
	}

	// Close scene renderer if exists
	if b.sceneRenderer != nil {
		b.sceneRenderer.Close()
		b.sceneRenderer = nil
	}

	// Resources are released when gogpu backend is destroyed
	// Individual handles don't need explicit release in most cases
	// as they are managed by the backend

	b.device = 0
	b.adapter = 0
	b.instance = 0
	b.queue = 0
	b.gpuBackend = nil
	b.initialized = false

	log.Println("gogpu: backend closed")
}

// NewRenderer creates a renderer for immediate mode rendering.
// The renderer is sized for the given dimensions.
//
// Returns a gg.Renderer that can be used with gg.Context for drawing.
func (b *Backend) NewRenderer(width, height int) gg.Renderer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.initialized {
		log.Println("gogpu: warning - creating renderer on uninitialized backend")
		return nil
	}

	if width <= 0 || height <= 0 {
		log.Printf("gogpu: warning - invalid dimensions: %dx%d", width, height)
		return nil
	}

	return newGPURenderer(b, width, height)
}

// RenderScene renders a scene to the target pixmap using retained mode.
//
// Phase 1 Implementation:
// Uses software scene rendering via scene.Renderer. Future phases will add
// GPU-accelerated scene rendering with tessellation and compute shaders.
func (b *Backend) RenderScene(target *gg.Pixmap, s *scene.Scene) error {
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

	width := target.Width()
	height := target.Height()

	// Phase 1: Use software scene renderer
	b.mu.Lock()
	if b.sceneRenderer == nil || b.sceneWidth != width || b.sceneHeight != height {
		if b.sceneRenderer != nil {
			b.sceneRenderer.Close()
		}
		b.sceneRenderer = scene.NewRenderer(width, height)
		b.sceneWidth = width
		b.sceneHeight = height
	}
	renderer := b.sceneRenderer
	b.mu.Unlock()

	// TODO Phase 2: GPU scene tessellation and rendering
	// TODO Phase 3: Compute shader pipeline integration

	return renderer.Render(target, s)
}

// IsInitialized returns true if the backend has been initialized.
func (b *Backend) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized
}

// GPUBackend returns the underlying gogpu GPU backend.
// Returns nil if the backend is not initialized.
func (b *Backend) GPUBackend() gpu.Backend {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.gpuBackend
}

// Device returns the GPU device handle.
// Returns 0 if the backend is not initialized.
func (b *Backend) Device() types.Device {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.device
}

// Queue returns the GPU queue handle.
// Returns 0 if the backend is not initialized.
func (b *Backend) Queue() types.Queue {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.queue
}
