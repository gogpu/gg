package text

// Face represents a font face at a specific size.
// This is a lightweight object that can be created from a FontSource.
// Face is the interface that will be fully implemented in TASK-043.
//
// For now, this is a stub to satisfy the FontSource.Face() method.
type Face interface {
	// private prevents external implementation
	private()
}

// sourceFace is the internal implementation of Face.
// This is a stub implementation for TASK-042.
type sourceFace struct {
	source *FontSource
	size   float64
	config faceConfig
}

// private implements the Face interface.
func (f *sourceFace) private() {}
