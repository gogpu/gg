package scene

import (
	"math"

	"github.com/gogpu/gg"
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

	// brushes holds brush definitions referenced by draw commands
	brushes []Brush

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
		tags:       make([]Tag, 0, 64),
		pathData:   make([]float32, 0, 256),
		drawData:   make([]uint32, 0, 32),
		transforms: make([]Affine, 0, 8),
		brushes:    make([]Brush, 0, 16),
		bounds:     EmptyRect(),
		pathBounds: EmptyRect(),
	}
}

// Reset clears the encoding for reuse without deallocating memory.
// This is the key method for zero-allocation pooling.
func (e *Encoding) Reset() {
	e.tags = e.tags[:0]
	e.pathData = e.pathData[:0]
	e.drawData = e.drawData[:0]
	e.transforms = e.transforms[:0]
	e.brushes = e.brushes[:0]
	e.bounds = EmptyRect()
	e.pathBounds = EmptyRect()
	e.pathCount = 0
	e.shapeCount = 0
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

	elements := p.Elements()
	if len(elements) == 0 {
		return
	}

	e.tags = append(e.tags, TagBeginPath)
	e.pathBounds = EmptyRect()
	e.pathCount++

	for _, elem := range elements {
		switch el := elem.(type) {
		case gg.MoveTo:
			e.encodeMoveTo(float32(el.Point.X), float32(el.Point.Y))
		case gg.LineTo:
			e.encodeLineTo(float32(el.Point.X), float32(el.Point.Y))
		case gg.QuadTo:
			e.encodeQuadTo(
				float32(el.Control.X), float32(el.Control.Y),
				float32(el.Point.X), float32(el.Point.Y),
			)
		case gg.CubicTo:
			e.encodeCubicTo(
				float32(el.Control1.X), float32(el.Control1.Y),
				float32(el.Control2.X), float32(el.Control2.Y),
				float32(el.Point.X), float32(el.Point.Y),
			)
		case gg.Close:
			e.tags = append(e.tags, TagClosePath)
		}
	}

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

// Bounds returns the cumulative bounding box of all encoded content.
func (e *Encoding) Bounds() Rect {
	return e.bounds
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
func (e *Encoding) Append(other *Encoding) {
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

	// Adjust brush indices in the appended draw data
	// This requires walking through the other's tags to find Fill/Stroke commands
	drawIdx := 0
	for _, tag := range other.tags {
		switch tag {
		case TagFill:
			// Fill has brush index at drawIdx, fill style at drawIdx+1
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 2
		case TagStroke:
			// Stroke has brush index at drawIdx, then style params
			if drawIdx < len(other.drawData) {
				e.drawData[drawDataStart+drawIdx] += brushOffset
			}
			drawIdx += 5 // brush + width + miterLimit + cap + join
		case TagPushLayer:
			drawIdx += 2 // blend mode + alpha
		case TagImage:
			drawIdx++ // image index (not adjusted, handled separately)
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
		len(e.transforms)*24 + // 6 float32 per Affine
		len(e.brushes)*20 // approximate brush size
}

// Capacity returns the total allocated capacity in bytes.
func (e *Encoding) Capacity() int {
	return cap(e.tags) +
		cap(e.pathData)*4 +
		cap(e.drawData)*4 +
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
