package backend

import (
	"errors"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// Common backend errors.
var (
	// ErrBackendNotAvailable is returned when a requested backend is not available.
	ErrBackendNotAvailable = errors.New("backend: not available")

	// ErrNotInitialized is returned when operations are called before Init.
	ErrNotInitialized = errors.New("backend: not initialized")
)

// RenderBackend is the interface for rendering backends.
// It abstracts the rendering implementation, allowing the library to
// support multiple backends (software, GPU via wgpu, etc.).
//
// Backends must be registered via Register() and are selected via
// Get() or Default().
type RenderBackend interface {
	// Name returns the backend identifier (e.g., "software", "wgpu").
	Name() string

	// Init initializes the backend.
	// This should be called before any rendering operations.
	Init() error

	// Close releases all backend resources.
	// The backend should not be used after Close is called.
	Close()

	// NewRenderer creates a renderer for immediate mode rendering.
	// The renderer is sized for the given dimensions and can be
	// used with gg.Context for drawing operations.
	NewRenderer(width, height int) gg.Renderer

	// RenderScene renders a scene to the target pixmap using retained mode.
	// This is more efficient for complex scenes as it can optimize
	// the rendering pipeline.
	RenderScene(target *gg.Pixmap, s *scene.Scene) error
}
