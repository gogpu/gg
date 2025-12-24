package gg

import (
	"image"
	"image/color"
	"math"

	"github.com/gogpu/gg/internal/clip"
	"github.com/gogpu/gg/text"
)

// Context is the main drawing context.
// It maintains a pixmap, current path, paint state, and transformation stack.
type Context struct {
	width    int
	height   int
	pixmap   *Pixmap
	renderer Renderer

	// Current state
	path      *Path
	paint     *Paint
	face      text.Face       // Current font face for text drawing
	clipStack *clip.ClipStack // Clipping stack

	// Transform and state stack
	matrix         Matrix
	stack          []Matrix
	clipStackDepth []int // Tracks clip stack depth for each Push/Pop

	// Layer support
	layerStack *layerStack // Layer stack for compositing
	basePixmap *Pixmap     // Base pixmap when layers are active
}

// NewContext creates a new drawing context with the given dimensions.
func NewContext(width, height int) *Context {
	return &Context{
		width:          width,
		height:         height,
		pixmap:         NewPixmap(width, height),
		renderer:       NewSoftwareRenderer(width, height),
		path:           NewPath(),
		paint:          NewPaint(),
		matrix:         Identity(),
		stack:          make([]Matrix, 0, 8),
		clipStackDepth: make([]int, 0, 8),
	}
}

// NewContextForImage creates a context for drawing on an existing image.
func NewContextForImage(img image.Image) *Context {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixmap := FromImage(img)

	return &Context{
		width:          width,
		height:         height,
		pixmap:         pixmap,
		renderer:       NewSoftwareRenderer(width, height),
		path:           NewPath(),
		paint:          NewPaint(),
		matrix:         Identity(),
		stack:          make([]Matrix, 0, 8),
		clipStackDepth: make([]int, 0, 8),
	}
}

// Width returns the width of the context.
func (c *Context) Width() int {
	return c.width
}

// Height returns the height of the context.
func (c *Context) Height() int {
	return c.height
}

// Image returns the context's image.
func (c *Context) Image() image.Image {
	return c.pixmap.ToImage()
}

// SavePNG saves the context to a PNG file.
func (c *Context) SavePNG(path string) error {
	return c.pixmap.SavePNG(path)
}

// Clear fills the entire context with a color.
func (c *Context) Clear() {
	c.pixmap.Clear(Transparent)
}

// ClearWithColor fills the entire context with a specific color.
func (c *Context) ClearWithColor(col RGBA) {
	c.pixmap.Clear(col)
}

// SetColor sets the current drawing color.
func (c *Context) SetColor(col color.Color) {
	c.paint.SetBrush(Solid(FromColor(col)))
}

// SetRGB sets the current color using RGB values (0-1).
func (c *Context) SetRGB(r, g, b float64) {
	c.paint.SetBrush(SolidRGB(r, g, b))
}

// SetRGBA sets the current color using RGBA values (0-1).
func (c *Context) SetRGBA(r, g, b, a float64) {
	c.paint.SetBrush(SolidRGBA(r, g, b, a))
}

// SetHexColor sets the current color using a hex string.
func (c *Context) SetHexColor(hex string) {
	c.paint.SetBrush(SolidHex(hex))
}

// SetFillBrush sets the brush used for fill operations.
// This is the preferred way to set fill styling in new code.
//
// Example:
//
//	ctx.SetFillBrush(gg.Solid(gg.Red))
//	ctx.SetFillBrush(gg.SolidHex("#FF5733"))
//	ctx.SetFillBrush(gg.HorizontalGradient(gg.Red, gg.Blue, 0, 100))
func (c *Context) SetFillBrush(b Brush) {
	c.paint.SetBrush(b)
}

// SetStrokeBrush sets the brush used for stroke operations.
// Note: In the current implementation, fill and stroke share the same brush.
// This method is provided for API symmetry and future extensibility.
//
// Example:
//
//	ctx.SetStrokeBrush(gg.Solid(gg.Black))
//	ctx.SetStrokeBrush(gg.SolidRGB(0.5, 0.5, 0.5))
func (c *Context) SetStrokeBrush(b Brush) {
	c.paint.SetBrush(b)
}

// FillBrush returns the current fill brush.
func (c *Context) FillBrush() Brush {
	return c.paint.GetBrush()
}

// StrokeBrush returns the current stroke brush.
// Note: In the current implementation, fill and stroke share the same brush.
func (c *Context) StrokeBrush() Brush {
	return c.paint.GetBrush()
}

// SetLineWidth sets the line width for stroking.
func (c *Context) SetLineWidth(width float64) {
	c.paint.LineWidth = width
}

// SetLineCap sets the line cap style.
func (c *Context) SetLineCap(lineCap LineCap) {
	c.paint.LineCap = lineCap
}

