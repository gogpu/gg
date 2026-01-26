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
	renderer       Renderer
	pixmap         *Pixmap
	analyticFiller AnalyticFillerInterface
}

// defaultOptions returns the default context options.
func defaultOptions() contextOptions {
	return contextOptions{
		renderer: nil, // Will be set to SoftwareRenderer if nil
		pixmap:   nil, // Will be created if nil
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
// For GPU-accelerated rendering, see gg's native backend (backend/native/)
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

// WithAnalyticAA enables analytic anti-aliasing for the Context.
// This requires providing an AnalyticFillerInterface implementation,
// typically from backend/native.NewAnalyticFillerAdapter().
//
// Analytic AA provides higher quality anti-aliasing than supersampling
// by computing exact geometric coverage for each pixel.
//
// Example:
//
//	import "github.com/gogpu/gg/backend/native"
//
//	adapter := native.NewAnalyticFillerAdapter(800, 600)
//	dc := gg.NewContext(800, 600, gg.WithAnalyticAA(adapter))
//
// Note: The adapter must be created with the same dimensions as the Context.
func WithAnalyticAA(filler AnalyticFillerInterface) ContextOption {
	return func(o *contextOptions) {
		if o.renderer == nil {
			// Will be set up in NewContext when renderer is created
			o.analyticFiller = filler
		}
	}
}
