// Package recording provides types for recording drawing operations.
//
// The recording system enables vector export (PDF, SVG) by capturing drawing
// operations as typed command structures instead of immediate rasterization.
// Commands are stored in a Recording and can be replayed to any backend.
//
// Design follows Cairo's approach of typed command structs for inspectability
// and debuggability, rather than Skia's binary serialization format.
//
// # Architecture
//
// Commands capture all drawing operations:
//   - State commands (Save, Restore, SetTransform, SetClip)
//   - Drawing commands (FillPath, StrokePath, FillRect, DrawImage, DrawText)
//   - Style commands (SetFillStyle, SetStrokeStyle, SetLineWidth, etc.)
//
// Resources (paths, brushes, images) are stored in a ResourcePool and
// referenced by typed handles (PathRef, BrushRef, ImageRef) to enable
// deduplication and efficient memory usage.
//
// # Example
//
//	// Record drawing operations
//	rec := recording.NewRecorder(800, 600)
//	rec.Save()
//	rec.SetTransform(matrix)
//	rec.FillPath(pathRef, brushRef, FillRuleNonZero)
//	rec.Restore()
//	r := rec.Finish()
//
//	// Replay to a backend
//	r.Playback(pdfBackend)
package recording

// CommandType identifies the type of a command.
// Each command type corresponds to a specific drawing operation.
type CommandType uint8

const (
	// State commands
	CmdSave         CommandType = iota // Save current state
	CmdRestore                         // Restore previous state
	CmdSetTransform                    // Set transformation matrix
	CmdSetClip                         // Set clipping region
	CmdClearClip                       // Clear clipping region

	// Drawing commands
	CmdFillPath   // Fill a path
	CmdStrokePath // Stroke a path
	CmdFillRect   // Fill a rectangle (optimized)
	CmdStrokeRect // Stroke a rectangle
	CmdDrawImage  // Draw an image
	CmdDrawText   // Draw text

	// Style commands
	CmdSetFillStyle   // Set fill brush
	CmdSetStrokeStyle // Set stroke brush
	CmdSetLineWidth   // Set stroke line width
	CmdSetLineCap     // Set stroke line cap
	CmdSetLineJoin    // Set stroke line join
	CmdSetMiterLimit  // Set miter limit
	CmdSetDash        // Set dash pattern
	CmdSetFillRule    // Set fill rule
)

// commandTypeNames maps CommandType values to their string representation.
var commandTypeNames = [...]string{
	CmdSave:           "Save",
	CmdRestore:        "Restore",
	CmdSetTransform:   "SetTransform",
	CmdSetClip:        "SetClip",
	CmdClearClip:      "ClearClip",
	CmdFillPath:       "FillPath",
	CmdStrokePath:     "StrokePath",
	CmdFillRect:       "FillRect",
	CmdStrokeRect:     "StrokeRect",
	CmdDrawImage:      "DrawImage",
	CmdDrawText:       "DrawText",
	CmdSetFillStyle:   "SetFillStyle",
	CmdSetStrokeStyle: "SetStrokeStyle",
	CmdSetLineWidth:   "SetLineWidth",
	CmdSetLineCap:     "SetLineCap",
	CmdSetLineJoin:    "SetLineJoin",
	CmdSetMiterLimit:  "SetMiterLimit",
	CmdSetDash:        "SetDash",
	CmdSetFillRule:    "SetFillRule",
}

// String returns the string representation of a CommandType.
func (c CommandType) String() string {
	if int(c) < len(commandTypeNames) {
		return commandTypeNames[c]
	}
	return "Unknown"
}

// Command is the interface implemented by all command types.
// Commands represent individual drawing operations that can be
// serialized and replayed to different backends.
type Command interface {
	// Type returns the CommandType for this command.
	Type() CommandType
}

// --------------------------------------------------------------------------
// Reference Types
// --------------------------------------------------------------------------

// PathRef is a reference to a path in the resource pool.
// The zero value is a valid reference to the first path (if any).
type PathRef uint32

// BrushRef is a reference to a brush in the resource pool.
// The zero value is a valid reference to the first brush (if any).
type BrushRef uint32

// ImageRef is a reference to an image in the resource pool.
// The zero value is a valid reference to the first image (if any).
type ImageRef uint32

// InvalidRef is the sentinel value for an invalid reference.
// Use this to indicate that a reference does not point to a valid resource.
const InvalidRef = ^uint32(0)

// IsValid returns true if the reference points to a valid path.
func (r PathRef) IsValid() bool {
	return uint32(r) != InvalidRef
}

