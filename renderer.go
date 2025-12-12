package gg

// Renderer is the interface for rendering paths to a pixmap.
type Renderer interface {
	// Fill fills a path with the given paint.
	Fill(pixmap *Pixmap, path *Path, paint *Paint)

	// Stroke strokes a path with the given paint.
	Stroke(pixmap *Pixmap, path *Path, paint *Paint)
}
