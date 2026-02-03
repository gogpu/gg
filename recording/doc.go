// Package recording provides a command-based drawing recording system for gg.
//
// The recording system captures drawing operations as commands that can be
// played back to different backends, enabling vector export (PDF, SVG) and
// other transformations of 2D graphics.
//
// # Architecture
//
// The system follows a Command Pattern with three main components:
//
//   - Recorder: Captures drawing operations as commands
//   - Recording: Stores commands and resources for playback
//   - Backend: Renders commands to a specific output format
//
// This design is inspired by Skia's SkPicture and Cairo's recording surface.
//
// # Basic Usage
//
// Record drawing operations using a Recorder:
//
//	rec := recording.NewRecorder(800, 600)
//
//	// Draw using the familiar Context-like API
//	rec.SetFillRGBA(1, 0, 0, 1) // Red
//	rec.DrawRectangle(100, 100, 200, 150)
//	rec.Fill()
//
//	rec.SetStrokeRGBA(0, 0, 1, 1) // Blue
//	rec.SetLineWidth(2)
//	rec.DrawCircle(400, 300, 50)
//	rec.Stroke()
//
//	// Finish recording
//	r := rec.FinishRecording()
//
// # Playback to Backends
//
// Play back recordings to different output formats:
//
//	// PDF output (requires github.com/gogpu/gg-pdf)
//	import _ "github.com/gogpu/gg-pdf"
//
//	pdfBackend, _ := recording.NewBackend("pdf")
//	r.Playback(pdfBackend)
//	pdfBackend.(recording.FileBackend).SaveToFile("output.pdf")
//
//	// SVG output (requires github.com/gogpu/gg-svg)
//	import _ "github.com/gogpu/gg-svg"
//
//	svgBackend, _ := recording.NewBackend("svg")
//	r.Playback(svgBackend)
//	svgBackend.(recording.FileBackend).SaveToFile("output.svg")
//
//	// Raster output (built-in)
//	rasterBackend, _ := recording.NewBackend("raster")
//	r.Playback(rasterBackend)
//	rasterBackend.(recording.PixmapBackend).Pixmap() // Get pixel data
//
// # Backend Registration
//
// Backends are registered using the database/sql driver pattern. Import a
// backend package with a blank identifier to automatically register it:
//
//	import (
//	    "github.com/gogpu/gg/recording"
//	    _ "github.com/gogpu/gg-pdf"  // Registers "pdf" backend
//	    _ "github.com/gogpu/gg-svg"  // Registers "svg" backend
//	)
//
// The built-in "raster" backend is always available via:
//
//	import _ "github.com/gogpu/gg/recording/backends/raster"
//
// # Resource Management
//
// The recording system uses reference-based resource pooling for efficient
// storage and playback:
//
//   - Paths are cloned and stored with PathRef references
//   - Brushes (solid colors, gradients) are stored with BrushRef references
//   - Images are stored with ImageRef references
//   - Fonts are stored with FontRef references
//
// This ensures recordings are immutable and can be safely played back
// multiple times to different backends.
//
// # Supported Operations
//
// The Recorder supports the full gg drawing API:
//
//   - Path operations: MoveTo, LineTo, QuadraticTo, CubicTo, ClosePath
//   - Shape helpers: DrawRectangle, DrawRoundedRectangle, DrawCircle, DrawEllipse, DrawArc, DrawEllipticalArc
//   - Fill and stroke with solid colors and gradients
//   - Line styles: width, cap, join, miter limit, dash patterns
//   - Transformations: Translate, Rotate, Scale, matrix operations
//   - Clipping: path-based clipping with fill rules
//   - State management: Push/Pop (Save/Restore)
//   - Text rendering (font-dependent)
//   - Image drawing
//
// # Brushes
//
// The recording system supports multiple brush types for fills and strokes:
//
//   - SolidBrush: Single solid color
//   - LinearGradientBrush: Linear color gradient
//   - RadialGradientBrush: Radial color gradient
//   - SweepGradientBrush: Angular/conic gradient
//
// Example gradient usage:
//
//	grad := recording.NewLinearGradientBrush(0, 0, 200, 200).
//	    AddColorStop(0, gg.RGBA{R: 1, G: 0, B: 0, A: 1}).
//	    AddColorStop(1, gg.RGBA{R: 0, G: 0, B: 1, A: 1})
//
//	rec.SetFillStyle(grad)
//	rec.Fill()
//
// # Custom Backends
//
// Implement the [Backend] interface to create custom output formats:
//
//	type Backend interface {
//	    Begin(width, height int) error
//	    End() error
//	    Save()
//	    Restore()
//	    SetTransform(m Matrix)
//	    SetClip(path *gg.Path, rule FillRule)
//	    ClearClip()
//	    FillPath(path *gg.Path, brush Brush, rule FillRule)
//	    StrokePath(path *gg.Path, brush Brush, stroke Stroke)
//	    FillRect(rect Rect, brush Brush)
//	    DrawImage(img image.Image, src, dst Rect, opts ImageOptions)
//	    DrawText(s string, x, y float64, face text.Face, brush Brush)
//	}
//
// Register custom backends with [Register]:
//
//	func init() {
//	    recording.Register("myformat", func() recording.Backend {
//	        return NewMyBackend()
//	    })
//	}
//
// # Thread Safety
//
// Recorder is NOT safe for concurrent use. Each goroutine should use its own
// Recorder instance. However, Recording objects are immutable after Finish()
// and can be safely shared and played back from multiple goroutines.
//
// # Performance Considerations
//
// The recording system is optimized for:
//
//   - Minimal allocations during recording (pre-allocated slices)
//   - Efficient resource deduplication via pooling
//   - Fast playback with direct command dispatch
//
// For real-time rendering, prefer direct gg.Context usage. Use recording
// when you need vector export or command inspection.
package recording