// IsValid returns true if the reference points to a valid brush.
func (r BrushRef) IsValid() bool {
	return uint32(r) != InvalidRef
}

// IsValid returns true if the reference points to a valid image.
func (r ImageRef) IsValid() bool {
	return uint32(r) != InvalidRef
}

// --------------------------------------------------------------------------
// State Commands
// --------------------------------------------------------------------------

// SaveCommand saves the current graphics state.
// The state includes transform, clip, fill style, stroke style, and line properties.
type SaveCommand struct{}

// Type implements Command.
func (SaveCommand) Type() CommandType { return CmdSave }

// RestoreCommand restores the previously saved graphics state.
type RestoreCommand struct{}

// Type implements Command.
func (RestoreCommand) Type() CommandType { return CmdRestore }

// SetTransformCommand sets the current transformation matrix.
type SetTransformCommand struct {
	// Matrix is the new transformation matrix.
	Matrix Matrix
}

// Type implements Command.
func (SetTransformCommand) Type() CommandType { return CmdSetTransform }

// SetClipCommand sets the clipping region.
type SetClipCommand struct {
	// Path references the clip path in the resource pool.
	Path PathRef
	// Rule specifies the fill rule for determining inside/outside.
	Rule FillRule
}

// Type implements Command.
func (SetClipCommand) Type() CommandType { return CmdSetClip }

// ClearClipCommand clears the clipping region to the full canvas.
type ClearClipCommand struct{}

// Type implements Command.
func (ClearClipCommand) Type() CommandType { return CmdClearClip }

// --------------------------------------------------------------------------
// Drawing Commands
// --------------------------------------------------------------------------

// FillPathCommand fills a path with a brush.
type FillPathCommand struct {
	// Path references the path to fill in the resource pool.
	Path PathRef
	// Brush references the fill brush in the resource pool.
	Brush BrushRef
	// Rule specifies the fill rule (non-zero or even-odd).
	Rule FillRule
}

// Type implements Command.
func (FillPathCommand) Type() CommandType { return CmdFillPath }

// StrokePathCommand strokes a path with a brush.
type StrokePathCommand struct {
	// Path references the path to stroke in the resource pool.
	Path PathRef
	// Brush references the stroke brush in the resource pool.
	Brush BrushRef
	// Stroke contains the stroke style (width, cap, join, dash).
	Stroke Stroke
}

// Type implements Command.
func (StrokePathCommand) Type() CommandType { return CmdStrokePath }

// FillRectCommand fills a rectangle with a brush.
// This is an optimization for the common case of axis-aligned rectangles.
type FillRectCommand struct {
	// Rect is the rectangle to fill.
	Rect Rect
	// Brush references the fill brush in the resource pool.
	Brush BrushRef
}

// Type implements Command.
func (FillRectCommand) Type() CommandType { return CmdFillRect }

// StrokeRectCommand strokes a rectangle with a brush.
type StrokeRectCommand struct {
	// Rect is the rectangle to stroke.
	Rect Rect
	// Brush references the stroke brush in the resource pool.
	Brush BrushRef
	// Stroke contains the stroke style.
	Stroke Stroke
}

// Type implements Command.
func (StrokeRectCommand) Type() CommandType { return CmdStrokeRect }

// DrawImageCommand draws an image.
type DrawImageCommand struct {
	// Image references the image in the resource pool.
	Image ImageRef
	// SrcRect is the source rectangle in image coordinates.
	// If zero, uses the entire image.
	SrcRect Rect
	// DstRect is the destination rectangle in canvas coordinates.
	DstRect Rect
	// Options contains rendering options (interpolation, etc.).
	Options ImageOptions
}

// Type implements Command.
func (DrawImageCommand) Type() CommandType { return CmdDrawImage }

// DrawTextCommand draws text at a specified position.
type DrawTextCommand struct {
	// Text is the string to render.
	Text string
	// X is the horizontal position.
	X float64
	// Y is the vertical position (baseline).
	Y float64
	// FontSize is the size of the font in points.
	FontSize float64
	// FontFamily is the name of the font family.
	FontFamily string
	// Brush references the text color/brush in the resource pool.
	Brush BrushRef
}

// Type implements Command.
func (DrawTextCommand) Type() CommandType { return CmdDrawText }

// --------------------------------------------------------------------------
// Style Commands
// --------------------------------------------------------------------------

// SetFillStyleCommand sets the fill brush.
type SetFillStyleCommand struct {
	// Brush references the fill brush in the resource pool.
	Brush BrushRef
}

// Type implements Command.
func (SetFillStyleCommand) Type() CommandType { return CmdSetFillStyle }

