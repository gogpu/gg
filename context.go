package gg

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"

	"github.com/gogpu/gg/internal/clip"
	"github.com/gogpu/gg/text"
)

// Context is the main drawing context.
// It maintains a pixmap, current path, paint state, and transformation stack.
// Context implements io.Closer for proper resource cleanup.
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

	// Mask support
	mask      *Mask   // Current alpha mask
	maskStack []*Mask // Mask stack for Push/Pop

	// Pipeline mode
	pipelineMode PipelineMode // GPU pipeline selection mode

	// Rasterizer mode
	rasterizerMode RasterizerMode // CPU rasterizer selection mode

	// Text rendering
	outlineExtractor *text.OutlineExtractor // lazy: for transform-aware text (Strategy B)

	// Lifecycle
	closed bool // Indicates whether Close has been called
}

// Ensure Context implements io.Closer
var _ io.Closer = (*Context)(nil)

// NewContext creates a new drawing context with the given dimensions.
// Optional ContextOption arguments can be used for dependency injection:
//
//	// Default software rendering (uses analytic anti-aliasing)
//	dc := gg.NewContext(800, 600)
//
//	// Custom GPU renderer (dependency injection)
//	dc := gg.NewContext(800, 600, gg.WithRenderer(gpuRenderer))
func NewContext(width, height int, opts ...ContextOption) *Context {
	// Apply options
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	// Use provided pixmap or create new one
	pixmap := options.pixmap
	if pixmap == nil {
		pixmap = NewPixmap(width, height)
	}

	// Use provided renderer or create software renderer
	renderer := options.renderer
	if renderer == nil {
		renderer = NewSoftwareRenderer(width, height)
	}

	return &Context{
		width:          width,
		height:         height,
		pixmap:         pixmap,
		renderer:       renderer,
		path:           NewPath(),
		paint:          NewPaint(),
		matrix:         Identity(),
		stack:          make([]Matrix, 0, 8),
		clipStackDepth: make([]int, 0, 8),
		pipelineMode:   options.pipelineMode,
	}
}

// NewContextForImage creates a context for drawing on an existing image.
// Optional ContextOption arguments can be used for dependency injection.
func NewContextForImage(img image.Image, opts ...ContextOption) *Context {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixmap := FromImage(img)

	// Apply options
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	// Use provided renderer or create software renderer
	renderer := options.renderer
	if renderer == nil {
		renderer = NewSoftwareRenderer(width, height)
	}

	return &Context{
		width:          width,
		height:         height,
		pixmap:         pixmap,
		renderer:       renderer,
		path:           NewPath(),
		paint:          NewPaint(),
		matrix:         Identity(),
		stack:          make([]Matrix, 0, 8),
		clipStackDepth: make([]int, 0, 8),
		pipelineMode:   options.pipelineMode,
	}
}

// Close releases resources associated with the Context.
// After Close, the Context should not be used.
// Close is idempotent - multiple calls are safe.
// Implements io.Closer.
//
// Close flushes any pending GPU accelerator operations to ensure all
// queued draw commands are rendered before releasing context state.
// Note: Close does NOT shut down the global GPU accelerator itself,
// since it may be shared by other contexts. To release GPU resources
// at application shutdown, call [CloseAccelerator].
func (c *Context) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true

	// Flush pending GPU operations so queued shapes are not lost.
	c.flushGPUAccelerator()

	// Clear path to release memory
	c.ClearPath()

	// Clear state stack
	c.stack = nil
	c.clipStackDepth = nil
	c.maskStack = nil
	c.mask = nil

	return nil
}

// SetPipelineMode sets the GPU rendering pipeline mode.
// See PipelineMode for available modes.
//
// If the registered accelerator implements PipelineModeAware, the mode is
// propagated so the accelerator can route operations to the correct pipeline
// (render pass vs compute).
func (c *Context) SetPipelineMode(mode PipelineMode) {
	c.pipelineMode = mode
	if a := Accelerator(); a != nil {
		if pma, ok := a.(PipelineModeAware); ok {
			pma.SetPipelineMode(mode)
		}
	}
}

// PipelineMode returns the current pipeline mode.
func (c *Context) PipelineMode() PipelineMode {
	return c.pipelineMode
}

// SetRasterizerMode sets the rasterization strategy for this context.
// RasterizerAuto (default) uses intelligent auto-selection based on path
// complexity, bounding box area, and shape type.
// Other modes force a specific algorithm, bypassing auto-selection.
//
// The mode is per-Context — different contexts can use different strategies.
func (c *Context) SetRasterizerMode(mode RasterizerMode) {
	c.rasterizerMode = mode
}

