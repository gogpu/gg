package scene

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// BlendMode represents a compositing blend mode.
type BlendMode uint32

// Blend mode constants following Porter-Duff and advanced blend modes.
const (
	BlendNormal BlendMode = iota
	BlendMultiply
	BlendScreen
	BlendOverlay
	BlendDarken
	BlendLighten
	BlendColorDodge
	BlendColorBurn
	BlendHardLight
	BlendSoftLight
	BlendDifference
	BlendExclusion
	BlendHue
	BlendSaturation
	BlendColor
	BlendLuminosity
	// Porter-Duff modes
	BlendClear
	BlendCopy
	BlendDestination
	BlendSourceOver
	BlendDestinationOver
	BlendSourceIn
	BlendDestinationIn
	BlendSourceOut
	BlendDestinationOut
	BlendSourceAtop
	BlendDestinationAtop
	BlendXor
	BlendPlus
)

// String returns a human-readable name for the blend mode.
func (mode BlendMode) String() string {
	switch mode {
	// Standard blend modes
	case BlendNormal:
		return "Normal"
	case BlendMultiply:
		return "Multiply"
	case BlendScreen:
		return "Screen"
	case BlendOverlay:
		return "Overlay"
	case BlendDarken:
		return "Darken"
	case BlendLighten:
		return "Lighten"
	case BlendColorDodge:
		return "ColorDodge"
	case BlendColorBurn:
		return "ColorBurn"
	case BlendHardLight:
		return "HardLight"
	case BlendSoftLight:
		return "SoftLight"
	case BlendDifference:
		return "Difference"
	case BlendExclusion:
		return "Exclusion"
	// HSL blend modes
	case BlendHue:
		return "Hue"
	case BlendSaturation:
		return "Saturation"
	case BlendColor:
		return "Color"
	case BlendLuminosity:
		return "Luminosity"
	// Porter-Duff modes
	case BlendClear:
		return "Clear"
	case BlendCopy:
		return "Copy"
	case BlendDestination:
		return "Destination"
	case BlendSourceOver:
		return "SourceOver"
	case BlendDestinationOver:
		return "DestinationOver"
	case BlendSourceIn:
		return "SourceIn"
	case BlendDestinationIn:
		return "DestinationIn"
	case BlendSourceOut:
		return "SourceOut"
	case BlendDestinationOut:
		return "DestinationOut"
	case BlendSourceAtop:
		return "SourceAtop"
	case BlendDestinationAtop:
		return "DestinationAtop"
	case BlendXor:
		return "Xor"
	case BlendPlus:
		return "Plus"
	default:
		return unknownStr
	}
}

// IsPorterDuff returns true if this is a Porter-Duff compositing mode.
func (mode BlendMode) IsPorterDuff() bool {
	return mode >= BlendClear && mode <= BlendPlus
}

// IsAdvanced returns true if this is an advanced separable blend mode.
func (mode BlendMode) IsAdvanced() bool {
	return (mode >= BlendMultiply && mode <= BlendExclusion) ||
		mode == BlendNormal
}

// IsHSL returns true if this is an HSL-based non-separable blend mode.
func (mode BlendMode) IsHSL() bool {
	return mode >= BlendHue && mode <= BlendLuminosity
}

// FillStyle represents the fill rule for paths.
type FillStyle uint32

const (
	// FillNonZero uses the non-zero winding rule.
	FillNonZero FillStyle = 0
	// FillEvenOdd uses the even-odd rule.
	FillEvenOdd FillStyle = 1
)

// StrokeStyle contains stroke parameters.
type StrokeStyle struct {
	Width      float32
	MiterLimit float32
	Cap        LineCap
	Join       LineJoin
}

// LineCap represents line endpoint shapes.
type LineCap uint32

const (
	LineCapButt LineCap = iota
	LineCapRound
	LineCapSquare
)

// LineJoin represents line join shapes.
type LineJoin uint32

const (
	LineJoinMiter LineJoin = iota
	LineJoinRound
	LineJoinBevel
)

// DefaultStrokeStyle returns default stroke parameters.
func DefaultStrokeStyle() *StrokeStyle {
	return &StrokeStyle{
		Width:      1.0,
		MiterLimit: 10.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
	}
}

// BrushKind identifies the type of brush.
type BrushKind uint32

const (
	BrushSolid BrushKind = iota
	BrushLinearGradient
	BrushRadialGradient
	BrushImage
)

// Brush represents a paint source for fill/stroke operations.
type Brush struct {
	Kind  BrushKind
	Color gg.RGBA // For solid brushes
	// Additional fields for gradients/images would go here
}

// SolidBrush creates a solid color brush.
func SolidBrush(c gg.RGBA) Brush {
	return Brush{
		Kind:  BrushSolid,
		Color: c,
	}
}

// Rect represents a bounding rectangle.
type Rect struct {
	MinX, MinY float32
	MaxX, MaxY float32
}