// SetStrokeStyleCommand sets the stroke brush.
type SetStrokeStyleCommand struct {
	// Brush references the stroke brush in the resource pool.
	Brush BrushRef
}

// Type implements Command.
func (SetStrokeStyleCommand) Type() CommandType { return CmdSetStrokeStyle }

// SetLineWidthCommand sets the stroke line width.
type SetLineWidthCommand struct {
	// Width is the line width in pixels.
	Width float64
}

// Type implements Command.
func (SetLineWidthCommand) Type() CommandType { return CmdSetLineWidth }

// SetLineCapCommand sets the stroke line cap style.
type SetLineCapCommand struct {
	// Cap is the line cap style.
	Cap LineCap
}

// Type implements Command.
func (SetLineCapCommand) Type() CommandType { return CmdSetLineCap }

// SetLineJoinCommand sets the stroke line join style.
type SetLineJoinCommand struct {
	// Join is the line join style.
	Join LineJoin
}

// Type implements Command.
func (SetLineJoinCommand) Type() CommandType { return CmdSetLineJoin }

// SetMiterLimitCommand sets the miter limit for line joins.
type SetMiterLimitCommand struct {
	// Limit is the miter limit value.
	Limit float64
}

// Type implements Command.
func (SetMiterLimitCommand) Type() CommandType { return CmdSetMiterLimit }

// SetDashCommand sets the dash pattern for stroking.
type SetDashCommand struct {
	// Pattern is the dash pattern (alternating dash and gap lengths).
	// Nil or empty means solid line.
	Pattern []float64
	// Offset is the starting offset into the pattern.
	Offset float64
}

// Type implements Command.
func (SetDashCommand) Type() CommandType { return CmdSetDash }

// SetFillRuleCommand sets the fill rule.
type SetFillRuleCommand struct {
	// Rule is the fill rule (non-zero or even-odd).
	Rule FillRule
}

// Type implements Command.
func (SetFillRuleCommand) Type() CommandType { return CmdSetFillRule }

// --------------------------------------------------------------------------
// Supporting Types
// --------------------------------------------------------------------------

// FillRule specifies how to determine which areas are inside a path.
type FillRule uint8

const (
	// FillRuleNonZero uses the non-zero winding rule.
	FillRuleNonZero FillRule = iota
	// FillRuleEvenOdd uses the even-odd rule.
	FillRuleEvenOdd
)

// LineCap specifies the shape of line endpoints.
type LineCap uint8

const (
	// LineCapButt specifies a flat line cap.
	LineCapButt LineCap = iota
	// LineCapRound specifies a rounded line cap.
	LineCapRound
	// LineCapSquare specifies a square line cap.
	LineCapSquare
)

// LineJoin specifies the shape of line joins.
type LineJoin uint8

const (
	// LineJoinMiter specifies a sharp (mitered) join.
	LineJoinMiter LineJoin = iota
	// LineJoinRound specifies a rounded join.
	LineJoinRound
	// LineJoinBevel specifies a beveled join.
	LineJoinBevel
)

// Stroke defines the style for stroking paths.
type Stroke struct {
	// Width is the line width in pixels.
	Width float64
	// Cap is the shape of line endpoints.
	Cap LineCap
	// Join is the shape of line joins.
	Join LineJoin
	// MiterLimit is the limit for miter joins.
	MiterLimit float64
	// DashPattern is the dash pattern (nil for solid line).
	DashPattern []float64
	// DashOffset is the starting offset into the dash pattern.
	DashOffset float64
}

// DefaultStroke returns a Stroke with default settings.
func DefaultStroke() Stroke {
	return Stroke{
		Width:      1.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
}

// Clone creates a deep copy of the Stroke.
func (s Stroke) Clone() Stroke {
	result := s
	if s.DashPattern != nil {
		result.DashPattern = make([]float64, len(s.DashPattern))
		copy(result.DashPattern, s.DashPattern)
	}
	return result
}

// ImageOptions contains options for image rendering.
type ImageOptions struct {
	// Interpolation specifies the interpolation mode.
	Interpolation InterpolationMode
	// Alpha is the opacity (0.0 to 1.0).
	Alpha float64
}

// DefaultImageOptions returns ImageOptions with default settings.
func DefaultImageOptions() ImageOptions {
	return ImageOptions{
		Interpolation: InterpolationBilinear,
		Alpha:         1.0,
	}
}

// InterpolationMode specifies how to interpolate between pixels.
type InterpolationMode uint8

const (
	// InterpolationNearest uses nearest-neighbor interpolation.
	InterpolationNearest InterpolationMode = iota
	// InterpolationBilinear uses bilinear interpolation.
	InterpolationBilinear
)
