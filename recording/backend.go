package recording

import (
	"image"
	"io"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// Backend is the interface that all export backends must implement.
// Backends receive high-level drawing commands and translate them to
// their output format (raster pixels, PDF content streams, SVG elements, etc.).
//
// A Backend manages its own state stack for Save/Restore operations and
// handles coordinate transformations as needed (e.g., PDF Y-flip).
//
// Backends are created via the registry using NewBackend(name) and
// registered via Register() in their init() functions.
//
// # Implementation Contract
//
// Each backend must:
//  1. Register in init() using recording.Register()
//  2. Handle all Backend methods (even if no-op for some)
//  3. Manage own state stack for Save/Restore
//  4. Translate coordinates if needed (e.g., PDF Y-flip)
//
// # Example Backend Registration
//
//	func init() {
//	    recording.Register("pdf", func() recording.Backend {
//	        return NewPDFBackend()
//	    })
//	}
type Backend interface {
	// Lifecycle methods

	// Begin initializes the backend for rendering at the given dimensions.
	// This must be called before any drawing operations.
	// Returns an error if initialization fails.
	Begin(width, height int) error

	// End finalizes the rendering and prepares the output.
	// After End is called, output methods (WriteTo, SaveToFile) can be used.
	// Returns an error if finalization fails.
	End() error

	// State management methods

	// Save saves the current graphics state (transform, clip) onto a stack.
	// The state can be restored with Restore.
	Save()

	// Restore restores the graphics state from the stack.
	// If the stack is empty, this is a no-op.
	Restore()

	// Transform methods

	// SetTransform sets the current transformation matrix.
	// This replaces any existing transform.
	SetTransform(m Matrix)

	// Clipping methods

	// SetClip sets the clipping region to the given path.
	// Only pixels inside the path (according to the fill rule) will be drawn.
	// The clip is intersected with any existing clip.
	SetClip(path *gg.Path, rule FillRule)

	// ClearClip removes any clipping region.
	ClearClip()

	// Drawing methods

	// FillPath fills the given path with the brush color/pattern.
	// The rule determines how to handle self-intersecting paths.
	FillPath(path *gg.Path, brush Brush, rule FillRule)

	// StrokePath strokes the given path with the brush and stroke style.
	StrokePath(path *gg.Path, brush Brush, stroke Stroke)

	// FillRect is an optimized method for filling axis-aligned rectangles.
	// Backends may implement this more efficiently than FillPath with a rect path.
	FillRect(rect Rect, brush Brush)

	// DrawImage draws an image from the source rectangle to the destination rectangle.
	// The opts parameter specifies interpolation quality and alpha.
	DrawImage(img image.Image, src, dst Rect, opts ImageOptions)

	// DrawText draws text at the given position with the specified font face and brush.
	// The position (x, y) is the baseline origin of the text.
	DrawText(s string, x, y float64, face text.Face, brush Brush)
}

// WriterBackend extends Backend with the ability to write output to an io.Writer.
// This is useful for streaming output or writing to network connections.
type WriterBackend interface {
	Backend

	// WriteTo writes the rendered content to the given writer.
	// This should only be called after End().
	// Returns the number of bytes written and any error.
	WriteTo(w io.Writer) (int64, error)
}

// FileBackend extends Backend with the ability to save output directly to a file.
// This is a convenience interface for backends that can optimize file output.
type FileBackend interface {
	Backend

	// SaveToFile saves the rendered content to a file at the given path.
	// This should only be called after End().
	// Returns an error if the file cannot be written.
	SaveToFile(path string) error
}

// PixmapBackend extends Backend with access to a rasterized pixmap.
// This is implemented by the raster backend and allows direct pixel access.
type PixmapBackend interface {
	Backend

	// Pixmap returns the rendered pixmap.
	// This should only be called after End().
	// Returns nil if no pixmap is available.
	Pixmap() *gg.Pixmap
}