// EmptyRect returns an empty rectangle (inverted bounds for union operations).
func EmptyRect() Rect {
	return Rect{
		MinX: math.MaxFloat32,
		MinY: math.MaxFloat32,
		MaxX: -math.MaxFloat32,
		MaxY: -math.MaxFloat32,
	}
}

// IsEmpty returns true if the rectangle has no area.
func (r Rect) IsEmpty() bool {
	return r.MinX >= r.MaxX || r.MinY >= r.MaxY
}

// ImageRect converts to image.Rectangle (floor min, ceil max for pixel coverage).
func (r Rect) ImageRect() image.Rectangle {
	if r.IsEmpty() {
		return image.Rectangle{}
	}
	return image.Rect(
		int(math.Floor(float64(r.MinX))),
		int(math.Floor(float64(r.MinY))),
		int(math.Ceil(float64(r.MaxX))),
		int(math.Ceil(float64(r.MaxY))),
	)
}

// Union returns the smallest rectangle containing both r and other.
func (r Rect) Union(other Rect) Rect {
	return Rect{
		MinX: min32(r.MinX, other.MinX),
		MinY: min32(r.MinY, other.MinY),
		MaxX: max32(r.MaxX, other.MaxX),
		MaxY: max32(r.MaxY, other.MaxY),
	}
}

// UnionPoint expands the rectangle to include the point.
func (r Rect) UnionPoint(x, y float32) Rect {
	return Rect{
		MinX: min32(r.MinX, x),
		MinY: min32(r.MinY, y),
		MaxX: max32(r.MaxX, x),
		MaxY: max32(r.MaxY, y),
	}
}

// Width returns the width of the rectangle.
func (r Rect) Width() float32 {
	if r.IsEmpty() {
		return 0
	}
	return r.MaxX - r.MinX
}