// SetLineJoin sets the line join style.
func (c *Context) SetLineJoin(join LineJoin) {
	c.paint.LineJoin = join
}

// SetFillRule sets the fill rule.
func (c *Context) SetFillRule(rule FillRule) {
	c.paint.FillRule = rule
}

// MoveTo starts a new subpath at the given point.
func (c *Context) MoveTo(x, y float64) {
	p := c.matrix.TransformPoint(Pt(x, y))
	c.path.MoveTo(p.X, p.Y)
}

// LineTo adds a line to the current path.
func (c *Context) LineTo(x, y float64) {
	p := c.matrix.TransformPoint(Pt(x, y))
	c.path.LineTo(p.X, p.Y)
}

// QuadraticTo adds a quadratic Bezier curve to the current path.
func (c *Context) QuadraticTo(cx, cy, x, y float64) {
	cp := c.matrix.TransformPoint(Pt(cx, cy))
	p := c.matrix.TransformPoint(Pt(x, y))
	c.path.QuadraticTo(cp.X, cp.Y, p.X, p.Y)
}

// CubicTo adds a cubic Bezier curve to the current path.
func (c *Context) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	cp1 := c.matrix.TransformPoint(Pt(c1x, c1y))
	cp2 := c.matrix.TransformPoint(Pt(c2x, c2y))
	p := c.matrix.TransformPoint(Pt(x, y))
	c.path.CubicTo(cp1.X, cp1.Y, cp2.X, cp2.Y, p.X, p.Y)
}

// ClosePath closes the current subpath.
func (c *Context) ClosePath() {
	c.path.Close()
}

// ClearPath clears the current path.
func (c *Context) ClearPath() {
	c.path.Clear()
}

// NewSubPath starts a new subpath without closing the previous one.
func (c *Context) NewSubPath() {
	// In most implementations, just starting with MoveTo creates a new subpath
	// This is a no-op but provided for API compatibility
}

// Fill fills the current path.
func (c *Context) Fill() {
	c.renderer.Fill(c.pixmap, c.path, c.paint)
}

// Stroke strokes the current path.
func (c *Context) Stroke() {
	c.renderer.Stroke(c.pixmap, c.path, c.paint)
}

// FillPreserve fills the current path and preserves it for additional operations.
func (c *Context) FillPreserve() {
	c.renderer.Fill(c.pixmap, c.path, c.paint)
	// Path is preserved
}

// StrokePreserve strokes the current path and preserves it.
func (c *Context) StrokePreserve() {
	c.renderer.Stroke(c.pixmap, c.path, c.paint)
	// Path is preserved
}

// Push saves the current state (transform, paint, and clip).
func (c *Context) Push() {
	c.stack = append(c.stack, c.matrix)

	// Save current clip stack depth
	depth := 0
	if c.clipStack != nil {
		depth = c.clipStack.Depth()
	}
	c.clipStackDepth = append(c.clipStackDepth, depth)
}

// Pop restores the last saved state.
func (c *Context) Pop() {
	if len(c.stack) == 0 {
		return
	}

	// Restore transform matrix
	c.matrix = c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]

	// Restore clip stack depth
	if len(c.clipStackDepth) > 0 {
		targetDepth := c.clipStackDepth[len(c.clipStackDepth)-1]
		c.clipStackDepth = c.clipStackDepth[:len(c.clipStackDepth)-1]

		// Pop clip stack entries until we reach the target depth
		if c.clipStack != nil {
			for c.clipStack.Depth() > targetDepth {
				c.clipStack.Pop()
			}
		}
	}
}

// Identity resets the transformation matrix to identity.
func (c *Context) Identity() {
	c.matrix = Identity()
}

// Translate applies a translation to the transformation matrix.
func (c *Context) Translate(x, y float64) {
	c.matrix = c.matrix.Multiply(Translate(x, y))
}

// Scale applies a scaling transformation.
func (c *Context) Scale(x, y float64) {
	c.matrix = c.matrix.Multiply(Scale(x, y))
}

// Rotate applies a rotation (angle in radians).
func (c *Context) Rotate(angle float64) {
	c.matrix = c.matrix.Multiply(Rotate(angle))
}

// RotateAbout rotates around a specific point.
func (c *Context) RotateAbout(angle, x, y float64) {
	c.Translate(x, y)
	c.Rotate(angle)
	c.Translate(-x, -y)
}

// Shear applies a shear transformation.
func (c *Context) Shear(x, y float64) {
	c.matrix = c.matrix.Multiply(Shear(x, y))
}

// TransformPoint transforms a point by the current matrix.
func (c *Context) TransformPoint(x, y float64) (float64, float64) {
	p := c.matrix.TransformPoint(Pt(x, y))
	return p.X, p.Y
}

