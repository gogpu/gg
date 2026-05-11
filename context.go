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
	"github.com/gogpu/gpucontext"
)

// Context is the main drawing context.
// It maintains a pixmap, current path, paint state, and transformation stack.
// Context implements io.Closer for proper resource cleanup.
//
// When deviceScale > 1.0 (HiDPI/Retina), the Context maintains a larger physical
// pixmap while exposing logical dimensions to user code. Drawing operations use
// logical coordinates; the Context applies a base scale transform transparently.
type Context struct {
	width    int // logical width (user-facing)
	height   int // logical height (user-facing)
	pixmap   *Pixmap
	renderer Renderer

	// HiDPI support
	deviceScale float64 // physical pixels per logical pixel (default 1.0)

	// Current state
	path        *Path
	paint       *Paint
	face        text.Face       // Current font face for text drawing
	clipStack   *clip.ClipStack // Clipping stack
	gpuClipPath *Path           // device-space clip path for GPU depth clipping (GPU-CLIP-003a)

	// Transform and state stack
	matrix         Matrix // user transform (starts as Identity, user-space only)
	deviceMatrix   Matrix // device scale transform (Identity when scale=1.0, NEVER modified by user)
	stack          []Matrix
	clipStackDepth []int // Tracks clip stack depth for each Push/Pop

	// Layer support
	layerStack *layerStack // Layer stack for compositing
	basePixmap *Pixmap     // Base pixmap when layers are active

	// Mask support
	mask      *Mask   // Current alpha mask
	maskStack []*Mask // Mask stack for Push/Pop

	// Per-frame damage tracking (ADR-021 Level 1).
	// List of per-operation bounding boxes — NOT a single union rect.
	// Each Fill/Stroke adds its own rect. Passed as-is to PresentWithDamage
	// for per-rect OS blit. Merged to bounding box if count exceeds threshold.
	frameDamageRects      []image.Rectangle
	damageTrackingEnabled bool

	// Pipeline mode
	pipelineMode PipelineMode // GPU pipeline selection mode

	// Rasterizer mode
	rasterizerMode RasterizerMode // CPU rasterizer selection mode

	// Text rendering
	textMode         TextMode               // text strategy selection (default: Auto)
	outlineExtractor *text.OutlineExtractor // lazy: for transform-aware text (Strategy B)
	glyphCache       *text.GlyphCache       // lazy: cached glyph outlines for drawStringAsOutlines

	// Per-context GPU render context (isolated pending commands, clips, frame tracking).
	// Lazily created when GPURenderContextProvider is available.
	// Typed as gpuContextOps (defined in this package) to avoid circular import
	// with internal/gpu while maintaining type safety.
	gpuCtx            gpuContextOps
	gpuFallbackWarned bool // true after first global fallback warning (avoid log spam)

	// Lifecycle
	closed bool // Indicates whether Close has been called
}

// Ensure Context implements io.Closer
var _ io.Closer = (*Context)(nil)

// NewContext creates a new drawing context with the given logical dimensions.
// Optional ContextOption arguments can be used for dependency injection:
//
//	// Default software rendering (uses analytic anti-aliasing)
//	dc := gg.NewContext(800, 600)
//
//	// Custom GPU renderer (dependency injection)
//	dc := gg.NewContext(800, 600, gg.WithRenderer(gpuRenderer))
//
//	// HiDPI/Retina rendering (logical 800x600, physical 1600x1200)
//	dc := gg.NewContext(800, 600, gg.WithDeviceScale(2.0))
//
// When WithDeviceScale is used, the internal pixmap is allocated at physical
// resolution (width*scale x height*scale) while Width/Height return the
// logical dimensions. All drawing operations use logical coordinates.
// NewContextForPixmap creates a Context backed by an existing Pixmap.
// The Context renders directly into the provided pixmap without allocating
// a new one. Used by scene.Renderer for GPU-accelerated scene rendering.
func NewContextForPixmap(pm *Pixmap) *Context {
	if pm == nil {
		return nil
	}
	return NewContext(pm.Width(), pm.Height(), func(o *contextOptions) {
		o.pixmap = pm
	})
}

func NewContext(width, height int, opts ...ContextOption) *Context {
	// Apply options
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	scale := options.deviceScale
	if scale <= 0 {
		scale = 1.0
	}

	// Physical dimensions for the pixmap
	pw := int(float64(width) * scale)
	ph := int(float64(height) * scale)

	// Use provided pixmap or create one at physical resolution
	pixmap := options.pixmap
	if pixmap == nil {
		pixmap = NewPixmap(pw, ph)
	}

	// Use provided renderer or create software renderer at physical resolution
	renderer := options.renderer
	if renderer == nil {
		sr := NewSoftwareRenderer(pw, ph)
		if scale > 1.0 {
			sr.SetDeviceScale(float32(scale))
		}
		renderer = sr
	}

	// Device matrix: maps user coordinates to physical pixels.
	// User matrix starts as Identity — user transforms never include device scale.
	deviceMatrix := Identity()
	if scale != 1.0 {
		deviceMatrix = Scale(scale, scale)
	}

	if scale != 1.0 {
		Logger().Info("NewContext HiDPI",
			"logical_w", width, "logical_h", height,
			"scale", scale,
			"physical_w", pw, "physical_h", ph,
		)
	}

	return &Context{
		width:                 width,
		height:                height,
		deviceScale:           scale,
		pixmap:                pixmap,
		renderer:              renderer,
		path:                  NewPath(),
		paint:                 NewPaint(),
		matrix:                Identity(),
		deviceMatrix:          deviceMatrix,
		stack:                 make([]Matrix, 0, 8),
		clipStackDepth:        make([]int, 0, 8),
		pipelineMode:          options.pipelineMode,
		damageTrackingEnabled: true,
	}
}