// Height returns the height of the rectangle.
func (r Rect) Height() float32 {
	if r.IsEmpty() {
		return 0
	}
	return r.MaxY - r.MinY
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// Affine represents a 2D affine transformation matrix.
// The matrix is stored in row-major order as:
//
//	| A  B  C |
//	| D  E  F |
//
// Where a point (x, y) is transformed to:
//
//	x' = A*x + B*y + C
//	y' = D*x + E*y + F
type Affine struct {
	A, B, C float32
	D, E, F float32
}

// IdentityAffine returns the identity transformation.
func IdentityAffine() Affine {
	return Affine{A: 1, B: 0, C: 0, D: 0, E: 1, F: 0}
}

// NewAffine creates an affine transformation from individual matrix components.
//
//	| a  b  c |
//	| d  e  f |
func NewAffine(a, b, c, d, e, f float32) Affine {
	return Affine{A: a, B: b, C: c, D: d, E: e, F: f}
}

// TranslateAffine creates a translation transformation.
func TranslateAffine(x, y float32) Affine {
	return Affine{A: 1, B: 0, C: x, D: 0, E: 1, F: y}
}

// ScaleAffine creates a scaling transformation.
func ScaleAffine(x, y float32) Affine {
	return Affine{A: x, B: 0, C: 0, D: 0, E: y, F: 0}
}

// RotateAffine creates a rotation transformation (angle in radians).
func RotateAffine(angle float32) Affine {
	cos := float32(math.Cos(float64(angle)))
	sin := float32(math.Sin(float64(angle)))
	return Affine{A: cos, B: -sin, C: 0, D: sin, E: cos, F: 0}
}

// Multiply returns the product of two affine transformations.
func (a Affine) Multiply(b Affine) Affine {
	return Affine{
		A: a.A*b.A + a.B*b.D,
		B: a.A*b.B + a.B*b.E,
		C: a.A*b.C + a.B*b.F + a.C,
		D: a.D*b.A + a.E*b.D,
		E: a.D*b.B + a.E*b.E,
		F: a.D*b.C + a.E*b.F + a.F,
	}
}

// TransformPoint transforms a point by the affine matrix.
func (a Affine) TransformPoint(x, y float32) (float32, float32) {
	return a.A*x + a.B*y + a.C, a.D*x + a.E*y + a.F
}

// IsIdentity returns true if this is the identity transformation.
func (a Affine) IsIdentity() bool {
	return a.A == 1 && a.B == 0 && a.C == 0 &&
		a.D == 0 && a.E == 1 && a.F == 0
}

// AffineFromMatrix converts a gg.Matrix to an Affine.
func AffineFromMatrix(m gg.Matrix) Affine {
	return Affine{
		A: float32(m.A),
		B: float32(m.B),
		C: float32(m.C),
		D: float32(m.D),
		E: float32(m.E),
		F: float32(m.F),
	}
}

// TextFlags controls text rendering behavior.
type TextFlags uint16

const (
	TextFlagHinting TextFlags = 1 << iota
	TextFlagCJK               // ADR-027: text contains CJK characters
)

// GlyphRunData is the header for a TagText scene encoding element.
// Followed by GlyphCount × GlyphEntry, then TextLen bytes of UTF-8 text.
type GlyphRunData struct {
	FontSourceID uint64
	FontSize     float32
	GlyphCount   uint16
	Flags        TextFlags
	OriginX      float32
	OriginY      float32
	BrushIndex   uint32
	TextLen      uint16
}

// glyphRunDataSize is the byte size of a serialized GlyphRunData header.
const glyphRunDataSize = 8 + 4 + 2 + 2 + 4 + 4 + 4 + 2 // = 30 bytes

// GlyphEntry is a single positioned glyph in a text run (10 bytes).
type GlyphEntry struct {
	GlyphID text.GlyphID // uint16
	X       float32
	Y       float32
}

// glyphEntrySize is the byte size of a serialized GlyphEntry.
const glyphEntrySize = 2 + 4 + 4 // = 10 bytes

// Encoding holds the dual-stream encoded representation of drawing commands.
// It uses separate streams for tags (1 byte each), path data, draw data,
// and transforms to maximize cache efficiency and GPU compatibility.
type Encoding struct {
	// tags is the command stream (1 byte per command)
	tags []Tag

	// pathData holds coordinate data for path commands (float32)
	// MoveTo: 2, LineTo: 2, QuadTo: 4, CubicTo: 6
	pathData []float32

	// drawData holds draw command parameters (uint32)
	// Fill: brush index, fill style
	// Stroke: brush index, then style params
	drawData []uint32

	// transforms holds affine transformation matrices
	transforms []Affine

	// textData holds serialized GlyphRunData + GlyphEntry arrays for TagText commands.
	// Separate stream from pathData — text data has different layout and lifetime.
	textData []byte

	// brushes holds brush definitions referenced by draw commands
	brushes []Brush

	// commandBounds tracks per-draw-command bounding boxes (ADR-021).
	// Parallel to shapeCount: commandBounds[i] = bounds of i-th shape command.
	// Used by DamageTracker for frame-to-frame object diff.
	commandBounds []Rect

	// bounds tracks cumulative bounding box
	bounds Rect

	// pathBounds tracks current path's bounding box
	pathBounds Rect

	// statistics for debugging/profiling
	pathCount  int
	shapeCount int
}

// NewEncoding creates a new empty encoding.
func NewEncoding() *Encoding {
	return &Encoding{
		tags:          make([]Tag, 0, 64),
		pathData:      make([]float32, 0, 256),
		drawData:      make([]uint32, 0, 32),
		transforms:    make([]Affine, 0, 8),
		textData:      make([]byte, 0, 256),
		brushes:       make([]Brush, 0, 16),
		commandBounds: make([]Rect, 0, 32),
		bounds:        EmptyRect(),
		pathBounds:    EmptyRect(),
	}
}

// Reset clears the encoding for reuse without deallocating memory.
// This is the key method for zero-allocation pooling.
func (e *Encoding) Reset() {
	e.tags = e.tags[:0]
	e.pathData = e.pathData[:0]
	e.drawData = e.drawData[:0]
	e.transforms = e.transforms[:0]
	e.textData = e.textData[:0]
	e.brushes = e.brushes[:0]
	e.commandBounds = e.commandBounds[:0]
	e.bounds = EmptyRect()
	e.pathBounds = EmptyRect()
	e.pathCount = 0
	e.shapeCount = 0
}

// EncodeAntiAlias adds an anti-aliasing state change command.
// The value is stored as 1 uint32 in drawData (0 = disabled, 1 = enabled).
func (e *Encoding) EncodeAntiAlias(enabled bool) {
	e.tags = append(e.tags, TagSetAntiAlias)
	var val uint32
	if enabled {
		val = 1
	}
	e.drawData = append(e.drawData, val)
}

// EncodeTransform adds a transform command.
func (e *Encoding) EncodeTransform(t Affine) {
	e.tags = append(e.tags, TagTransform)
	e.transforms = append(e.transforms, t)
}

// EncodeTransformFromMatrix adds a transform from a gg.Matrix.
func (e *Encoding) EncodeTransformFromMatrix(m gg.Matrix) {
	e.EncodeTransform(AffineFromMatrix(m))
}

// EncodePath encodes a complete path from a gg.Path.
func (e *Encoding) EncodePath(p *gg.Path) {
	if p == nil {
		return
	}

	if p.NumVerbs() == 0 {
		return
	}

	e.tags = append(e.tags, TagBeginPath)
	e.pathBounds = EmptyRect()
	e.pathCount++

	p.Iterate(func(verb gg.PathVerb, coords []float64) {
		switch verb {
		case gg.MoveTo:
			e.encodeMoveTo(float32(coords[0]), float32(coords[1]))
		case gg.LineTo:
			e.encodeLineTo(float32(coords[0]), float32(coords[1]))
		case gg.QuadTo:
			e.encodeQuadTo(
				float32(coords[0]), float32(coords[1]),
				float32(coords[2]), float32(coords[3]),
			)
		case gg.CubicTo:
			e.encodeCubicTo(
				float32(coords[0]), float32(coords[1]),
				float32(coords[2]), float32(coords[3]),
				float32(coords[4]), float32(coords[5]),
			)
		case gg.Close:
			e.tags = append(e.tags, TagClosePath)
		}
	})

	e.tags = append(e.tags, TagEndPath)
	e.bounds = e.bounds.Union(e.pathBounds)
}

// encodeMoveTo adds a MoveTo command.
func (e *Encoding) encodeMoveTo(x, y float32) {
	e.tags = append(e.tags, TagMoveTo)
	e.pathData = append(e.pathData, x, y)
	e.pathBounds = e.pathBounds.UnionPoint(x, y)
}

// encodeLineTo adds a LineTo command.
func (e *Encoding) encodeLineTo(x, y float32) {
	e.tags = append(e.tags, TagLineTo)
	e.pathData = append(e.pathData, x, y)
	e.pathBounds = e.pathBounds.UnionPoint(x, y)
}

// encodeQuadTo adds a QuadTo command.
func (e *Encoding) encodeQuadTo(cx, cy, x, y float32) {
	e.tags = append(e.tags, TagQuadTo)
	e.pathData = append(e.pathData, cx, cy, x, y)
	e.pathBounds = e.pathBounds.UnionPoint(cx, cy)
	e.pathBounds = e.pathBounds.UnionPoint(x, y)
}

// encodeCubicTo adds a CubicTo command.
func (e *Encoding) encodeCubicTo(c1x, c1y, c2x, c2y, x, y float32) {
	e.tags = append(e.tags, TagCubicTo)
	e.pathData = append(e.pathData, c1x, c1y, c2x, c2y, x, y)
	e.pathBounds = e.pathBounds.UnionPoint(c1x, c1y)
	e.pathBounds = e.pathBounds.UnionPoint(c2x, c2y)
	e.pathBounds = e.pathBounds.UnionPoint(x, y)
}

// EncodeFill adds a fill command with the given brush and fill style.
func (e *Encoding) EncodeFill(brush Brush, style FillStyle) {
	brushIdx := len(e.brushes)
	e.brushes = append(e.brushes, brush)

	e.tags = append(e.tags, TagFill)
	//nolint:gosec // brush index is bounded by slice length, overflow not possible in practice
	e.drawData = append(e.drawData, uint32(brushIdx), uint32(style))
	e.shapeCount++
}

// EncodeFillRoundRect adds a rounded rectangle fill command using SDF rendering.
// This bypasses path encoding entirely, storing the rectangle geometry directly
// in the data streams for dedicated SDF per-pixel rendering in the tile renderer.
func (e *Encoding) EncodeFillRoundRect(brush Brush, style FillStyle, rect Rect, rx, ry float32) {
	brushIdx := len(e.brushes)
	e.brushes = append(e.brushes, brush)

	e.tags = append(e.tags, TagFillRoundRect)
	//nolint:gosec // brush index is bounded by slice length, overflow not possible in practice
	e.drawData = append(e.drawData, uint32(brushIdx), uint32(style))
	e.pathData = append(e.pathData, rect.MinX, rect.MinY, rect.MaxX, rect.MaxY, rx, ry)

	e.bounds = e.bounds.Union(rect)
	e.shapeCount++
}

// EncodeStroke adds a stroke command with the given brush and stroke style.
func (e *Encoding) EncodeStroke(brush Brush, style *StrokeStyle) {
	if style == nil {
		style = DefaultStrokeStyle()
	}

	brushIdx := len(e.brushes)
	e.brushes = append(e.brushes, brush)

	e.tags = append(e.tags, TagStroke)
	//nolint:gosec // brush index is bounded by slice length, overflow not possible in practice
	e.drawData = append(e.drawData,
		uint32(brushIdx),
		math.Float32bits(style.Width),
		math.Float32bits(style.MiterLimit),
		uint32(style.Cap),
		uint32(style.Join),
	)
	e.shapeCount++
}

// EncodePushLayer pushes a new compositing layer.
func (e *Encoding) EncodePushLayer(blend BlendMode, alpha float32) {
	e.tags = append(e.tags, TagPushLayer)
	e.drawData = append(e.drawData, uint32(blend))
	e.drawData = append(e.drawData, math.Float32bits(alpha))
}

// EncodePopLayer pops the current compositing layer.
func (e *Encoding) EncodePopLayer() {
	e.tags = append(e.tags, TagPopLayer)
}

// EncodeBeginClip begins a clipping region.
func (e *Encoding) EncodeBeginClip() {
	e.tags = append(e.tags, TagBeginClip)
}

// EncodeEndClip ends the current clipping region.
func (e *Encoding) EncodeEndClip() {
	e.tags = append(e.tags, TagEndClip)
}

// EncodeBrush encodes a brush definition.
func (e *Encoding) EncodeBrush(brush Brush) int {
	idx := len(e.brushes)
	e.brushes = append(e.brushes, brush)

	e.tags = append(e.tags, TagBrush)
	// For solid brush, encode RGBA
	e.pathData = append(e.pathData,
		float32(brush.Color.R),
		float32(brush.Color.G),
		float32(brush.Color.B),
		float32(brush.Color.A),
	)

	return idx
}

// EncodeImage encodes an image reference.
func (e *Encoding) EncodeImage(imageIndex uint32, transform Affine) {
	e.tags = append(e.tags, TagImage)
	e.drawData = append(e.drawData, imageIndex)
	e.transforms = append(e.transforms, transform)
}

// EncodeText encodes a pre-shaped text run as a TagText command.
// The glyph run header, glyph entries, and original text are serialized into the textData stream.
func (e *Encoding) EncodeText(run GlyphRunData, glyphs []GlyphEntry, str string) {
	e.tags = append(e.tags, TagText)

	totalSize := glyphRunDataSize + len(glyphs)*glyphEntrySize + len(str)

	// Grow textData in one allocation, then write directly into the tail.
	base := len(e.textData)
	e.textData = append(e.textData, make([]byte, totalSize)...)
	buf := e.textData[base:]
	off := 0

	// Header
	binary.LittleEndian.PutUint64(buf[off:], run.FontSourceID)
	off += 8
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(run.FontSize))
	off += 4
	binary.LittleEndian.PutUint16(buf[off:], run.GlyphCount)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:], uint16(run.Flags))
	off += 2
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(run.OriginX))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(run.OriginY))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], run.BrushIndex)
	off += 4
	binary.LittleEndian.PutUint16(buf[off:], run.TextLen)
	off += 2

	// Glyph entries
	for _, g := range glyphs {
		binary.LittleEndian.PutUint16(buf[off:], uint16(g.GlyphID))
		off += 2
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(g.X))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(g.Y))
		off += 4
	}

	// Text bytes
	copy(buf[off:], str)

	e.shapeCount++
}