// InvertY inverts the Y axis (useful for coordinate system changes).
func (c *Context) InvertY() {
	c.Translate(0, float64(c.height))
	c.Scale(1, -1)
}

// SetPixel sets a single pixel.
func (c *Context) SetPixel(x, y int, col RGBA) {
	c.pixmap.SetPixel(x, y, col)
}

// DrawPoint draws a single point at the given coordinates.
func (c *Context) DrawPoint(x, y, r float64) {
	c.DrawCircle(x, y, r)
}

// DrawLine draws a line between two points.
func (c *Context) DrawLine(x1, y1, x2, y2 float64) {
	c.MoveTo(x1, y1)
	c.LineTo(x2, y2)
}

// DrawRectangle draws a rectangle.
func (c *Context) DrawRectangle(x, y, w, h float64) {
	c.MoveTo(x, y)
	c.LineTo(x+w, y)
	c.LineTo(x+w, y+h)
	c.LineTo(x, y+h)
	c.ClosePath()
}

// DrawRoundedRectangle draws a rectangle with rounded corners.
func (c *Context) DrawRoundedRectangle(x, y, w, h, r float64) {
	c.path.RoundedRectangle(x, y, w, h, r)
}

// DrawCircle draws a circle.
func (c *Context) DrawCircle(x, y, r float64) {
	const k = 0.5522847498307936
	offset := r * k

	c.MoveTo(x+r, y)
	c.CubicTo(x+r, y+offset, x+offset, y+r, x, y+r)
	c.CubicTo(x-offset, y+r, x-r, y+offset, x-r, y)
	c.CubicTo(x-r, y-offset, x-offset, y-r, x, y-r)
	c.CubicTo(x+offset, y-r, x+r, y-offset, x+r, y)
	c.ClosePath()
}

// DrawEllipse draws an ellipse.
func (c *Context) DrawEllipse(x, y, rx, ry float64) {
	const k = 0.5522847498307936
	ox := rx * k
	oy := ry * k

	c.MoveTo(x+rx, y)
	c.CubicTo(x+rx, y+oy, x+ox, y+ry, x, y+ry)
	c.CubicTo(x-ox, y+ry, x-rx, y+oy, x-rx, y)
	c.CubicTo(x-rx, y-oy, x-ox, y-ry, x, y-ry)
	c.CubicTo(x+ox, y-ry, x+rx, y-oy, x+rx, y)
	c.ClosePath()
}

// DrawArc draws a circular arc.
func (c *Context) DrawArc(x, y, r, angle1, angle2 float64) {
	// Transform center point
	center := c.matrix.TransformPoint(Pt(x, y))

	// Create arc in world space
	const twoPi = 2 * math.Pi
	for angle2 < angle1 {
		angle2 += twoPi
	}

	const maxAngle = math.Pi / 2
	numSegments := int(math.Ceil((angle2 - angle1) / maxAngle))
	angleStep := (angle2 - angle1) / float64(numSegments)

	for i := 0; i < numSegments; i++ {
		a1 := angle1 + float64(i)*angleStep
		a2 := a1 + angleStep
		c.arcSegment(center.X, center.Y, r, a1, a2)
	}
}

// arcSegment draws a single arc segment.
func (c *Context) arcSegment(cx, cy, r, a1, a2 float64) {
	alpha := math.Sin(a2-a1) * (math.Sqrt(4+3*math.Tan((a2-a1)/2)*math.Tan((a2-a1)/2)) - 1) / 3

	cos1, sin1 := math.Cos(a1), math.Sin(a1)
	cos2, sin2 := math.Cos(a2), math.Sin(a2)

	x1 := cx + r*cos1
	y1 := cy + r*sin1
	x2 := cx + r*cos2
	y2 := cy + r*sin2

	c1x := x1 - alpha*r*sin1
	c1y := y1 + alpha*r*cos1
	c2x := x2 + alpha*r*sin2
	c2y := y2 - alpha*r*cos2

	if len(c.path.Elements()) == 0 {
		c.path.MoveTo(x1, y1)
	}
	c.path.CubicTo(c1x, c1y, c2x, c2y, x2, y2)
}

// DrawEllipticalArc draws an elliptical arc (advanced).
func (c *Context) DrawEllipticalArc(x, y, rx, ry, angle1, angle2 float64) {
	// This is a simplified version; full implementation would handle rotation
	c.Push()
	c.Translate(x, y)
	c.Scale(rx, ry)
	c.DrawArc(0, 0, 1, angle1, angle2)
	c.Pop()
}

// currentColor returns the current drawing color from the paint.
// If the current pattern is a solid color, returns that color.
// Otherwise returns black as a fallback.
func (c *Context) currentColor() color.Color {
	if p, ok := c.paint.Pattern.(*SolidPattern); ok {
		return p.Color.Color()
	}
	return color.Black
}
