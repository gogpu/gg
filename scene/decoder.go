package scene

import "math"

// Decoder provides sequential decoding of an Encoding's command stream.
// It tracks position indices across all data streams (tags, paths, draws, transforms)
// and provides methods to read each command type's associated data.
//
// The decoder is designed for efficient rendering playback, supporting both
// full traversal and selective decoding for tile-based rendering.
//
// Example usage:
//
//	dec := NewDecoder(encoding)
//	for dec.Next() {
//	    switch dec.Tag() {
//	    case TagMoveTo:
//	        x, y := dec.MoveTo()
//	        // handle move
//	    case TagLineTo:
//	        x, y := dec.LineTo()
//	        // handle line
//	    case TagFill:
//	        brush, style := dec.Fill()
//	        // handle fill
//	    }
//	}
type Decoder struct {
	enc *Encoding

	// Stream position indices
	tagIdx   int
	pathIdx  int
	drawIdx  int
	transIdx int

	// Current tag being processed
	currentTag Tag
}

// NewDecoder creates a new decoder for the given encoding.
// Returns nil if encoding is nil.
func NewDecoder(enc *Encoding) *Decoder {
	if enc == nil {
		return nil
	}
	return &Decoder{
		enc: enc,
	}
}

// Reset resets the decoder to the beginning of the encoding.
// This allows reusing the decoder for multiple passes.
func (d *Decoder) Reset(enc *Encoding) {
	d.enc = enc
	d.tagIdx = 0
	d.pathIdx = 0
	d.drawIdx = 0
	d.transIdx = 0
	d.currentTag = 0
}

// Next advances to the next command in the stream.
// Returns true if there is another command, false when iteration is complete.
// After calling Next, use Tag() to get the current command type,
// then call the appropriate method (MoveTo, LineTo, Fill, etc.) to get the data.
func (d *Decoder) Next() bool {
	if d.enc == nil || d.tagIdx >= len(d.enc.tags) {
		return false
	}

	d.currentTag = d.enc.tags[d.tagIdx]
	d.tagIdx++
	return true
}

// Tag returns the current command tag.
// Call this after Next() returns true to determine which data method to call.
func (d *Decoder) Tag() Tag {
	return d.currentTag
}

// Peek returns the next tag without advancing the decoder.
// Returns 0 if at end of stream.
func (d *Decoder) Peek() Tag {
	if d.tagIdx >= len(d.enc.tags) {
		return 0
	}
	return d.enc.tags[d.tagIdx]
}

// HasMore returns true if there are more commands to decode.
func (d *Decoder) HasMore() bool {
	return d.enc != nil && d.tagIdx < len(d.enc.tags)
}

// Position returns the current position in the tag stream.
func (d *Decoder) Position() int {
	return d.tagIdx
}

// ---------------------------------------------------------------------------
// Path Command Decoders
// ---------------------------------------------------------------------------

// MoveTo reads the current MoveTo command data.
// Returns the destination point (x, y).
// Only valid when Tag() == TagMoveTo.
func (d *Decoder) MoveTo() (x, y float32) {
	if d.pathIdx+2 > len(d.enc.pathData) {
		return 0, 0
	}
	x = d.enc.pathData[d.pathIdx]
	y = d.enc.pathData[d.pathIdx+1]
	d.pathIdx += 2
	return x, y
}

// LineTo reads the current LineTo command data.
// Returns the destination point (x, y).
// Only valid when Tag() == TagLineTo.
func (d *Decoder) LineTo() (x, y float32) {
	if d.pathIdx+2 > len(d.enc.pathData) {
		return 0, 0
	}
	x = d.enc.pathData[d.pathIdx]
	y = d.enc.pathData[d.pathIdx+1]
	d.pathIdx += 2
	return x, y
}

// QuadTo reads the current QuadTo command data.
// Returns the control point (cx, cy) and destination point (x, y).
// Only valid when Tag() == TagQuadTo.
func (d *Decoder) QuadTo() (cx, cy, x, y float32) {
	if d.pathIdx+4 > len(d.enc.pathData) {
		return 0, 0, 0, 0
	}
	cx = d.enc.pathData[d.pathIdx]
	cy = d.enc.pathData[d.pathIdx+1]
	x = d.enc.pathData[d.pathIdx+2]
	y = d.enc.pathData[d.pathIdx+3]
	d.pathIdx += 4
	return cx, cy, x, y
}

// CubicTo reads the current CubicTo command data.
// Returns control point 1 (c1x, c1y), control point 2 (c2x, c2y), and destination (x, y).
// Only valid when Tag() == TagCubicTo.
//
//nolint:gocritic // tooManyResultsChecker: 6 results are natural for cubic curves (2 control points + endpoint)
func (d *Decoder) CubicTo() (c1x, c1y, c2x, c2y, x, y float32) {
	if d.pathIdx+6 > len(d.enc.pathData) {
		return 0, 0, 0, 0, 0, 0
	}
	c1x = d.enc.pathData[d.pathIdx]
	c1y = d.enc.pathData[d.pathIdx+1]
	c2x = d.enc.pathData[d.pathIdx+2]
	c2y = d.enc.pathData[d.pathIdx+3]
	x = d.enc.pathData[d.pathIdx+4]
	y = d.enc.pathData[d.pathIdx+5]
	d.pathIdx += 6
	return c1x, c1y, c2x, c2y, x, y
}

// ---------------------------------------------------------------------------
// Transform Command Decoder
// ---------------------------------------------------------------------------

