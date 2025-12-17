// Package scene provides a retained-mode scene graph encoding system
// for efficient GPU rendering.
//
// The encoding system uses a dual-stream architecture inspired by vello:
//   - A compact tags stream (1 byte per command)
//   - Separate data streams for paths, draws, and transforms
//
// This design is cache-friendly and GPU-ready, minimizing memory bandwidth
// and enabling efficient batched rendering.
package scene

// Tag represents a single-byte command identifier in the encoding stream.
// Tags are organized into groups by their high nibble:
//
//	0x0X: Transform operations
//	0x1X: Path commands
//	0x2X: Fill/Stroke operations
//	0x3X: Layer operations
//	0x4X: Clip operations
//	0x5X: Brush/Image operations
type Tag byte

// Tag constants define all encoding commands.
// Each tag has a fixed data layout documented in its comment.
const (
	// TagTransform encodes an affine transformation.
	// Data: 6 float32 values [a, b, c, d, e, f] representing the matrix:
	//   | a  b  c |
	//   | d  e  f |
	TagTransform Tag = 0x01

	// TagBeginPath marks the start of a new path.
	// Data: none (marker only)
	TagBeginPath Tag = 0x10

	// TagMoveTo moves the current point without drawing.
	// Data: 2 float32 values [x, y]
	TagMoveTo Tag = 0x11

	// TagLineTo draws a line to the specified point.
	// Data: 2 float32 values [x, y]
	TagLineTo Tag = 0x12

	// TagQuadTo draws a quadratic Bezier curve.
	// Data: 4 float32 values [cx, cy, x, y] (control point, end point)
	TagQuadTo Tag = 0x13

	// TagCubicTo draws a cubic Bezier curve.
	// Data: 6 float32 values [c1x, c1y, c2x, c2y, x, y] (control1, control2, end)
	TagCubicTo Tag = 0x14

	// TagClosePath closes the current subpath.
	// Data: none (uses implicit return to subpath start)
	TagClosePath Tag = 0x16

	// TagEndPath marks the end of a path definition.
	// Data: none (marker only)
	TagEndPath Tag = 0x17

	// TagFill fills the current path.
	// Data: 1 uint32 for brush index, 1 uint32 for fill style (NonZero=0, EvenOdd=1)
	TagFill Tag = 0x20

	// TagStroke strokes the current path.
	// Data: 1 uint32 for brush index, then stroke style:
	//   4 float32: [lineWidth, miterLimit, lineCap, lineJoin]
	TagStroke Tag = 0x21

	// TagPushLayer pushes a new compositing layer.
	// Data: 1 uint32 for blend mode, 1 float32 for alpha
	TagPushLayer Tag = 0x30

	// TagPopLayer pops the current compositing layer.
	// Data: none
	TagPopLayer Tag = 0x31

	// TagBeginClip begins a clipping region using the current path.
	// Data: none (uses current path as clip)
	TagBeginClip Tag = 0x40

	// TagEndClip ends the current clipping region.
	// Data: none
	TagEndClip Tag = 0x41

	// TagBrush defines a brush (solid color, gradient, etc.).
	// Data: variable depending on brush type
	//   Solid: 4 float32 [r, g, b, a]
	TagBrush Tag = 0x50

	// TagImage references an image resource.
	// Data: 1 uint32 for image index, 6 float32 for transform
	TagImage Tag = 0x51
)

// String returns a human-readable name for the tag.
func (t Tag) String() string {
	switch t {
	case TagTransform:
		return "Transform"
	case TagBeginPath:
		return "BeginPath"
	case TagMoveTo:
		return "MoveTo"
	case TagLineTo:
		return "LineTo"
	case TagQuadTo:
		return "QuadTo"
	case TagCubicTo:
		return "CubicTo"
	case TagClosePath:
		return "ClosePath"
	case TagEndPath:
		return "EndPath"
	case TagFill:
		return "Fill"
	case TagStroke:
		return "Stroke"
	case TagPushLayer:
		return "PushLayer"
	case TagPopLayer:
		return "PopLayer"
	case TagBeginClip:
		return "BeginClip"
	case TagEndClip:
		return "EndClip"
	case TagBrush:
		return "Brush"
	case TagImage:
		return "Image"
	default:
		return "Unknown"
	}
}

// IsPathCommand returns true if the tag is a path construction command.
func (t Tag) IsPathCommand() bool {
	return t >= TagBeginPath && t <= TagEndPath
}

// IsDrawCommand returns true if the tag is a draw command (fill/stroke).
func (t Tag) IsDrawCommand() bool {
	return t == TagFill || t == TagStroke
}

// IsLayerCommand returns true if the tag is a layer command.
func (t Tag) IsLayerCommand() bool {
	return t == TagPushLayer || t == TagPopLayer
}

// IsClipCommand returns true if the tag is a clip command.
func (t Tag) IsClipCommand() bool {
	return t == TagBeginClip || t == TagEndClip
}

// DataSize returns the number of float32 values this tag consumes from pathData.
// Returns -1 for tags that don't use pathData.
func (t Tag) DataSize() int {
	switch t {
	case TagTransform:
		return 6
	case TagMoveTo, TagLineTo:
		return 2
	case TagQuadTo:
		return 4
	case TagCubicTo:
		return 6
	case TagBrush:
		return 4 // RGBA
	default:
		return 0
	}
}