// TextData returns the text data stream.
func (e *Encoding) TextData() []byte {
	return e.textData
}

// Bounds returns the cumulative bounding box of all encoded content.
func (e *Encoding) Bounds() Rect {
	return e.bounds
}

// UpdateBounds expands the encoding's bounding box to include the given rect.
// This is used to propagate transformed bounds from Scene to Encoding,
// ensuring that the tile-based renderer's early-out intersection test
// uses correct post-transform coordinates.
func (e *Encoding) UpdateBounds(bounds Rect) {
	e.bounds = e.bounds.Union(bounds)
}

// RecordCommandBounds stores the bounding box for the current draw command.
// Called by Scene after each Fill/Stroke/DrawImage with the transformed shape bounds.
// Used by DamageTracker (ADR-021) for frame-to-frame object diff.
func (e *Encoding) RecordCommandBounds(bounds Rect) {
	e.commandBounds = append(e.commandBounds, bounds)
}

// CommandBounds returns per-draw-command bounding boxes as TaggedBounds.
// The command index serves as a stable ID — valid when scene is built
// in the same order each frame (standard immediate-mode pattern).
func (e *Encoding) CommandBounds() []TaggedBounds {
	result := make([]TaggedBounds, len(e.commandBounds))
	for i, b := range e.commandBounds {
		result[i] = TaggedBounds{
			ID:   uint64(i),
			Rect: b.ImageRect(),
		}
	}
	return result
}

