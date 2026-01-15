package backend

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// Backend name constants.
const (
	// BackendSoftware is the name of the CPU-based software backend.
	BackendSoftware = "software"
	// BackendNative is the name of the Pure Go GPU backend (gogpu/wgpu).
	BackendNative = "native"
	// BackendRust is the name of the Rust GPU backend (go-webgpu/webgpu FFI).
	BackendRust = "rust"
)

// SoftwareBackend is a CPU-based rendering backend.
// It wraps the existing SoftwareRenderer for immediate mode
// and scene.Renderer for retained mode rendering.
type SoftwareBackend struct {
	initialized   bool
	sceneRenderer *scene.Renderer
	width         int
	height        int
}

// init registers the software backend on package import.
func init() {
	Register(BackendSoftware, func() RenderBackend {
		return &SoftwareBackend{}
	})
}

// NewSoftwareBackend creates a new software rendering backend.
func NewSoftwareBackend() *SoftwareBackend {
	return &SoftwareBackend{}
}

// Name returns the backend identifier.
func (b *SoftwareBackend) Name() string {
	return BackendSoftware
}

// Init initializes the backend.
func (b *SoftwareBackend) Init() error {
	b.initialized = true
	return nil
}

// Close releases all backend resources.
func (b *SoftwareBackend) Close() {
	if b.sceneRenderer != nil {
		b.sceneRenderer.Close()
		b.sceneRenderer = nil
	}
	b.initialized = false
}

// NewRenderer creates a renderer for immediate mode rendering.
// This wraps gg.NewSoftwareRenderer to provide a gg.Renderer interface.
func (b *SoftwareBackend) NewRenderer(width, height int) gg.Renderer {
	return gg.NewSoftwareRenderer(width, height)
}

// RenderScene renders a scene to the target pixmap using retained mode.
// This uses parallel tile-based rendering for efficiency.
func (b *SoftwareBackend) RenderScene(target *gg.Pixmap, s *scene.Scene) error {
	if target == nil || s == nil {
		return nil
	}

	width := target.Width()
	height := target.Height()

	// Create or resize scene renderer as needed
	if b.sceneRenderer == nil || b.width != width || b.height != height {
		if b.sceneRenderer != nil {
			b.sceneRenderer.Close()
		}
		b.sceneRenderer = scene.NewRenderer(width, height)
		b.width = width
		b.height = height
	}

	return b.sceneRenderer.Render(target, s)
}

// SceneRenderer returns the underlying scene renderer for advanced usage.
// Returns nil if no scene has been rendered yet.
func (b *SoftwareBackend) SceneRenderer() *scene.Renderer {
	return b.sceneRenderer
}
