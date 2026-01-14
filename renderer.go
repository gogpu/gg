package gg

// Renderer is the interface for rendering paths to a pixmap.
type Renderer interface {
	// Fill fills a path with the given paint.
	// Returns an error if the rendering operation fails.
	Fill(pixmap *Pixmap, path *Path, paint *Paint) error

	// Stroke strokes a path with the given paint.
	// Returns an error if the rendering operation fails.
	Stroke(pixmap *Pixmap, path *Path, paint *Paint) error
}