// Hash computes a 64-bit FNV-1a hash of the encoding for cache keys.
// The hash includes all stream data to ensure uniqueness.
func (e *Encoding) Hash() uint64 {
	const (
		fnvOffset = 14695981039346656037
		fnvPrime  = 1099511628211
	)

	hash := uint64(fnvOffset)

	// Hash tags
	for _, t := range e.tags {
		hash ^= uint64(t)
		hash *= fnvPrime
	}

	// Hash path data
	for _, v := range e.pathData {
		bits := math.Float32bits(v)
		hash ^= uint64(bits)
		hash *= fnvPrime
	}

	// Hash draw data
	for _, v := range e.drawData {
		hash ^= uint64(v)
		hash *= fnvPrime
	}

	// Hash text data
	for _, b := range e.textData {
		hash ^= uint64(b)
		hash *= fnvPrime
	}

	// Hash transforms
	for _, t := range e.transforms {
		hash ^= uint64(math.Float32bits(t.A))
		hash *= fnvPrime
		hash ^= uint64(math.Float32bits(t.B))
		hash *= fnvPrime
		hash ^= uint64(math.Float32bits(t.C))
		hash *= fnvPrime
		hash ^= uint64(math.Float32bits(t.D))
		hash *= fnvPrime
		hash ^= uint64(math.Float32bits(t.E))
		hash *= fnvPrime
		hash ^= uint64(math.Float32bits(t.F))
		hash *= fnvPrime
	}

	return hash
}