// RasterizerMode returns the current rasterizer mode.
func (c *Context) RasterizerMode() RasterizerMode {
	return c.rasterizerMode
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
	_ = c.FlushGPU() // Flush pending GPU shapes before reading pixels.
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

// SetMiterLimit sets the miter limit for line joins.
func (c *Context) SetMiterLimit(limit float64) {
	c.paint.MiterLimit = limit
}

// SetStroke sets the complete stroke style.
// This is the preferred way to configure stroke properties.
//
// Example:
//
//	ctx.SetStroke(gg.DefaultStroke().WithWidth(2).WithCap(gg.LineCapRound))
//	ctx.SetStroke(gg.DashedStroke(5, 3))
func (c *Context) SetStroke(stroke Stroke) {
	c.paint.SetStroke(stroke)
}

// GetStroke returns the current stroke style.
func (c *Context) GetStroke() Stroke {
	return c.paint.GetStroke()
}

// SetDash sets the dash pattern for stroking.
// Pass alternating dash and gap lengths.
// Passing no arguments clears the dash pattern (returns to solid lines).
//
// Example:
//
//	ctx.SetDash(5, 3)       // 5 units dash, 3 units gap
//	ctx.SetDash(10, 5, 2, 5) // complex pattern
//	ctx.SetDash()           // clear dash (solid line)
func (c *Context) SetDash(lengths ...float64) {
	if len(lengths) == 0 {
		c.ClearDash()
		return
	}

	dash := NewDash(lengths...)
	if dash == nil {
		c.ClearDash()
		return
	}

	// Ensure we have a Stroke to set the dash on
	if c.paint.Stroke == nil {
		stroke := c.paint.GetStroke()
		c.paint.Stroke = &stroke
	}
	c.paint.Stroke.Dash = dash
}

// SetDashOffset sets the starting offset into the dash pattern.
// This has no effect if no dash pattern is set.
func (c *Context) SetDashOffset(offset float64) {
	if c.paint.Stroke == nil {
		// Create stroke from legacy fields if needed
		stroke := c.paint.GetStroke()
		c.paint.Stroke = &stroke
	}
	if c.paint.Stroke.Dash != nil {
		c.paint.Stroke.Dash = c.paint.Stroke.Dash.WithOffset(offset)
	}
}

// ClearDash removes the dash pattern, returning to solid lines.
func (c *Context) ClearDash() {
	if c.paint.Stroke != nil {
		c.paint.Stroke.Dash = nil
	}
}

// IsDashed returns true if the current stroke uses a dash pattern.
func (c *Context) IsDashed() bool {
	return c.paint.IsDashed()
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

// Fill fills the current path and clears it.
// If a GPU accelerator is registered and supports the path, it is used first.
// Otherwise, the software renderer handles the operation.
// The RasterizerMode set via SetRasterizerMode controls algorithm selection.
// Returns an error if the rendering operation fails.
func (c *Context) Fill() error {
	err := c.doFill()
	c.path.Clear()
	return err
}

// Stroke strokes the current path and clears it.
// If a GPU accelerator is registered and supports the path, it is used first.
// Otherwise, the software renderer handles the operation.
// The RasterizerMode set via SetRasterizerMode controls algorithm selection.
// Returns an error if the rendering operation fails.
func (c *Context) Stroke() error {
	err := c.doStroke()
	c.path.Clear()
	return err
}

// FillPreserve fills the current path without clearing it.
// If a GPU accelerator is registered and supports the path, it is used first.
// Otherwise, the software renderer handles the operation.
// Returns an error if the rendering operation fails.
func (c *Context) FillPreserve() error {
	return c.doFill()
}

// StrokePreserve strokes the current path without clearing it.
// If a GPU accelerator is registered and supports the path, it is used first.
// Otherwise, the software renderer handles the operation.
// Returns an error if the rendering operation fails.
func (c *Context) StrokePreserve() error {
	return c.doStroke()
}

// Push saves the current state (transform, paint, clip, and mask).
func (c *Context) Push() {
	c.stack = append(c.stack, c.matrix)

	// Save current clip stack depth
	depth := 0
	if c.clipStack != nil {
		depth = c.clipStack.Depth()
	}
	c.clipStackDepth = append(c.clipStackDepth, depth)

	// Save current mask (clone if exists)
	var maskCopy *Mask
	if c.mask != nil {
		maskCopy = c.mask.Clone()
	}
	c.maskStack = append(c.maskStack, maskCopy)
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

	// Restore mask
	if len(c.maskStack) > 0 {
		c.mask = c.maskStack[len(c.maskStack)-1]
		c.maskStack = c.maskStack[:len(c.maskStack)-1]
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

// Transform multiplies the current transformation matrix by the given matrix.
// This is similar to CanvasRenderingContext2D.transform() in web browsers.
// The transformation is applied in the order: current * m.
func (c *Context) Transform(m Matrix) {
	c.matrix = c.matrix.Multiply(m)
}

// SetTransform replaces the current transformation matrix with the given matrix.
// This is similar to CanvasRenderingContext2D.setTransform() in web browsers.
// Unlike Transform, this completely replaces the matrix rather than multiplying.
func (c *Context) SetTransform(m Matrix) {
	c.matrix = m
}

// GetTransform returns a copy of the current transformation matrix.
// This is similar to CanvasRenderingContext2D.getTransform() in web browsers.
// The returned matrix is a copy, so modifying it will not affect the context.
func (c *Context) GetTransform() Matrix {
	return c.matrix
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

// GetCurrentPoint returns the current point of the path.
// Returns (0, 0, false) if there is no current point.
func (c *Context) GetCurrentPoint() (x, y float64, ok bool) {
	if c.path == nil || !c.path.HasCurrentPoint() {
		return 0, 0, false
	}
	pt := c.path.CurrentPoint()
	return pt.X, pt.Y, true
}

// EncodePNG writes the image as PNG to the given writer.
// This is useful for streaming, network output, or custom storage.
func (c *Context) EncodePNG(w io.Writer) error {
	return png.Encode(w, c.Image())
}

// EncodeJPEG writes the image as JPEG with the given quality (1-100).
func (c *Context) EncodeJPEG(w io.Writer, quality int) error {
	return jpeg.Encode(w, c.Image(), &jpeg.Options{Quality: quality})
}

// Resize changes the context dimensions, reusing internal buffers where possible.
// If the dimensions haven't changed, this is a no-op.
// Returns an error if width or height is <= 0.
//
// After Resize:
//   - The pixmap is reallocated only if dimensions changed
//   - The clip region is reset to the full rectangle
//   - The transformation matrix is preserved (Push/Pop stack is preserved)
//   - The current path is cleared
//
// This method is useful for UI frameworks that need to resize the canvas
// when the window size changes, without creating a new Context.
func (c *Context) Resize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid dimensions: width=%d, height=%d (both must be > 0)", width, height)
	}

	// No-op if dimensions haven't changed
	if c.width == width && c.height == height {
		return nil
	}

	// Update dimensions
	c.width = width
	c.height = height

	// Reallocate pixmap
	c.pixmap = NewPixmap(width, height)

	// Resize renderer if it supports resizing
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.Resize(width, height)
	}

	// Reset clip stack to full rectangle
	c.clipStack = nil

	// Clear any existing path
	c.ClearPath()

	return nil
}

// ResizeTarget returns the underlying pixmap for resize operations.
// This is primarily used by renderers and advanced users who need
// direct access to the target buffer during resize operations.
func (c *Context) ResizeTarget() *Pixmap {
	return c.pixmap
}

// FlushGPU flushes any pending GPU accelerator operations to the pixel buffer.
// Call this before reading pixel data (e.g., SavePNG, Image) when using a
// batch-capable GPU accelerator. For immediate-mode accelerators this is a no-op.
func (c *Context) FlushGPU() error {
	a := Accelerator()
	if a == nil {
		return nil
	}
	return a.Flush(c.gpuRenderTarget())
}

// gpuRenderTarget returns the current context's pixel buffer as a GPU render target.
func (c *Context) gpuRenderTarget() GPURenderTarget {
	return GPURenderTarget{
		Data:   c.pixmap.Data(),
		Width:  c.pixmap.Width(),
		Height: c.pixmap.Height(),
		Stride: c.pixmap.Width() * 4,
	}
}

// flushGPUAccelerator flushes pending GPU shapes before a CPU fallback operation.
func (c *Context) flushGPUAccelerator() {
	a := Accelerator()
	if a == nil {
		return
	}
	_ = a.Flush(c.gpuRenderTarget())
}

// tryGPUFill attempts to fill the current path using the GPU accelerator.
func (c *Context) tryGPUFill() error {
	a := Accelerator()
	if a == nil {
		return ErrFallbackToCPU
	}
	return c.tryGPUOp(a, a.FillShape, a.FillPath, AccelFill)
}

// tryGPUStroke attempts to stroke the current path using the GPU accelerator.
func (c *Context) tryGPUStroke() error {
	a := Accelerator()
	if a == nil {
		return ErrFallbackToCPU
	}
	return c.tryGPUOp(a, a.StrokeShape, a.StrokePath, AccelStroke)
}

// tryGPUOp attempts GPU rendering using shape-specific SDF first, then general path.
//
// When PipelineModeCompute is active and the accelerator supports compute,
// all operations are routed directly to the path function (which accumulates
// for the compute pipeline). Shape detection is skipped because the compute
// pipeline handles all shapes uniformly.
//
// When PipelineModeRenderPass is active (or Auto selects RenderPass), the
// existing tier-based approach is used: shape SDF first, then general path.
func (c *Context) tryGPUOp(
	a GPUAccelerator,
	shapeFn func(GPURenderTarget, DetectedShape, *Paint) error,
	pathFn func(GPURenderTarget, *Path, *Paint) error,
	pathAccel AcceleratedOp,
) error {
	target := c.gpuRenderTarget()

	// When explicitly in Compute mode, skip shape detection and route
	// all operations directly to the path function. The accelerator's
	// FillPath/StrokePath accumulates into the compute scene.
	if c.pipelineMode == PipelineModeCompute {
		if cpa, ok := a.(ComputePipelineAware); ok && cpa.CanCompute() {
			if a.CanAccelerate(pathAccel) {
				return pathFn(target, c.path, c.paint)
			}
		}
		// Compute requested but not available — fall through to render pass.
	}

	// Try shape-specific SDF first for higher quality output.
	shape := DetectShape(c.path)
	if accel := sdfAccelForShape(shape.Kind); accel != 0 && a.CanAccelerate(accel) {
		if err := shapeFn(target, shape, c.paint); err == nil {
			return nil
		}
	}

	// Try general GPU path operation.
	if a.CanAccelerate(pathAccel) {
		return pathFn(target, c.path, c.paint)
	}

	return ErrFallbackToCPU
}

// sdfAccelForShape maps a shape kind to its SDF acceleration capability.
func sdfAccelForShape(kind ShapeKind) AcceleratedOp {
	switch kind {
	case ShapeCircle, ShapeEllipse:
		return AccelCircleSDF
	case ShapeRect, ShapeRRect:
		return AccelRRectSDF
	default:
		return 0
	}
}

// doFill performs the fill operation respecting the current RasterizerMode.
func (c *Context) doFill() error {
	mode := c.rasterizerMode
	cpuMode := mode

	// RasterizerSDF: try SDF without minimum size check.
	if mode == RasterizerSDF {
		c.setForceSDF(true)
		err := c.tryGPUFill()
		c.setForceSDF(false)
		if err == nil {
			return nil
		}
		// Non-SDF shape → auto CPU fallback.
		cpuMode = RasterizerAuto
	}

	// RasterizerAuto: try GPU normally (SDF with size check).
	if mode == RasterizerAuto {
		if err := c.tryGPUFill(); err == nil {
			return nil
		}
	}

	// CPU path: flush pending GPU, apply mode to software renderer.
	c.flushGPUAccelerator()
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.rasterizerMode = cpuMode
		defer func() { sr.rasterizerMode = RasterizerAuto }()
	}
	return c.renderer.Fill(c.pixmap, c.path, c.paint)
}

// doStroke performs the stroke operation respecting the current RasterizerMode.
func (c *Context) doStroke() error {
	c.paint.TransformScale = c.matrix.ScaleFactor()
	mode := c.rasterizerMode
	cpuMode := mode

	// RasterizerSDF: try SDF without minimum size check.
	if mode == RasterizerSDF {
		c.setForceSDF(true)
		err := c.tryGPUStroke()
		c.setForceSDF(false)
		if err == nil {
			return nil
		}
		cpuMode = RasterizerAuto
	}

	// RasterizerAuto: try GPU normally.
	if mode == RasterizerAuto {
		if err := c.tryGPUStroke(); err == nil {
			return nil
		}
	}

	c.flushGPUAccelerator()
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.rasterizerMode = cpuMode
		defer func() { sr.rasterizerMode = RasterizerAuto }()
	}
	return c.renderer.Stroke(c.pixmap, c.path, c.paint)
}

// setForceSDF enables/disables forced SDF on the registered accelerator.
func (c *Context) setForceSDF(force bool) {
	a := Accelerator()
	if a == nil {
		return
	}
	if f, ok := a.(ForceSDFAware); ok {
		f.SetForceSDF(force)
	}
}
