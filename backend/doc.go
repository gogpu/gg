// Package backend provides a pluggable rendering backend abstraction.
//
// The backend package allows the gg library to support multiple rendering
// implementations. Currently, only the software backend is available, but
// this architecture enables future GPU-accelerated backends via gogpu/wgpu.
//
// # Backend Registration
//
// Backends are registered via init() functions and selected at runtime.
// The software backend is automatically registered on import:
//
//	import _ "github.com/gogpu/gg/backend"
//
// # Backend Selection
//
// Use Default() to get the best available backend, or Get() to request
// a specific backend by name:
//
//	// Get the default (best available) backend
//	b := backend.Default()
//
//	// Or request a specific backend
//	b := backend.Get("software")
//
// # Usage with Context
//
// The backend provides renderers that implement gg.Renderer:
//
//	b := backend.Default()
//	if err := b.Init(); err != nil {
//		log.Fatal(err)
//	}
//	defer b.Close()
//
//	renderer := b.NewRenderer(800, 600)
//
// # Retained Mode Rendering
//
// For complex scenes, use RenderScene for optimized rendering:
//
//	scene := scene.NewScene()
//	scene.Fill(scene.FillNonZero, scene.IdentityAffine(),
//		scene.SolidBrush(gg.Red), circle)
//
//	pixmap := gg.NewPixmap(800, 600)
//	if err := b.RenderScene(pixmap, scene); err != nil {
//		log.Fatal(err)
//	}
//
// # Available Backends
//
// - "software": CPU-based scanline rasterizer (always available)
// - "wgpu": GPU-accelerated via gogpu/wgpu (future)
package backend