// Append merges another encoding into this one.
// The other encoding's content is appended after the current content.
// Append merges another Encoding into this one, adjusting brush indices.
// For scene-level merging with image registry offset, use AppendWithImages.
func (e *Encoding) Append(other *Encoding) {
	e.AppendWithImages(other, 0)
}

// AppendWithImages merges another Encoding into this one, adjusting both
// brush indices and image indices by their respective offsets.
// imageOffset is the number of images already registered in the target scene;
// it shifts TagImage drawData entries so they reference the correct images
// after merging two image registries.
func (e *Encoding) AppendWithImages(other *Encoding, imageOffset uint32) {
	if other == nil || len(other.tags) == 0 {
		return
	}

	// Calculate brush index offset for the appended encoding
	//nolint:gosec // brush slice length is bounded, overflow not possible in practice
	brushOffset := uint32(len(e.brushes))

	// Append tags directly
	e.tags = append(e.tags, other.tags...)

	// Append path data directly
	e.pathData = append(e.pathData, other.pathData...)

	// Append draw data with adjusted brush indices
	// We need to adjust brush references in fill/stroke commands
	drawDataStart := len(e.drawData)
	e.drawData = append(e.drawData, other.drawData...)

	// Adjust brush indices in the appended draw data.
	// Also adjust brush indices embedded in textData for TagText commands.
	drawIdx := 0
	textDataStart := len(e.textData)
	e.textData = append(e.textData, other.textData...)
	textOff := 0
	for _, tag := range other.tags {
		switch tag {
		case TagFill:
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 2
		case TagFillRoundRect:
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 2
		case TagStroke:
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 5
		case TagPushLayer:
			drawIdx += 2
		case TagSetAntiAlias:
			drawIdx++ // 1 uint32: 0 or 1
		case TagImage:
			if imageOffset > 0 && drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += imageOffset
			}
			drawIdx++
		case TagText:
			if textOff+glyphRunDataSize <= len(other.textData) {
				glyphCount := int(binary.LittleEndian.Uint16(other.textData[textOff+12:]))
				textLen := int(binary.LittleEndian.Uint16(other.textData[textOff+28:]))
				// Adjust brush index (at offset 24 in GlyphRunData)
				brushIdxOff := textDataStart + textOff + 24
				oldBrush := binary.LittleEndian.Uint32(e.textData[brushIdxOff:])
				binary.LittleEndian.PutUint32(e.textData[brushIdxOff:], oldBrush+brushOffset)
				textOff += glyphRunDataSize + glyphCount*glyphEntrySize + textLen
			}
		}
	}

	// Append transforms
	e.transforms = append(e.transforms, other.transforms...)

	// Append brushes
	e.brushes = append(e.brushes, other.brushes...)

	// Union bounds
	e.bounds = e.bounds.Union(other.bounds)

	// Update statistics
	e.pathCount += other.pathCount
	e.shapeCount += other.shapeCount
}