// Transform reads the current Transform command data.
// Returns the affine transformation matrix.
// Only valid when Tag() == TagTransform.
func (d *Decoder) Transform() Affine {
	if d.transIdx >= len(d.enc.transforms) {
		return IdentityAffine()
	}
	t := d.enc.transforms[d.transIdx]
	d.transIdx++
	return t
}

// ---------------------------------------------------------------------------
// Draw Command Decoders
// ---------------------------------------------------------------------------

// Fill reads the current Fill command data.
// Returns the brush and fill style.
// Only valid when Tag() == TagFill.
func (d *Decoder) Fill() (brush Brush, style FillStyle) {
	if d.drawIdx+2 > len(d.enc.drawData) {
		return Brush{}, FillNonZero
	}

	brushIdx := d.enc.drawData[d.drawIdx]
	styleVal := d.enc.drawData[d.drawIdx+1]
	d.drawIdx += 2

	if int(brushIdx) < len(d.enc.brushes) {
		brush = d.enc.brushes[brushIdx]
	}
	style = FillStyle(styleVal)
	return brush, style
}

// Stroke reads the current Stroke command data.
// Returns the brush and stroke style.
// Only valid when Tag() == TagStroke.
func (d *Decoder) Stroke() (brush Brush, style *StrokeStyle) {
	if d.drawIdx+5 > len(d.enc.drawData) {
		return Brush{}, DefaultStrokeStyle()
	}

	brushIdx := d.enc.drawData[d.drawIdx]
	widthBits := d.enc.drawData[d.drawIdx+1]
	miterBits := d.enc.drawData[d.drawIdx+2]
	capVal := d.enc.drawData[d.drawIdx+3]
	joinVal := d.enc.drawData[d.drawIdx+4]
	d.drawIdx += 5

	if int(brushIdx) < len(d.enc.brushes) {
		brush = d.enc.brushes[brushIdx]
	}

	style = &StrokeStyle{
		Width:      math.Float32frombits(widthBits),
		MiterLimit: math.Float32frombits(miterBits),
		Cap:        LineCap(capVal),
		Join:       LineJoin(joinVal),
	}
	return brush, style
}

// ---------------------------------------------------------------------------
// Layer Command Decoders
// ---------------------------------------------------------------------------

// PushLayer reads the current PushLayer command data.
// Returns the blend mode and alpha value.
// Only valid when Tag() == TagPushLayer.
func (d *Decoder) PushLayer() (blend BlendMode, alpha float32) {
	if d.drawIdx+2 > len(d.enc.drawData) {
		return BlendNormal, 1.0
	}

	blendVal := d.enc.drawData[d.drawIdx]
	alphaBits := d.enc.drawData[d.drawIdx+1]
	d.drawIdx += 2

	blend = BlendMode(blendVal)
	alpha = math.Float32frombits(alphaBits)
	return blend, alpha
}

// ---------------------------------------------------------------------------
// Image Command Decoder
// ---------------------------------------------------------------------------

// Image reads the current Image command data.
// Returns the image index and transform.
// Only valid when Tag() == TagImage.
func (d *Decoder) Image() (imageIndex uint32, transform Affine) {
	if d.drawIdx+1 > len(d.enc.drawData) {
		return 0, IdentityAffine()
	}

	imageIndex = d.enc.drawData[d.drawIdx]
	d.drawIdx++

	if d.transIdx < len(d.enc.transforms) {
		transform = d.enc.transforms[d.transIdx]
		d.transIdx++
	} else {
		transform = IdentityAffine()
	}
	return imageIndex, transform
}

// ---------------------------------------------------------------------------
// Brush Command Decoder
// ---------------------------------------------------------------------------

// Brush reads the current Brush command data.
// Returns the RGBA color values.
// Only valid when Tag() == TagBrush.
func (d *Decoder) Brush() (r, g, b, a float32) {
	if d.pathIdx+4 > len(d.enc.pathData) {
		return 0, 0, 0, 1
	}
	r = d.enc.pathData[d.pathIdx]
	g = d.enc.pathData[d.pathIdx+1]
	b = d.enc.pathData[d.pathIdx+2]
	a = d.enc.pathData[d.pathIdx+3]
	d.pathIdx += 4
	return r, g, b, a
}

// ---------------------------------------------------------------------------
// Utility Methods
// ---------------------------------------------------------------------------

// SkipPath advances past all path commands until EndPath is found.
// This is useful for quickly skipping paths that are outside the clip region.
func (d *Decoder) SkipPath() {
	for d.Next() {
		switch d.currentTag {
		case TagMoveTo:
			d.pathIdx += 2
		case TagLineTo:
			d.pathIdx += 2
		case TagQuadTo:
			d.pathIdx += 4
		case TagCubicTo:
			d.pathIdx += 6
		case TagEndPath:
			return
		}
	}
}

// CollectPath collects all path commands until EndPath into a new Path.
// Returns the collected path. The decoder is advanced past the path data.
// Returns nil if not currently at the start of a path.
func (d *Decoder) CollectPath() *Path {
	path := NewPath()

	for d.Next() {
		switch d.currentTag {
		case TagMoveTo:
			x, y := d.MoveTo()
			path.MoveTo(x, y)
		case TagLineTo:
			x, y := d.LineTo()
			path.LineTo(x, y)
		case TagQuadTo:
			cx, cy, x, y := d.QuadTo()
			path.QuadTo(cx, cy, x, y)
		case TagCubicTo:
			c1x, c1y, c2x, c2y, x, y := d.CubicTo()
			path.CubicTo(c1x, c1y, c2x, c2y, x, y)
		case TagClosePath:
			path.Close()
		case TagEndPath:
			return path
		}
	}

	return path
}

// Encoding returns the encoding being decoded.
func (d *Decoder) Encoding() *Encoding {
	return d.enc
}