// NewContextForImage creates a context for drawing on an existing image.
// Optional ContextOption arguments can be used for dependency injection.
// The image dimensions are treated as physical pixel dimensions (deviceScale=1.0).
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
		deviceScale:    1.0,
		pixmap:         pixmap,
		renderer:       renderer,
		path:           NewPath(),
		paint:          NewPaint(),
		matrix:         Identity(),
		deviceMatrix:   Identity(),
		stack:          make([]Matrix, 0, 8),
		clipStackDepth: make([]int, 0, 8),
		pipelineMode:   options.pipelineMode,
	}
}

// NewContextWithScale creates a new drawing context with the given logical
// dimensions and device scale factor. This is a convenience wrapper for:
//
//	gg.NewContext(w, h, gg.WithDeviceScale(scale))
//
// The internal pixmap is allocated at physical resolution (w*scale x h*scale).
// All drawing operations use logical coordinates (w x h).
//
// Example (macOS Retina 2x):
//
//	dc := gg.NewContextWithScale(800, 600, 2.0)
//	dc.Width()      // 800 (logical)
//	dc.PixelWidth() // 1600 (physical)
//	dc.DrawCircle(400, 300, 100) // logical coordinates
func NewContextWithScale(width, height int, scale float64) *Context {
	return NewContext(width, height, WithDeviceScale(scale))
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

	// Close per-context GPU render context if it was created.
	if c.gpuCtx != nil {
		type gpuCtxCloser interface {
			Close()
		}
		if closer, ok := c.gpuCtx.(gpuCtxCloser); ok {
			closer.Close()
		}
		c.gpuCtx = nil
	}

	// Clear path to release memory
	c.ClearPath()

	// Clear state stack
	c.stack = nil
	c.clipStackDepth = nil
	c.maskStack = nil
	c.mask = nil
	c.gpuClipPath = nil

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
	if rc := c.gpuCtxOps(); rc != nil {
		rc.SetPipelineMode(mode)
	} else if a := Accelerator(); a != nil {
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

// SetTextMode sets the text rendering strategy.
// See TextMode constants for available strategies.
//
// The mode is per-Context — different contexts can use different strategies.
func (c *Context) SetTextMode(mode TextMode) {
	c.textMode = mode
}

// TextMode returns the current text rendering strategy.
func (c *Context) TextMode() TextMode {
	return c.textMode
}

// SetLCDLayout sets the LCD subpixel layout for ClearType text rendering.
// Use LCDLayoutRGB for most monitors, LCDLayoutBGR for rare BGR panels,
// or LCDLayoutNone to disable subpixel rendering (grayscale, the default).
//
// When a GPU accelerator is registered and implements LCDLayoutAware,
// the layout is propagated so the glyph mask engine rasterizes glyphs
// with 3x horizontal oversampling and the GPU uses the LCD fragment shader.
//
// The setting is per-Context. Call this before drawing text.
func (c *Context) SetLCDLayout(layout LCDLayout) {
	a := Accelerator()
	if a == nil {
		return
	}
	if la, ok := a.(LCDLayoutAware); ok {
		la.SetLCDLayout(layout)
	}
}

// Width returns the logical width of the context.
// This is the coordinate space used by drawing operations.
// For the physical pixel dimensions, use PixelWidth.
func (c *Context) Width() int {
	return c.width
}

// Height returns the logical height of the context.
// This is the coordinate space used by drawing operations.
// For the physical pixel dimensions, use PixelHeight.
func (c *Context) Height() int {
	return c.height
}

// PixelWidth returns the physical pixel width of the internal pixmap.
// This equals Width() * DeviceScale(), rounded to int.
// On non-HiDPI displays (scale=1.0), this equals Width().
func (c *Context) PixelWidth() int {
	return int(float64(c.width) * c.deviceScale)
}

// PixelHeight returns the physical pixel height of the internal pixmap.
// This equals Height() * DeviceScale(), rounded to int.
// On non-HiDPI displays (scale=1.0), this equals Height().
func (c *Context) PixelHeight() int {
	return int(float64(c.height) * c.deviceScale)
}

// DeviceScale returns the device scale factor (physical pixels per logical pixel).
// Default is 1.0. On Retina/HiDPI displays, typical values are 2.0 or 3.0.
func (c *Context) DeviceScale() float64 {
	return c.deviceScale
}

// SetDeviceScale changes the device scale factor on an existing context.
// This reallocates the internal pixmap at the new physical resolution
// and adjusts the base transform. The logical dimensions (Width, Height)
// remain unchanged.
//
// Use this when the window moves to a display with a different scale factor.
// Scale must be > 0; values <= 0 are ignored.
func (c *Context) SetDeviceScale(scale float64) {
	if scale <= 0 || scale == c.deviceScale {
		return
	}

	oldScale := c.deviceScale
	c.deviceScale = scale

	// Physical dimensions
	pw := int(float64(c.width) * scale)
	ph := int(float64(c.height) * scale)

	Logger().Info("SetDeviceScale",
		"old_scale", oldScale, "new_scale", scale,
		"logical_w", c.width, "logical_h", c.height,
		"physical_w", pw, "physical_h", ph,
	)

	// Reallocate pixmap at new physical resolution
	c.pixmap = NewPixmap(pw, ph)

	// Update renderer dimensions and device scale
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.Resize(pw, ph)
		sr.SetDeviceScale(float32(scale))
	}

	// Update device matrix. User matrix (c.matrix) is NOT touched —
	// it contains only user transforms and is independent of device scale.
	c.deviceMatrix = Identity()
	if scale != 1.0 {
		c.deviceMatrix = Scale(scale, scale)
	}

	// Reset clip stack (clip regions are in pixel coordinates)
	c.clipStack = nil
	c.gpuClipPath = nil
	c.ClearPath()
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

// Clear resets the entire context to transparent (zero alpha).
// To fill with a specific background color, use [ClearWithColor].
func (c *Context) Clear() {
	c.pixmap.Clear(Transparent)
}

// ClearWithColor fills the entire context with the specified color.
// This is the recommended way to set a background color before drawing.
func (c *Context) ClearWithColor(col RGBA) {
	c.pixmap.Clear(col)
}

// maxDamageRects is the threshold above which individual rects are merged
// into a single bounding box. Too many small rects = too many OS blit calls.
// Wayland/Android compositors use similar thresholds.
const maxDamageRects = 16

// FrameDamage returns the list of damage rectangles from draw operations
// this frame. Each rect corresponds to one or more Fill/Stroke operations.
// Used by ggcanvas → SetDamageRects → PresentWithDamage for per-rect OS blit.
// Returns nil if no drawing operations occurred.
func (c *Context) FrameDamage() []image.Rectangle {
	if len(c.frameDamageRects) == 0 {
		return nil
	}
	return c.frameDamageRects
}

// FrameDamageUnion returns the bounding box of all damage rects this frame.
// Convenience method for debug display or single-rect consumers.
func (c *Context) FrameDamageUnion() image.Rectangle {
	var r image.Rectangle
	for _, dr := range c.frameDamageRects {
		r = r.Union(dr)
	}
	return r
}

// ResetFrameDamage clears the per-frame damage accumulator.
// Call at the start of each frame before drawing operations.
func (c *Context) ResetFrameDamage() {
	c.frameDamageRects = c.frameDamageRects[:0]
}

// SetDamageTracking enables or disables per-operation damage recording.
// When disabled, Fill/Stroke do not append to FrameDamage.
// Used by retained-mode compositors to suppress damage during replay
// of cached (clean) scene content (ADR-021 false positive fix).
func (c *Context) SetDamageTracking(enabled bool) {
	c.damageTrackingEnabled = enabled
}

// TrackDamageRect registers an external damage rectangle on the surface.
// Use this for compositor operations that modify the surface but don't use
// Fill/Stroke (e.g., DrawGPUTexture for dirty RepaintBoundary overlays).
// No-op when damage tracking is disabled or rect is empty.
//
// Callers with retained-mode knowledge (e.g., ui widget tree) should call
// this for each dirty boundary after compositing, so that FrameDamage()
// accurately reflects which surface regions changed this frame.
func (c *Context) TrackDamageRect(bounds image.Rectangle) {
	c.trackDamage(bounds)
}

// trackDamage adds a damage rectangle for the current draw operation.
// No-op when damage tracking is disabled (cached scene replay).
// If rect count exceeds maxDamageRects, merges all into bounding box.
func (c *Context) trackDamage(bounds image.Rectangle) {
	if !c.damageTrackingEnabled || bounds.Empty() {
		return
	}
	c.frameDamageRects = append(c.frameDamageRects, bounds)
	if len(c.frameDamageRects) > maxDamageRects {
		merged := c.frameDamageRects[0]
		for _, r := range c.frameDamageRects[1:] {
			merged = merged.Union(r)
		}
		c.frameDamageRects = c.frameDamageRects[:1]
		c.frameDamageRects[0] = merged
	}
}

// FillRectCPU fills a rectangle directly on the CPU pixmap without engaging
// the GPU SDF accelerator. Coordinates are in user space (device scale applied
// automatically). Pending GPU shapes are flushed first for correct z-ordering.
//
// Use for operations where GPU acceleration is counterproductive, such as
// dirty-region background clearing in retained-mode compositors. Without this,
// DrawRectangle+Fill routes through SDF accelerator → blocks non-MSAA blit path.
//
// See ADR-016, TASK-GG-COMPOSITOR-003.
func (c *Context) FillRectCPU(x, y, w, h float64, col RGBA) {
	c.flushGPUAccelerator()

	ctm := c.totalMatrix()
	tl := ctm.TransformPoint(Pt(x, y))
	br := ctm.TransformPoint(Pt(x+w, y+h))

	px0 := int(tl.X)
	py0 := int(tl.Y)
	px1 := int(br.X + 0.5)
	py1 := int(br.Y + 0.5)

	pr := uint8(clamp255(col.R * col.A * 255))
	pg := uint8(clamp255(col.G * col.A * 255))
	pb := uint8(clamp255(col.B * col.A * 255))
	pa := uint8(clamp255(col.A * 255))

	c.pixmap.FillRect(image.Rect(px0, py0, px1, py1), pr, pg, pb, pa)
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

// SetPath replaces the current path with p.
// The path is copied — subsequent modifications to p do not affect the context.
// Use this to render pre-built paths (e.g., from ParseSVGPath):
//
//	path, _ := gg.ParseSVGPath("M10,10 L90,10 L90,90 Z")
//	dc.SetPath(path)
//	dc.Fill()
func (c *Context) SetPath(p *Path) {
	c.path.Clear()
	if p != nil {
		c.path.Append(p)
	}
}

// AppendPath appends the elements of p to the current path without clearing it.
// This allows combining multiple sub-paths before a single Fill or Stroke call.
// Note: path coordinates are copied as-is (not transformed by the current matrix).
// Use DrawPath for transform-aware path rendering.
func (c *Context) AppendPath(p *Path) {
	if p != nil {
		c.path.Append(p)
	}
}

// DrawPath replays the elements of p through the current transform matrix,
// replacing the current path. Unlike SetPath (which copies raw coordinates),
// DrawPath applies the current matrix (Translate, Scale, Rotate) to all points.
// After DrawPath, call Fill() or Stroke() to render.
//
// This is the correct way to render pre-built paths (e.g., from ParseSVGPath)
// with transforms:
//
//	path, _ := gg.ParseSVGPath("M10,10 L90,10 L90,90 Z")
//	dc.Push()
//	dc.Translate(x, y)
//	dc.Scale(0.5, 0.5)
//	dc.DrawPath(path)
//	dc.Fill()
//	dc.Pop()
func (c *Context) DrawPath(p *Path) {
	c.ClearPath()
	if p == nil {
		return
	}
	p.Iterate(func(verb PathVerb, coords []float64) {
		switch verb {
		case MoveTo:
			c.MoveTo(coords[0], coords[1])
		case LineTo:
			c.LineTo(coords[0], coords[1])
		case QuadTo:
			c.QuadraticTo(coords[0], coords[1], coords[2], coords[3])
		case CubicTo:
			c.CubicTo(coords[0], coords[1], coords[2], coords[3], coords[4], coords[5])
		case Close:
			c.ClosePath()
		}
	})
}

// FillPath is a convenience method that replays path p through the current
// transform, fills it, and clears the path. Equivalent to DrawPath(p) + Fill().
func (c *Context) FillPath(p *Path) error {
	c.DrawPath(p)
	return c.Fill()
}

// StrokePath is a convenience method that replays path p through the current
// transform, strokes it, and clears the path. Equivalent to DrawPath(p) + Stroke().
func (c *Context) StrokePath(p *Path) error {
	c.DrawPath(p)
	return c.Stroke()
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
	c.trackDamage(c.path.Bounds())
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
	c.trackDamage(c.path.Bounds())
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
			// Clear GPU clip path if all path clips were popped.
			if c.gpuClipPath != nil && c.clipStack.IsRRectOnly() {
				c.gpuClipPath = nil
			}
		}
	}

	// Restore mask
	if len(c.maskStack) > 0 {
		c.mask = c.maskStack[len(c.maskStack)-1]
		c.maskStack = c.maskStack[:len(c.maskStack)-1]
	}
}

// Identity resets the user transformation matrix to the identity matrix.
// Device scale is applied separately at rendering boundaries (not in the CTM),
// so Identity() always resets to a pure identity matrix regardless of scale.
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
// Uses logical height so the inversion works correctly at any device scale.
func (c *Context) InvertY() {
	c.Translate(0, float64(c.height))
	c.Scale(1, -1)
}

// totalMatrix returns the combined device + user transform matrix.
// Used at rendering boundaries where device-space coordinates are needed.
// At scale=1.0, this is identical to c.matrix (zero overhead).
func (c *Context) totalMatrix() Matrix {
	if c.deviceMatrix.IsIdentity() {
		return c.matrix
	}
	return c.deviceMatrix.Multiply(c.matrix)
}

// deviceSpacePath returns the current path transformed to device-space.
// Path coordinates are in user-space (transformed by c.matrix only).
// The renderer operates in device-space, so we apply deviceMatrix here.
// At scale=1.0, returns the original path (zero copy).
func (c *Context) deviceSpacePath() *Path {
	if c.deviceMatrix.IsIdentity() {
		return c.path
	}
	return c.path.Transform(c.deviceMatrix)
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
//
// The corner radius r is clamped to half the smaller dimension.
// All coordinates are transformed through the current matrix,
// ensuring correct rendering on HiDPI/Retina displays.
func (c *Context) DrawRoundedRectangle(x, y, w, h, r float64) {
	maxR := math.Min(w, h) / 2
	if r > maxR {
		r = maxR
	}
	// Cubic Bézier approximation for 90° arcs (same constant as DrawCircle).
	const k = 0.5522847498307936
	kr := k * r
	// Top edge
	c.MoveTo(x+r, y)
	c.LineTo(x+w-r, y)
	// Top-right corner
	c.CubicTo(x+w-r+kr, y, x+w, y+r-kr, x+w, y+r)
	// Right edge
	c.LineTo(x+w, y+h-r)
	// Bottom-right corner
	c.CubicTo(x+w, y+h-r+kr, x+w-r+kr, y+h, x+w-r, y+h)
	// Bottom edge
	c.LineTo(x+r, y+h)
	// Bottom-left corner
	c.CubicTo(x+r-kr, y+h, x, y+h-r+kr, x, y+h-r)
	// Left edge
	c.LineTo(x, y+r)
	// Top-left corner
	c.CubicTo(x, y+r-kr, x+r-kr, y, x+r, y)
	c.ClosePath()
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

	if c.path.isEmpty() {
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

// Resize changes the context logical dimensions, reusing internal buffers where possible.
// If the dimensions haven't changed, this is a no-op.
// Returns an error if width or height is <= 0.
//
// The width and height are logical dimensions. The internal pixmap is
// allocated at physical resolution (width*deviceScale x height*deviceScale).
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

	// Update logical dimensions
	c.width = width
	c.height = height

	// Physical dimensions
	pw := int(float64(width) * c.deviceScale)
	ph := int(float64(height) * c.deviceScale)

	// Reallocate pixmap at physical resolution
	c.pixmap = NewPixmap(pw, ph)

	// Resize renderer if it supports resizing
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.Resize(pw, ph)
	}

	// Reset clip stack to full rectangle
	c.clipStack = nil
	c.gpuClipPath = nil

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
// SetSharedEncoder sets a shared command encoder for single-command-buffer
// frames (ADR-017, Flutter Impeller pattern). When set, FlushGPU/FlushGPUWithView
// record render passes into this encoder instead of creating their own and
// submitting. The caller is responsible for encoder.Finish() + queue.Submit().
//
// Pass a zero-value CommandEncoder (IsNil() == true) to restore normal
// per-context submit behavior.
func (c *Context) SetSharedEncoder(encoder gpucontext.CommandEncoder) {
	if rc := c.gpuCtxOps(); rc != nil {
		type encoderSetter interface {
			SetSharedEncoder(encoder gpucontext.CommandEncoder)
		}
		if es, ok := rc.(encoderSetter); ok {
			es.SetSharedEncoder(encoder)
		}
	}
}

// CreateSharedEncoder creates a command encoder for single-command-buffer
// frames (ADR-017). Multiple gg.Contexts record render passes into this
// encoder via SetSharedEncoder. Call SubmitSharedEncoder after all contexts
// have flushed to submit in one GPU call.
// Returns a zero-value CommandEncoder (IsNil() == true) if GPU is not available.
func (c *Context) CreateSharedEncoder() gpucontext.CommandEncoder {
	rc := c.gpuCtxOps()
	if rc == nil {
		return gpucontext.CommandEncoder{}
	}
	type encoderCreator interface {
		CreateEncoder() gpucontext.CommandEncoder
	}
	if ec, ok := rc.(encoderCreator); ok {
		return ec.CreateEncoder()
	}
	return gpucontext.CommandEncoder{}
}

// SubmitSharedEncoder finishes the shared encoder and submits the resulting
// command buffer to the GPU. Call after all contexts have flushed their
// render passes into the encoder. Returns the command buffer submission
// index for fence tracking, or error.
func (c *Context) SubmitSharedEncoder(encoder gpucontext.CommandEncoder) error {
	rc := c.gpuCtxOps()
	if rc == nil {
		return nil
	}
	type encoderSubmitter interface {
		SubmitEncoder(encoder gpucontext.CommandEncoder) error
	}
	if es, ok := rc.(encoderSubmitter); ok {
		return es.SubmitEncoder(encoder)
	}
	return nil
}

// BeginGPUFrame resets per-context GPU frame state so the next render pass
// uses LoadOpClear. Call this on persistent contexts before re-rendering
// to the same view — without it, frameRendered=true from the previous frame
// causes LoadOpLoad, preserving stale content.
//
// Not needed for one-shot contexts (NewContext + Close per frame).
// Not needed when the view changes between frames (auto-reset on view change).
func (c *Context) BeginGPUFrame() {
	if rc := c.gpuCtxOps(); rc != nil {
		rc.BeginFrame()
	}
}

func (c *Context) FlushGPU() error {
	t := c.gpuRenderTarget()
	if rc := c.gpuCtxOps(); rc != nil {
		return rc.Flush(t)
	}
	if a := Accelerator(); a != nil {
		c.warnGPUFallback("FlushGPU")
		return a.Flush(t)
	}
	return nil
}

// FlushGPUWithView flushes pending GPU operations, resolving directly to the
// given texture view instead of reading back to CPU. The view is passed
// through GPURenderTarget.View so the render session uses it as the per-pass
// resolve target, enabling multiple Contexts to render to different views
// without cross-contamination.
//
// This is the per-pass render target path for ggcanvas.RenderDirect.
// When view is nil/zero, behaves identically to FlushGPU (CPU readback).
func (c *Context) FlushGPUWithView(view gpucontext.TextureView, width, height uint32) error {
	t := c.gpuRenderTarget()
	if !view.IsNil() {
		t.View = view
		t.ViewWidth = width
		t.ViewHeight = height
	}
	rc := c.gpuCtxOps()
	if rc != nil {
		return rc.Flush(t)
	}
	if a := Accelerator(); a != nil {
		c.warnGPUFallback("FlushGPUWithView")
		return a.Flush(t)
	}
	return ErrFallbackToCPU
}

// FlushGPUWithViewDamage flushes pending GPU operations with damage-aware
// optimization. When damageRect is non-empty, the compositor uses LoadOpLoad
// (preserves previous frame) and scissor-clips to the dirty region — only
// the damaged pixels are re-composited. When damageRect is empty, behaves
// identically to FlushGPUWithView (full compositor pass).
//
// IMPORTANT: damage-aware rendering (LoadOpLoad + scissor) works only on the
// blit-only compositor path (frames with only DrawGPUTextureBase/DrawGPUTexture
// calls, no vector shapes). When the frame contains Fill/Stroke operations,
// the MSAA render path is used which always does LoadOpClear — damageRect is
// ignored and a warning is logged. This matches enterprise practice: Chrome,
// Flutter, and Skia all re-render dirty layers fully via MSAA and composite
// incrementally via blit-only path. See ADR-021.
//
// This enables sub-region compositing: a 48×48 spinner updates only 9KB
// instead of the full surface (8MB at 1080p). See ADR-016 Phase 2.
func (c *Context) FlushGPUWithViewDamage(view gpucontext.TextureView, width, height uint32, damageRect image.Rectangle) error {
	t := c.gpuRenderTarget()
	if !view.IsNil() {
		t.View = view
		t.ViewWidth = width
		t.ViewHeight = height
	}
	if !damageRect.Empty() {
		t.DamageRects = []image.Rectangle{damageRect}
	}
	if rc := c.gpuCtxOps(); rc != nil {
		return rc.Flush(t)
	}
	if a := Accelerator(); a != nil {
		c.warnGPUFallback("FlushGPUWithViewDamage")
		return a.Flush(t)
	}
	return nil
}

// FlushGPUWithViewDamageRects renders to a surface view with multiple damage rects
// (ADR-028). Each overlay gets its own scissor from the closest damage rect,
// enabling per-draw dynamic scissor for distant dirty regions.
func (c *Context) FlushGPUWithViewDamageRects(view gpucontext.TextureView, width, height uint32, rects []image.Rectangle) error {
	t := c.gpuRenderTarget()
	if !view.IsNil() {
		t.View = view
		t.ViewWidth = width
		t.ViewHeight = height
	}
	if len(rects) > 0 {
		t.DamageRects = rects
	}
	if rc := c.gpuCtxOps(); rc != nil {
		return rc.Flush(t)
	}
	if a := Accelerator(); a != nil {
		c.warnGPUFallback("FlushGPUWithViewDamageRects")
		return a.Flush(t)
	}
	return nil
}

// gpuContextOps is the per-context GPU rendering interface.
// GPURenderContext (internal/gpu) implements this, allowing context.go
// to route draw calls through the per-context queue without importing internal/gpu.
type gpuContextOps interface {
	FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error
	StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error
	FillPath(target GPURenderTarget, path *Path, paint *Paint) error
	StrokePath(target GPURenderTarget, path *Path, paint *Paint) error
	DrawText(target GPURenderTarget, face any, s string, x, y float64, color RGBA, matrix Matrix, deviceScale float64) error
	DrawGlyphMaskText(target GPURenderTarget, face any, s string, x, y float64, color RGBA, matrix Matrix, deviceScale float64) error
	QueueImageDraw(target GPURenderTarget, pixelData []byte, genID uint64, imgWidth, imgHeight, imgStride int,
		dstX, dstY, dstW, dstH, opacity float32, viewportW, viewportH uint32,
		u0, v0, u1, v1 float32)
	QueueGPUTextureDraw(target GPURenderTarget, view gpucontext.TextureView,
		dstX, dstY, dstW, dstH, opacity float32, vpW, vpH uint32)
	QueueBaseLayer(target GPURenderTarget, view gpucontext.TextureView,
		dstX, dstY, dstW, dstH, opacity float32, vpW, vpH uint32)
	Flush(target GPURenderTarget) error
	SetClipRect(x, y, w, h uint32)
	ClearClipRect()
	SetClipRRect(x, y, w, h, radius float32)
	ClearClipRRect()
	SetClipPath(path *Path)
	ClearClipPath()
	BeginFrame()
	SetPipelineMode(mode PipelineMode)
	PendingCount() int
	Close()
}

// GPURenderContext returns the per-context GPU render context, lazily created.
// Returns nil if no GPU accelerator is registered or it does not support
// per-context rendering. The returned value should be type-asserted to
// *gpu.GPURenderContext in internal/gpu consumers.
func (c *Context) GPURenderContext() any {
	c.ensureGPUCtx()
	return c.gpuCtx
}

// ensureGPUCtx lazily creates the per-context GPU render context.
func (c *Context) ensureGPUCtx() {
	if c.gpuCtx != nil {
		return
	}
	a := Accelerator()
	if a == nil {
		return
	}
	p, ok := a.(GPURenderContextProvider)
	if !ok {
		return
	}
	rc := p.NewGPURenderContext()
	if rc == nil {
		return
	}
	ops, ok := rc.(gpuContextOps)
	if !ok {
		return
	}
	c.gpuCtx = ops
}

// gpuCtxOps returns the per-context GPU ops interface, or nil if unavailable.
func (c *Context) gpuCtxOps() gpuContextOps {
	c.ensureGPUCtx()
	return c.gpuCtx
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

// warnGPUFallback logs a one-time warning when a GPU operation falls back to
// the global accelerator instead of the per-context GPURenderContext. This
// indicates shape leaking risk in multi-context scenarios (RepaintBoundary).
func (c *Context) warnGPUFallback(op string) {
	if !c.gpuFallbackWarned {
		c.gpuFallbackWarned = true
		Logger().Warn("GPU operation using global accelerator instead of per-context — shapes may leak in multi-context",
			"op", op, "w", c.width, "h", c.height)
	}
}

// flushGPUAccelerator flushes pending GPU shapes before a CPU fallback operation.
func (c *Context) flushGPUAccelerator() {
	if rc := c.gpuCtxOps(); rc != nil {
		_ = rc.Flush(c.gpuRenderTarget())
		return
	}
	if a := Accelerator(); a != nil {
		c.warnGPUFallback("flushGPUAccelerator")
		_ = a.Flush(c.gpuRenderTarget())
	}
}

// tryGPUFill attempts to fill the current path using the GPU accelerator.
// When a mask is active and the accelerator implements MaskAware, the mask
// is uploaded as a GPU texture. Otherwise, falls back to CPU.
func (c *Context) tryGPUFill() error {
	cleanup, err := c.setupGPUMask()
	if err != nil {
		return err
	}
	defer cleanup()
	if rc := c.gpuCtxOps(); rc != nil {
		return c.tryGPUOpRC(rc.FillShape, rc.FillPath)
	}
	a := Accelerator()
	if a == nil {
		return ErrFallbackToCPU
	}
	c.warnGPUFallback("tryGPUFill")
	return c.tryGPUOp(a, a.FillShape, a.FillPath, AccelFill)
}

// tryGPUStroke attempts to stroke the current path using the GPU accelerator.
// When a mask is active and the accelerator implements MaskAware, the mask
// is uploaded as a GPU texture. Otherwise, falls back to CPU.
func (c *Context) tryGPUStroke() error {
	cleanup, err := c.setupGPUMask()
	if err != nil {
		return err
	}
	defer cleanup()
	if rc := c.gpuCtxOps(); rc != nil {
		return c.tryGPUOpRC(rc.StrokeShape, rc.StrokePath)
	}
	a := Accelerator()
	if a == nil {
		return ErrFallbackToCPU
	}
	c.warnGPUFallback("tryGPUStroke")
	return c.tryGPUOp(a, a.StrokeShape, a.StrokePath, AccelStroke)
}

// setupGPUMask uploads the active alpha mask to the GPU accelerator.
// Returns a cleanup function to clear the mask (must be deferred by caller).
func (c *Context) setupGPUMask() (func(), error) {
	if c.mask == nil {
		return func() {}, nil
	}
	a := Accelerator()
	if a == nil {
		return func() {}, nil
	}
	ma, ok := a.(MaskAware)
	if !ok {
		return nil, ErrFallbackToCPU
	}
	ma.SetMaskTexture(c.mask.Data(), c.mask.Width(), c.mask.Height())
	return ma.ClearMaskTexture, nil
}

// tryGPUOpRC routes GPU operations through the per-context GPURenderContext.
func (c *Context) tryGPUOpRC(
	shapeFn func(GPURenderTarget, DetectedShape, *Paint) error,
	pathFn func(GPURenderTarget, *Path, *Paint) error,
) error {
	target := c.gpuRenderTarget()

	shape := DetectShape(c.path)
	if accel := sdfAccelForShape(shape.Kind); accel != 0 {
		if err := shapeFn(target, shape, c.paint); err == nil {
			return nil
		}
	}

	return pathFn(target, c.path, c.paint)
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

	// Set GPU scissor rect for rectangular clips.
	defer c.setGPUClipRect()()

	// Set clip/mask coverage BEFORE GPU attempt so that CPU SDF fallback
	// (SDFAccelerator.FillShape) can apply per-pixel clip+mask coverage.
	// The GPU render-pass path ignores paint.ClipCoverage (uses shader-based
	// clipping), so setting it early is harmless for the GPU path.
	c.applyClipToPaint()
	defer func() { c.paint.ClipCoverage = nil }()

	c.applyMaskToPaint()
	defer func() { c.paint.MaskCoverage = nil }()

	// Transform path to device-space for rendering.
	// At scale=1.0 this is a zero-copy no-op.
	devicePath := c.deviceSpacePath()

	// Temporarily swap c.path to device-space for GPU tryGPUOp
	// (which reads c.path for shape detection and path rendering).
	origPath := c.path
	c.path = devicePath
	ok, cpuMode := c.tryGPUFillWithMode(mode)
	c.path = origPath
	if ok {
		return nil
	}

	// CPU path: flush pending GPU, apply mode to software renderer.
	c.flushGPUAccelerator()
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.rasterizerMode = cpuMode
		defer func() { sr.rasterizerMode = RasterizerAuto }()
	}

	return c.renderer.Fill(c.pixmap, devicePath, c.paint)
}

// doStroke performs the stroke operation respecting the current RasterizerMode.
func (c *Context) doStroke() error {
	c.paint.TransformScale = c.totalMatrix().ScaleFactor()
	mode := c.rasterizerMode

	// Set GPU scissor rect for rectangular clips.
	defer c.setGPUClipRect()()

	// Set clip/mask coverage BEFORE GPU attempt so that CPU SDF fallback
	// (SDFAccelerator.StrokeShape) can apply per-pixel clip+mask coverage.
	c.applyClipToPaint()
	defer func() { c.paint.ClipCoverage = nil }()

	c.applyMaskToPaint()
	defer func() { c.paint.MaskCoverage = nil }()

	// Transform path to device-space for rendering.
	devicePath := c.deviceSpacePath()

	// Temporarily swap c.path to device-space for GPU tryGPUOp.
	origPath := c.path
	c.path = devicePath
	ok, cpuMode := c.tryGPUStrokeWithMode(mode)
	c.path = origPath
	if ok {
		return nil
	}

	c.flushGPUAccelerator()
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.rasterizerMode = cpuMode
		defer func() { sr.rasterizerMode = RasterizerAuto }()
	}

	return c.renderer.Stroke(c.pixmap, devicePath, c.paint)
}

// applyClipToPaint sets the ClipCoverage function on the paint when a clip
// stack is active and has entries. This allows the renderer to apply per-pixel
// clip masks during compositing.
func (c *Context) applyClipToPaint() {
	if c.clipStack == nil || c.clipStack.Depth() == 0 {
		return
	}
	cs := c.clipStack
	c.paint.ClipCoverage = func(x, y float64) byte {
		return cs.Coverage(x, y)
	}
}

// applyMaskToPaint sets the MaskCoverage function on the paint when an alpha
// mask is active. This allows the renderer to apply per-pixel mask modulation
// during compositing. Mask and clip compose multiplicatively.
func (c *Context) applyMaskToPaint() {
	if c.mask == nil {
		return
	}
	m := c.mask
	c.paint.MaskCoverage = func(x, y int) uint8 {
		return m.At(x, y)
	}
}

// isClipActive reports whether a clip region is currently active.
func (c *Context) isClipActive() bool {
	return c.clipStack != nil && c.clipStack.Depth() > 0
}

// setGPUClipRect sets the GPU scissor rect, RRect clip, or depth clip path if a
// clip region is active. Returns a cleanup function that must be deferred to
// clear the clip state. Handles three cases:
//
//  1. Rect-only clip → hardware scissor rect (free, zero per-pixel cost)
//  2. RRect clip → scissor rect (bounding box) + SDF in fragment shader
//  3. Path clip → scissor rect (bounding box) + depth buffer (GPU-CLIP-003a)
//
// If no clip is active or the accelerator doesn't support ClipAware, the
// returned function is a no-op.
func (c *Context) setGPUClipRect() func() {
	if !c.isClipActive() {
		return func() {}
	}
	rectOnly := c.clipStack.IsRectOnly()
	rrectOnly := c.clipStack.IsRRectOnly()

	// Arbitrary path clip → GPU depth clipping if available.
	if !rectOnly && !rrectOnly {
		return c.setGPUClipPath()
	}

	bounds := c.clipStack.Bounds()
	x0 := uint32(math.Floor(bounds.X))
	y0 := uint32(math.Floor(bounds.Y))
	x1 := uint32(math.Ceil(bounds.X + bounds.W))
	y1 := uint32(math.Ceil(bounds.Y + bounds.H))
	if x1 <= x0 || y1 <= y0 {
		return func() {}
	}

	// Per-context path (GPURenderContext available)
	if rc := c.gpuCtxOps(); rc != nil {
		rc.SetClipRect(x0, y0, x1-x0, y1-y0)
		if !rectOnly {
			rrBounds, radius, hasRRect := c.clipStack.RRectBounds()
			if hasRRect {
				rc.SetClipRRect(
					float32(rrBounds.X), float32(rrBounds.Y),
					float32(rrBounds.W), float32(rrBounds.H),
					float32(radius),
				)
				return func() {
					rc.ClearClipRect()
					rc.ClearClipRRect()
				}
			}
		}
		return func() { rc.ClearClipRect() }
	}

	// Fallback: global accelerator (backward compat for mock accelerators)
	a := Accelerator()
	if a == nil {
		return func() {}
	}
	ca, ok := a.(ClipAware)
	if !ok {
		return func() {}
	}
	ca.SetClipRect(x0, y0, x1-x0, y1-y0)
	if !rectOnly {
		if rca, ok2 := a.(RRectClipAware); ok2 {
			rrBounds, radius, hasRRect := c.clipStack.RRectBounds()
			if hasRRect {
				rca.SetClipRRect(
					float32(rrBounds.X), float32(rrBounds.Y),
					float32(rrBounds.W), float32(rrBounds.H),
					float32(radius),
				)
				return func() {
					ca.ClearClipRect()
					rca.ClearClipRRect()
				}
			}
		}
	}
	return func() { ca.ClearClipRect() }
}

// setGPUClipPath handles arbitrary path clips by sending the device-space clip
// path to the GPU depth clip pipeline (GPU-CLIP-003a). Returns a cleanup
// function that clears the depth clip state after drawing completes.
// If no GPU context is available or no clip path is stored, returns a no-op.
func (c *Context) setGPUClipPath() func() {
	if c.gpuClipPath == nil {
		return func() {}
	}
	rc := c.gpuCtxOps()
	if rc == nil {
		return func() {}
	}
	// Set scissor rect to clip bounding box (coarse clip, free).
	bounds := c.clipStack.Bounds()
	x0 := uint32(math.Floor(bounds.X))
	y0 := uint32(math.Floor(bounds.Y))
	x1 := uint32(math.Ceil(bounds.X + bounds.W))
	y1 := uint32(math.Ceil(bounds.Y + bounds.H))
	if x1 > x0 && y1 > y0 {
		rc.SetClipRect(x0, y0, x1-x0, y1-y0)
	}
	// Set depth clip path for fine per-pixel clipping.
	rc.SetClipPath(c.gpuClipPath)
	return func() {
		rc.ClearClipPath()
		rc.ClearClipRect()
	}
}

// tryGPUFillWithMode attempts GPU fill based on the rasterizer mode.
// Returns (true, _) if GPU handled the fill, or (false, cpuMode) with the
// fallback CPU mode to use.
func (c *Context) tryGPUFillWithMode(mode RasterizerMode) (bool, RasterizerMode) {
	if mode == RasterizerSDF {
		c.setForceSDF(true)
		err := c.tryGPUFill()
		c.setForceSDF(false)
		if err == nil {
			return true, mode
		}
		mode = RasterizerAuto // Non-SDF shape → auto CPU fallback.
	}
	if mode == RasterizerAuto {
		if err := c.tryGPUFill(); err == nil {
			return true, mode
		}
	}
	return false, mode
}

// tryGPUStrokeWithMode attempts GPU stroke based on the rasterizer mode.
// Returns (true, _) if GPU handled the stroke, or (false, cpuMode) with the
// fallback CPU mode to use.
func (c *Context) tryGPUStrokeWithMode(mode RasterizerMode) (bool, RasterizerMode) {
	if mode == RasterizerSDF {
		c.setForceSDF(true)
		err := c.tryGPUStroke()
		c.setForceSDF(false)
		if err == nil {
			return true, mode
		}
		mode = RasterizerAuto
	}
	if mode == RasterizerAuto {
		if err := c.tryGPUStroke(); err == nil {
			return true, mode
		}
	}
	return false, mode
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