// AppendWithTranslation merges another encoding with a translation offset
// applied to all path coordinates.
//
// Architecture: pathData coordinate offset approach.
//
// Our SceneCanvas pre-bakes absolute coordinates via applyTransform() and
// records Identity transforms in the encoding. This differs from Vello where
// paths stay in local coordinates and the transform stream carries the full
// transformation (Vello composes transforms at append: parent * child).
//
// Because our pathData already contains absolute coordinates with Identity
// transforms, the correct offset strategy is:
//
//   - pathData: offset all coordinate float32 values by (dx, dy)
//   - transforms: copy VERBATIM (they are Identity; composing translation
//     would cause double-offset since the renderer applies transforms to
//     already-offset pathData coordinates)
//
// Alternative approaches considered:
//
//   - Vello pattern (transform composition only): multiply each child
//     transform by TranslateAffine(dx, dy). Does NOT work with our
//     pre-baked coordinate architecture — coordinates would stay at (0,0)
//     since transforms are Identity.
//
//   - Skia/Flutter pattern (replay-time canvas transform): wrap replay in
//     Push/Translate/Pop on the target canvas. Works with render.Canvas
//     (used by current desktop compositor) but NOT with SceneCanvas
//     (Scene.Append has no canvas context).
//
//   - Migrate to Vello architecture: stop pre-baking coordinates, record
//     transforms in encoding, compose at append. Correct long-term but
//     requires rewriting SceneCanvas coordinate handling.
//
// Tag exhaustiveness: every Tag that consumes pathData floats MUST have a
// case in the switch below. Adding a new tag with pathData without updating
// this switch will cause silent coordinate corruption. The default case
// handles tags with zero pathData/drawData (markers, clip, pop).
func (e *Encoding) AppendWithTranslation(other *Encoding, dx, dy float32, imageOffset uint32) {
	if other == nil || len(other.tags) == 0 {
		return
	}
	if dx == 0 && dy == 0 {
		e.AppendWithImages(other, imageOffset)
		return
	}

	//nolint:gosec // brush slice length is bounded
	brushOffset := uint32(len(e.brushes))

	e.tags = append(e.tags, other.tags...)

	pathStart := len(e.pathData)
	e.pathData = append(e.pathData, other.pathData...)

	drawDataStart := len(e.drawData)
	e.drawData = append(e.drawData, other.drawData...)

	pathIdx := 0
	drawIdx := 0
	for _, tag := range other.tags {
		switch tag {
		// --- Coordinate tags: offset pathData by (dx, dy) ---

		case TagMoveTo, TagLineTo:
			e.pathData[pathStart+pathIdx] += dx
			e.pathData[pathStart+pathIdx+1] += dy
			pathIdx += 2

		case TagQuadTo:
			for i := 0; i < 4; i += 2 {
				e.pathData[pathStart+pathIdx+i] += dx
				e.pathData[pathStart+pathIdx+i+1] += dy
			}
			pathIdx += 4

		case TagCubicTo:
			for i := 0; i < 6; i += 2 {
				e.pathData[pathStart+pathIdx+i] += dx
				e.pathData[pathStart+pathIdx+i+1] += dy
			}
			pathIdx += 6

		case TagFillRoundRect:
			// 6 floats: minX, minY, maxX, maxY (offset), radiusX, radiusY (no offset).
			e.pathData[pathStart+pathIdx] += dx
			e.pathData[pathStart+pathIdx+1] += dy
			e.pathData[pathStart+pathIdx+2] += dx
			e.pathData[pathStart+pathIdx+3] += dy
			pathIdx += 6
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 2

		// --- Non-coordinate pathData: skip without offset ---

		case TagBrush:
			pathIdx += 4 // 4 float32: R, G, B, A — not coordinates

		// --- Draw data only (no pathData) ---

		case TagFill:
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 2

		case TagStroke:
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 5 // brush + width + miterLimit + cap + join

		case TagPushLayer:
			drawIdx += 2 // blend mode + alpha

		case TagSetAntiAlias:
			drawIdx++ // 1 uint32: 0 or 1, no coordinate data

		case TagImage:
			if imageOffset > 0 && drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += imageOffset
			}
			drawIdx++ // image index only; transform is in transforms stream

		case TagText:
			// Text data is in the textData stream. Handled in a second pass below.

		// --- Marker/structural tags: zero pathData, zero drawData ---

		case TagTransform:
			// Transform data is in the separate transforms stream, not pathData.
			// Handled below (copied verbatim).

		case TagBeginPath, TagEndPath, TagClosePath,
			TagPopLayer, TagBeginClip, TagEndClip:
			// Pure markers — no data in any stream.

		default:
			panic(fmt.Sprintf("scene.AppendWithTranslation: unhandled tag 0x%02X (%s) — update switch to handle pathData/drawData layout for this tag", byte(tag), tag))
		}
	}

	e.appendTextDataWithTranslation(other, brushOffset, dx, dy)

	// Transforms copied verbatim — see architecture note above.
	e.transforms = append(e.transforms, other.transforms...)

	e.brushes = append(e.brushes, other.brushes...)

	ob := other.bounds
	ob.MinX += dx
	ob.MinY += dy
	ob.MaxX += dx
	ob.MaxY += dy
	e.bounds = e.bounds.Union(ob)

	e.pathCount += other.pathCount
	e.shapeCount += other.shapeCount
}

// appendTextDataWithTranslation appends other's textData, offsetting origins by (dx, dy)
// and adjusting brush indices by brushOffset.
func (e *Encoding) appendTextDataWithTranslation(other *Encoding, brushOffset uint32, dx, dy float32) {
	if len(other.textData) == 0 {
		return
	}
	textDataStart := len(e.textData)
	e.textData = append(e.textData, other.textData...)
	textOff := 0
	for _, tag := range other.tags {
		if tag != TagText {
			continue
		}
		if textOff+glyphRunDataSize > len(other.textData) {
			break
		}
		absOff := textDataStart + textOff
		oxBits := binary.LittleEndian.Uint32(e.textData[absOff+16:])
		oyBits := binary.LittleEndian.Uint32(e.textData[absOff+20:])
		binary.LittleEndian.PutUint32(e.textData[absOff+16:], math.Float32bits(math.Float32frombits(oxBits)+dx))
		binary.LittleEndian.PutUint32(e.textData[absOff+20:], math.Float32bits(math.Float32frombits(oyBits)+dy))
		oldBrush := binary.LittleEndian.Uint32(e.textData[absOff+24:])
		binary.LittleEndian.PutUint32(e.textData[absOff+24:], oldBrush+brushOffset)
		glyphCount := int(binary.LittleEndian.Uint16(other.textData[textOff+12:]))
		textLen := int(binary.LittleEndian.Uint16(other.textData[textOff+28:]))
		textOff += glyphRunDataSize + glyphCount*glyphEntrySize + textLen
	}
}

