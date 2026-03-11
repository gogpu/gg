package gg

// ContextOption configures a Context during creation.
// Use functional options to customize Context behavior.
//
// Example:
//
//	// Default software rendering
//	dc := gg.NewContext(800, 600)
//
//	// Custom GPU renderer (dependency injection)
//	dc := gg.NewContext(800, 600, gg.WithRenderer(gpuRenderer))
type ContextOption func(*contextOptions)

// contextOptions holds optional configuration for Context creation.
type contextOptions struct {
	renderer     Renderer
	pixmap       *Pixmap
	pipelineMode PipelineMode
	deviceScale  float64
}

// defaultOptions returns the default context options.
func defaultOptions() contextOptions {
	return contextOptions{
		renderer:     nil,              // Will be set to SoftwareRenderer if nil
		pixmap:       nil,              // Will be created if nil
		pipelineMode: PipelineModeAuto, // Auto-select pipeline
		deviceScale:  1.0,              // No HiDPI scaling by default
	}
}

// WithRenderer sets a custom renderer for the Context.
// Use this for dependency injection of GPU or custom renderers.
//
// Example:
//
//	// Using a custom renderer
//	customRenderer := mypackage.NewRenderer()
//	dc := gg.NewContext(800, 600, gg.WithRenderer(customRenderer))
//
// For GPU-accelerated rendering, see gg's gpu backend (internal/gpu/)
// which uses gogpu/wgpu directly.
func WithRenderer(r Renderer) ContextOption {
	return func(o *contextOptions) {
		o.renderer = r
	}
}

// WithPixmap sets a custom pixmap for the Context.
// The pixmap dimensions should match the Context dimensions.
//
// Example:
//
//	pm := gg.NewPixmap(800, 600)
//	dc := gg.NewContext(800, 600, gg.WithPixmap(pm))
func WithPixmap(pm *Pixmap) ContextOption {
	return func(o *contextOptions) {
		o.pixmap = pm
	}
}

// WithPipelineMode sets the GPU pipeline mode for the Context.
// Default is PipelineModeAuto, which lets the framework choose.
//
// Example:
//
//	// Force compute pipeline
//	dc := gg.NewContext(800, 600, gg.WithPipelineMode(gg.PipelineModeCompute))
func WithPipelineMode(mode PipelineMode) ContextOption {
	return func(o *contextOptions) {
		o.pipelineMode = mode
	}
}

// WithDeviceScale sets the HiDPI device scale factor for the Context.
// The scale factor determines the ratio between logical coordinates (used by
// drawing code) and physical pixels (in the internal pixmap).
//
// For example, on a macOS Retina display with 2x scaling:
//
//	// Logical size: 800x600, Physical pixmap: 1600x1200
//	dc := gg.NewContext(800, 600, gg.WithDeviceScale(2.0))
//	dc.Width()      // 800 (logical)
//	dc.PixelWidth() // 1600 (physical)
//
// The Context automatically applies a base scale transform so all drawing
// operations work in logical coordinates while rendering at physical resolution.
// Default is 1.0 (no scaling).
func WithDeviceScale(scale float64) ContextOption {
	return func(o *contextOptions) {
		if scale > 0 {
			o.deviceScale = scale
		}
	}
}