// Clone creates a deep copy of the encoding.
func (e *Encoding) Clone() *Encoding {
	clone := NewEncoding()

	clone.tags = make([]Tag, len(e.tags))
	copy(clone.tags, e.tags)

	clone.pathData = make([]float32, len(e.pathData))
	copy(clone.pathData, e.pathData)

	clone.drawData = make([]uint32, len(e.drawData))
	copy(clone.drawData, e.drawData)

	clone.transforms = make([]Affine, len(e.transforms))
	copy(clone.transforms, e.transforms)

	clone.textData = make([]byte, len(e.textData))
	copy(clone.textData, e.textData)

	clone.brushes = make([]Brush, len(e.brushes))
	copy(clone.brushes, e.brushes)

	clone.bounds = e.bounds
	clone.pathBounds = e.pathBounds
	clone.pathCount = e.pathCount
	clone.shapeCount = e.shapeCount

	return clone
}

// Tags returns the tag stream (read-only access for iteration).
func (e *Encoding) Tags() []Tag {
	return e.tags
}

// PathData returns the path data stream.
func (e *Encoding) PathData() []float32 {
	return e.pathData
}

// DrawData returns the draw data stream.
func (e *Encoding) DrawData() []uint32 {
	return e.drawData
}

// Transforms returns the transform stream.
func (e *Encoding) Transforms() []Affine {
	return e.transforms
}

// Brushes returns the brush definitions.
func (e *Encoding) Brushes() []Brush {
	return e.brushes
}

// PathCount returns the number of paths encoded.
func (e *Encoding) PathCount() int {
	return e.pathCount
}

// ShapeCount returns the number of shapes (fills + strokes) encoded.
func (e *Encoding) ShapeCount() int {
	return e.shapeCount
}

// IsEmpty returns true if the encoding contains no commands.
func (e *Encoding) IsEmpty() bool {
	return len(e.tags) == 0
}

// Size returns the approximate memory size in bytes.
func (e *Encoding) Size() int {
	return len(e.tags) +
		len(e.pathData)*4 +
		len(e.drawData)*4 +
		len(e.textData) +
		len(e.transforms)*24 + // 6 float32 per Affine
		len(e.brushes)*20 // approximate brush size
}

// Capacity returns the total allocated capacity in bytes.
func (e *Encoding) Capacity() int {
	return cap(e.tags) +
		cap(e.pathData)*4 +
		cap(e.drawData)*4 +
		cap(e.textData) +
		cap(e.transforms)*24 +
		cap(e.brushes)*20
}

// Iterator provides sequential access to encoded commands.
type Iterator struct {
	enc      *Encoding
	tagIdx   int
	pathIdx  int
	drawIdx  int
	transIdx int
	brushIdx int
}

// NewIterator creates an iterator for the encoding.
func (e *Encoding) NewIterator() *Iterator {
	return &Iterator{enc: e}
}

// Next advances to the next command and returns its tag.
// Returns false when iteration is complete.
func (it *Iterator) Next() (Tag, bool) {
	if it.tagIdx >= len(it.enc.tags) {
		return 0, false
	}

	tag := it.enc.tags[it.tagIdx]
	it.tagIdx++
	return tag, true
}

// ReadPathData reads n float32 values from the path data stream.
func (it *Iterator) ReadPathData(n int) []float32 {
	if it.pathIdx+n > len(it.enc.pathData) {
		return nil
	}
	data := it.enc.pathData[it.pathIdx : it.pathIdx+n]
	it.pathIdx += n
	return data
}

// ReadDrawData reads n uint32 values from the draw data stream.
func (it *Iterator) ReadDrawData(n int) []uint32 {
	if it.drawIdx+n > len(it.enc.drawData) {
		return nil
	}
	data := it.enc.drawData[it.drawIdx : it.drawIdx+n]
	it.drawIdx += n
	return data
}

// ReadTransform reads the next transform from the stream.
func (it *Iterator) ReadTransform() (Affine, bool) {
	if it.transIdx >= len(it.enc.transforms) {
		return Affine{}, false
	}
	t := it.enc.transforms[it.transIdx]
	it.transIdx++
	return t, true
}

// GetBrush returns the brush at the given index.
func (it *Iterator) GetBrush(idx uint32) (Brush, bool) {
	if int(idx) >= len(it.enc.brushes) {
		return Brush{}, false
	}
	return it.enc.brushes[idx], true
}

// Reset resets the iterator to the beginning.
func (it *Iterator) Reset() {
	it.tagIdx = 0
	it.pathIdx = 0
	it.drawIdx = 0
	it.transIdx = 0
	it.brushIdx = 0
}
